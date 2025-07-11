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
			name: "valid file",
			setupFunc: func() {
				createProjectPath = ""
				createFilePath = "../../example/simple_workflow.yaml"
			},
			args: []string{},
			outputContains: []string{
				"Loading project from:",
				"Workflow: simple_research",
				"Registering activities:",
				"quick_search",
				"summarize_results",
				"Workflow structure:",
				"Required inputs:",
				"query",
				"✓ Workflow 'simple_research' is valid!",
			},
			// Will fail to connect to Temporal server in tests
			expectedError: true,
			errorContains: "failed to connect to Temporal server",
		},
		{
			name: "valid project",
			setupFunc: func() {
				createProjectPath = "../../example/research_project"
				createFilePath = ""
			},
			args: []string{},
			outputContains: []string{
				"Loading project from:",
				"Project: research_project",
				"Workflow: research_report_workflow",
				"research_activity",
				"analyze_activity",
				"write_report_activity",
				"✓ Workflow 'research_report_workflow' is valid!",
			},
			// Will fail to connect to Temporal server in tests
			expectedError: true,
			errorContains: "failed to connect to Temporal server",
		},
		{
			name: "project with specific workflow not found",
			setupFunc: func() {
				createProjectPath = "../../example/research_project"
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
	assert.Contains(t, output, "Create a workflow from a YAML file")
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

func TestWorkflowCreateOutput(t *testing.T) {
	// Test that create command suggests the correct run command
	t.Run("file suggestion", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:  "create",
			RunE: createWorkflow,
		}

		cmd.Flags().StringVar(&createProjectPath, "project", "", "")
		cmd.Flags().StringVar(&createFilePath, "file", "", "")

		createFilePath = "../../example/simple_workflow.yaml"
		createProjectPath = ""

		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--file", createFilePath})

		err := cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "ono workflow run --file ../../example/simple_workflow.yaml")
	})

	t.Run("project suggestion", func(t *testing.T) {
		cmd := &cobra.Command{
			Use:  "create",
			RunE: createWorkflow,
		}

		cmd.Flags().StringVar(&createProjectPath, "project", "", "")
		cmd.Flags().StringVar(&createFilePath, "file", "", "")

		createProjectPath = "../../example/research_project"
		createFilePath = ""

		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--project", createProjectPath})

		err := cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "ono workflow run --project ../../example/research_project research_report_workflow")
	})
}

func TestWorkflowCreateInputOutput(t *testing.T) {
	// Test that inputs and outputs are properly displayed
	cmd := &cobra.Command{
		Use:  "create",
		RunE: createWorkflow,
	}

	cmd.Flags().StringVar(&createProjectPath, "project", "", "")
	cmd.Flags().StringVar(&createFilePath, "file", "", "")

	createProjectPath = "../../example/research_project"
	createFilePath = ""

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--project", createProjectPath})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	// Check inputs section
	assert.Contains(t, output, "Required inputs:")
	assert.Contains(t, output, "topic")
	assert.Contains(t, output, "max_sources")
	assert.Contains(t, output, "[default: 10]")

	// Check outputs section
	assert.Contains(t, output, "Outputs:")
	assert.Contains(t, output, "report")
	assert.Contains(t, output, "sources")
}

func TestWorkflowCreateParallelSteps(t *testing.T) {
	// Test that parallel steps are properly displayed
	// This would require a workflow with parallel steps
	// For now, we'll check the basic structure is shown
	cmd := &cobra.Command{
		Use:  "create",
		RunE: createWorkflow,
	}

	cmd.Flags().StringVar(&createProjectPath, "project", "", "")
	cmd.Flags().StringVar(&createFilePath, "file", "", "")

	createFilePath = "../../example/simple_workflow.yaml"
	createProjectPath = ""

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--file", createFilePath})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	// Check workflow structure
	assert.Contains(t, output, "Workflow structure:")
	assert.Contains(t, output, "1. search (activity: quick_search)")
	assert.Contains(t, output, "2. summarize (activity: summarize_results)")
}

// Integration test that runs against real example files
func TestWorkflowCreateIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	examples := []struct {
		name   string
		args   []string
		checks []string
	}{
		{
			name: "simple workflow file",
			args: []string{"--file", "../../example/simple_workflow.yaml"},
			checks: []string{
				"simple_research",
				"v1.0",
				"A simple research workflow",
			},
		},
		{
			name: "research project",
			args: []string{"--project", "../../example/research_project"},
			checks: []string{
				"research_report_workflow",
				"Research a topic and generate a comprehensive report",
				"3", // number of steps
			},
		},
		{
			name: "gemini workflow",
			args: []string{"--file", "../../example/gemini_workflow.yaml"},
			checks: []string{
				"gemini_research_workflow",
				"gemini_report_activity",
			},
		},
	}

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:  "create",
				RunE: createWorkflow,
			}

			cmd.Flags().StringVar(&createProjectPath, "project", "", "")
			cmd.Flags().StringVar(&createFilePath, "file", "", "")

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(ex.args)

			err := cmd.Execute()

			output := buf.String()
			if err != nil {
				t.Logf("Error output: %s", output)
			}

			// We expect connection error since no server is running
			require.Error(t, err, "Command should fail to connect to server")

			for _, check := range ex.checks {
				assert.Contains(t, output, check, "Output should contain: %s", check)
			}

			// All examples should validate successfully
			assert.Contains(t, output, "✓ Workflow")
			assert.Contains(t, output, "is valid!")
			
			// But should fail to connect to server
			if err != nil {
				assert.Contains(t, err.Error(), "failed to connect to Temporal server")
			}
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
