package shai

import (
    "bytes"
    "bufio"
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/divisive-ai/vibethis/server/container/internal/devcontainer"
    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/image"
    "github.com/docker/docker/api/types/mount"
    "github.com/docker/docker/client"
    "github.com/moby/term"
    "os/signal"
    "syscall"
)

// ProgressDisplay manages clean progress output with spinners and checkmarks
type ProgressDisplay struct {
	current string
	spinner *time.Ticker
	done    chan bool
	spinnerChars []string
	spinnerIdx   int
}

// NewProgressDisplay creates a new progress display
func NewProgressDisplay() *ProgressDisplay {
	return &ProgressDisplay{
		spinnerChars: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		done:         make(chan bool),
	}
}

// Start begins showing a spinner for the given message
func (p *ProgressDisplay) Start(message string) {
	p.Stop() // Stop any existing progress
	p.current = message
	fmt.Printf("%s %s", p.spinnerChars[0], message)
	
	p.spinner = time.NewTicker(100 * time.Millisecond)
	go func() {
		for {
			select {
			case <-p.spinner.C:
				p.spinnerIdx = (p.spinnerIdx + 1) % len(p.spinnerChars)
				fmt.Printf("\r%s %s", p.spinnerChars[p.spinnerIdx], p.current)
			case <-p.done:
				return
			}
		}
	}()
}

// Success completes the current progress with a checkmark
func (p *ProgressDisplay) Success(message string) {
	p.Stop()
	if message == "" {
		message = p.current
	}
	fmt.Printf("\r✅ %s\n", message)
	p.current = ""
}

// Error completes the current progress with an error
func (p *ProgressDisplay) Error(message string, err error) {
	p.Stop()
	if message == "" {
		message = p.current
	}
	fmt.Printf("\r❌ %s: %v\n", message, err)
	p.current = ""
}

// Stop stops the current spinner
func (p *ProgressDisplay) Stop() {
	if p.spinner != nil {
		p.spinner.Stop()
		p.done <- true
		p.spinner = nil
	}
}

// EphemeralConfig represents configuration for ephemeral container execution
type EphemeralConfig struct {
    WorkingDir          string   // Directory containing .devcontainer
    ReadWritePaths      []string // Paths to mount as read-write
    NoCache             bool     // Force rebuild
    HideProgressMarkers bool     // Hide progress markers from output
    DebugScript         bool     // Print generated setup script
}

// EphemeralProgressCallback is a callback for ephemeral progress updates
type EphemeralProgressCallback func(update ProgressUpdate)

// EphemeralRunner runs ephemeral devcontainers with automatic cleanup
type EphemeralRunner struct {
    config           EphemeralConfig
    devContainer     *devcontainer.DevContainer
    docker           *client.Client
    mountBuilder     *MountBuilder
    progressCallback EphemeralProgressCallback
    progress         *ProgressDisplay
    // resolvedFeatures are features fetched to local temp dirs for mounting
    resolvedFeatures []ResolvedFeature
    debugScript      bool
}

// NewEphemeralRunner creates a new ephemeral runner
func NewEphemeralRunner(config EphemeralConfig) (*EphemeralRunner, error) {
	// Use current directory if not specified
	if config.WorkingDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		config.WorkingDir = wd
	}

	// Load devcontainer configuration
	devPath := filepath.Join(config.WorkingDir, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(devPath); err != nil {
		// Try alternate location
		devPath = filepath.Join(config.WorkingDir, ".devcontainer.json")
		if _, err := os.Stat(devPath); err != nil {
			return nil, fmt.Errorf("no devcontainer.json found in %s", config.WorkingDir)
		}
	}

	dc, err := devcontainer.LoadDevContainer(devPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load devcontainer: %w", err)
	}

	// Create Docker client using our custom connection logic
	dockerClientWrapper, err := devcontainer.NewDockerClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	
	// Extract the underlying Docker client for direct use
	dockerClient := dockerClientWrapper.GetClient()

	// Create mount builder
	mountBuilder, err := NewMountBuilder(config.WorkingDir, config.ReadWritePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to create mount builder: %w", err)
	}

    return &EphemeralRunner{
        config:       config,
        devContainer: dc,
        docker:       dockerClient,
        mountBuilder: mountBuilder,
        progress:     NewProgressDisplay(),
        debugScript:  config.DebugScript,
    }, nil
}

// OnProgress sets the progress callback
func (r *EphemeralRunner) OnProgress(cb EphemeralProgressCallback) {
	r.progressCallback = cb
}

// Run creates and runs an ephemeral devcontainer
func (r *EphemeralRunner) Run(ctx context.Context) error {
    // If features were pre-resolved (e.g., in tests), validate them immediately
    if len(r.resolvedFeatures) > 0 {
        if err := validateFeatures(r.resolvedFeatures); err != nil {
            return err
        }
    }

    // Resolve devcontainer features (download to temp dirs for mounting)
    if r.devContainer != nil && r.devContainer.Features != nil && r.devContainer.Features.AdditionalProperties != nil {
        feats, err := ResolveOCIFeatures(r.devContainer.Features.AdditionalProperties)
        if err != nil {
            return fmt.Errorf("failed to resolve features: %w", err)
        }
        // Order features using installsAfter
        ordered, err := orderResolvedFeatures(feats)
        if err != nil { return fmt.Errorf("failed to order features: %w", err) }
        r.resolvedFeatures = ordered
        // Validate unsupported flags
        if err := validateFeatures(r.resolvedFeatures); err != nil {
            return err
        }
    }

    // Build docker configuration
    config, err := r.buildConfig()
    if err != nil {
        return fmt.Errorf("failed to build config: %w", err)
    }

    // Generate setup script
    setupScript := r.generateSetupScript()
    if r.debugScript {
        fmt.Println("--- devcontainer setup script ---")
        fmt.Println(setupScript)
        fmt.Println("--- end script ---")
    }

	// Run ephemeral container
	return r.runEphemeralContainer(ctx, config, setupScript)
}

// orderResolvedFeatures performs a stable topological sort using InstallsAfter within the provided set.
func orderResolvedFeatures(feats []ResolvedFeature) ([]ResolvedFeature, error) {
    // Map id->index for quick lookup
    index := map[string]int{}
    for i, f := range feats { index[f.ID] = i }
    // Build graph among known features only
    adj := make([][]int, len(feats))
    indeg := make([]int, len(feats))
    for i, f := range feats {
        for _, dep := range f.InstallsAfter {
            if j, ok := index[dep]; ok {
                adj[j] = append(adj[j], i)
                indeg[i]++
            }
        }
    }
    // Kahn's algorithm
    var q []int
    for i := range feats { if indeg[i]==0 { q = append(q, i) } }
    var out []ResolvedFeature
    for len(q)>0 {
        n := q[0]; q = q[1:]
        out = append(out, feats[n])
        for _, v := range adj[n] { indeg[v]--; if indeg[v]==0 { q = append(q, v) } }
    }
    if len(out) != len(feats) { return nil, fmt.Errorf("feature dependency cycle detected") }
    return out, nil
}

// validateFeatures returns error if any feature declares unsupported fields
func validateFeatures(feats []ResolvedFeature) error {
    var errs []string
    for _, f := range feats {
        if len(f.Unsupported) > 0 {
            errs = append(errs, fmt.Sprintf("%s: %v", f.ID, f.Unsupported))
        }
    }
    if len(errs) > 0 {
        return fmt.Errorf("unsupported feature fields: %s", strings.Join(errs, "; "))
    }
    return nil
}

// buildConfig builds Docker configuration from devcontainer
func (r *EphemeralRunner) buildConfig() (*devcontainer.DockerRunConfig, error) {
    config, err := devcontainer.BuildDockerRunCommand(r.devContainer, r.config.WorkingDir)
    if err != nil {
        return nil, err
    }

	// Apply custom mounts
	dockerMounts := r.mountBuilder.BuildMounts()
	for _, m := range dockerMounts {
		mountStr := fmt.Sprintf("type=%s,source=%s,target=%s", m.Type, m.Source, m.Target)
		if m.ReadOnly {
			mountStr += ",readonly"
		}
		config.Mounts = append(config.Mounts, mountStr)
	}

    return config, nil
}

// generateSetupScript generates the setup script with progress markers
func (r *EphemeralRunner) generateSetupScript() string {
	targetUser := "root" // default
	if r.devContainer.RemoteUser != nil && *r.devContainer.RemoteUser != "" {
		targetUser = *r.devContainer.RemoteUser
	} else if r.devContainer.ContainerUser != nil && *r.devContainer.ContainerUser != "" {
		targetUser = *r.devContainer.ContainerUser
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

# Replace this process with user shell (login). Prefer sudo to avoid job-control warnings.
if command -v sudo >/dev/null 2>&1; then
  exec sudo -iu %USER% /bin/bash -l
else
  exec su - %USER% -c 'exec /bin/bash --login'
fi
`

	// Replace placeholders with actual commands
    script = strings.ReplaceAll(script, "%FEATURES%", r.generateFeatureInstallsWithProgress())
	script = strings.ReplaceAll(script, "%ONCREATE%", r.generateCommandWithProgress(r.devContainer.OnCreateCommand, "ONCREATE"))
	script = strings.ReplaceAll(script, "%UPDATECONTENT%", r.generateCommandWithProgress(r.devContainer.UpdateContentCommand, "UPDATECONTENT"))
	script = strings.ReplaceAll(script, "%POSTCREATE%", r.generateCommandWithProgress(r.devContainer.PostCreateCommand, "POSTCREATE"))
	script = strings.ReplaceAll(script, "%POSTSTART%", r.generateCommandWithProgress(r.devContainer.PostStartCommand, "POSTSTART"))
	script = strings.ReplaceAll(script, "%POSTATTACH%", r.generateCommandWithProgress(r.devContainer.PostAttachCommand, "POSTATTACH"))
	script = strings.ReplaceAll(script, "%USER%", targetUser)

	return script
}

// generateCommandWithProgress wraps a command with progress markers
func (r *EphemeralRunner) generateCommandWithProgress(cmd interface{}, phase string) string {
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

// parseLifecycleCommand converts a lifecycle command to shell command
func (r *EphemeralRunner) parseLifecycleCommand(cmd interface{}) string {
	if cmd == nil {
		return ""
	}

	lifecycleCmd, err := devcontainer.ParseLifecycleCommand(cmd)
	if err != nil || lifecycleCmd == nil {
		return ""
	}

	return lifecycleCmd.ToShellCommand()
}

// generateFeatureInstallsWithProgress generates feature installation commands
// It assumes features have been resolved and mounted under /tmp/devcontainer-features/<name>
func (r *EphemeralRunner) generateFeatureInstallsWithProgress() string {
    if len(r.resolvedFeatures) == 0 {
        return ""
    }

    var script strings.Builder
    script.WriteString("echo \"::DEVCONTAINER::FEATURES::START::Installing devcontainer features\"\n")
    // Provide common environment expected by many features
    script.WriteString("export DEBIAN_FRONTEND=noninteractive\n")
    script.WriteString("export PATH=\"$PATH:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"\n")
    script.WriteString("export LANG=C.UTF-8\n")
    script.WriteString("export LC_ALL=C.UTF-8\n")
    script.WriteString("export SHELL=/bin/bash\n")
    // Provide user env variables per spec
    containerUser := "root"
    containerHome := "/root"
    remoteUser := containerUser
    remoteHome := containerHome
    if r.devContainer != nil {
        if r.devContainer.RemoteUser != nil && *r.devContainer.RemoteUser != "" {
            remoteUser = *r.devContainer.RemoteUser
            if remoteUser == "root" { remoteHome = "/root" } else { remoteHome = "/home/" + remoteUser }
        } else if r.devContainer.ContainerUser != nil && *r.devContainer.ContainerUser != "" {
            remoteUser = *r.devContainer.ContainerUser
            if remoteUser == "root" { remoteHome = "/root" } else { remoteHome = "/home/" + remoteUser }
        }
    }
    script.WriteString("export _CONTAINER_USER='" + containerUser + "'\n")
    script.WriteString("export _CONTAINER_USER_HOME='" + containerHome + "'\n")
    script.WriteString("export _REMOTE_USER='" + remoteUser + "'\n")
    script.WriteString("export _REMOTE_USER_HOME='" + remoteHome + "'\n")
    // Derive target arch/platform in-container for portability
    script.WriteString("ARCH=$(uname -m)\n")
    script.WriteString("case \"$ARCH\" in\n")
    script.WriteString("  x86_64) TARGETARCH=amd64 ;;\n")
    script.WriteString("  aarch64|arm64) TARGETARCH=arm64 ;;\n")
    script.WriteString("  armv7l) TARGETARCH=armv7 ;;\n")
    script.WriteString("  *) TARGETARCH=$ARCH ;;\n")
    script.WriteString("esac\n")
    script.WriteString("export TARGETOS=linux\n")
    script.WriteString("export TARGETARCH\n")
    script.WriteString("export TARGETPLATFORM=\"linux/$TARGETARCH\"\n")

    for _, feat := range r.resolvedFeatures {
        mountPath := "/tmp/devcontainer-features/" + feat.SafeName
        script.WriteString(fmt.Sprintf("echo \"::DEVCONTAINER::FEATURES::PROGRESS::Installing feature %s\"\n", feat.ID))
        // Export option env vars (explicit values from devcontainer.json)
        opts := feat.OptionsEnv()
        // If no USERNAME provided by feature options, prefer devcontainer RemoteUser, otherwise 'automatic'
        if _, ok := opts["USERNAME"]; !ok {
            if r.devContainer != nil && r.devContainer.RemoteUser != nil && *r.devContainer.RemoteUser != "" {
                opts["USERNAME"] = *r.devContainer.RemoteUser
            } else {
                opts["USERNAME"] = "automatic"
            }
        }
        // Apply feature option defaults from the feature spec (devcontainer-feature.json)
        for k, v := range feat.Defaults {
            if _, exists := opts[k]; !exists {
                opts[k] = v
            }
        }
        // If the feature has a "VERSION" option (by default or provided), also export <FEATURE>_VERSION alias
        // Determine feature short name from ID (last path segment before colon)
        featureID := feat.ID
        short := featureID
        if idx := strings.LastIndex(short, "/"); idx >= 0 { short = short[idx+1:] }
        if idx := strings.Index(short, ":"); idx >= 0 { short = short[:idx] }
        upperShort := strings.ToUpper(strings.ReplaceAll(short, "-", "_"))
        // Also export SHORT_VERSION alias (e.g., GIT_VERSION)
        aliasKey := upperShort + "_VERSION"
        if _, ok := opts[aliasKey]; !ok {
            if v, ok := opts["VERSION"]; ok {
                opts[aliasKey] = v
            }
        }
        for k, v := range opts {
            script.WriteString(fmt.Sprintf("export %s=\"%s\"\n", k, escapeShell(v)))
        }
        // Execute install.sh if present. Prefer bash (many features use bashisms), fallback to sh.
        script.WriteString("if [ -f '" + mountPath + "/install.sh' ]; then\n")
        script.WriteString("  if command -v bash >/dev/null 2>&1; then\n")
        script.WriteString("    bash '" + mountPath + "/install.sh'\n")
        script.WriteString("  else\n")
        script.WriteString("    sh '" + mountPath + "/install.sh'\n")
        script.WriteString("  fi\n")
        script.WriteString("else\n")
        script.WriteString("  echo 'Feature install script not found: " + mountPath + "/install.sh'\n")
        script.WriteString("fi\n")
    }

    script.WriteString("echo \"::DEVCONTAINER::FEATURES::COMPLETE::All features installed\"\n")
    return script.String()
}

// escapeShell performs minimal escaping for double-quoted shell strings
func escapeShell(s string) string {
    s = strings.ReplaceAll(s, "\\", "\\\\")
    s = strings.ReplaceAll(s, "\"", "\\\"")
    s = strings.ReplaceAll(s, "`", "\\`")
    return s
}

// runEphemeralContainer runs the ephemeral container with setup script
func (r *EphemeralRunner) runEphemeralContainer(ctx context.Context, config *devcontainer.DockerRunConfig, setupScript string) error {
	// Create a temporary script file
	scriptFile, err := os.CreateTemp("", "devcontainer-setup-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create setup script: %w", err)
	}
	defer os.Remove(scriptFile.Name())

	if _, err := scriptFile.WriteString(setupScript); err != nil {
		return fmt.Errorf("failed to write setup script: %w", err)
	}
	if err := scriptFile.Chmod(0755); err != nil {
		return fmt.Errorf("failed to chmod setup script: %w", err)
	}
	scriptFile.Close()

	// Build container configuration
	containerConfig := &container.Config{
		Image:        config.Image,
		Entrypoint:   []string{"/devcontainer-setup.sh"},
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		OpenStdin:    true,
		WorkingDir:   config.WorkspaceFolder,
		Env:          r.buildEnvVars(config),
	}

	// Build mounts and validate them
	mounts := r.buildMounts(config, scriptFile.Name())
	if err := r.validateMounts(mounts); err != nil {
		return fmt.Errorf("invalid mount configuration: %w", err)
	}
	
	// Build host configuration
	hostConfig := &container.HostConfig{
		AutoRemove: true, // --rm equivalent
		Mounts:     mounts,
	}

	// Add capabilities
	if len(config.CapAdd) > 0 {
		hostConfig.CapAdd = config.CapAdd
	}

	// Add security options
	if len(config.SecurityOpt) > 0 {
		hostConfig.SecurityOpt = config.SecurityOpt
	}

	// Add init
	if config.Init {
		hostConfig.Init = &config.Init
	}

	// Add privileged
	if config.Privileged {
		hostConfig.Privileged = config.Privileged
	}

    // Ensure image is available locally (suppress internal spinner)
    if err := r.ensureImageNoSpinner(ctx, config.Image); err != nil {
        return fmt.Errorf("failed to ensure image %s: %w", config.Image, err)
    }

    // Create container (suppress internal spinner; CLI handles progress)
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

    // Local stdin file descriptor for TTY detection and resize (raw mode enabled at user switch)
    fd := os.Stdin.Fd()

    // Start container (no internal spinner)
    if err := r.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
        return fmt.Errorf("failed to start container %s: %w", resp.ID, err)
    }
    // Resize container TTY to match local terminal size and keep it updated
    if term.IsTerminal(fd) {
        // initial resize
        if ws, err := term.GetWinsize(fd); err == nil && ws != nil {
            _ = r.docker.ContainerResize(context.Background(), resp.ID, container.ResizeOptions{Height: uint(ws.Height), Width: uint(ws.Width)})
        }
        // watch for SIGWINCH
        go func() {
            sigCh := make(chan os.Signal, 1)
            signal.Notify(sigCh, syscall.SIGWINCH)
            defer signal.Stop(sigCh)
            for {
                select {
                case <-ctx.Done():
                    return
                case <-sigCh:
                    if ws, err := term.GetWinsize(fd); err == nil && ws != nil {
                        _ = r.docker.ContainerResize(context.Background(), resp.ID, container.ResizeOptions{Height: uint(ws.Height), Width: uint(ws.Width)})
                    }
                }
            }
        }()
    }

    // Do not modify terminal mode; let the CLI/terminal handle TTY behavior

	// Handle I/O with progress monitoring
	errCh := make(chan error, 2)
	progressCh := make(chan ProgressUpdate, 100)
	outputCh := make(chan string, 1) // Channel to send buffered output

	// Goroutine to handle stdin
	go func() {
		_, err := io.Copy(hijacked.Conn, os.Stdin)
		errCh <- err
	}()

    // Goroutine to handle stdout/stderr with progress parsing
    go func() {
        reader := bufio.NewReader(hijacked.Reader)
        var outputBuffer strings.Builder
        passThrough := false // after USERSWITCH we pass all output through

        for {
            line, err := reader.ReadString('\n')
            if err != nil {
                if err != io.EOF {
                    errCh <- err
                } else {
                    // Send the buffer when done reading
                    outputCh <- outputBuffer.String()
                }
                break
            }

            // Buffer all output (for potential error display)
            outputBuffer.WriteString(line)

            if passThrough {
                // After user switch, show all output to the user
                fmt.Print(line)
                continue
            }

            // Check for progress markers
            if strings.HasPrefix(line, "::DEVCONTAINER::") {
                if progress := parseProgressMarker(line); progress != nil {
                    // If switching to user, enable pass-through and stream the rest byte-wise
                    if strings.EqualFold(progress.Phase, "USERSWITCH") && strings.EqualFold(progress.Status, "START") {
                        // Move to a fresh line to ensure left-aligned prompt
                        fmt.Print("\n")
                        // Enable raw mode now so interactive keys work for the user shell
                        var saved *term.State
                        if term.IsTerminal(fd) {
                            if st, err := term.MakeRaw(fd); err == nil {
                                saved = st
                                defer term.RestoreTerminal(fd, saved)
                            }
                        }
                        // Pass-through streaming with filtering of noisy logout lines
                        var tail bytes.Buffer
                        buf := make([]byte, 2048)
                        for {
                            n, err := reader.Read(buf)
                            if n > 0 {
                                data := buf[:n]
                                // Append to tail and process full lines
                                tail.Write(data)
                                b := tail.Bytes()
                                // Find last newline
                                idx := bytes.LastIndexByte(b, '\n')
                                if idx >= 0 {
                                    lines := bytes.Split(b[:idx], []byte{'\n'})
                                    for _, ln := range lines {
                                        s := string(ln)
                                        if strings.Contains(s, "Session terminated, killing shell") || strings.Contains(s, "...killed.") {
                                            continue
                                        }
                                        fmt.Println(s)
                                    }
                                    // Remainder (possibly prompt without newline)
                                    tail.Reset()
                                    tail.Write(b[idx+1:])
                                }
                                // Print partial (e.g., prompt) without newline immediately
                                if tail.Len() > 0 {
                                    os.Stdout.Write(tail.Bytes())
                                    tail.Reset()
                                }
                            }
                            if err != nil {
                                if err != io.EOF {
                                    errCh <- err
                                }
                                break
                            }
                        }
                        // Done with this goroutine
                        outputCh <- outputBuffer.String()
                        errCh <- nil
                        return
                    }
                    select {
                    case progressCh <- *progress:
                    default: // Don't block if channel is full
                    }
                }
                // Hide progress markers from direct output
                continue
            }

            // Prior to pass-through, suppress non-marker output to keep logs clean
            // (We still buffer it for error reporting.)
        }
        // Always send buffer at the end
        outputCh <- outputBuffer.String()
        errCh <- nil
    }()

    // Goroutine to handle progress updates
    go func() {
        for progress := range progressCh {
            // Stop any internal spinner as soon as we see progress (CLI handles display)
            r.progress.Stop()
            r.handleProgress(progress)
        }
    }()

    // Wait for container to exit (CLI progress will reflect phases)
	statusCh, errWaitCh := r.docker.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)

	var containerOutput string
	
	select {
	case err := <-errCh:
		if err != nil && err != io.EOF {
			// Try to get output for debugging
			select {
			case output := <-outputCh:
				containerOutput = output
			default:
				// No output available
			}
            // No internal progress output here; CLI will display error
			if containerOutput != "" {
				fmt.Printf("\n--- Container Output (due to I/O error) ---\n")
				fmt.Print(containerOutput)
				fmt.Printf("--- End Container Output ---\n")
			}
			return fmt.Errorf("I/O error: %w", err)
		}
	case err := <-errWaitCh:
            if err != nil {
                return fmt.Errorf("container wait error for %s: %w", resp.ID, err)
            }
	case status := <-statusCh:
		// Get the output buffer
		select {
		case containerOutput = <-outputCh:
		case <-time.After(1 * time.Second):
			// Timeout getting output
		}
		
		close(progressCh)
            if status.StatusCode != 0 {
                // Show the output for debugging the failure
                if containerOutput != "" {
                    fmt.Printf("\n--- Container Output (exit code %d) ---\n", status.StatusCode)
                    fmt.Print(containerOutput)
                    fmt.Printf("--- End Container Output ---\n")
                }
                return fmt.Errorf("container exited with code %d", status.StatusCode)
            }
            // Setup complete
	case <-ctx.Done():
        // No internal progress printing on cancel
        close(progressCh)
        return ctx.Err()
	}

	return nil
}

// ensureImage pulls the image if it doesn't exist locally
func (r *EphemeralRunner) ensureImage(ctx context.Context, imageName string) error {
    r.progress.Start("Checking image availability")
    defer r.progress.Stop()
    // Use no-spinner variant to avoid interleaving with CLI output
    return r.ensureImageNoSpinner(ctx, imageName)
}

// ensureImageNoSpinner is a non-UI helper to validate/pull images
func (r *EphemeralRunner) ensureImageNoSpinner(ctx context.Context, imageName string) error {
    // Check if image exists locally
    _, _, err := r.docker.ImageInspectWithRaw(ctx, imageName)
    if err == nil {
        return nil // Image exists locally
    }
    // Image doesn't exist locally, pull it
    pullOptions := image.PullOptions{}
    pullReader, err := r.docker.ImagePull(ctx, imageName, pullOptions)
    if err != nil {
        return fmt.Errorf("failed to pull image: %w", err)
    }
    defer pullReader.Close()
    // Read the pull output to ensure it completes but discard output
    _, err = io.Copy(io.Discard, pullReader)
    if err != nil {
        return fmt.Errorf("failed to complete image pull: %w", err)
    }
    return nil
}

// validateMounts checks that bind mount source directories exist
func (r *EphemeralRunner) validateMounts(mounts []mount.Mount) error {
	for _, m := range mounts {
		if m.Type == mount.TypeBind && m.Source != "" {
			if _, err := os.Stat(m.Source); err != nil {
				if os.IsNotExist(err) {
					// For directories that might need to be created (like cache dirs)
					if strings.Contains(m.Source, ".cache") {
						if err := os.MkdirAll(m.Source, 0755); err != nil {
							return fmt.Errorf("failed to create cache directory %s: %w", m.Source, err)
						}
						continue
					}
					return fmt.Errorf("bind mount source does not exist: %s", m.Source)
				}
				return fmt.Errorf("cannot access bind mount source %s: %w", m.Source, err)
			}
		}
	}
	return nil
}

// buildEnvVars builds environment variables for the container
func (r *EphemeralRunner) buildEnvVars(config *devcontainer.DockerRunConfig) []string {
    var env []string
    for k, v := range config.Environment {
        env = append(env, fmt.Sprintf("%s=%s", k, v))
    }
    // Propagate TERM if available for proper terminal behavior
    if termEnv := os.Getenv("TERM"); termEnv != "" {
        env = append(env, fmt.Sprintf("TERM=%s", termEnv))
    }
    return env
}

// buildMounts builds mount configurations for the container
func (r *EphemeralRunner) buildMounts(config *devcontainer.DockerRunConfig, scriptPath string) []mount.Mount {
    mountMap := make(map[string]mount.Mount)

	// Add setup script mount
	mountMap["/devcontainer-setup.sh"] = mount.Mount{
		Type:     mount.TypeBind,
		Source:   scriptPath,
		Target:   "/devcontainer-setup.sh",
		ReadOnly: true,
	}

	// Add workspace mount if not "none"
	if config.WorkspaceMount != "" && config.WorkspaceMount != "none" {
		// Parse workspace mount string
		if strings.HasPrefix(config.WorkspaceMount, "type=") {
			// Parse mount string format
			parts := strings.Split(config.WorkspaceMount, ",")
			m := mount.Mount{Type: mount.TypeBind}
			for _, part := range parts {
				kv := strings.SplitN(part, "=", 2)
				if len(kv) == 2 {
					switch kv[0] {
					case "type":
						m.Type = mount.Type(kv[1])
					case "source":
						m.Source = kv[1]
					case "target":
						m.Target = kv[1]
					}
				}
			}
			if m.Source != "" && m.Target != "" {
				mountMap[m.Target] = m
			}
		}
	}

	// Add custom mounts from mount builder (these override workspace mount for specific paths)
	for _, m := range r.mountBuilder.BuildMounts() {
		mountMap[m.Target] = m
	}

    // Add additional mounts from config (string format may list keys in any order)
    for _, mountStr := range config.Mounts {
        parts := strings.Split(mountStr, ",")
        m := mount.Mount{Type: mount.TypeBind}
        for _, part := range parts {
            p := strings.TrimSpace(part)
            if p == "readonly" || p == "ro" {
                m.ReadOnly = true
                continue
            }
            kv := strings.SplitN(p, "=", 2)
            if len(kv) == 2 {
                switch kv[0] {
                case "type":
                    m.Type = mount.Type(kv[1])
                case "source":
                    m.Source = kv[1]
                case "target":
                    m.Target = kv[1]
                }
            }
        }
        if m.Source != "" && m.Target != "" {
            if _, exists := mountMap[m.Target]; !exists {
                mountMap[m.Target] = m
            }
        }
    }

    // Mount resolved features into container at /tmp/devcontainer-features/<safe>
    if len(r.resolvedFeatures) > 0 {
        for _, feat := range r.resolvedFeatures {
            target := "/tmp/devcontainer-features/" + feat.SafeName
            mountMap[target] = mount.Mount{
                Type:   mount.TypeBind,
                Source: feat.Dir,
                Target: target,
            }
        }
    }

    // Convert map to slice
    var mounts []mount.Mount
    for _, m := range mountMap {
        mounts = append(mounts, m)
    }

	return mounts
}

// handleProgress handles progress updates
func (r *EphemeralRunner) handleProgress(progress ProgressUpdate) {
	if r.progressCallback != nil {
		r.progressCallback(progress)
	}
}

// parseProgressMarker parses a progress marker from output
func parseProgressMarker(line string) *ProgressUpdate {
	// Format: ::DEVCONTAINER::<phase>::<status>::<message>
	parts := strings.Split(strings.TrimPrefix(line, "::DEVCONTAINER::"), "::")
	if len(parts) < 3 {
		return nil
	}

	return &ProgressUpdate{
		Phase:   parts[0],
		Status:  parts[1],
		Message: strings.TrimSpace(strings.Join(parts[2:], "::")),
	}
}

// Close closes the Docker client connection
func (r *EphemeralRunner) Close() error {
    // Stop any active progress display
    if r.progress != nil {
        r.progress.Stop()
    }

    // Cleanup resolved feature directories
    for _, f := range r.resolvedFeatures {
        if f.Dir != "" {
            _ = os.RemoveAll(f.Dir)
        }
    }

    if r.docker != nil {
        return r.docker.Close()
    }
    return nil
}
