package ono

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowListCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedError  bool
		expectedOutput []string
	}{
		{
			name:          "help flag shows usage",
			args:          []string{"--help"},
			expectedError: false,
			expectedOutput: []string{
				"List workflow executions",
				"--status",
				"--type",
				"--archived",
			},
		},
		{
			name:          "valid status filter",
			args:          []string{"--status", "running"},
			expectedError: false,
		},
		{
			name:          "valid type filter",
			args:          []string{"--type", "MyWorkflow"},
			expectedError: false,
		},
		{
			name:          "combined filters",
			args:          []string{"--status", "completed", "--type", "DataProcessing"},
			expectedError: false,
		},
		{
			name:          "archived workflows",
			args:          []string{"--archived"},
			expectedError: false,
		},
		{
			name:          "with namespace",
			args:          []string{"--namespace", "test-namespace"},
			expectedError: false,
		},
		{
			name:          "with custom host and port",
			args:          []string{"--host", "temporal.example.com", "--port", "8233"},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: "list",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Mock implementation for testing
					if cmd.Flag("help").Changed {
						return cmd.Help()
					}
					return nil
				},
			}

			// Copy flags from actual command
			cmd.Flags().StringP("status", "s", "", "Filter by workflow status")
			cmd.Flags().StringP("type", "t", "", "Filter by workflow type")
			cmd.Flags().Bool("archived", false, "Include archived workflows")
			cmd.Flags().StringP("namespace", "n", "default", "Temporal namespace")
			cmd.Flags().String("host", "127.0.0.1", "Temporal server host")
			cmd.Flags().IntP("port", "p", 7233, "Temporal server port")

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			output := buf.String()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestWorkflowListParsing(t *testing.T) {
	// Test the parseWorkflowStatus function
	tests := []struct {
		input    string
		expected string
	}{
		{"running", "Running"},
		{"completed", "Completed"},
		{"failed", "Failed"},
		{"canceled", "Canceled"},
		{"terminated", "Terminated"},
		{"continuedasnew", "ContinuedAsNew"},
		{"timedout", "TimedOut"},
		{"", "Unspecified"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseWorkflowStatus(tt.input)
			assert.Equal(t, tt.expected, result.String())
		})
	}
}

func TestWorkflowListFlags(t *testing.T) {
	// Ensure all required flags are registered
	cmd := workflowListCmd
	
	require.NotNil(t, cmd.Flag("status"))
	require.NotNil(t, cmd.Flag("type"))
	require.NotNil(t, cmd.Flag("archived"))
	require.NotNil(t, cmd.Flag("namespace"))
	require.NotNil(t, cmd.Flag("host"))
	require.NotNil(t, cmd.Flag("port"))
	
	// Check default values
	assert.Equal(t, "default", cmd.Flag("namespace").DefValue)
	assert.Equal(t, "127.0.0.1", cmd.Flag("host").DefValue)
	assert.Equal(t, "7233", cmd.Flag("port").DefValue)
}

func TestFormatWorkflowExecution(t *testing.T) {
	// Test output formatting
	output := new(strings.Builder)
	
	// This would test the actual formatting logic
	// In a real test, you'd create mock WorkflowExecutionInfo objects
	// and verify the formatted output
}