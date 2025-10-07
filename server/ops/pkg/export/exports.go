package export

import (
	"github.com/divisive-ai/vibethis/server/ops/pkg/extensions"
	"github.com/divisive-ai/vibethis/server/ops/pkg/input"
	"github.com/divisive-ai/vibethis/server/ops/pkg/llm"
	"github.com/divisive-ai/vibethis/server/ops/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/ops/pkg/recipeset"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

// GetAll returns all activities available in this module
// This provides a single entry point for consumers like recipe-worker to discover and register all activities
func GetAll() []ops.RegisterableOp {
	base := []ops.RegisterableOp{
		llm.GetOp(),
		llm.GetEnhancedOp(),
		recipe.GetOp(),
		recipeset.GetOp(),
		input.GetOp(),
	}
	// Discover extension ops under .vibethis/ops and append them
	if discovered, err := extensions.Discover(""); err == nil && len(discovered) > 0 {
		base = append(base, discovered...)
	}
	return base
}
