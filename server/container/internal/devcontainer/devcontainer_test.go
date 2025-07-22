package devcontainer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDevContainer(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name: "simple image container",
			json: `{
				"image": "mcr.microsoft.com/devcontainers/go:1.21",
				"workspaceFolder": "/workspaces/myproject"
			}`,
			wantErr: false,
		},
		{
			name: "container with features",
			json: `{
				"image": "ubuntu:22.04",
				"features": {
					"ghcr.io/devcontainers/features/go:1": {
						"version": "1.21"
					}
				},
				"forwardPorts": [8080, "3000:3000"],
				"containerEnv": {
					"MY_VAR": "value"
				}
			}`,
			wantErr: false,
		},
		{
			name: "container with mounts",
			json: `{
				"image": "node:18",
				"mounts": [
					{
						"type": "bind",
						"source": "/host/path",
						"target": "/container/path"
					},
					{
						"type": "volume",
						"source": "myvolume",
						"target": "/data"
					}
				],
				"capAdd": ["SYS_PTRACE"],
				"securityOpt": ["seccomp=unconfined"]
			}`,
			wantErr: false,
		},
		{
			name:    "invalid json",
			json:    `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "devcontainer.json")
			if err := os.WriteFile(tmpFile, []byte(tt.json), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			// Test LoadDevContainer
			dc, err := LoadDevContainer(tmpFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadDevContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && dc == nil {
				t.Error("LoadDevContainer() returned nil without error")
			}
		})
	}
}

func TestFindDevContainerFile(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(dir string) error
		wantFile string
		wantErr  bool
	}{
		{
			name: ".devcontainer/devcontainer.json",
			setup: func(dir string) error {
				devDir := filepath.Join(dir, ".devcontainer")
				if err := os.Mkdir(devDir, 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(devDir, "devcontainer.json"), []byte("{}"), 0644)
			},
			wantFile: ".devcontainer/devcontainer.json",
			wantErr:  false,
		},
		{
			name: ".devcontainer.json in root",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, ".devcontainer.json"), []byte("{}"), 0644)
			},
			wantFile: ".devcontainer.json",
			wantErr:  false,
		},
		{
			name: "no devcontainer file",
			setup: func(dir string) error {
				return nil
			},
			wantFile: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			
			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			got, err := FindDevContainerFile(tmpDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindDevContainerFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				var expectedPath string
				if tt.wantFile != "" {
					expectedPath = filepath.Join(tmpDir, tt.wantFile)
				}
				if got != expectedPath {
					t.Errorf("FindDevContainerFile() = %v, want %v", got, expectedPath)
				}
			}
		})
	}
}