package input

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// DeploymentRecipeWorkflow simulates a real deployment recipe that requires user approval
func DeploymentRecipeWorkflow(ctx workflow.Context, params map[string]interface{}) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	
	// Extract parameters
	projectName := params["project_name"].(string)
	environment := params["environment"].(string)
	version := params["version"].(string)
	
	logger.Info("Starting deployment recipe", 
		"project", projectName,
		"environment", environment,
		"version", version)
	
	result := map[string]interface{}{
		"project_name": projectName,
		"environment":  environment,
		"version":      version,
		"steps":        []interface{}{},
	}
	
	// Step 1: Pre-deployment checks
	workflow.Sleep(ctx, 100*time.Millisecond) // Simulate work
	steps := result["steps"].([]interface{})
	steps = append(steps, "pre_deployment_checks_completed")
	result["steps"] = steps
	
	// Step 2: Request user approval for deployment
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	
	// Create approval form with deployment details
	var approvalOutput Output
	approvalConfig := Config{
		Title: "Deployment Approval Required",
		Fields: []FormField{
			{
				ID:       "approval_decision",
				Type:     FieldTypeMultipleChoice,
				Question: "Do you approve this deployment?",
				Required: true,
				Options: []Option{
					{Value: "approve", Label: "Approve Deployment"},
					{Value: "reject", Label: "Reject Deployment"},
					{Value: "defer", Label: "Defer Decision"},
				},
			},
			{
				ID:          "approval_notes",
				Type:        FieldTypeParagraphText,
				Question:    "Additional notes or conditions",
				Required:    false,
				Placeholder: "Enter any conditions or notes for this deployment",
			},
			{
				ID:       "risk_assessment",
				Type:     FieldTypeLinearScale,
				Question: "Risk level assessment",
				Required: true,
				Scale: &LinearScale{
					Min:      1,
					Max:      5,
					MinLabel: "Low Risk",
					MaxLabel: "High Risk",
				},
			},
		},
		Context: FormContext{
			Artifacts: []Artifact{
				{Path: "deployment-plan.yaml"},
				{Path: "changelog.md"},
			},
		},
		Timeout:          300, // 5 minutes
		DefaultOnTimeout: map[string]interface{}{"approval_decision": "defer"},
	}
	
	approvalInput := Input{
		BoxID:      projectName,
		ActivityID: "deployment-approval",
		Context: map[string]interface{}{
			"project":     projectName,
			"environment": environment,
			"version":     version,
		},
	}
	
        approvalInput.Config = approvalConfig
        err := workflow.ExecuteActivity(ctx, InputActivityExecute, approvalInput).Get(ctx, &approvalOutput)
	if err != nil {
		logger.Error("Failed to get approval", "error", err)
		result["error"] = err.Error()
		result["status"] = "failed"
		return result, err
	}
	
	steps = result["steps"].([]interface{})
	steps = append(steps, "user_approval_received")
	result["steps"] = steps
	
	// Process approval decision
	decision := ""
	if approvalOutput.Fields != nil {
		decision = approvalOutput.Fields["approval_decision"].(string)
	} else if approvalOutput.Response != nil {
		decision = approvalOutput.Response.(string)
	}
	
	result["approval_decision"] = decision
	result["approved_by"] = approvalOutput.UserID
	
	if decision != "approve" {
		result["status"] = "deployment_rejected"
		if decision == "defer" {
			result["status"] = "deployment_deferred"
		}
		return result, nil
	}
	
	// Step 3: Perform deployment
	workflow.Sleep(ctx, 200*time.Millisecond) // Simulate deployment
	steps = result["steps"].([]interface{})
	steps = append(steps, "deployment_executed")
	result["steps"] = steps
	
	// Step 4: Post-deployment verification with user confirmation
	var verificationOutput Output
	verificationConfig := Config{
		Question: "Has the deployment been verified successfully?",
		Type:     FieldTypeMultipleChoice,
		Options: []Option{
			{Value: "verified", Label: "Yes, all checks passed"},
			{Value: "issues", Label: "Issues detected, needs rollback"},
			{Value: "partial", Label: "Partially successful, manual intervention needed"},
		},
		Timeout:          120, // 2 minutes
		DefaultOnTimeout: "partial",
	}
	
	verificationInput := Input{
		BoxID:      projectName,
		ActivityID: "deployment-verification",
		Context: map[string]interface{}{
			"deployment_id": workflow.GetInfo(ctx).WorkflowExecution.ID,
		},
	}
	
        verificationInput.Config = verificationConfig
        err = workflow.ExecuteActivity(ctx, InputActivityExecute, verificationInput).Get(ctx, &verificationOutput)
	if err != nil {
		logger.Warn("Verification timeout or error", "error", err)
		result["verification_status"] = "timeout"
	} else {
		result["verification_status"] = verificationOutput.Response
		result["verified_by"] = verificationOutput.UserID
	}
	
	steps = result["steps"].([]interface{})
	steps = append(steps, "post_deployment_verification_completed")
	result["steps"] = steps
	
	// Final status
	if result["verification_status"] == "verified" {
		result["status"] = "deployment_successful"
	} else if result["verification_status"] == "issues" {
		result["status"] = "deployment_rolled_back"
		// Simulate rollback
		workflow.Sleep(ctx, 100*time.Millisecond)
		steps = result["steps"].([]interface{})
		steps = append(steps, "rollback_completed")
		result["steps"] = steps
	} else {
		result["status"] = "deployment_needs_attention"
	}
	
	return result, nil
}

// TestDeploymentRecipeWithUserInput tests a complete deployment recipe flow
func TestDeploymentRecipeWithUserInput(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Register the deployment recipe workflow
	env.RegisterWorkflow(DeploymentRecipeWorkflow)
	
	// Track activity executions
	activityCalls := []string{}
	
	// Mock the InputActivity to simulate user responses
    env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything).Return(
        func(ctx context.Context, input Input) (Output, error) {
            activityCalls = append(activityCalls, input.ActivityID)
			
			// Return different responses based on the activity
			switch input.ActivityID {
			case "deployment-approval":
				// User approves deployment with notes
				return Output{
					Fields: map[string]interface{}{
						"approval_decision": "approve",
						"approval_notes":    "Approved for off-peak deployment window",
						"risk_assessment":   2,
					},
					UserID: "approver-john-doe",
					Metadata: map[string]interface{}{
						"approval_time": time.Now().Unix(),
					},
				}, nil
				
			case "deployment-verification":
				// Deployment verified successfully
				return Output{
					Response: "verified",
					UserID:   "verifier-jane-smith",
					Metadata: map[string]interface{}{
						"verification_time": time.Now().Unix(),
					},
				}, nil
				
			default:
				return Output{}, nil
			}
		},
	)
	
	// Execute the deployment recipe
	params := map[string]interface{}{
		"project_name": "web-app",
		"environment":  "production",
		"version":      "v2.1.0",
	}
	
	env.ExecuteWorkflow(DeploymentRecipeWorkflow, params)
	
	// Verify workflow completed successfully
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	// Get and verify the result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify the deployment was successful
	assert.Equal(t, "deployment_successful", result["status"])
	assert.Equal(t, "web-app", result["project_name"])
	assert.Equal(t, "production", result["environment"])
	assert.Equal(t, "v2.1.0", result["version"])
	
	// Verify approval details
	assert.Equal(t, "approve", result["approval_decision"])
	assert.Equal(t, "approver-john-doe", result["approved_by"])
	
	// Verify verification details
	assert.Equal(t, "verified", result["verification_status"])
	assert.Equal(t, "verifier-jane-smith", result["verified_by"])
	
	// Verify all steps were executed
	steps := result["steps"].([]interface{})
	stepStrings := make([]string, len(steps))
	for i, step := range steps {
		stepStrings[i] = step.(string)
	}
	assert.Contains(t, stepStrings, "pre_deployment_checks_completed")
	assert.Contains(t, stepStrings, "user_approval_received")
	assert.Contains(t, stepStrings, "deployment_executed")
	assert.Contains(t, stepStrings, "post_deployment_verification_completed")
	
	// Verify activities were called
	assert.Contains(t, activityCalls, "deployment-approval")
	assert.Contains(t, activityCalls, "deployment-verification")
}

// TestDeploymentRecipeRejection tests deployment rejection scenario
func TestDeploymentRecipeRejection(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	env.RegisterWorkflow(DeploymentRecipeWorkflow)
	
	// Mock activity to reject deployment
    env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything).Return(
        func(ctx context.Context, input Input) (Output, error) {
            if input.ActivityID == "deployment-approval" {
                return Output{
                    Fields: map[string]interface{}{
						"approval_decision": "reject",
						"approval_notes":    "Changes need more testing",
						"risk_assessment":   4,
					},
					UserID: "reviewer-alice",
				}, nil
			}
			return Output{}, nil
		},
	)
	
	params := map[string]interface{}{
		"project_name": "api-service",
		"environment":  "production",
		"version":      "v3.0.0",
	}
	
	env.ExecuteWorkflow(DeploymentRecipeWorkflow, params)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify deployment was rejected
	assert.Equal(t, "deployment_rejected", result["status"])
	assert.Equal(t, "reject", result["approval_decision"])
	assert.Equal(t, "reviewer-alice", result["approved_by"])
	
	// Verify deployment steps were not executed
	steps := result["steps"].([]interface{})
	stepStrings := make([]string, len(steps))
	for i, step := range steps {
		stepStrings[i] = step.(string)
	}
	assert.NotContains(t, stepStrings, "deployment_executed")
	assert.NotContains(t, steps, "post_deployment_verification_completed")
}

// TestDeploymentRecipeTimeout tests timeout scenarios
func TestDeploymentRecipeTimeout(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	env.RegisterWorkflow(DeploymentRecipeWorkflow)
	
	// Mock activity to simulate timeout on approval
	callCount := 0
    env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything).Return(
        func(ctx context.Context, input Input) (Output, error) {
            callCount++
			if input.ActivityID == "deployment-approval" {
				// Simulate timeout - return default value
				return Output{
					Fields: map[string]interface{}{
						"approval_decision": "defer", // Default on timeout
					},
					UserID: "system-timeout",
				}, nil
			}
			return Output{}, nil
		},
	)
	
	params := map[string]interface{}{
		"project_name": "backend-service",
		"environment":  "staging",
		"version":      "v1.5.0",
	}
	
	env.ExecuteWorkflow(DeploymentRecipeWorkflow, params)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify deployment was deferred due to timeout
	assert.Equal(t, "deployment_deferred", result["status"])
	assert.Equal(t, "defer", result["approval_decision"])
	assert.Equal(t, "system-timeout", result["approved_by"])
	
	// Verify only approval activity was called
	assert.Equal(t, 1, callCount)
}

// TestDeploymentRecipeWithRollback tests deployment with rollback scenario
func TestDeploymentRecipeWithRollback(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	env.RegisterWorkflow(DeploymentRecipeWorkflow)
	
	// Mock activities for approval and failed verification
env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything).Return(
    func(ctx context.Context, input Input) (Output, error) {
        switch input.ActivityID {
			case "deployment-approval":
				return Output{
					Fields: map[string]interface{}{
						"approval_decision": "approve",
						"risk_assessment":   3,
					},
					UserID: "approver-bob",
				}, nil
				
			case "deployment-verification":
				// Verification detects issues
				return Output{
					Response: "issues",
					UserID:   "monitor-system",
				}, nil
			}
			return Output{}, nil
		},
	)
	
	params := map[string]interface{}{
		"project_name": "payment-service",
		"environment":  "production",
		"version":      "v4.2.1",
	}
	
	env.ExecuteWorkflow(DeploymentRecipeWorkflow, params)
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify deployment was rolled back
	assert.Equal(t, "deployment_rolled_back", result["status"])
	assert.Equal(t, "issues", result["verification_status"])
	
	// Verify rollback was executed
	steps := result["steps"].([]interface{})
	stepStrings := make([]string, len(steps))
	for i, step := range steps {
		stepStrings[i] = step.(string)
	}
	assert.Contains(t, stepStrings, "deployment_executed")
	assert.Contains(t, steps, "rollback_completed")
}

// TestMultiStageApprovalWorkflow tests a workflow requiring multiple approvals
func TestMultiStageApprovalWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	
	// Define a workflow with multiple approval stages
	env.RegisterWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		approvals := []string{}
		
		activityOptions := workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
		}
		ctx = workflow.WithActivityOptions(ctx, activityOptions)
		
		// Stage 1: Technical approval
		var techApproval Output
		techConfig := Config{
			Question: "Technical review complete?",
			Type:     FieldTypeMultipleChoice,
			Options:  []Option{{Value: "yes"}, {Value: "no"}},
			Timeout:  60,
		}
		techInput := Input{
			BoxID:      "review-cell",
			ActivityID: "tech-approval",
		}
		
        techInput.Config = techConfig
        err := workflow.ExecuteActivity(ctx, InputActivityExecute, techInput).Get(ctx, &techApproval)
		if err != nil {
			return nil, err
		}
		
		if techApproval.Response == "yes" {
			approvals = append(approvals, "technical")
		}
		
		// Stage 2: Security approval
		var secApproval Output
		secConfig := Config{
			Question: "Security review complete?",
			Type:     FieldTypeMultipleChoice,
			Options:  []Option{{Value: "yes"}, {Value: "no"}},
			Timeout:  60,
		}
		secInput := Input{
			BoxID:      "review-cell",
			ActivityID: "security-approval",
		}
		
        secInput.Config = secConfig
        err = workflow.ExecuteActivity(ctx, InputActivityExecute, secInput).Get(ctx, &secApproval)
		if err != nil {
			return nil, err
		}
		
		if secApproval.Response == "yes" {
			approvals = append(approvals, "security")
		}
		
		// Stage 3: Management approval (only if both previous approvals granted)
		if len(approvals) == 2 {
			var mgmtApproval Output
			mgmtConfig := Config{
				Question: "Management approval?",
				Type:     FieldTypeMultipleChoice,
				Options:  []Option{{Value: "yes"}, {Value: "no"}},
				Timeout:  60,
			}
			mgmtInput := Input{
				BoxID:      "review-cell",
				ActivityID: "management-approval",
			}
			
                mgmtInput.Config = mgmtConfig
                err = workflow.ExecuteActivity(ctx, InputActivityExecute, mgmtInput).Get(ctx, &mgmtApproval)
			if err != nil {
				return nil, err
			}
			
			if mgmtApproval.Response == "yes" {
				approvals = append(approvals, "management")
			}
		}
		
		return map[string]interface{}{
			"approvals": approvals,
			"approved":  len(approvals) == 3,
		}, nil
	})
	
	// Mock all approvals as successful
env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything).Return(
    func(ctx context.Context, input Input) (Output, error) {
        return Output{
            Response: "yes",
            UserID:   "approver-" + input.ActivityID,
        }, nil
    },
)
	
	// Execute the workflow using the anonymous function directly
	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		approvals := []string{}
		
		activityOptions := workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
		}
		ctx = workflow.WithActivityOptions(ctx, activityOptions)
		
		// Stage 1: Technical approval
		var techApproval Output
		techConfig := Config{
			Question: "Technical review complete?",
			Type:     FieldTypeMultipleChoice,
			Options:  []Option{{Value: "yes"}, {Value: "no"}},
			Timeout:  60,
		}
		techInput := Input{
			BoxID:      "review-cell",
			ActivityID: "tech-approval",
		}
		
techInput.Config = techConfig
err := workflow.ExecuteActivity(ctx, InputActivityExecute, techInput).Get(ctx, &techApproval)
		if err != nil {
			return nil, err
		}
		
		if techApproval.Response == "yes" {
			approvals = append(approvals, "technical")
		}
		
		// Stage 2: Security approval
		var secApproval Output
		secConfig := Config{
			Question: "Security review complete?",
			Type:     FieldTypeMultipleChoice,
			Options:  []Option{{Value: "yes"}, {Value: "no"}},
			Timeout:  60,
		}
		secInput := Input{
			BoxID:      "review-cell",
			ActivityID: "security-approval",
		}
		
secInput.Config = secConfig
err = workflow.ExecuteActivity(ctx, InputActivityExecute, secInput).Get(ctx, &secApproval)
		if err != nil {
			return nil, err
		}
		
		if secApproval.Response == "yes" {
			approvals = append(approvals, "security")
		}
		
		// Stage 3: Management approval (only if both previous approvals granted)
		if len(approvals) == 2 {
			var mgmtApproval Output
			mgmtConfig := Config{
				Question: "Management approval?",
				Type:     FieldTypeMultipleChoice,
				Options:  []Option{{Value: "yes"}, {Value: "no"}},
				Timeout:  60,
			}
			mgmtInput := Input{
				BoxID:      "review-cell",
				ActivityID: "management-approval",
			}
			
mgmtInput.Config = mgmtConfig
err = workflow.ExecuteActivity(ctx, InputActivityExecute, mgmtInput).Get(ctx, &mgmtApproval)
			if err != nil {
				return nil, err
			}
			
			if mgmtApproval.Response == "yes" {
				approvals = append(approvals, "management")
			}
		}
		
		return map[string]interface{}{
			"approvals": approvals,
			"approved":  len(approvals) == 3,
		}, nil
	})
	
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))
	
	// Verify all approvals were obtained
	approvalsInterface := result["approvals"].([]interface{})
	approvals := make([]string, len(approvalsInterface))
	for i, v := range approvalsInterface {
		approvals[i] = v.(string)
	}
	assert.Contains(t, approvals, "technical")
	assert.Contains(t, approvals, "security")
	assert.Contains(t, approvals, "management")
	assert.True(t, result["approved"].(bool))
}
