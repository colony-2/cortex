package devcontainer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/divisive-ai/vibethis/server/container/pkg/container"
)

// Manager implements the container.Manager interface using devcontainers
type Manager struct {
	docker *DockerClient
	devContainer *DevContainer // Optional pre-configured devcontainer
	dockerClient *DockerClient // Alias for consistency with terminal.go
	customMounts []container.Mount // Custom mount configurations
}

// NewManager creates a new devcontainer manager
func NewManager() (*Manager, error) {
	docker, err := NewDockerClient()
	if err != nil {
		return nil, err
	}

	return &Manager{
		docker: docker,
		dockerClient: docker, // Set alias for terminal.go compatibility
	}, nil
}

// SetDevContainer sets a pre-configured devcontainer for the manager
func (m *Manager) SetDevContainer(dc *DevContainer) {
	m.devContainer = dc
}

// Create creates a new container for the specified node
func (m *Manager) Create(ctx context.Context, nodePath string) (string, error) {
	var dc *DevContainer
	
	// Use pre-configured devcontainer if available
	if m.devContainer != nil {
		dc = m.devContainer
	} else {
		// Look for devcontainer.json in the node path
		devcontainerPath := filepath.Join(nodePath, ".devcontainer", "devcontainer.json")

		// Load devcontainer configuration
		var err error
		dc, err = LoadDevContainer(devcontainerPath)
		if err != nil {
			// If no devcontainer.json, use a default configuration
			dc = &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "alpine:latest",
				},
				DevContainerCommon: DevContainerCommon{
					WorkspaceFolder: "/workspace",
				},
			}
		}
		
	}
	
	// Apply custom mounts if configured
	if len(m.customMounts) > 0 {
		if err := m.applyCustomMounts(dc); err != nil {
			return "", fmt.Errorf("failed to apply custom mounts: %w", err)
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

// ConfigureMounts configures custom mount points for containers
func (m *Manager) ConfigureMounts(mounts []container.Mount) error {
	m.customMounts = mounts
	return nil
}

// applyCustomMounts applies custom mount configurations to a DevContainer
func (m *Manager) applyCustomMounts(dc *DevContainer) error {
    // Build custom mounts in devcontainer object format (object style)
    var custom []interface{}
    for _, mount := range m.customMounts {
        custom = append(custom, map[string]interface{}{
            "type":     mount.Type,
            "source":   mount.Source,
            "target":   mount.Target,
            "readonly": mount.ReadOnly,
        })
    }

    // Merge: preserve mounts declared in devcontainer.json and append custom mounts.
    // If there are duplicate object-style targets, prefer custom by removing earlier duplicates.
    // Note: string-style mounts are kept as-is (cannot safely de-dup without parsing).
    var merged []interface{}

    // Track targets we will override to avoid duplicates
    targets := map[string]bool{}
    for _, cm := range custom {
        if m, ok := cm.(map[string]interface{}); ok {
            if tgt, ok := m["target"].(string); ok && tgt != "" {
                targets[tgt] = true
            }
        }
    }

    // First, copy existing mounts that are not overridden by a custom mount (object-style)
    for _, em := range dc.Mounts {
        if mobj, ok := em.(map[string]interface{}); ok {
            if tgt, ok := mobj["target"].(string); ok && tgt != "" {
                if targets[tgt] {
                    // Skip, will be provided by custom
                    continue
                }
            }
        }
        merged = append(merged, em)
    }

    // Append all custom mounts
    merged = append(merged, custom...)

    dc.Mounts = merged

    // Clear workspace mount to prevent conflicts (shai manages workspace mount separately)
    dc.WorkspaceMount = "none"

    // Ensure workspace folder is set
    if dc.WorkspaceFolder == "" {
        dc.WorkspaceFolder = "/workspace"
    }

    return nil
}

// Close closes the Docker client connection
func (m *Manager) Close() error {
	if m.docker != nil {
		return m.docker.Close()
	}
	return nil
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
