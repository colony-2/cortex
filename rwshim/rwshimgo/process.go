package rwshimgo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Process represents a monitored process
type Process struct {
	cmd        *exec.Cmd
	monitor    *Monitor
	shimPath   string
	mu         sync.Mutex
	started    bool
	finished   chan struct{}
}

// ProcessOption configures process execution
type ProcessOption func(*Process)

// WithShimPath sets a custom path to the intercept.so library
func WithShimPath(path string) ProcessOption {
	return func(p *Process) {
		p.shimPath = path
	}
}

// WithEnv adds environment variables to the process
func WithEnv(env []string) ProcessOption {
	return func(p *Process) {
		p.cmd.Env = append(p.cmd.Env, env...)
	}
}

// WithDir sets the working directory for the process
func WithDir(dir string) ProcessOption {
	return func(p *Process) {
		p.cmd.Dir = dir
	}
}

// WithStdin sets the process stdin
func WithStdin(stdin *os.File) ProcessOption {
	return func(p *Process) {
		p.cmd.Stdin = stdin
	}
}

// WithStdout sets the process stdout
func WithStdout(stdout *os.File) ProcessOption {
	return func(p *Process) {
		p.cmd.Stdout = stdout
	}
}

// WithStderr sets the process stderr
func WithStderr(stderr *os.File) ProcessOption {
	return func(p *Process) {
		p.cmd.Stderr = stderr
	}
}

// StartProcess starts a new process with the shim library preloaded
func (m *Monitor) StartProcess(ctx context.Context, command string, args []string, opts ...ProcessOption) (*Process, error) {
	// Ensure monitor is running
	if !m.IsRunning() {
		return nil, fmt.Errorf("monitor must be started before launching processes")
	}

	cmd := exec.CommandContext(ctx, command, args...)
	
	// Copy current environment
	cmd.Env = append([]string{}, os.Environ()...)

	// Create process object
	p := &Process{
		cmd:      cmd,
		monitor:  m,
		finished: make(chan struct{}),
	}

	// Apply options
	for _, opt := range opts {
		opt(p)
	}

	// Find shim library if not specified
	if p.shimPath == "" {
		shimPath, err := findShimLibrary()
		if err != nil {
			return nil, fmt.Errorf("failed to find shim library: %w", err)
		}
		p.shimPath = shimPath
	}

	// Set LD_PRELOAD
	ldPreload := p.shimPath
	for i, env := range cmd.Env {
		if len(env) > 11 && env[:11] == "LD_PRELOAD=" {
			// Append to existing LD_PRELOAD
			ldPreload = env[11:] + ":" + ldPreload
			cmd.Env[i] = "LD_PRELOAD=" + ldPreload
			ldPreload = ""
			break
		}
	}
	if ldPreload != "" {
		cmd.Env = append(cmd.Env, "LD_PRELOAD="+ldPreload)
	}

	// Set default I/O if not already set
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	return p, nil
}

// Start begins execution of the process
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("process already started")
	}

	err := p.cmd.Start()
	if err != nil {
		return err
	}

	p.started = true

	// Start goroutine to wait for process completion
	go func() {
		p.cmd.Wait()
		close(p.finished)
	}()

	return nil
}

// Wait waits for the process to complete
func (p *Process) Wait() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return fmt.Errorf("process not started")
	}
	p.mu.Unlock()
	
	<-p.finished
	if p.cmd.ProcessState.ExitCode() == 0 {
		return nil
	}
	return fmt.Errorf("process exited with code %d", p.cmd.ProcessState.ExitCode())
}

// WaitWithTimeout waits for the process to complete with a timeout
func (p *Process) WaitWithTimeout(timeout time.Duration) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return fmt.Errorf("process not started")
	}
	p.mu.Unlock()

	select {
	case <-p.finished:
		if p.cmd.ProcessState.ExitCode() == 0 {
			return nil
		}
		return fmt.Errorf("process exited with code %d", p.cmd.ProcessState.ExitCode())
	case <-time.After(timeout):
		return fmt.Errorf("process wait timeout")
	}
}

// Kill forcefully terminates the process
func (p *Process) Kill() error {
	if p.cmd.Process == nil {
		return fmt.Errorf("process not started")
	}
	return p.cmd.Process.Kill()
}

// Signal sends a signal to the process
func (p *Process) Signal(sig syscall.Signal) error {
	if p.cmd.Process == nil {
		return fmt.Errorf("process not started")
	}
	return p.cmd.Process.Signal(sig)
}

// Pid returns the process ID
func (p *Process) Pid() int {
	if p.cmd.Process == nil {
		return -1
	}
	return p.cmd.Process.Pid
}

// ExitCode returns the process exit code (only valid after Wait)
func (p *Process) ExitCode() int {
	if p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

// findShimLibrary attempts to locate the intercept.so library
func findShimLibrary() (string, error) {
	// Try common locations relative to the binary
	searchPaths := []string{
		"./intercept.so",
		"../clib/intercept.so",
		"./clib/intercept.so",
		"/usr/local/lib/vibethis/intercept.so",
		"/usr/lib/vibethis/intercept.so",
	}

	// Also check if we're on macOS and look for .dylib
	if isDarwin() {
		dylibPaths := make([]string, 0, len(searchPaths)*2)
		for _, path := range searchPaths {
			dylibPaths = append(dylibPaths, path)
			dylibPaths = append(dylibPaths, filepath.Join(filepath.Dir(path), 
				filepath.Base(path[:len(path)-3])+".dylib"))
		}
		searchPaths = dylibPaths
	}

	// Try to find relative to current executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		searchPaths = append([]string{
			filepath.Join(exeDir, "intercept.so"),
			filepath.Join(exeDir, "..", "clib", "intercept.so"),
			filepath.Join(exeDir, "intercept.dylib"),
			filepath.Join(exeDir, "..", "clib", "intercept.dylib"),
		}, searchPaths...)
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return filepath.Abs(path)
		}
	}

	return "", fmt.Errorf("intercept library not found in any of the search paths")
}

// isDarwin checks if we're running on macOS
func isDarwin() bool {
	return os.Getenv("GOOS") == "darwin" || 
		(os.Getenv("GOOS") == "" && filepath.Base(os.Args[0]) == "darwin")
}