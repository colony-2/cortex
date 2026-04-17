package export

import (
	"github.com/colony-2/c2j/pkg/ops"
	"github.com/colony-2/colony2/server/cell-ops/pkg/cells"
)

// GetAll returns all ops owned by the cell-ops module.
func GetAll() []ops.RegisterableOp {
	return []ops.RegisterableOp{
		cells.GetListOp(),
	}
}
