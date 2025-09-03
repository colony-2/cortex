# Ephemeral DevContainer Execution Specification

## Overview
This specification defines a simplified, single-stage approach for running ephemeral devcontainers. Containers are created with `--rm` flag, run setup as root, then exec to replace the process with the target user's shell. When the user exits, everything is automatically cleaned up.

## Core Principles
1. **Ephemeral**: Containers are removed automatically when they exit (`--rm`)
2. **Single Stage**: Setup and user session happen in one continuous flow
3. **No Persistence**: No long-lived containers, no start/stop lifecycle
4. **Process Replacement**: Use exec to replace root process with user shell
5. **Automatic Cleanup**: Container and all resources cleaned up on exit

## Implementation Architecture

### Single Run Command
```go
// Run creates and runs an ephemeral devcontainer
func (r *Runner) Run(ctx context.Context) error {
    // 1. Load devcontainer configuration
    dc, err := r.loadDevContainer()
    if err != nil {
        return err
    }
    
    // 2. Build docker configuration
    config, err := r.buildConfig(dc)
    if err != nil {
        return err
    }
    
    // 3. Create setup script that will:
    //    - Install features
    //    - Run lifecycle commands
    //    - Switch to target user
    setupScript := r.generateSetupScript(dc)
    
    // 4. Run container with setup script as entrypoint
    return r.runEphemeralContainer(ctx, config, setupScript)
}
```

### Setup Script Generation with Progress Markers
```go
func (r *Runner) generateSetupScript(dc *DevContainer) string {
    targetUser := "root" // default
    if dc.RemoteUser != nil && *dc.RemoteUser != "" {
        targetUser = *dc.RemoteUser
    } else if dc.ContainerUser != nil && *dc.ContainerUser != "" {
        targetUser = *dc.ContainerUser
    }
    
    script := `#!/bin/sh
set -e

# Progress marker format: ::DEVCONTAINER::<phase>::<status>::<message>
# Phases: FEATURES, ONCREATE, UPDATECONTENT, POSTCREATE, POSTSTART, POSTATTACH, USERSWITCH
# Status: START, COMPLETE, ERROR, PROGRESS

echo "::DEVCONTAINER::INIT::START::Initializing devcontainer setup"

# Install features if specified
%FEATURES%

# Run onCreate lifecycle commands
%ONCREATE%

# Run updateContent commands
%UPDATECONTENT%

# Run postCreate commands
%POSTCREATE%

# Run postStart commands
%POSTSTART%

# Run postAttach commands as target user
%POSTATTACH%

echo "::DEVCONTAINER::USERSWITCH::START::Switching to user %USER%"

# Replace this process with user shell
exec su - %USER% -c 'exec /bin/bash --login'
`
    
    // Replace placeholders with actual commands
    script = strings.ReplaceAll(script, "%FEATURES%", r.generateFeatureInstallsWithProgress(dc))
    script = strings.ReplaceAll(script, "%ONCREATE%", r.generateCommandWithProgress(dc.OnCreateCommand, "ONCREATE"))
    script = strings.ReplaceAll(script, "%UPDATECONTENT%", r.generateCommandWithProgress(dc.UpdateContentCommand, "UPDATECONTENT"))
    script = strings.ReplaceAll(script, "%POSTCREATE%", r.generateCommandWithProgress(dc.PostCreateCommand, "POSTCREATE"))
    script = strings.ReplaceAll(script, "%POSTSTART%", r.generateCommandWithProgress(dc.PostStartCommand, "POSTSTART"))
    script = strings.ReplaceAll(script, "%POSTATTACH%", r.generateCommandWithProgress(dc.PostAttachCommand, "POSTATTACH"))
    script = strings.ReplaceAll(script, "%USER%", targetUser)
    
    return script
}

// generateCommandWithProgress wraps a command with progress markers
func (r *Runner) generateCommandWithProgress(cmd interface{}, phase string) string {
    if cmd == nil {
        return ""
    }
    
    commandStr := r.parseLifecycleCommand(cmd)
    if commandStr == "" {
        return ""
    }
    
    return fmt.Sprintf(`
echo "::DEVCONTAINER::%s::START::Executing %s command"
%s
echo "::DEVCONTAINER::%s::COMPLETE::Completed %s command"
`, phase, strings.ToLower(phase), commandStr, phase, strings.ToLower(phase))
}

// generateFeatureInstallsWithProgress generates feature installation with progress
func (r *Runner) generateFeatureInstallsWithProgress(dc *DevContainer) string {
    if dc.Features == nil || len(dc.Features.Features) == 0 {
        return ""
    }
    
    var script strings.Builder
    script.WriteString(`echo "::DEVCONTAINER::FEATURES::START::Installing devcontainer features"` + "\n")
    
    for featureID, config := range dc.Features.Features {
        script.WriteString(fmt.Sprintf(`echo "::DEVCONTAINER::FEATURES::PROGRESS::Installing feature %s"` + "\n", featureID))
        script.WriteString(r.generateFeatureInstallScript(featureID, config) + "\n")
    }
    
    script.WriteString(`echo "::DEVCONTAINER::FEATURES::COMPLETE::All features installed"` + "\n")
    return script.String()
}
```

### Container Execution
```go
func (r *Runner) runEphemeralContainer(ctx context.Context, config *DockerRunConfig, setupScript string) error {
    // Create a temporary script file
    scriptFile, err := os.CreateTemp("", "devcontainer-setup-*.sh")
    if err != nil {
        return err
    }
    defer os.Remove(scriptFile.Name())
    
    if _, err := scriptFile.WriteString(setupScript); err != nil {
        return err
    }
    scriptFile.Close()
    
    // Build docker run command with:
    // - --rm for automatic cleanup
    // - -it for interactive terminal
    // - Mount setup script
    // - Override entrypoint to run setup script
    
    containerConfig := &container.Config{
        Image:        config.Image,
        Entrypoint:   []string{"/devcontainer-setup.sh"},
        Tty:          true,
        AttachStdin:  true,
        AttachStdout: true,
        AttachStderr: true,
        OpenStdin:    true,
        WorkingDir:   config.WorkspaceFolder,
        Env:          config.Environment,
    }
    
    hostConfig := &container.HostConfig{
        AutoRemove: true, // --rm equivalent
        Mounts: append(config.Mounts, mount.Mount{
            Type:     mount.TypeBind,
            Source:   scriptFile.Name(),
            Target:   "/devcontainer-setup.sh",
            ReadOnly: true,
        }),
    }
    
    // Create and start container
    resp, err := r.docker.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
    if err != nil {
        return fmt.Errorf("failed to create container: %w", err)
    }
    
    // Attach to container
    attachOptions := container.AttachOptions{
        Stream: true,
        Stdin:  true,
        Stdout: true,
        Stderr: true,
    }
    
    hijacked, err := r.docker.ContainerAttach(ctx, resp.ID, attachOptions)
    if err != nil {
        return fmt.Errorf("failed to attach: %w", err)
    }
    defer hijacked.Close()
    
    // Start container
    if err := r.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
        return fmt.Errorf("failed to start: %w", err)
    }
    
    // Set terminal to raw mode
    oldState, err := term.MakeRaw(os.Stdin.Fd())
    if err != nil {
        return fmt.Errorf("failed to set raw mode: %w", err)
    }
    defer term.RestoreTerminal(os.Stdin.Fd(), oldState)
    
    // Handle I/O with progress monitoring
    errCh := make(chan error, 2)
    progressCh := make(chan ProgressUpdate, 100)
    
    // Goroutine to handle stdin
    go func() {
        _, err := io.Copy(hijacked.Conn, os.Stdin)
        errCh <- err
    }()
    
    // Goroutine to handle stdout/stderr with progress parsing
    go func() {
        reader := bufio.NewReader(hijacked.Reader)
        for {
            line, err := reader.ReadString('\n')
            if err != nil {
                if err != io.EOF {
                    errCh <- err
                }
                break
            }
            
            // Check for progress markers
            if strings.HasPrefix(line, "::DEVCONTAINER::") {
                if progress := parseProgressMarker(line); progress != nil {
                    select {
                    case progressCh <- *progress:
                    default: // Don't block if channel is full
                    }
                }
                // Optionally hide progress markers from user
                if r.config.HideProgressMarkers {
                    continue
                }
            }
            
            // Write to stdout
            fmt.Print(line)
        }
        errCh <- nil
    }()
    
    // Goroutine to handle progress updates
    go func() {
        for progress := range progressCh {
            r.handleProgress(progress)
        }
    }()
    
    // Wait for container to exit
    statusCh, errWaitCh := r.docker.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
    
    select {
    case err := <-errCh:
        if err != nil && err != io.EOF {
            return fmt.Errorf("I/O error: %w", err)
        }
    case err := <-errWaitCh:
        if err != nil {
            return fmt.Errorf("wait error: %w", err)
        }
    case <-statusCh:
        // Container exited normally
    case <-ctx.Done():
        return ctx.Err()
    }
    
    return nil
}
```

### Progress Parsing and Display
```go
type ProgressUpdate struct {
    Phase   string // FEATURES, ONCREATE, etc.
    Status  string // START, COMPLETE, ERROR, PROGRESS
    Message string // Human-readable message
}

func parseProgressMarker(line string) *ProgressUpdate {
    // Format: ::DEVCONTAINER::<phase>::<status>::<message>
    parts := strings.Split(strings.TrimPrefix(line, "::DEVCONTAINER::"), "::")
    if len(parts) < 3 {
        return nil
    }
    
    return &ProgressUpdate{
        Phase:   parts[0],
        Status:  parts[1],
        Message: strings.TrimSpace(strings.Join(parts[2:], "::"))
    }
}

func (r *Runner) handleProgress(progress ProgressUpdate) {
    // Update progress display based on configuration
    if r.progressCallback != nil {
        r.progressCallback(progress)
    }
    
    // Could also update a spinner, progress bar, etc.
    switch progress.Status {
    case "START":
        r.startPhase(progress.Phase, progress.Message)
    case "COMPLETE":
        r.completePhase(progress.Phase, progress.Message)
    case "ERROR":
        r.errorPhase(progress.Phase, progress.Message)
    case "PROGRESS":
        r.updatePhase(progress.Phase, progress.Message)
    }
}
```

### CLI Progress Display
```go
// cmd/shai/main.go - progress display implementation
func setupProgressDisplay(runner *shai.Runner) {
    var currentPhase string
    var spinner *spinner.Spinner
    
    runner.OnProgress(func(update shai.ProgressUpdate) {
        switch update.Status {
        case "START":
            if spinner != nil {
                spinner.Stop()
            }
            
            // Clear previous line and show new phase
            fmt.Printf("\r\033[K") // Clear line
            
            switch update.Phase {
            case "FEATURES":
                fmt.Printf("🔧 %s\n", update.Message)
            case "ONCREATE":
                fmt.Printf("📦 %s\n", update.Message)
            case "POSTCREATE":
                fmt.Printf("🔨 %s\n", update.Message)
            case "USERSWITCH":
                fmt.Printf("👤 %s\n", update.Message)
            default:
                fmt.Printf("⚙️  %s\n", update.Message)
            }
            
            currentPhase = update.Phase
            
        case "COMPLETE":
            fmt.Printf("✅ %s\n", update.Message)
            
        case "ERROR":
            fmt.Printf("❌ %s\n", update.Message)
            
        case "PROGRESS":
            // Update current line with progress info
            fmt.Printf("\r\033[K  → %s", update.Message)
        }
    })
}
```

### Simplified CLI Usage
```go
// cmd/shai/main.go
func main() {
    config := shai.Config{
        WorkingDir:     workingDir,
        ReadWritePaths: rwPaths,
        NoCache:        noCache,
    }
    
    runner, err := shai.New(config)
    if err != nil {
        log.Fatal(err)
    }
    
    // Single run command - no create/start/attach separation
    if err := runner.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
    
    // Container is automatically removed when user exits
}
```

## Test Cases

### 1. Progress Monitoring Test
```go
func TestProgressMonitoring(t *testing.T) {
    tests := []struct {
        name           string
        setupScript    string
        expectedPhases []string
    }{
        {
            name: "progress markers are parsed correctly",
            setupScript: `
                echo "::DEVCONTAINER::INIT::START::Initializing"
                echo "::DEVCONTAINER::FEATURES::START::Installing features"
                echo "::DEVCONTAINER::FEATURES::PROGRESS::Installing git"
                echo "::DEVCONTAINER::FEATURES::COMPLETE::Features installed"
                echo "::DEVCONTAINER::ONCREATE::START::Running onCreate"
                echo "::DEVCONTAINER::ONCREATE::COMPLETE::onCreate complete"
                echo "::DEVCONTAINER::USERSWITCH::START::Switching to user"
            `,
            expectedPhases: []string{"INIT", "FEATURES", "ONCREATE", "USERSWITCH"},
        },
        {
            name: "real-time progress streaming",
            setupScript: `
                for i in 1 2 3; do
                    echo "::DEVCONTAINER::FEATURES::PROGRESS::Installing package $i"
                    sleep 0.1
                done
                echo "::DEVCONTAINER::FEATURES::COMPLETE::All packages installed"
            `,
            expectedPhases: []string{"FEATURES"},
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var receivedUpdates []ProgressUpdate
            runner := &Runner{
                progressCallback: func(update ProgressUpdate) {
                    receivedUpdates = append(receivedUpdates, update)
                },
            }
            
            // Run script and capture progress
            output := captureScriptOutput(t, tt.setupScript, runner)
            
            // Verify we received expected phases
            phases := make(map[string]bool)
            for _, update := range receivedUpdates {
                phases[update.Phase] = true
            }
            
            for _, expectedPhase := range tt.expectedPhases {
                assert.True(t, phases[expectedPhase], "Missing phase: %s", expectedPhase)
            }
            
            // Verify real-time streaming (updates received before script completes)
            assert.True(t, len(receivedUpdates) > 0)
        })
    }
}
```

### 2. Setup and User Switching Test
```go
func TestEphemeralContainerSetup(t *testing.T) {
    tests := []struct {
        name     string
        dc       *DevContainer
        validate func(t *testing.T, output string)
    }{
        {
            name: "runs setup as root then switches to user",
            dc: &DevContainer{
                Image: "ubuntu:22.04",
                RemoteUser: stringPtr("vscode"),
                PostCreateCommand: "touch /tmp/setup-complete",
            },
            validate: func(t *testing.T, output string) {
                // Output should show progress markers
                assert.Contains(t, output, "::DEVCONTAINER::INIT::START::")
                assert.Contains(t, output, "::DEVCONTAINER::POSTCREATE::COMPLETE::")
                assert.Contains(t, output, "::DEVCONTAINER::USERSWITCH::START::Switching to user vscode")
                // User should not be able to return to root
            },
        },
        {
            name: "lifecycle commands execute in order",
            dc: &DevContainer{
                OnCreateCommand:      "echo 'Step 1'",
                UpdateContentCommand: "echo 'Step 2'",
                PostCreateCommand:    "echo 'Step 3'",
                PostStartCommand:     "echo 'Step 4'",
                PostAttachCommand:    "echo 'Step 5'",
            },
            validate: func(t *testing.T, output string) {
                // Verify order
                step1Idx := strings.Index(output, "Step 1")
                step2Idx := strings.Index(output, "Step 2")
                step3Idx := strings.Index(output, "Step 3")
                step4Idx := strings.Index(output, "Step 4")
                step5Idx := strings.Index(output, "Step 5")
                
                assert.True(t, step1Idx < step2Idx)
                assert.True(t, step2Idx < step3Idx)
                assert.True(t, step3Idx < step4Idx)
                assert.True(t, step4Idx < step5Idx)
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            runner := &Runner{devContainer: tt.dc}
            script := runner.generateSetupScript(tt.dc)
            
            // Run in test container and capture output
            output := runScriptInTestContainer(t, script)
            tt.validate(t, output)
        })
    }
}
```

### 2. Process Replacement Test
```go
func TestProcessReplacement(t *testing.T) {
    t.Run("exec replaces process - no return to root", func(t *testing.T) {
        dc := &DevContainer{
            Image: "ubuntu:22.04",
            RemoteUser: stringPtr("nobody"),
        }
        
        runner := &Runner{devContainer: dc}
        
        // Start container with test script that tries to escalate
        testScript := `
        # Try to return to root after exec
        exec su - nobody -c 'whoami; exit'
        # This line should never execute
        echo "SHOULD NOT SEE THIS"
        `
        
        output := runScriptInTestContainer(t, testScript)
        
        assert.Contains(t, output, "nobody")
        assert.NotContains(t, output, "SHOULD NOT SEE THIS")
    })
}
```

### 3. Automatic Cleanup Test
```go
func TestAutomaticCleanup(t *testing.T) {
    t.Run("container is removed after exit", func(t *testing.T) {
        runner, _ := shai.New(shai.Config{
            WorkingDir: "/tmp/test",
        })
        
        // Run container in background
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        
        done := make(chan error)
        go func() {
            done <- runner.Run(ctx)
        }()
        
        // Wait a moment for container to start
        time.Sleep(1 * time.Second)
        
        // Get container ID (would need to expose this)
        containerID := runner.getContainerID()
        
        // Verify container exists
        _, err := docker.ContainerInspect(ctx, containerID)
        assert.NoError(t, err)
        
        // Cancel context to exit container
        cancel()
        <-done
        
        // Verify container is removed
        _, err = docker.ContainerInspect(context.Background(), containerID)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "No such container")
    })
}
```

### 4. Mount Permissions Test
```go
func TestMountPermissionsWithEphemeralContainer(t *testing.T) {
    t.Run("selective read-write mounts work correctly", func(t *testing.T) {
        config := shai.Config{
            WorkingDir:     "/workspace",
            ReadWritePaths: []string{"src", "tests"},
        }
        
        runner, _ := shai.New(config)
        script := runner.generateSetupScript(runner.devContainer)
        
        // Add test commands to script
        testCommands := `
        # Test writing to RW directory
        echo "test" > /workspace/src/test.txt && echo "Write to src: OK" || echo "Write to src: FAILED"
        
        # Test writing to RO directory  
        echo "test" > /workspace/docs/test.txt && echo "Write to docs: FAILED" || echo "Write to docs: OK (blocked)"
        `
        
        output := runScriptWithCommands(t, script, testCommands)
        
        assert.Contains(t, output, "Write to src: OK")
        assert.Contains(t, output, "Write to docs: OK (blocked)")
    })
}
```

### 5. Feature Installation Test
```go
func TestFeatureInstallation(t *testing.T) {
    t.Run("features install before user switch", func(t *testing.T) {
        dc := &DevContainer{
            Image: "ubuntu:22.04",
            RemoteUser: stringPtr("vscode"),
            Features: &DevContainerCommonFeatures{
                Features: map[string]interface{}{
                    "ghcr.io/devcontainers/features/git:1": map[string]interface{}{},
                },
            },
        }
        
        runner := &Runner{devContainer: dc}
        script := runner.generateSetupScript(dc)
        
        // Script should install git before switching to vscode user
        assert.Contains(t, script, "apt-get install -y git")
        
        // Verify order: features before user switch
        featuresIdx := strings.Index(script, "apt-get install")
        userSwitchIdx := strings.Index(script, "exec su - vscode")
        assert.True(t, featuresIdx < userSwitchIdx)
    })
}
```

### 6. Error Handling Test
```go
func TestSetupErrorHandling(t *testing.T) {
    t.Run("setup fails fast on error", func(t *testing.T) {
        dc := &DevContainer{
            PostCreateCommand: "exit 1",
            PostStartCommand:  "echo 'Should not run'",
        }
        
        runner := &Runner{devContainer: dc}
        err := runner.Run(context.Background())
        
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "setup failed")
        // PostStartCommand should not have run due to set -e
    })
}
```

## Progress Monitoring Benefits

1. **Real-time Feedback**: Users see what's happening during setup (features installing, commands running)
2. **Debugging**: Clear indication of which phase failed if setup errors occur
3. **User Experience**: No "black box" - users understand why setup takes time
4. **CLI Flexibility**: Can display progress as text, spinner, progress bar, or hide entirely
5. **Structured Output**: Machine-parseable format allows for automation and tooling

## Key Differences from Previous Spec

1. **Single Stage**: No separate create/start/attach phases - just `Run()`
2. **Ephemeral Only**: Containers always use `--rm` flag
3. **Setup Script**: All lifecycle commands run in a single setup script
4. **Process Replacement**: Setup script ends with `exec su -` to replace itself
5. **No Container Management**: No stop/restart/remove - container cleans itself up
6. **Simpler API**: Just `runner.Run()` instead of create/start/attach dance
7. **Progress Monitoring**: Real-time progress updates via stdout markers

## Security Benefits

1. **No Persistent State**: Containers can't accumulate security issues over time
2. **Clean Environment**: Every run starts fresh
3. **Process Replacement**: No way to return to root after user switch
4. **Automatic Cleanup**: No orphaned containers with potential vulnerabilities

## Performance Considerations

1. **Slower Startup**: Setup runs every time (no caching between runs)
2. **Mitigation**: Use images with common tools pre-installed
3. **Future**: Could implement image layer caching for feature installation

## Implementation Notes

The implementation uses a single approach: override the container's entrypoint with our setup script. This works with all images since Docker always allows entrypoint override via the `--entrypoint` flag or API equivalent.

## Implementation Priority

1. **Phase 1**: Basic setup script generation with progress markers
2. **Phase 2**: Lifecycle command support with progress reporting
3. **Phase 3**: Progress parsing and display in CLI
4. **Phase 4**: Feature installation with detailed progress
5. **Phase 5**: Enhanced error handling with progress context