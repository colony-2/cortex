# Devcontainer Management Specification

## Overview

This specification defines the devcontainer management system for vibethis, including mount strategies, configuration handling, and the new `devcontainer_management` activity type that enables programmatic control of devcontainers from workflows.

## Architecture

### Components

1. **Devcontainer CLI Integration**: Wraps the VS Code devcontainer CLI
2. **Mount Configuration**: Manages complex bind mount scenarios
3. **Container Lifecycle**: Start, stop, and monitor containers
4. **Activity Implementation**: Temporal activity for devcontainer operations

## Mount Strategy

### Overview

The mount strategy enables read-only access to the entire source tree while providing read-write access to specific cell directories.

### Mount Configuration

```yaml
mounts:
  # Base read-only mount of entire repository
  - type: bind
    source: /path/to/repo
    target: /src
    readonly: true
    
  # Cell-specific read-write overlay
  - type: bind
    source: /path/to/repo/cell-name
    target: /src/cell-name
    readonly: false
    
  # Recipe directory mount
  - type: bind
    source: /path/to/repo/recipes/nucleus
    target: /recipes
    readonly: true
```

### Mount Behavior

1. **Base Mount**: Entire repository mounted read-only at `/src`
2. **Overlay Mount**: Cell directory mounted read-write, overlaying the base mount
3. **Atomic Operations**: Cell can modify its own files without affecting other cells
4. **Recipe Access**: Nucleus recipes available at `/recipes`

## Devcontainer Configuration

### Configuration Resolution

Priority order for devcontainer.json discovery:

1. `<cell>/.devcontainer/devcontainer.json`
2. `<cell>/.devcontainer.json`
3. `.devcontainer/devcontainer.json` (root)
4. `.devcontainer.json` (root)

### Default Devcontainer

```json
{
  "name": "VibeThis Cell Environment",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu",
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {},
    "ghcr.io/devcontainers/features/git:1": {},
    "ghcr.io/devcontainers/features/docker-in-docker:2": {}
  },
  "customizations": {
    "vibethis": {
      "nucleus": {
        "enabled": true,
        "port": 7233
      }
    }
  },
  "postCreateCommand": "echo 'Cell environment ready'",
  "remoteUser": "vscode"
}
```

### Cell-Specific Overrides

Cells can provide their own devcontainer.json to:
- Use different base images
- Install language-specific tools
- Configure environment variables
- Set up databases or services

## Activity Implementation

### Activity Type: devcontainer_management

```go
package devcontainer

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "strings"
)

type DevcontainerManagementConfig struct {
    Operation string `yaml:"operation"` // up, down, exec, logs
    Timeout   string `yaml:"timeout"`
}

type DevcontainerUpInput struct {
    WorkspaceFolder string            `json:"workspace_folder"`
    ConfigPath      string            `json:"config_path"`
    ContainerName   string            `json:"container_name"`
    Mounts          []MountConfig     `json:"mounts"`
    Labels          map[string]string `json:"labels"`
    Environment     map[string]string `json:"environment"`
    Features        []string          `json:"features"`
}

type MountConfig struct {
    Type     string `json:"type"`     // bind, volume, tmpfs
    Source   string `json:"source"`   // Host path or volume name
    Target   string `json:"target"`   // Container path
    ReadOnly bool   `json:"readonly"` // Read-only mount
}

type DevcontainerUpOutput struct {
    ContainerID   string                 `json:"container_id"`
    ContainerInfo map[string]interface{} `json:"container_info"`
    Warnings      []string               `json:"warnings"`
}
```

### CLI Integration

The activity wraps the devcontainer CLI with additional functionality:

```bash
# Start container with mounts
devcontainer up \
  --workspace-folder /path/to/workspace \
  --config /path/to/devcontainer.json \
  --container-name custom-name \
  --mount type=bind,source=/host/path,target=/container/path,readonly \
  --label key=value

# Execute command in container
devcontainer exec \
  --workspace-folder /path/to/workspace \
  command args

# Stop container
devcontainer down \
  --workspace-folder /path/to/workspace
```

### Implementation Details

```go
func (a *DevcontainerActivity) Execute(ctx context.Context, config DevcontainerManagementConfig, input DevcontainerUpInput) (DevcontainerUpOutput, error) {
    switch config.Operation {
    case "up":
        return a.startContainer(ctx, input)
    case "down":
        return a.stopContainer(ctx, input)
    case "exec":
        return a.execInContainer(ctx, input)
    default:
        return DevcontainerUpOutput{}, fmt.Errorf("unknown operation: %s", config.Operation)
    }
}

func (a *DevcontainerActivity) startContainer(ctx context.Context, input DevcontainerUpInput) (DevcontainerUpOutput, error) {
    args := []string{
        "up",
        "--workspace-folder", input.WorkspaceFolder,
    }
    
    if input.ConfigPath != "" {
        args = append(args, "--config", input.ConfigPath)
    }
    
    if input.ContainerName != "" {
        args = append(args, "--container-name", input.ContainerName)
    }
    
    // Add mounts
    for _, mount := range input.Mounts {
        mountStr := fmt.Sprintf("type=%s,source=%s,target=%s", 
            mount.Type, mount.Source, mount.Target)
        if mount.ReadOnly {
            mountStr += ",readonly"
        }
        args = append(args, "--mount", mountStr)
    }
    
    // Add labels
    for key, value := range input.Labels {
        args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
    }
    
    // Execute devcontainer CLI
    cmd := exec.CommandContext(ctx, "devcontainer", args...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return DevcontainerUpOutput{}, fmt.Errorf("devcontainer up failed: %w\nOutput: %s", err, output)
    }
    
    // Parse container ID from output
    containerID := extractContainerID(string(output))
    
    // Get container info
    containerInfo, err := getContainerInfo(containerID)
    if err != nil {
        return DevcontainerUpOutput{}, fmt.Errorf("failed to get container info: %w", err)
    }
    
    return DevcontainerUpOutput{
        ContainerID:   containerID,
        ContainerInfo: containerInfo,
    }, nil
}
```

## Container Lifecycle Management

### Start Sequence

1. **Validate Configuration**: Check devcontainer.json exists and is valid
2. **Prepare Mounts**: Resolve paths and validate mount points
3. **Start Container**: Use devcontainer CLI with configuration
4. **Wait for Ready**: Poll container health status
5. **Return Container Info**: Provide ID and connection details

### Stop Sequence

1. **Graceful Shutdown**: Send stop signal to processes
2. **Wait for Termination**: Allow processes to clean up
3. **Force Stop**: Kill container if timeout exceeded
4. **Cleanup**: Remove container if configured

### Health Monitoring

```go
type ContainerHealth struct {
    Status      string    `json:"status"`       // running, stopped, error
    StartedAt   time.Time `json:"started_at"`
    Memory      int64     `json:"memory_bytes"`
    CPUPercent  float64   `json:"cpu_percent"`
    NetworkRx   int64     `json:"network_rx"`
    NetworkTx   int64     `json:"network_tx"`
}
```

## Resource Management

### Container Limits

Default resource constraints:
- CPU: 2 cores
- Memory: 4GB
- Disk: 20GB (if using volume)
- Network: No restrictions

### Cleanup Policy

1. **On Success**: Optional cleanup based on workflow input
2. **On Failure**: Default cleanup unless debugging enabled
3. **Orphan Detection**: Label-based identification of abandoned containers
4. **Scheduled Cleanup**: Periodic cleanup of old containers

## Security Considerations

### Mount Security

1. **Path Validation**: Ensure mounts stay within repository boundaries
2. **ReadOnly Enforcement**: Verify read-only mounts are enforced
3. **No Privileged Access**: Containers run without privileged mode
4. **User Mapping**: Map container user to host user for file permissions

### Network Security

1. **Default Isolation**: Containers on isolated network
2. **No Host Network**: Prevent host network access
3. **Port Restrictions**: Limited exposed ports
4. **DNS Control**: Custom DNS for service discovery

### Container Security

1. **Image Scanning**: Scan base images for vulnerabilities
2. **No Root**: Run as non-root user inside container
3. **Capability Dropping**: Remove unnecessary Linux capabilities
4. **Seccomp Profiles**: Apply security profiles

## Error Handling

### Common Errors

1. **Configuration Not Found**: Clear error with suggestions
2. **Mount Failed**: Validate paths and permissions
3. **Container Start Failed**: Include docker logs
4. **Resource Exhausted**: Queue or reject with clear message
5. **Network Issues**: Retry with backoff

### Error Response Format

```go
type DevcontainerError struct {
    Code        string   `json:"code"`
    Message     string   `json:"message"`
    Details     string   `json:"details"`
    Suggestions []string `json:"suggestions"`
    Retryable   bool     `json:"retryable"`
}
```

## Monitoring and Observability

### Metrics

1. **Container Metrics**: CPU, memory, network, disk usage
2. **Lifecycle Events**: Start, stop, error events
3. **Mount Performance**: I/O statistics per mount
4. **Activity Duration**: Time spent in each operation

### Logging

1. **Structured Logs**: JSON format with correlation IDs
2. **Log Levels**: Debug mount operations, info for lifecycle
3. **Audit Trail**: Record all container operations
4. **Error Context**: Include relevant configuration in errors

## Future Enhancements

1. **Volume Management**: Persistent volumes for dependencies
2. **Image Caching**: Pre-built images for common configurations
3. **GPU Support**: Enable GPU access for ML workloads
4. **Distributed Execution**: Spread containers across nodes
5. **Live Migration**: Move running containers between hosts