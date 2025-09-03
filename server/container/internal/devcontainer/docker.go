package devcontainer

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// DockerClient provides Docker operations using the Docker SDK
type DockerClient struct {
	client *client.Client
}

// NewDockerClient creates a new Docker client using the SDK
func NewDockerClient() (*DockerClient, error) {
	var connectionAttempts []func() (*client.Client, error)
	
	// On macOS, prioritize Docker Desktop locations
	if runtime.GOOS == "darwin" {
		connectionAttempts = []func() (*client.Client, error){
			// 1. Try environment settings first (respects DOCKER_HOST)
			func() (*client.Client, error) {
				return client.NewClientWithOpts(
					client.FromEnv,
					client.WithAPIVersionNegotiation(),
				)
			},
			// 2. Try Docker Desktop socket location on macOS (primary)
			func() (*client.Client, error) {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				return client.NewClientWithOpts(
					client.WithHost("unix://"+homeDir+"/.docker/run/docker.sock"),
					client.WithAPIVersionNegotiation(),
				)
			},
			// 3. Try Docker Desktop alternative location
			func() (*client.Client, error) {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				return client.NewClientWithOpts(
					client.WithHost("unix://"+homeDir+"/.docker/desktop/docker.sock"),
					client.WithAPIVersionNegotiation(),
				)
			},
			// 4. Try the default Unix socket (less common on macOS)
			func() (*client.Client, error) {
				return client.NewClientWithOpts(
					client.WithHost("unix:///var/run/docker.sock"),
					client.WithAPIVersionNegotiation(),
				)
			},
		}
	} else {
		// On Linux, prioritize system locations
		connectionAttempts = []func() (*client.Client, error){
			// 1. Try environment settings first (respects DOCKER_HOST)
			func() (*client.Client, error) {
				return client.NewClientWithOpts(
					client.FromEnv,
					client.WithAPIVersionNegotiation(),
				)
			},
			// 2. Try the default Unix socket
			func() (*client.Client, error) {
				return client.NewClientWithOpts(
					client.WithHost("unix:///var/run/docker.sock"),
					client.WithAPIVersionNegotiation(),
				)
			},
			// 3. Try rootless Docker socket
			func() (*client.Client, error) {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				// Try XDG_RUNTIME_DIR first for rootless Docker
				xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
				if xdgRuntimeDir != "" {
					return client.NewClientWithOpts(
						client.WithHost("unix://"+xdgRuntimeDir+"/docker.sock"),
						client.WithAPIVersionNegotiation(),
					)
				}
				// Fallback to common rootless location
				return client.NewClientWithOpts(
					client.WithHost("unix://"+homeDir+"/.docker/run/docker.sock"),
					client.WithAPIVersionNegotiation(),
				)
			},
		}
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	var lastErr error
	for _, attempt := range connectionAttempts {
		cli, err := attempt()
		if err != nil {
			lastErr = err
			continue
		}
		
		// Test connection
		_, err = cli.Ping(ctx)
		if err == nil {
			return &DockerClient{client: cli}, nil
		}
		
		cli.Close()
		lastErr = err
	}
	
	return nil, fmt.Errorf("failed to connect to Docker daemon: %w", lastErr)
}

// Close closes the Docker client connection
func (c *DockerClient) Close() error {
	return c.client.Close()
}

// GetClient returns the underlying Docker client for direct access
func (c *DockerClient) GetClient() *client.Client {
	return c.client
}

// RunContainer runs a Docker container with the given configuration
func (c *DockerClient) RunContainer(ctx context.Context, config *DockerRunConfig) error {
	// Create container
	containerID, err := c.CreateContainer(ctx, config)
	if err != nil {
		return err
	}
	
	// Start container
	return c.StartContainer(ctx, containerID)
}

// CreateContainer creates a Docker container without starting it
func (c *DockerClient) CreateContainer(ctx context.Context, config *DockerRunConfig) (string, error) {
	// Convert environment map to slice
	var envSlice []string
	for k, v := range config.Environment {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}
	
	// Convert our config to Docker SDK types
	containerConfig := &container.Config{
		Image:        config.Image,
		Cmd:          strslice.StrSlice(config.Command),
		Env:          envSlice,
		WorkingDir:   config.WorkspaceFolder,
		User:         config.User,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		OpenStdin:    true,
		StdinOnce:    false,
	}
	
	// Convert Init bool to *bool
	var initPtr *bool
	if config.Init {
		initPtr = &config.Init
	}
	
	// Convert port bindings
	hostConfig := &container.HostConfig{
		Privileged: config.Privileged,
		Init:       initPtr,
	}
	
	// Parse and add mounts
	for _, mountStr := range config.Mounts {
		// Parse mount string (e.g., "type=bind,source=/host/path,target=/container/path,readonly")
		mountParts := make(map[string]string)
		mountReadOnly := false
		
		for _, part := range strings.Split(mountStr, ",") {
			if part == "readonly" || part == "ro" {
				mountReadOnly = true
				continue
			}
			if part == "rw" {
				mountReadOnly = false
				continue
			}
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				mountParts[kv[0]] = kv[1]
			}
		}
		
		mountType := mount.TypeBind
		switch mountParts["type"] {
		case "volume":
			mountType = mount.TypeVolume
		case "tmpfs":
			mountType = mount.TypeTmpfs
		}
		
		dockerMount := mount.Mount{
			Type:     mountType,
			Source:   mountParts["source"],
			Target:   mountParts["target"],
			ReadOnly: mountReadOnly,
		}
		
		// Check for empty target and fail fast
		if dockerMount.Target == "" {
			return "", fmt.Errorf("mount target is empty for mount string: %s", mountStr)
		}
		
		hostConfig.Mounts = append(hostConfig.Mounts, dockerMount)
	}
	
	// Add capabilities
	if len(config.CapAdd) > 0 {
		hostConfig.CapAdd = strslice.StrSlice(config.CapAdd)
	} else if len(config.Capabilities) > 0 {
		hostConfig.CapAdd = strslice.StrSlice(config.Capabilities)
	}
	
	// Add security options
	if len(config.SecurityOpt) > 0 {
		hostConfig.SecurityOpt = config.SecurityOpt
	} else if len(config.SecurityOpts) > 0 {
		hostConfig.SecurityOpt = config.SecurityOpts
	}
	
	// Add the workspace mount if specified
	if config.WorkspaceMount != "" && config.WorkspaceMount != "none" {
		
		// Parse workspace mount
		mountParts := make(map[string]string)
		mountReadOnly := false
		
		for _, part := range strings.Split(config.WorkspaceMount, ",") {
			if part == "readonly" || part == "ro" {
				mountReadOnly = true
				continue
			}
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				mountParts[kv[0]] = kv[1]
			}
		}
		
		mountType := mount.TypeBind
		switch mountParts["type"] {
		case "volume":
			mountType = mount.TypeVolume
		case "tmpfs":
			mountType = mount.TypeTmpfs
		}
		
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:     mountType,
			Source:   mountParts["source"],
			Target:   mountParts["target"],
			ReadOnly: mountReadOnly,
		})
	}
	
	
	resp, err := c.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, config.Name)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}
	
	return resp.ID, nil
}

// StartContainer starts an existing container
func (c *DockerClient) StartContainer(ctx context.Context, containerID string) error {
	err := c.client.ContainerStart(ctx, containerID, container.StartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	
	return nil
}

// StopContainer stops a running container
func (c *DockerClient) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10 // seconds
	err := c.client.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: &timeout,
	})
	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	
	return nil
}

// RemoveContainer removes a container
func (c *DockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	err := c.client.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force: true,
	})
	if err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	
	return nil
}

// ExecInContainer executes a command in a running container
func (c *DockerClient) ExecInContainer(ctx context.Context, containerID string, command []string) (string, error) {
	execConfig := container.ExecOptions{
		Cmd:          command,
		AttachStdout: true,
		AttachStderr: true,
	}
	
	execResp, err := c.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}
	
	resp, err := c.client.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach exec: %w", err)
	}
	defer resp.Close()
	
	// Read output - Docker multiplexes stdout/stderr with headers
	var stdout, stderr strings.Builder
	_, err = stdcopy.StdCopy(&stdout, &stderr, resp.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to read exec output: %w", err)
	}
	
	// Check exec exit code
	inspectResp, err := c.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect exec: %w", err)
	}
	
	if inspectResp.ExitCode != 0 {
		return "", fmt.Errorf("exec failed with exit code %d: %s", inspectResp.ExitCode, stderr.String())
	}
	
	return stdout.String(), nil
}

// GetContainerStatus gets the status of a container
func (c *DockerClient) GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	resp, err := c.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}
	
	return resp.State.Status, nil
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
func (c *DockerClient) ValidateImage(ctx context.Context, imageName string) error {
	// First try to inspect the image locally
	_, _, err := c.client.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil // Image exists locally
	}
	
	// If not found locally, try to pull it
	reader, err := c.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()
	
	// Consume the output to ensure pull completes
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	
	return nil
}

// CreateVolume creates a Docker volume
func (c *DockerClient) CreateVolume(ctx context.Context, name string) error {
	_, err := c.client.VolumeCreate(ctx, volume.CreateOptions{
		Name: name,
	})
	if err != nil {
		return fmt.Errorf("failed to create volume %s: %w", name, err)
	}
	
	return nil
}

// RemoveVolume removes a Docker volume
func (c *DockerClient) RemoveVolume(ctx context.Context, name string) error {
	err := c.client.VolumeRemove(ctx, name, true)
	if err != nil {
		return fmt.Errorf("failed to remove volume %s: %w", name, err)
	}
	
	return nil
}

// GetContainerLogs gets logs from a container
func (c *DockerClient) GetContainerLogs(ctx context.Context, containerID string, tail int) (string, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
	}
	
	if tail <= 0 {
		options.Tail = "all"
	}
	
	reader, err := c.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}
	defer reader.Close()
	
	logs, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read container logs: %w", err)
	}
	
	// Strip Docker log headers (8 bytes per line)
	lines := strings.Split(string(logs), "\n")
	var cleanedLines []string
	for _, line := range lines {
		if len(line) > 8 {
			cleanedLines = append(cleanedLines, line[8:])
		} else if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}
	
	return strings.Join(cleanedLines, "\n"), nil
}