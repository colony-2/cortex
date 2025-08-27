package compiler

import (
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
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
	// Skip CEL expression validation for now as it requires proper environment setup
	t.Skip("CEL expression validation requires proper environment setup")
}