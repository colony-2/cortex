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
    "github.com/divisive-ai/vibethis/server/storage/pkg/storage"
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
    inputops "github.com/divisive-ai/vibethis/server/ops/pkg/input"
    temporalclient "go.temporal.io/sdk/client"
)

type svc struct { sse interface{} }

func (s *svc) Get(name string) (interface{}, error) {
    switch name {
    case "sse":
        if s.sse != nil {
            return s.sse, nil
        }
        return nil, fmt.Errorf("service not found: %s", name)
    case "temporal_client":
        var c temporalclient.Client = nil
        return c, nil
    default:
        return nil, fmt.Errorf("service not found: %s", name)
    }
}

// WorkflowControl implements ops.ServiceDependencies2. No controller available here.
func (s *svc) WorkflowControl() (workflowctl.WorkflowControl, bool) { return nil, false }

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
    // Provide a basic SSE manager so input management routes can initialize
    sseMgr := inputops.NewSimpleSSEManager()
    svcContext := &svc{sse: sseMgr}

	extensionRoutes, cleanup, err := shared.SetupOps(svcContext)
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
