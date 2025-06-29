package setup

import (
	"context"
	"fmt"

	"vibethis/container/pkg/container"
	"vibethis/files/pkg/files"
	"vibethis/git/pkg/git"
	"vibethis/graph/pkg/graph"
	"vibethis/storage/pkg/storage"
	"vibethis/vibethis/internal/config"
	"vibethis/web/pkg/web"
)

// InitializeDependencies initializes all application dependencies
func InitializeDependencies(ctx context.Context, cfg config.Config) (web.Dependencies, func(), error) {
	var cleanup []func()

	// Initialize storage
	storageImpl, err := storage.NewBoltStorage(storage.Config{
		DatabasePath: cfg.DatabasePath,
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
	webConfig := web.Config{
		Port:        cfg.Port,
		CORSOrigins: cfg.CORSOrigins,
		StaticPath:  cfg.StaticPath,
	}

	server := web.NewServer(webConfig, deps)
	return server, nil
}
