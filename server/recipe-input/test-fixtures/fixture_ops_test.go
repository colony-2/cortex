package test_fixtures_test

import (
	"context"
	"sync"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	childops "github.com/colony-2/colony2/server/recipe-child/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/commandop"
)

type fixtureOps struct {
	inputOp coreops.RegisterableOp
}

var (
	fixtureOpsOnce sync.Once
	fixtureOpsInst fixtureOps
)

func ensureFixtureOps() fixtureOps {
	fixtureOpsOnce.Do(func() {
		coreops.Register(childops.GetOps()...)

		fixtureOpsInst.inputOp = input.GetOp()
		coreops.Register(fixtureOpsInst.inputOp)
		coreops.Register(input.GetAutoFillOp())
		coreops.Register(commandop.GetOp())

		coreops.Register(coreops.NewActivityMappedOpV2[echoInput, echoOutput](
			coreops.OpMetadata{Type: "echo"},
			func(_ coreops.OpDependencies, _ context.Context, in echoInput) (echoOutput, error) {
				return echoOutput{Output: in.Message}, nil
			},
		))
	})
	return fixtureOpsInst
}

