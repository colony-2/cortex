package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestWorkflowExecution_Basic(t *testing.T) {
	// Test basic unified recipe definition creation
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:        "test-execution",
		Description: "Test execution recipe",
		Version:     "1.0",
		Op:          "test-activity",
		Inputs: map[string]interface{}{
			"type": "function",
			"data": "test",
		},
	}

	assert.NotNil(t, recipeDef)
	assert.Equal(t, "test-execution", recipeDef.Name)
	assert.Equal(t, "test-activity", recipeDef.Op)
	assert.NotNil(t, recipeDef.Inputs)
}

func TestWorkflowExecution_Shared(t *testing.T) {
	// Test recipe with shared activities
	recipeDef := &yamlpkg.RecipeDefinition{
		Name:    "shared-execution",
		Version: "1.0",
		Shared: map[string]yamlpkg.Node{
			"my_llm": {
				Op: "llm",
				Inputs: map[string]interface{}{
					"type":  "ai_prompt",
					"model": "gpt-4",
				},
			},
		},
		Sequence: []yamlpkg.Node{
			{
				ID:     "analyze",
				Shared: "my_llm",
				Inputs: map[string]interface{}{
					"prompt": "test",
				},
			},
		},
	}

	assert.NotNil(t, recipeDef)
	assert.Len(t, recipeDef.Shared, 1)
	assert.Contains(t, recipeDef.Shared, "my_llm")
	assert.Equal(t, "llm", recipeDef.Shared["my_llm"].Op)
}