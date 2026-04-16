package opregistry

import (
	cellopsexport "github.com/colony-2/colony2/server/cell-ops/pkg/export"
	gitexport "github.com/colony-2/colony2/server/git/pkg/export"
	"github.com/colony-2/colony2/server/ops/pkg/extensions"
	"github.com/colony-2/colony2/server/recipe-child/pkg/recipe"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	workerexport "github.com/colony-2/colony2/server/recipe-worker/pkg/export"
	ticketop "github.com/colony-2/colony2/server/ticket/pkg/op"
)

type Profile string

const (
	ProfileServer Profile = "server"
	ProfileC2J    Profile = "c2j"
)

// Register resets the global op registry and installs the requested profile.
func Register(profile Profile) []coreops.RegisterableOp {
	impls := OpsForProfile(profile)
	coreops.Replace(impls...)
	return impls
}

// OpsForProfile returns the ops that should be available for the given runtime profile.
func OpsForProfile(profile Profile) []coreops.RegisterableOp {
	switch profile {
	case ProfileC2J:
		return c2jOps()
	case ProfileServer, "":
		fallthrough
	default:
		return serverOps()
	}
}

func serverOps() []coreops.RegisterableOp {
	impls := []coreops.RegisterableOp{extensions.GetExecutionOp()}
	impls = append(impls, cellopsexport.GetAll()...)
	impls = append(impls, workerexport.GetAll()...)
	impls = append(impls, input.GetOp())
	impls = append(impls, input.GetAutoFillOp())
	impls = append(impls, recipe.GetOps()...)
	impls = append(impls, gitexport.GetAll()...)
	impls = append(impls, ticketop.GetOp())
	return impls
}

func c2jOps() []coreops.RegisterableOp {
	impls := []coreops.RegisterableOp{extensions.GetExecutionOp()}
	impls = append(impls, workerexport.GetAll()...)
	impls = append(impls, input.GetOp())
	impls = append(impls, input.GetAutoFillOp())
	impls = append(impls, recipe.GetOps()...)
	impls = append(impls, gitexport.GetAll()...)
	return impls
}
