package setup

import (
	"context"
	"fmt"
	"os"

	"vibethis/api/pkg/web"
	"vibethis/container/pkg/container"
	"vibethis/files/pkg/files"
	"vibethis/git/pkg/git"
	"vibethis/graph/pkg/graph"
	"vibethis/storage/pkg/storage"
	"vibethis/vibethis/internal/config"
)

// InitializeDependencies initializes all application dependencies
func InitializeDependencies(ctx context.Context, cfg config.Config) (web.Dependencies, func(), error) {
	var cleanup []func()

	// Initialize storage with default database path
	databasePath := cfg.RootPath + "/.vibethis"
	
	// Ensure database directory exists if CreateNew flag is set
	if cfg.CreateNew {
		if err := os.MkdirAll(databasePath, 0755); err != nil {
			return web.Dependencies{}, nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}
	
	storageImpl, err := storage.NewBoltStorage(storage.Config{
		DatabasePath: databasePath,
		ReadOnly:     false,
	})
	if err != nil {
		return web.Dependencies{}, nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	cleanup = append(cleanup, func() { storageImpl.Close() })

	// Initialize graph builder
	graphBuilder := graph.NewBuilder(cfg.RootPath)

	// Initialize file browser
	fileBrowser := files.NewBrowser(files.Config{
		RootPath: cfg.RootPath,
	})

	// Initialize git repository
	gitRepo := git.NewRepository(git.Config{
		DefaultAuthor: "vibethis",
		DefaultEmail:  "vibethis@example.com",
	})

	// Initialize container manager
	containerMgr := container.NewManager(container.Config{})

	deps := web.Dependencies{
		Storage:   storageImpl,
		Graph:     graphBuilder,
		Files:     fileBrowser,
		Git:       gitRepo,
		Container: containerMgr,
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
