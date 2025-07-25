package activity

import (
	"github.com/vibethis/server/activity/pkg/command"
	"github.com/vibethis/server/activity/pkg/gitshallow"
	"github.com/vibethis/server/activity/pkg/llm"
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
	}
}