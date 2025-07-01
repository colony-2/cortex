package handlers

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// NewEmbeddedSPAHandler creates a handler for serving the single-page application from an embedded filesystem
func NewEmbeddedSPAHandler(fsys fs.FS) http.Handler {
	return &embeddedSPAHandler{
		fs: fsys,
	}
}

type embeddedSPAHandler struct {
	fs fs.FS
}

func (h *embeddedSPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clean the path
	urlPath := path.Clean(r.URL.Path)
	if urlPath == "/" {
		urlPath = "/index.html"
	}
	
	// Remove leading slash for fs.FS
	if strings.HasPrefix(urlPath, "/") {
		urlPath = urlPath[1:]
	}
	
	// Try to open the file
	file, err := h.fs.Open(urlPath)
	if err != nil {
		// If file not found, serve index.html for client-side routing
		indexFile, err := h.fs.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		defer indexFile.Close()
		
		// Set content type based on file extension
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		
		// Copy file contents
		io.Copy(w, indexFile)
		return
	}
	defer file.Close()
	
	// Get file info
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// If it's a directory, serve index.html
	if stat.IsDir() {
		indexFile, err := h.fs.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		defer indexFile.Close()
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.Copy(w, indexFile)
		return
	}
	
	// Serve the file with appropriate content type
	contentType := getContentType(urlPath)
	w.Header().Set("Content-Type", contentType)
	
	io.Copy(w, file)
}

func getContentType(filePath string) string {
	switch path.Ext(filePath) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".eot":
		return "application/vnd.ms-fontobject"
	default:
		return "application/octet-stream"
	}
}