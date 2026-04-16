package c2jops

import (
	gitexport "github.com/colony-2/colony2/server/git/pkg/export"
	"github.com/colony-2/colony2/server/ops/pkg/extensions"
	"github.com/colony-2/colony2/server/recipe-child/pkg/recipe"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	workerexport "github.com/colony-2/colony2/server/recipe-worker/pkg/export"
)

// Register resets the global op registry and installs the c2j-safe op set.
func Register() []coreops.RegisterableOp {
	impls := Ops()
	coreops.Replace(impls...)
	return impls
}

// Ops returns the ops that are safe to register in the c2j runtime.
func Ops() []coreops.RegisterableOp {
	impls := []coreops.RegisterableOp{extensions.GetExecutionOp()}
	impls = append(impls, workerexport.GetAll()...)
	impls = append(impls, input.GetOp())
	impls = append(impls, input.GetAutoFillOp())
	impls = append(impls, recipe.GetOps()...)
	impls = append(impls, gitexport.GetAll()...)
	return impls
}
