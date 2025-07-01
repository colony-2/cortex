//go:build !prod
// +build !prod

package static

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

// GetFileSystem returns a filesystem for development builds
func GetFileSystem() (http.FileSystem, error) {
	// In development, serve from the internal/static/assets directory relative to the binary
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	staticPath := filepath.Join(dir, "internal/static/assets")
	return http.Dir(staticPath), nil
}

// GetFS returns a filesystem for development builds
func GetFS() (fs.FS, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	staticPath := filepath.Join(dir, "internal/static/assets")
	return os.DirFS(staticPath), nil
}