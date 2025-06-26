//go:build prod
// +build prod

package main

import (
	"io/fs"
	"log"
	"net/http"
	"vibethis/static"
)

func getFrontendHandler() http.Handler {
	fsys, err := static.GetFileSystem()
	if err != nil {
		log.Fatal("Failed to get filesystem:", err)
	}
	return &embeddedSPAHandler{fs: fsys}
}

// embeddedSPAHandler handles SPA routing for embedded filesystem
type embeddedSPAHandler struct {
	fs http.FileSystem
}

func (h *embeddedSPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to open the requested file
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	
	file, err := h.fs.Open(path)
	if err != nil {
		// If file not found, serve index.html for client-side routing
		if _, ok := err.(*fs.PathError); ok {
			indexFile, err := h.fs.Open("/index.html")
			if err != nil {
				http.Error(w, "index.html not found", http.StatusNotFound)
				return
			}
			defer indexFile.Close()
			
			// Get file info for content length
			stat, err := indexFile.Stat()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			
			http.ServeContent(w, r, "index.html", stat.ModTime(), indexFile)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	file.Close()
	
	// File exists, serve it normally
	http.FileServer(h.fs).ServeHTTP(w, r)
}