//go:build prod
// +build prod

package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var content embed.FS

// GetFileSystem returns the embedded filesystem
func GetFileSystem() (http.FileSystem, error) {
	fsys, err := fs.Sub(content, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(fsys), nil
}