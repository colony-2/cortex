// Package container provides devcontainer lifecycle management for vibethis.
package container

import (
	"context"
	"fmt"
)

// Status represents the current status of a container.
type Status string

const (
	StatusNone    Status = "none"    // No container exists
	StatusStopped Status = "stopped" // Container exists but is not running
	StatusRunning Status = "running" // Container is running
	StatusError   Status = "error"   // Container is in error state
)

// Info contains information about a container.
type Info struct {
	ID      string            `json:"id"`
	Status  Status            `json:"status"`
	Image   string            `json:"image,omitempty"`
	Created string            `json:"created,omitempty"`
	Ports   map[string]string `json:"ports,omitempty"`
}

// Mount represents a container mount configuration
type Mount struct {
	Type     string // bind, volume, tmpfs
	Source   string // host path
	Target   string // container path  
	ReadOnly bool   // read-only flag
}

// Manager provides container lifecycle operations.
type Manager interface {
	// Create creates a new container for the specified node.
	Create(ctx context.Context, nodePath string) (containerID string, err error)
	
	// Start starts an existing container.
	Start(ctx context.Context, containerID string) error
	
	// Stop stops a running container.
	Stop(ctx context.Context, containerID string) error
	
	// Restart restarts a container.
	Restart(ctx context.Context, containerID string) error
	
	// Remove removes a container.
	Remove(ctx context.Context, containerID string) error
	
	// GetInfo returns information about a container.
	GetInfo(ctx context.Context, containerID string) (*Info, error)
	
	// GetStatus returns the current status of a container.
	GetStatus(ctx context.Context, containerID string) (Status, error)
	
	// Exec executes a command in a running container.
	Exec(ctx context.Context, containerID string, command []string) (output string, err error)
	
	// AttachWebSocket attaches a WebSocket for terminal access.
	AttachWebSocket(ctx context.Context, containerID string) (TerminalConnection, error)
	
	// ConfigureMounts configures custom mount points for containers.
	ConfigureMounts(mounts []Mount) error
}

// TerminalConnection represents a terminal connection to a container.
type TerminalConnection interface {
	// Read reads data from the terminal.
	Read() ([]byte, error)
	
	// Write writes data to the terminal.
	Write(data []byte) error
	
	// Resize resizes the terminal.
	Resize(rows, cols uint16) error
	
	// Close closes the terminal connection.
	Close() error
}

// Config defines configuration for the container manager.
type Config struct {
	// DockerHost is the Docker daemon socket path or URL.
	// If empty, uses the default Docker socket.
	DockerHost string
	
	// NetworkName is the Docker network to attach containers to.
	// If empty, uses the default bridge network.
	NetworkName string
	
	// RegistryAuth provides authentication for private registries.
	RegistryAuth *RegistryAuth
}

// RegistryAuth contains registry authentication information.
type RegistryAuth struct {
	Username string
	Password string
	Registry string
}

// NewManager creates a new container manager.
func NewManager(config Config) Manager {
	// For now, we'll create the internal manager directly
	// In a real implementation, we'd pass the config through
	mgr, err := newInternalManager()
	if err != nil {
		// Return a stub manager if we can't create the real one
		return &stubManager{}
	}
	return mgr
}

// This is a temporary function to create the internal manager
// It will be replaced with proper dependency injection
func newInternalManager() (Manager, error) {
	// We can't import internal packages from pkg, so we'll use a stub for now
	return &stubManager{}, nil
}

type stubManager struct{}

// Create creates a new container for the specified node
func (m *stubManager) Create(ctx context.Context, nodePath string) (containerID string, err error) {
	return "", fmt.Errorf("container creation not implemented")
}

// Start starts an existing container
func (m *stubManager) Start(ctx context.Context, containerID string) error {
	return fmt.Errorf("container start not implemented")
}

// Stop stops a running container
func (m *stubManager) Stop(ctx context.Context, containerID string) error {
	return fmt.Errorf("container stop not implemented")
}

// Restart restarts a container
func (m *stubManager) Restart(ctx context.Context, containerID string) error {
	return fmt.Errorf("container restart not implemented")
}

// Remove removes a container
func (m *stubManager) Remove(ctx context.Context, containerID string) error {
	return fmt.Errorf("container remove not implemented")
}

// GetInfo returns information about a container
func (m *stubManager) GetInfo(ctx context.Context, containerID string) (*Info, error) {
	return nil, fmt.Errorf("container info not implemented")
}

// GetStatus returns the current status of a container
func (m *stubManager) GetStatus(ctx context.Context, containerID string) (Status, error) {
	return StatusNone, fmt.Errorf("container status not implemented")
}

// Exec executes a command in a running container
func (m *stubManager) Exec(ctx context.Context, containerID string, command []string) (output string, err error) {
	return "", fmt.Errorf("container exec not implemented")
}

// AttachWebSocket attaches a WebSocket for terminal access
func (m *stubManager) AttachWebSocket(ctx context.Context, containerID string) (TerminalConnection, error) {
	return nil, fmt.Errorf("container websocket not implemented")
}

// ConfigureMounts configures custom mount points for containers
func (m *stubManager) ConfigureMounts(mounts []Mount) error {
	return fmt.Errorf("container mount configuration not implemented")
}