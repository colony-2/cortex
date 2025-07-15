# api/openapi Directory

## Purpose
This directory contains the OpenAPI 3.0.3 specification for the VibeThis REST API. It serves as the single source of truth for API documentation, client generation, and API contract validation.

## Key Files

### vibethis-api.yaml
The complete OpenAPI specification defining all API endpoints, request/response schemas, and error handling. This is the primary API contract file.

### moon.yml
Moon build system configuration for this module. Defines this as an 'api-openapi' project with file groups for specs (*.yaml, *.yml) and docs (*.md).

## API Endpoints Overview

The API is organized into five main categories:

### 1. Graph Operations (`/api/graph`)
- **GET /api/graph**: Retrieves the complete dependency graph of all nodes (boxes) and their relationships
- Returns nodes with their IDs, names, paths, types, and dependencies
- Returns edges representing dependency connections between nodes

### 2. Position Management (`/api/positions`)
- **GET /api/positions**: Retrieves saved visual positions for nodes in the UI
- **POST /api/positions**: Saves or updates node positions (x, y coordinates)
- Used for persisting the visual layout of the dependency graph

### 3. File Operations (`/api/nodes/{nodeId}/files`)
- **GET /api/nodes/{nodeId}/files**: Lists files and directories within a node
- **GET /api/nodes/{nodeId}/files/{filePath}**: Reads file content
- **PUT /api/nodes/{nodeId}/files/{filePath}**: Writes/updates file content
- Supports subdirectory navigation and file type detection

### 4. Git Integration (`/api/nodes/{nodeId}/git/*`)
- **GET /git/status**: Returns branch, clean state, modified files, ahead/behind counts
- **GET /git/diff**: Shows uncommitted changes (with optional staged filter)
- **GET /git/history**: Retrieves commit history with configurable limit
- **POST /git/commit**: Creates commits with optional file staging

### 5. Container Management (`/api/nodes/{nodeId}/container/*`)
- **GET /container/status**: Returns container status (none/stopped/running/error)
- **POST /container/create**: Creates a new DevContainer for the node
- **POST /container/start**: Starts an existing container
- **POST /container/stop**: Stops a running container
- **POST /container/restart**: Restarts a container
- **POST /container/reset**: Removes container and clears associations

## Data Models

### Core Models
- **Node**: Represents a box with id, name, path, type, and dependencies
- **Edge**: Represents a dependency connection between nodes
- **Graph**: Container for nodes and edges

### Supporting Models
- **Position**: Stores x,y coordinates for UI layout
- **FileInfo**: File metadata including name, path, isDir, size, and type
- **GitStatus**: Complete git repository state
- **GitCommit**: Commit metadata with hash, author, date, and message
- **ContainerStatus**: Enumeration of container states

## Integration Points

1. **Frontend (React)**: The web UI consumes these endpoints to:
   - Display and manipulate the dependency graph
   - Browse and edit files within nodes
   - Perform git operations
   - Manage DevContainers

2. **Backend (Golang server)**: Implements these endpoints in the server directory
   - Handles file system operations
   - Executes git commands
   - Manages Docker containers
   - Maintains graph structure

3. **Node System**: Each "box" is a directory that can:
   - Contain files and subdirectories
   - Be a git repository
   - Have an associated DevContainer
   - Declare dependencies on other nodes

## Usage Notes

- All node operations require a valid `nodeId` parameter
- File paths are relative to the node's directory
- Git operations assume the node directory is a git repository
- Container operations integrate with Docker/DevContainers
- The API uses standard HTTP status codes for error handling
- JSON is the primary content type for requests/responses
- File content operations use plain text format

## API Validation

The specification can be validated using standard OpenAPI tools:
- openapi-generator-cli
- swagger-cli
- Can be imported into Swagger UI, ReDoc, Postman, or Insomnia
- Supports client code generation for multiple languages