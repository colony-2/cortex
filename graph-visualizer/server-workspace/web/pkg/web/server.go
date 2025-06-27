// Package web provides the HTTP server and API for vibethis.
package web

import (
	"context"
	"fmt"
	"net/http"
	"vibethis/core/pkg/core"
	"vibethis/container/pkg/container"
	"vibethis/files/pkg/files"
	"vibethis/git/pkg/git"
	"vibethis/web/internal/handlers"
	"vibethis/web/internal/middleware"
)

// Config defines configuration for the web server.
type Config struct {
	// Port is the port to listen on.
	Port int
	
	// CORSOrigins is a list of allowed CORS origins.
	// If empty, CORS is disabled.
	CORSOrigins []string
	
	// StaticPath is the path to static assets.
	// If empty, no static assets are served.
	StaticPath string
	
	// EnableWebSocket enables WebSocket support for terminal connections.
	EnableWebSocket bool
	
	// MaxUploadSize is the maximum file upload size in bytes.
	MaxUploadSize int64
}

// Dependencies contains all the dependencies required by the web server.
type Dependencies struct {
	Storage   core.Storage
	Graph     core.GraphBuilder
	Files     files.Browser
	Git       git.Repository
	Container container.Manager
}

// Server represents the HTTP server.
type Server struct {
	config   Config
	deps     Dependencies
	server   *http.Server
	handlers *handlers.Handlers
}

// NewServer creates a new HTTP server with the given configuration and dependencies.
func NewServer(config Config, deps Dependencies) *Server {
	h := handlers.New(deps.Storage, deps.Graph, deps.Files, deps.Git, deps.Container)
	
	router := h.SetupRoutes()
	
	// Apply middleware
	var handler http.Handler = router
	
	if len(config.CORSOrigins) > 0 {
		handler = middleware.CORS(config.CORSOrigins, handler)
	}
	
	handler = middleware.Logging(handler)
	handler = middleware.Recovery(handler)
	
	return &Server{
		config:   config,
		deps:     deps,
		handlers: h,
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", config.Port),
			Handler: handler,
		},
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Stop gracefully stops the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// ServeHTTP implements http.Handler for testing purposes.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.server.Handler.ServeHTTP(w, r)
}