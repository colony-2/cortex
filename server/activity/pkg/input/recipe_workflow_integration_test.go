package input

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// SimulatedRecipeWorkflow represents a recipe workflow that uses the input activity
// This simulates how the recipe-worker would execute a recipe with input steps
func SimulatedRecipeWorkflow(ctx workflow.Context, recipeConfig map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting recipe workflow", "config", recipeConfig)
	
	result := map[string]interface{}{
		"recipe_id": recipeConfig["id"],
		"status":    "started",
		"outputs":   map[string]interface{}{},
	}
	
	// Get the steps from the recipe config
	steps := recipeConfig["steps"].([]interface{})
	
	for i, stepInterface := range steps {
		step := stepInterface.(map[string]interface{})
		stepName := step["name"].(string)
		stepType := step["type"].(string)
		
		logger.Info("Executing step", "name", stepName, "type", stepType, "index", i)
		
		if stepType == "input" {
			// This is an input activity step
			activityOptions := workflow.ActivityOptions{
				StartToCloseTimeout: 5 * time.Minute,
			}
			ctx = workflow.WithActivityOptions(ctx, activityOptions)
			
			// Get the input activity configuration
			inputConfig := step["config"].(map[string]interface{})
			
			// Create the Config struct from the recipe config
			var activityConfig Config
			if question, ok := inputConfig["question"].(string); ok {
				// Single question format
				activityConfig.Question = question
				if typeStr, ok := inputConfig["type"].(string); ok {
					activityConfig.Type = FieldType(typeStr)
				}
				if options, ok := inputConfig["options"].([]interface{}); ok {
					for _, opt := range options {
						optMap := opt.(map[string]interface{})
						activityConfig.Options = append(activityConfig.Options, Option{
							Value: optMap["value"].(string),
							Label: optMap["label"].(string),
						})
					}
				}
			} else if fields, ok := inputConfig["fields"].([]interface{}); ok {
				// Multi-field format
				activityConfig.Title = inputConfig["title"].(string)
				for _, field := range fields {
					fieldMap := field.(map[string]interface{})
					formField := FormField{
						ID:       fieldMap["id"].(string),
						Type:     FieldType(fieldMap["type"].(string)),
						Question: fieldMap["question"].(string),
						Required: fieldMap["required"].(bool),
					}
					if options, ok := fieldMap["options"].([]interface{}); ok {
						for _, opt := range options {
							optMap := opt.(map[string]interface{})
							// Handle both string and other types for value
							var value string
							switch v := optMap["value"].(type) {
							case string:
								value = v
							case bool:
								if v {
									value = "true"
								} else {
									value = "false"
								}
							default:
								value = fmt.Sprintf("%v", v)
							}
							formField.Options = append(formField.Options, Option{
								Value: value,
								Label: optMap["label"].(string),
							})
						}
					}
					activityConfig.Fields = append(activityConfig.Fields, formField)
				}
			}
			
			if timeout, ok := inputConfig["timeout"].(float64); ok {
				activityConfig.Timeout = int(timeout)
			} else {
				activityConfig.Timeout = 300 // Default 5 minutes
			}
			
			// Handle default on timeout
			if defaultVal, ok := inputConfig["default_on_timeout"]; ok {
				activityConfig.DefaultOnTimeout = defaultVal
			}
			
			// Create the input for the activity
			activityInput := Input{
				BoxID:      recipeConfig["box_id"].(string),
				ActivityID: stepName,
				Context:    result["outputs"].(map[string]interface{}),
			}
			
			// Execute the input activity
			var output Output
			err := workflow.ExecuteActivity(ctx, InputActivityExecute, activityConfig, activityInput).Get(ctx, &output)
			if err != nil {
				logger.Error("Input activity failed", "step", stepName, "error", err)
				result["status"] = "failed"
				result["error"] = err.Error()
				return result, err
			}
			
			// Store the output
			outputs := result["outputs"].(map[string]interface{})
			outputs[stepName] = map[string]interface{}{
				"response": output.Response,
				"fields":   output.Fields,
				"user_id":  output.UserID,
				"metadata": output.Metadata,
			}
			result["outputs"] = outputs
			
		} else if stepType == "action" {
			// Simulate other activity types (not implemented in this test)
			outputs := result["outputs"].(map[string]interface{})
			outputs[stepName] = map[string]interface{}{
				"status": "completed",
				"type":   stepType,
			}
			result["outputs"] = outputs
		}
	}
	
	result["status"] = "completed"
	return result, nil
}

// TestRecipeWithSingleInputActivity tests a recipe that has a single input activity
func TestRecipeWithSingleInputActivity(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Register the simulated recipe workflow
	env.RegisterWorkflow(SimulatedRecipeWorkflow)
	
	// Mock the input activity to return a specific response
	env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, config Config, input Input) (Output, error) {
			// Verify the activity received the correct configuration
			assert.Equal(t, "Do you want to proceed with deployment?", config.Question)
			assert.Equal(t, FieldTypeMultipleChoice, config.Type)
			assert.Len(t, config.Options, 2)
			
			// Return a mock response
			return Output{
				Response: "yes",
				UserID:   "test-user-123",
				Metadata: map[string]interface{}{
					"timestamp": time.Now().Unix(),
				},
			}, nil
		},
	)
	
	// Define a recipe configuration with an input step
	recipeConfig := map[string]interface{}{
		"id":     "deployment-recipe",
		"box_id": "deployment-box",
		"steps": []interface{}{
			map[string]interface{}{
				"name": "pre_check",
				"type": "action",
				"config": map[string]interface{}{
					"action": "validate_environment",
				},
			},
			map[string]interface{}{
				"name": "approval",
				"type": "input",
				"config": map[string]interface{}{
					"question": "Do you want to proceed with deployment?",
					"type":     "multiple_choice",
					"options": []interface{}{
						map[string]interface{}{
							"value": "yes",
							"label": "Yes, deploy",
						},
						map[string]interface{}{
							"value": "no",
							"label": "No, cancel",
						},
					},
					"timeout": 300,
				},
			},
			map[string]interface{}{
				"name": "deploy",
				"type": "action",
				"config": map[string]interface{}{
					"action": "execute_deployment",
				},
			},
		},
	}
	
	// Execute the recipe workflow
	env.ExecuteWorkflow(SimulatedRecipeWorkflow, recipeConfig)
	
	// Verify workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	// Get and verify the result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	assert.Equal(t, "completed", result["status"])
	assert.Equal(t, "deployment-recipe", result["recipe_id"])
	
	// Verify the outputs contain the input activity response
	outputs := result["outputs"].(map[string]interface{})
	approvalOutput := outputs["approval"].(map[string]interface{})
	assert.Equal(t, "yes", approvalOutput["response"])
	assert.Equal(t, "test-user-123", approvalOutput["user_id"])
}

// TestRecipeWithMultiFieldInput tests a recipe with a multi-field input form
func TestRecipeWithMultiFieldInput(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	env.RegisterWorkflow(SimulatedRecipeWorkflow)
	
	// Mock the input activity for multi-field form
	env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, config Config, input Input) (Output, error) {
			// Verify multi-field configuration
			assert.Equal(t, "Deployment Configuration", config.Title)
			assert.Len(t, config.Fields, 3)
			
			return Output{
				Fields: map[string]interface{}{
					"environment": "staging",
					"strategy":    "blue_green",
					"rollback":    true,
				},
				UserID: "config-admin",
				Metadata: map[string]interface{}{
					"submitted_at": time.Now().Unix(),
				},
			}, nil
		},
	)
	
	recipeConfig := map[string]interface{}{
		"id":     "config-recipe",
		"box_id": "config-box",
		"steps": []interface{}{
			map[string]interface{}{
				"name": "get_config",
				"type": "input",
				"config": map[string]interface{}{
					"title": "Deployment Configuration",
					"fields": []interface{}{
						map[string]interface{}{
							"id":       "environment",
							"type":     "dropdown",
							"question": "Select environment",
							"required": true,
							"options": []interface{}{
								map[string]interface{}{"value": "dev", "label": "Development"},
								map[string]interface{}{"value": "staging", "label": "Staging"},
								map[string]interface{}{"value": "prod", "label": "Production"},
							},
						},
						map[string]interface{}{
							"id":       "strategy",
							"type":     "multiple_choice",
							"question": "Deployment strategy",
							"required": true,
							"options": []interface{}{
								map[string]interface{}{"value": "rolling", "label": "Rolling"},
								map[string]interface{}{"value": "blue_green", "label": "Blue-Green"},
								map[string]interface{}{"value": "canary", "label": "Canary"},
							},
						},
						map[string]interface{}{
							"id":       "rollback",
							"type":     "multiple_choice",
							"question": "Enable auto-rollback?",
							"required": false,
							"options": []interface{}{
								map[string]interface{}{"value": true, "label": "Yes"},
								map[string]interface{}{"value": false, "label": "No"},
							},
						},
					},
					"timeout": 600,
				},
			},
			map[string]interface{}{
				"name": "apply_config",
				"type": "action",
				"config": map[string]interface{}{
					"action": "apply_deployment_config",
				},
			},
		},
	}
	
	env.ExecuteWorkflow(SimulatedRecipeWorkflow, recipeConfig)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	assert.Equal(t, "completed", result["status"])
	
	// Verify multi-field responses
	outputs := result["outputs"].(map[string]interface{})
	configOutput := outputs["get_config"].(map[string]interface{})
	fields := configOutput["fields"].(map[string]interface{})
	
	assert.Equal(t, "staging", fields["environment"])
	assert.Equal(t, "blue_green", fields["strategy"])
	assert.Equal(t, true, fields["rollback"])
	assert.Equal(t, "config-admin", configOutput["user_id"])
}

// TestRecipeWithConditionalInput tests a recipe that uses input responses to make decisions
func TestRecipeWithConditionalInput(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Extended recipe workflow that makes decisions based on input
	conditionalRecipeWorkflow := func(ctx workflow.Context, recipeConfig map[string]interface{}) (map[string]interface{}, error) {
		// First, run the standard recipe workflow
		result, err := SimulatedRecipeWorkflow(ctx, recipeConfig)
		if err != nil {
			return result, err
		}
		
		// Then make decisions based on the input
		outputs := result["outputs"].(map[string]interface{})
		if approvalOutput, ok := outputs["approval"]; ok {
			approval := approvalOutput.(map[string]interface{})
			if approval["response"] == "emergency" {
				// Add an emergency deployment step
				result["emergency_mode"] = true
				outputs["emergency_deploy"] = map[string]interface{}{
					"status": "executed",
					"type":   "emergency",
				}
			}
		}
		
		return result, nil
	}
	
	env.RegisterWorkflow(conditionalRecipeWorkflow)
	
	// Mock input activity to return emergency response
	env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything, mock.Anything).Return(
		Output{
			Response: "emergency",
			UserID:   "emergency-approver",
		}, nil,
	)
	
	recipeConfig := map[string]interface{}{
		"id":     "conditional-recipe",
		"box_id": "emergency-box",
		"steps": []interface{}{
			map[string]interface{}{
				"name": "approval",
				"type": "input",
				"config": map[string]interface{}{
					"question": "Select deployment type",
					"type":     "multiple_choice",
					"options": []interface{}{
						map[string]interface{}{"value": "normal", "label": "Normal"},
						map[string]interface{}{"value": "emergency", "label": "Emergency"},
						map[string]interface{}{"value": "cancel", "label": "Cancel"},
					},
				},
			},
		},
	}
	
	env.ExecuteWorkflow(conditionalRecipeWorkflow, recipeConfig)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify emergency mode was triggered
	assert.Equal(t, true, result["emergency_mode"])
	
	outputs := result["outputs"].(map[string]interface{})
	assert.Contains(t, outputs, "emergency_deploy")
	emergencyDeploy := outputs["emergency_deploy"].(map[string]interface{})
	assert.Equal(t, "emergency", emergencyDeploy["type"])
}

// TestRecipeWithInputTimeout tests timeout handling in recipes
func TestRecipeWithInputTimeout(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	env.RegisterWorkflow(SimulatedRecipeWorkflow)
	
	// Mock input activity to simulate timeout by returning default value
	// The activity should immediately return the default value without error
	// to avoid retries
	env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, config Config, input Input) (Output, error) {
			// When there's a default on timeout, return it immediately as success
			// This simulates the activity handling the timeout gracefully
			if config.DefaultOnTimeout != nil {
				return Output{
					Response: config.DefaultOnTimeout,
					UserID:   "system-timeout",
					Metadata: map[string]interface{}{
						"reason": "timeout",
					},
				}, nil
			}
			// If no default, return a non-retryable error
			return Output{}, temporal.NewNonRetryableApplicationError("input timeout", "TIMEOUT", nil)
		},
	)
	
	recipeConfig := map[string]interface{}{
		"id":     "timeout-recipe",
		"box_id": "timeout-box",
		"steps": []interface{}{
			map[string]interface{}{
				"name": "quick_decision",
				"type": "input",
				"config": map[string]interface{}{
					"question": "Quick! Yes or no?",
					"type":     "multiple_choice",
					"options": []interface{}{
						map[string]interface{}{"value": "yes", "label": "Yes"},
						map[string]interface{}{"value": "no", "label": "No"},
					},
					"timeout":           5, // Very short timeout
					"default_on_timeout": "no",
				},
			},
		},
	}
	
	env.ExecuteWorkflow(SimulatedRecipeWorkflow, recipeConfig)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify the default value was used
	outputs := result["outputs"].(map[string]interface{})
	quickDecision := outputs["quick_decision"].(map[string]interface{})
	assert.Equal(t, "no", quickDecision["response"])
	assert.Equal(t, "system-timeout", quickDecision["user_id"])
}

// TestRecipeWithMultipleInputSteps tests a recipe with multiple input activities
func TestRecipeWithMultipleInputSteps(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	env.RegisterWorkflow(SimulatedRecipeWorkflow)
	
	// Track which input activities were called
	callOrder := []string{}
	
	env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, config Config, input Input) (Output, error) {
			callOrder = append(callOrder, input.ActivityID)
			
			// Return different responses based on the activity
			switch input.ActivityID {
			case "initial_approval":
				return Output{Response: "approve", UserID: "approver-1"}, nil
			case "technical_review":
				return Output{
					Fields: map[string]interface{}{
						"risk_level": "medium",
						"notes":      "Needs monitoring",
					},
					UserID: "tech-reviewer",
				}, nil
			case "final_confirmation":
				return Output{Response: "confirmed", UserID: "final-approver"}, nil
			default:
				return Output{}, nil
			}
		},
	)
	
	recipeConfig := map[string]interface{}{
		"id":     "multi-input-recipe",
		"box_id": "multi-box",
		"steps": []interface{}{
			map[string]interface{}{
				"name": "initial_approval",
				"type": "input",
				"config": map[string]interface{}{
					"question": "Initial approval?",
					"type":     "multiple_choice",
					"options": []interface{}{
						map[string]interface{}{"value": "approve", "label": "Approve"},
						map[string]interface{}{"value": "reject", "label": "Reject"},
					},
				},
			},
			map[string]interface{}{
				"name": "technical_review",
				"type": "input",
				"config": map[string]interface{}{
					"title": "Technical Review",
					"fields": []interface{}{
						map[string]interface{}{
							"id":       "risk_level",
							"type":     "dropdown",
							"question": "Risk assessment",
							"required": true,
							"options": []interface{}{
								map[string]interface{}{"value": "low", "label": "Low"},
								map[string]interface{}{"value": "medium", "label": "Medium"},
								map[string]interface{}{"value": "high", "label": "High"},
							},
						},
						map[string]interface{}{
							"id":       "notes",
							"type":     "paragraph_text",
							"question": "Additional notes",
							"required": false,
						},
					},
				},
			},
			map[string]interface{}{
				"name": "final_confirmation",
				"type": "input",
				"config": map[string]interface{}{
					"question": "Final confirmation?",
					"type":     "multiple_choice",
					"options": []interface{}{
						map[string]interface{}{"value": "confirmed", "label": "Confirm"},
						map[string]interface{}{"value": "cancel", "label": "Cancel"},
					},
				},
			},
		},
	}
	
	env.ExecuteWorkflow(SimulatedRecipeWorkflow, recipeConfig)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify all input activities were called in order
	assert.Equal(t, []string{"initial_approval", "technical_review", "final_confirmation"}, callOrder)
	
	// Verify all outputs are present
	outputs := result["outputs"].(map[string]interface{})
	assert.Contains(t, outputs, "initial_approval")
	assert.Contains(t, outputs, "technical_review")
	assert.Contains(t, outputs, "final_confirmation")
	
	// Verify specific responses
	initial := outputs["initial_approval"].(map[string]interface{})
	assert.Equal(t, "approve", initial["response"])
	
	technical := outputs["technical_review"].(map[string]interface{})
	fields := technical["fields"].(map[string]interface{})
	assert.Equal(t, "medium", fields["risk_level"])
	assert.Equal(t, "Needs monitoring", fields["notes"])
	
	final := outputs["final_confirmation"].(map[string]interface{})
	assert.Equal(t, "confirmed", final["response"])
}