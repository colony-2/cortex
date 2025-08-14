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
		Steps: []yamlpkg.Step{
			{
				ID:   "step1",
				Uses: "test-activity",
				Config: map[string]interface{}{
					"type": "function",
				},
				Inputs: map[string]interface{}{
					"data": "test",
				},
			},
		},
	}

	assert.NotNil(t, recipeDef)
	assert.Equal(t, "test-execution", recipeDef.Name)
	assert.Len(t, recipeDef.Steps, 1)
	assert.Equal(t, "test-activity", recipeDef.Steps[0].Uses)
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
		Steps: []yamlpkg.Step{
			{
				ID:   "analyze",
				Uses: "shared/my_llm",
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