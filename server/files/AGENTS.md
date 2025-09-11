# Files Module

## Overview
The files module provides secure file system operations for vibethis node directories. It enables controlled file browsing, reading, writing, and manipulation while enforcing security boundaries to prevent unauthorized access outside designated node paths.

## Architecture
The module follows a layered architecture with clear separation of concerns:

- **Public API Layer** (`pkg/files/`): Exposes the main `Browser` interface and configuration types
- **Internal Browser** (`internal/browser/`): Core file operation implementation with file type detection
- **Security Layer** (`internal/security/`): Path validation and security enforcement
- **Adapter Pattern**: The public interface wraps internal implementation for type consistency

Key relationships:
- `Browser` interface defines all file operations (ListFiles, ReadFile, WriteFile, CreateDirectory, Delete)
- `browserAdapter` implements the interface by delegating to internal browser
- Security validation occurs before every file operation
- File type detection automatically categorizes files by extension

## Key Interfaces

### Browser Interface
```go
type Browser interface {
    ListFiles(ctx context.Context, nodePath string) ([]FileInfo, error)
    ReadFile(ctx context.Context, nodePath, filePath string) ([]byte, error)
    WriteFile(ctx context.Context, nodePath, filePath string, content []byte) error
    CreateDirectory(ctx context.Context, nodePath, dirPath string) error
    Delete(ctx context.Context, nodePath, path string) error
}
```

### FileInfo Structure
```go
type FileInfo struct {
    Name  string `json:"name"`   // File/directory name
    Path  string `json:"path"`   // Relative path within node
    IsDir bool   `json:"isDir"`  // Whether item is directory
    Size  int64  `json:"size"`   // File size in bytes
    Type  string `json:"type"`   // Detected file type
}
```

### Configuration
```go
type Config struct {
    AllowedExtensions []string // File extension whitelist (empty = all allowed)
    MaxFileSize       int64    // Maximum file size in bytes (0 = no limit)
}
```

### Factory Function
```go
func NewBrowser(config Config) Browser
```

## Usage Examples

### Basic File Operations
```go
// Create browser with configuration
browser := files.NewBrowser(files.Config{
    AllowedExtensions: []string{".go", ".yaml", ".json"},
    MaxFileSize:       10 * 1024 * 1024, // 10MB limit
})

// List files in node directory
files, err := browser.ListFiles(ctx, "/path/to/node")
if err != nil {
    return err
}

// Read specific file
content, err := browser.ReadFile(ctx, "/path/to/node", "config.yaml")
if err != nil {
    return err
}

// Write file with automatic directory creation
err = browser.WriteFile(ctx, "/path/to/node", "output/result.json", jsonData)
if err != nil {
    return err
}
```

### File Type Detection
```go
// The module automatically detects file types:
// - Programming languages: .go -> "go", .js -> "javascript", .py -> "python"
// - Config files: .yaml -> "yaml", .json -> "json", .toml -> "toml"
// - Documentation: .md -> "markdown", .rst -> "restructuredtext"
// - Special files: "Dockerfile" -> "dockerfile", "Makefile" -> "makefile"
// - Media: .png -> "image", .mp4 -> "video", .mp3 -> "audio"
```

### Directory Operations
```go
// Create nested directory structure
err := browser.CreateDirectory(ctx, "/path/to/node", "src/components")

// Delete file or entire directory tree
err = browser.Delete(ctx, "/path/to/node", "old_file.txt")
err = browser.Delete(ctx, "/path/to/node", "old_directory")
```

## Configuration

### Security Configuration
- **No-Op Validator**: Currently uses minimal validation (only prevents obvious directory traversal)
- **Node Path Scoping**: All operations require absolute node path, ensuring containment
- **Path Validation**: Rejects paths containing ".." patterns

### Size Limits
- **MaxFileSize**: Enforced on both read and write operations
- **Zero Value**: No size limit when set to 0

### Extension Filtering
- **AllowedExtensions**: Whitelist of permitted file extensions for listing
- **Empty List**: All file types allowed when no extensions specified
- **Directory Exemption**: Directories always shown regardless of extension filters