package devcontainer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"vibethis/container/pkg/container"
)

// Manager implements the container.Manager interface using devcontainers
type Manager struct {
	docker *DockerClient
}

// NewManager creates a new devcontainer manager
func NewManager() (*Manager, error) {
	docker, err := NewDockerClient()
	if err != nil {
		return nil, err
	}
	
	return &Manager{
		docker: docker,
	}, nil
}

// Create creates a new container for the specified node
func (m *Manager) Create(ctx context.Context, nodePath string) (string, error) {
	// Look for devcontainer.json in the node path
	devcontainerPath := filepath.Join(nodePath, ".devcontainer", "devcontainer.json")
	
	// Load devcontainer configuration
	dc, err := LoadDevContainer(devcontainerPath)
	if err != nil {
		// If no devcontainer.json, use a default configuration
		dc = &DevContainer{
			ImageContainer: &ImageContainer{
				Image: "mcr.microsoft.com/devcontainers/base:ubuntu",
			},
			DevContainerCommon: DevContainerCommon{
				WorkspaceFolder: "/workspace",
			},
		}
	}
	
	// Build docker run configuration
	config, err := BuildDockerRunCommand(dc, nodePath)
	if err != nil {
		return "", fmt.Errorf("failed to build docker config: %w", err)
	}
	
	// Validate the image exists
	if err := m.docker.ValidateImage(ctx, config.Image); err != nil {
		return "", fmt.Errorf("invalid image: %w", err)
	}
	
	// Create the container
	containerID, err := m.docker.CreateContainer(ctx, config)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}
	
	return containerID, nil
}

// Start starts an existing container
func (m *Manager) Start(ctx context.Context, containerID string) error {
	return m.docker.StartContainer(ctx, containerID)
}

// Stop stops a running container
func (m *Manager) Stop(ctx context.Context, containerID string) error {
	return m.docker.StopContainer(ctx, containerID)
}

// Restart restarts a container
func (m *Manager) Restart(ctx context.Context, containerID string) error {
	if err := m.Stop(ctx, containerID); err != nil {
		return err
	}
	return m.Start(ctx, containerID)
}

// Remove removes a container
func (m *Manager) Remove(ctx context.Context, containerID string) error {
	return m.docker.RemoveContainer(ctx, containerID)
}

// GetInfo returns information about a container
func (m *Manager) GetInfo(ctx context.Context, containerID string) (*container.Info, error) {
	status, err := m.docker.GetContainerStatus(ctx, containerID)
	if err != nil {
		return nil, err
	}
	
	return &container.Info{
		ID:     containerID,
		Status: mapDockerStatus(status),
	}, nil
}

// GetStatus returns the current status of a container
func (m *Manager) GetStatus(ctx context.Context, containerID string) (container.Status, error) {
	status, err := m.docker.GetContainerStatus(ctx, containerID)
	if err != nil {
		return container.StatusNone, err
	}
	
	return mapDockerStatus(status), nil
}

// Exec executes a command in a running container
func (m *Manager) Exec(ctx context.Context, containerID string, command []string) (string, error) {
	return m.docker.ExecInContainer(ctx, containerID, command)
}

// AttachWebSocket attaches a WebSocket for terminal access
func (m *Manager) AttachWebSocket(ctx context.Context, containerID string) (container.TerminalConnection, error) {
	// This would require a more complex implementation with websockets
	// For now, return an error
	return nil, fmt.Errorf("websocket attachment not implemented")
}

// mapDockerStatus maps Docker status to container.Status
func mapDockerStatus(dockerStatus string) container.Status {
	switch strings.ToLower(dockerStatus) {
	case "running":
		return container.StatusRunning
	case "exited", "stopped":
		return container.StatusStopped
	case "error", "dead":
		return container.StatusError
	default:
		return container.StatusNone
	}
}