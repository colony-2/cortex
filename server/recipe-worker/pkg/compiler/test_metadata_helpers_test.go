package compiler

import "go.temporal.io/sdk/testsuite"

// Temporal metadata signals are no longer used in tests; keep a no-op helper to
// avoid refactoring callers while the suite migrates to SWF.
func primeDefaultMetadataSignal(env *testsuite.TestWorkflowEnvironment) {}
