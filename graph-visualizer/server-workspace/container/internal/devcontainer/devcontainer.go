// Package devcontainer provides functionality for managing dev containers
package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DevContainer represents the devcontainer.json configuration
type DevContainer struct {
	// Embedded common fields
	DevContainerCommon
	
	// Container type fields (one of these should be set)
	ImageContainer      *ImageContainer      `json:"-"`
	DockerfileContainer string               `json:"dockerFile,omitempty"`
	ComposeContainer    *ComposeContainer    `json:"-"`
	
	// Docker compose
	DockerComposeFile interface{}      `json:"dockerComposeFile,omitempty"`
	Service          string            `json:"service,omitempty"`
	RunServices      []string          `json:"runServices,omitempty"`
	
	// NonComposeBase fields
	NonComposeBase   *NonComposeBase  `json:"-"`
}

// DevContainerCommon contains common fields for all container types
type DevContainerCommon struct {
	// Basic container configuration
	Image           string            `json:"image,omitempty"`
	DockerFile      string            `json:"dockerFile,omitempty"`
	Build           Build             `json:"build,omitempty"`
	Context         string            `json:"context,omitempty"`
	WorkspaceFolder string            `json:"workspaceFolder,omitempty"`
	WorkspaceMount  string            `json:"workspaceMount,omitempty"`
	
	// Environment
	ContainerEnv    map[string]string `json:"containerEnv,omitempty"`
	RemoteEnv       map[string]string `json:"remoteEnv,omitempty"`
	
	// User configuration
	ContainerUser   *string           `json:"containerUser,omitempty"`
	RemoteUser      *string           `json:"remoteUser,omitempty"`
	
	// Ports and networking
	ForwardPorts    interface{}       `json:"forwardPorts,omitempty"`
	AppPort         interface{}       `json:"appPort,omitempty"`
	
	// Commands
	OnCreateCommand      interface{}    `json:"onCreateCommand,omitempty"`
	UpdateContentCommand interface{}    `json:"updateContentCommand,omitempty"`
	PostCreateCommand    interface{}    `json:"postCreateCommand,omitempty"`
	PostStartCommand     interface{}    `json:"postStartCommand,omitempty"`
	PostAttachCommand    interface{}    `json:"postAttachCommand,omitempty"`
	InitializeCommand    interface{}    `json:"initializeCommand,omitempty"`
	
	// Mounts and volumes
	Mounts          interface{}       `json:"mounts,omitempty"`
	
	// Security
	CapAdd          []string          `json:"capAdd,omitempty"`
	SecurityOpt     []string          `json:"securityOpt,omitempty"`
	Init            *bool             `json:"init,omitempty"`
	Privileged      *bool             `json:"privileged,omitempty"`
	
	// Features
	Features        *DevContainerCommonFeatures `json:"features,omitempty"`
	
	// Extensions
	Customizations  map[string]interface{} `json:"customizations,omitempty"`
	
	// Other settings
	Name            string            `json:"name,omitempty"`
	UpdateRemoteUserUID *bool         `json:"updateRemoteUserUID,omitempty"`
	UserEnvProbe    string            `json:"userEnvProbe,omitempty"`
	OverrideCommand *bool             `json:"overrideCommand,omitempty"`
	ShutdownAction  string            `json:"shutdownAction,omitempty"`
}

// ImageContainer represents an image-based container
type ImageContainer struct {
	Image string `json:"image"`
}

// DevContainerCommonFeatures represents devcontainer features
type DevContainerCommonFeatures struct {
	Fish                 string                 `json:"fish,omitempty"`
	Gradle               string                 `json:"gradle,omitempty"`
	Maven                string                 `json:"maven,omitempty"`
	AdditionalProperties map[string]interface{} `json:"-"`
}

// DevContainerCommonHostRequirements represents host requirements
type DevContainerCommonHostRequirements struct {
	CPUs     string `json:"cpus,omitempty"`
	Memory   string `json:"memory,omitempty"`
	Storage  string `json:"storage,omitempty"`
	Gpu      string `json:"gpu,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for DevContainerCommonFeatures
func (f *DevContainerCommonFeatures) UnmarshalJSON(data []byte) error {
	// First unmarshal known fields
	type Alias DevContainerCommonFeatures
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(f),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	// Then unmarshal everything to get additional properties
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	
	// Remove known fields
	delete(raw, "fish")
	delete(raw, "gradle")
	delete(raw, "maven")
	
	// Store the rest as additional properties
	if len(raw) > 0 {
		f.AdditionalProperties = raw
	}
	
	return nil
}

// ComposeContainer represents Docker Compose configuration
type ComposeContainer struct {
	DockerComposeFile interface{} `json:"dockerComposeFile,omitempty"`
	Service          string       `json:"service,omitempty"`
}

// NonComposeBase contains fields specific to non-compose configurations
type NonComposeBase struct {
	RunArgs         []string    `json:"runArgs,omitempty"`
	WorkspaceFolder *string     `json:"workspaceFolder,omitempty"`
	WorkspaceMount  *string     `json:"workspaceMount,omitempty"`
	AppPort         interface{} `json:"appPort,omitempty"`
}

// Build represents build configuration
type Build struct {
	Dockerfile string            `json:"dockerfile,omitempty"`
	Context    string            `json:"context,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Target     string            `json:"target,omitempty"`
	CacheFrom  []string          `json:"cacheFrom,omitempty"`
}

// Mount types
const (
	MountTypeBind   = "bind"
	MountTypeVolume = "volume"
	MountTypeTmpfs  = "tmpfs"
)

// DevContainerCommonMountsElem represents a mount configuration
type DevContainerCommonMountsElem struct {
	Type     string  `json:"type"`
	Source   *string `json:"source,omitempty"`
	Target   string  `json:"target"`
	ReadOnly bool    `json:"readOnly,omitempty"`
}

// LoadDevContainer loads a devcontainer.json file
func LoadDevContainer(path string) (*DevContainer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read devcontainer.json: %w", err)
	}
	
	var dc DevContainer
	if err := json.Unmarshal(data, &dc); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer.json: %w", err)
	}
	
	// Also parse raw JSON to get runArgs if present
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err == nil {
		// Initialize NonComposeBase if needed
		if runArgs, ok := raw["runArgs"].([]interface{}); ok {
			if dc.NonComposeBase == nil {
				dc.NonComposeBase = &NonComposeBase{}
			}
			dc.NonComposeBase.RunArgs = make([]string, 0, len(runArgs))
			for _, arg := range runArgs {
				if s, ok := arg.(string); ok {
					dc.NonComposeBase.RunArgs = append(dc.NonComposeBase.RunArgs, s)
				}
			}
		}
		
		// Set container type based on what's present
		if dc.Image != "" {
			dc.ImageContainer = &ImageContainer{
				Image: dc.Image,
			}
		}
		
		// Set ComposeContainer if dockerComposeFile is present
		if dc.DockerComposeFile != nil {
			dc.ComposeContainer = &ComposeContainer{
				DockerComposeFile: dc.DockerComposeFile,
				Service:          dc.Service,
			}
		}
	}
	
	return &dc, nil
}

// DockerRunConfig represents Docker run configuration
type DockerRunConfig struct {
	Image           string
	WorkspaceMount  string
	WorkspaceFolder string
	Environment     map[string]string
	Ports           []string
	Mounts          []string // Changed to []string to match tests
	CapAdd          []string
	Capabilities    []string // Alias for CapAdd
	SecurityOpt     []string
	SecurityOpts    []string // Alias for SecurityOpt
	Init            bool
	Privileged      bool
	User            string
	Name            string
	Command         []string
	RunArgs         []string // Additional run arguments
}

// Mount represents a Docker mount
type Mount struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

// BuildDockerRunCommand builds a Docker run configuration from a DevContainer
func BuildDockerRunCommand(dc *DevContainer, workspaceFolder string) (*DockerRunConfig, error) {
	config := &DockerRunConfig{
		WorkspaceFolder: dc.WorkspaceFolder,
		Environment:     make(map[string]string),
		Ports:           []string{},
		Mounts:          []string{},
		CapAdd:          dc.CapAdd,
		Capabilities:    dc.CapAdd, // Set both for compatibility
		SecurityOpt:     dc.SecurityOpt,
		SecurityOpts:    dc.SecurityOpt, // Set both for compatibility
	}
	
	// Determine the image
	if dc.ImageContainer != nil {
		config.Image = dc.ImageContainer.Image
	} else if dc.Image != "" {
		config.Image = dc.Image
	} else {
		return nil, fmt.Errorf("no image specified")
	}
	
	// Set workspace folder
	if dc.NonComposeBase != nil && dc.NonComposeBase.WorkspaceFolder != nil {
		config.WorkspaceFolder = *dc.NonComposeBase.WorkspaceFolder
	} else if config.WorkspaceFolder == "" {
		config.WorkspaceFolder = "/workspaces/" + filepath.Base(workspaceFolder)
	}
	
	// Handle workspace mount
	if dc.NonComposeBase != nil && dc.NonComposeBase.WorkspaceMount != nil {
		config.WorkspaceMount = *dc.NonComposeBase.WorkspaceMount
	} else if dc.WorkspaceMount != "" {
		config.WorkspaceMount = dc.WorkspaceMount
	} else {
		absPath, _ := filepath.Abs(workspaceFolder)
		config.WorkspaceMount = fmt.Sprintf("type=bind,source=%s,target=%s", absPath, config.WorkspaceFolder)
	}
	
	// Handle environment variables
	for k, v := range dc.ContainerEnv {
		config.Environment[k] = v
	}
	
	// Handle ports
	if dc.ForwardPorts != nil {
		ports := parseForwardPorts(dc.ForwardPorts)
		config.Ports = append(config.Ports, ports...)
	}
	
	// Handle app ports
	if dc.AppPort != nil {
		ports := parseAppPorts(dc.AppPort)
		config.Ports = append(config.Ports, ports...)
	}
	
	// Handle mounts
	if dc.Mounts != nil {
		mounts := parseMounts(dc.Mounts)
		for _, mount := range mounts {
			mountStr := buildMountStringFromMount(mount)
			config.Mounts = append(config.Mounts, mountStr)
		}
	}
	
	// Handle init
	if dc.Init != nil {
		config.Init = *dc.Init
	}
	
	// Handle privileged
	if dc.Privileged != nil {
		config.Privileged = *dc.Privileged
	}
	
	// Handle user
	if dc.ContainerUser != nil && *dc.ContainerUser != "" {
		config.User = *dc.ContainerUser
	}
	
	// Handle name
	if dc.Name != "" {
		config.Name = dc.Name
	}
	
	// Handle run args
	if dc.NonComposeBase != nil && dc.NonComposeBase.RunArgs != nil {
		config.RunArgs = dc.NonComposeBase.RunArgs
	}
	
	return config, nil
}

// ToDockerRunArgs converts the config to docker run arguments
func (c *DockerRunConfig) ToDockerRunArgs() []string {
	args := []string{"run", "-it", "--rm"}
	
	// Add name if specified
	if c.Name != "" {
		args = append(args, "--name", c.Name)
	}
	
	// Add workspace mount
	if c.WorkspaceMount != "" {
		args = append(args, "-v", c.WorkspaceMount)
	}
	
	// Add working directory
	if c.WorkspaceFolder != "" {
		args = append(args, "-w", c.WorkspaceFolder)
	}
	
	// Add environment variables
	for k, v := range c.Environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	
	// Add ports
	for _, port := range c.Ports {
		args = append(args, "-p", port)
	}
	
	// Add additional run args first
	if c.RunArgs != nil {
		args = append(args, c.RunArgs...)
	}
	
	// Add mounts
	for _, mountStr := range c.Mounts {
		args = append(args, "--mount", mountStr)
	}
	
	// Add capabilities
	for _, cap := range c.CapAdd {
		args = append(args, "--cap-add", cap)
	}
	
	// Add security options
	for _, opt := range c.SecurityOpt {
		args = append(args, "--security-opt", opt)
	}
	
	// Add init
	if c.Init {
		args = append(args, "--init")
	}
	
	// Add privileged
	if c.Privileged {
		args = append(args, "--privileged")
	}
	
	// Add user
	if c.User != "" {
		args = append(args, "-u", c.User)
	}
	
	// Add image
	args = append(args, c.Image)
	
	// Add command
	args = append(args, c.Command...)
	
	return args
}

// Validate validates the docker run configuration
func (c *DockerRunConfig) Validate() error {
	if c.Image == "" {
		return fmt.Errorf("image is required")
	}
	return nil
}

// Helper functions

func parseForwardPorts(ports interface{}) []string {
	var result []string
	
	switch v := ports.(type) {
	case []interface{}:
		for _, port := range v {
			switch p := port.(type) {
			case float64:
				result = append(result, fmt.Sprintf("%d:%d", int(p), int(p)))
			case string:
				result = append(result, p)
			}
		}
	}
	
	return result
}

func parseAppPorts(ports interface{}) []string {
	var result []string
	
	switch v := ports.(type) {
	case float64:
		result = append(result, fmt.Sprintf("%d:%d", int(v), int(v)))
	case string:
		result = append(result, v)
	case []interface{}:
		for _, port := range v {
			switch p := port.(type) {
			case float64:
				result = append(result, fmt.Sprintf("%d:%d", int(p), int(p)))
			case string:
				result = append(result, p)
			}
		}
	}
	
	return result
}

func formatForwardPort(port interface{}) string {
	switch p := port.(type) {
	case float64:
		return fmt.Sprintf("%d:%d", int(p), int(p))
	case int:
		return fmt.Sprintf("%d:%d", p, p)
	case string:
		return p
	default:
		return ""
	}
}

func parseMounts(mounts interface{}) []Mount {
	var result []Mount
	
	switch v := mounts.(type) {
	case []interface{}:
		for _, mount := range v {
			if m, ok := mount.(map[string]interface{}); ok {
				mountObj := Mount{}
				if t, ok := m["type"].(string); ok {
					mountObj.Type = t
				}
				if s, ok := m["source"].(string); ok {
					mountObj.Source = s
				}
				if t, ok := m["target"].(string); ok {
					mountObj.Target = t
				}
				if r, ok := m["readOnly"].(bool); ok {
					mountObj.ReadOnly = r
				}
				result = append(result, mountObj)
			}
		}
	}
	
	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ValidateDockerCommand validates docker command arguments
func ValidateDockerCommand(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("invalid docker command")
	}
	if args[0] != "run" {
		return fmt.Errorf("expected 'run' command")
	}
	return nil
}

// ExtractDockerImage extracts the image from docker run arguments
func ExtractDockerImage(args []string) (string, error) {
	// Find the image (last non-flag argument before any command)
	for i := len(args) - 1; i >= 0; i-- {
		if !strings.HasPrefix(args[i], "-") && i > 0 && !strings.HasPrefix(args[i-1], "-") {
			return args[i], nil
		}
	}
	return "", fmt.Errorf("image not found in docker command")
}

// DryRunDockerCommand performs a dry run of a docker command
func DryRunDockerCommand(args []string) error {
	// In a real implementation, this would execute docker with --dry-run
	// For now, just validate the command
	return ValidateDockerCommand(args)
}

// buildMountString builds a mount string from a DevContainerCommonMountsElem
func buildMountString(dcMount DevContainerCommonMountsElem) string {
	result := fmt.Sprintf("type=%s,target=%s", dcMount.Type, dcMount.Target)
	if dcMount.Source != nil && *dcMount.Source != "" {
		result += fmt.Sprintf(",source=%s", *dcMount.Source)
	}
	return result
}

// buildMountStringFromMount builds a mount string from a Mount struct
func buildMountStringFromMount(mount Mount) string {
	result := fmt.Sprintf("type=%s", mount.Type)
	if mount.Source != "" {
		result += fmt.Sprintf(",source=%s", mount.Source)
	}
	result += fmt.Sprintf(",target=%s", mount.Target)
	if mount.ReadOnly {
		result += ",readonly"
	}
	return result
}

// LifecycleCommand represents a lifecycle command that can be a string, array, or object
type LifecycleCommand struct {
	Type     string                            // "string", "array", or "object"
	Command  string                            // For string commands
	Args     []string                          // For array commands
	Commands map[string]*LifecycleCommand      // For object commands (nested commands)
	Object   map[string]interface{}            // Raw object data
}

// ParseLifecycleCommand parses an interface{} into a LifecycleCommand
func ParseLifecycleCommand(cmd interface{}) (*LifecycleCommand, error) {
	if cmd == nil {
		return nil, nil
	}
	
	result := &LifecycleCommand{}
	
	switch v := cmd.(type) {
	case string:
		result.Type = "string"
		result.Command = v
	case []interface{}:
		result.Type = "array"
		result.Args = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result.Args = append(result.Args, s)
			}
		}
	case map[string]interface{}:
		result.Type = "object"
		result.Object = v
		result.Commands = make(map[string]*LifecycleCommand)
		// Parse nested commands
		for name, cmdValue := range v {
			if nestedCmd, _ := ParseLifecycleCommand(cmdValue); nestedCmd != nil {
				result.Commands[name] = nestedCmd
			}
		}
	default:
		return nil, fmt.Errorf("unsupported command type: %T", cmd)
	}
	
	return result, nil
}

// ToShellCommand converts a LifecycleCommand to a shell command string
func (lc *LifecycleCommand) ToShellCommand() string {
	if lc == nil {
		return ""
	}
	
	switch lc.Type {
	case "string":
		return lc.Command
	case "array":
		if len(lc.Args) == 0 {
			return ""
		}
		// TODO: Proper shell escaping
		return strings.Join(lc.Args, " ")
	case "object":
		// For object commands, we can't convert to a single shell command
		// This would need to be handled differently (e.g., parallel execution)
		return ""
	default:
		return ""
	}
}

// strPtr returns a pointer to a string
func strPtr(s string) *string {
	return &s
}

// checkDockerAvailable checks if Docker is available
func checkDockerAvailable() error {
	client, err := NewDockerClient()
	if err != nil {
		return err
	}
	_ = client
	return nil
}

// FindDevContainerFile finds a devcontainer.json file in the given directory
func FindDevContainerFile(dir string) (string, error) {
	// Check .devcontainer/devcontainer.json
	path := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	
	// Check .devcontainer.json
	path = filepath.Join(dir, ".devcontainer.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	
	return "", fmt.Errorf("no devcontainer.json found in %s", dir)
}

// ProcessLifecycleCommands processes lifecycle commands from a DevContainer
func ProcessLifecycleCommands(dc *DevContainer) (map[string]*LifecycleCommand, error) {
	commands := make(map[string]*LifecycleCommand)
	
	// Process each lifecycle command
	if dc.InitializeCommand != nil {
		if cmd, err := ParseLifecycleCommand(dc.InitializeCommand); err == nil && cmd != nil {
			commands["initializeCommand"] = cmd
		}
	}
	
	if dc.OnCreateCommand != nil {
		if cmd, err := ParseLifecycleCommand(dc.OnCreateCommand); err == nil && cmd != nil {
			commands["onCreateCommand"] = cmd
		}
	}
	
	if dc.UpdateContentCommand != nil {
		if cmd, err := ParseLifecycleCommand(dc.UpdateContentCommand); err == nil && cmd != nil {
			commands["updateContentCommand"] = cmd
		}
	}
	
	if dc.PostCreateCommand != nil {
		if cmd, err := ParseLifecycleCommand(dc.PostCreateCommand); err == nil && cmd != nil {
			commands["postCreateCommand"] = cmd
		}
	}
	
	if dc.PostStartCommand != nil {
		if cmd, err := ParseLifecycleCommand(dc.PostStartCommand); err == nil && cmd != nil {
			commands["postStartCommand"] = cmd
		}
	}
	
	if dc.PostAttachCommand != nil {
		if cmd, err := ParseLifecycleCommand(dc.PostAttachCommand); err == nil && cmd != nil {
			commands["postAttachCommand"] = cmd
		}
	}
	
	return commands, nil
}

// GetLifecycleScript generates a shell script for lifecycle commands
func GetLifecycleScript(dc *DevContainer, phase string) (string, error) {
	commands, err := ProcessLifecycleCommands(dc)
	if err != nil {
		return "", err
	}
	
	var script strings.Builder
	script.WriteString("#!/bin/bash\nset -e\n\n")
	
	// If phase is specified, only include that command
	if phase != "" {
		if cmd, exists := commands[phase]; exists && cmd != nil {
			script.WriteString(fmt.Sprintf("# %s\n", phase))
			if shellCmd := cmd.ToShellCommand(); shellCmd != "" {
				script.WriteString(shellCmd + "\n")
			}
		}
	} else {
		// Include all commands in order
		order := []string{"initializeCommand", "onCreateCommand", "updateContentCommand", "postCreateCommand", "postStartCommand", "postAttachCommand"}
		
		for _, name := range order {
			if cmd, exists := commands[name]; exists && cmd != nil {
				script.WriteString(fmt.Sprintf("# %s\n", name))
				if shellCmd := cmd.ToShellCommand(); shellCmd != "" {
					script.WriteString(shellCmd + "\n\n")
				}
			}
		}
	}
	
	return script.String(), nil
}

// ExpandVariables expands variables in a DevContainer's command strings
func ExpandVariables(dc *DevContainer, vars map[string]string) {
	// Helper function to expand variables in interface{}
	var expandInterface func(cmd interface{}) interface{}
	expandInterface = func(cmd interface{}) interface{} {
		switch v := cmd.(type) {
		case string:
			return expandVariableString(v, vars)
		case []interface{}:
			result := make([]interface{}, len(v))
			for i, item := range v {
				if s, ok := item.(string); ok {
					result[i] = expandVariableString(s, vars)
				} else {
					result[i] = item
				}
			}
			return result
		case map[string]interface{}:
			result := make(map[string]interface{})
			for k, val := range v {
				result[k] = expandInterface(val)
			}
			return result
		default:
			return cmd
		}
	}
	
	// Expand variables in all commands
	if dc.InitializeCommand != nil {
		dc.InitializeCommand = expandInterface(dc.InitializeCommand)
	}
	if dc.OnCreateCommand != nil {
		dc.OnCreateCommand = expandInterface(dc.OnCreateCommand)
	}
	if dc.UpdateContentCommand != nil {
		dc.UpdateContentCommand = expandInterface(dc.UpdateContentCommand)
	}
	if dc.PostCreateCommand != nil {
		dc.PostCreateCommand = expandInterface(dc.PostCreateCommand)
	}
	if dc.PostStartCommand != nil {
		dc.PostStartCommand = expandInterface(dc.PostStartCommand)
	}
	if dc.PostAttachCommand != nil {
		dc.PostAttachCommand = expandInterface(dc.PostAttachCommand)
	}
}

// expandVariableString expands variables in a string
func expandVariableString(s string, vars map[string]string) string {
	result := s
	for key, value := range vars {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
		result = strings.ReplaceAll(result, "$"+key, value)
	}
	return result
}

// HostRequirementsCheck checks if host requirements are valid
func HostRequirementsCheck(req *DevContainerCommonHostRequirements) error {
	if req == nil {
		return nil
	}
	
	// Check CPU count
	if req.CPUs != "" {
		if cpus, err := strconv.Atoi(req.CPUs); err != nil || cpus <= 0 {
			return fmt.Errorf("invalid CPU count: %s", req.CPUs)
		}
	}
	
	// TODO: Add more validation for memory, storage, GPU
	
	return nil
}

// MergeDevContainers merges multiple DevContainers
func MergeDevContainers(base, override *DevContainer) *DevContainer {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	
	// Simple merge - override takes precedence
	result := *base
	
	if override.Image != "" {
		result.Image = override.Image
	}
	if override.ImageContainer != nil {
		result.ImageContainer = override.ImageContainer
	}
	// TODO: Implement full merge logic
	
	return &result
}

// LoadDevContainerWithExtends loads a devcontainer.json with extends support
func LoadDevContainerWithExtends(path string) (*DevContainer, error) {
	// For now, just load normally
	// TODO: Implement extends support
	return LoadDevContainer(path)
}

// GetStandardVariables returns standard devcontainer variables
func GetStandardVariables(workspaceFolder string) map[string]string {
	return map[string]string{
		"localWorkspaceFolder":     workspaceFolder,
		"containerWorkspaceFolder": "/workspace",
		"localWorkspaceFolderBasename": filepath.Base(workspaceFolder),
	}
}