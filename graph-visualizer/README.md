# Graph Visualizer

A powerful tool for visualizing directory dependencies as an interactive directed acyclic graph (DAG). Built with Go backend and Svelte frontend.

![Graph Visualizer](https://img.shields.io/badge/version-1.0.0-blue.svg)
![Go](https://img.shields.io/badge/go-1.21-00ADD8.svg)
![Svelte](https://img.shields.io/badge/svelte-5.0-FF3E00.svg)

## Features

- 📊 **Interactive Graph Visualization**: View your project dependencies as a hierarchical DAG
- 🔍 **File Browser**: Browse files within any node directly from the graph
- 🎨 **Smart Edge Highlighting**: Parent dependencies shown in green, child dependencies in blue
- 🚀 **Single Binary**: Production builds embed all assets into one executable
- 🔄 **Hot Reloading**: Development mode with automatic refresh
- 📱 **Responsive Design**: Works on desktop and tablet devices

## Quick Start

```bash
# Install dependencies
make install

# Run in development mode
make dev

# Build production binary
make build

# Run production binary
./build/graph-visualizer -path="/path/to/scan"
```

## Installation

### Prerequisites

- Go 1.21 or higher
- Node.js 20 or higher
- npm 10 or higher

### From Source

```bash
git clone <repository-url>
cd graph-visualizer
make install
make build
```

## Usage

### Development Mode

Run both frontend and backend in development mode with hot reloading:

```bash
make dev
```

Or run them separately:

```bash
# Terminal 1 - Backend
make dev-server

# Terminal 2 - Frontend
make dev-frontend
```

### Production Mode

Build and run the production binary:

```bash
make build
./build/graph-visualizer -path="./example"
```

### Docker

```bash
# Build Docker image
docker build -t graph-visualizer .

# Run container
docker run -p 8080:8080 -v /path/to/scan:/data graph-visualizer -path="/data"
```

## Dependency File Format

Create a `dependencies.yaml` file in any directory you want to include in the graph:

```yaml
dependencies:
  - module1
  - module2
  - shared-lib
```

Example structure:
```
project/
├── auth-service/
│   └── dependencies.yaml
├── api-gateway/
│   └── dependencies.yaml
├── database/
│   └── dependencies.yaml
└── shared-utils/
    └── dependencies.yaml
```

## Graph Interaction

- **Click a node** to select it and highlight its dependencies
- **Click the folder icon** on any node to browse its files
- **Pan** by dragging the graph
- **Zoom** using the controls or mouse wheel
- **Green edges** show parent dependencies (what this node depends on)
- **Blue edges** show child dependencies (what depends on this node)

## API Endpoints

- `GET /api/graph` - Get the complete dependency graph
- `GET /api/files/{nodeId}?path=` - Browse files for a specific node
- `WS /ws/terminal/{nodeId}` - WebSocket endpoint for future terminal support

## Development

### Project Structure

```
graph-visualizer/
├── server/                 # Go backend
│   ├── main.go            # Main server file
│   ├── embed.go           # Production embedding
│   └── static/            # Embedded assets
├── web/                   # Svelte frontend
│   ├── src/
│   │   ├── lib/          # Components
│   │   └── App.svelte    # Main app
│   └── tests/            # Playwright tests
├── example/              # Example dependency structure
├── Makefile             # Build automation
└── Dockerfile           # Container build
```

### Available Commands

```bash
make help         # Show all available commands
make test         # Run all tests
make lint         # Lint code
make fmt          # Format code
make clean        # Clean build artifacts
make stop         # Stop running server
```

### Testing

```bash
# Run all tests
make test

# Run frontend tests only
make test-frontend

# Run backend tests only
make test-server
```

## Configuration

### Environment Variables

- `PORT` - Server port (default: 8080)
- `SCAN_PATH` - Default path to scan (default: ./example)

### Command Line Options

```bash
graph-visualizer -path="/path/to/scan"
```

## Troubleshooting

### Port Already in Use

```bash
make stop  # Stop server on port 8080
```

### Build Errors

```bash
make clean
make install
make build
```

### Missing Dependencies

See [BUILD.md](BUILD.md) for detailed installation instructions.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- [Svelte Flow](https://svelteflow.dev/) for the graph visualization library
- [Gorilla Mux](https://github.com/gorilla/mux) for HTTP routing
- [Dagre](https://github.com/dagrejs/dagre) for graph layout algorithms