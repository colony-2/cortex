package devcontainer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DockerClient provides Docker operations
type DockerClient struct {
	dockerPath string
}

// NewDockerClient creates a new Docker client
func NewDockerClient() (*DockerClient, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker not found in PATH: %w", err)
	}
	
	return &DockerClient{
		dockerPath: dockerPath,
	}, nil
}

// RunContainer runs a Docker container with the given configuration
func (c *DockerClient) RunContainer(ctx context.Context, config *DockerRunConfig) error {
	args := config.ToDockerRunArgs()
	
	cmd := exec.CommandContext(ctx, c.dockerPath, args...)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker run failed: %w: %s", err, stderr.String())
	}
	
	return nil
}

// CreateContainer creates a Docker container without starting it
func (c *DockerClient) CreateContainer(ctx context.Context, config *DockerRunConfig) (string, error) {
	args := config.ToDockerRunArgs()
	// Replace "run" with "create"
	args[0] = "create"
	
	cmd := exec.CommandContext(ctx, c.dockerPath, args...)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker create failed: %w: %s", err, stderr.String())
	}
	
	containerID := strings.TrimSpace(stdout.String())
	return containerID, nil
}

// StartContainer starts an existing container
func (c *DockerClient) StartContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, c.dockerPath, "start", containerID)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker start failed: %w: %s", err, stderr.String())
	}
	
	return nil
}

// StopContainer stops a running container
func (c *DockerClient) StopContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, c.dockerPath, "stop", containerID)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker stop failed: %w: %s", err, stderr.String())
	}
	
	return nil
}

// RemoveContainer removes a container
func (c *DockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, c.dockerPath, "rm", "-f", containerID)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker rm failed: %w: %s", err, stderr.String())
	}
	
	return nil
}

// ExecInContainer executes a command in a running container
func (c *DockerClient) ExecInContainer(ctx context.Context, containerID string, command []string) (string, error) {
	args := []string{"exec", containerID}
	args = append(args, command...)
	
	cmd := exec.CommandContext(ctx, c.dockerPath, args...)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker exec failed: %w: %s", err, stderr.String())
	}
	
	return stdout.String(), nil
}

// GetContainerStatus gets the status of a container
func (c *DockerClient) GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	cmd := exec.CommandContext(ctx, c.dockerPath, "inspect", "-f", "{{.State.Status}}", containerID)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker inspect failed: %w: %s", err, stderr.String())
	}
	
	return strings.TrimSpace(stdout.String()), nil
}

// WaitForContainer waits for a container to reach a specific status
func (c *DockerClient) WaitForContainer(ctx context.Context, containerID string, desiredStatus string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		status, err := c.GetContainerStatus(ctx, containerID)
		if err != nil {
			return err
		}
		
		if status == desiredStatus {
			return nil
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			// Continue checking
		}
	}
	
	return fmt.Errorf("timeout waiting for container to reach status %s", desiredStatus)
}

// ValidateImage checks if a Docker image exists locally or can be pulled
func (c *DockerClient) ValidateImage(ctx context.Context, image string) error {
	// First try to inspect the image locally
	cmd := exec.CommandContext(ctx, c.dockerPath, "image", "inspect", image)
	if err := cmd.Run(); err == nil {
		return nil // Image exists locally
	}
	
	// Try to pull the image
	cmd = exec.CommandContext(ctx, c.dockerPath, "pull", image)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull image %s: %w: %s", image, err, stderr.String())
	}
	
	return nil
}

// CreateVolume creates a Docker volume
func (c *DockerClient) CreateVolume(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, c.dockerPath, "volume", "create", name)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create volume %s: %w: %s", name, err, stderr.String())
	}
	
	return nil
}

// RemoveVolume removes a Docker volume
func (c *DockerClient) RemoveVolume(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, c.dockerPath, "volume", "rm", name)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove volume %s: %w: %s", name, err, stderr.String())
	}
	
	return nil
}

// GetContainerLogs gets logs from a container
func (c *DockerClient) GetContainerLogs(ctx context.Context, containerID string, tail int) (string, error) {
	args := []string{"logs"}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}
	args = append(args, containerID)
	
	cmd := exec.CommandContext(ctx, c.dockerPath, args...)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker logs failed: %w: %s", err, stderr.String())
	}
	
	return stdout.String(), nil
}