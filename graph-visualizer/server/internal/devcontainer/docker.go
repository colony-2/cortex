package devcontainer

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// DockerRunConfig represents the configuration for docker run command
type DockerRunConfig struct {
	Image           string
	WorkspaceMount  string
	WorkspaceFolder string
	Mounts          []string
	Environment     map[string]string
	RemoteEnv       map[string]*string // Remote environment variables (may be unset)
	Ports           []string
	RunArgs         []string
	Privileged      bool
	Init            bool
	User            string
	RemoteUser      string // User for remote processes
	Capabilities    []string
	SecurityOpts    []string
	Hostname        string
	Network         string
	DNS             []string
	ExtraHosts      []string
	Volumes         []string // Legacy volume format
}

// BuildDockerRunCommand constructs a docker run command from a DevContainer
func BuildDockerRunCommand(dc *DevContainer, workspaceRoot string) (*DockerRunConfig, error) {
	config := &DockerRunConfig{
		Environment: make(map[string]string),
	}

	// Handle image-based container
	if dc.ImageContainer != nil && dc.ImageContainer.Image != "" {
		config.Image = dc.ImageContainer.Image
	} else if dc.DockerfileContainer != nil {
		// For Dockerfile-based containers, we'd need to build first
		return nil, fmt.Errorf("dockerfile-based containers require building first")
	} else if dc.ComposeContainer != nil {
		return nil, fmt.Errorf("docker-compose containers are not supported for docker run")
	} else {
		return nil, fmt.Errorf("no container configuration found")
	}

	// Set workspace mount and folder
	if dc.NonComposeBase != nil {
		if dc.NonComposeBase.WorkspaceMount != nil {
			// Expand variables in workspace mount
			mount := *dc.NonComposeBase.WorkspaceMount
			mount = strings.ReplaceAll(mount, "${localWorkspaceFolder}", workspaceRoot)
			config.WorkspaceMount = mount
		} else {
			// Default workspace mount
			projectName := filepath.Base(workspaceRoot)
			config.WorkspaceMount = fmt.Sprintf("type=bind,source=%s,target=/workspaces/%s", workspaceRoot, projectName)
		}

		if dc.NonComposeBase.WorkspaceFolder != nil {
			config.WorkspaceFolder = *dc.NonComposeBase.WorkspaceFolder
		} else {
			// Default workspace folder
			projectName := filepath.Base(workspaceRoot)
			config.WorkspaceFolder = fmt.Sprintf("/workspaces/%s", projectName)
		}

		// Add run args
		config.RunArgs = append(config.RunArgs, dc.NonComposeBase.RunArgs...)

		// Handle ports
		if dc.NonComposeBase.AppPort != nil {
			ports := parseAppPorts(dc.NonComposeBase.AppPort)
			config.Ports = append(config.Ports, ports...)
		}
	} else {
		// Set defaults if NonComposeBase is nil
		projectName := filepath.Base(workspaceRoot)
		config.WorkspaceMount = fmt.Sprintf("type=bind,source=%s,target=/workspaces/%s", workspaceRoot, projectName)
		config.WorkspaceFolder = fmt.Sprintf("/workspaces/%s", projectName)
	}

	// Handle common properties
	if dc.Privileged != nil && *dc.Privileged {
		config.Privileged = true
	}

	if dc.Init != nil && *dc.Init {
		config.Init = true
	}

	// Set user
	if dc.ContainerUser != nil {
		config.User = *dc.ContainerUser
	}

	// Add capabilities
	config.Capabilities = append(config.Capabilities, dc.CapAdd...)

	// Add security options
	config.SecurityOpts = append(config.SecurityOpts, dc.SecurityOpt...)

	// Handle mounts
	for _, mount := range dc.Mounts {
		mountStr := buildMountString(mount)
		if mountStr != "" {
			config.Mounts = append(config.Mounts, mountStr)
		}
	}

	// Handle environment variables
	for k, v := range dc.ContainerEnv {
		config.Environment[k] = v
	}
	
	// Handle remote environment variables
	config.RemoteEnv = dc.RemoteEnv
	
	// Set remote user if specified
	if dc.RemoteUser != nil {
		config.RemoteUser = *dc.RemoteUser
	}

	// Handle forward ports
	for _, port := range dc.ForwardPorts {
		portStr := formatForwardPort(port)
		if portStr != "" && !contains(config.Ports, portStr) {
			config.Ports = append(config.Ports, portStr)
		}
	}

	return config, nil
}

// Validate checks if the DockerRunConfig has valid settings
func (c *DockerRunConfig) Validate() error {
	if c.Image == "" {
		return fmt.Errorf("image is required")
	}

	// Validate mount syntax
	for _, mount := range c.Mounts {
		if !strings.Contains(mount, "type=") {
			return fmt.Errorf("invalid mount format: %s (missing type=)", mount)
		}
		if !strings.Contains(mount, "target=") {
			return fmt.Errorf("invalid mount format: %s (missing target=)", mount)
		}
	}

	// Validate port syntax
	for _, port := range c.Ports {
		// Port should be either "port" or "host:container"
		parts := strings.Split(port, ":")
		if len(parts) > 2 {
			return fmt.Errorf("invalid port format: %s", port)
		}
	}

	return nil
}

// ToDockerRunArgs converts DockerRunConfig to docker run command arguments
func (c *DockerRunConfig) ToDockerRunArgs() []string {
	args := []string{"run", "--rm", "-it"}

	// Add workspace mount
	if c.WorkspaceMount != "" {
		args = append(args, "--mount", c.WorkspaceMount)
	}

	// Add working directory
	if c.WorkspaceFolder != "" {
		args = append(args, "-w", c.WorkspaceFolder)
	}

	// Add additional mounts
	for _, mount := range c.Mounts {
		args = append(args, "--mount", mount)
	}

	// Add environment variables
	for k, v := range c.Environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add ports
	for _, port := range c.Ports {
		args = append(args, "-p", port)
	}

	// Add privileged flag
	if c.Privileged {
		args = append(args, "--privileged")
	}

	// Add init flag
	if c.Init {
		args = append(args, "--init")
	}

	// Add user
	if c.User != "" {
		args = append(args, "--user", c.User)
	}

	// Add capabilities
	for _, cap := range c.Capabilities {
		args = append(args, "--cap-add", cap)
	}

	// Add security options
	for _, opt := range c.SecurityOpts {
		args = append(args, "--security-opt", opt)
	}

	// Add hostname
	if c.Hostname != "" {
		args = append(args, "--hostname", c.Hostname)
	}

	// Add network
	if c.Network != "" {
		args = append(args, "--network", c.Network)
	}

	// Add DNS servers
	for _, dns := range c.DNS {
		args = append(args, "--dns", dns)
	}

	// Add extra hosts
	for _, host := range c.ExtraHosts {
		args = append(args, "--add-host", host)
	}

	// Add legacy volumes
	for _, vol := range c.Volumes {
		args = append(args, "-v", vol)
	}

	// Add custom run args
	args = append(args, c.RunArgs...)

	// Add image
	args = append(args, c.Image)

	return args
}

func buildMountString(mount DevContainerCommonMountsElem) string {
	parts := []string{
		fmt.Sprintf("type=%s", mount.Type),
		fmt.Sprintf("target=%s", mount.Target),
	}
	
	if mount.Source != nil && *mount.Source != "" {
		parts = append(parts, fmt.Sprintf("source=%s", *mount.Source))
	}

	return strings.Join(parts, ",")
}

func parseAppPorts(appPort interface{}) []string {
	if appPort == nil {
		return nil
	}

	var ports []string

	switch v := appPort.(type) {
	case float64:
		// Single port number
		ports = append(ports, fmt.Sprintf("%d:%d", int(v), int(v)))
	case string:
		// Port string (e.g., "8000:8010")
		ports = append(ports, v)
	case []interface{}:
		// Array of ports
		if len(v) == 0 {
			return nil
		}
		for _, p := range v {
			switch port := p.(type) {
			case float64:
				ports = append(ports, fmt.Sprintf("%d:%d", int(port), int(port)))
			case string:
				ports = append(ports, port)
			}
		}
	}

	return ports
}

func formatForwardPort(port interface{}) string {
	switch v := port.(type) {
	case float64:
		return fmt.Sprintf("%d:%d", int(v), int(v))
	case string:
		return v
	}
	return ""
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// DockerManager manages Docker operations for devcontainers
type DockerManager struct{}

// CreateContainer creates a new Docker container from a DevContainer configuration
func (d *DockerManager) CreateContainer(ctx context.Context, dc *DevContainer, workspaceRoot string) (string, error) {
	// Build docker run configuration
	config, err := BuildDockerRunCommand(dc, workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("failed to build docker run config: %w", err)
	}

	// Build the docker command arguments
	args := config.ToDockerRunArgs()
	
	// Modify for create command: replace "run" with "create" and remove interactive flags
	createArgs := []string{"create", "-d"}
	// Skip "run", "--rm", "-it" from the beginning
	for i := 3; i < len(args); i++ {
		createArgs = append(createArgs, args[i])
	}
	
	// Add a command to keep container running
	createArgs = append(createArgs, "sleep", "infinity")
	
	// Execute docker create command
	cmd := exec.CommandContext(ctx, "docker", createArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Include the docker output in the error for better debugging
		return "", fmt.Errorf("failed to create container: %w\nDocker output: %s", err, string(output))
	}

	// Extract container ID from output
	containerID := strings.TrimSpace(string(output))
	return containerID, nil
}