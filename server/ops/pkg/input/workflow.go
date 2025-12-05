package input

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// InputCollectionWorkflow handles the actual input collection from users
func InputCollectionWorkflow() (InputWorkflowResult, error) {
	if params.ID == "" {
		return InputWorkflowResult{}, temporal.NewApplicationError("input id missing", "BAD_REQUEST")
	}

	// Record initial pending status and form details (typed)
	waitTimeout := params.Timeout
	if waitTimeout == 0 {
		waitTimeout = 5 * time.Minute
	}
	createdAt := workflow.Now(ctx)
	expiresAt := createdAt.Add(waitTimeout)

	upsertInputSearchAttributes(ctx, map[string]interface{}{
		"InputKey":        params.ID,
		"InputStatus":     "pending",
		"InputFormTitle":  params.Form.Title,
		"InputBoxID":      params.BoxID,
		"InputActivityID": params.ActivityID,
		"InputCreatedAt":  createdAt,
		"InputExpiresAt":  expiresAt,
	})

	// Create a channel to receive user response signal
	responseChan := workflow.GetSignalChannel(ctx, userResponseSignalName(params.ID))

	// Create a cancelable context for timeout handling
	timeoutCtx, cancel := workflow.WithCancel(ctx)

	// Setup timeout goroutine
	workflow.Go(timeoutCtx, func(ctx workflow.Context) {
		workflow.Sleep(ctx, waitTimeout)
		cancel()
	})

	// Wait for user response or timeout
	var response UserResponseSignal
	responseChan.Receive(timeoutCtx, &response)

	// Check if we timed out
	if timeoutCtx.Err() != nil {
		upsertInputSearchAttributes(ctx, map[string]interface{}{
			"InputStatus": "timeout",
		})
		return InputWorkflowResult{}, temporal.NewApplicationError("input timeout", "TIMEOUT")
	}

	// Update status to completed
	upsertInputSearchAttributes(ctx, map[string]interface{}{
		"InputStatus":      "completed",
		"InputRespondedBy": response.UserID,
		"InputRespondedAt": response.RespondedAt,
	})

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
