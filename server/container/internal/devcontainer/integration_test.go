// +build integration

package devcontainer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRealContainers tests with actual Docker containers
// Run with: go test -tags=integration ./internal/devcontainer/...
func TestIntegrationRealContainers(t *testing.T) {
	// Skip if docker is not available
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available, skipping integration tests")
	}

	tests := []struct {
		name         string
		devContainer *DevContainer
		validate     func(*testing.T, []string)
	}{
		{
			name: "basic alpine container",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "alpine:latest",
				},
				NonComposeBase: &NonComposeBase{
					WorkspaceFolder: strPtr("/workspace"),
				},
			},
			validate: func(t *testing.T, args []string) {
				// Verify the command structure
				cmdStr := strings.Join(args, " ")
				if !strings.Contains(cmdStr, "alpine:latest") {
					t.Error("missing alpine:latest image")
				}
				if !strings.Contains(cmdStr, "-w /workspace") {
					t.Error("missing workspace folder")
				}
			},
		},
		{
			name: "node development container",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "node:18-alpine",
				},
				DevContainerCommon: DevContainerCommon{
					ContainerEnv: map[string]string{
						"NODE_ENV": "development",
						"PORT":     "3000",
					},
					ForwardPorts: []interface{}{float64(3000), float64(9229)},
					Mounts: []interface{}{
						map[string]interface{}{
							"type":   "volume",
							"source": "node_modules",
							"target": "/workspace/node_modules",
						},
					},
				},
			},
			validate: func(t *testing.T, args []string) {
				cmdStr := strings.Join(args, " ")
				if !strings.Contains(cmdStr, "NODE_ENV=development") {
					t.Error("missing NODE_ENV environment variable")
				}
				if !strings.Contains(cmdStr, "-p 3000:3000") {
					t.Error("missing port 3000 mapping")
				}
				if !strings.Contains(cmdStr, "type=volume") {
					t.Error("missing volume mount")
				}
			},
		},
		{
			name: "secure container with limited capabilities",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "ubuntu:22.04",
				},
				DevContainerCommon: DevContainerCommon{
					Init:         boolPtr(true),
					Privileged:   boolPtr(false),
					CapAdd:       []string{"SYS_PTRACE"},
					SecurityOpt:  []string{"no-new-privileges"},
					ContainerUser: strPtr("1000:1000"),
				},
			},
			validate: func(t *testing.T, args []string) {
				cmdStr := strings.Join(args, " ")
				if !strings.Contains(cmdStr, "--init") {
					t.Error("missing --init flag")
				}
				if strings.Contains(cmdStr, "--privileged") {
					t.Error("should not have --privileged flag")
				}
				if !strings.Contains(cmdStr, "--cap-add SYS_PTRACE") {
					t.Error("missing capability add")
				}
				if !strings.Contains(cmdStr, "--security-opt no-new-privileges") {
					t.Error("missing security option")
				}
				if !strings.Contains(cmdStr, "--user 1000:1000") && !strings.Contains(cmdStr, "-u 1000:1000") {
					t.Error("missing user specification")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the docker run command
			config, err := BuildDockerRunCommand(tt.devContainer, "/tmp/test-workspace")
			if err != nil {
				t.Fatalf("BuildDockerRunCommand failed: %v", err)
			}

			args := config.ToDockerRunArgs()
			
			// Log the command for debugging
			t.Logf("Generated command: docker %s", strings.Join(args, " "))

			// Validate command syntax
			if err := ValidateDockerCommand(args); err != nil {
				t.Errorf("Command validation failed: %v", err)
			}

			// Run custom validations
			if tt.validate != nil {
				tt.validate(t, args)
			}

			// Try a dry run
			if err := DryRunDockerCommand(args); err != nil {
				// Only log, don't fail - image might not be available
				t.Logf("Dry run failed (image might not be available): %v", err)
			}
		})
	}
}

// TestIntegrationWithRealDevContainerFiles tests with actual devcontainer.json files
func TestIntegrationWithRealDevContainerFiles(t *testing.T) {
	// Skip if docker is not available
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available, skipping integration tests")
	}

	testConfigs := []struct {
		name string
		json string
	}{
		{
			name: "Go development container",
			json: `{
				"name": "Go Development",
				"image": "mcr.microsoft.com/devcontainers/go:1.21",
				"features": {
					"ghcr.io/devcontainers/features/git:1": {}
				},
				"forwardPorts": [8080],
				"postCreateCommand": "go mod download",
				"customizations": {
					"vscode": {
						"extensions": ["golang.go"]
					}
				}
			}`,
		},
		{
			name: "Python data science container",
			json: `{
				"name": "Python Data Science",
				"image": "mcr.microsoft.com/devcontainers/python:3.11",
				"features": {
					"ghcr.io/devcontainers/features/python:1": {
						"version": "3.11"
					}
				},
				"forwardPorts": [8888],
				"postCreateCommand": "pip install -r requirements.txt",
				"containerEnv": {
					"PYTHONPATH": "/workspace"
				},
				"mounts": [{
					"type": "volume",
					"source": "jupyter-data",
					"target": "/home/vscode/.jupyter"
				}]
			}`,
		},
		{
			name: "Full-stack web development",
			json: `{
				"name": "Full Stack Web",
				"image": "mcr.microsoft.com/devcontainers/typescript-node:18",
				"forwardPorts": [3000, 5000, 5432],
				"portsAttributes": {
					"3000": {"label": "Frontend"},
					"5000": {"label": "Backend API"},
					"5432": {"label": "PostgreSQL"}
				},
				"postCreateCommand": "npm install && npm run setup",
				"features": {
					"ghcr.io/devcontainers/features/docker-in-docker:2": {}
				}
			}`,
		},
	}

	for _, tc := range testConfigs {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary devcontainer.json
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, ".devcontainer", "devcontainer.json")
			os.MkdirAll(filepath.Dir(configPath), 0755)
			
			if err := os.WriteFile(configPath, []byte(tc.json), 0644); err != nil {
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

			args := config.ToDockerRunArgs()
			
			// Validate the command
			if err := ValidateDockerCommand(args); err != nil {
				t.Errorf("Command validation failed: %v", err)
				t.Logf("Command: docker %s", strings.Join(args, " "))
			}

			// Verify image was extracted correctly
			image, err := ExtractDockerImage(args)
			if err != nil {
				t.Errorf("Failed to extract image: %v", err)
			}
			
			t.Logf("Successfully validated %s with image %s", tc.name, image)
		})
	}
}

// TestIntegrationDockerCommandEquivalence verifies our commands match Docker's expectations
func TestIntegrationDockerCommandEquivalence(t *testing.T) {
	// Skip if docker is not available
	if err := exec.Command("docker", "--version").Run(); err != nil {
		t.Skip("Docker not available, skipping integration tests")
	}

	// Test that our generated commands are equivalent to hand-written ones
	testCases := []struct {
		name           string
		devContainer   *DevContainer
		expectedArgs   []string // Key arguments that should be present
		forbiddenArgs  []string // Arguments that should NOT be present
	}{
		{
			name: "workspace mount handling",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "alpine:latest",
				},
				NonComposeBase: &NonComposeBase{
					WorkspaceFolder: strPtr("/app"),
					WorkspaceMount:  strPtr("type=bind,source=/host/project,target=/app"),
				},
			},
			expectedArgs: []string{
				"-v", "type=bind,source=/host/project,target=/app",
				"-w", "/app",
			},
		},
		{
			name: "environment variables escaping",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "alpine:latest",
				},
				DevContainerCommon: DevContainerCommon{
					ContainerEnv: map[string]string{
						"PATH":         "/custom/bin:$PATH",
						"QUOTED_VALUE": `"value with quotes"`,
						"MULTI_LINE":   "line1\nline2",
					},
				},
			},
			expectedArgs: []string{
				"-e", "PATH=/custom/bin:$PATH",
				"-e", `QUOTED_VALUE="value with quotes"`,
				"-e", "MULTI_LINE=line1\nline2",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := BuildDockerRunCommand(tc.devContainer, "/workspace")
			if err != nil {
				t.Fatal(err)
			}

			args := config.ToDockerRunArgs()
			argsStr := " " + strings.Join(args, " ") + " "

			// Check expected arguments
			for i := 0; i < len(tc.expectedArgs); i += 2 {
				flag := tc.expectedArgs[i]
				value := tc.expectedArgs[i+1]
				expected := fmt.Sprintf(" %s %s ", flag, value)
				if !strings.Contains(argsStr, expected) {
					t.Errorf("Expected to find %q in command", expected)
					t.Logf("Full command: docker %s", strings.Join(args, " "))
				}
			}

			// Check forbidden arguments
			for _, forbidden := range tc.forbiddenArgs {
				if strings.Contains(argsStr, " "+forbidden+" ") {
					t.Errorf("Found forbidden argument %q in command", forbidden)
				}
			}
		})
	}
}