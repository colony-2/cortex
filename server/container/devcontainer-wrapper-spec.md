# DevContainer Wrapper Specification

## Overview
An executable within the server/container repository that provides selective read-write mounting capabilities for git repositories using the devcontainer specification.

## Architecture

### Components
```
server/container/
├── cmd/
│   └── devcontainer-wrapper/
│       └── main.go              # Thin CLI wrapper
├── pkg/
│   └── devcontainer/
│       ├── wrapper.go            # Core library functionality
│       ├── config.go             # Configuration handling
│       ├── overlay.go            # Overlay configuration logic
│       └── mount.go              # Mount strategy implementation
```

## Configuration Pattern

### Standard Hierarchy
1. **Base Configuration**: `.devcontainer/devcontainer.json` at git repository root
2. **Overlay Configuration**: `.devcontainer/devcontainer.overlay.json` in target subdirectory
3. **Runtime Configuration**: CLI flags and environment variables

### Example Structure
```
/path/to/git-repo/                         # Git repository root
├── .devcontainer/
│   └── devcontainer.json                  # Base configuration
├── project1/
│   └── .devcontainer/
│       └── devcontainer.overlay.json      # Project-specific overlay
└── nested/path/project2/
    └── .devcontainer/
        └── devcontainer.overlay.json      # Another overlay
```

## Devcontainer Configuration

### Base Configuration (.devcontainer/devcontainer.json)
```json
{
  "name": "Base Development Container",
  "image": "golang:1.21-bullseye",
  "features": {
    "ghcr.io/devcontainers/features/git:1": {}
  },
  "customizations": {
    "vscode": {
      "extensions": ["golang.go"]
    }
  },
  "postCreateCommand": "go version"
}
```

### Overlay Configuration (.devcontainer/devcontainer.overlay.json)
```json
{
  "name": "Project-Specific Container",
  "containerEnv": {
    "PROJECT_ROOT": "${containerWorkspaceFolder}"
  },
  "postCreateCommand": "cd ${containerWorkspaceFolder} && go mod download",
  "mounts": [
    {
      "source": "${localWorkspaceFolder}/.cache",
      "target": "/workspace/.cache",
      "type": "bind"
    }
  ]
}
```

## Library API

### Core Types
```go
package devcontainer

type WrapperConfig struct {
    GitRoot      string              // Path to git repository
    TargetPath   string              // Relative path for read-write access
    BaseConfig   DevContainerConfig  // Parsed base devcontainer.json
    OverlayConfig *DevContainerConfig // Optional overlay configuration
}

type MountStrategy struct {
    ReadOnlyRoot  string   // Mount entire git repo as read-only
    ReadWritePath string   // Specific path for read-write
    ExtraMounts   []Mount  // Additional mounts from configs
}

type Runner interface {
    Start(ctx context.Context, config *WrapperConfig) (*Container, error)
    Stop(ctx context.Context, containerID string) error
    Attach(ctx context.Context, containerID string) error
    Exec(ctx context.Context, containerID string, cmd []string) error
}
```

### Main Library Functions
```go
package devcontainer

// LoadConfiguration loads base and overlay configurations
func LoadConfiguration(gitRoot, targetPath string) (*WrapperConfig, error)

// BuildMountStrategy creates the mount configuration
func BuildMountStrategy(config *WrapperConfig) (*MountStrategy, error)

// CreateContainer creates a container with proper mounts
func CreateContainer(ctx context.Context, config *WrapperConfig) (*Container, error)

// MergeConfigurations merges base and overlay configurations
func MergeConfigurations(base, overlay *DevContainerConfig) *DevContainerConfig
```

## CLI Interface

### Command Structure
```bash
devcontainer-wrapper [flags] <command> [args]
```

### Commands
```bash
# Start a devcontainer with selective mounts
devcontainer-wrapper start [--git-root=.] --target=<relative-path>

# Stop a running devcontainer
devcontainer-wrapper stop --container=<name-or-id>

# Attach to running devcontainer
devcontainer-wrapper attach --container=<name-or-id>

# Execute command in devcontainer
devcontainer-wrapper exec --container=<name-or-id> -- <command> [args]

# List running devcontainers
devcontainer-wrapper list

# Show merged configuration (debug)
devcontainer-wrapper config --target=<relative-path>
```

### Flags
```
Global Flags:
  --git-root string     Git repository root (default: current directory)
  --docker-host string  Docker daemon socket (default: unix:///var/run/docker.sock)
  --verbose            Enable verbose logging

Start Flags:
  --target string      Target subdirectory for read-write access (required)
  --name string        Container name (default: generated from target path)
  --no-overlay         Skip overlay configuration
  --detach            Run container in background
```

## Implementation Details

### Mount Implementation
```go
func (r *DockerRunner) setupMounts(config *WrapperConfig) []mount.Mount {
    mounts := []mount.Mount{
        // 1. Mount entire git repo as read-only
        {
            Type:     mount.TypeBind,
            Source:   config.GitRoot,
            Target:   "/workspace",
            ReadOnly: true,
        },
        // 2. Overlay specific directory as read-write
        {
            Type:     mount.TypeBind,
            Source:   filepath.Join(config.GitRoot, config.TargetPath),
            Target:   filepath.Join("/workspace", config.TargetPath),
            ReadOnly: false,
        },
    }
    
    // 3. Add any additional mounts from configurations
    mounts = append(mounts, config.ExtraMounts...)
    
    return mounts
}
```

### Configuration Merge Strategy
1. Start with base devcontainer.json
2. Deep merge overlay configuration
3. Apply runtime overrides
4. Validate final configuration

Priority (highest to lowest):
- CLI flags
- Environment variables
- Overlay configuration
- Base configuration
- Defaults

### Container Naming Convention
```
devcontainer-<git-repo-name>-<sanitized-target-path>
```

Example: `devcontainer-myproject-nested_path_project2`

## Integration with server/container

### Reuse Existing Components
- Use existing Docker client wrapper
- Leverage container lifecycle management
- Integrate with logging infrastructure
- Use existing error handling patterns

### Example Integration
```go
package main

import (
    "github.com/youorg/server/container/pkg/docker"
    "github.com/youorg/server/container/pkg/devcontainer"
)

func main() {
    client := docker.NewClient()
    wrapper := devcontainer.NewWrapper(client)
    
    config, err := devcontainer.LoadConfiguration(".", "nested/project")
    if err != nil {
        log.Fatal(err)
    }
    
    container, err := wrapper.Start(context.Background(), config)
    if err != nil {
        log.Fatal(err)
    }
    
    defer wrapper.Stop(context.Background(), container.ID)
}
```

## Error Handling

### Common Error Cases
1. Git repository not found
2. Target path doesn't exist
3. No devcontainer.json found
4. Invalid overlay configuration
5. Docker daemon not accessible
6. Mount conflicts

### Error Messages
```
Error: Target path "foo/bar" does not exist in git repository
Error: No .devcontainer/devcontainer.json found at repository root
Error: Failed to merge configurations: invalid overlay structure
```

## Testing Strategy

### Unit Tests
- Configuration loading and merging
- Mount strategy generation
- Path validation and sanitization

### Integration Tests
- Container creation with mounts
- Configuration overlay behavior
- Container lifecycle management

### E2E Tests
- Full workflow from CLI
- Multiple overlay scenarios
- Edge cases (deeply nested paths, etc.)

## Future Enhancements

1. **Watch Mode**: Auto-restart on configuration changes
2. **Multi-Container**: Support devcontainer compose scenarios
3. **Remote Development**: SSH tunneling for remote containers
4. **Cache Management**: Persistent volume for build caches
5. **Security Policies**: Restrict certain mount patterns
6. **Template Support**: Project templates with pre-configured overlays

## Example Usage

```bash
# Basic usage - start container with project2 as read-write
cd /path/to/git-repo
devcontainer-wrapper start --target=nested/path/project2

# Attach to running container
devcontainer-wrapper attach --container=devcontainer-myrepo-nested_path_project2

# Execute command
devcontainer-wrapper exec --container=devcontainer-myrepo-nested_path_project2 -- go test ./...

# Stop container
devcontainer-wrapper stop --container=devcontainer-myrepo-nested_path_project2
```