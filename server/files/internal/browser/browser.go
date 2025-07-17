package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/divisive-ai/vibethis/server/files/internal/security"
)

// FileInfo represents information about a file or directory
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Type  string `json:"type"`
}

// Browser implements the file browser functionality
type Browser struct {
	validator         security.Validator
	allowedExtensions []string
	maxFileSize       int64
}

// New creates a new file browser
func New(validator security.Validator, allowedExtensions []string, maxFileSize int64) *Browser {
	return &Browser{
		validator:         validator,
		allowedExtensions: allowedExtensions,
		maxFileSize:       maxFileSize,
	}
}

// ListFiles returns a list of files in the specified node directory
func (b *Browser) ListFiles(ctx context.Context, nodePath string) ([]FileInfo, error) {
	// nodePath is already an absolute path
	fullPath := nodePath

	// Validate path
	if b.validator != nil {
		if err := b.validator.ValidatePath(fullPath); err != nil {
			return nil, err
		}
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var fileInfos []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileInfo := FileInfo{
			Name:  entry.Name(),
			Path:  entry.Name(), // Path relative to the node directory
			IsDir: entry.IsDir(),
			Size:  info.Size(),
			Type:  getFileType(entry.Name(), entry.IsDir()),
		}

		// Filter by extensions if specified
		if len(b.allowedExtensions) > 0 && !entry.IsDir() {
			allowed := false
			for _, ext := range b.allowedExtensions {
				if strings.HasSuffix(entry.Name(), ext) {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		fileInfos = append(fileInfos, fileInfo)
	}

	return fileInfos, nil
}

// ReadFile reads the contents of a file within a node directory
func (b *Browser) ReadFile(ctx context.Context, nodePath, filePath string) ([]byte, error) {
	// nodePath is already an absolute path
	fullPath := filepath.Join(nodePath, filePath)

	// Validate path
	if b.validator != nil {
		if err := b.validator.ValidatePath(fullPath); err != nil {
			return nil, err
		}
	}

	// Check file size if limit is set
	if b.maxFileSize > 0 {
		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat file: %w", err)
		}
		if info.Size() > b.maxFileSize {
			return nil, fmt.Errorf("file size %d exceeds maximum allowed size %d", info.Size(), b.maxFileSize)
		}
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return content, nil
}

// WriteFile writes content to a file within a node directory
func (b *Browser) WriteFile(ctx context.Context, nodePath, filePath string, content []byte) error {
	// nodePath is already an absolute path
	fullPath := filepath.Join(nodePath, filePath)

	// Validate path
	if b.validator != nil {
		if err := b.validator.ValidatePath(fullPath); err != nil {
			return err
		}
	}

	// Check content size if limit is set
	if b.maxFileSize > 0 && int64(len(content)) > b.maxFileSize {
		return fmt.Errorf("content size %d exceeds maximum allowed size %d", len(content), b.maxFileSize)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// CreateDirectory creates a new directory within a node directory
func (b *Browser) CreateDirectory(ctx context.Context, nodePath, dirPath string) error {
	// nodePath is already an absolute path
	fullPath := filepath.Join(nodePath, dirPath)

	// Validate path
	if b.validator != nil {
		if err := b.validator.ValidatePath(fullPath); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

// Delete removes a file or directory within a node directory
func (b *Browser) Delete(ctx context.Context, nodePath, path string) error {
	// nodePath is already an absolute path
	fullPath := filepath.Join(nodePath, path)

	// Validate path
	if b.validator != nil {
		if err := b.validator.ValidatePath(fullPath); err != nil {
			return err
		}
	}

	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}

	return nil
}

// getFileType determines the file type based on the file name
func getFileType(name string, isDir bool) string {
	if isDir {
		return "directory"
	}

	ext := strings.ToLower(filepath.Ext(name))
	base := strings.ToLower(filepath.Base(name))

	// Check special files first
	switch base {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	case ".gitignore":
		return "git"
	}

	// Check by extension
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h", ".hpp":
		return "header"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".r":
		return "r"
	case ".m":
		return "matlab"
	case ".sh", ".bash":
		return "shell"
	case ".bat", ".cmd":
		return "batch"
	case ".ps1":
		return "powershell"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".scss", ".sass":
		return "sass"
	case ".less":
		return "less"
	case ".xml":
		return "xml"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".ini", ".cfg", ".conf":
		return "config"
	case ".md", ".markdown":
		return "markdown"
	case ".rst":
		return "restructuredtext"
	case ".tex":
		return "latex"
	case ".sql":
		return "sql"
	case ".graphql", ".gql":
		return "graphql"
	case ".proto":
		return "protobuf"
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".ico":
		return "image"
	case ".mp4", ".avi", ".mov", ".mkv", ".webm":
		return "video"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg":
		return "audio"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx":
		return "word"
	case ".xls", ".xlsx":
		return "excel"
	case ".ppt", ".pptx":
		return "powerpoint"
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar":
		return "archive"
	case ".exe", ".dll", ".so", ".dylib":
		return "binary"
	case ".txt":
		return "text"
	default:
		return "file"
	}
}
