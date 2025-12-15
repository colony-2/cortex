package export

import (
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/commandop"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/sleepop"
)

func GetAll() []ops.RegisterableOp {
	return []ops.RegisterableOp{
		commandop.GetOp(),
		sleepop.GetOp(),
	}
}
