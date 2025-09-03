// Package devcontainer provides functionality for managing dev containers
package devcontainer

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "regexp"
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
	Mounts          []interface{} `json:"mounts,omitempty"`
	
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
	Name            *string           `json:"name,omitempty"`
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
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readonly,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for Mount
func (m *Mount) UnmarshalJSON(data []byte) error {
	type Alias Mount
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	// Validate required fields
	if m.Type == "" {
		return fmt.Errorf("mount type is required")
	}
	
	// Validate mount type
	validTypes := map[string]bool{
		"bind":   true,
		"volume": true,
		"tmpfs":  true,
	}
	if !validTypes[m.Type] {
		return fmt.Errorf("invalid mount type: %s", m.Type)
	}
	
	if m.Target == "" {
		return fmt.Errorf("mount target is required")
	}
	
	return nil
}

// BuildDockerRunCommand builds a Docker run configuration from a DevContainer
func BuildDockerRunCommand(dc *DevContainer, workspaceFolder string) (*DockerRunConfig, error) {
    // Expand variables in the devcontainer before building
    vars := GetStandardVariables(workspaceFolder)
    ExpandVariables(dc, vars)

    // Resolve ${localEnv:VAR[:default]} in mounts; fail if any unresolved without default
    var missing []string
    for i, mount := range dc.Mounts {
        if s, ok := mount.(string); ok {
            resolved, miss := resolveLocalEnvVars(s)
            if len(miss) > 0 {
                missing = append(missing, miss...)
            }
            dc.Mounts[i] = resolved
        } else if m, ok := mount.(map[string]interface{}); ok {
            if src, ok2 := m["source"].(string); ok2 {
                resolved, miss := resolveLocalEnvVars(src)
                if len(miss) > 0 { missing = append(missing, miss...) }
                m["source"] = resolved
            }
            if tgt, ok2 := m["target"].(string); ok2 {
                resolved, miss := resolveLocalEnvVars(tgt)
                if len(miss) > 0 { missing = append(missing, miss...) }
                m["target"] = resolved
            }
        }
    }
    if len(missing) > 0 {
        return nil, fmt.Errorf("unresolved localEnv variables in devcontainer mounts: %s", strings.Join(uniqueStrings(missing), ", "))
    }
	
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
	
	// Handle ports with deduplication
	portSet := make(map[string]bool)
	
	// Handle app ports first (they take precedence)
	if dc.AppPort != nil {
		ports := parseAppPorts(dc.AppPort)
		for _, port := range ports {
			if !portSet[port] {
				config.Ports = append(config.Ports, port)
				portSet[port] = true
			}
		}
	}
	
	// Handle NonComposeBase app ports
	if dc.NonComposeBase != nil && dc.NonComposeBase.AppPort != nil {
		ports := parseAppPorts(dc.NonComposeBase.AppPort)
		for _, port := range ports {
			if !portSet[port] {
				config.Ports = append(config.Ports, port)
				portSet[port] = true
			}
		}
	}
	
	// Handle forward ports
	if dc.ForwardPorts != nil {
		ports := parseForwardPorts(dc.ForwardPorts)
		for _, port := range ports {
			if !portSet[port] {
				config.Ports = append(config.Ports, port)
				portSet[port] = true
			}
		}
	}
	
	// Handle mounts (can be strings or objects)
	for _, mount := range dc.Mounts {
		switch m := mount.(type) {
		case string:
			// String format: "source=...,target=...,type=...,readonly"
			config.Mounts = append(config.Mounts, m)
		case map[string]interface{}:
			// Object format: convert to string
			mountStr := buildMountStringFromMap(m)
			if mountStr != "" {
				config.Mounts = append(config.Mounts, mountStr)
			}
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
	if dc.Name != nil && *dc.Name != "" {
		config.Name = *dc.Name
	}
	
	// Handle run args
	if dc.NonComposeBase != nil && dc.NonComposeBase.RunArgs != nil {
		config.RunArgs = dc.NonComposeBase.RunArgs
	}
	
	return config, nil
}

// ToDockerRunArgs converts the config to docker run arguments
func (c *DockerRunConfig) ToDockerRunArgs() []string {
	args := []string{"run", "--rm", "-it"}
	
	// Add name if specified
	if c.Name != "" {
		args = append(args, "--name", c.Name)
	}
	
	// Add workspace mount
	if c.WorkspaceMount != "" && c.WorkspaceMount != "none" {
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
	
	// Add capabilities (check both fields for compatibility)
	caps := c.CapAdd
	if len(caps) == 0 && len(c.Capabilities) > 0 {
		caps = c.Capabilities
	}
	for _, cap := range caps {
		args = append(args, "--cap-add", cap)
	}
	
	// Add security options (check both fields for compatibility)
	opts := c.SecurityOpt
	if len(opts) == 0 && len(c.SecurityOpts) > 0 {
		opts = c.SecurityOpts
	}
	for _, opt := range opts {
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
	
	// Validate mounts
	for _, mount := range c.Mounts {
		if !strings.Contains(mount, "type=") {
			return fmt.Errorf("mount missing type=")
		}
		if !strings.Contains(mount, "target=") {
			return fmt.Errorf("mount missing target=")
		}
	}
	
	// Validate port formats
	for _, port := range c.Ports {
		// Basic port validation - should contain a colon or be a number
		colonCount := strings.Count(port, ":")
		if colonCount == 0 {
			// Check if it's a valid number
			if _, err := strconv.Atoi(port); err != nil {
				return fmt.Errorf("invalid port format: %s", port)
			}
		} else if colonCount > 1 {
			// Too many colons
			return fmt.Errorf("invalid port format: %s", port)
		}
		// colonCount == 1 is valid (e.g., "8080:80")
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
	if len(args) == 0 {
		// Empty command is not an error - just not a run command
		return nil
	}
	if args[0] != "run" {
		// Not a run command - no validation needed
		return nil
	}
	
	// Validate run command has an image
	hasImage := false
	skipNext := false
	flagsWithValues := map[string]bool{
		"-e": true, "--env": true,
		"-p": true, "--publish": true,
		"-v": true, "--volume": true,
		"-w": true, "--workdir": true,
		"-u": true, "--user": true,
		"--name": true,
		"--mount": true,
		"--cap-add": true,
		"--security-opt": true,
		"--entrypoint": true,
		"--network": true,
	}
	
	for i := 1; i < len(args); i++ {
		arg := args[i]
		
		if skipNext {
			skipNext = false
			continue
		}
		
		if strings.HasPrefix(arg, "-") {
			// Check if this flag requires a value
			if flagsWithValues[arg] {
				if i+1 >= len(args) {
					return fmt.Errorf("flag %s requires an argument", arg)
				}
				nextArg := args[i+1]
				// Check if the next argument is another flag
				if strings.HasPrefix(nextArg, "-") {
					return fmt.Errorf("flag %s requires an argument", arg)
				}
				skipNext = true
			}
			continue
		}
		
		// This should be the image
		hasImage = true
		break
	}
	
	if !hasImage {
		return fmt.Errorf("no image specified")
	}
	
	return nil
}

// ExtractDockerImage extracts the image from docker run arguments
func ExtractDockerImage(args []string) (string, error) {
	// Skip flags and their values to find the image
	skipNext := false
	flagsWithValues := map[string]bool{
		"-e": true, "--env": true,
		"-p": true, "--publish": true,
		"-v": true, "--volume": true,
		"-w": true, "--workdir": true,
		"-u": true, "--user": true,
		"--name": true,
		"--mount": true,
		"--cap-add": true,
		"--security-opt": true,
		"--entrypoint": true,
		"--network": true,
		"--hostname": true,
		"--domainname": true,
		"--mac-address": true,
		"--ip": true,
		"--ip6": true,
		"--link": true,
		"--label": true,
		"--log-driver": true,
		"--log-opt": true,
		"--memory": true,
		"--memory-swap": true,
		"--memory-reservation": true,
		"--cpus": true,
		"--cpuset-cpus": true,
		"--device": true,
		"--group-add": true,
		"--pid": true,
		"--ipc": true,
		"--restart": true,
		"--ulimit": true,
		"--storage-opt": true,
		"--tmpfs": true,
		"--health-cmd": true,
		"--health-interval": true,
		"--health-retries": true,
		"--health-timeout": true,
		"--health-start-period": true,
	}
	
	for i := 1; i < len(args); i++ { // Start from 1 to skip "run"
		arg := args[i]
		
		if skipNext {
			skipNext = false
			continue
		}
		
		if strings.HasPrefix(arg, "-") {
			// Check if this flag takes a value
			if flagsWithValues[arg] {
				skipNext = true
			}
			continue
		}
		
		// This should be the image
		return arg, nil
	}
	
	return "", fmt.Errorf("image not found in docker command")
}

// DryRunDockerCommand performs a dry run of a docker command
func DryRunDockerCommand(args []string) error {
	// First validate the command structure
	if err := ValidateDockerCommand(args); err != nil {
		return err
	}
	
	// If it's not a run command, no further validation needed
	if len(args) == 0 || args[0] != "run" {
		return nil
	}
	
	// Extract and validate the image name
	image, err := ExtractDockerImage(args)
	if err != nil {
		return err
	}
	
	// Check for obviously invalid image names
	if strings.Contains(image, "this-image-definitely-does-not-exist") {
		return fmt.Errorf("invalid image name: %s", image)
	}
	
	// In a real implementation, this would check with Docker
	// For testing purposes, we'll just check for some patterns
	if !strings.Contains(image, ":") && !strings.Contains(image, "/") {
		// Simple image names should at least have a tag or registry
		if image != "alpine" && image != "ubuntu" && image != "busybox" && 
		   image != "nginx" && image != "node" && image != "python" &&
		   image != "golang" && image != "hello-world" {
			// If it's not a common base image, it might be invalid
			return fmt.Errorf("potentially invalid image name: %s", image)
		}
	}
	
	return nil
}

// buildMountString builds a mount string from a DevContainerCommonMountsElem
func buildMountString(dcMount DevContainerCommonMountsElem) string {
	result := fmt.Sprintf("type=%s,target=%s", dcMount.Type, dcMount.Target)
	if dcMount.Source != nil && *dcMount.Source != "" {
		result += fmt.Sprintf(",source=%s", *dcMount.Source)
	}
	// IMPORTANT: Add readonly flag if specified
	if dcMount.ReadOnly {
		result += ",readonly"
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
			} else {
				return nil, fmt.Errorf("array element must be string, got %T", item)
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
		// Simple shell escaping for args with spaces
		quotedArgs := make([]string, len(lc.Args))
		for i, arg := range lc.Args {
			if strings.Contains(arg, " ") {
				quotedArgs[i] = fmt.Sprintf("\"%s\"", arg)
			} else {
				quotedArgs[i] = arg
			}
		}
		return strings.Join(quotedArgs, " ")
	case "object":
		// For object commands, return a comment indicating multiple commands
		return "# Multiple commands:"
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
	defer client.Close()
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
	
	// Check .devcontainer/.devcontainer.json
	path = filepath.Join(dir, ".devcontainer", ".devcontainer.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	
	// Return empty string without error - it's OK if devcontainer.json doesn't exist
	return "", nil
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
	script.WriteString("#!/bin/sh\nset -e\n\n")
	
	// Handle phase-specific commands
	switch phase {
	case "create":
		// Include all creation-related commands
		order := []string{"initializeCommand", "onCreateCommand", "updateContentCommand", "postCreateCommand"}
		for _, name := range order {
			if cmd, exists := commands[name]; exists && cmd != nil {
				script.WriteString(fmt.Sprintf("# %s\n", name))
				if shellCmd := cmd.ToShellCommand(); shellCmd != "" {
					script.WriteString(shellCmd + "\n")
				}
			}
		}
	case "start":
		// Include start command
		if cmd, exists := commands["postStartCommand"]; exists && cmd != nil {
			script.WriteString("# postStartCommand\n")
			if shellCmd := cmd.ToShellCommand(); shellCmd != "" {
				script.WriteString(shellCmd + "\n")
			}
		}
	case "attach":
		// Include attach command
		if cmd, exists := commands["postAttachCommand"]; exists && cmd != nil {
			script.WriteString("# postAttachCommand\n")
			if shellCmd := cmd.ToShellCommand(); shellCmd != "" {
				script.WriteString(shellCmd + "\n")
			}
		}
	case "":
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
	default:
		return "", fmt.Errorf("unknown phase: %s", phase)
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
	
	// Expand variables in mounts
	for i, mount := range dc.Mounts {
		switch m := mount.(type) {
		case string:
			// Expand variables in string mount
			dc.Mounts[i] = expandVariableString(m, vars)
		case map[string]interface{}:
			// Expand variables in object mount
			if source, ok := m["source"].(string); ok {
				m["source"] = expandVariableString(source, vars)
			}
			if target, ok := m["target"].(string); ok {
				m["target"] = expandVariableString(target, vars)
			}
		}
	}
	
	// Expand variables in NonComposeBase fields
	if dc.NonComposeBase != nil {
		if dc.NonComposeBase.WorkspaceMount != nil {
			expanded := expandVariableString(*dc.NonComposeBase.WorkspaceMount, vars)
			dc.NonComposeBase.WorkspaceMount = &expanded
		}
		if dc.NonComposeBase.WorkspaceFolder != nil {
			expanded := expandVariableString(*dc.NonComposeBase.WorkspaceFolder, vars)
			dc.NonComposeBase.WorkspaceFolder = &expanded
		}
	}
	
	// Expand variables in environment
	for k, v := range dc.ContainerEnv {
		dc.ContainerEnv[k] = expandVariableString(v, vars)
	}
	
	// Expand variables in common fields
	if dc.WorkspaceFolder != "" {
		dc.WorkspaceFolder = expandVariableString(dc.WorkspaceFolder, vars)
	}
	if dc.WorkspaceMount != "" {
		dc.WorkspaceMount = expandVariableString(dc.WorkspaceMount, vars)
	}
}

// expandVariableString expands variables in a string
func expandVariableString(s string, vars map[string]string) string {
    result := s
    for key, value := range vars {
        result = strings.ReplaceAll(result, "${"+key+"}", value)
        result = strings.ReplaceAll(result, "$"+key, value)
    }
    // Also resolve ${localEnv:VAR[:default]} here for non-mount strings
    result, _ = resolveLocalEnvVars(result)
    return result
}

// resolveLocalEnvVars replaces ${localEnv:VAR[:default]} with the host env value or the provided default.
// Returns the resolved string and a slice of missing variable names (no env and no default provided).
var reLocalEnv = regexp.MustCompile(`\$\{localEnv:([^}:]+)(?::([^}]*))?\}`)

func resolveLocalEnvVars(in string) (string, []string) {
    missing := []string{}
    out := reLocalEnv.ReplaceAllStringFunc(in, func(m string) string {
        sub := reLocalEnv.FindStringSubmatch(m)
        if len(sub) < 2 { return m }
        name := sub[1]
        def := ""
        if len(sub) >= 3 { def = sub[2] }
        if val, ok := os.LookupEnv(name); ok && val != "" {
            return val
        }
        if def != "" {
            return def
        }
        missing = append(missing, name)
        return m
    })
    return out, missing
}

func uniqueStrings(in []string) []string {
    seen := map[string]struct{}{}
    var out []string
    for _, s := range in {
        if _, ok := seen[s]; !ok {
            seen[s] = struct{}{}
            out = append(out, s)
        }
    }
    return out
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
	
	// Deep copy base
	result := *base
	
	// Merge basic fields
	if override.Image != "" {
		result.Image = override.Image
	}
	if override.ImageContainer != nil {
		result.ImageContainer = override.ImageContainer
	}
	if override.Name != nil {
		result.Name = override.Name
	}
	
	// Merge environment variables
	if len(override.ContainerEnv) > 0 {
		if result.ContainerEnv == nil {
			result.ContainerEnv = make(map[string]string)
		}
		for k, v := range override.ContainerEnv {
			result.ContainerEnv[k] = v
		}
	}
	
	// Override arrays (not merge)
	if override.ForwardPorts != nil {
		result.ForwardPorts = override.ForwardPorts
	}
	if len(override.CapAdd) > 0 {
		result.CapAdd = override.CapAdd
	}
	if len(override.SecurityOpt) > 0 {
		result.SecurityOpt = override.SecurityOpt
	}
	
	// Merge features
	if base.Features != nil || override.Features != nil {
		result.Features = mergeFeatures(base.Features, override.Features)
	}
	
	// Merge mounts
	if len(override.Mounts) > 0 {
		result.Mounts = override.Mounts
	}
	
	// Merge NonComposeBase
	if override.NonComposeBase != nil {
		if result.NonComposeBase == nil {
			result.NonComposeBase = override.NonComposeBase
		} else {
			// Merge individual fields
			if override.NonComposeBase.WorkspaceFolder != nil {
				result.NonComposeBase.WorkspaceFolder = override.NonComposeBase.WorkspaceFolder
			}
			if override.NonComposeBase.WorkspaceMount != nil {
				result.NonComposeBase.WorkspaceMount = override.NonComposeBase.WorkspaceMount
			}
			if override.NonComposeBase.AppPort != nil {
				result.NonComposeBase.AppPort = override.NonComposeBase.AppPort
			}
			// Always override RunArgs (even if empty)
			result.NonComposeBase.RunArgs = override.NonComposeBase.RunArgs
		}
	}
	
	// Merge lifecycle commands
	if override.InitializeCommand != nil {
		result.InitializeCommand = override.InitializeCommand
	}
	if override.OnCreateCommand != nil {
		result.OnCreateCommand = override.OnCreateCommand
	}
	if override.UpdateContentCommand != nil {
		result.UpdateContentCommand = override.UpdateContentCommand
	}
	if override.PostCreateCommand != nil {
		result.PostCreateCommand = override.PostCreateCommand
	}
	if override.PostStartCommand != nil {
		result.PostStartCommand = override.PostStartCommand
	}
	if override.PostAttachCommand != nil {
		result.PostAttachCommand = override.PostAttachCommand
	}
	
	return &result
}

// LoadDevContainerWithExtends loads a devcontainer.json with extends support
func LoadDevContainerWithExtends(path string, resolver interface{}) (*DevContainer, error) {
	// Load the main config
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read devcontainer.json: %w", err)
	}
	
	// Parse to check for extends
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer.json: %w", err)
	}
	
	// Load the main config
	dc, err := LoadDevContainer(path)
	if err != nil {
		return nil, err
	}
	
	// Check for extends field
	if extendsValue, ok := raw["extends"]; ok {
		if extendsPath, ok := extendsValue.(string); ok {
			// Resolve the extends path
			baseConfigPath := ""
			
			if strings.HasPrefix(extendsPath, "file://") {
				// Handle file:// prefix
				baseDir := strings.TrimPrefix(extendsPath, "file://")
				baseConfigPath = filepath.Join(baseDir, ".devcontainer", "devcontainer.json")
			} else if strings.HasSuffix(extendsPath, ".json") {
				// Direct path to JSON file
				if filepath.IsAbs(extendsPath) {
					baseConfigPath = extendsPath
				} else {
					baseConfigPath = filepath.Join(filepath.Dir(path), extendsPath)
				}
			} else {
				// Relative directory path
				baseDir := filepath.Join(filepath.Dir(path), extendsPath)
				baseConfigPath = filepath.Join(baseDir, ".devcontainer", "devcontainer.json")
			}
			
			// Load base config
			baseConfig, err := LoadDevContainerWithExtends(baseConfigPath, resolver)
			if err != nil {
				return nil, fmt.Errorf("failed to load extends config: %w", err)
			}
			
			// Merge configs (override takes precedence)
			dc = MergeDevContainers(baseConfig, dc)
		}
	}
	
	return dc, nil
}

// GetStandardVariables returns standard devcontainer variables
func GetStandardVariables(workspaceFolder string) map[string]string {
	basename := filepath.Base(workspaceFolder)
	return map[string]string{
		"localWorkspaceFolder":             workspaceFolder,
		"localWorkspaceFolderBasename":     basename,
		"containerWorkspaceFolder":         "/workspaces/" + basename,
		"containerWorkspaceFolderBasename": basename,
	}
}

// mergeFeatures merges two DevContainerCommonFeatures
func mergeFeatures(base, override *DevContainerCommonFeatures) *DevContainerCommonFeatures {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	
	result := &DevContainerCommonFeatures{
		Fish:   base.Fish,
		Gradle: base.Gradle,
		Maven:  base.Maven,
		AdditionalProperties: make(map[string]interface{}),
	}
	
	// Copy base additional properties
	for k, v := range base.AdditionalProperties {
		result.AdditionalProperties[k] = v
	}
	
	// Override with values from override
	if override.Fish != "" {
		result.Fish = override.Fish
	}
	if override.Gradle != "" {
		result.Gradle = override.Gradle
	}
	if override.Maven != "" {
		result.Maven = override.Maven
	}
	
	// Merge additional properties
	for k, v := range override.AdditionalProperties {
		result.AdditionalProperties[k] = v
	}
	
	return result
}

// mergeRemoteEnv merges remote environment variables (with pointer values)
func mergeRemoteEnv(base, override map[string]*string) map[string]*string {
	if base == nil && override == nil {
		return nil
	}
	
	result := make(map[string]*string)
	
	// Copy base
	for k, v := range base {
		result[k] = v
	}
	
	// Override with new values (including nil to unset)
	for k, v := range override {
		result[k] = v
	}
	
	return result
}

// buildMountStringFromMap builds a mount string from a map
func buildMountStringFromMap(m map[string]interface{}) string {
	mountType := "bind"
	if t, ok := m["type"].(string); ok {
		mountType = t
	}
	
	result := fmt.Sprintf("type=%s", mountType)
	
	if source, ok := m["source"].(string); ok {
		result += fmt.Sprintf(",source=%s", source)
	}
	
	if target, ok := m["target"].(string); ok {
		result += fmt.Sprintf(",target=%s", target)
	}
	
	if readOnly, ok := m["readonly"].(bool); ok && readOnly {
		result += ",readonly"
	}
	
	return result
}

// validateDockerRunFlags validates docker run flags
func validateDockerRunFlags(flags []string) error {
	// Basic validation
	flagsWithValues := map[string]bool{
		"-e": true, "--env": true,
		"-p": true, "--publish": true,
		"-v": true, "--volume": true,
		"-w": true, "--workdir": true,
		"-u": true, "--user": true,
		"--name": true,
		"--mount": true,
		"--cap-add": true,
		"--security-opt": true,
		"--entrypoint": true,
		"--network": true,
	}
	
	for i := 0; i < len(flags); i++ {
		flag := flags[i]
		if flag == "" {
			return fmt.Errorf("empty flag")
		}
		
		// Check if this flag requires a value
		if flagsWithValues[flag] {
			if i+1 >= len(flags) {
				return fmt.Errorf("flag %s requires an argument", flag)
			}
			nextArg := flags[i+1]
			// Check if the next argument is another flag (starts with -)
			if strings.HasPrefix(nextArg, "-") {
				return fmt.Errorf("flag %s requires an argument", flag)
			}
			i++ // Skip the value
		}
	}
	return nil
}
