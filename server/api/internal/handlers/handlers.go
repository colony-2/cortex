package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/core/pkg/core"
	"github.com/colony-2/colony2/server/core/pkg/logutil"
	"github.com/colony-2/colony2/server/project/pkg/project"
	recipesvc "github.com/colony-2/colony2/server/recipes/pkg/recipe"
	"github.com/colony-2/colony2/server/registry/pkg/registry"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/colony-2/colony2/server/workflow/pkg/workflow"
	"github.com/gorilla/mux"
)

// ExtensionRoute represents a route added by an external module
type ExtensionRoute struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// GraphFactory constructs a GraphBuilder for a specific project context.
type GraphFactory func(ctx context.Context, projectID string) (core.GraphBuilder, error)

// RecipeRegistryFactory constructs a recipe registry rooted in the project's repository.
// It returns the registry instance, the recipes root directory, and an optional cleanup function.
type RecipeRegistryFactory func(ctx context.Context, projectID project.ID, repoPath string) (*registry.Registry, string, func(), error)

// Handlers contains all HTTP handlers
type Handlers struct {
	graphFactory GraphFactory
	recipes      RecipeRegistryFactory

	projects  project.Service
	cells     cell.Service
	tickets   ticket.Service
	workflows workflow.Service
	recipeSvc recipesvc.Service

	cellDeps cellDependencyLister
}

// New creates a new handlers instance
func New(factory GraphFactory, recipes RecipeRegistryFactory, projects project.Service, cells cell.Service, tickets ticket.Service, workflows workflow.Service, recipeSvc recipesvc.Service, cellDeps cellDependencyLister) *Handlers {
	if recipes == nil {
		recipes = defaultRecipeRegistryFactory
	}
	return &Handlers{
		graphFactory: factory,
		recipes:      recipes,
		projects:     projects,
		cells:        cells,
		tickets:      tickets,
		workflows:    workflows,
		recipeSvc:    recipeSvc,
		cellDeps:     cellDeps,
	}
}

func defaultRecipeRegistryFactory(_ context.Context, projectID project.ID, repoPath string) (*registry.Registry, string, func(), error) {
	recipesDir := filepath.Join(repoPath, ".colony2", "recipes")
	reg, err := registry.NewRegistry(nil, recipesDir)
	if err != nil {
		return nil, recipesDir, nil, err
	}
	if err := reg.Start(); err != nil {
		return nil, recipesDir, nil, err
	}
	cleanup := func() {
		if err := reg.Stop(); err != nil {
			slog.Default().Warn("failed to stop recipe registry",
				"project_id", projectID,
				"error", err,
				"error_chain", logutil.ErrorChain(err),
				"stacktrace", logutil.Stacktrace(4),
			)
		}
	}
	return reg, recipesDir, cleanup, nil
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

	// OpenAPI endpoints
	registerAPIRoutes(api, h)

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
		reqBodyPrefix := captureRequestBodyPrefix(r, 4096)

		lrw := &logResponseWriter{ResponseWriter: w, status: http.StatusOK, bodyLimit: 4096}
		start := time.Now()
		fn(lrw, r)
		dur := time.Since(start)
		if lrw.status < 400 {
			return
		}

		template := "unmatched"
		if route := mux.CurrentRoute(r); route != nil {
			if tpl, err := route.GetPathTemplate(); err == nil {
				template = tpl
			}
		}

		level := slog.LevelWarn
		attrs := []any{
			"tag", tag,
			"method", r.Method,
			"path", r.URL.Path,
			"template", template,
			"content_type", r.Header.Get("Content-Type"),
			"content_length", r.ContentLength,
			"request_body_prefix", reqBodyPrefix,
			"status", lrw.status,
			"duration_ms", dur.Milliseconds(),
			"response_bytes", lrw.bytesWritten,
			"error", lrw.err,
			"error_chain", lrw.errChain,
			"response_body_prefix", lrw.bodyPrefix(),
		}
		if lrw.status >= 500 {
			level = slog.LevelError
			stack := lrw.errStacktrace
			if len(stack) == 0 {
				stack = logutil.Stacktrace(5)
			}
			attrs = append(attrs, "stacktrace", stack)
		}

		slog.Default().Log(r.Context(), level, "http handler returned error", attrs...)
	}
}

type logResponseWriter struct {
	http.ResponseWriter
	status int

	bodyLimit    int64
	bodyCaptured bytes.Buffer
	bytesWritten int64

	err           error
	errChain      []string
	errStacktrace []string
}

func (l *logResponseWriter) WriteHeader(code int) {
	l.status = code
	l.ResponseWriter.WriteHeader(code)
}

func (l *logResponseWriter) captureError(err error, chain []string, stacktrace []string) {
	if l.err != nil {
		return
	}
	l.err = err
	l.errChain = chain
	l.errStacktrace = stacktrace
}

func (l *logResponseWriter) Write(p []byte) (int, error) {
	n, err := l.ResponseWriter.Write(p)
	l.bytesWritten += int64(n)

	if l.bodyLimit > 0 && l.bodyCaptured.Len() < int(l.bodyLimit) && n > 0 {
		remaining := int(l.bodyLimit) - l.bodyCaptured.Len()
		if remaining > 0 {
			toWrite := n
			if toWrite > remaining {
				toWrite = remaining
			}
			_, _ = l.bodyCaptured.Write(p[:toWrite])
		}
	}

	return n, err
}

func (l *logResponseWriter) bodyPrefix() string {
	if l.bodyCaptured.Len() == 0 {
		return ""
	}
	// Best-effort trim of leading/trailing whitespace and ensure valid UTF-8-ish output.
	b := bytes.TrimSpace(l.bodyCaptured.Bytes())
	return string(b)
}

// Flush implements http.Flusher to support SSE streaming
func (l *logResponseWriter) Flush() {
	if f, ok := l.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	} else {
		slog.Default().Debug("response writer does not support Flusher", "type", fmt.Sprintf("%T", l.ResponseWriter))
	}
}

func captureRequestBodyPrefix(r *http.Request, limit int64) string {
	if r == nil || r.Body == nil || limit <= 0 {
		return ""
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return ""
	}
	if r.ContentLength == 0 {
		return ""
	}
	ctype := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.HasPrefix(ctype, "multipart/form-data") {
		return ""
	}
	if strings.HasPrefix(ctype, "application/octet-stream") {
		return ""
	}

	prefix, _ := io.ReadAll(io.LimitReader(r.Body, limit))
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(prefix), r.Body))
	return string(bytes.TrimSpace(prefix))
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
