package compiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
)

func TestSimpleRecipeStructure(t *testing.T) {
	// Create a simple recipe definition using unified format
	node := &yamlpkg.Node{
		ID:   "test-recipe",
		Desc: "Test recipe",
		Op:   "test_activity",
		Inputs: map[string]interface{}{
			"type":   "function",
			"param1": "test_value",
		},
		Outputs: map[string]interface{}{
			"result": "activity_output",
		},
	}

	// Create activity registry
	registry := ops.NewActivityRegistry()
	
	// Verify the structure
	assert.NotNil(t, node)
	assert.Equal(t, "test-recipe", node.ID)
	assert.Equal(t, "test_activity", node.Op)
	assert.NotNil(t, registry)
}

func TestParallelRecipeStructure(t *testing.T) {
	node := &yamlpkg.Node{
		ID:       "parallel-recipe",
		Parallel: []yamlpkg.Node{
			{
				ID: "task_a",
				Op: "activity_a",
				Inputs: map[string]interface{}{
					"type": "function",
				},
				Outputs: map[string]interface{}{
					"result": "output_a",
				},
			},
			{
				ID: "task_b",
				Op: "activity_b",
				Inputs: map[string]interface{}{
					"type": "function",
				},
				Outputs: map[string]interface{}{
					"result": "output_b",
				},
			},
		},
	}

	// Verify the structure
	assert.NotNil(t, node)
	assert.Len(t, node.Parallel, 2)
	assert.Equal(t, "task_a", node.Parallel[0].ID)
	assert.Equal(t, "task_b", node.Parallel[1].ID)
}

func TestRecipeWithSharedDefinitions(t *testing.T) {
	recipeDef := &yamlpkg.RecipeDefinition{
		Node: yamlpkg.Node{
			ID: "shared-recipe",
			Sequence: []yamlpkg.Node{
				{
					ID:     "analyze",
					Shared: "my_llm",
					Inputs: map[string]interface{}{
						"prompt": "Analyze this data",
					},
				},
			},
		},
		Version: "1.0",
		Defs: map[string]yamlpkg.Node{
			"my_llm": {
				Op: "llm",
				Inputs: map[string]interface{}{
					"type":  "ai_prompt",
					"model": "gpt-4",
				},
			},
		},
	}

	// Verify the structure
	assert.NotNil(t, recipeDef)
	assert.Equal(t, "1.0", recipeDef.Version)
	assert.NotNil(t, recipeDef.Defs)
	assert.Len(t, recipeDef.Sequence, 1)
	assert.Contains(t, recipeDef.Defs, "my_llm")
}