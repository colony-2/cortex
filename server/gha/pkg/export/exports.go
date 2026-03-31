package export

import (
	"github.com/colony-2/colony2/server/gha/pkg/gha"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

// GetAll returns all GitHub Actions ops exposed by this module.
func GetAll() []ops.RegisterableOp {
	return []ops.RegisterableOp{
		gha.GetOp(),
		gha.GetRunJobOp(),
		gha.GetRunsOp(),
	}
}
