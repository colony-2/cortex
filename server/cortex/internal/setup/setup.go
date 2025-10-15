package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/divisive-ai/vibethis/server/api/pkg/web"
	"github.com/divisive-ai/vibethis/server/container/pkg/container"
	"github.com/divisive-ai/vibethis/server/cortex/internal/config"
	"github.com/divisive-ai/vibethis/server/cortex/internal/shared"
	"github.com/divisive-ai/vibethis/server/cortex/internal/static"
	"github.com/divisive-ai/vibethis/server/files/pkg/files"
	"github.com/divisive-ai/vibethis/server/git/pkg/git"
	"github.com/divisive-ai/vibethis/server/graph/pkg/graph"
	inputops "github.com/divisive-ai/vibethis/server/ops/pkg/input"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/divisive-ai/vibethis/server/storage/pkg/storage"
	"github.com/divisive-ai/vibethis/server/ticket/pkg/database"
	"go.temporal.io/sdk/testsuite"
)

// WorkflowControl implements ops.ServiceDependencies2 using a suite-backed controller when available.

// suiteWorkflowCtl adapts the Temporal WorkflowTestSuite environment to workflowctl.WorkflowControl
type suiteWorkflowCtl struct {
	env *testsuite.TestWorkflowEnvironment
}

func (c *suiteWorkflowCtl) Describe(ctx context.Context, ref workflowctl.ExecutionRef) (workflowctl.WorkflowSummary, error) {
	status := workflowctl.StatusRunning
	if c.env.IsWorkflowCompleted() {
		status = workflowctl.StatusCompleted
	}
	return workflowctl.WorkflowSummary{WorkflowID: ref.WorkflowID, Status: status}, nil
}

func (c *suiteWorkflowCtl) Signal(ctx context.Context, ref workflowctl.ExecutionRef, signalName string, payload any) error {
	c.env.SignalWorkflow(signalName, payload)
	return nil
}

func (c *suiteWorkflowCtl) Cancel(ctx context.Context, ref workflowctl.ExecutionRef, reason string) error {
	return nil
}

func (c *suiteWorkflowCtl) ResetWorkflow(ctx context.Context, req workflowctl.ResetRequest) (workflowctl.ResetResponse, error) {
	return workflowctl.ResetResponse{Execution: req.Execution, Completed: req.WaitForResult}, nil
}

func (c *suiteWorkflowCtl) StartWorkflow(ctx context.Context, req workflowctl.StartRequest) (workflowctl.StartResponse, error) {
	return workflowctl.StartResponse{Execution: workflowctl.ExecutionRef{WorkflowID: req.WorkflowID}}, nil
}

func (c *suiteWorkflowCtl) StartChildWorkflow(ctx context.Context, req workflowctl.StartChildRequest) (workflowctl.StartChildResponse, error) {
	return workflowctl.StartChildResponse{Execution: workflowctl.ExecutionRef{WorkflowID: req.WorkflowID}}, nil
}

// InitializeDependencies initializes all application dependencies
func InitializeDependencies(ctx context.Context, cfg config.Config) (web.Dependencies, func(), error) {
	var cleanup []func()

	// Initialize storage with default database path, allow override for tests/CI
	dbDir := os.Getenv("VIBETHIS_DB_DIR")
	if dbDir == "" {
		dbDir = filepath.Join(cfg.RootPath, ".vibethis")
	}

	// Ensure database directory exists when creating new or when overridden via env
	if cfg.CreateNew || os.Getenv("VIBETHIS_DB_DIR") != "" {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return web.Dependencies{}, nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	storageImpl, err := storage.NewBoltStorage(storage.Config{
		DatabasePath: dbDir,
		ReadOnly:     false,
	})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	cleanup = append(cleanup, func() { storageImpl.Close() })

	ticketDBPath := filepath.Join(cfg.RootPath, "ticket.db")
	ticketDB, closeTicketDB, err := database.Open(database.Config{FallbackPath: ticketDBPath})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to open ticket database: %w", err)
	}
	cleanup = append(cleanup, func() {
		if closeTicketDB != nil {
			if cerr := closeTicketDB(); cerr != nil {
				fmt.Fprintf(os.Stderr, "ticket database close failed: %v\n", cerr)
			}
		}
	})

	// Initialize graph builder
	graphBuilder := graph.NewBuilder(cfg.RootPath)

	// Initialize file browser
	fileBrowser := files.NewBrowser(files.Config{})

	// Initialize git repository
	gitRepo := git.NewRepository(git.Config{
		DefaultAuthor: "github.com/divisive-ai/vibethis/server",
		DefaultEmail:  "github.com/divisive-ai/vibethis/server@example.com",
	})

	// Initialize container manager
	containerMgr := container.NewManager(container.Config{})
	// Provide a basic SSE manager and a WorkflowTestSuite-backed controller
	sseMgr := inputops.NewSimpleSSEManager()
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	depsContainer := coreops.NewServiceDepsBuilder().
		WithSSEManager(sseMgr).
		WithWorkflowControl(&suiteWorkflowCtl{env: env}).
		WithDatabase(ticketDB).
		Build()

	extensionRoutes, cleanup, err := shared.SetupOps(depsContainer)
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to setup ops: %w", err)
	}

	// Get static filesystem (will be embedded in production builds)
	staticFS, err := static.GetFileSystem()
	if err != nil {
		// Log warning but continue - server can still work without static files
		fmt.Fprintf(os.Stderr, "Warning: Could not load static files: %v\n", err)
	}

	deps := web.Dependencies{
		Storage:         storageImpl,
		Graph:           graphBuilder,
		Files:           fileBrowser,
		Git:             gitRepo,
		Container:       containerMgr,
		StaticFS:        staticFS,
		ExtensionRoutes: extensionRoutes,
	}

	cleanupFunc := func() {
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
	}

	return deps, cleanupFunc, nil
}

// CreateServer creates the HTTP server
func CreateServer(cfg config.Config, deps web.Dependencies) (*web.Server, error) {
	// Use default values for removed configuration
	webConfig := web.Config{
		Port:        cfg.Port,
		CORSOrigins: []string{"http://localhost:3000", "http://localhost:5173"},
		StaticPath:  "embedded", // Default to embedded static files
	}

	server := web.NewServer(webConfig, deps)
	return server, nil
}
