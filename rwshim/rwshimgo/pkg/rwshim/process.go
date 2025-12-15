package rwshim

import (
	"os"
	"syscall"
	"time"

	"github.com/colony-2/colony2/rwshim/rwshimgo/internal"
)

// Process represents a monitored process
type Process struct {
	impl *internal.ProcessImpl
}

// ProcessOption configures process execution
type ProcessOption func(*internal.ProcessImpl)

// WithShimPath sets a custom path to the intercept.so library
func WithShimPath(path string) ProcessOption {
	return ProcessOption(internal.WithShimPath(path))
}

// WithEnv adds environment variables to the process
func WithEnv(env []string) ProcessOption {
	return ProcessOption(internal.WithEnv(env))
}

// WithDir sets the working directory for the process
func WithDir(dir string) ProcessOption {
	return ProcessOption(internal.WithDir(dir))
}

// WithStdin sets the process stdin
func WithStdin(stdin *os.File) ProcessOption {
	return ProcessOption(internal.WithStdin(stdin))
}

// WithStdout sets the process stdout
func WithStdout(stdout *os.File) ProcessOption {
	return ProcessOption(internal.WithStdout(stdout))
}

// WithStderr sets the process stderr
func WithStderr(stderr *os.File) ProcessOption {
	return ProcessOption(internal.WithStderr(stderr))
}

// Start begins execution of the process
func (p *Process) Start() error {
	return p.impl.Start()
}

// Wait waits for the process to complete
func (p *Process) Wait() error {
	return p.impl.Wait()
}

// WaitWithTimeout waits for the process to complete with a timeout
func (p *Process) WaitWithTimeout(timeout time.Duration) error {
	return p.impl.WaitWithTimeout(timeout)
}

// Kill forcefully terminates the process
func (p *Process) Kill() error {
	return p.impl.Kill()
}

// Signal sends a signal to the process
func (p *Process) Signal(sig syscall.Signal) error {
	return p.impl.Signal(sig)
}

// Pid returns the process ID
func (p *Process) Pid() int {
	return p.impl.Pid()
}

// ExitCode returns the process exit code (only valid after Wait)
func (p *Process) ExitCode() int {
	return p.impl.ExitCode()
}
