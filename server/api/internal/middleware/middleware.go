package middleware

import (
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/core/pkg/logutil"
	"github.com/gorilla/mux"
)

// CORS adds CORS headers to responses
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Logging logs HTTP requests
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Skip logging for static assets
		if !strings.HasPrefix(r.URL.Path, "/api") {
			next.ServeHTTP(w, r)
			return
		}

		// Create a response writer to capture the status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		log.Printf("%s %s - %d (%v)", r.Method, r.URL.Path, rw.statusCode, duration)
	})
}

// RouteLogging logs the matched route template and method for debugging
func RouteLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			route := mux.CurrentRoute(r)
			if route != nil {
				if tpl, err := route.GetPathTemplate(); err == nil {
					log.Printf("ROUTE_LOG: method=%s path=%s template=%s", r.Method, r.URL.Path, tpl)
				} else {
					log.Printf("ROUTE_LOG: method=%s path=%s template_err=%v", r.Method, r.URL.Path, err)
				}
			} else {
				// Route not yet available (middleware wrapping the router); avoid panic
				log.Printf("ROUTE_LOG: method=%s path=%s template=unmatched", r.Method, r.URL.Path)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Recovery recovers from panics
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Default().Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", err,
					"stacktrace", logutil.Stacktrace(6),
				)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher to support SSE streaming
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
