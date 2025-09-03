package compiler

import (
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/stretchr/testify/assert"
)

func TestConvertOutputMap(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "simple string remains unchanged",
			input:    "test value",
			expected: "test value",
		},
		{
			name:     "simple number remains unchanged",
			input:    42,
			expected: 42,
		},
		{
			name: "OutputMap is converted to map[string]interface{}",
			input: recipe.OutputMap{
				"key1": "value1",
				"key2": 123,
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": 123,
			},
		},
		{
			name: "nested OutputMap is recursively converted",
			input: recipe.OutputMap{
				"level1": recipe.OutputMap{
					"level2": recipe.OutputMap{
						"deep": "value",
					},
					"sibling": "test",
				},
				"top": "level",
			},
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"deep": "value",
					},
					"sibling": "test",
				},
				"top": "level",
			},
		},
		{
			name: "array with OutputMap elements",
			input: []interface{}{
				"string",
				recipe.OutputMap{
					"key": "value",
				},
				42,
			},
			expected: []interface{}{
				"string",
				map[string]interface{}{
					"key": "value",
				},
				42,
			},
		},
		{
			name: "complex nested structure with OutputMap",
			input: recipe.OutputMap{
				"array": []interface{}{
					recipe.OutputMap{
						"nested": recipe.OutputMap{
							"deep": "value1",
						},
					},
					"plain string",
				},
				"map": recipe.OutputMap{
					"level1": []interface{}{
						recipe.OutputMap{
							"item": "value2",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"array": []interface{}{
					map[string]interface{}{
						"nested": map[string]interface{}{
							"deep": "value1",
						},
					},
					"plain string",
				},
				"map": map[string]interface{}{
					"level1": []interface{}{
						map[string]interface{}{
							"item": "value2",
						},
					},
				},
			},
		},
		{
			name: "mixed map[string]interface{} with nested OutputMap",
			input: map[string]interface{}{
				"regular": "value",
				"nested": recipe.OutputMap{
					"converted": "yes",
				},
			},
			expected: map[string]interface{}{
				"regular": "value",
				"nested": map[string]interface{}{
					"converted": "yes",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertOutputMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessNodeOutputsWithOutputMap(t *testing.T) {
	// Test that processNodeOutputs correctly handles OutputMap in templates
	outputTemplates := map[string]interface{}{
		"simple": "{{ inputs.value1 }}",
		"nested": recipe.OutputMap{
			"level1": "{{ inputs.value1 }}",
			"level2": recipe.OutputMap{
				"deep": "{{ inputs.value2 }}",
			},
		},
		"array": []interface{}{
			"{{ inputs.value1 }}",
			recipe.OutputMap{
				"item": "{{ inputs.value2 }}",
			},
		},
	}

	inputs := map[string]interface{}{
		"value1": "test1",
		"value2": "test2",
	}

	nodeOutputs := map[string]interface{}{
		"node1": map[string]interface{}{
			"result": "node output",
		},
	}

	expected := map[string]interface{}{
		"simple": "test1",
		"nested": map[string]interface{}{
			"level1": "test1",
			"level2": map[string]interface{}{
				"deep": "test2",
			},
		},
		"array": []interface{}{
			"test1",
			map[string]interface{}{
				"item": "test2",
			},
		},
	}

	result, err := processNodeOutputs(nodeOutputs, outputTemplates, inputs, nil)
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}