# Shai - Interactive DevContainer Shell Specification

## Overview
`shai` starts a devcontainer in the current directory with selective read-write mount permissions. It requires an existing `.devcontainer/devcontainer.json` configuration and provides both a CLI executable and reusable library API.

**Core functionality:**
- Start devcontainer from current directory
- Mount current directory as read-only by default
- Mount specific paths as read-write (separate bind mounts, not overlays)
- Drop into interactive shell
- Show progress for long-running operations
- Auto-cleanup via Docker's --rm flag

**Library API** enables:
- Programmatic devcontainer creation with selective RW paths
- Custom progress callbacks
- Integration into other tools

**Important Notes:**
- Requires pre-built images with DevContainer features already installed, OR
- Must support DevContainer feature installation during container build
- The current server/container implementation parses but doesn't install features

## Command Interface

### Usage
```bash
shai -rw <path1> [-rw <path2> ...] [flags]
```

### Examples
```bash
# Single read-write directory
shai -rw src

# Multiple read-write directories (applied in order)
shai -rw src -rw tests -rw docs

# Current directory as read-write
shai -rw .

# With custom container name
shai -rw src --name my-dev-env
```

### Flags
```
-rw string     Read-write directory (relative to current dir, can be specified multiple times)
--name         Container name (default: auto-generated)
--verbose      Enable verbose logging
--no-cache     Force rebuild of container image
```

## Architecture

### Integration with server/container
```
server/container/
├── cmd/
│   └── shai/
│       └── main.go              # Thin CLI wrapper
├── pkg/
│   ├── devcontainer/           # Existing devcontainer support
│   │   ├── spec.go             # DevContainer spec parsing
│   │   ├── builder.go          # Container building
│   │   └── runner.go           # Container execution
│   └── shai/                   # Shai-specific logic
│       └── mounts.go           # Mount configuration logic
```

### Responsibility Split

**shai executable (cmd/shai):**
- Parse CLI arguments (-rw flags)
- Call library with current directory
- Display progress to user
- Attach to container with interactive shell
- Handle TTY/signal forwarding

**shai library (pkg/shai):**
- Validate devcontainer exists
- Configure selective mounts (RO base + RW paths)
- Coordinate with devcontainer library
- Provide progress callbacks

**server/container library (pkg/devcontainer):**
- Parse .devcontainer/devcontainer.json
- Handle image building/pulling
- Install devcontainer features
- Process standard mounts from spec
- Execute lifecycle commands (postCreateCommand, etc.)
- Create and manage container

## Shell Handling and TTY Bridging

### Approach: Direct Docker Attach with TTY
The cleanest approach uses Docker's native attach functionality with proper TTY handling:

```go
package shai

import (
    "context"
    "os"
    "os/signal"
    
    "github.com/docker/docker/api/types"
    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/client"
    "github.com/moby/term"
)

type ShellBridge struct {
    client *client.Client
    containerID string
    oldState *term.State
}

func (s *ShellBridge) Start(ctx context.Context) error {
    // 1. Set terminal to raw mode
    oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
    if err != nil {
        return err
    }
    s.oldState = oldState
    
    // 2. Create hijacked connection for attach
    resp, err := s.client.ContainerAttach(ctx, s.containerID, types.ContainerAttachOptions{
        Stream: true,
        Stdin:  true,
        Stdout: true,
        Stderr: true,
    })
    if err != nil {
        return err
    }
    defer resp.Close()
    
    // 3. Handle resize events
    go s.handleResize(ctx)
    
    // 4. Start the container
    if err := s.client.ContainerStart(ctx, s.containerID, types.ContainerStartOptions{}); err != nil {
        return err
    }
    
    // 5. Stream I/O between local terminal and container
    errCh := make(chan error, 2)
    
    // Copy stdin to container
    go func() {
        _, err := io.Copy(resp.Conn, os.Stdin)
        errCh <- err
    }()
    
    // Copy container output to stdout
    go func() {
        _, err := stdcopy.StdCopy(os.Stdout, os.Stderr, resp.Reader)
        errCh <- err
    }()
    
    // Wait for either copy to finish or interrupt
    select {
    case err := <-errCh:
        return err
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (s *ShellBridge) handleResize(ctx context.Context) {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGWINCH)
    
    for {
        select {
        case <-sigCh:
            s.resize()
        case <-ctx.Done():
            return
        }
    }
}

func (s *ShellBridge) resize() {
    size, _ := term.GetWinsize(int(os.Stdin.Fd()))
    s.client.ContainerResize(context.Background(), s.containerID, types.ResizeOptions{
        Height: uint(size.Height),
        Width:  uint(size.Width),
    })
}

func (s *ShellBridge) Cleanup() {
    if s.oldState != nil {
        term.RestoreTerminal(int(os.Stdin.Fd()), s.oldState)
    }
}
```


## Container Configuration

### Base Image
```go
const DefaultImage = "golang:1.24-bookworm"  // Or latest stable Go version
```

### DevContainer Configuration
```json
{
  "name": "shai-dev-environment",
  "image": "golang:1.24-bookworm",
  "features": {
    "ghcr.io/devcontainers/features/git:1": {},
    "ghcr.io/devcontainers/features/common-utils:2": {
      "installZsh": true,
      "configureZshAsDefaultShell": true,
      "uid": "1000",
      "gid": "1000",
      "username": "devuser"
    }
  },
  "customizations": {
    "vscode": {
      "extensions": ["golang.go"]
    }
  },
  "remoteUser": "devuser",
  "containerEnv": {
    "USER": "devuser",
    "HOME": "/home/devuser"
  },
  "postCreateCommand": "go version",
  "mounts": [
    "source=${localEnv:HOME}/.ssh/config,target=/home/devuser/.ssh/config,type=bind,readonly",
    "source=${localEnv:HOME}/.ssh/known_hosts,target=/home/devuser/.ssh/known_hosts,type=bind,readonly",
    "source=${localEnv:HOME}/.gitconfig,target=/home/devuser/.gitconfig,type=bind,readonly",
    "source=${localEnv:HOME}/.aws/credentials,target=/home/devuser/.aws/credentials,type=bind,readonly",
    "source=${localEnv:HOME}/.cache/go-build,target=/home/devuser/.cache/go-build,type=bind",
    "source=${localEnv:HOME}/.cache/go-mod,target=/home/devuser/.cache/go-mod,type=bind",
    "source=${localEnv:HOME}/Library/Application Support/Claude,target=/home/devuser/Library/Application Support/Claude,type=bind",
    "source=${localEnv:HOME}/.local/share/nvim,target=/home/devuser/.local/share/nvim,type=bind",
    "source=${localEnv:HOME}/.config/gh,target=/home/devuser/.config/gh,type=bind"
  ]
}
```

## Library API

### Core Types and Interfaces
```go
// Package shai provides selective read-write mount overlays for devcontainers
package shai

import (
    "context"
    "github.com/server/container/pkg/devcontainer"
)

// Config represents the configuration for a shai session
type Config struct {
    WorkingDir     string   // Directory containing .devcontainer
    ReadWritePaths []string // Paths to mount as read-write (in order)
    ContainerName  string   // Optional container name
    NoCache        bool     // Force rebuild
}

// ProgressCallback reports progress during long operations
type ProgressCallback func(phase Phase, message string)

// Phase represents a stage in container creation
type Phase string

const (
    PhaseValidating  Phase = "validating"
    PhasePulling     Phase = "pulling"
    PhaseBuilding    Phase = "building"
    PhaseInstalling  Phase = "installing"
    PhaseCreating    Phase = "creating"
    PhaseStarting    Phase = "starting"
)

// Runner provides the main library interface
type Runner struct {
    config           Config
    devcontainer     *devcontainer.Runner
    progressCallback ProgressCallback
}

// New creates a new shai runner
func New(config Config) (*Runner, error) {
    // Validate .devcontainer exists
    devPath := filepath.Join(config.WorkingDir, ".devcontainer", "devcontainer.json")
    if _, err := os.Stat(devPath); err != nil {
        return nil, fmt.Errorf("no devcontainer.json found at %s", devPath)
    }
    
    return &Runner{
        config: config,
    }, nil
}

// OnProgress sets the progress callback
func (r *Runner) OnProgress(cb ProgressCallback) {
    r.progressCallback = cb
}

// Start creates and starts the container, returning when ready for interaction
func (r *Runner) Start(ctx context.Context) (*Container, error) {
    // Load devcontainer config
    r.reportProgress(PhaseValidating, "Loading devcontainer configuration...")
    devConfig, err := devcontainer.LoadConfig(r.config.WorkingDir)
    if err != nil {
        return nil, err
    }
    
    // Add selective mounts
    additionalMounts := r.buildMounts()
    devConfig.Mounts = append(devConfig.Mounts, additionalMounts...)
    
    // Create devcontainer runner
    r.devcontainer = devcontainer.NewRunner(devcontainer.RunnerOptions{
        Config:     devConfig,
        WorkingDir: r.getWorkingDir(),
        AutoRemove: true,
        NoCache:    r.config.NoCache,
    })
    
    // Forward progress
    r.devcontainer.OnProgress = func(phase string, msg string) {
        r.reportProgress(Phase(phase), msg)
    }
    
    // Create and start container
    return r.devcontainer.Create(ctx)
}

// AttachInteractive attaches an interactive shell session
func (r *Runner) AttachInteractive(ctx context.Context, containerID string) error {
    return r.devcontainer.AttachInteractive(ctx, containerID)
}

// buildMounts creates mount specifications with selective RW paths
// Note: These are separate bind mounts, not true overlay filesystems.
// The base directory is mounted read-only, then specific subdirectories
// are mounted read-write on top.
func (r *Runner) buildMounts() []string {
    var mounts []string
    
    // Base mount: working directory as read-only
    mounts = append(mounts, fmt.Sprintf(
        "source=%s,target=/workspace,type=bind,readonly",
        r.config.WorkingDir,
    ))
    
    // Mount specific paths as read-write (these override the RO base for their paths)
    for _, rwPath := range r.config.ReadWritePaths {
        mounts = append(mounts, fmt.Sprintf(
            "source=%s,target=/workspace/%s,type=bind",
            filepath.Join(r.config.WorkingDir, rwPath),
            rwPath,
        ))
    }
    
    return mounts
}
```

### Home Directory Mount Resolution
```go
// ParseDevContainerMounts parses standard devcontainer mount specifications
func ParseDevContainerMounts(mounts []interface{}) ([]Mount, error) {
    var result []Mount
    
    for _, m := range mounts {
        switch v := m.(type) {
        case string:
            // Parse string format: "source=...,target=...,type=...[,readonly]"
            mount := Mount{}
            parts := strings.Split(v, ",")
            for _, part := range parts {
                kv := strings.SplitN(part, "=", 2)
                if len(kv) == 1 {
                    if kv[0] == "readonly" {
                        mount.ReadOnly = true
                    }
                } else {
                    switch kv[0] {
                    case "source":
                        mount.Source = expandEnvVars(kv[1])
                    case "target":
                        mount.Target = kv[1]
                    case "type":
                        mount.Type = kv[1]
                    }
                }
            }
            result = append(result, mount)
            
        case map[string]interface{}:
            // Parse object format
            mount := Mount{
                Source:   expandEnvVars(getString(v, "source")),
                Target:   getString(v, "target"),
                Type:     getString(v, "type"),
                ReadOnly: getBool(v, "readonly"),
            }
            result = append(result, mount)
        }
    }
    
    return result, nil
}

func expandEnvVars(s string) string {
    // Handle ${localEnv:VAR} syntax
    re := regexp.MustCompile(`\$\{localEnv:([^}]+)\}`)
    return re.ReplaceAllStringFunc(s, func(match string) string {
        varName := re.FindStringSubmatch(match)[1]
        return os.Getenv(varName)
    })
}

func LoadHomeMounts(config *DevContainerConfig) (*MountConfig, error) {
    mc := &MountConfig{
        UID: os.Getuid(),
        GID: os.Getgid(),
    }
    
    // Parse standard mounts array
    mounts, err := ParseDevContainerMounts(config.Mounts)
    if err != nil {
        return nil, err
    }
    
    homeDir := os.Getenv("HOME")
    for _, mount := range mounts {
        // Check if this is a home directory mount
        if strings.HasPrefix(mount.Source, homeDir) {
            relPath := strings.TrimPrefix(mount.Source, homeDir+"/")
            if mount.ReadOnly {
                mc.HomeROMounts = append(mc.HomeROMounts, relPath)
            } else {
                mc.HomeRWMounts = append(mc.HomeRWMounts, relPath)
            }
        }
    }
    
    // Add defaults if not already present
    addIfMissing := func(list []string, item string) []string {
        for _, v := range list {
            if v == item {
                return list
            }
        }
        // Check if file exists before adding
        if _, err := os.Stat(filepath.Join(homeDir, item)); err == nil {
            return append(list, item)
        }
        return list
    }
    
    // Default read-only mounts
    mc.HomeROMounts = addIfMissing(mc.HomeROMounts, ".ssh/config")
    mc.HomeROMounts = addIfMissing(mc.HomeROMounts, ".gitconfig")
    
    // Default read-write mounts for caches
    mc.HomeRWMounts = addIfMissing(mc.HomeRWMounts, ".cache/go-build")
    mc.HomeRWMounts = addIfMissing(mc.HomeRWMounts, ".cache/go-mod")
    
    return mc, nil
}
```

## User Experience

### What the User Sees
```bash
$ shai -rw src -rw tests
✓ Validating devcontainer configuration
✓ Pulling image golang:1.24-bookworm (using cache)
⠋ Installing devcontainer features [git, common-utils]... (45s)
✓ Creating container with mounts:
  - /current/dir → /workspace (read-only)
  - /current/dir/src → /workspace/src (read-write)
  - /current/dir/tests → /workspace/tests (read-write)
  - ~/.gitconfig → /home/devuser/.gitconfig (read-only)
  - ~/.cache/go-mod → /home/devuser/.cache/go-mod (read-write)
⠙ Running post-create commands... (3s)
✓ Starting interactive shell

devuser@container:/workspace/src$ 
```

### Progress Indicators
Each long-running operation shows:
- **Spinner** (⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏) for ongoing operations
- **Checkmark** (✓) for completed steps
- **Time elapsed** for operations > 2 seconds
- **Clear descriptions** of what's happening

### Startup Phases with Timing
1. **Validation** (< 0.1s): Check devcontainer.json exists
2. **Image Pull** (0-30s): Pull base image if not cached
3. **Feature Installation** (0-60s): Install devcontainer features
4. **Container Creation** (< 1s): Create container with all mounts
5. **Post-Create** (0-10s): Run postCreateCommand from spec
6. **Shell** (instant): Drop into interactive session

## Main Flow (CLI Implementation)

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"
    
    "github.com/server/container/pkg/shai"
    "github.com/briandowns/spinner"
)

func main() {
    // Parse CLI flags
    flags := parseFlags()
    
    // Get current directory
    workingDir, err := os.Getwd()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    
    // Create shai runner
    runner, err := shai.New(shai.Config{
        WorkingDir:     workingDir,
        ReadWritePaths: flags.ReadWritePaths,
        ContainerName:  flags.Name,
        NoCache:        flags.NoCache,
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    
    // Set up progress display with spinner
    var currentSpinner *spinner.Spinner
    runner.OnProgress(func(phase shai.Phase, message string) {
        // Stop previous spinner if running
        if currentSpinner != nil {
            currentSpinner.Stop()
            fmt.Printf("✓ %s\n", currentSpinner.Suffix)
        }
        
        // Handle different phases
        switch phase {
        case shai.PhaseValidating, shai.PhaseCreating:
            // Quick operations - just show checkmark
            fmt.Printf("✓ %s\n", message)
            
        case shai.PhasePulling, shai.PhaseBuilding, shai.PhaseInstalling:
            // Long operations - show spinner
            currentSpinner = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
            currentSpinner.Suffix = " " + message
            currentSpinner.Start()
            
        case shai.PhaseStarting:
            fmt.Printf("✓ %s\n", message)
        }
    })
    
    // Create and start container
    ctx := context.Background()
    container, err := runner.Start(ctx)
    if err != nil {
        if currentSpinner != nil {
            currentSpinner.Stop()
        }
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    
    // Stop final spinner
    if currentSpinner != nil {
        currentSpinner.Stop()
        fmt.Printf("✓ %s\n", currentSpinner.Suffix)
    }
    
    // Attach interactive shell
    err = runner.AttachInteractive(ctx, container.ID)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

## Library Usage Example

```go
package myapp

import (
    "context"
    "log"
    
    "github.com/server/container/pkg/shai"
)

func RunDevContainer() error {
    // Create runner with configuration
    runner, err := shai.New(shai.Config{
        WorkingDir:     "/path/to/project",
        ReadWritePaths: []string{"src", "tests"},
    })
    if err != nil {
        return err
    }
    
    // Optional: Set progress callback
    runner.OnProgress(func(phase shai.Phase, msg string) {
        log.Printf("[%s] %s", phase, msg)
    })
    
    // Start container
    ctx := context.Background()
    container, err := runner.Start(ctx)
    if err != nil {
        return err
    }
    
    // Use container programmatically or attach shell
    return runner.AttachInteractive(ctx, container.ID)
}
```

## Container Lifecycle & Cleanup Strategy

### Simple Cleanup with --rm Flag
```go
func RunContainer(ctx context.Context, config *Config) error {
    // Use docker run with --rm for automatic cleanup
    args := []string{
        "run",
        "--rm",        // Automatically remove container when it exits
        "-it",         // Interactive with TTY
        "--name", generateContainerName(config),
    }
    
    // Add user configuration
    uid := os.Getuid()
    gid := os.Getgid()
    args = append(args, "--user", fmt.Sprintf("%d:%d", uid, gid))
    
    // Add mounts
    for _, mount := range config.BuildMounts() {
        args = append(args, "-v", mount.String())
    }
    
    // Add environment
    args = append(args, 
        "-e", "USER=devuser",
        "-e", "HOME=/home/devuser",
        "-w", "/workspace",
    )
    
    // Image and command
    args = append(args, config.Image, "/bin/bash")
    
    // Execute docker run
    cmd := exec.Command("docker", args...)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    return cmd.Run()
}
```

### Direct Docker API with AutoRemove
```go
func CreateContainer(ctx context.Context, config *Config) (*container.ContainerCreateCreatedBody, error) {
    containerConfig := &container.Config{
        Image:      config.Image,
        User:       fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
        Env:        []string{"USER=devuser", "HOME=/home/devuser"},
        WorkingDir: "/workspace",
        Tty:        true,
        OpenStdin:  true,
        StdinOnce:  true,
        Cmd:        []string{"/bin/bash"},
    }
    
    hostConfig := &container.HostConfig{
        AutoRemove: true,  // Container removes itself when stopped
        Mounts:     config.BuildMounts(),
    }
    
    return client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
}
```

1. **Startup**:
   - Parse command line arguments
   - Detect git repository root
   - Validate all RW paths exist
   - Create container with `--rm` or `AutoRemove: true`
   - Configure container to run as non-root user (current UID/GID)
   - Attach to container with TTY
   - Start container
   - Enter interactive shell

2. **During Session**:
   - Forward all I/O between local terminal and container
   - Handle terminal resize events
   - Container has full access to mounted directories

3. **Shutdown** (any termination including kill -9):
   - Docker automatically removes container when process exits
   - No manual cleanup needed
   - Works even if shai is forcefully killed

## Error Handling

### Common Errors
```
Error: not in a git repository
Error: directory "foo/bar" does not exist in repository
Error: Docker daemon not accessible
Error: failed to allocate TTY
Error: container failed to start
```

### Recovery
- Always restore terminal state on exit
- Always clean up container, even on error
- Provide clear error messages with context

## Implementation Notes

### Signal Handling
- SIGWINCH: Resize container TTY
- SIGINT/SIGTERM: Graceful shutdown
- Other signals: Pass through to container

### TTY Requirements
- Requires running in a terminal (check with `terminal.IsTerminal`)
- Falls back to non-interactive mode if no TTY available
- Properly saves/restores terminal state

### Working Directory
The container starts in `/workspace` or as specified in devcontainer.json:
```go
func GetWorkingDirectory(devConfig *DevContainerConfig) string {
    // Use devcontainer.json workspaceFolder if specified
    if devConfig.WorkspaceFolder != nil {
        return *devConfig.WorkspaceFolder
    }
    // Default to /workspace
    return "/workspace"
}
```

## Testing Approach

### Manual Testing
```bash
# Test single RW directory
echo "test" > project1/test.txt
shai -rw project1
# In container: echo "modified" > test.txt
# Exit and verify file was modified

# Test multiple RW directories
shai -rw project1 -rw project2
# Verify both are writable

# Test read-only enforcement
shai -rw project1
# Try to write outside project1, should fail
```

### Automated Testing
- Mock Docker client for unit tests
- Integration tests with real Docker daemon
- TTY handling tests with pseudo-terminals (pty)

## Future Enhancements

1. **Session Persistence**: Option to keep container alive after exit
2. **Named Sessions**: Reattach to existing containers
3. **Custom Images**: Support for different base images via devcontainer.json
4. **Port Forwarding**: Automatic port forwarding for development servers
5. **Volume Caching**: Persistent volumes for package caches

## User Configuration

### DevContainer Standard Approach
User provisioning is handled by DevContainer features during image build time, not at runtime:

```json
// In devcontainer.json
{
  "features": {
    "ghcr.io/devcontainers/features/common-utils:2": {
      "installZsh": true,
      "configureZshAsDefaultShell": true,
      "uid": "1000",
      "gid": "1000",
      "username": "devuser"
    }
  },
  "remoteUser": "devuser",    // User to switch to after container starts
  "containerUser": "devuser"  // User to run container processes as
}
```

### Runtime User Mapping
```go
func SetupContainerUser(config *container.Config, devConfig *DevContainerConfig) {
    // If devcontainer specifies a user, use it
    if devConfig.RemoteUser != nil {
        config.User = *devConfig.RemoteUser
    } else if devConfig.ContainerUser != nil {
        config.User = *devConfig.ContainerUser
    } else {
        // Fall back to current user's UID/GID for file permission consistency
        uid := os.Getuid()
        gid := os.Getgid()
        config.User = fmt.Sprintf("%d:%d", uid, gid)
    }
}
```

**Note**: The container must either:
1. Use a pre-built image with the user already created (via DevContainer features)
2. Or run as root initially to allow feature installation, then switch to the specified user

## Example Session

```bash
$ cd /path/to/my-git-repo
$ shai -rw server/api -rw server/worker
Starting container with golang:1.24-bookworm...
Mounting /path/to/my-git-repo as read-only
Mounting server/api as read-write
Mounting server/worker as read-write
Mounting .ssh/config (read-only)
Mounting .gitconfig (read-only)
Mounting .cache/go-build (read-write)
Mounting .cache/go-mod (read-write)
Mounting Library/Application Support/Claude (read-write)
Running as user devuser (1000:1000)
Attaching to container...

devuser@container:/workspace/server/api$ go test ./...
# ... test output (uses shared go-build cache) ...

devuser@container:/workspace/server/api$ cd ../worker
devuser@container:/workspace/server/worker$ go build .
# ... build output (caches to shared go-build) ...

devuser@container:/workspace/server/worker$ git log --oneline
# Works because .gitconfig is mounted

devuser@container:/workspace/server/worker$ exit
# Container automatically removed by Docker

$ # Even if killed with -9, container is removed:
$ shai -rw project1 &
[1] 12345
$ kill -9 12345
$ docker ps -a | grep shai
# No orphaned containers - Docker's --rm handled cleanup
```

### Cache Persistence Benefits
```bash
# First run - downloads dependencies
$ shai -rw server/api
devuser@container:/workspace/server/api$ go mod download
# Downloads to ~/.cache/go-mod

devuser@container:/workspace/server/api$ go build .
# Builds to ~/.cache/go-build

devuser@container:/workspace/server/api$ exit

# Second run - uses cached dependencies
$ shai -rw server/worker  
devuser@container:/workspace/server/worker$ go build .
# Uses cached modules and build artifacts - much faster!
```