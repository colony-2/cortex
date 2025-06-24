package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MergeOptions controls how devcontainer configurations are merged
type MergeOptions struct {
	// BaseConfigPath is the path to resolve relative extends paths
	BaseConfigPath string
}

// MergeDevContainers merges multiple devcontainer configurations
// Later configurations override earlier ones following the devcontainer spec
func MergeDevContainers(base, override *DevContainer) *DevContainer {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	result := &DevContainer{}

	// Copy base configuration
	*result = *base

	// Override with values from the override config
	// Handle ImageContainer
	if override.ImageContainer != nil {
		result.ImageContainer = override.ImageContainer
	}

	// Handle DockerfileContainer
	if override.DockerfileContainer != nil {
		result.DockerfileContainer = override.DockerfileContainer
	}

	// Handle ComposeContainer
	if override.ComposeContainer != nil {
		result.ComposeContainer = override.ComposeContainer
	}

	// Handle NonComposeBase
	if override.NonComposeBase != nil {
		if result.NonComposeBase == nil {
			result.NonComposeBase = override.NonComposeBase
		} else {
			// Merge NonComposeBase fields
			if override.NonComposeBase.AppPort != nil {
				result.NonComposeBase.AppPort = override.NonComposeBase.AppPort
			}
			if override.NonComposeBase.OverrideCommand != nil {
				result.NonComposeBase.OverrideCommand = override.NonComposeBase.OverrideCommand
			}
			// Always override RunArgs from override (even if empty)
			result.NonComposeBase.RunArgs = override.NonComposeBase.RunArgs
			if override.NonComposeBase.ShutdownAction != nil {
				result.NonComposeBase.ShutdownAction = override.NonComposeBase.ShutdownAction
			}
			if override.NonComposeBase.WorkspaceFolder != nil {
				result.NonComposeBase.WorkspaceFolder = override.NonComposeBase.WorkspaceFolder
			}
			if override.NonComposeBase.WorkspaceMount != nil {
				result.NonComposeBase.WorkspaceMount = override.NonComposeBase.WorkspaceMount
			}
		}
	}

	// Merge DevContainerCommon fields
	result.DevContainerCommon = mergeDevContainerCommon(base.DevContainerCommon, override.DevContainerCommon)

	return result
}

// mergeDevContainerCommon merges common devcontainer fields
func mergeDevContainerCommon(base, override DevContainerCommon) DevContainerCommon {
	result := base

	// Simple overrides
	if override.Schema != nil {
		result.Schema = override.Schema
	}
	if override.Name != nil {
		result.Name = override.Name
	}
	if override.ContainerUser != nil {
		result.ContainerUser = override.ContainerUser
	}
	if override.RemoteUser != nil {
		result.RemoteUser = override.RemoteUser
	}
	if override.Init != nil {
		result.Init = override.Init
	}
	if override.Privileged != nil {
		result.Privileged = override.Privileged
	}
	if override.UpdateRemoteUserUID != nil {
		result.UpdateRemoteUserUID = override.UpdateRemoteUserUID
	}
	if override.UserEnvProbe != nil {
		result.UserEnvProbe = override.UserEnvProbe
	}
	if override.WaitFor != nil {
		result.WaitFor = override.WaitFor
	}

	// Merge arrays (override replaces)
	if len(override.CapAdd) > 0 {
		result.CapAdd = override.CapAdd
	}
	if len(override.SecurityOpt) > 0 {
		result.SecurityOpt = override.SecurityOpt
	}
	if len(override.Mounts) > 0 {
		result.Mounts = override.Mounts
	}
	if len(override.ForwardPorts) > 0 {
		result.ForwardPorts = override.ForwardPorts
	}
	if len(override.OverrideFeatureInstallOrder) > 0 {
		result.OverrideFeatureInstallOrder = override.OverrideFeatureInstallOrder
	}

	// Merge maps
	result.ContainerEnv = mergeStringMaps(base.ContainerEnv, override.ContainerEnv)
	result.RemoteEnv = mergeRemoteEnv(base.RemoteEnv, override.RemoteEnv)
	result.Customizations = mergeMaps(base.Customizations, override.Customizations)
	result.AdditionalProperties = mergeMaps(base.AdditionalProperties, override.AdditionalProperties)

	// Merge features (special handling)
	if override.Features != nil {
		result.Features = mergeFeatures(base.Features, override.Features)
	}

	// Merge port attributes
	if override.PortsAttributes != nil {
		result.PortsAttributes = mergeMaps(base.PortsAttributes, override.PortsAttributes)
	}
	if override.OtherPortsAttributes != nil {
		result.OtherPortsAttributes = override.OtherPortsAttributes
	}

	// Merge lifecycle commands (override replaces)
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

	// Merge secrets
	if override.Secrets != nil {
		result.Secrets = mergeMaps(base.Secrets, override.Secrets)
	}

	// Merge host requirements
	if override.HostRequirements != nil {
		result.HostRequirements = override.HostRequirements
	}

	return result
}

// mergeMaps merges two maps, with values from the second map overriding the first
func mergeMaps(base, override map[string]interface{}) map[string]interface{} {
	if base == nil && override == nil {
		return nil
	}

	result := make(map[string]interface{})
	
	// Copy base values
	for k, v := range base {
		result[k] = v
	}
	
	// Override with new values
	for k, v := range override {
		result[k] = v
	}
	
	return result
}

// mergeStringMaps merges two string maps
func mergeStringMaps(base, override map[string]string) map[string]string {
	if base == nil && override == nil {
		return nil
	}

	result := make(map[string]string)
	
	// Copy base values
	for k, v := range base {
		result[k] = v
	}
	
	// Override with new values
	for k, v := range override {
		result[k] = v
	}
	
	return result
}

// mergeRemoteEnv merges remote environment variables
func mergeRemoteEnv(base, override map[string]*string) map[string]*string {
	if base == nil && override == nil {
		return nil
	}

	result := make(map[string]*string)
	
	// Copy base values
	for k, v := range base {
		result[k] = v
	}
	
	// Override with new values
	for k, v := range override {
		result[k] = v
	}
	
	return result
}

// mergeFeatures merges feature configurations
func mergeFeatures(base, override *DevContainerCommonFeatures) *DevContainerCommonFeatures {
	if override == nil {
		return base
	}
	if base == nil {
		return override
	}

	result := &DevContainerCommonFeatures{}
	
	// Copy base
	*result = *base
	
	// Override specific features
	if override.Fish != nil {
		result.Fish = override.Fish
	}
	if override.Gradle != nil {
		result.Gradle = override.Gradle
	}
	if override.Homebrew != nil {
		result.Homebrew = override.Homebrew
	}
	if override.Jupyterlab != nil {
		result.Jupyterlab = override.Jupyterlab
	}
	if override.Maven != nil {
		result.Maven = override.Maven
	}
	
	// Merge additional properties
	if addProps, ok := override.AdditionalProperties.(map[string]interface{}); ok {
		if baseProps, ok := base.AdditionalProperties.(map[string]interface{}); ok {
			result.AdditionalProperties = mergeMaps(baseProps, addProps)
		} else {
			result.AdditionalProperties = addProps
		}
	}
	
	return result
}

// LoadDevContainerWithExtends loads a devcontainer.json file and processes any extends directives
func LoadDevContainerWithExtends(path string, opts *MergeOptions) (*DevContainer, error) {
	// First, read the raw JSON to check for extends
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read devcontainer.json: %w", err)
	}

	// Parse as generic map to look for extends
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Load the main configuration
	dc, err := LoadDevContainer(path)
	if err != nil {
		return nil, err
	}

	// Check for extends in the raw JSON
	if extends, ok := raw["extends"].(string); ok {
		baseConfig, err := loadExtendedConfig(extends, path, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to load extended config: %w", err)
		}
		
		// Merge with base configuration
		dc = MergeDevContainers(baseConfig, dc)
	}

	return dc, nil
}

// loadExtendedConfig loads a configuration referenced by extends
func loadExtendedConfig(extends string, currentPath string, opts *MergeOptions) (*DevContainer, error) {
	var basePath string
	
	// Handle different extends formats
	if strings.HasPrefix(extends, "file://") {
		// Local file reference
		basePath = strings.TrimPrefix(extends, "file://")
	} else if strings.Contains(extends, "://") {
		// Other URI schemes not supported yet
		return nil, fmt.Errorf("unsupported extends URI: %s", extends)
	} else {
		// Relative path
		basePath = extends
	}

	// Resolve relative paths
	if !filepath.IsAbs(basePath) {
		dir := filepath.Dir(currentPath)
		if opts != nil && opts.BaseConfigPath != "" {
			dir = opts.BaseConfigPath
		}
		basePath = filepath.Join(dir, basePath)
	}

	// Check if it's a directory (need to look for devcontainer.json inside)
	info, err := os.Stat(basePath)
	if err != nil {
		return nil, fmt.Errorf("cannot access extends path %s: %w", basePath, err)
	}

	if info.IsDir() {
		// Look for devcontainer.json in the directory
		possiblePaths := []string{
			filepath.Join(basePath, ".devcontainer", "devcontainer.json"),
			filepath.Join(basePath, ".devcontainer.json"),
			filepath.Join(basePath, "devcontainer.json"),
		}
		
		found := false
		for _, p := range possiblePaths {
			if _, err := os.Stat(p); err == nil {
				basePath = p
				found = true
				break
			}
		}
		
		if !found {
			return nil, fmt.Errorf("no devcontainer.json found in extends directory: %s", basePath)
		}
	}

	// Load the base configuration recursively
	return LoadDevContainerWithExtends(basePath, opts)
}

// ExpandVariables expands variables in devcontainer configuration
func ExpandVariables(dc *DevContainer, variables map[string]string) {
	if dc == nil {
		return
	}

	// Expand in NonComposeBase
	if dc.NonComposeBase != nil {
		if dc.NonComposeBase.WorkspaceMount != nil {
			expanded := expandString(*dc.NonComposeBase.WorkspaceMount, variables)
			dc.NonComposeBase.WorkspaceMount = &expanded
		}
		if dc.NonComposeBase.WorkspaceFolder != nil {
			expanded := expandString(*dc.NonComposeBase.WorkspaceFolder, variables)
			dc.NonComposeBase.WorkspaceFolder = &expanded
		}
		
		// Expand in run args
		for i, arg := range dc.NonComposeBase.RunArgs {
			dc.NonComposeBase.RunArgs[i] = expandString(arg, variables)
		}
	}

	// Expand in mounts
	for i, mount := range dc.Mounts {
		if mount.Source != nil {
			expanded := expandString(*mount.Source, variables)
			dc.Mounts[i].Source = &expanded
		}
		expanded := expandString(mount.Target, variables)
		dc.Mounts[i].Target = expanded
	}

	// Expand in environment variables
	expandedEnv := make(map[string]string)
	for k, v := range dc.ContainerEnv {
		expandedEnv[k] = expandString(v, variables)
	}
	dc.ContainerEnv = expandedEnv

	// Expand in lifecycle commands
	dc.InitializeCommand = expandLifecycleCommand(dc.InitializeCommand, variables)
	dc.OnCreateCommand = expandLifecycleCommand(dc.OnCreateCommand, variables)
	dc.UpdateContentCommand = expandLifecycleCommand(dc.UpdateContentCommand, variables)
	dc.PostCreateCommand = expandLifecycleCommand(dc.PostCreateCommand, variables)
	dc.PostStartCommand = expandLifecycleCommand(dc.PostStartCommand, variables)
	dc.PostAttachCommand = expandLifecycleCommand(dc.PostAttachCommand, variables)
}

// expandString expands variables in a string
func expandString(s string, variables map[string]string) string {
	result := s
	for k, v := range variables {
		result = strings.ReplaceAll(result, "${"+k+"}", v)
	}
	return result
}

// expandLifecycleCommand expands variables in lifecycle commands
func expandLifecycleCommand(cmd interface{}, variables map[string]string) interface{} {
	if cmd == nil {
		return nil
	}

	switch v := cmd.(type) {
	case string:
		return expandString(v, variables)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			if s, ok := item.(string); ok {
				result[i] = expandString(s, variables)
			} else {
				result[i] = item
			}
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			if s, ok := val.(string); ok {
				result[k] = expandString(s, variables)
			} else if arr, ok := val.([]interface{}); ok {
				result[k] = expandLifecycleCommand(arr, variables)
			} else {
				result[k] = val
			}
		}
		return result
	default:
		return cmd
	}
}

// GetStandardVariables returns standard devcontainer variables
func GetStandardVariables(workspaceFolder string) map[string]string {
	return map[string]string{
		"localWorkspaceFolder":       workspaceFolder,
		"localWorkspaceFolderBasename": filepath.Base(workspaceFolder),
		"containerWorkspaceFolder":    "/workspaces/" + filepath.Base(workspaceFolder),
		"containerWorkspaceFolderBasename": filepath.Base(workspaceFolder),
	}
}