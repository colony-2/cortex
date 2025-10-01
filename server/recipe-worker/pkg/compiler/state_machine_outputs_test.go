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

func TestStateOutputsIssue(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	// Create activity registry
	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	// User's recipe as YAML - outputs should be at same level as state
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

	// Parse the recipe
	r, err := recipe.LoadRecipeFromString([]byte(recipeYAML))
	require.NoError(t, err, "Failed to parse recipe YAML")

	// Log what type of recipe was parsed
	t.Logf("Recipe type: %T", r.RecipeImpl)

	// Check if it's a RecipeState
	if rs, ok := r.RecipeImpl.(*recipe.RecipeState); ok {
		t.Logf("RecipeState with initial: %s", rs.StateData.States.Initial)
		t.Logf("RecipeState outputs: %+v", rs.StateData.Outputs)
		t.Logf("RecipeState states: %+v", rs.StateData.States)
		if rs.StateData.States != nil {
			for name, state := range rs.StateData.States.States {
				t.Logf("State %s: %+v", name, state)
			}
		}
	}

	inputs := map[string]interface{}{
		"message": "test",
	}

	// Test the recipe execution
	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		return ExecuteRecipe(ctx, registry, *r, withRequiredGitInputs(inputs))
	})

	require.True(t, env.IsWorkflowCompleted())

	err = env.GetWorkflowError()
	if err != nil {
		t.Logf("Workflow error: %v", err)
		t.FailNow()
	}

	var result map[string]interface{}
	err = env.GetWorkflowResult(&result)
	require.NoError(t, err)

	t.Logf("Actual result: %+v", result)

	// Expected: {"result": "test"} from the outputs template
	// Check what we actually get
	assert.Contains(t, result, "result")

	// The outputs template should resolve inputs.message to "test"
	assert.Equal(t, "test", result["result"], "Should return the outputs template result")
}

func TestStateMachineInternalExecution(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	registry, err := ops.NewActivityRegistry()
	require.NoError(t, err)

	// Parse the same recipe to get the state machine structure
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

	inputs := map[string]interface{}{
		"message": "test",
	}

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		tracker := newInvocationTracker(rs.RecipeMetadata)
		stateTracker := tracker.child(segmentForMetadata(rs.RecipeMetadata.NodeMetadata, "recipe-state"))
		return executeStateMachine(ctx, registry, stateTracker, rs.StateData.States, inputs)
	})

	require.True(t, env.IsWorkflowCompleted())

	err = env.GetWorkflowError()
	if err != nil {
		t.Logf("State machine execution error: %v", err)
		t.FailNow()
	}

	var result map[string]interface{}
	err = env.GetWorkflowResult(&result)
	require.NoError(t, err)

	t.Logf("State machine internal result: %+v", result)

	// This should show us what executeStateMachine returns
	// which should be the terminal state's outputs
}

func TestProcessNodeOutputsFunction(t *testing.T) {
	// Test the processNodeOutputs function directly

	// Simulate state machine outputs (what executeStateMachine returns)
	stateMachineOutputs := map[string]interface{}{
		"simple_state": map[string]interface{}{
			"message": "Hello test",
			"status":  "success",
		},
	}

	// Define the output templates (from the recipe)
	outputTemplates := map[string]interface{}{
		"result": "{{ inputs.message }}",
	}

	inputs := map[string]interface{}{
		"message": "test",
	}

	result, err := processNodeOutputs(stateMachineOutputs, outputTemplates, inputs, nil)
	require.NoError(t, err)

	t.Logf("processNodeOutputs result: %+v", result)

	// This should show "result": "test" since the template is {{ inputs.message }}
	assert.Equal(t, "test", result["result"])
}

func TestTerminalStateOutputs(t *testing.T) {
	// Test that a terminal state with outputs defined returns those outputs

	// Parse a recipe with terminal state outputs
	recipeYAML := `
id: terminal_outputs_test
state:
  initial: terminal
  states:
    terminal:
      op: echo_activity
      inputs:
        message: 'test message'
      outputs:
        custom_result: '{{ inputs.message }}'
        processed: 'true'
outputs:
  recipe_output: '{{ states.terminal.outputs.custom_result }}'
`

	r, err := recipe.LoadRecipeFromString([]byte(recipeYAML))
	require.NoError(t, err)

	rs := r.RecipeImpl.(*recipe.RecipeState)

	// Check what outputs are defined on the terminal state
	terminalState := rs.StateData.States.States["terminal"]
	t.Logf("Terminal state: %+v", terminalState)

	// Check if the Node has outputs
	if nodeOp, ok := terminalState.Node.NodeImpl.(*recipe.NodeOp); ok {
		t.Logf("Terminal state NodeOp: %+v", nodeOp)
		// NodeMetadata doesn't have Outputs field, check OpData
		t.Logf("Terminal state OpData: %+v", nodeOp.OpData)
	}

	// Check if outputs are in another structure
	if nodeState, ok := terminalState.Node.NodeImpl.(*recipe.NodeState); ok {
		t.Logf("Terminal state NodeState outputs: %+v", nodeState.Outputs)
	}
}
