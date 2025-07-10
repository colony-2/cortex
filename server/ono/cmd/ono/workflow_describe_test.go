package ono

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowDescribeCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedError  bool
		errorContains  string
		expectedOutput []string
	}{
		{
			name:          "help flag shows usage",
			args:          []string{"--help"},
			expectedError: false,
			expectedOutput: []string{
				"Show detailed information about a workflow execution",
				"workflow-id",
				"--run-id",
				"--namespace",
			},
		},
		{
			name:          "missing workflow ID",
			args:          []string{},
			expectedError: true,
			errorContains: "accepts 1 arg",
		},
		{
			name:          "valid workflow ID",
			args:          []string{"my-workflow-123"},
			expectedError: false,
		},
		{
			name:          "with run ID",
			args:          []string{"my-workflow-123", "--run-id", "abc-def-123"},
			expectedError: false,
		},
		{
			name:          "with namespace",
			args:          []string{"my-workflow-123", "--namespace", "production"},
			expectedError: false,
		},
		{
			name:          "too many arguments",
			args:          []string{"workflow1", "workflow2"},
			expectedError: true,
			errorContains: "accepts 1 arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "describe [workflow-id]",
				Short: "Show detailed information about a workflow execution",
				Args:  cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					if cmd.Flag("help").Changed {
						return cmd.Help()
					}
					return nil
				},
			}

			// Copy flags from actual command
			cmd.Flags().String("run-id", "", "Specific run ID")
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
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestWorkflowDescribeFlags(t *testing.T) {
	cmd := workflowDescribeCmd
	
	// Ensure all required flags are registered
	require.NotNil(t, cmd.Flag("run-id"))
	require.NotNil(t, cmd.Flag("namespace"))
	require.NotNil(t, cmd.Flag("host"))
	require.NotNil(t, cmd.Flag("port"))
	
	// Check flag types and defaults
	assert.Equal(t, "string", cmd.Flag("run-id").Value.Type())
	assert.Equal(t, "", cmd.Flag("run-id").DefValue)
	
	assert.Equal(t, "default", cmd.Flag("namespace").DefValue)
	assert.Equal(t, "127.0.0.1", cmd.Flag("host").DefValue)
	assert.Equal(t, "7233", cmd.Flag("port").DefValue)
}

func TestWorkflowDescribeArgs(t *testing.T) {
	cmd := workflowDescribeCmd
	
	// Test argument validation
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err, "should require at least one argument")
	
	err = cmd.Args(cmd, []string{"workflow-id"})
	assert.NoError(t, err, "should accept one argument")
	
	err = cmd.Args(cmd, []string{"workflow-id", "extra"})
	assert.Error(t, err, "should reject extra arguments")
}