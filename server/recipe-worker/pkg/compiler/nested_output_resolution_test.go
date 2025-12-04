//go:build test44

package compiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNestedOutputResolution(t *testing.T) {
	tests := []struct {
		name            string
		outputTemplates map[string]interface{}
		inputs          map[string]interface{}
		nodeOutputs     map[string]interface{}
		expected        map[string]interface{}
	}{
		{
			name: "nested object with template expressions",
			outputTemplates: map[string]interface{}{
				"simple": "{{ inputs.value1 }}",
				"nested": map[string]interface{}{
					"level1": "{{ inputs.value1 }}",
					"level2": map[string]interface{}{
						"deep": "{{ inputs.value2 }}",
					},
				},
				"array": []interface{}{
					"{{ inputs.value1 }}",
					map[string]interface{}{
						"item": "{{ inputs.value2 }}",
					},
				},
			},
			inputs: map[string]interface{}{
				"value1": "test1",
				"value2": "test2",
			},
			nodeOutputs: map[string]interface{}{},
			expected: map[string]interface{}{
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
			},
		},
		{
			name: "deeply nested with node outputs",
			outputTemplates: map[string]interface{}{
				"result": map[string]interface{}{
					"data": map[string]interface{}{
						"fromInput": "{{ inputs.inputValue }}",
						"fromNode":  "{{ sequence.node1.outputs.result }}",
						"nested": map[string]interface{}{
							"level3": map[string]interface{}{
								"value": "{{ sequence.node1.outputs.nested }}",
							},
						},
					},
				},
			},
			inputs: map[string]interface{}{
				"inputValue": "from-input",
			},
			nodeOutputs: map[string]interface{}{
				"node1": map[string]interface{}{
					"result": "from-node",
					"nested": "deep-value",
				},
			},
			expected: map[string]interface{}{
				"result": map[string]interface{}{
					"data": map[string]interface{}{
						"fromInput": "from-input",
						"fromNode":  "from-node",
						"nested": map[string]interface{}{
							"level3": map[string]interface{}{
								"value": "deep-value",
							},
						},
					},
				},
			},
		},
		{
			name: "array with nested objects containing templates",
			outputTemplates: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id": "{{ inputs.id1 }}",
						"details": map[string]interface{}{
							"name": "{{ inputs.name1 }}",
						},
					},
					map[string]interface{}{
						"id": "{{ inputs.id2 }}",
						"details": map[string]interface{}{
							"name": "{{ inputs.name2 }}",
						},
					},
				},
			},
			inputs: map[string]interface{}{
				"id1":   "first",
				"name1": "First Item",
				"id2":   "second",
				"name2": "Second Item",
			},
			nodeOutputs: map[string]interface{}{},
			expected: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id": "first",
						"details": map[string]interface{}{
							"name": "First Item",
						},
					},
					map[string]interface{}{
						"id": "second",
						"details": map[string]interface{}{
							"name": "Second Item",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processNodeOutputs(tt.nodeOutputs, tt.outputTemplates, tt.inputs, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
