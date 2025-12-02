//go:build temporal
// +build temporal

package compiler

import (
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

// TestStateOutputsDebug shows the exact issue with the user's recipe
func TestStateOutputsDebug(t *testing.T) {
	t.Log("=== DEBUGGING USER'S RECIPE ===")

	// This is the exact recipe the user provided
	userRecipeYAML := `
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

	t.Log("User's Recipe Structure:")
	t.Log("- Has 'state:' section (makes it a RecipeState)")
	t.Log("- Has 'outputs:' at recipe level")
	t.Log("- Terminal state 'simple_state' is a simple op (NodeOp)")
	t.Log("")

	// Parse the recipe
	r, err := recipe.LoadRecipeFromString([]byte(userRecipeYAML))
	require.NoError(t, err)

	rs := r.RecipeImpl.(*recipe.RecipeState)

	t.Log("Parsed Recipe:")
	t.Logf("- Type: %T", r.RecipeImpl)
	t.Logf("- Initial state: %s", rs.StateData.States.Initial)
	t.Logf("- Recipe-level outputs: %+v", rs.StateData.Outputs)
	t.Log("")

	// Setup test environment
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeDefaultMetadataSignal(env)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	inputs, execCtx := withRequiredGitInputs(map[string]interface{}{
		"message": "test",
	})

	// Test 1: What does executeStateMachine return?
	t.Log("Test 1: Direct state machine execution")
	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		tracker := newInvocationTracker(rs.RecipeMetadata, nil)
		stateTracker := tracker.child(segmentForMetadata(rs.RecipeMetadata.NodeMetadata, "recipe-state"))
		return executeStateMachine(ctx, registry, stateTracker, rs.StateData.States, inputs, execCtx)
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var stateMachineResult map[string]interface{}
	err = env.GetWorkflowResult(&stateMachineResult)
	require.NoError(t, err)

	t.Logf("executeStateMachine returns: %+v", stateMachineResult)
	if len(stateMachineResult) == 0 {
		t.Log("❌ PROBLEM: State machine returns empty map!")
		t.Log("   Expected: The echo_activity output or something meaningful")
	}
	t.Log("")

	// Test 2: What does the full recipe execution return?
	t.Log("Test 2: Full recipe execution (through ExecuteRecipe)")
	env2 := testSuite.NewTestWorkflowEnvironment()
	defer env2.AssertExpectations(t)
	primeDefaultMetadataSignal(env2)

	env2.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		return ExecuteRecipe(ctx, registry, *r, inputs, execCtx)
	})

	require.True(t, env2.IsWorkflowCompleted())
	require.NoError(t, env2.GetWorkflowError())

	var recipeResult map[string]interface{}
	err = env2.GetWorkflowResult(&recipeResult)
	require.NoError(t, err)

	t.Logf("ExecuteRecipe returns: %+v", recipeResult)
	if val, ok := recipeResult["result"]; ok {
		t.Logf("✓ Recipe outputs work: result = %v", val)
		t.Log("  This is because processNodeOutputs() processes the recipe-level outputs")
	} else {
		t.Log("❌ Recipe outputs don't work")
	}
	t.Log("")

	t.Log("=== ISSUE SUMMARY ===")
	t.Log("The user expects: {'result': 'test'}")
	t.Log("")
	t.Log("What's happening:")
	t.Log("1. State machine executes the terminal state (echo_activity)")
	t.Log("2. executeStateMachine returns an empty map (the bug)")
	t.Log("3. ExecuteRecipe calls processNodeOutputs with:")
	t.Log("   - outputs: {} (empty from state machine)")
	t.Log("   - outputTemplates: {'result': '{{ inputs.message }}'}")
	t.Log("4. processNodeOutputs resolves the templates and returns {'result': 'test'}")
	t.Log("")
	t.Log("So the recipe DOES work, but only because processNodeOutputs saves it.")
	t.Log("The state machine itself is not returning the activity outputs.")
}

// TestWhatShouldStateMachineReturn shows what we expect from the state machine
func TestWhatShouldStateMachineReturn(t *testing.T) {
	t.Log("=== WHAT SHOULD THE STATE MACHINE RETURN? ===")
	t.Log("")
	t.Log("For a terminal state that's an operation (NodeOp):")
	t.Log("- The state machine should return the activity's output")
	t.Log("- Currently it returns an empty map")
	t.Log("")
	t.Log("The bug is in executeStateMachine() at statemachine_compiler.go:73-107")
	t.Log("- Lines 76-81: Only check NodeState and NodeSequence for outputs")
	t.Log("- NodeOp (the user's case) falls through to line 97")
	t.Log("- Line 97-99: Tries to return stored state outputs, but none exist")
	t.Log("- Falls through to return empty map")
	t.Log("")
	t.Log("The fix needed:")
	t.Log("- When terminal state is NodeOp, return the activity execution result")
	t.Log("- OR store the activity result in resCtx.TemplateData.States")
}
