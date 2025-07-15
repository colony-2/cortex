# server/api

## Purpose
The API server provides the HTTP REST API for the vibethis application. It serves as the primary interface between the React frontend and the backend services, handling graph visualization, file management, Git operations, and container orchestration for "boxes" (component directories).

## Architecture

### Core Components
- **Web Server** (`pkg/web/server.go`): Main HTTP server with configurable middleware
- **Handlers** (`internal/handlers/`): Request handlers organized by domain
- **Middleware** (`internal/middleware/`): Cross-cutting concerns (CORS, logging, recovery)
- **Test Server** (`cmd/testserver/`): Standalone server for e2e testing

### Dependencies
The API server integrates with multiple internal packages:
- `core`: Storage and graph building interfaces
- `files`: File browsing capabilities
- `git`: Git repository operations
- `container`: Docker container management
- `openapi`: Shared API types and contracts

## API Endpoints

### Graph Management
- `GET /api/graph` - Retrieve the complete node graph with dependencies
- `GET /api/positions` - Get saved node positions for UI layout
- `POST /api/positions` - Save node positions

### File Operations (per node)
- `GET /api/nodes/{nodeId}/files` - List files in a node's directory
- `GET /api/nodes/{nodeId}/files/{filePath}` - Read specific file content
- `PUT /api/nodes/{nodeId}/files/{filePath}` - Update file content

### Git Operations (per node)
- `GET /api/nodes/{nodeId}/git/status` - Git status for node directory
- `GET /api/nodes/{nodeId}/git/diff` - Git diff for changes
- `GET /api/nodes/{nodeId}/git/history` - Commit history
- `POST /api/nodes/{nodeId}/git/commit` - Create new commit

### Container Management (per node)
- `GET /api/nodes/{nodeId}/container/status` - Container status
- `POST /api/nodes/{nodeId}/container/create` - Create new container
- `POST /api/nodes/{nodeId}/container/start` - Start container
- `POST /api/nodes/{nodeId}/container/stop` - Stop container
- `POST /api/nodes/{nodeId}/container/restart` - Restart container
- `POST /api/nodes/{nodeId}/container/reset` - Reset container

## Request/Response Patterns

### Standard JSON Responses
All API endpoints return JSON responses using types defined in the `openapi` package. Error responses follow HTTP status codes with descriptive error messages.

### Node-Centric Design
Most endpoints are scoped to a specific node ID, which maps to a directory containing a "box" component. The node ID is resolved to a filesystem path through the graph builder.

### File Handling
- File paths in URLs support nested directories
- Maximum upload size is configurable (default: 10MB)
- File browser respects filesystem boundaries

## Authentication & Authorization
Currently, the API server does not implement authentication. The CORS middleware allows configurable origins, and the Authorization header is accepted for future auth implementation.

## Static File Serving

### SPA Support
The server includes Single Page Application (SPA) support:
- Serves static files from configurable directory or embedded filesystem
- Falls back to `index.html` for client-side routing
- Excludes `/api` paths from static handling

### Content Types
Automatically detects and sets appropriate content types for common web assets (HTML, JS, CSS, images, fonts).

## Server Configuration

### Configurable Options
- `Port`: HTTP listen port
- `CORSOrigins`: Allowed CORS origins
- `StaticPath`: Directory for static assets
- `EnableWebSocket`: WebSocket support flag
- `MaxUploadSize`: File upload size limit

### Middleware Stack
1. Recovery (panic handling)
2. Logging (API requests only)
3. CORS (if origins configured)
4. Route handling

## Integration Points

### Storage
Uses pluggable storage interface for:
- Node position persistence
- Container ID mapping

### Graph Builder
Dynamically builds node dependency graphs from filesystem structure.

### Container Manager
Abstracts Docker operations for node-specific containers.

## Testing
Includes a test server binary (`testserver`) that can be run standalone with example data for e2e testing. Supports memory storage and configurable node directories.

## Usage Notes
- All file paths must be absolute internally
- API paths are prefixed with `/api`
- Non-API routes serve the SPA
- Graceful shutdown supported with signal handling