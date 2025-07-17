//go:build prod
// +build prod

package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var embeddedFiles embed.FS

// GetFileSystem returns the embedded filesystem for production builds
func GetFileSystem() (http.FileSystem, error) {
	// Create a sub filesystem to serve files from the assets directory
	sub, err := fs.Sub(embeddedFiles, "assets")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}

// GetFS returns the embedded fs.FS for production builds
func GetFS() (fs.FS, error) {
	return fs.Sub(embeddedFiles, "assets")
}