package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/core/pkg/core"
	"github.com/divisive-ai/vibethis/server/files/pkg/files"
	"github.com/divisive-ai/vibethis/server/git/pkg/git"
	"github.com/divisive-ai/vibethis/server/openapi/pkg/openapi"
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
	files   files.Browser
	git     git.Repository
}

// New creates a new handlers instance
func New(storage core.Storage, graph core.GraphBuilder, files files.Browser, git git.Repository) *Handlers {
	return &Handlers{
		storage: storage,
		graph:   graph,
		files:   files,
		git:     git,
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

	// Graph endpoints
	api.HandleFunc("/graph", withHandlerLog("core:GetGraph", h.GetGraph)).Methods("GET")

	// Position endpoints
	api.HandleFunc("/positions", withHandlerLog("core:GetPositions", h.GetPositions)).Methods("GET")
	api.HandleFunc("/positions", withHandlerLog("core:SavePositions", h.SavePositions)).Methods("POST")

	// Cell endpoints
	api.HandleFunc("/cells/{cellId}/files", withHandlerLog("core:GetFiles", h.GetFiles)).Methods("GET")
	api.HandleFunc("/cells/{cellId}/files/{filePath:.*}", withHandlerLog("core:GetFile", h.GetFile)).Methods("GET")
	api.HandleFunc("/cells/{cellId}/files/{filePath:.*}", withHandlerLog("core:PutFile", h.PutFile)).Methods("PUT")

	// Git endpoints
	api.HandleFunc("/cells/{cellId}/git/status", withHandlerLog("core:GetGitStatus", h.GetGitStatus)).Methods("GET")
	api.HandleFunc("/cells/{cellId}/git/diff", withHandlerLog("core:GetGitDiff", h.GetGitDiff)).Methods("GET")
	api.HandleFunc("/cells/{cellId}/git/history", withHandlerLog("core:GetGitHistory", h.GetGitHistory)).Methods("GET")
	api.HandleFunc("/cells/{cellId}/git/commit", withHandlerLog("core:CreateGitCommit", h.CreateGitCommit)).Methods("POST")

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

// GetPositions handles GET /api/positions
func (h *Handlers) GetPositions(w http.ResponseWriter, r *http.Request) {
	positions, err := h.storage.GetPositions(r.Context())
	if err != nil {
		http.Error(w, "Failed to get positions", http.StatusInternalServerError)
		return
	}

	// Convert core.Position to openapi.Position
	apiPositions := make([]openapi.Position, len(positions))
	for i, pos := range positions {
		apiPositions[i] = openapi.Position{
			CellId: pos.CellID,
			X:      pos.X,
			Y:      pos.Y,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiPositions)
}

// SavePositions handles POST /api/positions
func (h *Handlers) SavePositions(w http.ResponseWriter, r *http.Request) {
	var apiPositions []openapi.Position

	if r.Body == nil {
		http.Error(w, "Request body is required", http.StatusBadRequest)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&apiPositions); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Convert openapi.Position to core.Position
	for _, apiPos := range apiPositions {
		corePos := core.Position{
			CellID: apiPos.CellId,
			X:      apiPos.X,
			Y:      apiPos.Y,
		}
		if err := h.storage.SavePosition(r.Context(), corePos); err != nil {
			http.Error(w, "Failed to save positions", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
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
