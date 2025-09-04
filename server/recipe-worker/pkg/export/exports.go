package export

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/commandop"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/sleepop"
)

func GetAll() []ops.RegisterableOp {
	return []ops.RegisterableOp{
		commandop.GetOp(),
		sleepop.GetOp(),
	}
}
