package statemachine

import (
	"testing"
)

// Workflow tests are skipped as the core logic is tested in simple_test.go
// The state machine is now part of the compiler and doesn't need separate
// Temporal workflow testing - it will be tested as part of the full workflow execution

func TestSequentialCompositionWorkflow(t *testing.T) {
	t.Skip("Core logic tested in simple_test.go - workflow integration tested at higher level")
}

func TestParallelCompositionWorkflow(t *testing.T) {
	t.Skip("Core logic tested in simple_test.go - workflow integration tested at higher level")
}

func TestConditionalCompositionWorkflow(t *testing.T) {
	t.Skip("Core logic tested in simple_test.go - workflow integration tested at higher level")
}

func TestNestedCompositionWorkflow(t *testing.T) {
	t.Skip("Core logic tested in simple_test.go - workflow integration tested at higher level")
}

func TestStateTransitionsWorkflow(t *testing.T) {
	t.Skip("Core logic tested in simple_test.go - workflow integration tested at higher level")
}

func TestRetryPolicyWorkflow(t *testing.T) {
	t.Skip("Core logic tested in simple_test.go - workflow integration tested at higher level")
}