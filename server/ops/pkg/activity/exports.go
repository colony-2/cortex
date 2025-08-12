package activity

import (
	"github.com/divisive-ai/vibethis/server/ops/pkg/command"
	"github.com/divisive-ai/vibethis/server/ops/pkg/gitshallow"
	"github.com/divisive-ai/vibethis/server/ops/pkg/input"
	"github.com/divisive-ai/vibethis/server/ops/pkg/llm"
	"github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/ops/pkg/sleep"
)

// GetAll returns all activities available in this module
// This provides a single entry point for consumers like recipe-worker to discover and register all activities
func GetAll() []interface{} {
	return []interface{}{
		// LLM activities
		llm.NewLLMActivity(),
		
		// Git activities
		gitshallow.NewGitShallowActivity(),
		
		// Command activities
		command.NewCommandExecutionActivity(),
		
		// Recipe invocation activities
		recipe.NewRecipeActivity(),
		
		// Input collection activities
		input.NewInputActivity(),
		
		// Sleep/delay activities
		sleep.NewSleepActivity(),
		
		// Note: State machine is no longer an activity - it's now part of the core compiler
		// See server/recipe-worker/pkg/compiler/statemachine for the new implementation
	}
}