package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

func TestWorkflowExecution_Basic(t *testing.T) {
	// Test basic unified recipe definition creation
	recipeDef := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeOp{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: recipe.NodeMetadata{
					ID:   "test-execution",
					Desc: "Test execution recipe",
					Inputs: map[string]interface{}{
						"type": "function",
						"data": "test",
					},
				},
				Version: "1.0",
			},
			OpData: recipe.OpData{
				Op: "test-activity",
			},
		},
	}

	assert.NotNil(t, recipeDef)
	recipeOp := recipeDef.RecipeImpl.(*recipe.RecipeOp)
	assert.Equal(t, "test-execution", recipeOp.NodeMetadata.ID)
	assert.Equal(t, "test-activity", recipeOp.Op)
	assert.NotNil(t, recipeOp.NodeMetadata.Inputs)
}

func TestWorkflowExecution_Shared(t *testing.T) {
	// Test recipe with shared activities
	recipeDef := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeSequence{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: recipe.NodeMetadata{
					ID: "shared-execution",
				},
				Version: "1.0",
				Defs: map[string]recipe.Node{
					"my_llm": {
						NodeImpl: &recipe.NodeOp{
							NodeMetadata: recipe.NodeMetadata{
								Inputs: map[string]interface{}{
									"type":  "ai_prompt",
									"model": "gpt-4",
								},
							},
							OpData: recipe.OpData{
								Op: "llm",
							},
						},
					},
				},
			},
			SequenceData: recipe.SequenceData{
				Sequence: []recipe.Node{
					{
						NodeImpl: &recipe.NodeShared{
							Shared: "my_llm",
						},
					},
				},
			},
		},
	}

	assert.NotNil(t, recipeDef)
	recipeSeq := recipeDef.RecipeImpl.(*recipe.RecipeSequence)
	assert.Len(t, recipeSeq.RecipeMetadata.Defs, 1)
	assert.Contains(t, recipeSeq.RecipeMetadata.Defs, "my_llm")
	llmNode := recipeSeq.RecipeMetadata.Defs["my_llm"].NodeImpl.(*recipe.NodeOp)
	assert.Equal(t, "llm", llmNode.Op)
}