package cortex

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func spaHandler(dist fs.FS) http.Handler {
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(dist, "index.html"); err != nil {
			http.Error(w, "Cortex UI has not been built into this executable", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(dist, name); err != nil {
			r = r.Clone(r.Context())
			// FileServer redirects /index.html to ./, which loops on SPA deep links.
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
