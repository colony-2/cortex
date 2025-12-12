package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/gorilla/mux"
)

// ExtensionRoute represents a route added by an external module
type ExtensionRoute struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// Handlers contains all HTTP handlers
type Handlers struct {
	storage core.Storage
	graph   core.GraphBuilder
}

// New creates a new handlers instance
func New(storage core.Storage, graph core.GraphBuilder) *Handlers {
	return &Handlers{
		storage: storage,
		graph:   graph,
	}
}

// SetupRoutes configures all HTTP routes
func (h *Handlers) SetupRoutes(staticHandler http.Handler) *mux.Router {
	return h.SetupRoutesWithExtensions(staticHandler, nil)
}

// SetupRoutesWithExtensions configures all HTTP routes including extensions
func (h *Handlers) SetupRoutesWithExtensions(staticHandler http.Handler, extensions []ExtensionRoute) *mux.Router {
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()

	// Add extension routes if provided
	if extensions != nil {
		for _, ext := range extensions {
			api.HandleFunc(ext.Path, withHandlerLog("ext:"+ext.Path, ext.Handler)).Methods(ext.Method)
		}
	}

	// Static files and SPA routes (everything not under /api)
	if staticHandler != nil {
		// Use a custom handler that excludes /api paths
		r.PathPrefix("/").Handler(&nonAPIHandler{staticHandler: staticHandler})
	}

	return r
}

// withHandlerLog wraps a handler to log entry/exit with status for identifying which handler responded.
func withHandlerLog(tag string, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lrw := &logResponseWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		log.Printf("HANDLER_LOG: enter tag=%s method=%s path=%s", tag, r.Method, r.URL.Path)
		fn(lrw, r)
		dur := time.Since(start)
		log.Printf("HANDLER_LOG: exit tag=%s status=%d dur=%s", tag, lrw.status, dur)
	}
}

type logResponseWriter struct {
	http.ResponseWriter
	status int
}

func (l *logResponseWriter) WriteHeader(code int) {
	l.status = code
	l.ResponseWriter.WriteHeader(code)
}

// NewSPAHandler creates a handler for serving the single-page application
func NewSPAHandler(staticPath string) http.Handler {
	return &spaHandler{
		staticPath: staticPath,
		indexPath:  filepath.Join(staticPath, "index.html"),
	}
}

type spaHandler struct {
	staticPath string
	indexPath  string
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get the absolute path to prevent directory traversal
	path := filepath.Join(h.staticPath, r.URL.Path)

	// Check if the path exists
	absPath, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Security check - ensure the path is within staticPath
	absStaticPath, _ := filepath.Abs(h.staticPath)
	if !strings.HasPrefix(absPath, absStaticPath) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Check if file exists
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		// File doesn't exist or is a directory, serve index.html
		http.ServeFile(w, r, h.indexPath)
		return
	}

	// Serve the actual file
	http.ServeFile(w, r, absPath)
}

// nonAPIHandler wraps a static handler to exclude /api paths
type nonAPIHandler struct {
	staticHandler http.Handler
}

func (h *nonAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Skip static handling for /api paths
	if strings.HasPrefix(r.URL.Path, "/api") {
		http.NotFound(w, r)
		return
	}

	// Serve static files for all other paths
	h.staticHandler.ServeHTTP(w, r)
}
