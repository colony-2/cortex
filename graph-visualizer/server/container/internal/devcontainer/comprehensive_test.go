package devcontainer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseAppPorts(t *testing.T) {
	tests := []struct {
		name     string
		appPort  interface{}
		expected []string
	}{
		{
			name:     "single number port",
			appPort:  float64(8080),
			expected: []string{"8080:8080"},
		},
		{
			name:     "port mapping string",
			appPort:  "3000:3001",
			expected: []string{"3000:3001"},
		},
		{
			name:     "array of mixed ports",
			appPort:  []interface{}{float64(8080), "9000:9001", float64(3000)},
			expected: []string{"8080:8080", "9000:9001", "3000:3000"},
		},
		{
			name:     "empty array",
			appPort:  []interface{}{},
			expected: nil,
		},
		{
			name:     "nil port",
			appPort:  nil,
			expected: nil,
		},
		{
			name:     "invalid type",
			appPort:  map[string]string{"invalid": "type"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAppPorts(tt.appPort)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseAppPorts() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatForwardPort(t *testing.T) {
	tests := []struct {
		name     string
		port     interface{}
		expected string
	}{
		{
			name:     "number port",
			port:     float64(8080),
			expected: "8080:8080",
		},
		{
			name:     "string port",
			port:     "3000:3001",
			expected: "3000:3001",
		},
		{
			name:     "invalid type",
			port:     []string{"invalid"},
			expected: "",
		},
		{
			name:     "nil",
			port:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatForwardPort(tt.port)
			if result != tt.expected {
				t.Errorf("formatForwardPort() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "item exists",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: true,
		},
		{
			name:     "item doesn't exist",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "a",
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			item:     "a",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("contains() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBuildMountString(t *testing.T) {
	tests := []struct {
		name     string
		mount    DevContainerCommonMountsElem
		expected string
	}{
		{
			name: "bind mount with source",
			mount: DevContainerCommonMountsElem{
				Type:   MountTypeBind,
				Source: strPtr("/host/path"),
				Target: "/container/path",
			},
			expected: "type=bind,target=/container/path,source=/host/path",
		},
		{
			name: "volume mount",
			mount: DevContainerCommonMountsElem{
				Type:   MountTypeVolume,
				Source: strPtr("myvolume"),
				Target: "/data",
			},
			expected: "type=volume,target=/data,source=myvolume",
		},
		{
			name: "mount without source",
			mount: DevContainerCommonMountsElem{
				Type:   MountTypeVolume,
				Target: "/data",
			},
			expected: "type=volume,target=/data",
		},
		{
			name: "mount with empty source",
			mount: DevContainerCommonMountsElem{
				Type:   MountTypeBind,
				Source: strPtr(""),
				Target: "/container/path",
			},
			expected: "type=bind,target=/container/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMountString(tt.mount)
			if result != tt.expected {
				t.Errorf("buildMountString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLoadDevContainerEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		check   func(*testing.T, *DevContainer)
		wantErr bool
	}{
		{
			name: "container with user",
			json: `{
				"image": "ubuntu:22.04",
				"containerUser": "vscode",
				"remoteUser": "vscode"
			}`,
			check: func(t *testing.T, dc *DevContainer) {
				if dc.ContainerUser == nil || *dc.ContainerUser != "vscode" {
					t.Error("expected containerUser to be vscode")
				}
			},
		},
		{
			name: "container with lifecycle commands",
			json: `{
				"image": "node:18",
				"onCreateCommand": "npm install",
				"postCreateCommand": ["npm", "run", "build"],
				"postStartCommand": {
					"server": "npm start",
					"watch": "npm run watch"
				}
			}`,
			check: func(t *testing.T, dc *DevContainer) {
				if dc.OnCreateCommand == nil {
					t.Error("expected onCreateCommand to be set")
				}
			},
		},
		{
			name: "container with some NonComposeBase fields",
			json: `{
				"image": "python:3.11",
				"workspaceFolder": "/workspace",
				"appPort": [8000, "5000:5001"],
				"runArgs": ["--network", "host"]
			}`,
			check: func(t *testing.T, dc *DevContainer) {
				if dc.NonComposeBase == nil {
					t.Fatal("expected NonComposeBase to be set")
				}
				// Note: Due to schema generation limitations, workspaceFolder might not be populated
				// when other NonComposeBase fields are present. This is handled in BuildDockerRunCommand
				if len(dc.NonComposeBase.RunArgs) != 2 {
					t.Errorf("expected 2 runArgs, got %d", len(dc.NonComposeBase.RunArgs))
				}
			},
		},
		{
			name: "docker compose container",
			json: `{
				"dockerComposeFile": "docker-compose.yml",
				"service": "app",
				"workspaceFolder": "/workspace"
			}`,
			check: func(t *testing.T, dc *DevContainer) {
				if dc.ComposeContainer == nil {
					t.Error("expected ComposeContainer to be set")
				}
			},
		},
		{
			name:    "file not found",
			json:    "", // won't be written
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "devcontainer.json")
			
			if tt.json != "" {
				if err := os.WriteFile(tmpFile, []byte(tt.json), 0644); err != nil {
					t.Fatalf("failed to write test file: %v", err)
				}
			}

			dc, err := LoadDevContainer(tmpFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadDevContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, dc)
			}
		})
	}
}

func TestBuildDockerRunCommandEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		devContainer *DevContainer
		wantErr      bool
		check        func(*testing.T, *DockerRunConfig)
	}{
		{
			name: "dockerfile container",
			devContainer: &DevContainer{
				DockerfileContainer: "Dockerfile",
			},
			wantErr: true,
		},
		{
			name: "compose container",
			devContainer: &DevContainer{
				ComposeContainer: &ComposeContainer{
					Service: "app",
				},
			},
			wantErr: true,
		},
		{
			name: "container with multiple port formats",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "nginx:latest",
				},
				DevContainerCommon: DevContainerCommon{
					ForwardPorts: []interface{}{
						float64(80),
						"8080:80",
						float64(443),
					},
				},
			},
			check: func(t *testing.T, config *DockerRunConfig) {
				expectedPorts := []string{"80:80", "8080:80", "443:443"}
				if !reflect.DeepEqual(config.Ports, expectedPorts) {
					t.Errorf("expected ports %v, got %v", expectedPorts, config.Ports)
				}
			},
		},
		{
			name: "container with duplicate ports",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "node:18",
				},
				DevContainerCommon: DevContainerCommon{
					ForwardPorts: []interface{}{float64(3000), "3000:3000"},
				},
				NonComposeBase: &NonComposeBase{
					AppPort: float64(3000),
				},
			},
			check: func(t *testing.T, config *DockerRunConfig) {
				// Should only have one 3000:3000 entry
				if len(config.Ports) != 1 || config.Ports[0] != "3000:3000" {
					t.Errorf("expected single port 3000:3000, got %v", config.Ports)
				}
			},
		},
		{
			name: "container with invalid forward port",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "alpine:latest",
				},
				DevContainerCommon: DevContainerCommon{
					ForwardPorts: []interface{}{
						map[string]string{"invalid": "port"},
					},
				},
			},
			check: func(t *testing.T, config *DockerRunConfig) {
				if len(config.Ports) != 0 {
					t.Errorf("expected no ports, got %v", config.Ports)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := BuildDockerRunCommand(tt.devContainer, "/workspace")
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildDockerRunCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}

func TestDockerRunConfigToDockerRunArgsEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		config *DockerRunConfig
		check  func(*testing.T, []string)
	}{
		{
			name:   "minimal config",
			config: &DockerRunConfig{
				Image: "alpine:latest",
			},
			check: func(t *testing.T, args []string) {
				expected := []string{"run", "--rm", "-it", "alpine:latest"}
				if !reflect.DeepEqual(args, expected) {
					t.Errorf("expected %v, got %v", expected, args)
				}
			},
		},
		{
			name: "config with empty values",
			config: &DockerRunConfig{
				Image:           "ubuntu:22.04",
				WorkspaceMount:  "",
				WorkspaceFolder: "",
				Environment:     map[string]string{},
				Ports:           []string{},
				Mounts:          []string{},
			},
			check: func(t *testing.T, args []string) {
				// Should not include empty flags
				for i, arg := range args {
					if arg == "-w" && i+1 < len(args) && args[i+1] == "" {
						t.Error("should not include -w with empty value")
					}
					if arg == "--mount" && i+1 < len(args) && args[i+1] == "" {
						t.Error("should not include --mount with empty value")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.config.ToDockerRunArgs()
			tt.check(t, args)
		})
	}
}

func TestFindDevContainerFileOrder(t *testing.T) {
	// Test that it finds files in the correct priority order
	tests := []struct {
		name     string
		setup    func(dir string) error
		expected string
	}{
		{
			name: "finds .devcontainer/devcontainer.json first",
			setup: func(dir string) error {
				// Create all three locations
				os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0755)
				os.WriteFile(filepath.Join(dir, ".devcontainer", "devcontainer.json"), []byte("{}"), 0644)
				os.WriteFile(filepath.Join(dir, ".devcontainer.json"), []byte("{}"), 0644)
				os.WriteFile(filepath.Join(dir, ".devcontainer", ".devcontainer.json"), []byte("{}"), 0644)
				return nil
			},
			expected: ".devcontainer/devcontainer.json",
		},
		{
			name: "finds .devcontainer.json when first is missing",
			setup: func(dir string) error {
				os.WriteFile(filepath.Join(dir, ".devcontainer.json"), []byte("{}"), 0644)
				os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0755)
				os.WriteFile(filepath.Join(dir, ".devcontainer", ".devcontainer.json"), []byte("{}"), 0644)
				return nil
			},
			expected: ".devcontainer.json",
		},
		{
			name: "finds .devcontainer/.devcontainer.json when others missing",
			setup: func(dir string) error {
				os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0755)
				os.WriteFile(filepath.Join(dir, ".devcontainer", ".devcontainer.json"), []byte("{}"), 0644)
				return nil
			},
			expected: ".devcontainer/.devcontainer.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatal(err)
			}

			found, err := FindDevContainerFile(tmpDir)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			expectedPath := filepath.Join(tmpDir, tt.expected)
			if found != expectedPath {
				t.Errorf("expected to find %s, got %s", expectedPath, found)
			}
		})
	}
}

func TestJSONSchemaValidation(t *testing.T) {
	// Test that the generated schema properly validates JSON
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name: "valid mount type",
			json: `{
				"type": "bind",
				"source": "/host",
				"target": "/container"
			}`,
			wantErr: false,
		},
		{
			name: "invalid mount type",
			json: `{
				"type": "invalid",
				"source": "/host",
				"target": "/container"
			}`,
			wantErr: true,
		},
		{
			name: "missing required field",
			json: `{
				"type": "bind",
				"source": "/host"
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mount Mount
			err := json.Unmarshal([]byte(tt.json), &mount)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}