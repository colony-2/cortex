package activity

import (
	"github.com/divisive-ai/vibethis/server/activity/pkg/command"
	"github.com/divisive-ai/vibethis/server/activity/pkg/gitshallow"
	"github.com/divisive-ai/vibethis/server/activity/pkg/llm"
	"github.com/divisive-ai/vibethis/server/activity/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/activity/pkg/statemachine"
)

// GetAll returns all activities available in this module
// This provides a single entry point for consumers like recipe-worker to discover and register all activities
func GetAll() []interface{} {
	// Create a default executor for state machine
	executor := statemachine.NewDefaultExecutor()
	stateMachineActivity, _ := statemachine.NewStateMachineActivity(executor)
	
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
		
		// State machine activities
		stateMachineActivity,
	}
}