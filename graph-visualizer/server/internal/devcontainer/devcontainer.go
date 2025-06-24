package devcontainer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DevContainer represents a parsed devcontainer.json file
type DevContainer struct {
	// Embedded common properties
	DevContainerCommon

	// Image-based container
	*ImageContainer

	// Dockerfile-based container
	DockerfileContainer DockerfileContainer

	// Docker Compose container
	*ComposeContainer

	// Non-compose base properties
	*NonComposeBase
}

// LoadDevContainer reads and parses a devcontainer.json file
func LoadDevContainer(path string) (*DevContainer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read devcontainer.json: %w", err)
	}

	// First unmarshal into a map to check for NonComposeBase fields
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer.json: %w", err)
	}

	var dc DevContainer
	if err := json.Unmarshal(data, &dc); err != nil {
		return nil, fmt.Errorf("failed to parse devcontainer.json: %w", err)
	}

	// If we have NonComposeBase fields but NonComposeBase is nil, initialize it
	hasNonComposeFields := false
	nonComposeFields := []string{"workspaceFolder", "workspaceMount", "appPort", "runArgs", "shutdownAction", "overrideCommand"}
	for _, field := range nonComposeFields {
		if _, ok := raw[field]; ok {
			hasNonComposeFields = true
			break
		}
	}

	if hasNonComposeFields && dc.NonComposeBase == nil {
		dc.NonComposeBase = &NonComposeBase{}
		// Re-unmarshal to populate NonComposeBase
		json.Unmarshal(data, dc.NonComposeBase)
	}

	return &dc, nil
}

// FindDevContainerFile searches for a devcontainer.json file in common locations
func FindDevContainerFile(workspaceRoot string) (string, error) {
	// Common locations for devcontainer.json
	locations := []string{
		filepath.Join(workspaceRoot, ".devcontainer", "devcontainer.json"),
		filepath.Join(workspaceRoot, ".devcontainer.json"),
		filepath.Join(workspaceRoot, ".devcontainer", ".devcontainer.json"),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
	}

	return "", fmt.Errorf("no devcontainer.json file found in workspace")
}