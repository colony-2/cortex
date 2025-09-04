package export

import (
	"github.com/divisive-ai/vibethis/server/ops/pkg/input"
	"github.com/divisive-ai/vibethis/server/ops/pkg/llm"
	"github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

// GetAll returns all activities available in this module
// This provides a single entry point for consumers like recipe-worker to discover and register all activities
func GetAll() []ops.RegisterableOp {
	return []ops.RegisterableOp{
		llm.GetOp(),
		llm.GetEnhancedOp(),
		recipe.GetOp(),
		input.GetOp(),
	}
}
