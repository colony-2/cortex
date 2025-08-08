package activity

import (
	"github.com/divisive-ai/vibethis/server/activity/pkg/command"
	"github.com/divisive-ai/vibethis/server/activity/pkg/gitshallow"
	"github.com/divisive-ai/vibethis/server/activity/pkg/input"
	"github.com/divisive-ai/vibethis/server/activity/pkg/llm"
	"github.com/divisive-ai/vibethis/server/activity/pkg/recipe"
)

// GetAll returns all activities available in this module
// This provides a single entry point for consumers like recipe-worker to discover and register all activities
func GetAll() []interface{} {
	return []interface{}{
		// LLM activities
		llm.NewLLMActivity(),
		llm.NewGitShallowCloneActivity(),
		
		// Git activities
		gitshallow.NewGitShallowActivity(),
		
		// Command activities
		command.NewCommandExecutionActivity(),
		
		// Recipe invocation activities
		recipe.NewRecipeActivity(),
		
		// Input collection activities
		input.NewInputActivity(),
		
		// Note: State machine is no longer an activity - it's now part of the core compiler
		// See server/recipe-worker/pkg/compiler/statemachine for the new implementation
	}
}