package export

import (
	"github.com/colony-2/colony2/server/cell-ops/pkg/cells"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

// GetAll returns all ops owned by the cell-ops module.
func GetAll() []ops.RegisterableOp {
	return []ops.RegisterableOp{
		cells.GetListOp(),
	}
}
