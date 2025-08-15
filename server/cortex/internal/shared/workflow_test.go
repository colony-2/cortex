package shared

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// SharedWorkflowTestSuite tests workflow execution using Temporal's test framework
type SharedWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *SharedWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.env.SetTestTimeout(5 * time.Second)
}

func (s *SharedWorkflowTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func TestSharedWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(SharedWorkflowTestSuite))
}

// Mock activity implementations
func validateDocumentActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	score, _ := inputs["score"].(float64)
	confidence, _ := inputs["confidence"].(float64)
	
	return map[string]interface{}{
		"valid":      score >= 60,
		"score":      score,
		"confidence": confidence,
		"timestamp":  time.Now().Unix(),
	}, nil
}

func processDataActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	data, _ := inputs["data"].(string)
	return map[string]interface{}{
		"processed": data + "_processed",
		"size":      len(data),
	}, nil
}

func transformActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	data, _ := inputs["data"].(string)
	return map[string]interface{}{
		"transformed": "transformed_" + data,
	}, nil
}

func aggregateResultsActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	results := []interface{}{}
	if r1, ok := inputs["result1"]; ok {
		results = append(results, r1)
	}
	if r2, ok := inputs["result2"]; ok {
		results = append(results, r2)
	}
	
	return map[string]interface{}{
		"aggregated": results,
		"count":      len(results),
	}, nil
}

func retryableActivity(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
	attempt, _ := inputs["attempt"].(int)
	maxAttempts, _ := inputs["max_attempts"].(int)
	
	if attempt < maxAttempts {
		return nil, temporal.NewApplicationError("temporary failure", "RETRY_ERROR", nil)
	}
	
	return map[string]interface{}{
		"success": true,
		"attempt": attempt,
	}, nil
}

// TestNestedSequentialParallelComposition tests deeply nested compositions
func (s *SharedWorkflowTestSuite) TestNestedSequentialParallelComposition() {
	// Register activities
	s.env.RegisterActivityWithOptions(
		processDataActivity,
		activity.RegisterOptions{Name: "process_data"},
	)
	s.env.RegisterActivityWithOptions(
		transformActivity,
		activity.RegisterOptions{Name: "transform"},
	)
	s.env.RegisterActivityWithOptions(
		validateDocumentActivity,
		activity.RegisterOptions{Name: "validate"},
	)
	s.env.RegisterActivityWithOptions(
		aggregateResultsActivity,
		activity.RegisterOptions{Name: "aggregate_results"},
	)

	// Create nested composition workflow
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		outputs := make(map[string]interface{})
		
		// Sequential execution
		// Step 1: Prepare data
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Second,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		var prepareResult map[string]interface{}
		err := workflow.ExecuteActivity(ctx, "process_data", map[string]interface{}{
			"data": inputs["data"],
		}).Get(ctx, &prepareResult)
		if err != nil {
			return nil, err
		}
		outputs["prepare"] = prepareResult
		
		// Step 2: Parallel processing
		selector := workflow.NewSelector(ctx)
		
		// Branch 1: Sequential transform and validate
		// Execute both activities in sequence using futures
		var branch1Result map[string]interface{}
		branch1Chan := workflow.NewChannel(ctx)
		
		workflow.Go(ctx, func(ctx workflow.Context) {
			branch1Outputs := make(map[string]interface{})
			
			// Transform
			var transformResult map[string]interface{}
			err := workflow.ExecuteActivity(ctx, "transform", map[string]interface{}{
				"data": prepareResult["processed"],
			}).Get(ctx, &transformResult)
			if err != nil {
				branch1Chan.Send(ctx, nil)
				return
			}
			branch1Outputs["transform"] = transformResult
			
			// Validate
			var validateResult map[string]interface{}
			err = workflow.ExecuteActivity(ctx, "validate", map[string]interface{}{
				"score":      85.0,
				"confidence": 0.95,
			}).Get(ctx, &validateResult)
			if err != nil {
				branch1Chan.Send(ctx, nil)
				return
			}
			branch1Outputs["validate"] = validateResult
			
			branch1Chan.Send(ctx, branch1Outputs)
		})
		
		// Branch 2: Direct process
		branch2Future := workflow.ExecuteActivity(ctx, "process_data", map[string]interface{}{
			"data": prepareResult["processed"],
		})
		
		// Wait for both branches
		var branch2Result map[string]interface{}
		selector.AddReceive(branch1Chan, func(c workflow.ReceiveChannel, more bool) {
			c.Receive(ctx, &branch1Result)
		})
		selector.AddFuture(branch2Future, func(f workflow.Future) {
			f.Get(ctx, &branch2Result)
		})
		
		// Wait for both to complete
		for i := 0; i < 2; i++ {
			selector.Select(ctx)
		}
		
		outputs["branch1"] = branch1Result
		outputs["branch2"] = branch2Result
		
		// Step 3: Aggregate results
		var aggregateResult map[string]interface{}
		err = workflow.ExecuteActivity(ctx, "aggregate_results", map[string]interface{}{
			"result1": branch1Result,
			"result2": branch2Result,
		}).Get(ctx, &aggregateResult)
		if err != nil {
			return nil, err
		}
		outputs["aggregate"] = aggregateResult
		
		return outputs, nil
	}

	// Register and execute workflow
	s.env.RegisterWorkflow(workflowFunc)
	
	// Mock activity expectations
	s.env.OnActivity("process_data", mock.Anything, mock.MatchedBy(func(inputs map[string]interface{}) bool {
		return inputs["data"] == "test_data"
	})).Return(map[string]interface{}{
		"processed": "test_data_processed",
		"size":      9,
	}, nil).Once()
	
	s.env.OnActivity("transform", mock.Anything, mock.Anything).Return(map[string]interface{}{
		"transformed": "transformed_data",
	}, nil)
	
	s.env.OnActivity("validate", mock.Anything, mock.Anything).Return(map[string]interface{}{
		"valid":      true,
		"score":      85.0,
		"confidence": 0.95,
	}, nil)
	
	s.env.OnActivity("process_data", mock.Anything, mock.MatchedBy(func(inputs map[string]interface{}) bool {
		return inputs["data"] == "test_data_processed"
	})).Return(map[string]interface{}{
		"processed": "double_processed",
		"size":      15,
	}, nil).Once()
	
	s.env.OnActivity("aggregate_results", mock.Anything, mock.Anything).Return(map[string]interface{}{
		"aggregated": []interface{}{"result1", "result2"},
		"count":      2,
	}, nil)
	
	// Execute workflow
	s.env.ExecuteWorkflow(workflowFunc, map[string]interface{}{
		"data": "test_data",
	})
	
	// Verify results
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result["prepare"])
	s.NotNil(result["branch1"])
	s.NotNil(result["branch2"])
	s.NotNil(result["aggregate"])
	
	// Verify aggregate contains both results
	aggregate := result["aggregate"].(map[string]interface{})
	s.Equal(2, int(aggregate["count"].(float64)))
}

// TestComplexStateTransitions tests state machine transitions with CEL expressions
func (s *SharedWorkflowTestSuite) TestComplexStateTransitions() {
	// Create state machine workflow
	stateMachineWorkflow := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		state := "validation"
		outputs := make(map[string]interface{})
		stateOutputs := make(map[string]interface{})
		
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Second,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		for state != "" && state != "complete" && state != "rejected" {
			switch state {
			case "validation":
				var result map[string]interface{}
				err := workflow.ExecuteActivity(ctx, "validate_document", inputs).Get(ctx, &result)
				if err != nil {
					return nil, err
				}
				stateOutputs = result
				
				// Evaluate transitions (simulating CEL evaluation)
				score := result["score"].(float64)
				confidence := result["confidence"].(float64)
				
				if score >= 80 && confidence > 0.9 {
					state = "approved"
				} else if score >= 60 {
					state = "review"
				} else {
					state = "rejected"
				}
				
			case "review":
				// Manual review simulation
				var result map[string]interface{}
				err := workflow.ExecuteActivity(ctx, "manual_review", stateOutputs).Get(ctx, &result)
				if err != nil {
					return nil, err
				}
				
				if result["approved"].(bool) {
					state = "approved"
				} else {
					state = "rejected"
				}
				
			case "approved":
				// Process approved document
				var result map[string]interface{}
				err := workflow.ExecuteActivity(ctx, "process_approved", stateOutputs).Get(ctx, &result)
				if err != nil {
					return nil, err
				}
				outputs["final_result"] = result
				state = "complete"
				
			default:
				state = ""
			}
			
			outputs["last_state"] = state
		}
		
		outputs["final_state"] = state
		outputs["state_outputs"] = stateOutputs
		return outputs, nil
	}
	
	// Register activities
	s.env.RegisterActivityWithOptions(
		validateDocumentActivity,
		activity.RegisterOptions{Name: "validate_document"},
	)
	
	s.env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"approved": true}, nil
		},
		activity.RegisterOptions{Name: "manual_review"},
	)
	
	s.env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"processed": true}, nil
		},
		activity.RegisterOptions{Name: "process_approved"},
	)
	
	// Register workflow
	s.env.RegisterWorkflow(stateMachineWorkflow)
	
	// Test Case 1: High score, high confidence -> direct approval
	s.env.OnActivity("validate_document", mock.Anything, mock.Anything).Return(
		map[string]interface{}{
			"score":      85.0,
			"confidence": 0.95,
			"valid":      true,
		}, nil,
	).Once()
	
	s.env.OnActivity("process_approved", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"processed": true}, nil,
	).Once()
	
	s.env.ExecuteWorkflow(stateMachineWorkflow, map[string]interface{}{
		"document": "test.pdf",
	})
	
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal("complete", result["final_state"])
	s.Equal("complete", result["last_state"])
}

// TestRetryPolicyWithBackoff tests retry logic with exponential backoff
func (s *SharedWorkflowTestSuite) TestRetryPolicyWithBackoff() {
	retryWorkflow := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		// Configure retry policy
		retryPolicy := &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    3,
		}
		
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy:         retryPolicy,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		// Track attempts
		attempts := 0
		outputs := make(map[string]interface{})
		
		// Execute activity with retry
		var result map[string]interface{}
		err := workflow.ExecuteActivity(ctx, "flaky_activity", map[string]interface{}{
			"attempt": attempts,
		}).Get(ctx, &result)
		
		if err != nil {
			outputs["error"] = err.Error()
			outputs["attempts"] = attempts
			return outputs, err
		}
		
		outputs["result"] = result
		outputs["success"] = true
		return outputs, nil
	}
	
	// Register flaky activity that fails first 2 times
	attemptCount := 0
	s.env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			attemptCount++
			if attemptCount < 3 {
				return nil, temporal.NewApplicationError("temporary failure", "RETRY_ERROR", nil)
			}
			return map[string]interface{}{
				"success": true,
				"attempt": attemptCount,
			}, nil
		},
		activity.RegisterOptions{Name: "flaky_activity"},
	)
	
	// Register workflow
	s.env.RegisterWorkflow(retryWorkflow)
	
	// Execute workflow
	s.env.ExecuteWorkflow(retryWorkflow, map[string]interface{}{
		"data": "test",
	})
	
	// Verify successful completion after retries
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.True(result["success"].(bool))
	s.NotNil(result["result"])
}

// TestErrorHandlingAndRecovery tests error handling at various levels
func (s *SharedWorkflowTestSuite) TestErrorHandlingAndRecovery() {
	errorWorkflow := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		outputs := make(map[string]interface{})
		errors := []string{}
		
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts: 1, // No retries for this test
			},
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		// Execute parallel activities with potential failures
		selector := workflow.NewSelector(ctx)
		
		// Activity 1: Will succeed
		future1 := workflow.ExecuteActivity(ctx, "success_activity", inputs)
		
		// Activity 2: Will fail
		future2 := workflow.ExecuteActivity(ctx, "failing_activity", inputs)
		
		// Activity 3: Will succeed
		future3 := workflow.ExecuteActivity(ctx, "success_activity_2", inputs)
		
		// Collect results
		var result1, result3 map[string]interface{}
		var err2 error
		
		selector.AddFuture(future1, func(f workflow.Future) {
			f.Get(ctx, &result1)
			outputs["activity1"] = result1
		})
		
		selector.AddFuture(future2, func(f workflow.Future) {
			err2 = f.Get(ctx, nil)
			if err2 != nil {
				errors = append(errors, err2.Error())
			}
		})
		
		selector.AddFuture(future3, func(f workflow.Future) {
			f.Get(ctx, &result3)
			outputs["activity3"] = result3
		})
		
		// Wait for all activities
		for i := 0; i < 3; i++ {
			selector.Select(ctx)
		}
		
		// Compensation logic if errors occurred
		if len(errors) > 0 {
			outputs["errors"] = errors
			outputs["compensation_needed"] = true
			
			// Execute compensation
			var compensationResult map[string]interface{}
			err := workflow.ExecuteActivity(ctx, "compensation_activity", map[string]interface{}{
				"errors": errors,
			}).Get(ctx, &compensationResult)
			
			if err != nil {
				return outputs, err
			}
			
			outputs["compensation"] = compensationResult
		}
		
		outputs["completed"] = true
		return outputs, nil
	}
	
	// Register activities
	s.env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"success": true}, nil
		},
		activity.RegisterOptions{Name: "success_activity"},
	)
	
	s.env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return nil, errors.New("simulated failure")
		},
		activity.RegisterOptions{Name: "failing_activity"},
	)
	
	s.env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"success": true}, nil
		},
		activity.RegisterOptions{Name: "success_activity_2"},
	)
	
	s.env.RegisterActivityWithOptions(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"compensated": true}, nil
		},
		activity.RegisterOptions{Name: "compensation_activity"},
	)
	
	// Register and execute workflow
	s.env.RegisterWorkflow(errorWorkflow)
	s.env.ExecuteWorkflow(errorWorkflow, map[string]interface{}{
		"data": "test",
	})
	
	// Verify workflow completed despite errors
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.True(result["completed"].(bool))
	s.True(result["compensation_needed"].(bool))
	s.NotNil(result["compensation"])
	s.NotNil(result["activity1"])
	s.NotNil(result["activity3"])
	s.NotNil(result["errors"])
}

// TestDocumentProcessingPipeline tests a complete document processing workflow
func (s *SharedWorkflowTestSuite) TestDocumentProcessingPipeline() {
	pipelineWorkflow := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		outputs := make(map[string]interface{})
		
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		// Stage 1: Document ingestion
		var ingestionResult map[string]interface{}
		err := workflow.ExecuteActivity(ctx, "ingest_document", inputs).Get(ctx, &ingestionResult)
		if err != nil {
			return nil, err
		}
		outputs["ingestion"] = ingestionResult
		
		// Stage 2: Parallel processing (OCR and metadata extraction)
		selector := workflow.NewSelector(ctx)
		
		ocrFuture := workflow.ExecuteActivity(ctx, "ocr_processing", ingestionResult)
		metadataFuture := workflow.ExecuteActivity(ctx, "extract_metadata", ingestionResult)
		
		var ocrResult, metadataResult map[string]interface{}
		
		selector.AddFuture(ocrFuture, func(f workflow.Future) {
			f.Get(ctx, &ocrResult)
		})
		
		selector.AddFuture(metadataFuture, func(f workflow.Future) {
			f.Get(ctx, &metadataResult)
		})
		
		// Wait for both
		for i := 0; i < 2; i++ {
			selector.Select(ctx)
		}
		
		outputs["ocr"] = ocrResult
		outputs["metadata"] = metadataResult
		
		// Stage 3: Review process (state machine)
		reviewState := "initial_review"
		reviewComplete := false
		
		for !reviewComplete {
			switch reviewState {
			case "initial_review":
				var reviewResult map[string]interface{}
				err := workflow.ExecuteActivity(ctx, "automated_review", map[string]interface{}{
					"ocr":      ocrResult,
					"metadata": metadataResult,
				}).Get(ctx, &reviewResult)
				if err != nil {
					return nil, err
				}
				
				if reviewResult["score"].(float64) >= 80 {
					reviewState = "approved"
				} else {
					reviewState = "manual_review"
				}
				
			case "manual_review":
				var manualResult map[string]interface{}
				err := workflow.ExecuteActivity(ctx, "manual_review", map[string]interface{}{
					"document": ingestionResult,
				}).Get(ctx, &manualResult)
				if err != nil {
					return nil, err
				}
				
				if manualResult["approved"].(bool) {
					reviewState = "approved"
				} else {
					reviewState = "rejected"
				}
				
			case "approved", "rejected":
				reviewComplete = true
				outputs["review_state"] = reviewState
			}
		}
		
		// Stage 4: Final storage
		if reviewState == "approved" {
			var storageResult map[string]interface{}
			err := workflow.ExecuteActivity(ctx, "store_document", map[string]interface{}{
				"document": ingestionResult,
				"metadata": metadataResult,
				"ocr":      ocrResult,
			}).Get(ctx, &storageResult)
			if err != nil {
				return nil, err
			}
			outputs["storage"] = storageResult
		}
		
		outputs["status"] = reviewState
		outputs["completed"] = true
		return outputs, nil
	}
	
	// Register all activities
	activities := map[string]interface{}{
		"ingest_document": func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"document_id": "doc123",
				"size":        1024,
				"type":        "pdf",
			}, nil
		},
		"ocr_processing": func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"text":       "Extracted text content",
				"confidence": 0.95,
			}, nil
		},
		"extract_metadata": func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"author":      "John Doe",
				"created":     "2024-01-01",
				"page_count":  10,
			}, nil
		},
		"automated_review": func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"score":      85.0,
				"confidence": 0.9,
			}, nil
		},
		"store_document": func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"storage_path": "/documents/doc123",
				"stored":       true,
			}, nil
		},
	}
	
	for name, fn := range activities {
		s.env.RegisterActivityWithOptions(fn, activity.RegisterOptions{Name: name})
	}
	
	// Register and execute workflow
	s.env.RegisterWorkflow(pipelineWorkflow)
	s.env.ExecuteWorkflow(pipelineWorkflow, map[string]interface{}{
		"document_path": "/input/test.pdf",
	})
	
	// Verify complete pipeline execution
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.True(result["completed"].(bool))
	s.Equal("approved", result["status"])
	s.NotNil(result["ingestion"])
	s.NotNil(result["ocr"])
	s.NotNil(result["metadata"])
	s.NotNil(result["storage"])
}
