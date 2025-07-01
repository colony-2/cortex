package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"vibethis/container/pkg/container"
	"vibethis/core/pkg/core"
	"vibethis/files/pkg/files"
	"vibethis/git/pkg/git"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	storage   core.Storage
	graph     core.GraphBuilder
	files     files.Browser
	git       git.Repository
	container container.Manager
}

// New creates a new handlers instance
func New(storage core.Storage, graph core.GraphBuilder, files files.Browser, git git.Repository, container container.Manager) *Handlers {
	return &Handlers{
		storage:   storage,
		graph:     graph,
		files:     files,
		git:       git,
		container: container,
	}
}

// SetupRoutes configures all HTTP routes
func (h *Handlers) SetupRoutes(staticHandler http.Handler) *mux.Router {
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	
	// Graph endpoints
	api.HandleFunc("/graph", h.GetGraph).Methods("GET")
	
	// Position endpoints
	api.HandleFunc("/positions", h.GetPositions).Methods("GET")
	api.HandleFunc("/positions", h.SavePositions).Methods("POST")
	
	// Node endpoints
	api.HandleFunc("/nodes/{nodeId}/files", h.GetFiles).Methods("GET")
	api.HandleFunc("/nodes/{nodeId}/files/{filePath:.*}", h.GetFile).Methods("GET")
	api.HandleFunc("/nodes/{nodeId}/files/{filePath:.*}", h.PutFile).Methods("PUT")
	
	// Git endpoints
	api.HandleFunc("/nodes/{nodeId}/git/status", h.GetGitStatus).Methods("GET")
	api.HandleFunc("/nodes/{nodeId}/git/diff", h.GetGitDiff).Methods("GET")
	api.HandleFunc("/nodes/{nodeId}/git/history", h.GetGitHistory).Methods("GET")
	api.HandleFunc("/nodes/{nodeId}/git/commit", h.CreateGitCommit).Methods("POST")
	
	// Container endpoints
	api.HandleFunc("/nodes/{nodeId}/container/status", h.GetContainerStatus).Methods("GET")
	api.HandleFunc("/nodes/{nodeId}/container/create", h.CreateContainer).Methods("POST")
	api.HandleFunc("/nodes/{nodeId}/container/start", h.StartContainer).Methods("POST")
	api.HandleFunc("/nodes/{nodeId}/container/stop", h.StopContainer).Methods("POST")
	api.HandleFunc("/nodes/{nodeId}/container/restart", h.RestartContainer).Methods("POST")
	api.HandleFunc("/nodes/{nodeId}/container/reset", h.ResetContainer).Methods("POST")
	
	// Static files and SPA routes (everything not under /api)
	if staticHandler != nil {
		r.PathPrefix("/").Handler(staticHandler)
	}
	
	return r
}

// GetPositions handles GET /api/positions
func (h *Handlers) GetPositions(w http.ResponseWriter, r *http.Request) {
	positions, err := h.storage.GetPositions(r.Context())
	if err != nil {
		http.Error(w, "Failed to get positions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(positions)
}

// SavePositions handles POST /api/positions
func (h *Handlers) SavePositions(w http.ResponseWriter, r *http.Request) {
	var positions []core.Position
	
	if r.Body == nil {
		http.Error(w, "Request body is required", http.StatusBadRequest)
		return
	}
	
	if err := json.NewDecoder(r.Body).Decode(&positions); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	for _, pos := range positions {
		if err := h.storage.SavePosition(r.Context(), pos); err != nil {
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