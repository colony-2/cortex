package activity

import (
	"github.com/divisive-ai/vibethis/server/git/pkg/gitcommit"
	"github.com/divisive-ai/vibethis/server/git/pkg/gitshallow"
)

// GetAll returns all Git activities available in this module
// This provides a single entry point for consumers like recipe-worker to discover and register all Git activities
func GetAll() []interface{} {
	return []interface{}{
		// Git shallow clone activity
		gitshallow.NewGitShallowActivity(),
		
		// Git persist commit activity
		gitcommit.NewPersistCommitActivity(),
		
		// Git restore commit activity
		gitcommit.NewRestoreCommitActivity(),
	}
}