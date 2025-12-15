package export

import (
	"github.com/colony-2/colony2/server/git/pkg/gitcollector"
	"github.com/colony-2/colony2/server/git/pkg/gitcommit"
	"github.com/colony-2/colony2/server/git/pkg/gitshallow"
	"github.com/colony-2/colony2/server/git/pkg/squashrebasemerge"
	"github.com/colony-2/colony2/server/git/pkg/thinpackrebase"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

// GetAll returns all Git activities available in this module
// This provides a single entry point for consumers to discover and register all Git activities
func GetAll() []ops.RegisterableOp {
	return []ops.RegisterableOp{
		gitcollector.GetOp(),
		gitshallow.GetOp(),
		gitcommit.GetPersistOp(),
		gitcommit.GetRestoreOp(),
		thinpackrebase.GetOp(),
		squashrebasemerge.GetOp(),
	}
}
