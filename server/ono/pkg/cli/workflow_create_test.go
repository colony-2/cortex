package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowCreateCommand(t *testing.T) {

	tests := []struct {
		name           string
		args           []string
		setupFunc      func()
		expectedError  bool
		errorContains  string
		outputContains []string
	}{
		{
			name: "missing flags",
			setupFunc: func() {
				createProjectPath = ""
				createFilePath = ""
			},
			args:          []string{},
			expectedError: true,
			errorContains: "either --project or --file must be specified",
		},
		{
			name: "both flags specified",
			setupFunc: func() {
				createProjectPath = "./project"
				createFilePath = "test.yaml"
			},
			args:          []string{},
			expectedError: true,
			errorContains: "cannot specify both --project and --file",
		},
		{
			name: "file not found",
			setupFunc: func() {
				createProjectPath = ""
				createFilePath = "nonexistent.yaml"
			},
			args:          []string{},
			expectedError: true,
			errorContains: "failed to parse project",
		},
		{
			name: "project with specific workflow not found",
			setupFunc: func() {
				createProjectPath = "../../../recipe-worker/examples/research_project"
				createFilePath = ""
			},
			args:          []string{"nonexistent"},
			expectedError: true,
			errorContains: "workflow 'nonexistent' not found in project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			createProjectPath = ""
			createFilePath = ""

			// Create command with output buffer
			cmd := &cobra.Command{
				Use:  "create",
				RunE: createWorkflow,
			}

			// Add flags to command
			cmd.Flags().StringVar(&createProjectPath, "project", "", "")
			cmd.Flags().StringVar(&createFilePath, "file", "", "")

			// Setup test - this sets the flag variables
			tt.setupFunc()

			// Set up output capture
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			// Execute command
			err := cmd.Execute()
			output := buf.String()

			if tt.expectedError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}

			for _, expected := range tt.outputContains {
				assert.Contains(t, output, expected, "Output should contain: %s", expected)
			}
		})
	}
}

func TestWorkflowCreateValidation(t *testing.T) {
	// Test specific validation scenarios
	t.Run("workflow without activities", func(t *testing.T) {
		// This would require creating a test YAML file
		// For now, we'll skip this detailed test
		t.Skip("Requires test fixtures")
	})
}

func TestWorkflowCreateHelp(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create and validate a workflow from YAML definition",
		Long:  workflowCreateCmd.Long,
	}

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Create and register a workflow from a YAML file")
	assert.Contains(t, output, "--file")
	assert.Contains(t, output, "--project")
}

func TestIsDirectory(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "current directory",
			path:     ".",
			expected: true,
		},
		{
			name:     "file",
			path:     "workflow_create_test.go",
			expected: false,
		},
		{
			name:     "non-existent",
			path:     "nonexistent/path",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDirectory(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}





func TestWorkflowCreateErrorMessages(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		errorContains string
	}{
		{
			name:          "invalid YAML file",
			args:          []string{"--file", "workflow_create.go"}, // Not a YAML file
			errorContains: "failed to parse",
		},
		{
			name:          "directory as file",
			args:          []string{"--file", "../../example"},
			errorContains: "failed to parse",
		},
		{
			name:          "non-existent project",
			args:          []string{"--project", "../../example/non-existent"},
			errorContains: "failed to stat path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:  "create",
				RunE: createWorkflow,
			}

			cmd.Flags().StringVar(&createProjectPath, "project", "", "")
			cmd.Flags().StringVar(&createFilePath, "file", "", "")

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			require.Error(t, err)

			// Check that error contains expected message
			errStr := err.Error()
			assert.True(t, strings.Contains(errStr, tt.errorContains) ||
				strings.Contains(buf.String(), tt.errorContains),
				"Error should contain '%s', got: %s", tt.errorContains, errStr)
		})
	}
}
