package compiler

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"gopkg.in/yaml.v3"
)

func TestSequenceOutputReferences(t *testing.T) {
	// Define a YAML recipe with sequence nodes and output templates
	recipeYAML := `
id: test-sequence-output-refs
desc: Test sequence output references
version: '1.0'
inputs:
  prefix:
    type: string
    default: "Test"
sequence:
- id: step1
  op: command_execution
  inputs:
    run: echo "Hello from step 1"
    shell: bash
- id: step2
  op: command_execution
  inputs:
    run: echo "Processing data"
    shell: bash
- id: step3
  op: sleep
  inputs:
    duration: 1s
outputs:
  first_message: '{{ sequence.step1.outputs.stdout }}'
  transform_result: '{{ sequence.step2.outputs.stdout }}'
  sleep_duration: '{{ sequence.step3.outputs.slept }}'
  combined: '{{ inputs.prefix + ": " + sequence.step1.outputs.stdout }}'
`

	// Parse the YAML into a Recipe struct
	var r recipe.Recipe
	err := yaml.Unmarshal([]byte(recipeYAML), &r)
	require.NoError(t, err)
	
	// Check that it's a RecipeSequence
	recipeSeq, ok := r.RecipeImpl.(*recipe.RecipeSequence)
	require.True(t, ok, "Expected RecipeSequence")
	require.NotNil(t, recipeSeq.SequenceData.Sequence)
	require.Len(t, recipeSeq.SequenceData.Sequence, 3)

	// Create a resolution context
	ctx, err := NewResolutionContext("sequence", "test-exec")
	require.NoError(t, err)

	// Add inputs
	ctx.TemplateData.Inputs = map[string]interface{}{
		"prefix": "Result",
	}

	// Simulate sequence node execution by adding their outputs
	// This mimics what happens in the actual compiler during sequence execution
	ctx.AddSequenceNode("step1", map[string]interface{}{
		"stdout": "Hello from step 1\n",
		"stderr": "",
		"exit_code": 0,
	})

	ctx.AddSequenceNode("step2", map[string]interface{}{
		"stdout": "Processing data\n",
		"stderr": "",
		"exit_code": 0,
	})

	ctx.AddSequenceNode("step3", map[string]interface{}{
		"slept": "1s",
		"actual_duration": 1000,
	})

	// Test resolving output templates
	testCases := []struct {
		name     string
		template string
		expected interface{}
	}{
		{
			name:     "step1 stdout output",
			template: "{{ sequence.step1.outputs.stdout }}",
			expected: "Hello from step 1\n",
		},
		{
			name:     "step2 stdout output",
			template: "{{ sequence.step2.outputs.stdout }}",
			expected: "Processing data\n",
		},
		{
			name:     "step3 sleep output",
			template: "{{ sequence.step3.outputs.slept }}",
			expected: "1s",
		},
		{
			name:     "combined with input",
			template: `{{ inputs.prefix + ": " + sequence.step1.outputs.stdout }}`,
			expected: "Result: Hello from step 1\n",
		},
		{
			name:     "exit code check",
			template: "{{ sequence.step1.outputs.exit_code }}",
			expected: int64(0),
		},
		{
			name:     "duration check",
			template: "{{ sequence.step3.outputs.actual_duration }}",
			expected: int64(1000),
		},
		{
			name:     "CEL expression with sequence output",
			template: "{{ sequence.step1.outputs.exit_code == 0 }}",
			expected: true,
		},
		{
			name:     "multiple sequence nodes in expression",
			template: `{{ sequence.step1.outputs.exit_code + sequence.step2.outputs.exit_code }}`,
			expected: int64(0),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ctx.ResolveTemplate(tc.template)
			require.NoError(t, err, "Failed to resolve template: %s", tc.template)
			assert.Equal(t, tc.expected, result, "Template resolution mismatch")
		})
	}
}

func TestSequenceOutputWithActualRecipe(t *testing.T) {
	// Test with the actual test-sequence-node.yaml structure
	recipeYAML := `
id: test-sequence-node
desc: Test recipe for sequence node with multiple operations
version: '1.0'
sequence:
- id: step1
  op: command_execution
  inputs:
    run: echo 'first step'
    shell: bash
- id: step2
  op: sleep
  inputs:
    duration: 2s
- id: step3
  op: command_execution
  inputs:
    run: echo 'Step 3 complete'
    shell: bash
outputs:
  command_output: '{{ sequence.step1.outputs.stdout }}'
  sleep_completed: '{{ has(sequence.step2.outputs.slept) }}'
  final_output: '{{ sequence.step3.outputs.stdout }}'
`

	// Parse the YAML
	var r recipe.Recipe
	err := yaml.Unmarshal([]byte(recipeYAML), &r)
	require.NoError(t, err)

	// Create resolution context
	ctx, err := NewResolutionContext("sequence", "test-exec-2")
	require.NoError(t, err)

	// Simulate execution of sequence nodes
	ctx.AddSequenceNode("step1", map[string]interface{}{
		"stdout": "first step\n",
		"stderr": "",
		"exit_code": 0,
	})

	ctx.AddSequenceNode("step2", map[string]interface{}{
		"slept": "2s",
		"actual_duration": 2000,
	})

	ctx.AddSequenceNode("step3", map[string]interface{}{
		"stdout": "Step 3 complete\n",
		"stderr": "",
		"exit_code": 0,
	})

	// Get the outputs from the parsed recipe
	recipeSeq, ok := r.RecipeImpl.(*recipe.RecipeSequence)
	require.True(t, ok, "Expected RecipeSequence")
	
	// Test the output templates from the recipe
	tests := []struct {
		name     string
		template string
		expected interface{}
	}{
		{
			name:     "command_output from step1",
			template: recipeSeq.SequenceData.Outputs["command_output"].(string),
			expected: "first step\n",
		},
		{
			name:     "sleep_completed check",
			template: recipeSeq.SequenceData.Outputs["sleep_completed"].(string),
			expected: true,
		},
		{
			name:     "direct stdout access",
			template: "{{ sequence.step1.outputs.stdout }}",
			expected: "first step\n",
		},
		{
			name:     "check slept field exists",
			template: "{{ has(sequence.step2.outputs.slept) }}",
			expected: true,
		},
		{
			name:     "access step3 stdout",
			template: "{{ sequence.step3.outputs.stdout }}",
			expected: "Step 3 complete\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ctx.ResolveTemplate(tc.template)
			require.NoError(t, err, "Failed to resolve template: %s", tc.template)
			assert.Equal(t, tc.expected, result, "Template resolution mismatch for: %s", tc.template)
		})
	}
}

func TestSequenceOutputErrorCases(t *testing.T) {
	ctx, err := NewResolutionContext("sequence", "test-errors")
	require.NoError(t, err)

	// Add a sequence node
	ctx.AddSequenceNode("existing", map[string]interface{}{
		"data": "test",
	})

	errorCases := []struct {
		name        string
		template    string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "non-existent node",
			template:    "{{ sequence.nonexistent.outputs.field }}",
			shouldError: true,
			errorMsg:    "no such key: nonexistent",
		},
		{
			name:        "non-existent field",
			template:    "{{ sequence.existing.outputs.missing }}",
			shouldError: true,
			errorMsg:    "no such key: missing",
		},
		{
			name:        "missing outputs keyword",
			template:    "{{ sequence.existing.data }}", // Wrong: should be sequence.existing.outputs.data
			shouldError: true,
			errorMsg:    "no such key: data",
		},
		{
			name:        "correct access",
			template:    "{{ sequence.existing.outputs.data }}",
			shouldError: false,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ctx.ResolveTemplate(tc.template)
			if tc.shouldError {
				require.Error(t, err, "Expected error for template: %s", tc.template)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err, "Should not error for template: %s", tc.template)
				assert.NotNil(t, result)
			}
		})
	}
}