//go:build integration
// +build integration

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

// TestRecipeToRecipeInvocation_Integration tests parent-to-child recipe invocation
func TestRecipeToRecipeInvocation_Integration(t *testing.T) {
	// Create test suite
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register activities and workflows
	env.RegisterActivity(ExecuteRecipeActivity)
	env.RegisterWorkflow(ParentRecipeWorkflow)
	env.RegisterWorkflow(ChildRecipeWorkflow)

	// Set up input for parent workflow
	parentInput := map[string]interface{}{
		"data":      "test-data",
		"operation": "process",
	}

	// Execute parent workflow
	env.ExecuteWorkflow(ParentRecipeWorkflow, parentInput)

	// Verify workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get workflow result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	// Verify result structure
	assert.NotNil(t, result)
	assert.Contains(t, result, "parent_result")
	assert.Contains(t, result, "child_result")
	assert.Equal(t, "processed", result["status"])
}

// ParentRecipeWorkflow is a test workflow that invokes a child recipe
func ParentRecipeWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Set activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Prepare child recipe invocation
	childRecipeInput := RecipeActivity{
		Recipe:  "test-child-recipe",
		Timeout: 2 * time.Minute,
		Inputs: map[string]interface{}{
			"parent_data": input["data"],
			"step":        1,
		},
		Context: &RecipeContext{
			Recipe: RecipeInfo{
				Name:        "parent-recipe",
				Version:     "1.0.0",
				ExecutionID: workflow.GetInfo(ctx).WorkflowExecution.ID,
			},
			Environment: EnvironmentInfo{
				Name:    "test",
				Region:  "local",
				Cluster: "test-cluster",
			},
			Execution: ExecutionInfo{
				Host:      "test-host",
				Namespace: workflow.GetInfo(ctx).Namespace,
				TaskQueue: workflow.GetInfo(ctx).TaskQueueName,
				StartedAt: workflow.Now(ctx),
				Timeout:   5 * time.Minute,
			},
			Auth: AuthInfo{
				Identity: "test-service",
			},
		},
	}

	// Execute child recipe
	childResult, err := ExecuteChildRecipeWorkflow(ctx, childRecipeInput)
	if err != nil {
		return nil, err
	}

	// Process results
	result := map[string]interface{}{
		"parent_result": "completed",
		"child_result":  childResult.Result,
		"status":        "processed",
		"execution_id":  childResult.ExecutionID,
		"metadata":      childResult.ExecutionMetadata,
	}

	return result, nil
}

// ChildRecipeWorkflow is a test child workflow
func ChildRecipeWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Extract context if provided
	var recipeContext *RecipeContext
	if ctxData, ok := input["context"]; ok {
		if rc, ok := ctxData.(*RecipeContext); ok {
			recipeContext = rc
		}
	}

	// Extract inputs
	inputs, _ := input["inputs"].(map[string]interface{})

	// Simulate some processing
	result := map[string]interface{}{
		"processed_data": inputs["parent_data"],
		"step_completed": inputs["step"],
		"child_status":   "success",
	}

	// Include parent context info if available
	if recipeContext != nil {
		result["parent_execution_id"] = recipeContext.Recipe.ParentExecutionID
		result["environment"] = recipeContext.Environment.Name
	}

	return result, nil
}

// TestRecipeContextPropagation tests context propagation between recipes
func TestRecipeContextPropagation(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register workflows
	env.RegisterWorkflow(ContextPropagationParentWorkflow)
	env.RegisterWorkflow(ContextPropagationChildWorkflow)

	// Execute parent workflow
	env.ExecuteWorkflow(ContextPropagationParentWorkflow)

	// Verify completion
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	// Verify context was properly propagated
	assert.Equal(t, "production", result["child_environment"])
	assert.Equal(t, "us-west-2", result["child_region"])
	assert.NotEmpty(t, result["child_parent_id"])
}

// ContextPropagationParentWorkflow tests context propagation
func ContextPropagationParentWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	parentContext := &RecipeContext{
		Recipe: RecipeInfo{
			Name:        "context-parent",
			Version:     "1.0.0",
			ExecutionID: "parent-exec-123",
		},
		Environment: EnvironmentInfo{
			Name:    "production",
			Region:  "us-west-2",
			Cluster: "main",
		},
		Execution: ExecutionInfo{
			Host:      "worker-1",
			Namespace: "recipes",
			TaskQueue: "recipe-queue",
			StartedAt: workflow.Now(ctx),
			Timeout:   10 * time.Minute,
		},
		Auth: AuthInfo{
			Identity: "parent-service",
			Token:    "parent-token",
		},
	}

	childInput := RecipeActivity{
		Recipe:  "context-child",
		Timeout: 5 * time.Minute,
		Inputs: map[string]interface{}{
			"test": "data",
		},
		Context: parentContext,
	}

	childResult, err := ExecuteChildRecipeWorkflow(ctx, childInput)
	if err != nil {
		return nil, err
	}

	// Extract child context info from result
	childEnv := ""
	childRegion := ""
	childParentID := ""
	
	if childCtx, ok := childResult.Result["context_info"].(map[string]interface{}); ok {
		childEnv, _ = childCtx["environment"].(string)
		childRegion, _ = childCtx["region"].(string)
		childParentID, _ = childCtx["parent_id"].(string)
	}

	return map[string]interface{}{
		"child_environment": childEnv,
		"child_region":      childRegion,
		"child_parent_id":   childParentID,
	}, nil
}

// ContextPropagationChildWorkflow receives and uses propagated context
func ContextPropagationChildWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Extract context
	var recipeContext *RecipeContext
	if ctxData, ok := input["context"]; ok {
		if rc, ok := ctxData.(*RecipeContext); ok {
			recipeContext = rc
		}
	}

	result := map[string]interface{}{
		"status": "completed",
	}

	// Include context info in result
	if recipeContext != nil {
		result["context_info"] = map[string]interface{}{
			"environment": recipeContext.Environment.Name,
			"region":      recipeContext.Environment.Region,
			"parent_id":   recipeContext.Recipe.ParentExecutionID,
		}
	}

	return result, nil
}

// TestRecipeRetryPolicy tests retry policy configuration
func TestRecipeRetryPolicy(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register workflows
	env.RegisterWorkflow(RetryPolicyParentWorkflow)
	env.RegisterWorkflow(RetryPolicyChildWorkflow)

	// Set up to simulate failures
	failCount := 0
	env.OnWorkflow(RetryPolicyChildWorkflow, mock.Anything, mock.Anything).Return(
		func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
			failCount++
			if failCount < 3 {
				return nil, temporal.NewApplicationError("simulated failure", "TestError")
			}
			return map[string]interface{}{"attempts": failCount}, nil
		},
	).Times(3)

	// Execute parent workflow
	env.ExecuteWorkflow(RetryPolicyParentWorkflow)

	// Verify completion after retries
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	// Verify retry happened
	assert.Equal(t, 3, result["total_attempts"])
}

// RetryPolicyParentWorkflow tests retry policy
func RetryPolicyParentWorkflow(ctx workflow.Context) (map[string]interface{}, error) {
	childInput := RecipeActivity{
		Recipe:  "retry-child",
		Timeout: 1 * time.Minute,
		RetryPolicy: &RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
		},
		Inputs: map[string]interface{}{
			"data": "retry-test",
		},
	}

	childResult, err := ExecuteChildRecipeWorkflow(ctx, childInput)
	if err != nil {
		return nil, err
	}

	attempts := 0
	if a, ok := childResult.Result["attempts"].(int); ok {
		attempts = a
	}

	return map[string]interface{}{
		"total_attempts": attempts,
		"status":         childResult.Status,
	}, nil
}

// RetryPolicyChildWorkflow is used for retry testing
func RetryPolicyChildWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// This workflow is mocked in the test
	return map[string]interface{}{"attempts": 1}, nil
}

// TestRecipeVersioning tests recipe versioning support
func TestRecipeVersioning(t *testing.T) {
	tests := []struct {
		name            string
		recipePath      string
		configVersion   string
		expectedVersion string
	}{
		{
			name:            "version in path",
			recipePath:      "data-processor@v2.0.0",
			configVersion:   "",
			expectedVersion: "v2.0.0",
		},
		{
			name:            "version in config",
			recipePath:      "data-processor",
			configVersion:   "v1.5.0",
			expectedVersion: "v1.5.0",
		},
		{
			name:            "version in both (path takes precedence)",
			recipePath:      "data-processor@v3.0.0",
			configVersion:   "v1.0.0",
			expectedVersion: "v3.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := RecipeConfig{
				Recipe:  tt.recipePath,
				Version: tt.configVersion,
				Timeout: "10m",
			}

			// Parse the recipe path to verify version handling
			recipeName, version := parseRecipePath(config.Recipe)
			if version == "" {
				version = config.Version
			}

			assert.Equal(t, tt.expectedVersion, version)
			assert.NotContains(t, recipeName, "@")
		})
	}
}

// BenchmarkParentChildExecution benchmarks parent-to-child recipe execution
func BenchmarkParentChildExecution(b *testing.B) {
	suite := &testsuite.WorkflowTestSuite{}
	
	for i := 0; i < b.N; i++ {
		env := suite.NewTestWorkflowEnvironment()
		env.RegisterWorkflow(ParentRecipeWorkflow)
		env.RegisterWorkflow(ChildRecipeWorkflow)
		
		input := map[string]interface{}{
			"data": "benchmark-data",
		}
		
		env.ExecuteWorkflow(ParentRecipeWorkflow, input)
		
		if !env.IsWorkflowCompleted() {
			b.Fatal("Workflow did not complete")
		}
	}
}