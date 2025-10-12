package gitstate

import (
	"time"

	"go.temporal.io/sdk/testsuite"
)

func primeRecipeMetadataSignal(env *testsuite.TestWorkflowEnvironment) {
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("recipe_run_metadata", map[string]interface{}{})
	}, time.Millisecond)
}
