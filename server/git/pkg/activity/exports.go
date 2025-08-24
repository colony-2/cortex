package activity

import (
	"github.com/divisive-ai/vibethis/server/git/pkg/gitcollector"
	"github.com/divisive-ai/vibethis/server/git/pkg/gitcommit"
	"github.com/divisive-ai/vibethis/server/git/pkg/gitshallow"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
)

// GetAll returns all Git activities available in this module
// This provides a single entry point for consumers to discover and register all Git activities
func GetAll() []types.RegisterableOp {
	return []types.RegisterableOp{
		gitcollector.GetOp(),
		gitshallow.GetOp(),
		gitcommit.GetPersistOp(),
		gitcommit.GetRestoreOp(),
	}
}
