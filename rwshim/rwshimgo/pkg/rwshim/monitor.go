package rwshim

import (
	"context"
	"fmt"
	
	"github.com/divisive-ai/vibethis/rwshim/rwshimgo/internal"
)

const (
	// DefaultSocketPath is the default Unix domain socket path
	DefaultSocketPath = "/tmp/vibethis-rwshim.sock"
)

// Monitor manages the Unix domain socket server and intercepts I/O operations
type Monitor struct {
	impl *internal.MonitorImpl
}

// NewMonitor creates a new Monitor with the given policy function
func NewMonitor(policy PolicyFunc) *Monitor {
	// Create a wrapper that converts between public and internal types
	internalPolicy := func(req internal.Request) internal.Response {
		publicReq := Request{
			Operation: Operation(req.Operation),
			FD:        req.FD,
			Size:      req.Size,
			Filename:  req.Filename,
		}
		publicResp := policy(publicReq)
		return internal.Response{Allow: publicResp.Allow}
	}
	
	return &Monitor{
		impl: internal.NewMonitorImpl(DefaultSocketPath, internalPolicy),
	}
}

// NewMonitorWithPath creates a new Monitor with a custom socket path
func NewMonitorWithPath(socketPath string, policy PolicyFunc) *Monitor {
	// Create a wrapper that converts between public and internal types
	internalPolicy := func(req internal.Request) internal.Response {
		publicReq := Request{
			Operation: Operation(req.Operation),
			FD:        req.FD,
			Size:      req.Size,
			Filename:  req.Filename,
		}
		publicResp := policy(publicReq)
		return internal.Response{Allow: publicResp.Allow}
	}
	
	return &Monitor{
		impl: internal.NewMonitorImpl(socketPath, internalPolicy),
	}
}

// Start begins listening on the Unix domain socket
func (m *Monitor) Start() error {
	return m.impl.Start()
}

// Stop gracefully shuts down the monitor
func (m *Monitor) Stop() error {
	return m.impl.Stop()
}

// IsRunning returns whether the monitor is currently running
func (m *Monitor) IsRunning() bool {
	return m.impl.IsRunning()
}

// SocketPath returns the path to the Unix domain socket
func (m *Monitor) SocketPath() string {
	return m.impl.SocketPath()
}

// StartProcess starts a new process with the shim library preloaded
func (m *Monitor) StartProcess(ctx context.Context, command string, args []string, opts ...ProcessOption) (*Process, error) {
	// Ensure monitor is running
	if !m.IsRunning() {
		return nil, fmt.Errorf("monitor must be started before launching processes")
	}

	// Convert options
	internalOpts := make([]internal.ProcessOption, 0, len(opts))
	for _, opt := range opts {
		internalOpts = append(internalOpts, internal.ProcessOption(opt))
	}

	procImpl, err := m.impl.StartProcess(ctx, command, args, internalOpts...)
	if err != nil {
		return nil, err
	}

	return &Process{impl: procImpl}, nil
}