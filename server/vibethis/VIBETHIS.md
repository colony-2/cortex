# server/vibethis

## Purpose
This is the main server application entry point for the vibethis project. It provides a CLI-based Go server that serves both the API endpoints and the React frontend as an embedded single-page application.

## Main Components

### Entry Point
- `main.go` - CLI application using Cobra framework
  - Accepts path argument for project root (defaults to current directory)
  - Supports `--port/-p` flag for custom port (default: 8080)
  - Supports `--new/-n` flag to create new state database
  - Handles graceful shutdown with signal handling

### Internal Structure
- `internal/config/` - Configuration management
  - Validates paths and port numbers
  - Resolves relative paths to absolute
  - Creates directories when `--new` flag is used

- `internal/setup/` - Dependency initialization and server creation
  - Initializes all system dependencies (storage, graph, files, git, container)
  - Creates `.vibethis` database directory in project root
  - Configures CORS for local development (localhost:3000, localhost:5173)

- `internal/static/` - Static file embedding
  - `embed.go` - Production build with embedded React assets
  - `embed_dev.go` - Development build serving from filesystem
  - Uses Go build tags (`prod` vs default) to switch modes

### Build System
- Uses Moon build system (moon.yml configuration)
- Tasks:
  - `build` - Compiles Go binary after copying frontend assets
  - `build-frontend-copy` - Copies React build from web/app/dist
  - `serve` - Runs development server
  - `install` - Production install with embedded assets

### Integration Points
- Depends on sibling server modules:
  - `api` - Web server and HTTP routing
  - `storage` - BoltDB persistence layer
  - `graph` - Project dependency graph builder
  - `files` - File system browser
  - `git` - Git repository operations
  - `container` - Development container management

## Key Features
- Single binary distribution with embedded frontend
- Automatic database initialization
- Graceful shutdown handling
- Development/production build modes
- CORS support for frontend development

## Important Notes
- The `.vibethis` directory is created in the scanned project root
- Frontend assets must be built before production binary compilation
- Uses go:embed for zero-dependency asset serving in production
- Development mode serves assets from filesystem for hot reloading