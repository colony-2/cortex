package devcontainer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMergeDevContainers(t *testing.T) {
	tests := []struct {
		name     string
		base     *DevContainer
		override *DevContainer
		validate func(*testing.T, *DevContainer)
	}{
		{
			name: "nil base returns override",
			base: nil,
			override: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "alpine:latest",
				},
			},
			validate: func(t *testing.T, result *DevContainer) {
				if result.ImageContainer == nil || result.ImageContainer.Image != "alpine:latest" {
					t.Error("expected override to be returned")
				}
			},
		},
		{
			name: "nil override returns base",
			base: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "ubuntu:22.04",
				},
			},
			override: nil,
			validate: func(t *testing.T, result *DevContainer) {
				if result.ImageContainer == nil || result.ImageContainer.Image != "ubuntu:22.04" {
					t.Error("expected base to be returned")
				}
			},
		},
		{
			name: "override image",
			base: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "node:16",
				},
			},
			override: &DevContainer{
				ImageContainer: &ImageContainer{
					Image: "node:18",
				},
			},
			validate: func(t *testing.T, result *DevContainer) {
				if result.ImageContainer.Image != "node:18" {
					t.Errorf("expected image to be overridden, got %s", result.ImageContainer.Image)
				}
			},
		},
		{
			name: "merge environment variables",
			base: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					ContainerEnv: map[string]string{
						"FOO": "bar",
						"BAZ": "qux",
					},
				},
			},
			override: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					ContainerEnv: map[string]string{
						"FOO":   "overridden",
						"HELLO": "world",
					},
				},
			},
			validate: func(t *testing.T, result *DevContainer) {
				expected := map[string]string{
					"FOO":   "overridden",
					"BAZ":   "qux",
					"HELLO": "world",
				}
				if !reflect.DeepEqual(result.ContainerEnv, expected) {
					t.Errorf("expected env %v, got %v", expected, result.ContainerEnv)
				}
			},
		},
		{
			name: "merge arrays (override replaces)",
			base: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					CapAdd:       []string{"SYS_PTRACE"},
					ForwardPorts: []interface{}{float64(8080)},
				},
			},
			override: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					CapAdd:       []string{"NET_ADMIN", "SYS_TIME"},
					ForwardPorts: []interface{}{float64(3000), "5000:5000"},
				},
			},
			validate: func(t *testing.T, result *DevContainer) {
				expectedCaps := []string{"NET_ADMIN", "SYS_TIME"}
				if !reflect.DeepEqual(result.CapAdd, expectedCaps) {
					t.Errorf("expected caps %v, got %v", expectedCaps, result.CapAdd)
				}
				
				expectedPorts := []interface{}{float64(3000), "5000:5000"}
				if !reflect.DeepEqual(result.ForwardPorts, expectedPorts) {
					t.Errorf("expected ports %v, got %v", expectedPorts, result.ForwardPorts)
				}
			},
		},
		{
			name: "merge NonComposeBase fields",
			base: &DevContainer{
				NonComposeBase: &NonComposeBase{
					WorkspaceFolder: strPtr("/workspace"),
					RunArgs:         []string{"--network", "bridge"},
				},
			},
			override: &DevContainer{
				NonComposeBase: &NonComposeBase{
					WorkspaceFolder: strPtr("/app"),
					AppPort:         []interface{}{float64(8080)},
				},
			},
			validate: func(t *testing.T, result *DevContainer) {
				if result.NonComposeBase == nil {
					t.Fatal("expected NonComposeBase to be set")
				}
				if *result.NonComposeBase.WorkspaceFolder != "/app" {
					t.Error("expected workspace folder to be overridden")
				}
				if result.NonComposeBase.AppPort == nil {
					t.Error("expected app port to be set")
				}
				// RunArgs should be overridden (empty in override)
				if len(result.NonComposeBase.RunArgs) != 0 {
					t.Error("expected run args to be empty")
				}
			},
		},
		{
			name: "merge lifecycle commands",
			base: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					OnCreateCommand:   "echo 'base'",
					PostCreateCommand: []interface{}{"npm", "install"},
				},
			},
			override: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					OnCreateCommand:  "echo 'override'",
					PostStartCommand: "npm start",
				},
			},
			validate: func(t *testing.T, result *DevContainer) {
				if result.OnCreateCommand != "echo 'override'" {
					t.Error("expected onCreate command to be overridden")
				}
				// PostCreateCommand should remain from base
				if result.PostCreateCommand == nil {
					t.Error("expected postCreate command to be preserved")
				}
				if result.PostStartCommand != "npm start" {
					t.Error("expected postStart command to be set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeDevContainers(tt.base, tt.override)
			tt.validate(t, result)
		})
	}
}

func TestExpandVariables(t *testing.T) {
	tests := []struct {
		name      string
		dc        *DevContainer
		variables map[string]string
		validate  func(*testing.T, *DevContainer)
	}{
		{
			name: "expand workspace mount",
			dc: &DevContainer{
				NonComposeBase: &NonComposeBase{
					WorkspaceMount:  strPtr("source=${localWorkspaceFolder},target=/workspace,type=bind"),
					WorkspaceFolder: strPtr("/workspace/${containerWorkspaceFolderBasename}"),
				},
			},
			variables: map[string]string{
				"localWorkspaceFolder":              "/home/user/project",
				"containerWorkspaceFolderBasename":  "project",
			},
			validate: func(t *testing.T, dc *DevContainer) {
				expected := "source=/home/user/project,target=/workspace,type=bind"
				if *dc.NonComposeBase.WorkspaceMount != expected {
					t.Errorf("expected mount %s, got %s", expected, *dc.NonComposeBase.WorkspaceMount)
				}
				
				expectedFolder := "/workspace/project"
				if *dc.NonComposeBase.WorkspaceFolder != expectedFolder {
					t.Errorf("expected folder %s, got %s", expectedFolder, *dc.NonComposeBase.WorkspaceFolder)
				}
			},
		},
		{
			name: "expand environment variables",
			dc: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					ContainerEnv: map[string]string{
						"PROJECT_PATH": "${containerWorkspaceFolder}",
						"PROJECT_NAME": "${containerWorkspaceFolderBasename}",
						"LITERAL":      "no-expansion",
					},
				},
			},
			variables: map[string]string{
				"containerWorkspaceFolder":         "/workspaces/myapp",
				"containerWorkspaceFolderBasename": "myapp",
			},
			validate: func(t *testing.T, dc *DevContainer) {
				expected := map[string]string{
					"PROJECT_PATH": "/workspaces/myapp",
					"PROJECT_NAME": "myapp",
					"LITERAL":      "no-expansion",
				}
				if !reflect.DeepEqual(dc.ContainerEnv, expected) {
					t.Errorf("expected env %v, got %v", expected, dc.ContainerEnv)
				}
			},
		},
		{
			name: "expand lifecycle commands",
			dc: &DevContainer{
				DevContainerCommon: DevContainerCommon{
					OnCreateCommand: "cd ${localWorkspaceFolder} && npm install",
					PostCreateCommand: []interface{}{
						"echo",
						"Working in ${containerWorkspaceFolder}",
					},
					PostStartCommand: map[string]interface{}{
						"server": "cd ${containerWorkspaceFolder} && npm start",
						"watch":  []interface{}{"npm", "run", "watch", "--prefix", "${containerWorkspaceFolder}"},
					},
				},
			},
			variables: map[string]string{
				"localWorkspaceFolder":     "/home/user/app",
				"containerWorkspaceFolder": "/workspace/app",
			},
			validate: func(t *testing.T, dc *DevContainer) {
				expectedOnCreate := "cd /home/user/app && npm install"
				if dc.OnCreateCommand != expectedOnCreate {
					t.Errorf("expected onCreate %s, got %v", expectedOnCreate, dc.OnCreateCommand)
				}
				
				if cmd, ok := dc.PostCreateCommand.([]interface{}); ok {
					if len(cmd) != 2 || cmd[1] != "Working in /workspace/app" {
						t.Errorf("unexpected postCreate command: %v", cmd)
					}
				}
				
				if cmds, ok := dc.PostStartCommand.(map[string]interface{}); ok {
					if server, ok := cmds["server"].(string); ok {
						expected := "cd /workspace/app && npm start"
						if server != expected {
							t.Errorf("expected server command %s, got %s", expected, server)
						}
					}
				}
			},
		},
		{
			name: "expand mount sources",
			dc: &DevContainer{
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
							"target": "/cache",
						},
					},
				},
			},
			variables: map[string]string{
				"localWorkspaceFolder":             "/projects/app",
				"containerWorkspaceFolderBasename": "app",
			},
			validate: func(t *testing.T, dc *DevContainer) {
				if len(dc.Mounts) != 2 {
					t.Fatalf("expected 2 mounts, got %d", len(dc.Mounts))
				}
				
				// Check first mount (bind)
				mount0, ok := dc.Mounts[0].(map[string]interface{})
				if !ok {
					t.Fatalf("expected first mount to be a map")
				}
				if mount0["source"] != "/projects/app/data" {
					t.Errorf("expected first mount source to be expanded, got %v", mount0["source"])
				}
				
				// Check second mount (volume)
				mount1, ok := dc.Mounts[1].(map[string]interface{})
				if !ok {
					t.Fatalf("expected second mount to be a map")
				}
				if mount1["source"] != "app-cache" {
					t.Errorf("expected second mount source to be expanded, got %v", mount1["source"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ExpandVariables(tt.dc, tt.variables)
			tt.validate(t, tt.dc)
		})
	}
}

func TestLoadDevContainerWithExtends(t *testing.T) {
	// Create test directory structure
	tmpDir := t.TempDir()
	
	// Create base configuration
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(filepath.Join(baseDir, ".devcontainer"), 0755)
	
	baseConfig := `{
		"image": "ubuntu:22.04",
		"containerEnv": {
			"BASE_VAR": "base_value",
			"OVERRIDE_ME": "base"
		},
		"forwardPorts": [8080],
		"capAdd": ["SYS_PTRACE"]
	}`
	
	baseConfigPath := filepath.Join(baseDir, ".devcontainer", "devcontainer.json")
	os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	
	// Create extending configuration
	extendingConfig := `{
		"extends": "../base",
		"containerEnv": {
			"OVERRIDE_ME": "overridden",
			"NEW_VAR": "new_value"
		},
		"forwardPorts": [3000, 5000]
	}`
	
	projectDir := filepath.Join(tmpDir, "project")
	os.MkdirAll(projectDir, 0755)
	projectConfigPath := filepath.Join(projectDir, "devcontainer.json")
	os.WriteFile(projectConfigPath, []byte(extendingConfig), 0644)
	
	// Test loading with extends
	t.Run("load with extends", func(t *testing.T) {
		dc, err := LoadDevContainerWithExtends(projectConfigPath, nil)
		if err != nil {
			t.Fatalf("failed to load with extends: %v", err)
		}
		
		// Check merged result
		if dc.ImageContainer == nil || dc.ImageContainer.Image != "ubuntu:22.04" {
			t.Error("expected base image to be inherited")
		}
		
		// Check environment merge
		expectedEnv := map[string]string{
			"BASE_VAR":    "base_value",
			"OVERRIDE_ME": "overridden",
			"NEW_VAR":     "new_value",
		}
		if !reflect.DeepEqual(dc.ContainerEnv, expectedEnv) {
			t.Errorf("expected env %v, got %v", expectedEnv, dc.ContainerEnv)
		}
		
		// Check arrays are replaced
		expectedPorts := []interface{}{float64(3000), float64(5000)}
		if !reflect.DeepEqual(dc.ForwardPorts, expectedPorts) {
			t.Errorf("expected ports %v, got %v", expectedPorts, dc.ForwardPorts)
		}
		
		// Check base-only values are preserved
		expectedCaps := []string{"SYS_PTRACE"}
		if !reflect.DeepEqual(dc.CapAdd, expectedCaps) {
			t.Errorf("expected caps %v, got %v", expectedCaps, dc.CapAdd)
		}
	})
	
	// Test with file:// prefix
	t.Run("load with file:// extends", func(t *testing.T) {
		fileExtendConfig := `{
			"extends": "file://` + baseDir + `",
			"name": "My Project"
		}`
		
		fileConfigPath := filepath.Join(projectDir, "file-extend.json")
		os.WriteFile(fileConfigPath, []byte(fileExtendConfig), 0644)
		
		dc, err := LoadDevContainerWithExtends(fileConfigPath, nil)
		if err != nil {
			t.Fatalf("failed to load with file:// extends: %v", err)
		}
		
		if dc.ImageContainer == nil || dc.ImageContainer.Image != "ubuntu:22.04" {
			t.Error("expected base image to be inherited")
		}
		
		if dc.Name == nil || *dc.Name != "My Project" {
			t.Error("expected name to be set")
		}
	})
	
	// Test nested extends
	t.Run("nested extends", func(t *testing.T) {
		// Create middle configuration that extends base
		middleConfig := `{
			"extends": "../base",
			"containerEnv": {
				"MIDDLE_VAR": "middle"
			},
			"securityOpt": ["no-new-privileges"]
		}`
		
		middleDir := filepath.Join(tmpDir, "middle")
		os.MkdirAll(middleDir, 0755)
		middleConfigPath := filepath.Join(middleDir, "devcontainer.json")
		os.WriteFile(middleConfigPath, []byte(middleConfig), 0644)
		
		// Create final configuration that extends middle
		finalConfig := `{
			"extends": "../middle/devcontainer.json",
			"containerEnv": {
				"FINAL_VAR": "final"
			}
		}`
		
		finalDir := filepath.Join(tmpDir, "final")
		os.MkdirAll(finalDir, 0755)
		finalConfigPath := filepath.Join(finalDir, "devcontainer.json")
		os.WriteFile(finalConfigPath, []byte(finalConfig), 0644)
		
		dc, err := LoadDevContainerWithExtends(finalConfigPath, nil)
		if err != nil {
			t.Fatalf("failed to load nested extends: %v", err)
		}
		
		// Should have values from all levels
		expectedEnv := map[string]string{
			"BASE_VAR":    "base_value",
			"OVERRIDE_ME": "base",
			"MIDDLE_VAR":  "middle",
			"FINAL_VAR":   "final",
		}
		if !reflect.DeepEqual(dc.ContainerEnv, expectedEnv) {
			t.Errorf("expected env %v, got %v", expectedEnv, dc.ContainerEnv)
		}
		
		if !reflect.DeepEqual(dc.SecurityOpt, []string{"no-new-privileges"}) {
			t.Error("expected security options from middle config")
		}
	})
}

func TestGetStandardVariables(t *testing.T) {
	workspace := "/home/user/my-project"
	vars := GetStandardVariables(workspace)
	
	expected := map[string]string{
		"localWorkspaceFolder":              "/home/user/my-project",
		"localWorkspaceFolderBasename":      "my-project",
		"containerWorkspaceFolder":          "/workspaces/my-project",
		"containerWorkspaceFolderBasename":  "my-project",
	}
	
	if !reflect.DeepEqual(vars, expected) {
		t.Errorf("expected vars %v, got %v", expected, vars)
	}
}

func TestMergeFeatures(t *testing.T) {
	base := &DevContainerCommonFeatures{
		Fish:     "v1",
		Gradle:   "v2",
		AdditionalProperties: map[string]interface{}{
			"ghcr.io/devcontainers/features/go:1": map[string]interface{}{
				"version": "1.20",
			},
		},
	}
	
	override := &DevContainerCommonFeatures{
		Fish:   "v2", // Override
		Maven:  "v1", // New
		AdditionalProperties: map[string]interface{}{
			"ghcr.io/devcontainers/features/go:1": map[string]interface{}{
				"version": "1.21", // Override version
			},
			"ghcr.io/devcontainers/features/node:1": map[string]interface{}{
				"version": "18",
			},
		},
	}
	
	result := mergeFeatures(base, override)
	
	if result.Fish != "v2" {
		t.Error("expected Fish to be overridden")
	}
	if result.Gradle != "v2" {
		t.Error("expected Gradle to be preserved")
	}
	if result.Maven != "v1" {
		t.Error("expected Maven to be added")
	}
	
	// Check additional properties merge
	props := result.AdditionalProperties
	if props == nil {
		t.Fatal("expected additional properties to not be nil")
	}
	
	goFeature, ok := props["ghcr.io/devcontainers/features/go:1"].(map[string]interface{})
	if !ok || goFeature["version"] != "1.21" {
		t.Error("expected Go feature version to be overridden")
	}
	
	nodeFeature, ok := props["ghcr.io/devcontainers/features/node:1"].(map[string]interface{})
	if !ok || nodeFeature["version"] != "18" {
		t.Error("expected Node feature to be added")
	}
}

func TestMergeRemoteEnv(t *testing.T) {
	str1 := "value1"
	str2 := "value2"
	str3 := "value3"
	
	base := map[string]*string{
		"VAR1": &str1,
		"VAR2": &str2,
	}
	
	override := map[string]*string{
		"VAR2": &str3, // Override
		"VAR3": nil,   // Unset
	}
	
	result := mergeRemoteEnv(base, override)
	
	if result["VAR1"] != &str1 {
		t.Error("expected VAR1 to be preserved")
	}
	if result["VAR2"] != &str3 {
		t.Error("expected VAR2 to be overridden")
	}
	if result["VAR3"] != nil {
		t.Error("expected VAR3 to be nil")
	}
}