package setup

import (
	"context"

	"github.com/colony-2/colony2/server/api/pkg/web"
	"github.com/colony-2/colony2/server/cortex/internal/config"
)

// InitializeDependencies initializes all application dependencies
func InitializeDependencies(ctx context.Context, cfg config.Config) (web.Dependencies, func(), error) {
	// TODO: port over test server stuff.
	return web.Dependencies{}, nil, nil
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
