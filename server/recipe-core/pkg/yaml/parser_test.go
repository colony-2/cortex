package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSimpleOperation(t *testing.T) {
	yamlContent := `
name: test_recipe
description: A test recipe with single operation
version: "1.0"

# Root is a single operation
op: research_activity
inputs:
  topic: "Climate Change"
  max_sources: 10
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	assert.Equal(t, "test_recipe", recipe.Name)
	assert.Equal(t, "A test recipe with single operation", recipe.Description)
	assert.Equal(t, "1.0", recipe.Version)
	
	// Check root node
	assert.Equal(t, "research_activity", recipe.Op)
	assert.NotNil(t, recipe.Inputs)
	assert.Equal(t, "Climate Change", recipe.Inputs["topic"])
	assert.Equal(t, 10, recipe.Inputs["max_sources"])
}

func TestParseSequence(t *testing.T) {
	yamlContent := `
name: sequence_recipe
version: "1.0"

# Root is a sequence
sequence:
  - id: validate
    op: validation_activity
    inputs:
      data: "{{ .inputs.data }}"
  
  - id: analyze
    op: llm
    inputs:
      prompt: "Analyze: {{ .nodes.validate.outputs }}"
      model: "gpt-4"

inputs:
  data: "test data"
outputs:
  result: "{{ .nodes.analyze.response }}"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	// Check sequence
	require.NotNil(t, recipe.Sequence)
	require.Len(t, recipe.Sequence, 2)
	
	// First node
	assert.Equal(t, "validate", recipe.Sequence[0].ID)
	assert.Equal(t, "validation_activity", recipe.Sequence[0].Op)
	assert.Equal(t, "{{ .inputs.data }}", recipe.Sequence[0].Inputs["data"])
	
	// Second node
	assert.Equal(t, "analyze", recipe.Sequence[1].ID)
	assert.Equal(t, "llm", recipe.Sequence[1].Op)
	assert.Equal(t, "gpt-4", recipe.Sequence[1].Inputs["model"])
}

func TestParseParallel(t *testing.T) {
	yamlContent := `
name: parallel_recipe
version: "1.0"

# Root is parallel
parallel:
  - id: task1
    op: command_execution
    inputs:
      run: "echo task1"
  
  - id: task2
    op: command_execution
    inputs:
      run: "echo task2"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	// Check parallel
	require.NotNil(t, recipe.Parallel)
	require.Len(t, recipe.Parallel, 2)
	
	assert.Equal(t, "task1", recipe.Parallel[0].ID)
	assert.Equal(t, "command_execution", recipe.Parallel[0].Op)
	assert.Equal(t, "echo task1", recipe.Parallel[0].Inputs["run"])
	
	assert.Equal(t, "task2", recipe.Parallel[1].ID)
	assert.Equal(t, "command_execution", recipe.Parallel[1].Op)
	assert.Equal(t, "echo task2", recipe.Parallel[1].Inputs["run"])
}

func TestParseStateMachine(t *testing.T) {
	yamlContent := `
name: state_machine_recipe
version: "1.0"

# Root is a state machine
states:
  initial: validate
  
  validate:
    op: validation_activity
    inputs:
      data: "{{ .inputs.data }}"
    transitions:
      - to: process
        when: ".outputs.valid == true"
      - to: error
  
  process:
    op: processor
    transitions:
      - to: complete
  
  complete:
    op: command_execution
    inputs:
      run: "echo 'Done'"
  
  error:
    error: "Validation failed"

inputs:
  data: "test"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	// Check state machine
	require.NotNil(t, recipe.States)
	assert.Equal(t, "validate", recipe.States.Initial)
	
	// Check states
	validateState, exists := recipe.States.States["validate"]
	require.True(t, exists)
	assert.Equal(t, "validation_activity", validateState.Op)
	assert.Len(t, validateState.Transitions, 2)
	assert.Equal(t, "process", validateState.Transitions[0].To)
	assert.Equal(t, ".outputs.valid == true", validateState.Transitions[0].When)
	
	errorState, exists := recipe.States.States["error"]
	require.True(t, exists)
	assert.Equal(t, "Validation failed", errorState.Error)
}

func TestParseSharedNodes(t *testing.T) {
	yamlContent := `
name: recipe_with_shared
version: "1.0"

shared:
  data_processor:
    op: llm
    inputs:
      model: "gpt-4"
      temperature: 0.3
    timeout: "30s"
  
  validator:
    sequence:
      - id: check1
        op: validation_activity
        inputs:
          type: "schema"
      
      - id: check2
        op: validation_activity
        inputs:
          type: "business_rules"

# Root is a sequence using shared nodes
sequence:
  - id: validate
    shared: validator
  
  - id: process
    shared: data_processor
    inputs:
      prompt: "Process this data"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	// Check shared nodes
	require.NotNil(t, recipe.Shared)
	require.Len(t, recipe.Shared, 2)
	
	// Check data processor shared node
	dataProcessor, exists := recipe.Shared["data_processor"]
	require.True(t, exists)
	assert.Equal(t, "llm", dataProcessor.Op)
	assert.Equal(t, "gpt-4", dataProcessor.Inputs["model"])
	assert.Equal(t, "30s", dataProcessor.Timeout)
	
	// Check validator shared node (sequence)
	validator, exists := recipe.Shared["validator"]
	require.True(t, exists)
	require.NotNil(t, validator.Sequence)
	assert.Len(t, validator.Sequence, 2)
	
	// Check root sequence references shared nodes
	require.Len(t, recipe.Sequence, 2)
	assert.Equal(t, "validator", recipe.Sequence[0].Shared)
	assert.Equal(t, "data_processor", recipe.Sequence[1].Shared)
}

func TestParseNestedComposition(t *testing.T) {
	yamlContent := `
name: nested_composition
version: "1.0"

# Root sequence with nested parallel
sequence:
  - id: prepare
    op: setup_activity
  
  - id: parallel_process
    parallel:
      - id: branch1
        sequence:
          - id: step1
            op: processor
            inputs:
              id: 1
          
          - id: step2
            op: validator
      
      - id: branch2
        op: processor
        inputs:
          id: 2
  
  - id: aggregate
    op: aggregator
    inputs:
      data: "{{ .nodes.parallel_process.nodes }}"
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	
	// Check root sequence
	require.Len(t, recipe.Sequence, 3)
	
	// Check nested parallel
	parallelNode := recipe.Sequence[1]
	assert.Equal(t, "parallel_process", parallelNode.ID)
	require.NotNil(t, parallelNode.Parallel)
	assert.Len(t, parallelNode.Parallel, 2)
	
	// Check nested sequence in parallel
	branch1 := parallelNode.Parallel[0]
	assert.Equal(t, "branch1", branch1.ID)
	require.NotNil(t, branch1.Sequence)
	assert.Len(t, branch1.Sequence, 2)
}

func TestValidationErrors(t *testing.T) {
	testCases := []struct {
		name        string
		yamlContent string
		expectError string
	}{
		{
			name: "no root node",
			yamlContent: `
name: invalid
version: "1.0"
`,
			expectError: "recipe must have one of",
		},
		{
			name: "multiple root nodes",
			yamlContent: `
name: invalid
version: "1.0"
op: some_op
sequence:
  - id: step1
    op: another_op
`,
			expectError: "exactly one of",
		},
		{
			name: "operation with outputs",
			yamlContent: `
name: invalid
version: "1.0"
op: some_op
outputs:
  result: "value"
`,
			expectError: "operation nodes cannot have outputs",
		},
	}
	
	parser := NewParser()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.ParseRecipeReader(strings.NewReader(tc.yamlContent))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectError)
		})
	}
}