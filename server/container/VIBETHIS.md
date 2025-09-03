# server/container

## Overview

Container orchestration and devcontainer lifecycle management system for vibethis "boxes". Provides isolated development environments using Docker and devcontainer specifications, with full container lifecycle operations, ephemeral containers, and terminal integration. Includes `shai` CLI for interactive devcontainer management.

## Architecture

### Public API (`pkg/container/`)
- **Manager Interface**: Core container operations contract with lifecycle methods
- **Info Struct**: Container status and metadata representation
- **Status Enum**: Container states (none, stopped, running, error)
- **TerminalConnection Interface**: WebSocket terminal access abstraction
- **Config Struct**: Docker host and registry authentication configuration

### Internal Implementation (`internal/devcontainer/`)
- **DevContainer**: Complete devcontainer.json parser with flexible mount support (string/object formats)
- **DockerClient**: Multi-platform Docker SDK wrapper with automatic connection detection and image pulling
- **Manager**: Bridges public API to Docker operations with devcontainer integration
- **Docker Integration**: Supports Docker Desktop (prioritized on macOS), rootless Docker, and remote hosts

### Shai CLI (`pkg/shai/` & `cmd/shai/`)
- **EphemeralRunner**: Automated devcontainer setup with lifecycle script execution and progress display
- **MountBuilder**: Selective read-write path mounting for workspace isolation
- **ProgressDisplay**: Clean spinner/checkmark UI with error output on failure only

### Core Components
```
Manager (interface) -> devcontainer.Manager -> DockerClient -> Docker SDK
DevContainer (struct) -> DockerRunConfig -> Container Creation
EphemeralRunner -> DevContainer + MountBuilder -> Automated Setup
Variable Expansion -> Standard devcontainer variables
Lifecycle Commands -> Shell script generation for container setup
Shai CLI -> Ephemeral/Persistent modes with progress display
```

## Key Interfaces

### Manager Interface
```go
type Manager interface {
    Create(ctx context.Context, nodePath string) (containerID string, err error)
    Start(ctx context.Context, containerID string) error
    Stop(ctx context.Context, containerID string) error
    Restart(ctx context.Context, containerID string) error
    Remove(ctx context.Context, containerID string) error
    GetInfo(ctx context.Context, containerID string) (*Info, error)
    GetStatus(ctx context.Context, containerID string) (Status, error)
    Exec(ctx context.Context, containerID string, command []string) (output string, err error)
    AttachWebSocket(ctx context.Context, containerID string) (TerminalConnection, error)
}
```

### DevContainer Configuration
```go
type DevContainer struct {
    DevContainerCommon
    ImageContainer      *ImageContainer
    DockerfileContainer string
    ComposeContainer    *ComposeContainer
    NonComposeBase     *NonComposeBase
}

func LoadDevContainer(path string) (*DevContainer, error)
func BuildDockerRunCommand(dc *DevContainer, workspaceFolder string) (*DockerRunConfig, error)
```

### Docker Operations
```go
type DockerClient struct {
    client *client.Client
}

func (c *DockerClient) CreateContainer(ctx context.Context, config *DockerRunConfig) (string, error)
func (c *DockerClient) ValidateImage(ctx context.Context, imageName string) error
func (c *DockerClient) ExecInContainer(ctx context.Context, containerID string, command []string) (string, error)
```

### Variable Expansion
```go
func GetStandardVariables(workspaceFolder string) map[string]string
func ExpandVariables(dc *DevContainer, vars map[string]string)
```

### Shai CLI Interface
```go
// Ephemeral containers (auto-removed)
type EphemeralRunner struct {
    config           EphemeralConfig
    devContainer     *devcontainer.DevContainer
    docker           *client.Client
    mountBuilder     *MountBuilder
    progress         *ProgressDisplay
}

func NewEphemeralRunner(config EphemeralConfig) (*EphemeralRunner, error)
func (r *EphemeralRunner) Run(ctx context.Context) error

// Mount builder for selective workspace access
type MountBuilder struct {
    workingDir     string
    readWritePaths []string
    dockerMounts   []mount.Mount
}

func NewMountBuilder(workingDir string, rwPaths []string) (*MountBuilder, error)
```

## Shai CLI Tool

Interactive devcontainer management with automatic setup and cleanup.

### Basic Usage
```bash
# Ephemeral mode (default) - container auto-removed on exit
shai -rw /path/to/workspace

# Persistent mode - container remains after exit
shai -rw /path/to/workspace -ephemeral=false -name=my-container

# Multiple read-write paths
shai -rw /path/to/workspace -rw /path/to/data

# Build options
shai -rw /path/to/workspace -no-cache -verbose
```

### Features
- **Automatic Image Pulling**: Downloads missing Docker images
- **Lifecycle Script Execution**: Runs onCreate, postCreate, postStart commands
- **Progress Display**: Clean spinner UI with error output on failure only
- **Mount Validation**: Auto-creates missing cache directories
- **Platform-Optimized**: Docker Desktop prioritized on macOS

### Progress Display
```
⠋ Checking image availability
✅ Image found locally
⠋ Creating container  
✅ Container created
⠋ Starting container
✅ Container started
⠋ Running devcontainer setup
❌ Container setup failed: exit code 1

--- Container Output (exit code 1) ---
[error details shown only on failure]
--- End Container Output ---
```

## Usage Examples

### Basic Container Creation
```go
// Create manager
mgr, err := devcontainer.NewManager()
if err != nil {
    return err
}
defer mgr.Close()

// Create container from node path
containerID, err := mgr.Create(ctx, "/path/to/node")
if err != nil {
    return err
}

// Start container
err = mgr.Start(ctx, containerID)
```

### DevContainer Configuration Loading
```go
// Load devcontainer.json
dc, err := LoadDevContainer("/path/to/.devcontainer/devcontainer.json")
if err != nil {
    return err
}

// Build Docker configuration
config, err := BuildDockerRunCommand(dc, "/workspace")
if err != nil {
    return err
}

// Create container with Docker client
client, _ := NewDockerClient()
containerID, err := client.CreateContainer(ctx, config)
```

### Lifecycle Command Processing
```go
// Parse lifecycle commands
commands, err := ProcessLifecycleCommands(dc)
if err != nil {
    return err
}

// Generate setup script
script, err := GetLifecycleScript(dc, "create")
if err != nil {
    return err
}

// Execute in container
output, err := mgr.Exec(ctx, containerID, []string{"sh", "-c", script})
```

### Variable Expansion
```go
// Get standard variables
vars := GetStandardVariables("/local/workspace")
// vars contains:
// "localWorkspaceFolder": "/local/workspace"
// "containerWorkspaceFolder": "/workspaces/workspace"
// "localWorkspaceFolderBasename": "workspace"

// Expand variables in devcontainer
ExpandVariables(dc, vars)
```

## Configuration

### DevContainer Files Support
- `.devcontainer/devcontainer.json` - Primary configuration location
- `.devcontainer.json` - Root-level alternative
- `extends` field - Configuration inheritance from base configs

### Configuration Types
- **Image-based**: `"image": "mcr.microsoft.com/devcontainers/base:ubuntu"`
- **Dockerfile-based**: `"dockerFile": "Dockerfile"`
- **Docker Compose**: `"dockerComposeFile": "docker-compose.yml"`

### Standard Variables
- `${localWorkspaceFolder}` - Host workspace path
- `${containerWorkspaceFolder}` - Container workspace path
- `${localWorkspaceFolderBasename}` - Workspace directory name
- `${containerWorkspaceFolderBasename}` - Container workspace name

### Docker Connection Detection
**macOS (Darwin)**:
1. Environment settings (DOCKER_HOST)
2. Docker Desktop primary (~/.docker/run/docker.sock)
3. Docker Desktop alternative (~/.docker/desktop/docker.sock)
4. Linux default fallback (/var/run/docker.sock)

**Linux**:
1. Environment settings (DOCKER_HOST)
2. Default Unix socket (/var/run/docker.sock)
3. Rootless Docker (XDG_RUNTIME_DIR or ~/.docker/run/docker.sock)

### Mount Format Support
- **String format**: `"source=/host/path,target=/container/path,type=bind,readonly"`
- **Object format**: `{"type": "bind", "source": "/host/path", "target": "/container/path", "readonly": true}`
- **Auto-creation**: Cache directories (`.cache` paths) created automatically