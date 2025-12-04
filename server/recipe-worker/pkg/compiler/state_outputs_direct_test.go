//go:build test44

package compiler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDirectStateOutputStorage tests the storage and retrieval directly
func TestDirectStateOutputStorage(t *testing.T) {
	// Create a resolution context like the state machine does
	inputs := map[string]interface{}{
		"message": "test",
	}
	resCtx, err := NewResolutionContext("state_machine", "test-sm", inputs, ExecutionContext{})
	require.NoError(t, err)

	// Simulate what happens when an activity executes
	// This is what executeStateNode returns for a NodeOp
	activityOutputs := map[string]interface{}{
		"message": "Hello test",
		"status":  "success",
		"result":  "echo_result",
	}

	// Store the outputs like line 57 in statemachine_compiler.go
	t.Logf("Storing outputs: %+v", activityOutputs)
	resCtx.AddStateOutput("simple_state", activityOutputs)

	// Try to retrieve like the state machine does at line 100
	if lastState, ok := resCtx.TemplateData.States["simple_state"]; ok {
		t.Logf("Retrieved state: %+v", lastState)
		t.Logf("Retrieved outputs: %+v", lastState.Outputs)
		outMap := convertRawToMap(lastState.Outputs)
		if len(outMap) > 0 {
			t.Log("✓ Outputs successfully stored and retrieved")
		} else {
			t.Log("✗ Outputs are nil or empty!")
		}
	} else {
		t.Log("✗ State not found in TemplateData.States!")
	}

	// Also check what resCtx.TemplateData.States contains
	t.Logf("All states in context: %+v", resCtx.TemplateData.States)
}

// TestStateMachineFlowSimulation simulates the exact flow of the state machine
func TestStateMachineFlowSimulation(t *testing.T) {
	// Simulate the exact flow in executeStateMachine

	// 1. Create resolution context (line 14-18)
	inputs := map[string]interface{}{"message": "test"}
	resCtx, err := NewResolutionContext("state_machine", "sm-root", inputs, ExecutionContext{})
	require.NoError(t, err)

	// 2. Simulate state execution (what executeStateNode returns)
	stateOutputs := map[string]interface{}{
		"message": "Hello test",
		"status":  "success",
	}

	// 3. Store state outputs (line 57)
	currentState := "simple_state"
	resCtx.AddStateOutput(currentState, stateOutputs)

	// 4. Check if state is terminal (it is - no transitions)
	// This brings us to line 73-111

	// 5. Try to retrieve outputs (line 100-104)
	t.Log("=== Attempting to retrieve outputs like state machine does ===")

	// Check if we can get the state
	if lastState, ok := resCtx.TemplateData.States[currentState]; ok {
		t.Logf("Found state '%s' in context", currentState)
		t.Logf("State structure: %+v", lastState)

		// This is the check at line 102
		if len(convertRawToMap(lastState.Outputs)) > 0 {
			t.Logf("✓ Would return: %+v", lastState.Outputs)
		} else {
			t.Log("✗ Outputs are nil or empty - would not return")
		}
	} else {
		t.Logf("✗ State '%s' not found in context!", currentState)
	}

	// 6. If that fails, what happens at line 108-110?
	t.Log("\n=== Fallback: Return all state outputs ===")
	allOutputs := make(map[string]interface{})
	for stateName, stateOutput := range resCtx.TemplateData.States {
		allOutputs[stateName] = convertRawToMap(stateOutput.Outputs)
	}
	t.Logf("Would return all outputs: %+v", allOutputs)

	if len(allOutputs) == 0 {
		t.Log("✗ This explains the empty map return!")
	}
}
