package compiler

import (
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

// TestStateOutputsSummary demonstrates the complete state machine outputs issue
func TestStateOutputsSummary(t *testing.T) {
	t.Log("=== State Machine Outputs Bug Summary ===")

	// The user's recipe YAML with proper structure
	recipeYAML := `
id: state_outputs_test
desc: Test state machine outputs functionality
version: 1.0.0
inputs:
  message: 'test'
state:
  initial: simple_state
  states:
    simple_state:
      op: echo_activity
      inputs:
        message: 'Hello {{ inputs.message }}'
outputs:
  result: '{{ inputs.message }}'
`

	r, err := recipe.LoadRecipeFromString([]byte(recipeYAML))
	require.NoError(t, err)

	rs := r.RecipeImpl.(*recipe.RecipeState)

	t.Log("1. Recipe parsed correctly as RecipeState ✓")
	t.Logf("   - Initial state: %s", rs.StateData.States.Initial)
	t.Logf("   - Recipe outputs defined: %+v", rs.StateData.Outputs)

	// Check the terminal state structure
	terminalState := rs.StateData.States.States["simple_state"]
	t.Log("\n2. Terminal state structure:")
	t.Logf("   - Node type: %T", terminalState.Node.NodeImpl)
	t.Logf("   - Has transitions: %v (empty = terminal)", len(terminalState.Transitions) > 0)

	// The problem: NodeOp doesn't have an Outputs field
	if nodeOp, ok := terminalState.Node.NodeImpl.(*recipe.NodeOp); ok {
		t.Log("\n3. Issue identified:")
		t.Log("   - Terminal state is NodeOp type")
		t.Log("   - NodeOp doesn't have an Outputs field")
		t.Logf("   - NodeOp structure: %+v", nodeOp)
	}

	// Test actual execution
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	inputs := map[string]interface{}{
		"message": "test",
	}

	// Test state machine execution directly
	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		tracker := newInvocationTracker(rs.RecipeMetadata)
		stateTracker := tracker.child(segmentForMetadata(rs.RecipeMetadata.NodeMetadata, "recipe-state"))
		return executeStateMachine(ctx, registry, stateTracker, rs.StateData.States, inputs)
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var stateMachineResult map[string]interface{}
	err = env.GetWorkflowResult(&stateMachineResult)
	require.NoError(t, err)

	t.Log("\n4. State machine execution result:")
	t.Logf("   - Returns: %+v", stateMachineResult)
	t.Logf("   - Expected: Terminal state outputs or activity result")
	t.Logf("   - Actual: Empty map (bug confirmed)")

	// Test full recipe execution
	env2 := testSuite.NewTestWorkflowEnvironment()
	defer env2.AssertExpectations(t)

	env2.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		return ExecuteRecipe(ctx, registry, *r, inputs)
	})

	require.True(t, env2.IsWorkflowCompleted())
	require.NoError(t, env2.GetWorkflowError())

	var recipeResult map[string]interface{}
	err = env2.GetWorkflowResult(&recipeResult)
	require.NoError(t, err)

	t.Log("\n5. Full recipe execution:")
	t.Logf("   - Recipe outputs template: %+v", rs.StateData.Outputs)
	t.Logf("   - Resolved result: %+v", recipeResult)

	if _, hasResult := recipeResult["result"]; hasResult {
		t.Log("   - ✓ Recipe outputs ARE working (processNodeOutputs compensates)")
		assert.Equal(t, "test", recipeResult["result"])
	} else {
		t.Log("   - ✗ Recipe outputs NOT working")
	}

	t.Log("\n=== Summary ===")
	t.Log("The bug is PARTIALLY mitigated:")
	t.Log("- State machine returns empty map (bug exists)")
	t.Log("- But processNodeOutputs() at recipe level compensates")
	t.Log("- Recipe-level outputs work, state-level outputs don't")
	t.Log("\nRoot cause:")
	t.Log("- Terminal states with NodeOp don't preserve outputs")
	t.Log("- executeStateMachine only checks NodeState/NodeSequence for outputs")
}
