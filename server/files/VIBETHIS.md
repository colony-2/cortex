# server/files

## Purpose
The files module provides secure file system operations for managing files within vibethis box nodes. It acts as a controlled gateway for file browsing, reading, writing, and manipulation while enforcing security boundaries to prevent unauthorized access outside of designated node directories.

## Core Capabilities

### File Operations
- **List Files**: Browse directory contents within a node, with optional file type filtering
- **Read Files**: Read file contents with configurable size limits
- **Write Files**: Create or update files within node boundaries
- **Create Directories**: Create nested directory structures
- **Delete**: Remove files or directories

### Security Model
- **Path Validation**: All operations validate that paths remain within the node's directory boundary
- **Directory Traversal Protection**: Prevents "../" attacks and path escaping
- **Size Limits**: Configurable maximum file size for read/write operations
- **Extension Filtering**: Optional whitelist of allowed file extensions

## Key Interfaces

### Browser Interface
The main public interface (`pkg/files/browser.go`) provides:
```go
type Browser interface {
    ListFiles(ctx context.Context, nodePath string) ([]FileInfo, error)
    ReadFile(ctx context.Context, nodePath, filePath string) ([]byte, error)
    WriteFile(ctx context.Context, nodePath, filePath string, content []byte) error
    CreateDirectory(ctx context.Context, nodePath, dirPath string) error
    Delete(ctx context.Context, nodePath, path string) error
}
```

### Configuration
```go
type Config struct {
    AllowedExtensions []string  // File extension whitelist (empty = all allowed)
    MaxFileSize       int64     // Maximum file size in bytes (0 = no limit)
}
```

## Architecture

### Internal Structure
- `internal/browser/`: Core file browsing implementation
- `internal/security/`: Path validation and security enforcement
- `pkg/files/`: Public API and interface definitions

### File Type Detection
The browser automatically detects and categorizes files by extension:
- Programming languages (go, js, python, rust, etc.)
- Config files (yaml, json, toml, etc.)
- Documentation (markdown, rst, tex)
- Media files (images, video, audio)
- Archives and binaries

## Integration Notes

### Node Path Context
All operations require a `nodePath` parameter - the absolute path to a box's node directory. The module ensures all file operations remain within this boundary.

### No-Op Validator
Currently uses a no-op validator since security is enforced by requiring the full node path for each operation. The validator still checks for obvious security issues like directory traversal patterns.

### Usage Example
```go
browser := files.NewBrowser(files.Config{
    AllowedExtensions: []string{".go", ".yaml", ".json"},
    MaxFileSize:       10 * 1024 * 1024, // 10MB
})

// List files in a node
files, err := browser.ListFiles(ctx, "/path/to/node")

// Read a specific file
content, err := browser.ReadFile(ctx, "/path/to/node", "config.yaml")
```

## Important Considerations
- All paths must be absolute when passed to the module
- File operations are synchronous and may block on large files
- The module does not implement file watching or change notifications
- No built-in versioning or backup functionality