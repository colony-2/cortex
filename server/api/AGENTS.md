# server/api

## Overview
The server/api module provides the HTTP REST API server for the vibethis application. It serves as the primary interface between frontend clients and backend services, handling graph visualization, file management, Git operations, and container orchestration for component cells (directories with devcontainer support).

## Architecture

### Core Components
- **Web Server** (`pkg/web/server.go`): HTTP server with configurable middleware and extensible routing
- **Handlers** (`internal/handlers/`): Domain-specific request handlers for graph, files, git, and container operations
- **Middleware** (`internal/middleware/`): Cross-cutting concerns including CORS, logging, and panic recovery
- **Test Server** (`cmd/testserver/`): Standalone server binary for e2e testing with memory storage

### Dependencies
The API server integrates with internal service modules:
- `core`: Storage interfaces and graph building abstractions
- `files`: File browsing and manipulation capabilities
- `git`: Git repository operations and version control
- `container`: Docker container management for development environments
- `openapi`: Shared API contracts and type definitions

### Request Flow
1. HTTP requests pass through middleware stack (Recovery → Logging → CORS)
2. Router dispatches to appropriate handlers based on path patterns
3. Handlers resolve cell IDs to filesystem paths via graph builder
4. Business logic delegates to service modules
5. Responses converted from internal types to OpenAPI types

## Key Interfaces

### Server Configuration
```go
type Config struct {
    Port            int      // HTTP listen port
    CORSOrigins     []string // Allowed CORS origins
    StaticPath      string   // Static asset directory path
    EnableWebSocket bool     // WebSocket support flag
    MaxUploadSize   int64    // File upload size limit
}
```

### Server Dependencies
```go
type Dependencies struct {
    Storage         core.Storage         // Position and container ID persistence
    Graph           core.GraphBuilder    // Cell graph construction
    Files           files.Browser        // File system operations
    Git             git.Repository       // Git repository management
    Container       container.Manager    // Docker container lifecycle
    StaticFS        http.FileSystem      // Optional static file system
    ExtensionRoutes []ExtensionRoute     // External module routes
}
```

### Core Handler Functions
```go
// Graph Management
func (h *Handlers) GetGraph(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetPositions(w http.ResponseWriter, r *http.Request)
func (h *Handlers) SavePositions(w http.ResponseWriter, r *http.Request)

// File Operations
func (h *Handlers) GetFiles(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetFile(w http.ResponseWriter, r *http.Request)
func (h *Handlers) PutFile(w http.ResponseWriter, r *http.Request)

// Git Operations
func (h *Handlers) GetGitStatus(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetGitDiff(w http.ResponseWriter, r *http.Request)
func (h *Handlers) GetGitHistory(w http.ResponseWriter, r *http.Request)
func (h *Handlers) CreateGitCommit(w http.ResponseWriter, r *http.Request)

// Container Operations
func (h *Handlers) GetContainerStatus(w http.ResponseWriter, r *http.Request)
func (h *Handlers) CreateContainer(w http.ResponseWriter, r *http.Request)
func (h *Handlers) StartContainer(w http.ResponseWriter, r *http.Request)
func (h *Handlers) StopContainer(w http.ResponseWriter, r *http.Request)
func (h *Handlers) RestartContainer(w http.ResponseWriter, r *http.Request)
func (h *Handlers) ResetContainer(w http.ResponseWriter, r *http.Request)
func (h *Handlers) UpdateDevcontainer(w http.ResponseWriter, r *http.Request)
```

## Usage Examples

### Creating and Starting Server
```go
import (
    "github.com/divisive-ai/vibethis/server/api/pkg/web"
    "github.com/divisive-ai/vibethis/server/storage/pkg/storage"
    "github.com/divisive-ai/vibethis/server/graph/pkg/graph"
    // ... other imports
)

// Configure server
config := web.Config{
    Port:            8080,
    CORSOrigins:     []string{"http://localhost:3000"},
    StaticPath:      "/path/to/static/files",
    EnableWebSocket: false,
    MaxUploadSize:   10 * 1024 * 1024, // 10MB
}

// Initialize dependencies
deps := web.Dependencies{
    Storage:   storage.NewMemoryStorage(),
    Graph:     graph.NewBuilder("/path/to/nodes"),
    Files:     files.NewBrowser(files.Config{}),
    Git:       git.NewRepository(git.Config{
        DefaultAuthor: "User",
        DefaultEmail:  "user@example.com",
    }),
    Container: container.NewManager(container.Config{}),
}

// Create and start server
server := web.NewServer(config, deps)
if err := server.Start(); err != nil {
    log.Fatal(err)
}
```

### Adding Extension Routes
```go
deps.ExtensionRoutes = []web.ExtensionRoute{
    {
        Method:  "GET",
        Path:    "/custom/endpoint",
        Handler: customHandler,
    },
}
```

### Client API Usage Examples
```bash
# Get complete cell graph
curl http://localhost:8080/api/graph

# List files in a cell
curl http://localhost:8080/api/cells/my-cell/files

# Read specific file
curl http://localhost:8080/api/cells/my-cell/files/README.md

# Update file content
curl -X PUT http://localhost:8080/api/cells/my-cell/files/config.json \
  -H "Content-Type: application/json" \
  -d '{"content": "new content"}'

# Get git status
curl http://localhost:8080/api/cells/my-cell/git/status

# Create git commit
curl -X POST http://localhost:8080/api/cells/my-cell/git/commit \
  -H "Content-Type: application/json" \
  -d '{"message": "Update configuration", "files": ["config.json"]}'

# Get container status
curl http://localhost:8080/api/cells/my-cell/container/status

# Create and start container
curl -X POST http://localhost:8080/api/cells/my-cell/container/create
curl -X POST http://localhost:8080/api/cells/my-cell/container/start
```

## Configuration

### Environment Variables
- `PORT`: HTTP server port (default: 8080)
- `CORS_ORIGINS`: Comma-separated list of allowed CORS origins
- `STATIC_PATH`: Directory path for static file serving

### File Upload Limits
- Maximum upload size configurable via `MaxUploadSize` (default: 10MB)
- File paths must be within cell boundaries for security

### Static File Serving
- SPA support with fallback to `index.html` for client-side routing
- Automatic content type detection for web assets
- API paths (`/api/*`) excluded from static handling

### Container Integration
- Automatic devcontainer.json detection and parsing
- Container ID persistence via storage interface
- Docker lifecycle management (create, start, stop, restart, reset)