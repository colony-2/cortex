# server/graph

The graph module builds and manages dependency graphs for the colony2 system by integrating with the Moon build system. It discovers project structure, analyzes dependencies, and provides APIs for graph visualization and traversal.

## Architecture

### Core Components
- **Internal Builder** (`internal/builder/builder.go`): Executes Moon CLI commands, parses JSON output, constructs core.Graph structures
- **Public API** (`pkg/graph/builder.go`): Implements GraphBuilder interface, provides clean public methods for graph operations
- **Core Types** (`core.Graph`, `core.Cell`, `core.Edge`): Domain objects representing dependency relationships

### Component Relationships
Builder → Moon CLI → JSON Parser → Graph Constructor → core.Graph

The internal builder executes `moon project-graph --json`, parses the structured output into MoonGraph intermediary types, then transforms these into core domain objects with proper path resolution and dependency mapping.

## Key Interfaces

### GraphBuilder Interface
```go
type GraphBuilder interface {
    BuildGraph(ctx context.Context) (*Graph, error)
    GetCell(ctx context.Context, cellID string) (*Cell, error)
}
```

### Public Builder API
```go
func NewBuilder(rootPath string) *Builder
func (b *Builder) BuildGraph(ctx context.Context) (*core.Graph, error)  
func (b *Builder) GetCell(ctx context.Context, cellID string) (*core.Cell, error)
```

### Core Domain Types
```go
type Graph struct {
    Cells []Cell `json:"cells"`
    Edges []Edge `json:"edges"`
}

type Cell struct {
    ID           string   `json:"id"`
    Name         string   `json:"name"`
    Path         string   `json:"path"`
    Type         string   `json:"type"`
    Dependencies []string `json:"dependencies"`
}

type Edge struct {
    ID     string `json:"id"`
    Source string `json:"source"`
    Target string `json:"target"`
}
```

## Usage Examples

### Basic Graph Building
```go
import "github.com/colony-2/colony2/server/graph"

builder := graph.NewBuilder("/path/to/workspace")
graph, err := builder.BuildGraph(context.Background())
if err != nil {
    return fmt.Errorf("failed to build graph: %w", err)
}

// Access discovered cells and their dependencies
for _, cell := range graph.Cells {
    fmt.Printf("Cell %s has %d dependencies\n", cell.ID, len(cell.Dependencies))
}
```

### Single Cell Lookup
```go
cell, err := builder.GetCell(context.Background(), "example-api")
if err != nil {
    if errors.Is(err, graph.ErrCellNotFound) {
        return fmt.Errorf("cell not found")
    }
    return err
}

fmt.Printf("Found cell: %s at path %s\n", cell.Name, cell.Path)
```

### Graph Traversal
```go
graph, _ := builder.BuildGraph(ctx)

// Find all cells that depend on a specific cell
targetCell := "database"
dependents := []string{}
for _, edge := range graph.Edges {
    if edge.Target == targetCell {
        dependents = append(dependents, edge.Source)
    }
}
```

## Configuration

The graph module requires:
- **Moon CLI**: Must be installed and available in PATH
- **Git Repository**: Moon requires git initialization in the workspace
- **Moon Workspace**: Workspace must contain `.moon/workspace.yml` configuration
- **Moon Projects**: Each cell directory must contain `moon.yml` with project configuration

### Moon Project Configuration Example
```yaml
id: 'example-api'
language: 'go'
project:
  description: 'API service'
dependsOn:
  - 'example-database'
  - 'example-cache'
```

The builder automatically resolves symlinks, handles absolute path conversion, and filters cells based on the specified root path during graph construction.