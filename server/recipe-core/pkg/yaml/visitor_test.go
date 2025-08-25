package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSharedReferences(t *testing.T) {
	tests := []struct {
		name          string
		yamlContent   string
		expectError   bool
		errorContains string
		validate      func(t *testing.T, resolved *RecipeDefinition)
	}{
		{
			name: "simple shared reference resolution",
			yamlContent: `
id: test-recipe
version: "1.0.0"
defs:
  validator:
    op: validation_activity
    inputs:
      type: schema
sequence:
  - id: step1
    shared: validator
    inputs:
      data: "test"
`,
			expectError: false,
			validate: func(t *testing.T, resolved *RecipeDefinition) {
				// Defs should be cleared
				assert.Nil(t, resolved.Defs)
				
				// Sequence should have resolved node
				require.Len(t, resolved.Sequence, 1)
				assert.Equal(t, "step1", resolved.Sequence[0].ID)
				assert.Equal(t, "validation_activity", resolved.Sequence[0].Op)
				assert.Equal(t, "", resolved.Sequence[0].Shared) // Shared ref should be cleared
				
				// Inputs should be merged
				assert.Equal(t, "schema", resolved.Sequence[0].Inputs["type"])
				assert.Equal(t, "test", resolved.Sequence[0].Inputs["data"])
			},
		},
		{
			name: "nested shared references",
			yamlContent: `
id: nested-test
version: "1.0.0"
defs:
  processor:
    op: llm
    inputs:
      model: gpt-4
  workflow:
    sequence:
      - id: validate
        op: validator
      - id: process
        shared: processor
sequence:
  - id: main
    shared: workflow
`,
			expectError: false,
			validate: func(t *testing.T, resolved *RecipeDefinition) {
				assert.Nil(t, resolved.Defs)
				require.Len(t, resolved.Sequence, 1)
				
				mainNode := resolved.Sequence[0]
				assert.Equal(t, "main", mainNode.ID)
				assert.Empty(t, mainNode.Shared)
				
				// Should have inherited sequence from workflow
				require.Len(t, mainNode.Sequence, 2)
				assert.Equal(t, "validate", mainNode.Sequence[0].ID)
				assert.Equal(t, "validator", mainNode.Sequence[0].Op)
				
				// Nested shared ref should also be resolved
				assert.Equal(t, "process", mainNode.Sequence[1].ID)
				assert.Equal(t, "llm", mainNode.Sequence[1].Op)
				assert.Empty(t, mainNode.Sequence[1].Shared)
				assert.Equal(t, "gpt-4", mainNode.Sequence[1].Inputs["model"])
			},
		},
		{
			name: "missing shared reference",
			yamlContent: `
id: test-recipe
version: "1.0.0"
defs:
  validator:
    op: validation_activity
sequence:
  - id: step1
    shared: non_existent
`,
			expectError:   true,
			errorContains: "shared node reference 'non_existent' not found",
			validate:      nil,
		},
		{
			name: "shared reference with overrides",
			yamlContent: `
id: test-recipe
version: "1.0.0"
defs:
  base_processor:
    op: llm
    inputs:
      model: gpt-3.5
      temperature: 0.7
    timeout: 30s
parallel:
  - id: task1
    shared: base_processor
    inputs:
      prompt: "Task 1"
      temperature: 0.9
  - id: task2
    shared: base_processor
    inputs:
      prompt: "Task 2"
`,
			expectError: false,
			validate: func(t *testing.T, resolved *RecipeDefinition) {
				assert.Nil(t, resolved.Defs)
				require.Len(t, resolved.Parallel, 2)
				
				// First task with overrides
				task1 := resolved.Parallel[0]
				assert.Equal(t, "task1", task1.ID)
				assert.Equal(t, "llm", task1.Op)
				assert.Empty(t, task1.Shared)
				assert.Equal(t, "gpt-3.5", task1.Inputs["model"])
				assert.Equal(t, 0.9, task1.Inputs["temperature"]) // Override
				assert.Equal(t, "Task 1", task1.Inputs["prompt"])
				
				// Second task with base values
				task2 := resolved.Parallel[1]
				assert.Equal(t, "task2", task2.ID)
				assert.Equal(t, "llm", task2.Op)
				assert.Empty(t, task2.Shared)
				assert.Equal(t, "gpt-3.5", task2.Inputs["model"])
				assert.Equal(t, 0.7, task2.Inputs["temperature"]) // Base value
				assert.Equal(t, "Task 2", task2.Inputs["prompt"])
			},
		},
		{
			name: "shared reference in state machine",
			yamlContent: `
id: state-recipe
version: "1.0.0"
defs:
  validator:
    op: validation_activity
    inputs:
      strict: true
states:
  initial: start
  start:
    shared: validator
    transitions:
      - to: end
  end:
    op: finalizer
`,
			expectError: false,
			validate: func(t *testing.T, resolved *RecipeDefinition) {
				assert.Nil(t, resolved.Defs)
				require.NotNil(t, resolved.States)
				
				startState, exists := resolved.States.States["start"]
				require.True(t, exists)
				assert.Equal(t, "validation_activity", startState.Op)
				assert.Empty(t, startState.Shared)
				assert.Equal(t, true, startState.Inputs["strict"])
				assert.Len(t, startState.Transitions, 1)
			},
		},
		{
			name: "no defs - recipe unchanged",
			yamlContent: `
id: simple-recipe
version: "1.0.0"
op: simple_activity
inputs:
  key: value
`,
			expectError: false,
			validate: func(t *testing.T, resolved *RecipeDefinition) {
				assert.Nil(t, resolved.Defs)
				assert.Equal(t, "simple-recipe", resolved.ID)
				assert.Equal(t, "simple_activity", resolved.Op)
				assert.Equal(t, "value", resolved.Inputs["key"])
			},
		},
		{
			name: "complex nested shared resolution",
			yamlContent: `
id: complex-recipe
version: "1.0.0"
defs:
  http_base:
    op: http
    inputs:
      method: GET
      headers:
        Accept: application/json
  api_call:
    shared: http_base
    inputs:
      url: "https://api.example.com"
  workflow:
    sequence:
      - id: fetch_data
        shared: api_call
        inputs:
          headers:
            Authorization: Bearer token
      - id: process
        op: processor
sequence:
  - id: main_workflow
    shared: workflow
`,
			expectError: false,
			validate: func(t *testing.T, resolved *RecipeDefinition) {
				assert.Nil(t, resolved.Defs)
				require.Len(t, resolved.Sequence, 1)
				
				mainWorkflow := resolved.Sequence[0]
				require.Len(t, mainWorkflow.Sequence, 2)
				
				// Check deeply nested resolution
				fetchData := mainWorkflow.Sequence[0]
				assert.Equal(t, "fetch_data", fetchData.ID)
				assert.Equal(t, "http", fetchData.Op)
				assert.Empty(t, fetchData.Shared)
				
				// Check input merging through multiple levels
				assert.Equal(t, "GET", fetchData.Inputs["method"])
				assert.Equal(t, "https://api.example.com", fetchData.Inputs["url"])
				
				headers, ok := fetchData.Inputs["headers"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "Bearer token", headers["Authorization"])
				// The Accept header should be preserved from base
				// Note: This depends on how mergeInputs handles nested maps
			},
		},
		{
			name: "circular reference detection",
			yamlContent: `
id: circular-recipe
version: "1.0.0"
defs:
  def1:
    shared: def2
  def2:
    shared: def1
sequence:
  - id: step1
    shared: def1
`,
			expectError:   true,
			errorContains: "circular reference detected",
			validate:      nil,
		},
		{
			name: "self-referencing def",
			yamlContent: `
id: self-ref-recipe
version: "1.0.0"
defs:
  recursive:
    shared: recursive
sequence:
  - id: step1
    shared: recursive
`,
			expectError:   true,
			errorContains: "circular reference detected",
			validate:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the YAML
			parser := NewParser()
			recipe, err := parser.ParseRecipeReader(strings.NewReader(tt.yamlContent))
			require.NoError(t, err)

			// Resolve shared references
			resolved, err := ResolveSharedReferences(recipe)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, resolved)
				if tt.validate != nil {
					tt.validate(t, resolved)
				}
			}
		})
	}
}

// nodeCounter is a test visitor that counts nodes
type nodeCounter struct {
	count     int
	nodeTypes map[string]int
}

func (nc *nodeCounter) PreVisit(node *Node, path []string) bool {
	return true // Always visit children
}

func (nc *nodeCounter) PostVisit(node *Node, path []string) error {
	return nil
}

func (nc *nodeCounter) VisitNode(node *Node, path []string) (*Node, error) {
	nc.count++
	
	if node.Op != "" {
		nc.nodeTypes["op"]++
	}
	if len(node.Sequence) > 0 {
		nc.nodeTypes["sequence"]++
	}
	if len(node.Parallel) > 0 {
		nc.nodeTypes["parallel"]++
	}
	if node.States != nil {
		nc.nodeTypes["states"]++
	}
	if node.Shared != "" {
		nc.nodeTypes["shared"]++
	}
	
	return node, nil
}

// TestVisitorPattern tests the visitor pattern implementation directly
func TestVisitorPattern(t *testing.T) {

	yamlContent := `
id: test-recipe
version: "1.0.0"
defs:
  processor:
    op: llm
sequence:
  - id: step1
    op: validator
  - id: step2
    parallel:
      - id: task1
        op: activity1
      - id: task2
        op: activity2
  - id: step3
    shared: processor
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)

	counter := &nodeCounter{
		nodeTypes: make(map[string]int),
	}

	_, err = recipe.Accept(counter)
	require.NoError(t, err)

	// Verify counts
	assert.Greater(t, counter.count, 0, "Should have visited nodes")
	assert.Greater(t, counter.nodeTypes["op"], 0, "Should have found op nodes")
	assert.Greater(t, counter.nodeTypes["sequence"], 0, "Should have found sequence nodes")
	assert.Greater(t, counter.nodeTypes["parallel"], 0, "Should have found parallel nodes")
	assert.Greater(t, counter.nodeTypes["shared"], 0, "Should have found shared references")
}

// errorVisitor is a test visitor that returns errors
type errorVisitor struct{}

func (ev *errorVisitor) PreVisit(node *Node, path []string) bool {
	return true
}

func (ev *errorVisitor) PostVisit(node *Node, path []string) error {
	return nil
}

func (ev *errorVisitor) VisitNode(node *Node, path []string) (*Node, error) {
	if node.Op == "fail_here" {
		return nil, assert.AnError
	}
	return node, nil
}

// TestErrorPropagation tests that errors in visitor are properly propagated
func TestErrorPropagation(t *testing.T) {

	yamlContent := `
id: test-recipe
version: "1.0.0"
sequence:
  - id: step1
    op: validator
  - id: step2
    op: fail_here
`

	parser := NewParser()
	recipe, err := parser.ParseRecipeReader(strings.NewReader(yamlContent))
	require.NoError(t, err)

	visitor := &errorVisitor{}
	_, err = recipe.Accept(visitor)
	assert.Error(t, err, "Should propagate visitor error")
}