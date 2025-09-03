# DevContainer Lifecycle Command Execution Specification

## Overview
This specification defines how devcontainer lifecycle commands should be executed during container creation, startup, and attachment. The implementation ensures proper user context switching and command execution without requiring custom Docker images.

## Key Requirements
1. Execute devcontainer lifecycle commands at appropriate stages
2. Start container as root for initial setup
3. Switch to remoteUser for interactive sessions
4. Ensure user cannot escalate back to root
5. Support features installation without custom images

## Lifecycle Stages and Commands

### 1. Container Creation Stage
When `Manager.Create()` is called:
```go
func (m *Manager) Create(ctx context.Context, nodePath string) (string, error) {
    // 1. Load devcontainer.json
    // 2. Build Docker config (WITHOUT setting User field - start as root)
    // 3. Create container
    // 4. Start container temporarily
    // 5. Execute onCreate lifecycle commands as root:
    //    - initializeCommand (run on host, not in container)
    //    - onCreateCommand  
    //    - updateContentCommand
    //    - postCreateCommand
    // 6. Install features if specified
    // 7. Stop container
    // 8. Return container ID
}
```

### 2. Container Start Stage
When `Manager.Start()` is called:
```go
func (m *Manager) Start(ctx context.Context, containerID string) error {
    // 1. Start the container
    // 2. Execute postStartCommand as root
    // 3. Container remains running
}
```

### 3. Interactive Attachment Stage
When `Manager.AttachInteractive()` is called:
```go
func (m *Manager) AttachInteractive(ctx context.Context, containerID string) error {
    // 1. Determine target user (remoteUser or containerUser or "root")
    // 2. Execute postAttachCommand as target user
    // 3. Exec interactive shell as target user using "exec" to replace process
    // 4. When user exits, container stops (no escalation possible)
}
```

## Implementation Details

### Lifecycle Command Execution
```go
// ExecuteLifecycleCommands runs lifecycle commands for a given phase
func (m *Manager) ExecuteLifecycleCommands(ctx context.Context, containerID string, dc *DevContainer, phase string) error {
    script, err := GetLifecycleScript(dc, phase)
    if err != nil {
        return fmt.Errorf("failed to generate lifecycle script: %w", err)
    }
    
    if script == "" {
        return nil // No commands for this phase
    }
    
    // Write script to container
    scriptPath := fmt.Sprintf("/tmp/devcontainer-%s.sh", phase)
    writeCmd := []string{"sh", "-c", fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", scriptPath, script)}
    if _, err := m.docker.ExecInContainer(ctx, containerID, writeCmd); err != nil {
        return fmt.Errorf("failed to write lifecycle script: %w", err)
    }
    
    // Make script executable
    if _, err := m.docker.ExecInContainer(ctx, containerID, []string{"chmod", "+x", scriptPath}); err != nil {
        return fmt.Errorf("failed to make script executable: %w", err)
    }
    
    // Execute script
    output, err := m.docker.ExecInContainer(ctx, containerID, []string{"sh", scriptPath})
    if err != nil {
        return fmt.Errorf("lifecycle script failed: %w\nOutput: %s", err, output)
    }
    
    return nil
}
```

### User Context Execution
```go
// ExecAsUser executes a command as a specific user
func (c *DockerClient) ExecAsUser(ctx context.Context, containerID string, user string, command []string) (string, error) {
    execConfig := container.ExecOptions{
        User:         user, // Key addition: specify user for exec
        Cmd:          command,
        AttachStdout: true,
        AttachStderr: true,
        Tty:          false,
    }
    
    // ... rest of exec implementation
}
```

### Interactive Shell with User Switching
```go
// AttachInteractive attaches an interactive terminal as the specified user
func (m *Manager) AttachInteractive(ctx context.Context, containerID string) error {
    // Determine target user
    targetUser := "root" // default
    if m.devContainer != nil {
        if m.devContainer.RemoteUser != nil && *m.devContainer.RemoteUser != "" {
            targetUser = *m.devContainer.RemoteUser
        } else if m.devContainer.ContainerUser != nil && *m.devContainer.ContainerUser != "" {
            targetUser = *m.devContainer.ContainerUser
        }
    }
    
    // Run postAttachCommand if specified
    if m.devContainer != nil {
        if err := m.ExecuteLifecycleCommands(ctx, containerID, m.devContainer, "attach"); err != nil {
            return fmt.Errorf("failed to run postAttachCommand: %w", err)
        }
    }
    
    // Create exec configuration for interactive shell
    attachment := &InteractiveExecAttachment{
        client:      m.dockerClient.client,
        containerID: containerID,
        user:        targetUser,
    }
    
    return attachment.Start(ctx)
}
```

### Interactive Exec Implementation
```go
type InteractiveExecAttachment struct {
    client      *client.Client
    containerID string
    user        string
    execID      string
    oldState    *term.State
}

func (a *InteractiveExecAttachment) Start(ctx context.Context) error {
    // Check terminal
    if !term.IsTerminal(os.Stdin.Fd()) {
        return fmt.Errorf("not running in a terminal")
    }
    
    // Create exec with user context
    execConfig := container.ExecOptions{
        User:         a.user,
        Cmd:          []string{"/bin/sh", "-l"}, // login shell
        Tty:          true,
        AttachStdin:  true,
        AttachStdout: true,
        AttachStderr: true,
    }
    
    execResp, err := a.client.ContainerExecCreate(ctx, a.containerID, execConfig)
    if err != nil {
        return fmt.Errorf("failed to create exec: %w", err)
    }
    a.execID = execResp.ID
    
    // Set terminal to raw mode
    oldState, err := term.MakeRaw(os.Stdin.Fd())
    if err != nil {
        return fmt.Errorf("failed to set terminal to raw mode: %w", err)
    }
    a.oldState = oldState
    defer a.Cleanup()
    
    // Start and attach to exec
    resp, err := a.client.ContainerExecAttach(ctx, a.execID, container.ExecAttachOptions{
        Tty: true,
    })
    if err != nil {
        return fmt.Errorf("failed to attach to exec: %w", err)
    }
    defer resp.Close()
    
    // Handle resize
    resizeCtx, cancelResize := context.WithCancel(ctx)
    defer cancelResize()
    go a.HandleResize(resizeCtx)
    
    // Stream I/O
    errCh := make(chan error, 2)
    
    go func() {
        _, err := io.Copy(resp.Conn, os.Stdin)
        errCh <- err
    }()
    
    go func() {
        _, err := io.Copy(os.Stdout, resp.Reader)
        errCh <- err
    }()
    
    // Wait for exit
    select {
    case err := <-errCh:
        if err != nil && err != io.EOF {
            return fmt.Errorf("I/O error: %w", err)
        }
    case <-ctx.Done():
        return ctx.Err()
    }
    
    return nil
}
```

## Features Installation

### Feature Installation Implementation
```go
func (m *Manager) InstallFeatures(ctx context.Context, containerID string, dc *DevContainer) error {
    if dc.Features == nil || len(dc.Features.Features) == 0 {
        return nil
    }
    
    // Features are typically installed using the devcontainer CLI or similar tools
    // For now, we'll implement basic feature support
    
    for featureID, featureConfig := range dc.Features.Features {
        if err := m.installFeature(ctx, containerID, featureID, featureConfig); err != nil {
            return fmt.Errorf("failed to install feature %s: %w", featureID, err)
        }
    }
    
    return nil
}

func (m *Manager) installFeature(ctx context.Context, containerID string, featureID string, config interface{}) error {
    // Parse feature ID (e.g., "ghcr.io/devcontainers/features/go:1")
    // Download and execute feature installation script
    // This is a simplified implementation - real features require more complex handling
    
    switch featureID {
    case "ghcr.io/devcontainers/features/common-utils:2":
        // Install common utilities
        script := `
            apt-get update
            apt-get install -y git curl wget sudo
            # Create non-root user if specified in config
        `
        _, err := m.docker.ExecInContainer(ctx, containerID, []string{"sh", "-c", script})
        return err
        
    // Add more feature implementations as needed
    default:
        // Log warning about unsupported feature
        return nil
    }
}
```

## Test Cases

### 1. Lifecycle Command Execution Tests

```go
func TestLifecycleCommandExecution(t *testing.T) {
    tests := []struct {
        name      string
        dc        *DevContainer
        phase     string
        checkFunc func(t *testing.T, containerID string, mgr *Manager)
    }{
        {
            name: "onCreate commands execute in order",
            dc: &DevContainer{
                DevContainerCommon: DevContainerCommon{
                    OnCreateCommand:      "touch /tmp/onCreate.txt",
                    UpdateContentCommand: "echo 'updated' > /tmp/update.txt",
                    PostCreateCommand:    "touch /tmp/postCreate.txt",
                },
            },
            phase: "create",
            checkFunc: func(t *testing.T, containerID string, mgr *Manager) {
                // Verify all files were created
                output, _ := mgr.Exec(context.Background(), containerID, []string{"ls", "/tmp"})
                assert.Contains(t, output, "onCreate.txt")
                assert.Contains(t, output, "update.txt")
                assert.Contains(t, output, "postCreate.txt")
            },
        },
        {
            name: "postStartCommand executes on start",
            dc: &DevContainer{
                DevContainerCommon: DevContainerCommon{
                    PostStartCommand: "echo 'started' > /tmp/started.txt",
                },
            },
            phase: "start",
            checkFunc: func(t *testing.T, containerID string, mgr *Manager) {
                output, _ := mgr.Exec(context.Background(), containerID, []string{"cat", "/tmp/started.txt"})
                assert.Equal(t, "started\n", output)
            },
        },
        {
            name: "complex command objects execute correctly",
            dc: &DevContainer{
                DevContainerCommon: DevContainerCommon{
                    PostCreateCommand: map[string]interface{}{
                        "install": "npm install",
                        "build":   []interface{}{"npm", "run", "build"},
                    },
                },
            },
            phase: "create",
            checkFunc: func(t *testing.T, containerID string, mgr *Manager) {
                // Verify commands executed
                // This would need appropriate test setup with npm
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup manager and container
            mgr, _ := NewManager()
            mgr.SetDevContainer(tt.dc)
            
            containerID, err := mgr.Create(context.Background(), "/test/path")
            require.NoError(t, err)
            defer mgr.Remove(context.Background(), containerID)
            
            // Execute lifecycle commands
            err = mgr.ExecuteLifecycleCommands(context.Background(), containerID, tt.dc, tt.phase)
            require.NoError(t, err)
            
            // Run test checks
            tt.checkFunc(t, containerID, mgr)
        })
    }
}
```

### 2. User Context Switching Tests

```go
func TestUserContextSwitching(t *testing.T) {
    tests := []struct {
        name         string
        dc           *DevContainer
        expectedUser string
    }{
        {
            name: "switches to remoteUser",
            dc: &DevContainer{
                DevContainerCommon: DevContainerCommon{
                    RemoteUser: stringPtr("vscode"),
                },
            },
            expectedUser: "vscode",
        },
        {
            name: "falls back to containerUser",
            dc: &DevContainer{
                DevContainerCommon: DevContainerCommon{
                    ContainerUser: stringPtr("developer"),
                },
            },
            expectedUser: "developer",
        },
        {
            name:         "defaults to root when no user specified",
            dc:           &DevContainer{},
            expectedUser: "root",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mgr, _ := NewManager()
            mgr.SetDevContainer(tt.dc)
            
            // Create and start container
            containerID, _ := mgr.Create(context.Background(), "/test")
            mgr.Start(context.Background(), containerID)
            defer mgr.Remove(context.Background(), containerID)
            
            // Execute command as the expected user
            output, err := mgr.docker.ExecAsUser(
                context.Background(),
                containerID,
                tt.expectedUser,
                []string{"whoami"},
            )
            
            require.NoError(t, err)
            assert.Equal(t, tt.expectedUser+"\n", output)
        })
    }
}
```

### 3. Interactive Attachment Tests

```go
func TestInteractiveAttachment(t *testing.T) {
    t.Run("exec session runs as specified user", func(t *testing.T) {
        dc := &DevContainer{
            DevContainerCommon: DevContainerCommon{
                RemoteUser: stringPtr("vscode"),
            },
        }
        
        mgr, _ := NewManager()
        mgr.SetDevContainer(dc)
        
        containerID, _ := mgr.Create(context.Background(), "/test")
        mgr.Start(context.Background(), containerID)
        defer mgr.Remove(context.Background(), containerID)
        
        // Create mock exec attachment
        attachment := &InteractiveExecAttachment{
            client:      mgr.dockerClient.client,
            containerID: containerID,
            user:        "vscode",
        }
        
        // Verify exec is created with correct user
        execConfig := container.ExecOptions{
            User: "vscode",
            Cmd:  []string{"whoami"},
            AttachStdout: true,
        }
        
        execResp, err := attachment.client.ContainerExecCreate(
            context.Background(),
            containerID,
            execConfig,
        )
        require.NoError(t, err)
        
        // Verify exec runs as vscode user
        resp, _ := attachment.client.ContainerExecAttach(
            context.Background(),
            execResp.ID,
            container.ExecAttachOptions{},
        )
        defer resp.Close()
        
        output, _ := io.ReadAll(resp.Reader)
        assert.Contains(t, string(output), "vscode")
    })
}
```

### 4. Feature Installation Tests

```go
func TestFeatureInstallation(t *testing.T) {
    t.Run("installs common-utils feature", func(t *testing.T) {
        dc := &DevContainer{
            Features: &DevContainerCommonFeatures{
                Features: map[string]interface{}{
                    "ghcr.io/devcontainers/features/common-utils:2": map[string]interface{}{
                        "installZsh": true,
                        "username":   "vscode",
                    },
                },
            },
        }
        
        mgr, _ := NewManager()
        containerID, _ := mgr.Create(context.Background(), "/test")
        defer mgr.Remove(context.Background(), containerID)
        
        err := mgr.InstallFeatures(context.Background(), containerID, dc)
        require.NoError(t, err)
        
        // Verify feature was installed
        output, _ := mgr.Exec(context.Background(), containerID, []string{"which", "git"})
        assert.NotEmpty(t, output)
    })
}
```

### 5. Error Handling Tests

```go
func TestLifecycleErrorHandling(t *testing.T) {
    t.Run("handles failing lifecycle command", func(t *testing.T) {
        dc := &DevContainer{
            DevContainerCommon: DevContainerCommon{
                PostCreateCommand: "exit 1",
            },
        }
        
        mgr, _ := NewManager()
        mgr.SetDevContainer(dc)
        
        containerID, _ := mgr.Create(context.Background(), "/test")
        defer mgr.Remove(context.Background(), containerID)
        
        err := mgr.ExecuteLifecycleCommands(context.Background(), containerID, dc, "create")
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "lifecycle script failed")
    })
    
    t.Run("continues on optional command failure", func(t *testing.T) {
        dc := &DevContainer{
            DevContainerCommon: DevContainerCommon{
                PostAttachCommand: "exit 1", // postAttach is typically optional
            },
        }
        
        mgr, _ := NewManager()
        mgr.SetDevContainer(dc)
        
        // Should log warning but not fail attachment
        // Implementation would need to handle optional vs required commands
    })
}
```

### 6. Security Tests

```go
func TestSecurityIsolation(t *testing.T) {
    t.Run("user cannot escalate to root", func(t *testing.T) {
        dc := &DevContainer{
            DevContainerCommon: DevContainerCommon{
                RemoteUser: stringPtr("vscode"),
            },
        }
        
        mgr, _ := NewManager()
        mgr.SetDevContainer(dc)
        
        containerID, _ := mgr.Create(context.Background(), "/test")
        mgr.Start(context.Background(), containerID)
        defer mgr.Remove(context.Background(), containerID)
        
        // Try to execute sudo without password
        output, err := mgr.docker.ExecAsUser(
            context.Background(),
            containerID,
            "vscode",
            []string{"sudo", "whoami"},
        )
        
        // Should fail or prompt for password (depending on sudo config)
        assert.Error(t, err)
    })
}
```

## Migration Path

### Phase 1: Lifecycle Command Support
1. Implement `ExecuteLifecycleCommands` method
2. Update `Create` to run onCreate commands
3. Update `Start` to run postStartCommand
4. Add tests for lifecycle execution

### Phase 2: User Context Support
1. Implement `ExecAsUser` in DockerClient
2. Create `InteractiveExecAttachment` to replace current attachment
3. Update `AttachInteractive` to use exec with user context
4. Add user switching tests

### Phase 3: Feature Support
1. Implement basic feature installation
2. Add support for common features
3. Add feature installation tests

### Phase 4: Complete Integration
1. Update shai to use new lifecycle-aware creation
2. Ensure all tests pass
3. Documentation updates

## Security Considerations

1. **Root Access**: Container starts as root only for setup, then switches to specified user
2. **No Escalation**: Using exec with specific user prevents returning to root context
3. **Command Injection**: All lifecycle commands must be properly escaped
4. **Feature Security**: Feature installation scripts run as root but should create proper user contexts

## Performance Considerations

1. **Caching**: Container creation with features can be slow; consider layer caching
2. **Parallel Execution**: Some lifecycle commands could run in parallel
3. **Script Optimization**: Combine multiple commands into single exec when possible

## Backward Compatibility

1. Containers without lifecycle commands continue to work
2. Default behavior (no user specified) remains as root
3. Existing mount configurations are preserved