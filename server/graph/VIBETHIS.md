# server/graph

This module provides dependency graph construction and management for the vibethis system. It integrates with the Moon build system to discover and analyze project dependencies.

## Purpose

The graph module is responsible for:
- Building dependency graphs from Moon project configurations
- Discovering nodes (boxes) and their relationships
- Providing a clean API for graph traversal and node lookup
- Supporting visualization of project architecture

## Architecture

### Key Components

1. **Internal Builder** (`internal/builder/builder.go`)
   - Executes `moon project-graph --json` to extract project structure
   - Parses Moon's dependency information
   - Constructs `core.Graph` structures with nodes and edges
   - Handles symlinks and path resolution

2. **Public API** (`pkg/graph/builder.go`)
   - Provides `GraphBuilder` interface implementation
   - Wraps internal builder with clean public methods
   - Supports node lookup by ID
   - Returns `ErrNodeNotFound` for missing nodes

### Data Flow

1. Builder receives a root path for analysis
2. Executes Moon CLI to get project graph JSON
3. Parses Moon's output into internal structures
4. Transforms Moon nodes into `core.Node` objects
5. Creates `core.Edge` objects from dependency relationships
6. Returns complete `core.Graph` with all nodes and edges

## Moon Integration

The module relies on Moon's project discovery:
- Reads `moon.yml` files from each directory
- Extracts project IDs, descriptions, and dependencies
- Respects Moon's workspace configuration
- Requires Git repository initialization for Moon to function

### Moon Data Structure
```go
type MoonNode struct {
    ID       string
    Source   string
    Root     string
    Language string
    Config   struct {
        DependsOn json.RawMessage // Can be string array or complex objects
    }
    Dependencies []MoonDependency
}
```

## Testing

The module includes comprehensive tests that:
- Create temporary Moon workspace structures
- Test with real Moon CLI execution
- Verify node discovery and dependency resolution
- Support mock data for Moon-less environments

## Usage Example

```go
builder := graph.NewBuilder("/path/to/project")
g, err := builder.BuildGraph(ctx)
// g.Nodes contains all discovered boxes
// g.Edges contains dependency relationships
```

## Important Notes

- Requires Moon CLI to be installed and available in PATH
- All paths are resolved to absolute paths for consistency
- Debug logging is enabled to trace Moon execution
- Supports filtering nodes based on root path
- Each node represents a "box" in the vibethis system