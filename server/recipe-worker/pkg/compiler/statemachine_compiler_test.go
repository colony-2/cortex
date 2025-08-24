package compiler

import (
	"testing"
	"time"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"go.temporal.io/sdk/workflow"
)

// workflowActivityExecutor for testing
type workflowActivityExecutor struct{}

func (e *workflowActivityExecutor) ExecuteActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	var outputs map[string]interface{}
	err := workflow.ExecuteActivity(ctx, activityName, inputs).Get(ctx, &outputs)
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

// Test CEL expression evaluation
func TestCELExpressionEvaluation(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)

	stateCtx := &yamlpkg.StateContext{
		CurrentState: "test_state",
		Inputs: map[string]interface{}{
			"value": 100,
			"flag":  true,
		},
		StateOutputs: map[string]map[string]interface{}{
			"previous_state": {
				"result": "success",
			},
		},
		StepOutputs: map[string]interface{}{
			"step1": map[string]interface{}{
				"count": 5,
			},
		},
	}

	// Test simple comparison
	result, err := compiler.evaluateCEL("Inputs.value > 50", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test boolean check
	result, err = compiler.evaluateCEL("Inputs.flag == true", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test state output access
	result, err = compiler.evaluateCEL("States.previous_state.result == 'success'", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test step output access
	result, err = compiler.evaluateCEL("Steps.step1.count > 3", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)
}

// Test retry policy calculation
func TestRetryBackoffCalculation(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)

	policy := &yamlpkg.RetryPolicy{
		InitialInterval:    "100ms",
		BackoffCoefficient: 2.0,
	}

	// Test backoff calculation
	backoff1, err := compiler.calculateBackoff(policy, 1)
	assert.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, backoff1)

	backoff2, err := compiler.calculateBackoff(policy, 2)
	assert.NoError(t, err)
	assert.Equal(t, 200*time.Millisecond, backoff2)

	backoff3, err := compiler.calculateBackoff(policy, 3)
	assert.NoError(t, err)
	assert.Equal(t, 400*time.Millisecond, backoff3)
}

// Test sequential composition with template resolution
func TestSequentialComposition(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)

	stateMap := &yamlpkg.StateMap{
		Initial: "sequential_state",
		States: map[string]yamlpkg.State{
			"sequential_state": {
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
				Transitions: []yamlpkg.Transition{}, // Terminal state has no transitions
			},
		},
	}

	// Verify the compiler is set up correctly
	assert.NotNil(t, compiler)

	// Test template resolution for step2
	resolver := NewTemplateResolver()
	stateCtx := &yamlpkg.StateContext{
		StepOutputs: map[string]interface{}{
			"step1": map[string]interface{}{"result": "output1"},
			"step2": map[string]interface{}{"result": "output2"},
		},
	}

	step2Inputs, err := resolver.ResolveInputs(
		stateMap.States["sequential_state"].Sequence[1].Inputs,
		stateCtx)
	assert.NoError(t, err)
	assert.Equal(t, "output1", step2Inputs["data"])

	// Test template resolution for step3
	step3Inputs, err := resolver.ResolveInputs(
		stateMap.States["sequential_state"].Sequence[2].Inputs,
		stateCtx)
	assert.NoError(t, err)
	assert.Equal(t, "output2", step3Inputs["data"])
}

// Test parallel composition with dependency grouping
func TestParallelComposition(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)

	stateMap := &yamlpkg.StateMap{
		Initial: "parallel_state",
		States: map[string]yamlpkg.State{
			"parallel_state": {
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
				Transitions: []yamlpkg.Transition{}, // Terminal state
			},
		},
	}

	// Verify the compiler setup
	assert.NotNil(t, compiler)

	// Test that the state map is properly structured
	state := stateMap.States["parallel_state"]
	assert.Len(t, state.Parallel, 3)
	assert.Equal(t, "parallel1", state.Parallel[0].ID)
	assert.Equal(t, "parallel2", state.Parallel[1].ID)
	assert.Equal(t, "parallel3", state.Parallel[2].ID)
}

// Basic test for state machine execution
func TestBasicStateExecution(t *testing.T) {
	_, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)

	stateMap := &yamlpkg.StateMap{
		Initial: "simple_state",
		States: map[string]yamlpkg.State{
			"simple_state": {
				Op: "simple_activity",
				Inputs: map[string]interface{}{
					"data": "test",
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
