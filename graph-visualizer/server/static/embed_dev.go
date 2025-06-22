//go:build !prod
// +build !prod

package static

import (
	"net/http"
	"os"
)

// GetFileSystem returns the development filesystem
func GetFileSystem() (http.FileSystem, error) {
	// In development, serve from the actual dist directory
	return http.Dir("../web/dist/"), nil
}