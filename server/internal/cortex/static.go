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
			r.URL.Path = "/index.html"
		}
		files.ServeHTTP(w, r)
	})
}
