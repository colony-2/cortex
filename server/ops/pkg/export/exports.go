package export

import (
	"github.com/colony-2/colony2/server/ops/pkg/codex"
	"github.com/colony-2/colony2/server/ops/pkg/extensions"
	"github.com/colony-2/colony2/server/ops/pkg/llm"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

// GetAll returns all activities available in this module
// This provides a single entry point for consumers like recipe-worker to discover and register all activities
func GetAll() []ops.RegisterableOp {
	base := []ops.RegisterableOp{
		codex.GetOp(),
		llm.GetOp(),
		llm.GetEnhancedOp(),
	}
	// Discover extension ops under .colony2/ops and append them
	if discovered, err := extensions.Discover(""); err == nil && len(discovered) > 0 {
		base = append(base, discovered...)
	}
	return base
}
