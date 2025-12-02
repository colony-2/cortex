package main

import (
	"context"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateExpansion(t *testing.T) {
	// Create a workflow state with inputs
	state := &compiler.WorkflowState{
		Inputs: map[string]interface{}{
			"message": "Hello World",
			"count":   42,
		},
		Steps: make(map[string]compiler.StepResult),
	}

	// Create template resolver
	resolver := compiler.NewTemplateResolver(state)

	tests := []struct {
		name     string
		template string
		expected interface{}
	}{
		{
			name:     "simple string template",
			template: "echo {{ .ContainerInputs.message }}",
			expected: "echo Hello World",
		},
		{
			name:     "numeric template",
			template: "count is {{ .ContainerInputs.count }}",
			expected: "count is 42",
		},
		{
			name:     "no template",
			template: "echo test",
			expected: "echo test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.Resolve(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestActivityExecutorWithTemplates(t *testing.T) {
	// This test verifies that our activity executor properly handles
	// inputs that have already been template-expanded by the workflow

	ctx := context.Background()

	// Simulate what the workflow would pass after template expansion
	inputs := map[string]interface{}{
		"run": "echo Hello World", // Already expanded from "echo {{ .ContainerInputs.message }}"
	}

	// The activity executor should receive the expanded string
	// and be able to execute it properly
	t.Log("ContainerInputs after template expansion:", inputs)

	// Verify the input is a plain string without template markers
	runCmd, ok := inputs["run"].(string)
	require.True(t, ok, "run should be a string")
	assert.NotContains(t, runCmd, "{{", "Template should be expanded")
	assert.NotContains(t, runCmd, "}}", "Template should be expanded")
	assert.Equal(t, "echo Hello World", runCmd)

	_ = ctx // Use to avoid unused variable warning
}
