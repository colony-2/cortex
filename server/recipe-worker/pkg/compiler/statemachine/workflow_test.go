package statemachine

import (
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// TestNestedCompositionWorkflow tests deeply nested compositions
func TestNestedCompositionWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(testNestedWorkflow)
	
	// Register mock activities
	env.OnActivity("prepare_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"prepared": true}, nil).Once()
	env.OnActivity("process1_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "process1_output"}, nil).Once()
	env.OnActivity("process2_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "process2_output"}, nil).Once()
	env.OnActivity("finalize_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"finalized": true}, nil).Once()

	env.ExecuteWorkflow(testNestedWorkflow)

	assert.True(t, env.IsWorkflowCompleted())
	assert.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	assert.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify nested structure results
	assert.Contains(t, result, "prepare")
	assert.Contains(t, result, "process")
	assert.Contains(t, result, "finalize")
}

func testNestedWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
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

	return compiler.Execute(ctx, config, map[string]interface{}{"data": "test_data"})
}

// TestStateTransitionsWorkflow tests state machine transitions
func TestStateTransitionsWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(testTransitionsWorkflow)
	
	// Register mock activities
	env.OnActivity("validate_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"valid": true}, nil).Once()
	env.OnActivity("process_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"processed": true}, nil).Once()

	env.ExecuteWorkflow(testTransitionsWorkflow)

	assert.True(t, env.IsWorkflowCompleted())
	assert.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	assert.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, "Processing complete", result["message"])
}

func testTransitionsWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
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

	return compiler.Execute(ctx, config, map[string]interface{}{"data": "test_data"})
}

// TestRetryPolicyWorkflow tests retry policy in state machine
func TestRetryPolicyWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(testRetryWorkflow)
	
	attempts := 0
	// First call fails, second succeeds
	env.OnActivity("flaky_activity", mock.Anything, mock.Anything).Return(
		func(inputs map[string]interface{}) (map[string]interface{}, error) {
			attempts++
			if attempts < 2 {
				return nil, assert.AnError
			}
			return map[string]interface{}{"result": "success"}, nil
		})

	env.ExecuteWorkflow(testRetryWorkflow)

	assert.True(t, env.IsWorkflowCompleted())
	assert.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	assert.NoError(t, env.GetWorkflowResult(&result))
	assert.Contains(t, result, "flaky_step")
}

func testRetryWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	executor := &workflowActivityExecutor{}
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
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
							InitialInterval:    100,
						},
					},
				},
				Terminal: true,
			},
		},
	}

	return compiler.Execute(ctx, config, map[string]interface{}{"data": "test_data"})
}