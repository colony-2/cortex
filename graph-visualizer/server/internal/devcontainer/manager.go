package devcontainer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DevContainerManager manages devcontainer operations
type DevContainerManager struct {
	WorkspaceRoot string
}

// CreateContainer creates a new devcontainer and returns its ID
func (m *DevContainerManager) CreateContainer() (string, error) {
	// Find devcontainer.json file
	configPath, err := FindDevContainerFile(m.WorkspaceRoot)
	if err != nil {
		return "", fmt.Errorf("failed to find devcontainer.json: %w", err)
	}

	// Load and validate the devcontainer configuration
	dc, err := LoadDevContainer(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to load devcontainer.json: %w", err)
	}

	// Create the container using docker
	ctx := context.Background()
	d := &DockerManager{}
	
	containerID, err := d.CreateContainer(ctx, dc, m.WorkspaceRoot)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return containerID, nil
}

// StartContainer starts an existing container
func (m *DevContainerManager) StartContainer(containerID string) error {
	cmd := exec.Command("docker", "start", containerID)
	return cmd.Run()
}

// RestartContainer restarts a container
func (m *DevContainerManager) RestartContainer(containerID string) error {
	cmd := exec.Command("docker", "restart", containerID)
	return cmd.Run()
}

// RemoveContainer stops and removes a container
func (m *DevContainerManager) RemoveContainer(containerID string) error {
	// Stop the container first
	stopCmd := exec.Command("docker", "stop", containerID)
	stopCmd.Run() // Ignore error as container might already be stopped

	// Remove the container
	removeCmd := exec.Command("docker", "rm", containerID)
	return removeCmd.Run()
}

// IsContainerRunning checks if a container is running
func (m *DevContainerManager) IsContainerRunning(containerID string) (bool, error) {
	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(output)) == "true", nil
}