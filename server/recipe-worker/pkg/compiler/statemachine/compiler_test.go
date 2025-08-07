package statemachine

import (
	"testing"
	"time"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

// MockActivityExecutor for testing
type MockActivityExecutor struct {
	mock.Mock
}

func (m *MockActivityExecutor) ExecuteActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	args := m.Called(ctx, activityName, inputs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
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
	result, err := compiler.evaluateCEL(".Inputs.value > 50", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test boolean check
	result, err = compiler.evaluateCEL(".Inputs.flag == true", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test state output access
	result, err = compiler.evaluateCEL(".States.previous_state.result == 'success'", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)

	// Test step output access
	result, err = compiler.evaluateCEL(".Steps.step1.count > 3", nil, stateCtx)
	assert.NoError(t, err)
	assert.True(t, result)
}

// Test retry policy calculation
func TestRetryBackoffCalculation(t *testing.T) {
	compiler, err := NewStateMachineCompiler(nil)
	assert.NoError(t, err)

	policy := &yamlpkg.StepRetryPolicy{
		InitialInterval:    100 * time.Millisecond,
		BackoffCoefficient: 2.0,
	}

	// Test backoff calculation
	backoff1 := compiler.calculateBackoff(policy, 1)
	assert.Equal(t, 100*time.Millisecond, backoff1)

	backoff2 := compiler.calculateBackoff(policy, 2)
	assert.Equal(t, 200*time.Millisecond, backoff2)

	backoff3 := compiler.calculateBackoff(policy, 3)
	assert.Equal(t, 400*time.Millisecond, backoff3)
}