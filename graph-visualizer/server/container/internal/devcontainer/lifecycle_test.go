package devcontainer

import (
	"strings"
	"testing"
)

func TestParseLifecycleCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		validate func(*testing.T, *LifecycleCommand)
		wantErr  bool
	}{
		{
			name:  "string command",
			input: "npm install",
			validate: func(t *testing.T, cmd *LifecycleCommand) {
				if cmd.Type != "string" {
					t.Errorf("expected type string, got %s", cmd.Type)
				}
				if cmd.Command != "npm install" {
					t.Errorf("expected command 'npm install', got %s", cmd.Command)
				}
			},
		},
		{
			name:  "array command",
			input: []interface{}{"npm", "run", "build"},
			validate: func(t *testing.T, cmd *LifecycleCommand) {
				if cmd.Type != "array" {
					t.Errorf("expected type array, got %s", cmd.Type)
				}
				expected := []string{"npm", "run", "build"}
				if len(cmd.Args) != len(expected) {
					t.Fatalf("expected %d args, got %d", len(expected), len(cmd.Args))
				}
				for i, arg := range expected {
					if cmd.Args[i] != arg {
						t.Errorf("arg[%d]: expected %s, got %s", i, arg, cmd.Args[i])
					}
				}
			},
		},
		{
			name: "object command",
			input: map[string]interface{}{
				"server": "npm start",
				"watch":  []interface{}{"npm", "run", "watch"},
			},
			validate: func(t *testing.T, cmd *LifecycleCommand) {
				if cmd.Type != "object" {
					t.Errorf("expected type object, got %s", cmd.Type)
				}
				if len(cmd.Commands) != 2 {
					t.Fatalf("expected 2 commands, got %d", len(cmd.Commands))
				}
				
				if server, ok := cmd.Commands["server"]; ok {
					if server.Type != "string" || server.Command != "npm start" {
						t.Error("unexpected server command")
					}
				} else {
					t.Error("missing server command")
				}
				
				if watch, ok := cmd.Commands["watch"]; ok {
					if watch.Type != "array" || len(watch.Args) != 3 {
						t.Error("unexpected watch command")
					}
				} else {
					t.Error("missing watch command")
				}
			},
		},
		{
			name:  "nil command",
			input: nil,
			validate: func(t *testing.T, cmd *LifecycleCommand) {
				if cmd != nil {
					t.Error("expected nil for nil input")
				}
			},
		},
		{
			name:    "invalid array element",
			input:   []interface{}{"npm", 123, "build"},
			wantErr: true,
		},
		{
			name:    "unsupported type",
			input:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := ParseLifecycleCommand(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLifecycleCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, cmd)
			}
		})
	}
}

func TestLifecycleCommandToShellCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *LifecycleCommand
		expected string
	}{
		{
			name: "string command",
			cmd: &LifecycleCommand{
				Type:    "string",
				Command: "echo 'Hello World'",
			},
			expected: "echo 'Hello World'",
		},
		{
			name: "array command",
			cmd: &LifecycleCommand{
				Type: "array",
				Args: []string{"echo", "Hello World"},
			},
			expected: `echo "Hello World"`,
		},
		{
			name: "array command no spaces",
			cmd: &LifecycleCommand{
				Type: "array",
				Args: []string{"npm", "install"},
			},
			expected: "npm install",
		},
		{
			name: "object command",
			cmd: &LifecycleCommand{
				Type: "object",
				Commands: map[string]*LifecycleCommand{
					"build": &LifecycleCommand{Type: "string", Command: "npm build"},
					"test":  &LifecycleCommand{Type: "string", Command: "npm test"},
				},
			},
			expected: "# Multiple commands:",
		},
		{
			name:     "nil command",
			cmd:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cmd.ToShellCommand()
			if tt.name == "object command" {
				if !strings.HasPrefix(result, tt.expected) {
					t.Errorf("expected prefix %q, got %q", tt.expected, result)
				}
			} else if result != tt.expected {
				t.Errorf("ToShellCommand() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestProcessLifecycleCommands(t *testing.T) {
	dc := &DevContainer{
		DevContainerCommon: DevContainerCommon{
			InitializeCommand:    "git config --global init.defaultBranch main",
			OnCreateCommand:      []interface{}{"npm", "install"},
			UpdateContentCommand: "git pull",
			PostCreateCommand: map[string]interface{}{
				"build": "npm run build",
				"test":  []interface{}{"npm", "test"},
			},
			PostStartCommand:  "npm start",
			PostAttachCommand: nil,
		},
	}

	commands, _ := ProcessLifecycleCommands(dc)

	// Check all commands were parsed
	expectedCommands := []string{
		"initializeCommand",
		"onCreateCommand",
		"updateContentCommand",
		"postCreateCommand",
		"postStartCommand",
	}

	for _, expected := range expectedCommands {
		if _, exists := commands[expected]; !exists {
			t.Errorf("missing command: %s", expected)
		}
	}

	// Verify specific commands
	if cmd := commands["onCreateCommand"]; cmd != nil {
		if cmd.Type != "array" || len(cmd.Args) != 2 {
			t.Error("unexpected onCreateCommand")
		}
	}

	if cmd := commands["postCreateCommand"]; cmd != nil {
		if cmd.Type != "object" || len(cmd.Commands) != 2 {
			t.Error("unexpected postCreateCommand")
		}
	}

	// postAttachCommand should not be in the map (it's nil)
	if _, exists := commands["postAttachCommand"]; exists {
		t.Error("postAttachCommand should not exist when nil")
	}
}

func TestGetLifecycleScript(t *testing.T) {
	dc := &DevContainer{
		DevContainerCommon: DevContainerCommon{
			InitializeCommand:    "echo 'Initializing'",
			OnCreateCommand:      "npm install",
			UpdateContentCommand: []interface{}{"git", "pull", "origin", "main"},
			PostCreateCommand:    "npm run build",
			PostStartCommand:     "npm start",
			PostAttachCommand:    "echo 'Attached'",
		},
	}

	tests := []struct {
		name     string
		phase    string
		contains []string
		wantErr  bool
	}{
		{
			name:  "create phase",
			phase: "create",
			contains: []string{
				"#!/bin/sh",
				"set -e",
				"# initializeCommand",
				"echo 'Initializing'",
				"# onCreateCommand",
				"npm install",
				"# updateContentCommand",
				"git pull origin main",
				"# postCreateCommand",
				"npm run build",
			},
		},
		{
			name:  "start phase",
			phase: "start",
			contains: []string{
				"#!/bin/sh",
				"# postStartCommand",
				"npm start",
			},
		},
		{
			name:  "attach phase",
			phase: "attach",
			contains: []string{
				"#!/bin/sh",
				"# postAttachCommand",
				"echo 'Attached'",
			},
		},
		{
			name:    "invalid phase",
			phase:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := GetLifecycleScript(dc, tt.phase)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetLifecycleScript() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				for _, expected := range tt.contains {
					if !strings.Contains(script, expected) {
						t.Errorf("script missing expected content: %q", expected)
					}
				}
			}
		})
	}
}

func TestLifecycleCommandsWithVariableExpansion(t *testing.T) {
	dc := &DevContainer{
		DevContainerCommon: DevContainerCommon{
			OnCreateCommand: "cd ${localWorkspaceFolder} && npm install",
			PostCreateCommand: []interface{}{
				"mkdir",
				"-p",
				"${containerWorkspaceFolder}/dist",
			},
			PostStartCommand: map[string]interface{}{
				"server": "cd ${containerWorkspaceFolder} && npm start",
				"watch":  []interface{}{"npm", "run", "watch", "--prefix", "${containerWorkspaceFolder}"},
			},
		},
	}

	variables := map[string]string{
		"localWorkspaceFolder":     "/home/user/project",
		"containerWorkspaceFolder": "/workspace/project",
	}

	ExpandVariables(dc, variables)

	// Parse the expanded commands
	commands, _ := ProcessLifecycleCommands(dc)

	// Check onCreateCommand expansion
	if cmd := commands["onCreateCommand"]; cmd != nil {
		expected := "cd /home/user/project && npm install"
		if cmd.ToShellCommand() != expected {
			t.Errorf("expected onCreateCommand %q, got %q", expected, cmd.ToShellCommand())
		}
	}

	// Check postCreateCommand expansion
	if cmd := commands["postCreateCommand"]; cmd != nil {
		shellCmd := cmd.ToShellCommand()
		if !strings.Contains(shellCmd, "/workspace/project/dist") {
			t.Errorf("postCreateCommand not properly expanded: %s", shellCmd)
		}
	}

	// Check postStartCommand expansion
	if cmd := commands["postStartCommand"]; cmd != nil {
		if serverCmd, ok := cmd.Commands["server"]; ok {
			expected := "cd /workspace/project && npm start"
			if serverCmd.ToShellCommand() != expected {
				t.Errorf("server command not properly expanded")
			}
		}
	}
}

func TestHostRequirementsCheck(t *testing.T) {
	tests := []struct {
		name    string
		req     *DevContainerCommonHostRequirements
		wantErr bool
	}{
		{
			name:    "nil requirements",
			req:     nil,
			wantErr: false,
		},
		{
			name: "valid requirements",
			req: &DevContainerCommonHostRequirements{
				CPUs:    "4",
				Memory:  "8gb",
				Storage: "50gb",
			},
			wantErr: false,
		},
		{
			name: "invalid CPU count",
			req: &DevContainerCommonHostRequirements{
				CPUs: "0",
			},
			wantErr: true,
		},
		{
			name: "GPU requirement",
			req: &DevContainerCommonHostRequirements{
				Gpu: "nvidia",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HostRequirementsCheck(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("HostRequirementsCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper function
func intPtr(i int) *int {
	return &i
}