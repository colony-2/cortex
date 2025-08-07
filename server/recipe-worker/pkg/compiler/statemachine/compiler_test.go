package statemachine

import (
	"testing"
	"time"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
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

// Test sequential composition
func TestSequentialComposition(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register mock activities
	env.OnActivity("step1_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "step1_output"}, nil)
	env.OnActivity("step2_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "step2_output"}, nil)
	env.OnActivity("step3_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "step3_output"}, nil)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		// Create activity executor that uses workflow context
		executor := &workflowActivityExecutor{}
		
		// Create compiler
		compiler, err := NewStateMachineCompiler(executor)
		assert.NoError(t, err)
		
		config := yamlpkg.StateMachineConfig{
			InitialState: "sequential_state",
			States: map[string]yamlpkg.StateDefinition{
				"sequential_state": {
					Sequential: []yamlpkg.CompositionStep{
						{
							ID:   "step1",
							Uses: "step1_activity",
							Inputs: map[string]interface{}{
								"data": "input1",
							},
						},
						{
							ID:   "step2",
							Uses: "step2_activity",
							Inputs: map[string]interface{}{
								"data": "{{ .Steps.step1.result }}",
							},
						},
						{
							ID:   "step3",
							Uses: "step3_activity",
							Inputs: map[string]interface{}{
								"data": "{{ .Steps.step2.result }}",
							},
						},
					},
					Terminal: true,
				},
			},
		}

		inputs := map[string]interface{}{
			"initial_data": "test_data",
		}

		outputs, err := compiler.Execute(ctx, config, inputs)
		assert.NoError(t, err)
		assert.NotNil(t, outputs)
		
		// Verify outputs contain results from all steps
		assert.Contains(t, outputs, "step1")
		assert.Contains(t, outputs, "step2")
		assert.Contains(t, outputs, "step3")
		
		return nil
	})

	env.AssertExpectations(t)
}

// Test parallel composition
func TestParallelComposition(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create mock executor
	mockExecutor := &MockActivityExecutor{}
	
	// Setup expected calls - all should be called
	mockExecutor.On("ExecuteActivity", mock.Anything, "parallel1_activity", mock.Anything).
		Return(map[string]interface{}{"result": "parallel1_output"}, nil)
	mockExecutor.On("ExecuteActivity", mock.Anything, "parallel2_activity", mock.Anything).
		Return(map[string]interface{}{"result": "parallel2_output"}, nil)
	mockExecutor.On("ExecuteActivity", mock.Anything, "parallel3_activity", mock.Anything).
		Return(map[string]interface{}{"result": "parallel3_output"}, nil)

	// Create compiler
	compiler, err := NewStateMachineCompiler(mockExecutor)
	assert.NoError(t, err)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		config := yamlpkg.StateMachineConfig{
			InitialState: "parallel_state",
			States: map[string]yamlpkg.StateDefinition{
				"parallel_state": {
					Parallel: []yamlpkg.CompositionStep{
						{
							ID:   "parallel1",
							Uses: "parallel1_activity",
							Inputs: map[string]interface{}{
								"data": "input1",
							},
						},
						{
							ID:   "parallel2",
							Uses: "parallel2_activity",
							Inputs: map[string]interface{}{
								"data": "input2",
							},
						},
						{
							ID:   "parallel3",
							Uses: "parallel3_activity",
							Inputs: map[string]interface{}{
								"data": "input3",
							},
						},
					},
					Terminal: true,
				},
			},
		}

		inputs := map[string]interface{}{
			"initial_data": "test_data",
		}

		outputs, err := compiler.Execute(ctx, config, inputs)
		assert.NoError(t, err)
		assert.NotNil(t, outputs)
		
		// Verify all parallel outputs are present
		assert.Contains(t, outputs, "parallel1")
		assert.Contains(t, outputs, "parallel2")
		assert.Contains(t, outputs, "parallel3")
		
		return nil
	})

	env.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

// Test conditional composition
func TestConditionalComposition(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create mock executor
	mockExecutor := &MockActivityExecutor{}
	
	// Only the high priority activity should be called
	mockExecutor.On("ExecuteActivity", mock.Anything, "high_priority_processor", mock.Anything).
		Return(map[string]interface{}{"result": "high_priority_output"}, nil)

	// Create compiler
	compiler, err := NewStateMachineCompiler(mockExecutor)
	assert.NoError(t, err)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		config := yamlpkg.StateMachineConfig{
			InitialState: "conditional_state",
			States: map[string]yamlpkg.StateDefinition{
				"conditional_state": {
					Conditional: []yamlpkg.ConditionalBranch{
						{
							When: ".Inputs.priority == 'high'",
							Uses: "high_priority_processor",
							Inputs: map[string]interface{}{
								"data": "{{ .Inputs.data }}",
							},
						},
						{
							When: ".Inputs.priority == 'medium'",
							Uses: "medium_priority_processor",
							Inputs: map[string]interface{}{
								"data": "{{ .Inputs.data }}",
							},
						},
						{
							Default: true,
							Uses:    "default_processor",
							Inputs: map[string]interface{}{
								"data": "{{ .Inputs.data }}",
							},
						},
					},
					Terminal: true,
				},
			},
		}

		inputs := map[string]interface{}{
			"priority": "high",
			"data":     "test_data",
		}

		outputs, err := compiler.Execute(ctx, config, inputs)
		assert.NoError(t, err)
		assert.NotNil(t, outputs)
		assert.Equal(t, "high_priority_output", outputs["result"])
		
		return nil
	})

	env.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

// Test nested compositions
func TestNestedComposition(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create mock executor
	mockExecutor := &MockActivityExecutor{}
	
	// Setup expected calls for nested composition
	mockExecutor.On("ExecuteActivity", mock.Anything, "prepare_activity", mock.Anything).
		Return(map[string]interface{}{"prepared": true}, nil)
	mockExecutor.On("ExecuteActivity", mock.Anything, "process1_activity", mock.Anything).
		Return(map[string]interface{}{"result": "process1_output"}, nil)
	mockExecutor.On("ExecuteActivity", mock.Anything, "process2_activity", mock.Anything).
		Return(map[string]interface{}{"result": "process2_output"}, nil)
	mockExecutor.On("ExecuteActivity", mock.Anything, "finalize_activity", mock.Anything).
		Return(map[string]interface{}{"finalized": true}, nil)

	// Create compiler
	compiler, err := NewStateMachineCompiler(mockExecutor)
	assert.NoError(t, err)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		config := yamlpkg.StateMachineConfig{
			InitialState: "nested_state",
			States: map[string]yamlpkg.StateDefinition{
				"nested_state": {
					Sequential: []yamlpkg.CompositionStep{
						{
							ID:   "prepare",
							Uses: "prepare_activity",
							Inputs: map[string]interface{}{
								"data": "{{ .Inputs.data }}",
							},
						},
						{
							ID: "process",
							Parallel: []yamlpkg.CompositionStep{
								{
									ID:   "process1",
									Uses: "process1_activity",
									Inputs: map[string]interface{}{
										"data": "{{ .Steps.prepare.prepared }}",
									},
								},
								{
									ID:   "process2",
									Uses: "process2_activity",
									Inputs: map[string]interface{}{
										"data": "{{ .Steps.prepare.prepared }}",
									},
								},
							},
						},
						{
							ID:   "finalize",
							Uses: "finalize_activity",
							Inputs: map[string]interface{}{
								"result1": "{{ .Steps.process.process1.result }}",
								"result2": "{{ .Steps.process.process2.result }}",
							},
						},
					},
					Terminal: true,
				},
			},
		}

		inputs := map[string]interface{}{
			"data": "test_data",
		}

		outputs, err := compiler.Execute(ctx, config, inputs)
		assert.NoError(t, err)
		assert.NotNil(t, outputs)
		
		// Verify nested structure results
		assert.Contains(t, outputs, "prepare")
		assert.Contains(t, outputs, "process")
		assert.Contains(t, outputs, "finalize")
		
		return nil
	})

	env.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}

// Test state transitions
func TestStateTransitions(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create mock executor
	mockExecutor := &MockActivityExecutor{}
	
	// Setup expected calls
	mockExecutor.On("ExecuteActivity", mock.Anything, "validate_activity", mock.Anything).
		Return(map[string]interface{}{"valid": true}, nil)
	mockExecutor.On("ExecuteActivity", mock.Anything, "process_activity", mock.Anything).
		Return(map[string]interface{}{"processed": true}, nil)

	// Create compiler
	compiler, err := NewStateMachineCompiler(mockExecutor)
	assert.NoError(t, err)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		config := yamlpkg.StateMachineConfig{
			InitialState: "validation",
			States: map[string]yamlpkg.StateDefinition{
				"validation": {
					Uses: "validate_activity",
					Inputs: map[string]interface{}{
						"data": "{{ .Inputs.data }}",
					},
					Transitions: []yamlpkg.TransitionSpec{
						{
							To:   "processing",
							When: ".Outputs.valid == true",
						},
						{
							To:   "error",
							When: ".Outputs.valid == false",
						},
					},
				},
				"processing": {
					Uses: "process_activity",
					Inputs: map[string]interface{}{
						"data": "{{ .Inputs.data }}",
					},
					Transitions: []yamlpkg.TransitionSpec{
						{
							To: "complete",
						},
					},
				},
				"complete": {
					Terminal: true,
					Outputs: map[string]interface{}{
						"message": "Processing complete",
					},
				},
				"error": {
					Terminal: true,
					Error:    "Validation failed",
				},
			},
		}

		inputs := map[string]interface{}{
			"data": "test_data",
		}

		outputs, err := compiler.Execute(ctx, config, inputs)
		assert.NoError(t, err)
		assert.NotNil(t, outputs)
		assert.Equal(t, "Processing complete", outputs["message"])
		
		return nil
	})

	env.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
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

// Test retry policy
func TestRetryPolicy(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create mock executor
	mockExecutor := &MockActivityExecutor{}
	
	// First call fails, second succeeds
	mockExecutor.On("ExecuteActivity", mock.Anything, "flaky_activity", mock.Anything).
		Return(nil, assert.AnError).Once()
	mockExecutor.On("ExecuteActivity", mock.Anything, "flaky_activity", mock.Anything).
		Return(map[string]interface{}{"result": "success"}, nil).Once()

	// Create compiler
	compiler, err := NewStateMachineCompiler(mockExecutor)
	assert.NoError(t, err)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		config := yamlpkg.StateMachineConfig{
			InitialState: "retry_state",
			States: map[string]yamlpkg.StateDefinition{
				"retry_state": {
					Sequential: []yamlpkg.CompositionStep{
						{
							ID:   "flaky_step",
							Uses: "flaky_activity",
							Retry: &yamlpkg.StepRetryPolicy{
								MaxAttempts:        2,
								BackoffCoefficient: 2.0,
								InitialInterval:    100 * time.Millisecond,
							},
						},
					},
					Terminal: true,
				},
			},
		}

		inputs := map[string]interface{}{
			"data": "test_data",
		}

		outputs, err := compiler.Execute(ctx, config, inputs)
		assert.NoError(t, err)
		assert.NotNil(t, outputs)
		assert.Contains(t, outputs, "flaky_step")
		
		return nil
	})

	env.AssertExpectations(t)
	mockExecutor.AssertExpectations(t)
}