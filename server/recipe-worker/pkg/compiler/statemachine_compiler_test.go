package compiler

import (
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test basic state machine structure validation
func TestBasicStateMachineStructure(t *testing.T) {
	stateMap := &yamlpkg.StateMap{
		Initial: "simple_state",
		States: map[string]yamlpkg.State{
			"simple_state": {
				Node: yamlpkg.Node{
					Op: "simple_activity",
					Inputs: map[string]interface{}{
						"data": "test",
					},
				},
				Transitions: []yamlpkg.Transition{}, // Terminal state
			},
		},
	}

	// Verify basic structure
	assert.Equal(t, "simple_state", stateMap.Initial)
	assert.Contains(t, stateMap.States, "simple_state")
	assert.Equal(t, "simple_activity", stateMap.States["simple_state"].Op)
}

// Test sequential composition structure
func TestSequentialCompositionStructure(t *testing.T) {
	stateMap := &yamlpkg.StateMap{
		Initial: "sequential_state",
		States: map[string]yamlpkg.State{
			"sequential_state": {
				Node: yamlpkg.Node{
					Sequence: []yamlpkg.Node{
						{
							ID: "step1",
							Op: "step1_activity",
							Inputs: map[string]interface{}{
								"data": "input1",
							},
						},
						{
							ID: "step2",
							Op: "step2_activity",
							Inputs: map[string]interface{}{
								"data": "{{ .Steps.step1.result }}",
							},
						},
						{
							ID: "step3",
							Op: "step3_activity",
							Inputs: map[string]interface{}{
								"data": "{{ .Steps.step2.result }}",
							},
						},
					},
				},
				Transitions: []yamlpkg.Transition{}, // Terminal state has no transitions
			},
		},
	}

	// Test that the state map is properly structured
	state := stateMap.States["sequential_state"]
	assert.Len(t, state.Sequence, 3)
	assert.Equal(t, "step1", state.Sequence[0].ID)
	assert.Equal(t, "step2", state.Sequence[1].ID)
	assert.Equal(t, "step3", state.Sequence[2].ID)
}

// Test parallel composition structure
func TestParallelCompositionStructure(t *testing.T) {
	stateMap := &yamlpkg.StateMap{
		Initial: "parallel_state",
		States: map[string]yamlpkg.State{
			"parallel_state": {
				Node: yamlpkg.Node{
					Parallel: []yamlpkg.Node{
						{
							ID: "parallel1",
							Op: "parallel1_activity",
							Inputs: map[string]interface{}{
								"data": "input1",
							},
						},
						{
							ID: "parallel2",
							Op: "parallel2_activity",
							Inputs: map[string]interface{}{
								"data": "input2",
							},
						},
						{
							ID: "parallel3",
							Op: "parallel3_activity",
							Inputs: map[string]interface{}{
								"data": "input3",
							},
						},
					},
				},
				Transitions: []yamlpkg.Transition{}, // Terminal state
			},
		},
	}

	// Test that the state map is properly structured
	state := stateMap.States["parallel_state"]
	assert.Len(t, state.Parallel, 3)
	assert.Equal(t, "parallel1", state.Parallel[0].ID)
	assert.Equal(t, "parallel2", state.Parallel[1].ID)
	assert.Equal(t, "parallel3", state.Parallel[2].ID)
}

// Test state transitions structure
func TestStateTransitionsStructure(t *testing.T) {
	// Create CEL expressions for transitions
	// Using proper CEL variable references
	successExpr, err := cel.NewCELExpr("Outputs.result == 'success'")
	require.NoError(t, err)
	
	errorExpr, err := cel.NewCELExpr("Outputs.result == 'error'")
	require.NoError(t, err)
	
	stateMap := &yamlpkg.StateMap{
		Initial: "state_a",
		States: map[string]yamlpkg.State{
			"state_a": {
				Node: yamlpkg.Node{
					Op: "activity_a",
					Inputs: map[string]interface{}{
						"data": "test",
					},
				},
				Transitions: []yamlpkg.Transition{
					{
						To:   "state_b",
						When: *successExpr,
					},
					{
						To:   "error_state",
						When: *errorExpr,
					},
				},
			},
			"state_b": {
				Node: yamlpkg.Node{
					Op: "activity_b",
				},
				Transitions: []yamlpkg.Transition{},
			},
			"error_state": {
				Error: "An error occurred",
			},
		},
	}

	// Test transitions
	stateA := stateMap.States["state_a"]
	assert.Len(t, stateA.Transitions, 2)
	assert.Equal(t, "state_b", stateA.Transitions[0].To)
	assert.Equal(t, "error_state", stateA.Transitions[1].To)
	
	// Test error state
	errorState := stateMap.States["error_state"]
	assert.Equal(t, "An error occurred", errorState.Error)
}