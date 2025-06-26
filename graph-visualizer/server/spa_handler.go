package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaHandler implements the http.Handler interface for serving a Single Page Application
type spaHandler struct {
	staticPath string
	indexPath  string
}

// ServeHTTP handles the request by serving static files or falling back to index.html
func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get the absolute path to prevent directory traversal
	path := r.URL.Path
	
	// Construct the file path
	fullPath := filepath.Join(h.staticPath, path)
	
	// Check if path exists
	_, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		// File doesn't exist, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		// If we got an error (other than file not found) report it
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Check if it's a directory, if so serve index.html
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	if fileInfo.IsDir() {
		// Try to serve index.html from the directory
		indexPath := filepath.Join(fullPath, "index.html")
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			// No index.html in directory, serve the main index.html
			http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
			return
		}
	}
	
	// Check if the request is for a file without extension (likely a route)
	// Exclude files with common static extensions
	ext := filepath.Ext(path)
	if ext == "" && !strings.Contains(path, ".") {
		// No extension, probably a route, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	}
	
	// Otherwise, serve the file
	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

// newSPAHandler creates a new SPA handler
func newSPAHandler(staticPath string) *spaHandler {
	return &spaHandler{
		staticPath: staticPath,
		indexPath:  "index.html",
	}
}