# Graph Visualizer

A dependency graph visualization tool built with Go backend and Svelte frontend using Svelte Flow.

## Features

- Scans directories for `dependencies.yaml` files
- Builds a hierarchical dependency graph
- Visualizes the graph using Svelte Flow
- Automatic hierarchical layout using dagre
- Zoom controls and minimap
- Clean, modern UI with node boxes showing dependency information

## Project Structure

```
graph-visualizer/
├── server/          # Go backend
├── web/            # Svelte frontend
├── example/        # Example directories with dependencies
└── run.sh         # Run script
```

## Dependency File Format

Each directory can have a `dependencies.yaml` file listing its dependencies:

```yaml
dependencies:
  - module1
  - module2
  - module3
```

## Running the Application

1. Build the frontend:
   ```bash
   cd web
   npm install
   npm run build
   ```

2. Run the server:
   ```bash
   ./run.sh [path-to-scan]
   ```

   The server will scan the specified path (defaults to `./example`) and serve the application on http://localhost:8080

## Development

### Backend (Go)
- Uses Gorilla mux for routing
- Scans directories recursively for dependency files
- Provides REST API at `/api/graph`
- WebSocket support for future terminal integration

### Frontend (Svelte)
- Built with Svelte 5 and TypeScript
- Uses @xyflow/svelte for graph visualization
- Dagre algorithm for automatic hierarchical layout
- Responsive design with zoom controls

## Example Graph

The `example/` directory contains a sample architecture with various services and their dependencies, demonstrating:
- Shared dependencies (e.g., `logger`, `cache`)
- Multi-level dependencies
- Complex service relationships