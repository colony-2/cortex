# Graph Visualizer Build Guide

## Quick Start

```bash
# Install dependencies
make install

# Run development servers (frontend + backend)
make dev

# Build production binary
make build

# Run production binary
make run
```

## Available Make Commands

### Development Commands

- `make dev` - Run both frontend and backend in development mode
  - Frontend runs on http://localhost:5173
  - Backend runs on http://localhost:8080
  
- `make dev-frontend` - Run only the frontend dev server
- `make dev-server` - Run only the Go backend server
- `make watch-server` - Auto-restart server on file changes (requires `entr`)

### Build Commands

- `make build` - Build production binary with embedded frontend
  - Creates a single executable in `build/graph-visualizer`
  - Frontend assets are embedded in the binary
  
- `make build-frontend` - Build only frontend assets
- `make prod` - Full production build (clean, install, test, build)

### Testing Commands

- `make test` - Run all tests (frontend and backend)
- `make test-frontend` - Run only frontend tests
- `make test-server` - Run only backend tests

### Utility Commands

- `make install` - Install all dependencies (npm and Go modules)
- `make clean` - Remove all build artifacts
- `make fmt` - Format Go code
- `make lint` - Lint all code (requires golangci-lint)
- `make help` - Show available commands

### Docker Commands

- `make docker-build` - Build Docker image
- `docker run -p 8080:8080 graph-visualizer:latest`

## Development Workflow

### Option 1: Using Make

```bash
# Terminal 1 - Backend
make dev-server

# Terminal 2 - Frontend
make dev-frontend
```

### Option 2: Using the Dev Script

```bash
# Interactive mode
./scripts/dev.sh

# Direct commands
./scripts/dev.sh dev      # Run both servers
./scripts/dev.sh frontend # Run frontend only
./scripts/dev.sh backend  # Run backend only
```

### Option 3: Single Command

```bash
# Run both servers concurrently
make dev
```

## Production Build

The production build embeds the frontend assets into the Go binary:

```bash
# Build the binary
make build

# Run with custom path
./build/graph-visualizer -path="/path/to/scan"

# Or use the run script
./run /path/to/scan prod
```

## Docker Deployment

```bash
# Build image
docker build -t graph-visualizer .

# Run container
docker run -p 8080:8080 -v /path/to/scan:/data graph-visualizer -path="/data"
```

## Environment Variables

- `PORT` - Server port (default: 8080)
- `SCAN_PATH` - Default path to scan (default: ./example)

## Troubleshooting

### Port Already in Use

```bash
# Kill process on port 8080
lsof -ti:8080 | xargs kill -9

# Or use a different port
PORT=8090 make dev-server
```

### Missing Dependencies

```bash
# Install Go (macOS)
brew install go

# Install Node.js (macOS)
brew install node

# Install entr for watch mode (macOS)
brew install entr
```

### Build Errors

```bash
# Clean and rebuild
make clean
make install
make build
```