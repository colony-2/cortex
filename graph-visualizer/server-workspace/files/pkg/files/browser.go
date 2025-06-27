// Package files provides file system operations for vibethis.
package files

import (
	"context"
	"vibethis/files/internal/browser"
	"vibethis/files/internal/security"
)

// FileInfo represents information about a file or directory.
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Type  string `json:"type"`
}

// Browser provides file system browsing and manipulation operations.
type Browser interface {
	// ListFiles returns a list of files in the specified node directory.
	ListFiles(ctx context.Context, nodePath string) ([]FileInfo, error)
	
	// ReadFile reads the contents of a file within a node directory.
	ReadFile(ctx context.Context, nodePath, filePath string) ([]byte, error)
	
	// WriteFile writes content to a file within a node directory.
	WriteFile(ctx context.Context, nodePath, filePath string, content []byte) error
	
	// CreateDirectory creates a new directory within a node directory.
	CreateDirectory(ctx context.Context, nodePath, dirPath string) error
	
	// Delete removes a file or directory within a node directory.
	Delete(ctx context.Context, nodePath, path string) error
}

// Config defines configuration for the file browser.
type Config struct {
	// RootPath is the root directory containing all nodes.
	RootPath string
	
	// AllowedExtensions restricts browsable files to these extensions.
	// If empty, all files are allowed.
	AllowedExtensions []string
	
	// MaxFileSize is the maximum file size that can be read.
	// If 0, no limit is enforced.
	MaxFileSize int64
}

// NewBrowser creates a new file browser with the given configuration.
func NewBrowser(config Config) Browser {
	validator := security.NewValidator(config.RootPath)
	b := browser.New(config.RootPath, validator, config.AllowedExtensions, config.MaxFileSize)
	return &browserAdapter{b: b}
}

// browserAdapter adapts the internal browser to the public interface
type browserAdapter struct {
	b *browser.Browser
}

func (a *browserAdapter) ListFiles(ctx context.Context, nodePath string) ([]FileInfo, error) {
	files, err := a.b.ListFiles(ctx, nodePath)
	if err != nil {
		return nil, err
	}
	
	// Convert internal FileInfo to public FileInfo
	result := make([]FileInfo, len(files))
	for i, f := range files {
		result[i] = FileInfo{
			Name:  f.Name,
			Path:  f.Path,
			IsDir: f.IsDir,
			Size:  f.Size,
			Type:  f.Type,
		}
	}
	return result, nil
}

func (a *browserAdapter) ReadFile(ctx context.Context, nodePath, filePath string) ([]byte, error) {
	return a.b.ReadFile(ctx, nodePath, filePath)
}

func (a *browserAdapter) WriteFile(ctx context.Context, nodePath, filePath string, content []byte) error {
	return a.b.WriteFile(ctx, nodePath, filePath, content)
}

func (a *browserAdapter) CreateDirectory(ctx context.Context, nodePath, dirPath string) error {
	return a.b.CreateDirectory(ctx, nodePath, dirPath)
}

func (a *browserAdapter) Delete(ctx context.Context, nodePath, path string) error {
	return a.b.Delete(ctx, nodePath, path)
}