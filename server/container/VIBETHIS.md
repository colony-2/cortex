# server/container

## Purpose

This directory provides container orchestration and devcontainer lifecycle management for vibethis "boxes". It enables each box to run in its own isolated development container environment using Docker and the devcontainer specification.

## Key Capabilities

### Container Management
- Full container lifecycle operations (create, start, stop, restart, remove)
- Container status monitoring and information retrieval
- Command execution within containers
- WebSocket terminal attachment for interactive access

### Devcontainer Support
- Parses and processes devcontainer.json configurations
- Supports image-based, Dockerfile-based, and Docker Compose containers
- Handles lifecycle commands (onCreate, postStart, postAttach, etc.)
- Variable expansion (${localWorkspaceFolder}, ${containerWorkspaceFolder}, etc.)
- Mount configuration and workspace management
- Environment variable management
- Port forwarding configuration
- Security capabilities and privileged mode support
- Extension and feature installation

### Docker Integration
- Multi-platform Docker client with automatic connection detection
- Supports Docker Desktop, rootless Docker, and remote Docker hosts
- Image validation and pulling
- Container creation with full configuration options
- Volume and bind mount management
- Network configuration

## Architecture

### Public API (`pkg/container/`)
- `Manager` interface: Defines container operations contract
- `Info` struct: Container status and metadata
- `Status` enum: Container states (none, stopped, running, error)
- `TerminalConnection` interface: WebSocket terminal access

### Internal Implementation (`internal/devcontainer/`)
- `DevContainer` struct: Parsed devcontainer.json representation
- `DockerClient`: Low-level Docker SDK wrapper
- `Manager` implementation: Bridges public API to Docker operations
- Comprehensive test coverage including E2E tests with real containers

## Integration Points

- Used by box management services to provide isolated environments
- Each box directory can contain `.devcontainer/devcontainer.json` for custom configuration
- Falls back to default Ubuntu-based devcontainer if no configuration exists
- Supports workspace mounting for code editing and persistence

## Configuration

Devcontainer files support:
- `.devcontainer/devcontainer.json`
- `.devcontainer.json`
- Extends functionality for configuration inheritance

## Testing

- Unit tests: Mock Docker client for fast testing
- Integration tests: Real Docker operations with test containers
- E2E tests: Full lifecycle testing (disabled in CI by default)