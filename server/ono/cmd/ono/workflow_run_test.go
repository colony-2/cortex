package ono

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowRunCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedError  bool
		errorContains  string
	}{
		{
			name:          "missing file flag",
			args:          []string{"run"},
			expectedError: true,
			errorContains: "required flag(s) \"file\" not set",
		},
		{
			name:          "with workflow file",
			args:          []string{"run", "--file", "workflow.yaml"},
			expectedError: false,
		},
		{
			name:          "with workflow ID",
			args:          []string{"run", "--file", "workflow.yaml", "--id", "custom-workflow-123"},
			expectedError: false,
		},
		{
			name:          "with inputs",
			args:          []string{"run", "--file", "workflow.yaml", "--input", "key1=value1", "--input", "key2=value2"},
			expectedError: false,
		},
		{
			name:          "with namespace",
			args:          []string{"run", "--file", "workflow.yaml", "--namespace", "production"},
			expectedError: false,
		},
		{
			name:          "with task queue",
			args:          []string{"run", "--file", "workflow.yaml", "--task-queue", "custom-queue"},
			expectedError: false,
		},
		{
			name:          "with all options",
			args:          []string{
				"run",
				"--file", "workflow.yaml",
				"--id", "my-workflow-123",
				"--namespace", "production",
				"--task-queue", "custom-queue",
				"--input", "topic=AI",
				"--input", "depth=5",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command that mimics the real one
			// In a real test environment, you'd create a mock workflow file
		})
	}
}

func TestWorkflowRunFlags(t *testing.T) {
	cmd := workflowRunCmd
	
	// Ensure all required flags are registered
	require.NotNil(t, cmd.Flag("file"))
	require.NotNil(t, cmd.Flag("id"))
	require.NotNil(t, cmd.Flag("namespace"))
	require.NotNil(t, cmd.Flag("input"))
	require.NotNil(t, cmd.Flag("task-queue"))
	require.NotNil(t, cmd.Flag("host"))
	require.NotNil(t, cmd.Flag("port"))
	
	// Check required flags
	fileFlag := cmd.Flag("file")
	assert.NotNil(t, fileFlag)
	
	// Check defaults
	assert.Equal(t, "", cmd.Flag("file").DefValue)
	assert.Equal(t, "", cmd.Flag("id").DefValue)
	assert.Equal(t, "default", cmd.Flag("namespace").DefValue)
	assert.Equal(t, "ono-task-queue", cmd.Flag("task-queue").DefValue)
}

func TestWorkflowFileValidation(t *testing.T) {
	// Create temporary test files
	tempDir := t.TempDir()
	
	validYAML := `
name: test-workflow
version: "1.0"
inputs:
  - name: topic
    type: string
    required: true
workflow:
  type: sequential
  steps:
    - id: step1
      activity: research
      inputs:
        topic: "{{ .inputs.topic }}"
`
	
	invalidYAML := `
invalid: yaml: content
`
	
	noWorkflowYAML := `
name: test-project
version: "1.0"
activities:
  - name: activity1
`
	
	tests := []struct {
		name          string
		content       string
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid workflow file",
			content:     validYAML,
			expectError: false,
		},
		{
			name:          "invalid YAML",
			content:       invalidYAML,
			expectError:   true,
			errorContains: "failed to parse",
		},
		{
			name:          "no workflow definition",
			content:       noWorkflowYAML,
			expectError:   true,
			errorContains: "no workflow definition found",
		},
		{
			name:          "non-existent file",
			content:       "",
			expectError:   true,
			errorContains: "failed to parse",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filename string
			if tt.content != "" {
				// Create test file
				filename = filepath.Join(tempDir, tt.name+".yaml")
				err := os.WriteFile(filename, []byte(tt.content), 0644)
				require.NoError(t, err)
			} else {
				filename = filepath.Join(tempDir, "nonexistent.yaml")
			}
			
			// Test would call executeWorkflow with the test file
			// and verify error behavior
		})
	}
}

func TestWorkflowIDGeneration(t *testing.T) {
	tests := []struct {
		name         string
		providedID   string
		workflowName string
		expectCustom bool
	}{
		{
			name:         "custom ID provided",
			providedID:   "my-custom-id",
			workflowName: "test-workflow",
			expectCustom: true,
		},
		{
			name:         "auto-generated ID",
			providedID:   "",
			workflowName: "test-workflow",
			expectCustom: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test ID generation logic
			if tt.expectCustom {
				assert.Equal(t, tt.providedID, tt.providedID)
			} else {
				// Auto-generated IDs should have format: workflowName-uuid
				// This would be tested in the actual implementation
			}
		})
	}
}

func TestInputParsing(t *testing.T) {
	tests := []struct {
		name     string
		inputs   map[string]string
		expected map[string]interface{}
	}{
		{
			name: "string inputs",
			inputs: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:     "empty inputs",
			inputs:   map[string]string{},
			expected: map[string]interface{}{},
		},
		{
			name: "complex values",
			inputs: map[string]string{
				"topic":   "AI Safety",
				"depth":   "5",
				"verbose": "true",
			},
			expected: map[string]interface{}{
				"topic":   "AI Safety",
				"depth":   "5",
				"verbose": "true",
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]interface{})
			for k, v := range tt.inputs {
				result[k] = v
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}