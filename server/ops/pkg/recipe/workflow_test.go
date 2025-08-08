package recipe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// TestRecipeWorkflowExecution tests recipe execution using Temporal's WorkflowTestSuite
func TestRecipeWorkflowExecution(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register the parent workflow
	env.RegisterWorkflow(ParentRecipeWorkflow)
	// Register the child workflow that will be invoked
	env.RegisterWorkflow(ChildRecipeWorkflow)
	// Register the recipe activity
	env.RegisterActivity(ExecuteRecipeActivity)

	// Set up client provider for activities
	mockProvider := new(MockClientProvider)
	mockClient := new(MockTemporalClient)
	mockProvider.On("GetClient", mock.Anything).Return(mockClient, nil)
	SetClientProvider(mockProvider)
	defer SetClientProvider(nil)

	// Input for the parent workflow
	parentInput := map[string]interface{}{
		"data": "test-data",
		"operation": "transform",
	}

	// Execute the parent workflow
	env.ExecuteWorkflow(ParentRecipeWorkflow, parentInput)

	// Verify workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get the workflow result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	// Verify the result
	assert.NotNil(t, result)
	assert.Equal(t, "completed", result["status"])
	assert.Contains(t, result, "child_results")
}

// ParentRecipeWorkflow is a test workflow that invokes child recipes
func ParentRecipeWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting parent recipe workflow", "input", input)

	// Set up activity options for recipe invocation
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
			InitialInterval: time.Second,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Prepare context for child recipe
	recipeContext := &RecipeContext{
		Recipe: RecipeInfo{
			Name:        "parent-workflow",
			Version:     "1.0.0",
			ExecutionID: workflow.GetInfo(ctx).WorkflowExecution.ID,
		},
		Environment: EnvironmentInfo{
			Name:    "test",
			Region:  "us-west-2",
			Cluster: "test-cluster",
		},
		Execution: ExecutionInfo{
			Host:      "test-host",
			Namespace: workflow.GetInfo(ctx).Namespace,
			TaskQueue: workflow.GetInfo(ctx).TaskQueueName,
			StartedAt: workflow.Now(ctx),
			Timeout:   10 * time.Minute,
		},
		Auth: AuthInfo{
			Identity: "test-service",
		},
	}

	// Execute child recipe using ExecuteChildWorkflow
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID: "child-recipe-" + workflow.GetInfo(ctx).WorkflowExecution.ID,
		TaskQueue:  workflow.GetInfo(ctx).TaskQueueName,
		WorkflowExecutionTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
			InitialInterval: 2 * time.Second,
		},
	})

	// Pass inputs and context to child
	childInput := map[string]interface{}{
		"parent_data": input["data"],
		"operation":   input["operation"],
		"context":     recipeContext,
	}

	var childResult map[string]interface{}
	childFuture := workflow.ExecuteChildWorkflow(childCtx, ChildRecipeWorkflow, childInput)
	
	if err := childFuture.Get(ctx, &childResult); err != nil {
		logger.Error("Child workflow failed", "error", err)
		return nil, err
	}

	logger.Info("Child workflow completed", "result", childResult)

	// Return combined results
	return map[string]interface{}{
		"status":        "completed",
		"parent_input":  input,
		"child_results": childResult,
	}, nil
}

// ChildRecipeWorkflow is a test child workflow
func ChildRecipeWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting child recipe workflow", "input", input)

	// Extract context if provided
	var parentExecutionID string
	var environment string
	
	if ctxData, ok := input["context"]; ok {
		// Try direct cast first
		if pc, ok := ctxData.(*RecipeContext); ok {
			parentExecutionID = pc.Recipe.ExecutionID
			environment = pc.Environment.Name
			logger.Info("Received parent context", 
				"parent_id", parentExecutionID,
				"environment", environment)
		} else if ctxMap, ok := ctxData.(map[string]interface{}); ok {
			// Handle serialized context
			if recipeData, ok := ctxMap["recipe"].(map[string]interface{}); ok {
				parentExecutionID, _ = recipeData["execution_id"].(string)
			}
			if envData, ok := ctxMap["environment"].(map[string]interface{}); ok {
				environment, _ = envData["name"].(string)
			}
			logger.Info("Received serialized parent context", 
				"parent_id", parentExecutionID,
				"environment", environment)
		}
	}

	// Simulate some processing
	workflow.Sleep(ctx, 100*time.Millisecond)

	// Process the data
	processedData := "processed: " + input["parent_data"].(string)

	result := map[string]interface{}{
		"processed_data": processedData,
		"operation":      input["operation"],
		"child_status":   "success",
	}

	// Include parent context info if available
	if parentExecutionID != "" {
		result["parent_execution_id"] = parentExecutionID
		result["environment"] = environment
	}

	logger.Info("Child workflow completed", "result", result)
	return result, nil
}

// TestRecipeActivityInWorkflow tests the recipe activity within a workflow
func TestRecipeActivityInWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows and activities
	env.RegisterWorkflow(WorkflowWithRecipeActivity)
	env.RegisterWorkflow(SimpleChildWorkflow)
	env.RegisterActivity(ExecuteRecipeActivity)

	// Mock the client provider
	env.OnActivity(ExecuteRecipeActivity, mock.Anything, mock.Anything).Return(
		&RecipeActivityOutput{
			ExecutionID: "test-execution-123",
			Result: map[string]interface{}{
				"processed": true,
				"data":      "transformed",
			},
			Status: "completed",
			ExecutionMetadata: ExecutionMetadata{
				StartTime:    time.Now(),
				EndTime:      time.Now().Add(time.Second),
				DurationMs:   1000,
				AttemptCount: 1,
			},
		}, nil)

	// Execute workflow
	env.ExecuteWorkflow(WorkflowWithRecipeActivity, map[string]interface{}{
		"input_data": "test",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, "success", result["status"])
	assert.Equal(t, "test-execution-123", result["child_execution_id"])
}

// WorkflowWithRecipeActivity demonstrates using the recipe activity in a workflow
func WorkflowWithRecipeActivity(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Set activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Prepare recipe activity input
	recipeInput := RecipeActivity{
		Recipe:  "simple-child",
		Timeout: 2 * time.Minute,
		Inputs:  input,
		Context: &RecipeContext{
			Recipe: RecipeInfo{
				Name:        "parent-with-activity",
				ExecutionID: workflow.GetInfo(ctx).WorkflowExecution.ID,
			},
			Execution: ExecutionInfo{
				TaskQueue: workflow.GetInfo(ctx).TaskQueueName,
				Namespace: workflow.GetInfo(ctx).Namespace,
			},
		},
	}

	// Execute the recipe activity
	var activityResult RecipeActivityOutput
	err := workflow.ExecuteActivity(ctx, ExecuteRecipeActivity, recipeInput).Get(ctx, &activityResult)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"status":             "success",
		"child_execution_id": activityResult.ExecutionID,
		"child_result":       activityResult.Result,
	}, nil
}

// SimpleChildWorkflow is a simple child workflow for testing
func SimpleChildWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"processed": true,
		"data":      "transformed",
	}, nil
}

// TestRecipeRetryBehavior tests retry behavior for recipe invocations
func TestRecipeRetryBehavior(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows
	env.RegisterWorkflow(WorkflowWithRetryableRecipe)
	env.RegisterWorkflow(FlakeyChildWorkflow)

	// Track attempt count
	attemptCount := 0
	env.RegisterDelayedCallback(func() {
		attemptCount++
	}, 0)

	// Execute workflow
	env.ExecuteWorkflow(WorkflowWithRetryableRecipe, map[string]interface{}{
		"retry_test": true,
	})

	require.True(t, env.IsWorkflowCompleted())
	
	var result map[string]interface{}
	err := env.GetWorkflowResult(&result)
	
	// Should eventually succeed after retries
	require.NoError(t, err)
	assert.Equal(t, "success_after_retry", result["status"])
}

// WorkflowWithRetryableRecipe demonstrates retry behavior
func WorkflowWithRetryableRecipe(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Configure retry policy
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID: "retryable-child",
		TaskQueue:  workflow.GetInfo(ctx).TaskQueueName,
		WorkflowExecutionTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    100 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			NonRetryableErrorTypes: []string{"NonRetryableError"},
		},
	})

	var result map[string]interface{}
	err := workflow.ExecuteChildWorkflow(childCtx, FlakeyChildWorkflow, input).Get(ctx, &result)
	
	if err != nil {
		return map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"status": "success_after_retry",
		"result": result,
	}, nil
}

// FlakeyChildWorkflow simulates a workflow that fails initially but succeeds on retry
var flakeyAttemptCount = 0

func FlakeyChildWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	flakeyAttemptCount++
	
	// Fail first 2 attempts, succeed on third
	if flakeyAttemptCount < 3 {
		return nil, temporal.NewApplicationError("temporary failure", "TemporaryError")
	}

	return map[string]interface{}{
		"success": true,
		"attempt": flakeyAttemptCount,
	}, nil
}

// TestContextPropagationInWorkflow tests context propagation through workflows
func TestContextPropagationInWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows
	env.RegisterWorkflow(ContextAwareParentWorkflow)
	env.RegisterWorkflow(ContextAwareChildWorkflow)

	// Execute workflow with context
	env.ExecuteWorkflow(ContextAwareParentWorkflow, map[string]interface{}{
		"test": "context_propagation",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	// Verify context was properly propagated
	assert.Equal(t, "production", result["child_environment"])
	assert.Equal(t, "us-west-2", result["child_region"])
	assert.NotEmpty(t, result["child_parent_id"])
}

// ContextAwareParentWorkflow tests context propagation
func ContextAwareParentWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Create rich context
	parentContext := &RecipeContext{
		Recipe: RecipeInfo{
			Name:        "context-parent",
			Version:     "2.0.0",
			ExecutionID: workflow.GetInfo(ctx).WorkflowExecution.ID,
		},
		Environment: EnvironmentInfo{
			Name:    "production",
			Region:  "us-west-2",
			Cluster: "prod-cluster",
		},
		Execution: ExecutionInfo{
			Host:      "prod-worker-1",
			Namespace: "production",
			TaskQueue: "prod-queue",
			StartedAt: workflow.Now(ctx),
			Timeout:   30 * time.Minute,
		},
		Auth: AuthInfo{
			Identity: "prod-service",
			Token:    "secret-token",
		},
	}

	// Execute child with context
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID: "context-child",
		TaskQueue:  workflow.GetInfo(ctx).TaskQueueName,
	})

	childInput := map[string]interface{}{
		"data":    input,
		"context": parentContext,
	}

	var childResult map[string]interface{}
	err := workflow.ExecuteChildWorkflow(childCtx, ContextAwareChildWorkflow, childInput).Get(ctx, &childResult)
	if err != nil {
		return nil, err
	}

	return childResult, nil
}

// ContextAwareChildWorkflow receives and validates context
func ContextAwareChildWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Extract and validate context
	if ctxData, ok := input["context"]; ok {
		// Try direct cast first
		if parentContext, ok := ctxData.(*RecipeContext); ok {
			// Build response with context information
			return map[string]interface{}{
				"child_environment": parentContext.Environment.Name,
				"child_region":      parentContext.Environment.Region,
				"child_parent_id":   parentContext.Recipe.ExecutionID,
				"child_auth":        parentContext.Auth.Identity,
			}, nil
		}
		
		// Try as map (when serialized through workflow)
		if ctxMap, ok := ctxData.(map[string]interface{}); ok {
			// Extract environment info
			var envName, region, parentID, authIdentity string
			
			if envData, ok := ctxMap["environment"].(map[string]interface{}); ok {
				envName, _ = envData["name"].(string)
				region, _ = envData["region"].(string)
			}
			
			if recipeData, ok := ctxMap["recipe"].(map[string]interface{}); ok {
				parentID, _ = recipeData["execution_id"].(string)
			}
			
			if authData, ok := ctxMap["auth"].(map[string]interface{}); ok {
				authIdentity, _ = authData["identity"].(string)
			}
			
			return map[string]interface{}{
				"child_environment": envName,
				"child_region":      region,
				"child_parent_id":   parentID,
				"child_auth":        authIdentity,
			}, nil
		}
	}

	return map[string]interface{}{
		"error": "no context received",
	}, nil
}