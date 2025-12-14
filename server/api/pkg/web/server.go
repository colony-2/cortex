// Package web provides the HTTP server and API for vibethis.
package web

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/divisive-ai/vibethis/server/api/internal/handlers"
	"github.com/divisive-ai/vibethis/server/api/internal/middleware"
	"github.com/divisive-ai/vibethis/server/cell/pkg/cell"
	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/project/pkg/project"
	"github.com/divisive-ai/vibethis/server/ticket/pkg/ticket"
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
	Storage  core.Storage
	Graph    core.GraphBuilder
	StaticFS http.FileSystem // Optional: filesystem for static files

	// ExtensionRoutes allows external modules to add routes
	ExtensionRoutes []ExtensionRoute

	// Optional domain services for OpenAPI handlers
	Projects project.Service
	Cells    cell.Service
	Tickets  ticket.Service

	// GraphFactory builds a graph builder per project (overrides Graph when set)
	GraphFactory handlers.GraphFactory

	// RecipeRegistryFactory builds a registry per project rooted at its git repo .vibethis/recipes directory.
	RecipeRegistryFactory handlers.RecipeRegistryFactory

	// Optional dependency lister for cells (used to render dependencies)
	CellDeps interface {
		ListDependencies(ctx context.Context, projectID project.ID, from cell.ID) ([]cell.ID, error)
	}
}

// ExtensionRoute represents a route added by an external module
type ExtensionRoute struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
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
	h := handlers.New(deps.Storage, deps.Graph, deps.GraphFactory, deps.RecipeRegistryFactory, deps.Projects, deps.Cells, deps.Tickets, deps.CellDeps)

	// Setup static handler if filesystem is provided
	var staticHandler http.Handler
	if deps.StaticFS != nil {
		// Convert http.FileSystem to fs.FS for the embedded handler
		// The embedded SPA handler needs fs.FS for proper SPA routing
		fsys := &httpFSAdapter{deps.StaticFS}
		staticHandler = handlers.NewEmbeddedSPAHandler(fsys)
	} else if config.StaticPath != "" && config.StaticPath != "embedded" {
		// Fallback to directory-based static handler for development
		staticHandler = handlers.NewSPAHandler(config.StaticPath)
	}

	// Convert ExtensionRoute to handlers.ExtensionRoute
	var handlerExtensions []handlers.ExtensionRoute
	if deps.ExtensionRoutes != nil {
		handlerExtensions = make([]handlers.ExtensionRoute, len(deps.ExtensionRoutes))
		for i, ext := range deps.ExtensionRoutes {
			handlerExtensions[i] = handlers.ExtensionRoute{
				Method:  ext.Method,
				Path:    ext.Path,
				Handler: ext.Handler,
			}
		}
	}

	router := h.SetupRoutesWithExtensions(staticHandler, handlerExtensions)

	// Apply middleware
	var handler http.Handler = router

	if len(config.CORSOrigins) > 0 {
		handler = middleware.CORS(config.CORSOrigins, handler)
	}

	handler = middleware.RouteLogging(handler)
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

// httpFSAdapter adapts http.FileSystem to fs.FS
type httpFSAdapter struct {
	http.FileSystem
}

func (a *httpFSAdapter) Open(name string) (fs.File, error) {
	// Add leading slash for http.FileSystem
	if name != "" && name[0] != '/' {
		name = "/" + name
	}
	return a.FileSystem.Open(name)
}
