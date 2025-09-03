package devcontainer

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildDockerRunCommand(t *testing.T) {
	tests := []struct {
		name          string
		devContainer  *DevContainer
		workspaceRoot string
		wantErr       bool
		validateFunc  func(*testing.T, *DockerRunConfig)
	}{
		{
			name: "simple image container",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "ubuntu:22.04",
				},
				NonComposeBase: &NonComposeBase{},
			},
			workspaceRoot: "/home/user/myproject",
			wantErr:       false,
			validateFunc: func(t *testing.T, config *DockerRunConfig) {
				if config.Image != "ubuntu:22.04" {
					t.Errorf("expected image ubuntu:22.04, got %s", config.Image)
				}
				if config.WorkspaceFolder != "/workspaces/myproject" {
					t.Errorf("expected workspace folder /workspaces/myproject, got %s", config.WorkspaceFolder)
				}
			},
		},
		{
			name: "container with custom workspace",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "node:18",
				},
				NonComposeBase: &NonComposeBase{
					WorkspaceFolder: strPtr("/app"),
					WorkspaceMount:  strPtr("type=bind,source=/local/path,target=/app"),
				},
			},
			workspaceRoot: "/local/path",
			wantErr:       false,
			validateFunc: func(t *testing.T, config *DockerRunConfig) {
				if config.WorkspaceFolder != "/app" {
					t.Errorf("expected workspace folder /app, got %s", config.WorkspaceFolder)
				}
				if config.WorkspaceMount != "type=bind,source=/local/path,target=/app" {
					t.Errorf("unexpected workspace mount: %s", config.WorkspaceMount)
				}
			},
		},
		{
			name: "container with ports and env",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "python:3.11",
				},
				DevContainerCommon: DevContainerCommon{
					ContainerEnv: map[string]string{
						"FLASK_ENV": "development",
						"PORT":      "5000",
					},
					ForwardPorts: []interface{}{8080.0, "3000:3001"},
				},
				NonComposeBase: &NonComposeBase{
					AppPort: []interface{}{5000.0},
				},
			},
			workspaceRoot: "/workspace",
			wantErr:       false,
			validateFunc: func(t *testing.T, config *DockerRunConfig) {
				expectedPorts := []string{"5000:5000", "8080:8080", "3000:3001"}
				if !reflect.DeepEqual(config.Ports, expectedPorts) {
					t.Errorf("expected ports %v, got %v", expectedPorts, config.Ports)
				}
				if config.Environment["FLASK_ENV"] != "development" {
					t.Errorf("expected FLASK_ENV=development, got %s", config.Environment["FLASK_ENV"])
				}
			},
		},
		{
			name: "container with security options",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "rust:latest",
				},
				DevContainerCommon: DevContainerCommon{
					Privileged:  boolPtr(true),
					Init:        boolPtr(true),
					CapAdd:      []string{"SYS_PTRACE", "NET_ADMIN"},
					SecurityOpt: []string{"seccomp=unconfined"},
				},
				NonComposeBase: &NonComposeBase{
					RunArgs: []string{"--network", "host"},
				},
			},
			workspaceRoot: "/code",
			wantErr:       false,
			validateFunc: func(t *testing.T, config *DockerRunConfig) {
				if !config.Privileged {
					t.Error("expected privileged to be true")
				}
				if !config.Init {
					t.Error("expected init to be true")
				}
				if !reflect.DeepEqual(config.Capabilities, []string{"SYS_PTRACE", "NET_ADMIN"}) {
					t.Errorf("unexpected capabilities: %v", config.Capabilities)
				}
				if !reflect.DeepEqual(config.SecurityOpts, []string{"seccomp=unconfined"}) {
					t.Errorf("unexpected security opts: %v", config.SecurityOpts)
				}
			},
		},
		{
			name: "container with mounts",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "golang:1.21",
				},
				DevContainerCommon: DevContainerCommon{
					Mounts: []interface{}{
						map[string]interface{}{
							"type":   "bind",
							"source": "/host/cache",
							"target": "/go/pkg/mod",
						},
						map[string]interface{}{
							"type":   "volume",
							"source": "go-build-cache",
							"target": "/root/.cache/go-build",
						},
					},
				},
				NonComposeBase: &NonComposeBase{},
			},
			workspaceRoot: "/project",
			wantErr:       false,
			validateFunc: func(t *testing.T, config *DockerRunConfig) {
				expectedMounts := []string{
					"type=bind,source=/host/cache,target=/go/pkg/mod",
					"type=volume,source=go-build-cache,target=/root/.cache/go-build",
				}
				if !reflect.DeepEqual(config.Mounts, expectedMounts) {
					t.Errorf("expected mounts %v, got %v", expectedMounts, config.Mounts)
				}
			},
		},
		{
			name: "no container configuration",
			devContainer: &DevContainer{
				DevContainerCommon: DevContainerCommon{},
			},
			workspaceRoot: "/workspace",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := BuildDockerRunCommand(tt.devContainer, tt.workspaceRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildDockerRunCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validateFunc != nil {
				tt.validateFunc(t, config)
			}
		})
	}
}

func TestDockerRunConfig_ToDockerRunArgs(t *testing.T) {
	config := &DockerRunConfig{
		Image:           "node:18",
		WorkspaceMount:  "type=bind,source=/local,target=/workspace",
		WorkspaceFolder: "/workspace",
		Mounts:          []string{"type=volume,source=cache,target=/cache"},
		Environment: map[string]string{
			"NODE_ENV": "development",
		},
		Ports:        []string{"3000:3000"},
		RunArgs:      []string{"--network", "bridge"},
		Privileged:   true,
		Init:         true,
		User:         "1000:1000",
		Capabilities: []string{"SYS_PTRACE"},
		SecurityOpts: []string{"label=disable"},
	}

	args := config.ToDockerRunArgs()

	// Check that all expected arguments are present
	expectedContains := []string{
		"run", "--rm", "-it",
		"--mount", "type=bind,source=/local,target=/workspace",
		"-w", "/workspace",
		"--mount", "type=volume,source=cache,target=/cache",
		"-e", "NODE_ENV=development",
		"-p", "3000:3000",
		"--privileged",
		"--init",
		"-u", "1000:1000",
		"--cap-add", "SYS_PTRACE",
		"--security-opt", "label=disable",
		"--network", "bridge",
		"node:18",
	}

	argStr := " " + strings.Join(args, " ") + " "
	for _, expected := range expectedContains {
		if !strings.Contains(argStr, " "+expected+" ") {
			t.Errorf("expected args to contain %q, got: %v", expected, args)
		}
	}
}

// Helper functions
func boolPtr(b bool) *bool {
	return &b
}