package input

import (
    "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/workflow"
)

// InputCollectionWorkflow handles the actual input collection from users
func InputCollectionWorkflow(ctx workflow.Context, params InputWorkflowParams) (InputWorkflowResult, error) {
	// workflowID can be used for logging or other purposes if needed
	_ = workflow.GetInfo(ctx).WorkflowExecution.ID
	
    // Record initial pending status and form details (typed)
    err := workflow.UpsertTypedSearchAttributes(ctx,
        temporal.NewSearchAttributeKeyKeyword("InputStatus").ValueSet("pending"),
        temporal.NewSearchAttributeKeyString("InputFormTitle").ValueSet(params.Form.Title),
        temporal.NewSearchAttributeKeyString("InputBoxID").ValueSet(params.BoxID),
        temporal.NewSearchAttributeKeyTime("InputCreatedAt").ValueSet(workflow.Now(ctx)),
        temporal.NewSearchAttributeKeyTime("InputExpiresAt").ValueSet(workflow.Now(ctx).Add(params.Timeout)),
    )
	if err != nil {
		// Log error but continue - search attributes are not critical
		workflow.GetLogger(ctx).Error("Failed to update search attributes", "error", err)
	}
	
	// Create a channel to receive user response signal
	responseChan := workflow.GetSignalChannel(ctx, "user-response")
	
	// Create a cancelable context for timeout handling
	timeoutCtx, cancel := workflow.WithCancel(ctx)
	
	// Setup timeout goroutine
	workflow.Go(timeoutCtx, func(ctx workflow.Context) {
		workflow.Sleep(ctx, params.Timeout)
		cancel()
	})
	
	// Wait for user response or timeout
	var response UserResponseSignal
	responseChan.Receive(timeoutCtx, &response)
	
	// Check if we timed out
	if timeoutCtx.Err() != nil {
        // Update status to timed out (typed)
        workflow.UpsertTypedSearchAttributes(ctx,
            temporal.NewSearchAttributeKeyKeyword("InputStatus").ValueSet("timeout"),
        )
		return InputWorkflowResult{}, temporal.NewApplicationError("input timeout", "TIMEOUT")
	}
	
	// Update status to completed
    err = workflow.UpsertTypedSearchAttributes(ctx,
        temporal.NewSearchAttributeKeyKeyword("InputStatus").ValueSet("completed"),
        temporal.NewSearchAttributeKeyString("InputRespondedBy").ValueSet(response.UserID),
        temporal.NewSearchAttributeKeyTime("InputRespondedAt").ValueSet(response.RespondedAt),
    )
	if err != nil {
		workflow.GetLogger(ctx).Error("Failed to update search attributes", "error", err)
	}
	
	// Return the result
	return InputWorkflowResult{
		FormResponse: response.Fields,
		UserID:       response.UserID,
		Metadata:     response.Metadata,
	}, nil
}

// CancelInputWorkflow handles cancellation of a pending input request
func CancelInputWorkflow(ctx workflow.Context, workflowID string, reason string) error {
	// Send cancellation signal to the workflow
	err := workflow.SignalExternalWorkflow(ctx, workflowID, "", "cancel-input", reason).Get(ctx, nil)
	if err != nil {
		return err
	}
	
	return nil
}
