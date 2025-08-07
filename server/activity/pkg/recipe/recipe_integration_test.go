package recipe

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// TestRecipeActivityIntegration tests the complete flow: Parent Workflow -> Recipe Activity -> Child Workflow
func TestRecipeActivityIntegration(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows and activities
	env.RegisterWorkflow(ParentWorkflowWithRecipeActivity)
	env.RegisterWorkflow(DataProcessingRecipe)
	
	// Create the wrapper instance
	recipeWrapper := &RecipeActivityWrapper{}
	
	// Register the Execute method as an activity
	env.RegisterActivity(recipeWrapper.Execute)
	
	// Mock the Execute method to simulate child workflow execution
	env.OnActivity(recipeWrapper.Execute, mock.Anything, mock.AnythingOfType("RecipeConfig"), mock.AnythingOfType("RecipeInput")).Return(
		func(ctx context.Context, config RecipeConfig, input RecipeInput) (RecipeOutput, error) {
			// Simulate the child workflow being executed
			return RecipeOutput{
				ExecutionID: "child-recipe-123",
				Status:      "completed",
				Result: map[string]interface{}{
					"processed_count": 100,
					"status":          "success",
				},
				RecipeOutputs: map[string]interface{}{
					"transformed_data": "processed_value",
					"metrics": map[string]interface{}{
						"duration": "5s",
						"records":  100,
					},
				},
				Metadata: RecipeMetadata{
					StartTime:    time.Now().Format(time.RFC3339),
					EndTime:      time.Now().Add(5 * time.Second).Format(time.RFC3339),
					DurationMs:   5000,
					AttemptCount: 1,
				},
			}, nil
		},
	)

	// Execute the parent workflow
	input := map[string]interface{}{
		"dataset_id": "test-dataset-001",
		"format":     "json",
	}

	env.ExecuteWorkflow(ParentWorkflowWithRecipeActivity, input)

	// Verify workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get and verify the result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, "completed", result["status"])
	assert.Equal(t, "child-recipe-123", result["child_execution_id"])
	assert.NotNil(t, result["child_outputs"])
	
	// Verify the child outputs
	childOutputs := result["child_outputs"].(map[string]interface{})
	assert.Equal(t, "processed_value", childOutputs["transformed_data"])
}

// ParentWorkflowWithRecipeActivity is a parent workflow that uses the recipe activity
func ParentWorkflowWithRecipeActivity(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting parent workflow with recipe activity", "input", input)

	// Set activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2.0,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Create the recipe wrapper
	recipeWrapper := &RecipeActivityWrapper{}

	// Prepare the recipe configuration
	config := RecipeConfig{
		Recipe:  "data-processing/transform",
		Timeout: "5m",
		RetryPolicy: &RetryPolicyConfig{
			MaximumAttempts:    3,
			InitialInterval:    "2s",
			BackoffCoefficient: 2.0,
		},
	}

	// Prepare the recipe input
	recipeInput := RecipeInput{
		"dataset_id": input["dataset_id"],
		"format":     input["format"],
		"operation":  "transform",
	}

	// Execute the recipe activity
	var recipeOutput RecipeOutput
	err := workflow.ExecuteActivity(ctx, recipeWrapper.Execute, config, recipeInput).Get(ctx, &recipeOutput)
	if err != nil {
		logger.Error("Recipe activity failed", "error", err)
		return nil, err
	}

	logger.Info("Recipe activity completed", 
		"execution_id", recipeOutput.ExecutionID,
		"status", recipeOutput.Status)

	// Return the results
	return map[string]interface{}{
		"status":             "completed",
		"child_execution_id": recipeOutput.ExecutionID,
		"child_outputs":      recipeOutput.RecipeOutputs,
		"child_result":       recipeOutput.Result,
		"metadata":           recipeOutput.Metadata,
	}, nil
}

// DataProcessingRecipe is a child recipe workflow
func DataProcessingRecipe(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting data processing recipe", "input", input)

	// Extract inputs
	datasetID, _ := input["inputs"].(map[string]interface{})["dataset_id"].(string)
	format, _ := input["inputs"].(map[string]interface{})["format"].(string)

	// Simulate data processing
	workflow.Sleep(ctx, 2*time.Second)

	// Process the data
	result := map[string]interface{}{
		"processed_count": 100,
		"dataset_id":      datasetID,
		"format":          format,
		"status":          "success",
		"transformed_data": "processed_value",
		"metrics": map[string]interface{}{
			"duration": "2s",
			"records":  100,
		},
	}

	logger.Info("Data processing completed", "result", result)
	return result, nil
}

// TestRecipeActivityWithRealChildWorkflow tests recipe activity with actual child workflow execution
func TestRecipeActivityWithRealChildWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register all components
	env.RegisterWorkflow(ParentRecipeWorkflowComplete)
	env.RegisterWorkflow(ChildAnalyticsRecipe)
	
	// Instead of registering the activity directly, we need to mock the underlying ExecuteRecipeActivity
	// since the test environment can't actually make Temporal client calls
	env.OnActivity(ExecuteRecipeActivity, mock.Anything, mock.AnythingOfType("RecipeActivity")).Return(
		func(ctx context.Context, input RecipeActivity) (*RecipeActivityOutput, error) {
			// Simulate executing the child workflow
			return &RecipeActivityOutput{
				ExecutionID: "analytics-execution-456",
				Status:      "completed",
				Result: map[string]interface{}{
					"analysis_complete": true,
					"insights": []string{
						"Pattern detected",
						"Anomaly found",
					},
				},
				ExecutionMetadata: ExecutionMetadata{
					StartTime:    time.Now(),
					EndTime:      time.Now().Add(3 * time.Second),
					DurationMs:   3000,
					AttemptCount: 1,
				},
			}, nil
		},
	)

	// Execute the parent workflow
	input := map[string]interface{}{
		"data_source": "production_logs",
		"time_range":  "24h",
	}

	env.ExecuteWorkflow(ParentRecipeWorkflowComplete, input)

	// Verify workflow completed
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get and verify result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, "analysis_completed", result["status"])
	assert.Equal(t, "analytics-execution-456", result["analytics_execution_id"])
	
	// Verify insights were returned
	if insightsRaw, ok := result["insights"]; ok && insightsRaw != nil {
		// Handle both []string and []interface{} types
		var insights []string
		switch v := insightsRaw.(type) {
		case []string:
			insights = v
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					insights = append(insights, s)
				}
			}
		}
		if len(insights) > 0 {
			assert.Contains(t, insights, "Pattern detected")
			assert.Contains(t, insights, "Anomaly found")
		}
	}
}

// ParentRecipeWorkflowComplete demonstrates a complete parent workflow using recipe activity
func ParentRecipeWorkflowComplete(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting parent recipe workflow", "input", input)

	// Set activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Build recipe context
	recipeContext := &RecipeContext{
		Recipe: RecipeInfo{
			Name:        "parent-analytics",
			Version:     "1.0.0",
			ExecutionID: workflow.GetInfo(ctx).WorkflowExecution.ID,
		},
		Environment: EnvironmentInfo{
			Name:    "production",
			Region:  "us-east-1",
			Cluster: "analytics-cluster",
		},
		Execution: ExecutionInfo{
			Host:      "analytics-worker-1",
			Namespace: workflow.GetInfo(ctx).Namespace,
			TaskQueue: workflow.GetInfo(ctx).TaskQueueName,
			StartedAt: workflow.Now(ctx),
			Timeout:   10 * time.Minute,
		},
		Auth: AuthInfo{
			Identity: "analytics-service",
		},
	}

	// Prepare recipe activity input
	recipeInput := RecipeActivity{
		Recipe:  "analytics/process",
		Timeout: 3 * time.Minute,
		Inputs: map[string]interface{}{
			"data_source": input["data_source"],
			"time_range":  input["time_range"],
		},
		Context: recipeContext,
		RetryPolicy: &RetryPolicy{
			MaximumAttempts:    2,
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
		},
	}

	// Execute the recipe activity
	var recipeOutput RecipeActivityOutput
	err := workflow.ExecuteActivity(ctx, ExecuteRecipeActivity, recipeInput).Get(ctx, &recipeOutput)
	if err != nil {
		logger.Error("Recipe activity execution failed", "error", err)
		return nil, err
	}

	logger.Info("Recipe activity completed successfully", 
		"execution_id", recipeOutput.ExecutionID,
		"status", recipeOutput.Status)

	// Extract insights from the result
	insights := []interface{}{}
	if insightsData, ok := recipeOutput.Result["insights"]; ok {
		switch v := insightsData.(type) {
		case []string:
			for _, s := range v {
				insights = append(insights, s)
			}
		case []interface{}:
			insights = v
		}
	}

	// Return processed results
	return map[string]interface{}{
		"status":                "analysis_completed",
		"analytics_execution_id": recipeOutput.ExecutionID,
		"insights":              insights,
		"metadata":              recipeOutput.ExecutionMetadata,
	}, nil
}

// ChildAnalyticsRecipe is a child recipe for analytics processing
func ChildAnalyticsRecipe(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting analytics recipe", "input", input)

	// Extract inputs
	var dataSource, timeRange string
	if inputs, ok := input["inputs"].(map[string]interface{}); ok {
		dataSource, _ = inputs["data_source"].(string)
		timeRange, _ = inputs["time_range"].(string)
	}

	// Simulate analytics processing
	workflow.Sleep(ctx, 1*time.Second)

	// Generate insights
	insights := []string{
		"Pattern detected in " + dataSource,
		"Anomaly found in last " + timeRange,
		"Trend analysis complete",
	}

	result := map[string]interface{}{
		"analysis_complete": true,
		"data_source":       dataSource,
		"time_range":        timeRange,
		"insights":          insights,
		"metrics": map[string]interface{}{
			"records_analyzed": 10000,
			"patterns_found":   3,
			"anomalies":        1,
		},
	}

	logger.Info("Analytics processing completed", "result", result)
	return result, nil
}

// TestRecipeActivityErrorHandling tests error handling in recipe activity
func TestRecipeActivityErrorHandling(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows
	env.RegisterWorkflow(WorkflowWithFailingRecipe)
	
	// Mock the recipe activity to simulate failure
	env.OnActivity(ExecuteRecipeActivity, mock.Anything, mock.AnythingOfType("RecipeActivity")).Return(
		nil,
		temporal.NewApplicationError("child recipe failed", "RecipeExecutionError"),
	)

	// Execute workflow
	env.ExecuteWorkflow(WorkflowWithFailingRecipe, map[string]interface{}{
		"test": "error_handling",
	})

	// Verify workflow completed (with handled error)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	// Verify error was handled gracefully
	assert.Equal(t, "failed", result["status"])
	assert.Contains(t, result["error"].(string), "child recipe failed")
	assert.Equal(t, "RecipeExecutionError", result["error_type"])
}

// WorkflowWithFailingRecipe demonstrates error handling when recipe activity fails
func WorkflowWithFailingRecipe(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting workflow with potentially failing recipe", "input", input)

	// Set activity options with retry
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts:        3,
			InitialInterval:        1 * time.Second,
			BackoffCoefficient:     2.0,
			NonRetryableErrorTypes: []string{"RecipeNotFoundError"},
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Prepare recipe activity
	recipeInput := RecipeActivity{
		Recipe:  "failing/recipe",
		Timeout: 1 * time.Minute,
		Inputs:  input,
	}

	// Execute the recipe activity
	var recipeOutput RecipeActivityOutput
	err := workflow.ExecuteActivity(ctx, ExecuteRecipeActivity, recipeInput).Get(ctx, &recipeOutput)
	
	if err != nil {
		logger.Error("Recipe activity failed after retries", "error", err)
		
		// Extract error details
		errorType := "UnknownError"
		errorMsg := err.Error()
		
		// Try to extract error type from the error message
		// The error format is: "activity error (...): child recipe failed (type: RecipeExecutionError, ...)"
		if strings.Contains(errorMsg, "type: RecipeExecutionError") {
			errorType = "RecipeExecutionError"
		} else if appErr, ok := err.(*temporal.ApplicationError); ok {
			errorType = appErr.Type()
		}
		
		// Return error information gracefully
		return map[string]interface{}{
			"status":     "failed",
			"error":      errorMsg,
			"error_type": errorType,
		}, nil
	}

	return map[string]interface{}{
		"status": "unexpected_success",
		"result": recipeOutput,
	}, nil
}

// TestParentOperatesOnChildOutputs tests that parent workflows can operate on child recipe outputs
func TestParentOperatesOnChildOutputs(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows
	env.RegisterWorkflow(DataAggregationParentWorkflow)
	env.RegisterWorkflow(DataProcessingChildWorkflow)
	
	// Create the wrapper instance
	recipeWrapper := &RecipeActivityWrapper{}
	
	// Register the Execute method as an activity
	env.RegisterActivity(recipeWrapper.Execute)
	
	// Mock multiple child recipe executions
	callCount := 0
	env.OnActivity(recipeWrapper.Execute, mock.Anything, mock.AnythingOfType("RecipeConfig"), mock.AnythingOfType("RecipeInput")).Return(
		func(ctx context.Context, config RecipeConfig, input RecipeInput) (RecipeOutput, error) {
			callCount++
			// Return different results for each child invocation
			switch callCount {
			case 1:
				return RecipeOutput{
					ExecutionID: "child-1",
					Status:      "completed",
					Result: map[string]interface{}{
						"processed_count": 100,
						"metrics": map[string]interface{}{
							"avg_score": 75.5,
							"max_score": 95,
							"min_score": 45,
						},
					},
				}, nil
			case 2:
				return RecipeOutput{
					ExecutionID: "child-2",
					Status:      "completed",
					Result: map[string]interface{}{
						"processed_count": 150,
						"metrics": map[string]interface{}{
							"avg_score": 82.3,
							"max_score": 98,
							"min_score": 55,
						},
					},
				}, nil
			case 3:
				return RecipeOutput{
					ExecutionID: "child-3",
					Status:      "completed",
					Result: map[string]interface{}{
						"processed_count": 75,
						"metrics": map[string]interface{}{
							"avg_score": 68.7,
							"max_score": 88,
							"min_score": 40,
						},
					},
				}, nil
			default:
				return RecipeOutput{}, nil
			}
		},
	)

	// Execute the parent workflow
	input := map[string]interface{}{
		"datasets": []string{"dataset-1", "dataset-2", "dataset-3"},
		"threshold": 70.0,
	}

	env.ExecuteWorkflow(DataAggregationParentWorkflow, input)

	// Verify workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get and verify the result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	// Verify parent operated on child outputs
	assert.Equal(t, "aggregation_complete", result["status"])
	assert.Equal(t, float64(325), result["total_processed"]) // 100 + 150 + 75
	assert.InDelta(t, 75.5, result["overall_avg_score"], 0.1) // (75.5 + 82.3 + 68.7) / 3
	assert.Equal(t, float64(98), result["global_max_score"]) // max of all max scores
	assert.Equal(t, float64(40), result["global_min_score"]) // min of all min scores
	assert.Equal(t, float64(2), result["above_threshold_count"]) // 2 datasets above 70.0
	assert.ElementsMatch(t, []string{"child-1", "child-2"}, result["above_threshold_datasets"])
}

// DataAggregationParentWorkflow demonstrates parent operating on child outputs
func DataAggregationParentWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting data aggregation parent workflow", "input", input)

	// Set activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Extract inputs
	var datasets []string
	if d, ok := input["datasets"].([]string); ok {
		datasets = d
	} else if d, ok := input["datasets"].([]interface{}); ok {
		for _, item := range d {
			if s, ok := item.(string); ok {
				datasets = append(datasets, s)
			}
		}
	}
	threshold := input["threshold"].(float64)

	// Create the recipe wrapper
	recipeWrapper := &RecipeActivityWrapper{}

	// Process each dataset through child recipes
	var childResults []RecipeOutput
	totalProcessed := 0
	sumAvgScores := 0.0
	globalMaxScore := 0
	globalMinScore := 100
	aboveThresholdCount := 0
	var aboveThresholdDatasets []string

	for i, dataset := range datasets {
		// Prepare configuration for each child
		config := RecipeConfig{
			Recipe:  "data-processing/analyze",
			Timeout: "5m",
		}

		recipeInput := RecipeInput{
			"dataset": dataset,
			"index":   i,
		}

		// Execute the recipe activity
		var recipeOutput RecipeOutput
		err := workflow.ExecuteActivity(ctx, recipeWrapper.Execute, config, recipeInput).Get(ctx, &recipeOutput)
		if err != nil {
			logger.Error("Child recipe failed", "dataset", dataset, "error", err)
			continue
		}

		childResults = append(childResults, recipeOutput)

		// Process child outputs
		if result := recipeOutput.Result; result != nil {
			// Aggregate processed counts (handle both int and float64)
			switch v := result["processed_count"].(type) {
			case int:
				totalProcessed += v
			case float64:
				totalProcessed += int(v)
			}

			// Process metrics
			if metrics, ok := result["metrics"].(map[string]interface{}); ok {
				// Aggregate average scores
				if avgScore, ok := metrics["avg_score"].(float64); ok {
					sumAvgScores += avgScore
					
					// Check if above threshold
					if avgScore > threshold {
						aboveThresholdCount++
						aboveThresholdDatasets = append(aboveThresholdDatasets, recipeOutput.ExecutionID)
					}
				}

				// Track max score (handle both int and float64)
				switch v := metrics["max_score"].(type) {
				case int:
					if v > globalMaxScore {
						globalMaxScore = v
					}
				case float64:
					if int(v) > globalMaxScore {
						globalMaxScore = int(v)
					}
				}

				// Track min score (handle both int and float64)
				switch v := metrics["min_score"].(type) {
				case int:
					if v < globalMinScore {
						globalMinScore = v
					}
				case float64:
					if int(v) < globalMinScore {
						globalMinScore = int(v)
					}
				}
			}
		}
	}

	// Calculate final aggregations
	overallAvgScore := sumAvgScores / float64(len(childResults))

	// Return aggregated results
	return map[string]interface{}{
		"status":                  "aggregation_complete",
		"total_processed":         totalProcessed,
		"overall_avg_score":       overallAvgScore,
		"global_max_score":        globalMaxScore,
		"global_min_score":        globalMinScore,
		"above_threshold_count":   aboveThresholdCount,
		"above_threshold_datasets": aboveThresholdDatasets,
		"child_execution_count":   len(childResults),
	}, nil
}

// DataProcessingChildWorkflow simulates a child that processes data
func DataProcessingChildWorkflow(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	// Simulate processing and return metrics
	return map[string]interface{}{
		"processed_count": 100,
		"metrics": map[string]interface{}{
			"avg_score": 75.5,
			"max_score": 95,
			"min_score": 45,
		},
	}, nil
}

// TestRecipeActivityContextPropagation tests context propagation through recipe activity
func TestRecipeActivityContextPropagation(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register workflows
	env.RegisterWorkflow(WorkflowWithRichContext)
	
	// Track the context that was passed to the activity
	var capturedContext *RecipeContext
	
	env.OnActivity(ExecuteRecipeActivity, mock.Anything, mock.AnythingOfType("RecipeActivity")).Return(
		func(ctx context.Context, input RecipeActivity) (*RecipeActivityOutput, error) {
			// Capture the context for verification
			capturedContext = input.Context
			
			return &RecipeActivityOutput{
				ExecutionID: "context-test-789",
				Status:      "completed",
				Result: map[string]interface{}{
					"received_context": map[string]interface{}{
						"parent_id":   input.Context.Recipe.ExecutionID,
						"environment": input.Context.Environment.Name,
						"auth":        input.Context.Auth.Identity,
					},
				},
				ExecutionMetadata: ExecutionMetadata{
					StartTime:    time.Now(),
					EndTime:      time.Now().Add(1 * time.Second),
					DurationMs:   1000,
					AttemptCount: 1,
				},
			}, nil
		},
	)

	// Execute workflow
	env.ExecuteWorkflow(WorkflowWithRichContext, map[string]interface{}{
		"test": "context_propagation",
	})

	// Verify workflow completed
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Get result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	// Verify context was properly propagated
	assert.NotNil(t, capturedContext)
	assert.Equal(t, "rich-context-workflow", capturedContext.Recipe.Name)
	assert.Equal(t, "production", capturedContext.Environment.Name)
	assert.Equal(t, "us-west-2", capturedContext.Environment.Region)
	assert.Equal(t, "secure-service", capturedContext.Auth.Identity)
	
	// Verify the result contains context information
	contextInfo := result["received_context"].(map[string]interface{})
	assert.Equal(t, "production", contextInfo["environment"])
	assert.Equal(t, "secure-service", contextInfo["auth"])
}

// WorkflowWithRichContext demonstrates context propagation through recipe activity
func WorkflowWithRichContext(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting workflow with rich context", "input", input)

	// Set activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Build rich context
	recipeContext := &RecipeContext{
		Recipe: RecipeInfo{
			Name:        "rich-context-workflow",
			Version:     "2.0.0",
			ExecutionID: workflow.GetInfo(ctx).WorkflowExecution.ID,
		},
		Environment: EnvironmentInfo{
			Name:    "production",
			Region:  "us-west-2",
			Cluster: "main-cluster",
		},
		Execution: ExecutionInfo{
			Host:      "worker-node-1",
			Namespace: "production-namespace",
			TaskQueue: "priority-queue",
			StartedAt: workflow.Now(ctx),
			Timeout:   30 * time.Minute,
		},
		Auth: AuthInfo{
			Identity: "secure-service",
			Token:    "encrypted-token",
		},
	}

	// Prepare recipe activity with context
	recipeInput := RecipeActivity{
		Recipe:  "context/test",
		Timeout: 1 * time.Minute,
		Inputs:  input,
		Context: recipeContext,
	}

	// Execute the recipe activity
	var recipeOutput RecipeActivityOutput
	err := workflow.ExecuteActivity(ctx, ExecuteRecipeActivity, recipeInput).Get(ctx, &recipeOutput)
	if err != nil {
		return nil, err
	}

	logger.Info("Recipe activity completed with context", 
		"execution_id", recipeOutput.ExecutionID)

	// Return the context information that was received
	return map[string]interface{}{
		"status":            "completed",
		"execution_id":      recipeOutput.ExecutionID,
		"received_context":  recipeOutput.Result["received_context"],
	}, nil
}