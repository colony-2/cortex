package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/colony-2/colony2/server/api/pkg/web"
	"github.com/colony-2/colony2/server/cortex/internal/config"
	"github.com/colony-2/colony2/server/cortex/internal/shared"
	"github.com/colony-2/colony2/server/cortex/internal/static"
	"github.com/colony-2/colony2/server/git/pkg/git"
	"github.com/colony-2/colony2/server/graph/pkg/graph"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	inputops "github.com/colony-2/colony2/server/recipe-input/pkg/input"
	"github.com/colony-2/colony2/server/storage/pkg/storage"
	"github.com/colony-2/colony2/server/ticket/pkg/database"
)

// InitializeDependencies initializes all application dependencies
func InitializeDependencies(ctx context.Context, cfg config.Config) (web.Dependencies, func(), error) {
	var cleanup []func()

	// Initialize storage with default database path, allow override for tests/CI
	dbDir := os.Getenv("VIBETHIS_DB_DIR")
	if dbDir == "" {
		dbDir = filepath.Join(cfg.RootPath, ".colony2")
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
		DefaultAuthor: "github.com/colony-2/colony2/server",
		DefaultEmail:  "github.com/colony-2/colony2/server@example.com",
	})

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
