# DevContainer Package

This package provides functionality to read and parse devcontainer.json files and construct Docker run commands based on the devcontainer specification.

## Features

- Parse devcontainer.json files according to the official spec
- Support for image-based containers
- Convert devcontainer configuration to docker run commands
- Handle mounts, environment variables, ports, and security settings
- Auto-generated Go types from JSON schema

## Usage

```go
// Load a devcontainer.json file
dc, err := devcontainer.LoadDevContainer("/path/to/.devcontainer/devcontainer.json")
if err != nil {
    log.Fatal(err)
}

// Or find it automatically in a workspace
path, err := devcontainer.FindDevContainerFile("/path/to/workspace")
if err != nil {
    log.Fatal(err)
}
dc, err := devcontainer.LoadDevContainer(path)

// Build docker run configuration
config, err := devcontainer.BuildDockerRunCommand(dc, "/path/to/workspace")
if err != nil {
    log.Fatal(err)
}

// Get docker run arguments
args := config.ToDockerRunArgs()
// args = ["run", "--rm", "-it", "--mount", "...", "image:tag"]
```

## Supported Features

- Image-based containers
- Workspace mounts and folders
- Environment variables
- Port forwarding
- Privileged mode and capabilities
- Security options
- Init process
- Custom user
- Additional mounts

## Not Yet Supported

- Dockerfile-based containers (requires build step)
- Docker Compose configurations
- Features installation
- Lifecycle commands
- Remote environment variables

## Schema Generation

The Go types are auto-generated from the official devcontainer JSON schema:

```bash
go generate ./...
```