package ops

import (
	"github.com/divisive-ai/vibethis/server/ops/pkg/input"
	"github.com/divisive-ai/vibethis/server/ops/pkg/llm"
	"github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
)

// GetAll returns all activities available in this module
// This provides a single entry point for consumers like recipe-worker to discover and register all activities
func GetAll() []interface{} {
	return []interface{}{
		// LLM activities
		llm.NewLLMActivity(),

		// Recipe invocation activities
		recipe.NewRecipeActivity(),

		// Input collection activities
		input.newInputActivity(),
	}
}
