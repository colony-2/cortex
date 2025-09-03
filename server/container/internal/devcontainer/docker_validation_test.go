package devcontainer

import (
	"strings"
	"testing"
)

func TestDockerRunConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *DockerRunConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &DockerRunConfig{
				Image:           "alpine:latest",
				WorkspaceMount:  "type=bind,source=/host,target=/container",
				Ports:           []string{"8080:80", "3000"},
				Mounts:          []string{"type=volume,source=data,target=/data"},
			},
			wantErr: false,
		},
		{
			name: "missing image",
			config: &DockerRunConfig{
				WorkspaceMount: "type=bind,source=/host,target=/container",
			},
			wantErr: true,
			errMsg:  "image is required",
		},
		{
			name: "invalid mount - missing type",
			config: &DockerRunConfig{
				Image:  "ubuntu:22.04",
				Mounts: []string{"source=/host,target=/container"},
			},
			wantErr: true,
			errMsg:  "missing type=",
		},
		{
			name: "invalid mount - missing target",
			config: &DockerRunConfig{
				Image:  "ubuntu:22.04",
				Mounts: []string{"type=bind,source=/host"},
			},
			wantErr: true,
			errMsg:  "missing target=",
		},
		{
			name: "invalid port format",
			config: &DockerRunConfig{
				Image: "nginx:latest",
				Ports: []string{"8080:80:tcp"},
			},
			wantErr: true,
			errMsg:  "invalid port format",
		},
		{
			name: "multiple valid ports",
			config: &DockerRunConfig{
				Image: "node:18",
				Ports: []string{"3000", "8080:8080", "9229:9229"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}

func TestBuildDockerRunCommandValidation(t *testing.T) {
	// Test that BuildDockerRunCommand produces valid configs
	testCases := []struct {
		name         string
		devContainer *DevContainer
		shouldError  bool
	}{
		{
			name: "valid image container",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "golang:1.21",
				},
			},
			shouldError: false,
		},
		{
			name: "complex valid container",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "mcr.microsoft.com/devcontainers/python:3.11",
				},
				DevContainerCommon: DevContainerCommon{
					ContainerEnv: map[string]string{
						"PYTHONPATH": "/app",
					},
					ForwardPorts: []interface{}{float64(8000), "5432:5432"},
					Mounts: []interface{}{
						map[string]interface{}{
							"type":   "volume",
							"source": "pip-cache",
							"target": "/root/.cache/pip",
						},
						map[string]interface{}{
							"type":   "bind",
							"source": "/host/data",
							"target": "/container/data",
						},
					},
				},
			},
			shouldError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := BuildDockerRunCommand(tc.devContainer, "/workspace")
			if tc.shouldError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Validate the generated config
			if err := config.Validate(); err != nil {
				t.Errorf("generated config failed validation: %v", err)
			}

			// Also validate the generated command (skip if Docker not available)
			args := config.ToDockerRunArgs()
			if err := ValidateDockerCommand(args); err != nil {
				// Check if it's just Docker not being available
				if strings.Contains(err.Error(), "docker not available") {
					t.Skip("Docker not available, skipping command validation")
				}
				t.Errorf("generated command failed validation: %v", err)
			}
		})
	}
}