package devcontainer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealDockerExecution tests with actual Docker execution
// This test requires Docker to be installed and running
func TestRealDockerExecution(t *testing.T) {
	// Skip if docker is not available
	if err := checkDockerAvailable(); err != nil {
		t.Fatal("Docker is required for this test")
	}

	// Create a real temporary workspace
	tmpDir := t.TempDir()
	
	// Create some test files
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("Hello from devcontainer"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		devContainer *DevContainer
		validate     func(*testing.T, string) // Validate with actual workspace path
	}{
		{
			name: "alpine with real workspace mount",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "alpine:latest",
				},
				NonComposeBase: &NonComposeBase{
					WorkspaceFolder: strPtr("/workspace"),
				},
			},
			validate: func(t *testing.T, workspace string) {
				// The mount should reference the real workspace
				if !strings.Contains(workspace, tmpDir) {
					t.Errorf("expected workspace mount to contain %s", tmpDir)
				}
			},
		},
		{
			name: "ubuntu with volume and bind mounts",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "ubuntu:22.04",
				},
				DevContainerCommon: DevContainerCommon{
					Mounts: []interface{}{
						map[string]interface{}{
							"type":   "volume",
							"source": "test-cache",
							"target": "/cache",
						},
						map[string]interface{}{
							"type":   "bind",
							"source": tmpDir,
							"target": "/data",
						},
					},
				},
			},
			validate: func(t *testing.T, workspace string) {
				// Should have both volume and bind mounts
				if !strings.Contains(workspace, "type=volume") {
					t.Error("missing volume mount")
				}
				if !strings.Contains(workspace, tmpDir) {
					t.Error("missing bind mount with real path")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the docker run command with real workspace
			config, err := BuildDockerRunCommand(tt.devContainer, tmpDir)
			if err != nil {
				t.Fatalf("BuildDockerRunCommand failed: %v", err)
			}

			// Validate the configuration
			if err := config.Validate(); err != nil {
				t.Fatalf("Config validation failed: %v", err)
			}

			args := config.ToDockerRunArgs()
			cmdStr := strings.Join(args, " ")
			
			t.Logf("Generated command: docker %s", cmdStr)

			// Validate command syntax
			if err := ValidateDockerCommand(args); err != nil {
				t.Errorf("Command validation failed: %v", err)
			}

			// Custom validation
			if tt.validate != nil {
				tt.validate(t, cmdStr)
			}

			// Try a real dry run with the actual workspace
			if err := DryRunDockerCommand(args); err != nil {
				t.Errorf("Dry run failed: %v", err)
			}
		})
	}
}

// TestDockerImagePulling tests with common development images
func TestDockerImagePulling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping image pulling test in short mode")
	}

	// Skip if docker is not available
	if err := checkDockerAvailable(); err != nil {
		t.Fatal("Docker is required for this test")
	}

	// Test with lightweight images that should pull quickly
	images := []string{
		"alpine:latest",
		"busybox:latest",
		"hello-world:latest",
	}

	tmpDir := t.TempDir()

	for _, image := range images {
		t.Run(image, func(t *testing.T) {
			dc := &DevContainer{
				ImageContainer: &ImageContainer{
					Image: image,
				},
			}

			config, err := BuildDockerRunCommand(dc, tmpDir)
			if err != nil {
				t.Fatal(err)
			}

			args := config.ToDockerRunArgs()

			// This should work if the image can be pulled
			if err := DryRunDockerCommand(args); err != nil {
				if strings.Contains(err.Error(), "pull access denied") {
					t.Skip("Cannot pull image, skipping")
				}
				// hello-world is a special case - it doesn't have echo
				if strings.Contains(err.Error(), "executable file not found") && image == "hello-world:latest" {
					t.Logf("Expected error for hello-world (no echo command): %v", err)
				} else {
					t.Errorf("Dry run failed for %s: %v", image, err)
				}
			}
		})
	}
}

// TestComplexDevContainerScenarios tests more complex real-world scenarios
func TestComplexDevContainerScenarios(t *testing.T) {
	// Skip if docker is not available
	if err := checkDockerAvailable(); err != nil {
		t.Fatal("Docker is required for this test")
	}

	tmpDir := t.TempDir()

	// Create a devcontainer.json that mimics a real development setup
	devcontainerJSON := `{
		"name": "Test Development Container",
		"image": "alpine:latest",
		"workspaceFolder": "/workspace",
		"workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind",
		"mounts": [
			{
				"source": "test-vol",
				"target": "/cache",
				"type": "volume"
			}
		],
		"containerEnv": {
			"DEVELOPMENT": "true",
			"PORT": "8080"
		},
		"forwardPorts": [8080, 3000],
		"postCreateCommand": "echo 'Container ready'",
		"remoteUser": "root"
	}`

	configPath := filepath.Join(tmpDir, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(devcontainerJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Load the configuration
	dc, err := LoadDevContainer(configPath)
	if err != nil {
		t.Fatalf("Failed to load devcontainer.json: %v", err)
	}

	// Build docker command
	config, err := BuildDockerRunCommand(dc, tmpDir)
	if err != nil {
		t.Fatalf("Failed to build docker command: %v", err)
	}

	// Validate
	if err := config.Validate(); err != nil {
		t.Fatalf("Config validation failed: %v", err)
	}

	args := config.ToDockerRunArgs()

	// Check that workspace mount was properly expanded
	cmdStr := strings.Join(args, " ")
	if strings.Contains(cmdStr, "${localWorkspaceFolder}") {
		t.Error("Workspace folder variable was not expanded")
	}
	if !strings.Contains(cmdStr, tmpDir) {
		t.Error("Workspace mount does not contain actual workspace path")
	}

	// Validate the final command
	if err := ValidateDockerCommand(args); err != nil {
		t.Errorf("Command validation failed: %v", err)
	}

	// Dry run
	if err := DryRunDockerCommand(args); err != nil {
		t.Logf("Dry run error (may be expected): %v", err)
	}
}