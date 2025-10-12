package compiler

import (
	"time"

	"go.temporal.io/sdk/testsuite"
)

func signalMetadataForTest(env *testsuite.TestWorkflowEnvironment, payload RecipeRunMetadataSignal) {
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(recipeRunMetadataSignalName, payload)
	}, time.Millisecond)
}

func primeDefaultMetadataSignal(env *testsuite.TestWorkflowEnvironment) {
	signalMetadataForTest(env, RecipeRunMetadataSignal{})
}
