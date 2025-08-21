# VibeThis OpenAPI Directory

## Overview
OpenAPI 3.0.3 specification for the VibeThis REST API, providing comprehensive documentation for cell-based dependency graph management, file operations, git integration, and DevContainer lifecycle management. This specification serves as the single source of truth for API contracts and enables client code generation across multiple languages.

## Architecture

### Core Components
- **vibethis-api.yaml**: Complete OpenAPI specification with all endpoints, schemas, and examples
- **moon.yml**: Build system configuration defining the api-openapi project type
- **API Structure**: Five main endpoint categories organized by functional domain

### Component Relationships
```
OpenAPI Spec (vibethis-api.yaml)
├── Graph Operations (/api/graph)
├── Position Management (/api/positions) 
├── File Operations (/api/cells/{cellId}/files)
├── Git Integration (/api/cells/{cellId}/git/*)
└── Container Management (/api/cells/{cellId}/container/*)
```

### Data Model Hierarchy
- **Graph**: Contains Cells and Edges
- **Cell**: Core entity with id, name, path, type, dependencies
- **Position**: UI layout coordinates for cells
- **FileInfo**: File metadata with type detection
- **GitStatus/GitCommit**: Git repository state and history
- **ContainerStatus**: DevContainer lifecycle states

## Key Interfaces

### Graph Operations
```yaml
GET /api/graph
Returns: Graph { cells: Cell[], edges: Edge[] }
```

### Position Management
```yaml
GET /api/positions
Returns: Position[]

POST /api/positions
Body: Position[]
```

### File Operations
```yaml
GET /api/cells/{cellId}/files?path={optional}
Returns: { files: FileInfo[], path: string }

GET /api/cells/{cellId}/files/{filePath}
Returns: string (file content)

PUT /api/cells/{cellId}/files/{filePath}
Body: { content: string }
```

### Git Integration
```yaml
GET /api/cells/{cellId}/git/status
Returns: GitStatus

GET /api/cells/{cellId}/git/diff?staged={boolean}
Returns: string (diff output)

GET /api/cells/{cellId}/git/history?limit={integer}
Returns: GitCommit[]

POST /api/cells/{cellId}/git/commit
Body: { message: string, files?: string[] }
```

### Container Management
```yaml
GET /api/cells/{cellId}/container/status
Returns: { status: ContainerStatus, containerId?: string, hasDevcontainer: boolean }

POST /api/cells/{cellId}/container/{create|start|stop|restart|reset}
Returns: { containerId?: string } | success

PUT /api/cells/{cellId}/container/devcontainer
Body: { content: string }
```

## Usage Examples

### Retrieve Complete Dependency Graph
```typescript
const response = await fetch('/api/graph');
const graph = await response.json();
// graph.cells contains all nodes with dependencies
// graph.edges contains dependency relationships
```

### Save Node Positions
```typescript
const positions = [
  { cellId: "cell1", x: 100.0, y: 200.0 },
  { cellId: "cell2", x: 300.0, y: 400.0 }
];
await fetch('/api/positions', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(positions)
});
```

### Read File Content
```typescript
const response = await fetch('/api/cells/myCell/files/README.md');
const content = await response.text();
```

### Create Git Commit
```typescript
await fetch('/api/cells/myCell/git/commit', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    message: "Update configuration",
    files: ["config.json", "settings.yaml"]
  })
});
```

### Manage DevContainer
```typescript
// Check status
const status = await fetch('/api/cells/myCell/container/status')
  .then(r => r.json());

// Create container
await fetch('/api/cells/myCell/container/create', { method: 'POST' });

// Start container
await fetch('/api/cells/myCell/container/start', { method: 'POST' });
```

## Configuration

### Server Configuration
```yaml
servers:
  - url: http://localhost:8080
    description: Local development server
```

### Moon Build System
```yaml
id: 'api-openapi'
type: 'unknown'
fileGroups:
  specs: ['**/*.yaml', '**/*.yml']
  docs: ['**/*.md']
tasks:
  build:
    command: 'echo'
    args: ['OpenAPI spec is ready']
    inputs: [vibethis-api.yaml]
```

### API Validation Tools
- openapi-generator-cli: `openapi-generator-cli validate -i vibethis-api.yaml`
- swagger-cli: `swagger-cli validate vibethis-api.yaml`
- Import into Swagger UI, ReDoc, Postman, or Insomnia for interactive documentation
- Use OpenAPI Generator for client code generation in multiple languages