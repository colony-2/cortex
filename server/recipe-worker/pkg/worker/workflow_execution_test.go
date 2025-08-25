package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestWorkflowExecution_Basic(t *testing.T) {
	// Test basic unified recipe definition creation
	recipeDef := &yamlpkg.RecipeDefinition{
		Node: yamlpkg.Node{
			ID:   "test-execution",
			Desc: "Test execution recipe",
			Op:   "test-activity",
			Inputs: map[string]interface{}{
				"type": "function",
				"data": "test",
			},
		},
		Version: "1.0",
	}

	assert.NotNil(t, recipeDef)
	assert.Equal(t, "test-execution", recipeDef.ID)
	assert.Equal(t, "test-activity", recipeDef.Op)
	assert.NotNil(t, recipeDef.Inputs)
}

func TestWorkflowExecution_Shared(t *testing.T) {
	// Test recipe with shared activities
	recipeDef := &yamlpkg.RecipeDefinition{
		Node: yamlpkg.Node{
			ID: "shared-execution",
			Sequence: []yamlpkg.Node{
				{
					ID:     "analyze",
					Shared: "my_llm",
					Inputs: map[string]interface{}{
						"prompt": "test",
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

	assert.NotNil(t, recipeDef)
	assert.Len(t, recipeDef.Defs, 1)
	assert.Contains(t, recipeDef.Defs, "my_llm")
	assert.Equal(t, "llm", recipeDef.Defs["my_llm"].Op)
}