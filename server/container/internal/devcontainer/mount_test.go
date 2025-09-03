package devcontainer

import (
	"strings"
	"testing"
)

func TestBuildMountStringAdvanced(t *testing.T) {
	tests := []struct {
		name     string
		mount    DevContainerCommonMountsElem
		expected string
	}{
		{
			name: "bind mount with all options",
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
				Source: strPtr("my-volume"),
				Target: "/data",
			},
			expected: "type=volume,target=/data,source=my-volume",
		},
		{
			name: "anonymous volume",
			mount: DevContainerCommonMountsElem{
				Type:   MountTypeVolume,
				Target: "/cache",
			},
			expected: "type=volume,target=/cache",
		},
		{
			name: "tmpfs mount",
			mount: DevContainerCommonMountsElem{
				Type:   "tmpfs",
				Target: "/tmp/cache",
			},
			expected: "type=tmpfs,target=/tmp/cache",
		},
		{
			name: "mount with empty source",
			mount: DevContainerCommonMountsElem{
				Type:   MountTypeBind,
				Source: strPtr(""),
				Target: "/empty",
			},
			expected: "type=bind,target=/empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMountString(tt.mount)
			if result != tt.expected {
				t.Errorf("buildMountString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMountHandlingInDockerCommand(t *testing.T) {
	tests := []struct {
		name         string
		devContainer *DevContainer
		validateCmd  func(*testing.T, []string)
	}{
		{
			name: "multiple mount types",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "alpine:latest",
				},
				DevContainerCommon: DevContainerCommon{
					Mounts: []interface{}{
						map[string]interface{}{
							"type":   "bind",
							"source": "/host/code",
							"target": "/code",
						},
						map[string]interface{}{
							"type":   "volume",
							"source": "cache-vol",
							"target": "/cache",
						},
						map[string]interface{}{
							"type":   "tmpfs",
							"target": "/tmp/scratch",
						},
					},
				},
			},
			validateCmd: func(t *testing.T, args []string) {
				cmdStr := strings.Join(args, " ")
				
				// Check for bind mount
				if !strings.Contains(cmdStr, "--mount type=bind,source=/host/code,target=/code") {
					t.Error("missing bind mount")
				}
				
				// Check for volume mount
				if !strings.Contains(cmdStr, "--mount type=volume,source=cache-vol,target=/cache") {
					t.Error("missing volume mount")
				}
				
				// Check for tmpfs mount
				if !strings.Contains(cmdStr, "--mount type=tmpfs,target=/tmp/scratch") {
					t.Error("missing tmpfs mount")
				}
			},
		},
		{
			name: "mount with workspace mount",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "node:18",
				},
				NonComposeBase: &NonComposeBase{
					WorkspaceMount: strPtr("type=bind,source=/projects/app,target=/workspace"),
				},
				DevContainerCommon: DevContainerCommon{
					Mounts: []interface{}{
						map[string]interface{}{
							"type":   "volume",
							"source": "node_modules",
							"target": "/workspace/node_modules",
						},
					},
				},
			},
			validateCmd: func(t *testing.T, args []string) {
				cmdStr := strings.Join(args, " ")
				
				// Workspace mount should come first
				workspaceIdx := strings.Index(cmdStr, "type=bind,source=/projects/app,target=/workspace")
				additionalIdx := strings.Index(cmdStr, "type=volume,source=node_modules,target=/workspace/node_modules")
				
				if workspaceIdx == -1 {
					t.Error("missing workspace mount")
				}
				if additionalIdx == -1 {
					t.Error("missing additional mount")
				}
				if workspaceIdx > additionalIdx {
					t.Error("workspace mount should come before additional mounts")
				}
			},
		},
		{
			name: "mount order preservation",
			devContainer: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "ubuntu:22.04",
				},
				DevContainerCommon: DevContainerCommon{
					Mounts: []interface{}{
						map[string]interface{}{"type": "volume", "source": "first", "target": "/1"},
						map[string]interface{}{"type": "volume", "source": "second", "target": "/2"},
						map[string]interface{}{"type": "volume", "source": "third", "target": "/3"},
					},
				},
			},
			validateCmd: func(t *testing.T, args []string) {
				// Find mount arguments
				var mounts []string
				for i := 0; i < len(args)-1; i++ {
					if args[i] == "--mount" {
						mounts = append(mounts, args[i+1])
					}
				}
				
				// Should have workspace mount + 3 additional
				if len(mounts) < 3 {
					t.Fatalf("expected at least 3 mounts, got %d", len(mounts))
				}
				
				// Check order is preserved (skip workspace mount)
				expectedOrder := []string{"first", "second", "third"}
				foundOrder := []string{}
				
				for _, mount := range mounts {
					for _, expected := range expectedOrder {
						if strings.Contains(mount, "source="+expected) {
							foundOrder = append(foundOrder, expected)
						}
					}
				}
				
				if len(foundOrder) != 3 {
					t.Errorf("not all mounts found: %v", foundOrder)
				}
				
				for i, expected := range expectedOrder {
					if foundOrder[i] != expected {
						t.Errorf("mount order not preserved: expected %v, got %v", expectedOrder, foundOrder)
						break
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := BuildDockerRunCommand(tt.devContainer, "/tmp/workspace")
			if err != nil {
				t.Fatalf("BuildDockerRunCommand failed: %v", err)
			}
			
			args := config.ToDockerRunArgs()
			tt.validateCmd(t, args)
		})
	}
}

func TestMountValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *DockerRunConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid mounts",
			config: &DockerRunConfig{
				Image: "alpine:latest",
				Mounts: []string{
					"type=bind,source=/host,target=/container",
					"type=volume,source=vol,target=/data",
					"type=tmpfs,target=/tmp",
				},
			},
			wantErr: false,
		},
		{
			name: "mount missing type",
			config: &DockerRunConfig{
				Image: "alpine:latest",
				Mounts: []string{
					"source=/host,target=/container",
				},
			},
			wantErr: true,
			errMsg:  "missing type=",
		},
		{
			name: "mount missing target",
			config: &DockerRunConfig{
				Image: "alpine:latest",
				Mounts: []string{
					"type=bind,source=/host",
				},
			},
			wantErr: true,
			errMsg:  "missing target=",
		},
		{
			name: "complex mount options",
			config: &DockerRunConfig{
				Image: "alpine:latest",
				Mounts: []string{
					"type=bind,source=/host,target=/container,readonly",
					"type=volume,source=data,target=/data,volume-opt=type=nfs",
				},
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

func TestMountExpansion(t *testing.T) {
	dc := &DevContainer{
		DevContainerCommon: DevContainerCommon{
			Mounts: []interface{}{
				map[string]interface{}{
					"type":   "bind",
					"source": "${localWorkspaceFolder}/data",
					"target": "/data",
				},
				map[string]interface{}{
					"type":   "volume",
					"source": "${containerWorkspaceFolderBasename}-cache",
					"target": "${containerWorkspaceFolder}/cache",
				},
			},
		},
		NonComposeBase: &NonComposeBase{
			WorkspaceMount: strPtr("type=bind,source=${localWorkspaceFolder},target=${containerWorkspaceFolder}"),
		},
	}

	variables := map[string]string{
		"localWorkspaceFolder":             "/home/user/myproject",
		"containerWorkspaceFolder":         "/workspace/myproject",
		"containerWorkspaceFolderBasename": "myproject",
	}

	// Expand variables before checking
	ExpandVariables(dc, variables)

	// Check mount expansion
	mount0, ok := dc.Mounts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first mount to be a map")
	}
	if mount0["source"] != "/home/user/myproject/data" {
		t.Errorf("expected first mount source to be expanded, got %v", mount0["source"])
	}
	
	mount1, ok := dc.Mounts[1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected second mount to be a map")
	}
	if mount1["source"] != "myproject-cache" {
		t.Errorf("expected second mount source to be expanded, got %v", mount1["source"])
	}
	if mount1["target"] != "/workspace/myproject/cache" {
		t.Errorf("expected second mount target to be expanded, got %v", mount1["target"])
	}

	// Check workspace mount expansion
	expectedMount := "type=bind,source=/home/user/myproject,target=/workspace/myproject"
	if *dc.NonComposeBase.WorkspaceMount != expectedMount {
		t.Errorf("expected workspace mount to be expanded, got %s", *dc.NonComposeBase.WorkspaceMount)
	}
}