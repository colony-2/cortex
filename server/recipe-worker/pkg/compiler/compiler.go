package compiler

import (
	"errors"
	"fmt"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func ExecuteRecipe(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, r recipe.Recipe, inputs map[string]interface{}) (map[string]interface{}, error) {
	switch t := r.RecipeImpl.(type) {
	case *recipe.RecipeState:
		outputs, err := executeStateMachine(ctx, activityRegistry, t.States, inputs)
		return processNodeOutputs(outputs, t.Outputs, err)
	case *recipe.RecipeOp:
		return executeOp(ctx, activityRegistry, t.NodeMetadata, t.Op, t.Inputs, inputs)

	case *recipe.RecipeSequence:
		outputs, err := executeSequence(ctx, activityRegistry, t.NodeMetadata, t.Sequence, inputs)
		return processNodeOutputs(outputs, t.Outputs, err)
	default:
		return nil, fmt.Errorf("unsupported recipe type: %T", t)
	}
}

// ExecuteWorkflow implements the WorkflowExecutor interface for unified recipes
func executeNode(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, n *recipe.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	switch t := n.NodeImpl.(type) {
	case *recipe.NodeState:
		return executeStateMachine(ctx, activityRegistry, t.States, inputs)

	case *recipe.NodeOp:
		return executeOp(ctx, activityRegistry, t.NodeMetadata, t.Op, t.Inputs, inputs)

	case *recipe.NodeSequence:
		return executeSequence(ctx, activityRegistry, t.NodeMetadata, t.Sequence, inputs)

	default:
		return nil, fmt.Errorf("unsupported recipe type: %T", t)
	}

}

// processNodeOutputs processes outputs from node execution
func processNodeOutputs(outputs map[string]interface{}, outputTemplates map[string]interface{}, err error) (map[string]interface{}, error) {

	if err != nil {
		return outputs, err
	}

	if outputTemplates == nil {
		return outputs, nil
	}

	// TODO: Implement output template resolution for new node format
	// For now, return outputs as-is
	return outputs, nil
}

// WorkflowState maintains the runtime state of a workflow
type WorkflowState struct {
	Inputs  map[string]interface{}
	Steps   map[string]StepResult
	Outputs map[string]interface{}
}

// StepResult stores the result of a workflow step
type StepResult struct {
	Outputs map[string]interface{}
}

// executeOperation executes a single operation node
func executeOp(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, metadata recipe.NodeMetadata, op string, nodeInputs map[string]interface{}, workflowInputs map[string]interface{}) (map[string]interface{}, error) {
	// Create workflow state for template resolution
	state := &WorkflowState{
		Inputs:  workflowInputs,
		Outputs: make(map[string]interface{}),
	}

	// Resolve templates in node inputs
	resolver := NewTemplateResolver(state)
	resolvedNodeInputs := make(map[string]interface{})
	for k, v := range nodeInputs {
		resolved, err := resolver.ResolveValue(v)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve template in input %s: %w", k, err)
		}
		resolvedNodeInputs[k] = resolved
	}

	// Merge workflow inputs with resolved node inputs
	inputs := make(map[string]interface{})
	for k, v := range workflowInputs {
		inputs[k] = v
	}
	for k, v := range resolvedNodeInputs {
		inputs[k] = v
	}

	opImpl, exists := activityRegistry.Get(op)
	if !exists {
		return nil, fmt.Errorf("op type %q not found", op)
	}

	if !opImpl.Activity.ExecuteAsActivity() {
		return executeCompositeInEnvelope(ctx, ToTemporalRetryPolicy(metadata.Retry), time.Duration(metadata.Timeout), func(inner workflow.Context) (map[string]interface{}, error) {
			return opImpl.Activity.ExecuteInline(ctx, inputs)
		})
	}

	// Configure activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Duration(metadata.Timeout), // Default timeout
		RetryPolicy:         ToTemporalRetryPolicy(metadata.Retry),
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Execute the operation
	var outputs map[string]interface{}

	err := workflow.ExecuteActivity(ctx, op, inputs).Get(ctx, &outputs)
	if err != nil {
		return nil, err
	}

	return outputs, nil
}

func innerSequence(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, metadata recipe.NodeMetadata, sequence []recipe.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	outputs := make(map[string]interface{})
	for i, node := range sequence {
		nodeOutputs, err := executeNode(ctx, activityRegistry, &node, inputs)
		if err != nil {
			return nil, fmt.Errorf("sequence node %d failed: %w", i, err)
		}

		// Store outputs with node ID if specified
		if metadata.ID != "" {
			outputs[metadata.ID] = nodeOutputs
		}

		// Pass outputs as inputs to next node
		for k, v := range outputs {
			inputs[k] = v
		}
	}

	return outputs, nil
}

// executeSequenceNodes executes nodes in sequence
func executeSequence(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, metadata recipe.NodeMetadata, sequence []recipe.Node, nodeInputs map[string]interface{}) (map[string]interface{}, error) {
	return executeCompositeInEnvelope(ctx, ToTemporalRetryPolicy(metadata.Retry), time.Duration(metadata.Timeout), func(inner workflow.Context) (map[string]interface{}, error) {
		return innerSequence(inner, activityRegistry, metadata, sequence, nodeInputs)
	})

}

// executeCompositeInEnvelope executes a composite nodes in a retry/timeout envelope
func executeCompositeInEnvelope(ctx workflow.Context, retry *temporal.RetryPolicy, timeoutDuration time.Duration, fn func(inner workflow.Context) (map[string]interface{}, error)) (map[string]interface{}, error) {
	ctx, cancel := workflow.WithCancel(ctx)
	defer cancel()

	retryInterval := time.Duration(retry.InitialInterval)

	// Start timeout timer - cancel will stop all activities
	workflow.Go(ctx, func(ctx workflow.Context) {
		_ = workflow.Sleep(ctx, time.Duration(timeoutDuration))
		cancel()
	})

	var lastErr error

	for attempt := int32(1); attempt <= retry.MaximumAttempts; attempt++ {
		// Execute activities - they will be cancelled if timeout occurs
		val, err := fn(ctx)
		if err == nil {
			return val, nil // Success
		}

		lastErr = err

		// Check if the error was due to context cancellation (timeout)
		if temporal.IsCanceledError(err) {
			return nil, temporal.NewApplicationError(
				fmt.Sprintf("sequence timed out after %v during attempt %d",
					timeoutDuration, attempt),
				"SEQUENCE_TIMEOUT",
			)
		}

		// Check if error is non-retryable
		var appErr *temporal.ApplicationError
		if errors.As(err, &appErr) && appErr.NonRetryable() {
			return nil, err
		}

		// Sleep before retry (except for last attempt)
		if attempt < retry.MaximumAttempts {
			err := workflow.Sleep(ctx, retryInterval)
			if err != nil {
				// Sleep was cancelled - must be timeout
				return nil, temporal.NewApplicationError(
					fmt.Sprintf("sequence timed out after %v during retry delay after attempt %d",
						timeoutDuration, attempt),
					"SEQUENCE_TIMEOUT",
				)
			}

			retryInterval = time.Duration(float64(retryInterval) * retry.BackoffCoefficient)
			if retryInterval > retry.MaximumInterval {
				retryInterval = retry.MaximumInterval
			}
		}
	}

	return nil, temporal.NewApplicationError(
		fmt.Sprintf("sequence failed after %d attempts: %v", retry.MaximumAttempts, lastErr),
		"MAX_RETRIES_EXCEEDED",
		lastErr,
	)
}
