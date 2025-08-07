package statemachine

import (
	"context"
	"testing"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// workflowActivityExecutor wraps workflow.ExecuteActivity for use with state machine compiler
type workflowActivityExecutor struct{}

// ExecuteActivity implements the ActivityExecutor interface
func (e *workflowActivityExecutor) ExecuteActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	var outputs map[string]interface{}
	err := workflow.ExecuteActivity(ctx, activityName, inputs).Get(ctx, &outputs)
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

// Test activities for integration tests
type TestActivities struct{}

func (a *TestActivities) DataFetcher(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"data": "fetched_data",
		"size": 1500000, // Large data for conditional testing
	}, nil
}

func (a *TestActivities) DataValidator(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"valid": true,
	}, nil
}

func (a *TestActivities) DataCleaner(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"cleaned_data": "clean_" + inputs["data"].(string),
	}, nil
}

func (a *TestActivities) BatchProcessor(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"processed": true,
		"batch_id":  inputs["batch_id"],
	}, nil
}

func (a *TestActivities) FastProcessor(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"result":     "fast_processed",
		"time_taken": "100ms",
	}, nil
}

func (a *TestActivities) StandardProcessor(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"result":     "standard_processed",
		"time_taken": "500ms",
	}, nil
}

func (a *TestActivities) MLPipeline(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"confidence": 0.95,
		"prediction": "positive",
	}, nil
}

func (a *TestActivities) HighConfidenceHandler(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"action": "auto_approve",
		"result": inputs["result"],
	}, nil
}

func (a *TestActivities) ManualReview(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"action": "manual_review_required",
		"result": inputs["result"],
	}, nil
}

// Complex workflow that uses state machine compiler
func ComplexStateMachineWorkflow(ctx workflow.Context, config yamlpkg.StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Create activity executor that uses workflow context
	executor := &workflowActivityExecutor{}
	
	// Create state machine compiler
	compiler, err := NewStateMachineCompiler(executor)
	if err != nil {
		return nil, err
	}
	
	// Execute the state machine
	return compiler.Execute(ctx, config, inputs)
}

// Test complex document processing pipeline
func TestComplexDocumentProcessingIntegration(t *testing.T) {
	// Create test suite
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()
	
	// Register workflow
	env.RegisterWorkflow(ComplexStateMachineWorkflow)
	
	// Register activities
	activities := &TestActivities{}
	env.RegisterActivity(activities.DataFetcher)
	env.RegisterActivity(activities.DataValidator)
	env.RegisterActivity(activities.DataCleaner)
	env.RegisterActivity(activities.BatchProcessor)
	env.RegisterActivity(activities.FastProcessor)
	env.RegisterActivity(activities.StandardProcessor)
	env.RegisterActivity(activities.MLPipeline)
	env.RegisterActivity(activities.HighConfidenceHandler)
	env.RegisterActivity(activities.ManualReview)
	
	// Define complex state machine configuration
	config := yamlpkg.StateMachineConfig{
		InitialState: "intake",
		States: map[string]yamlpkg.StateDefinition{
			"intake": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "fetch",
						Uses: "DataFetcher",
						Inputs: map[string]interface{}{
							"source": "{{ .Inputs.source }}",
						},
					},
					{
						ID: "validate_and_clean",
						Parallel: []yamlpkg.CompositionStep{
							{
								ID:   "validate",
								Uses: "DataValidator",
								Inputs: map[string]interface{}{
									"data": "{{ .Steps.fetch.data }}",
								},
							},
							{
								ID:   "clean",
								Uses: "DataCleaner",
								Inputs: map[string]interface{}{
									"data": "{{ .Steps.fetch.data }}",
								},
							},
						},
					},
				},
				Transitions: []yamlpkg.TransitionSpec{
					{
						To:   "processing",
						When: ".Outputs.validate_and_clean.validate.valid == true",
					},
					{
						To:   "error",
						When: ".Outputs.validate_and_clean.validate.valid == false",
					},
				},
			},
			"processing": {
				Conditional: []yamlpkg.ConditionalBranch{
					{
						When: ".States.intake.fetch.size > 1000000",
						Parallel: []yamlpkg.CompositionStep{
							{
								ID:   "batch1",
								Uses: "BatchProcessor",
								Inputs: map[string]interface{}{
									"batch_id": "batch_1",
									"data":     "{{ .States.intake.validate_and_clean.clean.cleaned_data }}",
								},
							},
							{
								ID:   "batch2",
								Uses: "BatchProcessor",
								Inputs: map[string]interface{}{
									"batch_id": "batch_2",
									"data":     "{{ .States.intake.validate_and_clean.clean.cleaned_data }}",
								},
							},
						},
					},
					{
						When: ".Inputs.priority == 'high'",
						Uses: "FastProcessor",
						Inputs: map[string]interface{}{
							"data": "{{ .States.intake.validate_and_clean.clean.cleaned_data }}",
						},
					},
					{
						Default: true,
						Uses:    "StandardProcessor",
						Inputs: map[string]interface{}{
							"data": "{{ .States.intake.validate_and_clean.clean.cleaned_data }}",
						},
					},
				},
				Transitions: []yamlpkg.TransitionSpec{
					{
						To: "ml_analysis",
					},
				},
			},
			"ml_analysis": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "ml_pipeline",
						Uses: "MLPipeline",
						Inputs: map[string]interface{}{
							"data": "{{ .States.processing }}",
						},
					},
					{
						ID: "confidence_check",
						Conditional: []yamlpkg.ConditionalBranch{
							{
								When: ".Steps.ml_pipeline.confidence > 0.9",
								Uses: "HighConfidenceHandler",
								Inputs: map[string]interface{}{
									"result": "{{ .Steps.ml_pipeline }}",
								},
							},
							{
								Default: true,
								Uses:    "ManualReview",
								Inputs: map[string]interface{}{
									"result": "{{ .Steps.ml_pipeline }}",
								},
							},
						},
					},
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
					"status":  "completed",
					"message": "Document processing completed successfully",
				},
			},
			"error": {
				Terminal: true,
				Error:    "Document validation failed",
			},
		},
	}
	
	// Prepare inputs
	inputs := map[string]interface{}{
		"source":   "test_document.pdf",
		"priority": "normal",
	}
	
	// Execute workflow
	env.ExecuteWorkflow(ComplexStateMachineWorkflow, config, inputs)
	
	// Verify workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	// Get workflow result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify expected outputs
	assert.Equal(t, "completed", result["status"])
	assert.Equal(t, "Document processing completed successfully", result["message"])
}

// Test with Temporal dev server
func TestWithTemporalDevServer(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// Create client
	c, err := client.Dial(client.Options{
		HostPort: client.DefaultHostPort,
	})
	if err != nil {
		t.Skip("Temporal dev server not available")
	}
	defer c.Close()
	
	// Create worker
	w := worker.New(c, "state-machine-test", worker.Options{})
	
	// Register workflow
	w.RegisterWorkflow(ComplexStateMachineWorkflow)
	
	// Register activities
	activities := &TestActivities{}
	w.RegisterActivity(activities.DataFetcher)
	w.RegisterActivity(activities.DataValidator)
	w.RegisterActivity(activities.DataCleaner)
	w.RegisterActivity(activities.BatchProcessor)
	w.RegisterActivity(activities.FastProcessor)
	w.RegisterActivity(activities.StandardProcessor)
	w.RegisterActivity(activities.MLPipeline)
	w.RegisterActivity(activities.HighConfidenceHandler)
	w.RegisterActivity(activities.ManualReview)
	
	// Start worker
	err = w.Start()
	require.NoError(t, err)
	defer w.Stop()
	
	// Create simple state machine config
	config := yamlpkg.StateMachineConfig{
		InitialState: "start",
		States: map[string]yamlpkg.StateDefinition{
			"start": {
				Sequential: []yamlpkg.CompositionStep{
					{
						ID:   "fetch",
						Uses: "DataFetcher",
						Inputs: map[string]interface{}{
							"source": "test.txt",
						},
					},
					{
						ID:   "process",
						Uses: "StandardProcessor",
						Inputs: map[string]interface{}{
							"data": "{{ .Steps.fetch.data }}",
						},
					},
				},
				Terminal: true,
			},
		},
	}
	
	// Execute workflow
	we, err := c.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
		ID:        "test-state-machine-workflow",
		TaskQueue: "state-machine-test",
	}, ComplexStateMachineWorkflow, config, map[string]interface{}{
		"test": "data",
	})
	require.NoError(t, err)
	
	// Wait for completion
	var result map[string]interface{}
	err = we.Get(context.Background(), &result)
	require.NoError(t, err)
	
	// Verify result
	assert.NotNil(t, result)
}

// Test error handling and recovery
func TestErrorHandlingIntegration(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()
	
	env.RegisterWorkflow(ComplexStateMachineWorkflow)
	
	// Register activity that will fail
	env.OnActivity("FailingActivity", mock.Anything, mock.Anything).Return(nil, assert.AnError)
	
	config := yamlpkg.StateMachineConfig{
		InitialState: "error_test",
		States: map[string]yamlpkg.StateDefinition{
			"error_test": {
				Uses: "FailingActivity",
				Retry: &yamlpkg.StateRetryPolicy{
					MaxAttempts:        3,
					BackoffCoefficient: 2.0,
					InitialInterval:    "1s",
				},
				Transitions: []yamlpkg.TransitionSpec{
					{
						To: "fallback",
					},
				},
			},
			"fallback": {
				Terminal: true,
				Outputs: map[string]interface{}{
					"status": "handled_error",
				},
			},
		},
		Timeout: "10s",
	}
	
	env.ExecuteWorkflow(ComplexStateMachineWorkflow, config, map[string]interface{}{})
	
	// Should fail after retries
	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}