# Shai Implementation Gaps Specification

## Overview
This document specifies the missing functionality and test coverage needed to implement shai on top of the existing server/container infrastructure. The current implementation provides ~70% of required functionality, primarily lacking interactive terminal support, selective mount strategies, and CLI interface.

## Gap Analysis Summary

### Existing Capabilities (Can Reuse)
- ✅ DevContainer spec parsing and configuration loading
- ✅ Docker client with multi-platform connection support  
- ✅ Container lifecycle management (create, start, stop, remove)
- ✅ Mount parsing and basic configuration
- ✅ Environment variable expansion
- ✅ Lifecycle command processing
- ✅ Image validation and pulling

### Missing Functionality (Must Implement)
- ❌ TTY/Terminal attachment for interactive shells
- ❌ Selective read-write mount strategy
- ❌ CLI executable with flag parsing
- ❌ Progress reporting system
- ❌ DevContainer feature installation
- ❌ Mount conflict validation

## Required Implementations

### 1. TTY/Terminal Attachment System

#### Location
`internal/devcontainer/terminal.go`

#### Interface
```go
package devcontainer

import (
    "context"
    "io"
    "os"
    
    "github.com/docker/docker/api/types"
    "github.com/docker/docker/client"
    "github.com/moby/term"
)

// TerminalAttachment handles interactive terminal sessions
type TerminalAttachment struct {
    client      *client.Client
    containerID string
    oldState    *term.State
}

// AttachInteractive attaches an interactive terminal to a container
func (m *Manager) AttachInteractive(ctx context.Context, containerID string) error {
    attachment := &TerminalAttachment{
        client:      m.dockerClient.client,
        containerID: containerID,
    }
    return attachment.Start(ctx)
}

// Start begins an interactive terminal session
func (t *TerminalAttachment) Start(ctx context.Context) error {
    // Implementation required - see shai-spec.md lines 114-164
}

// HandleResize handles terminal resize events
func (t *TerminalAttachment) HandleResize(ctx context.Context) {
    // Implementation required
}

// Cleanup restores terminal state
func (t *TerminalAttachment) Cleanup() {
    // Implementation required
}
```

#### Tests Required
```go
// internal/devcontainer/terminal_test.go
func TestAttachInteractive(t *testing.T)
func TestTerminalResize(t *testing.T)  
func TestTerminalCleanup(t *testing.T)
func TestSignalForwarding(t *testing.T)
```

### 2. Selective Mount Strategy

#### Location
`pkg/shai/mounts.go`

#### Interface
```go
package shai

import (
    "fmt"
    "path/filepath"
    "github.com/docker/docker/api/types/mount"
)

// MountBuilder builds selective read-write mount configurations
type MountBuilder struct {
    WorkingDir     string
    ReadWritePaths []string
}

// NewMountBuilder creates a mount builder for selective RW access
func NewMountBuilder(workingDir string, rwPaths []string) (*MountBuilder, error) {
    // Validate paths exist
    // Check for conflicts
    return &MountBuilder{
        WorkingDir:     workingDir,
        ReadWritePaths: rwPaths,
    }, nil
}

// BuildMounts creates Docker mount specifications
// Base directory is read-only, specific paths are read-write
func (m *MountBuilder) BuildMounts() []mount.Mount {
    mounts := []mount.Mount{
        // Base mount: read-only
        {
            Type:     mount.TypeBind,
            Source:   m.WorkingDir,
            Target:   "/workspace",
            ReadOnly: true,
        },
    }
    
    // Add read-write overlays
    for _, rwPath := range m.ReadWritePaths {
        mounts = append(mounts, mount.Mount{
            Type:     mount.TypeBind,
            Source:   filepath.Join(m.WorkingDir, rwPath),
            Target:   filepath.Join("/workspace", rwPath),
            ReadOnly: false,
        })
    }
    
    return mounts
}

// ValidateNoConflicts ensures mount paths don't conflict
func (m *MountBuilder) ValidateNoConflicts() error {
    // Check for overlapping paths
    // Validate all paths exist
    // Return error if conflicts found
    return nil
}
```

#### Tests Required
```go
// pkg/shai/mounts_test.go
func TestBuildMounts(t *testing.T)
func TestMountConflictDetection(t *testing.T)
func TestReadOnlyBase(t *testing.T)
func TestReadWriteOverlays(t *testing.T)
func TestInvalidPaths(t *testing.T)
```

### 3. CLI Implementation

#### Location
`cmd/shai/main.go`

#### Implementation
```go
package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "time"
    
    "github.com/briandowns/spinner"
    "github.com/server/container/pkg/shai"
)

// MultiStringFlag allows multiple -rw flags
type MultiStringFlag []string

func (m *MultiStringFlag) String() string {
    return fmt.Sprint(*m)
}

func (m *MultiStringFlag) Set(value string) error {
    *m = append(*m, value)
    return nil
}

func main() {
    var rwPaths MultiStringFlag
    var containerName string
    var verbose bool
    var noCache bool
    
    flag.Var(&rwPaths, "rw", "Read-write directory (can be specified multiple times)")
    flag.StringVar(&containerName, "name", "", "Container name")
    flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
    flag.BoolVar(&noCache, "no-cache", false, "Force rebuild")
    flag.Parse()
    
    if len(rwPaths) == 0 {
        fmt.Fprintf(os.Stderr, "Error: at least one -rw path required\n")
        os.Exit(1)
    }
    
    // Get working directory
    workingDir, err := os.Getwd()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    
    // Create shai runner
    runner, err := shai.New(shai.Config{
        WorkingDir:     workingDir,
        ReadWritePaths: rwPaths,
        ContainerName:  containerName,
        NoCache:        noCache,
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    
    // Set up progress display
    setupProgressDisplay(runner)
    
    // Start container
    ctx := context.Background()
    container, err := runner.Start(ctx)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    
    // Attach interactive shell
    err = runner.AttachInteractive(ctx, container.ID)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}

func setupProgressDisplay(runner *shai.Runner) {
    var currentSpinner *spinner.Spinner
    
    runner.OnProgress(func(phase shai.Phase, message string) {
        if currentSpinner != nil {
            currentSpinner.Stop()
            fmt.Printf("✓ %s\n", currentSpinner.Suffix)
            currentSpinner = nil
        }
        
        switch phase {
        case shai.PhaseValidating, shai.PhaseCreating:
            fmt.Printf("✓ %s\n", message)
        case shai.PhasePulling, shai.PhaseBuilding, shai.PhaseInstalling:
            currentSpinner = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
            currentSpinner.Suffix = " " + message
            currentSpinner.Start()
        case shai.PhaseStarting:
            fmt.Printf("✓ %s\n", message)
        }
    })
}
```

#### Tests Required
```go
// cmd/shai/main_test.go
func TestFlagParsing(t *testing.T)
func TestMultipleRWPaths(t *testing.T)
func TestMissingRWPath(t *testing.T)
func TestProgressDisplay(t *testing.T)
```

### 4. Progress Reporting System

#### Location
`pkg/shai/progress.go`

#### Interface
```go
package shai

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

// ProgressCallback reports progress during operations
type ProgressCallback func(phase Phase, message string)

// ProgressReporter manages progress callbacks
type ProgressReporter struct {
    callback ProgressCallback
}

// NewProgressReporter creates a progress reporter
func NewProgressReporter() *ProgressReporter {
    return &ProgressReporter{}
}

// SetCallback sets the progress callback
func (p *ProgressReporter) SetCallback(cb ProgressCallback) {
    p.callback = cb
}

// Report sends a progress update
func (p *ProgressReporter) Report(phase Phase, message string) {
    if p.callback != nil {
        p.callback(phase, message)
    }
}

// ReportWithDuration reports progress with elapsed time
func (p *ProgressReporter) ReportWithDuration(phase Phase, message string, start time.Time) {
    elapsed := time.Since(start)
    if elapsed > 2*time.Second {
        message = fmt.Sprintf("%s (%ds)", message, int(elapsed.Seconds()))
    }
    p.Report(phase, message)
}
```

#### Tests Required
```go
// pkg/shai/progress_test.go
func TestProgressReporting(t *testing.T)
func TestProgressCallback(t *testing.T)
func TestProgressWithDuration(t *testing.T)
func TestNoCallbackSet(t *testing.T)
```

### 5. Shai Runner Library

#### Location
`pkg/shai/runner.go`

#### Interface
```go
package shai

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    
    "github.com/server/container/internal/devcontainer"
    "github.com/server/container/pkg/container"
)

// Config represents shai configuration
type Config struct {
    WorkingDir     string   // Directory containing .devcontainer
    ReadWritePaths []string // Paths to mount as read-write
    ContainerName  string   // Optional container name
    NoCache        bool     // Force rebuild
}

// Runner coordinates shai operations
type Runner struct {
    config           Config
    manager          container.Manager
    mountBuilder     *MountBuilder
    progressReporter *ProgressReporter
}

// New creates a new shai runner
func New(config Config) (*Runner, error) {
    // Validate .devcontainer exists
    devPath := filepath.Join(config.WorkingDir, ".devcontainer", "devcontainer.json")
    if _, err := os.Stat(devPath); err != nil {
        return nil, fmt.Errorf("no devcontainer.json found at %s", devPath)
    }
    
    // Create mount builder
    mountBuilder, err := NewMountBuilder(config.WorkingDir, config.ReadWritePaths)
    if err != nil {
        return nil, err
    }
    
    // Create manager
    manager, err := devcontainer.NewManager()
    if err != nil {
        return nil, err
    }
    
    return &Runner{
        config:           config,
        manager:          manager,
        mountBuilder:     mountBuilder,
        progressReporter: NewProgressReporter(),
    }, nil
}

// OnProgress sets the progress callback
func (r *Runner) OnProgress(cb ProgressCallback) {
    r.progressReporter.SetCallback(cb)
}

// Start creates and starts the container
func (r *Runner) Start(ctx context.Context) (*container.Info, error) {
    r.progressReporter.Report(PhaseValidating, "Loading devcontainer configuration...")
    
    // Load devcontainer
    dc, err := devcontainer.LoadDevContainer(filepath.Join(r.config.WorkingDir, ".devcontainer"))
    if err != nil {
        return nil, err
    }
    
    // Apply mount configuration
    mounts := r.mountBuilder.BuildMounts()
    // TODO: Merge mounts with devcontainer config
    
    r.progressReporter.Report(PhaseCreating, "Creating container with mounts...")
    
    // Create container
    containerID, err := r.manager.Create(ctx, r.config.WorkingDir)
    if err != nil {
        return nil, err
    }
    
    r.progressReporter.Report(PhaseStarting, "Starting interactive shell")
    
    // Start container
    err = r.manager.Start(ctx, containerID)
    if err != nil {
        return nil, err
    }
    
    // Get container info
    return r.manager.GetInfo(ctx, containerID)
}

// AttachInteractive attaches an interactive shell
func (r *Runner) AttachInteractive(ctx context.Context, containerID string) error {
    // This will use the new AttachInteractive method from Manager
    if mgr, ok := r.manager.(*devcontainer.Manager); ok {
        return mgr.AttachInteractive(ctx, containerID)
    }
    return fmt.Errorf("manager does not support interactive attachment")
}

// Close cleans up resources
func (r *Runner) Close() error {
    if closer, ok := r.manager.(interface{ Close() error }); ok {
        return closer.Close()
    }
    return nil
}
```

#### Tests Required
```go
// pkg/shai/runner_test.go
func TestRunnerCreation(t *testing.T)
func TestRunnerStart(t *testing.T)
func TestRunnerProgress(t *testing.T)
func TestRunnerAttach(t *testing.T)
func TestRunnerMissingDevContainer(t *testing.T)
```

## Integration Points

### 1. Extending Manager Interface
The existing `pkg/container/manager.go` interface needs one addition:

```go
type Manager interface {
    // ... existing methods ...
    
    // AttachInteractive attaches an interactive terminal session
    // (Optional - only implemented by devcontainer.Manager)
    AttachInteractive(ctx context.Context, containerID string) error
}
```

### 2. DevContainer Mount Integration
Modify `internal/devcontainer/devcontainer.go` to support mount merging:

```go
// MergeMounts combines shai selective mounts with devcontainer mounts
func MergeMounts(dc *DevContainer, shaiMounts []mount.Mount) {
    // Convert existing mounts to Docker SDK format
    // Merge with shai mounts, giving shai precedence
    // Update dc.Mounts field
}
```

### 3. Feature Installation Decision
Two options for DevContainer features:

**Option A: Document Pre-built Image Requirement**
```markdown
## Prerequisites
Shai requires DevContainer images with features pre-installed. 
Use the devcontainers CLI to build images with features:
`devcontainer build --image-name my-shai-image .`
```

**Option B: Implement Feature Installation**
```go
// internal/devcontainer/features.go
type FeatureInstaller struct {
    // Download features from GitHub Container Registry
    // Execute feature installation scripts
    // Handle feature dependencies
}
```

## Test Coverage Requirements

### Unit Tests (Required)
- [ ] Terminal attachment and TTY handling
- [ ] Mount builder with conflict detection
- [ ] Progress reporting callbacks
- [ ] CLI flag parsing
- [ ] Runner lifecycle operations

### Integration Tests (Required)
- [ ] Full shai flow with real Docker
- [ ] Multiple RW paths mounting
- [ ] Container cleanup on exit
- [ ] Signal handling (SIGINT, SIGTERM)
- [ ] Terminal resize events

### E2E Tests (Nice to Have)
- [ ] Complete user journey from CLI to shell
- [ ] File modification verification in RW paths
- [ ] Read-only enforcement outside RW paths
- [ ] Cache persistence across sessions

## Implementation Priority

### Phase 1: Core Functionality (Required for MVP)
1. **TTY Attachment** - Without this, no interactive shell
2. **Mount Builder** - Core shai functionality
3. **CLI Skeleton** - Basic flag parsing and execution
4. **Runner Library** - Coordinate components

### Phase 2: User Experience (Important)
1. **Progress Reporting** - Visual feedback for operations
2. **Error Messages** - Clear, actionable error handling
3. **Signal Handling** - Graceful shutdown
4. **Terminal Resize** - Proper TTY size handling

### Phase 3: Production Ready (Nice to Have)
1. **Feature Installation** - Or document pre-built requirement
2. **Session Management** - Named containers, reattachment
3. **Performance Optimization** - Cache management
4. **Comprehensive Tests** - Full coverage

## Success Criteria

### Functional Requirements Met
- [ ] Can start devcontainer with selective RW mounts
- [ ] Interactive shell works with proper TTY
- [ ] Files can be modified in RW paths
- [ ] Files are read-only outside RW paths
- [ ] Container auto-removes on exit

### Quality Requirements Met
- [ ] Test coverage > 80% for new code
- [ ] No goroutine leaks
- [ ] Graceful error handling
- [ ] Clear progress indicators
- [ ] Documented API

### User Experience Requirements Met
- [ ] < 5 second startup for cached images
- [ ] Responsive terminal interaction
- [ ] Clear error messages
- [ ] Intuitive CLI interface
- [ ] Seamless cleanup

## Conclusion

The existing server/container implementation provides a strong foundation for shai. The primary gaps are in terminal handling (30% of work), selective mounts (20% of work), and CLI interface (20% of work). The remaining 30% involves integration, testing, and polish.

With the existing DevContainer parsing, Docker client, and container lifecycle management already implemented, shai can be built as a focused extension rather than a complete rewrite, significantly reducing implementation complexity and time.