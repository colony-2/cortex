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
		outputs, err := executeStateMachine(ctx, activityRegistry, t.StateData.States, inputs)
		return processNodeOutputs(outputs, t.StateData.Outputs, inputs, err)
	case *recipe.RecipeOp:
		return executeOp(ctx, activityRegistry, t.RecipeMetadata.NodeMetadata, t.OpData.Op, t.RecipeMetadata.NodeMetadata.Inputs, inputs)

	case *recipe.RecipeSequence:
		outputs, modifiedInputs, err := executeSequence(ctx, activityRegistry, t.RecipeMetadata.NodeMetadata, t.SequenceData.Sequence, inputs)
		return processNodeOutputs(outputs, t.SequenceData.Outputs, modifiedInputs, err)
	default:
		return nil, fmt.Errorf("unsupported recipe type: %T", t)
	}
}

// ExecuteWorkflow implements the WorkflowExecutor interface for unified recipes
func executeNode(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, n *recipe.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	switch t := n.NodeImpl.(type) {
	case *recipe.NodeState:
		return executeStateMachine(ctx, activityRegistry, t.StateData.States, inputs)

	case *recipe.NodeOp:
		return executeOp(ctx, activityRegistry, t.NodeMetadata, t.OpData.Op, t.NodeMetadata.Inputs, inputs)

	case *recipe.NodeSequence:
		return executeSequence(ctx, activityRegistry, t.NodeMetadata, t.SequenceData.Sequence, inputs)

	default:
		return nil, fmt.Errorf("unsupported recipe type: %T", t)
	}

}

// processNodeOutputs processes outputs from node execution
func processNodeOutputs(outputs map[string]interface{}, outputTemplates map[string]interface{}, inputs map[string]interface{}, err error) (map[string]interface{}, error) {

	if err != nil {
		return nil, err
	}

	if outputTemplates == nil || len(outputTemplates) == 0 {
		return outputs, nil
	}

	// Build nodes structure for template resolution
	steps := make(map[string]StepResult)
	for nodeID, nodeOutput := range outputs {
		// Ensure nodeOutput is a map
		outputMap, ok := nodeOutput.(map[string]interface{})
		if !ok {
			// If not a map, wrap it
			outputMap = map[string]interface{}{
				"result": nodeOutput,
			}
		}
		steps[nodeID] = StepResult{
			Outputs: outputMap,
		}
	}
	
	// Create template resolver with workflow state
	resolver := &TemplateResolver{
		state: &WorkflowState{
			Inputs: inputs,
			Steps:  steps,
		},
	}
	
	// Resolve output templates
	resolvedOutputs := make(map[string]interface{})
	for key, tmpl := range outputTemplates {
		resolved, err := resolver.ResolveValue(tmpl)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve output template %s: %w", key, err)
		}
		resolvedOutputs[key] = resolved
	}
	
	return resolvedOutputs, nil
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

	// ExecuteAsActivity returns true when it has a handler (should be executed as activity)
	if !opImpl.Activity.ExecuteAsActivity() {
		timeout := time.Duration(metadata.Timeout)
		if timeout == 0 {
			timeout = 30 * time.Second // Default timeout
		}
		return executeCompositeInEnvelope(ctx, ToTemporalRetryPolicy(metadata.Retry), timeout, func(inner workflow.Context) (map[string]interface{}, error) {
			return opImpl.Activity.ExecuteInline(ctx, inputs)
		})
	}

	// Configure activity options
	timeout := time.Duration(metadata.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: timeout,
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
	// Track all node outputs for template resolution
	nodeOutputs := make(map[string]interface{})
	
	// Create nodes map for template resolution
	if inputs["nodes"] == nil {
		inputs["nodes"] = make(map[string]interface{})
	}
	nodesMap := inputs["nodes"].(map[string]interface{})
	
	for i, node := range sequence {
		outputs, err := executeNode(ctx, activityRegistry, &node, inputs)
		if err != nil {
			return nil, fmt.Errorf("sequence node %d failed: %w", i, err)
		}

		// Store outputs with node's own ID if specified
		nodeMetadata := node.GetMetadata()
		if nodeMetadata.ID != "" {
			// Store in nodes map for template access (.nodes.<id>.outputs)
			nodesMap[nodeMetadata.ID] = map[string]interface{}{
				"outputs": outputs,
			}
			nodeOutputs[nodeMetadata.ID] = outputs
		}
	}

	// Return all node outputs
	return nodeOutputs, nil
}

// executeSequenceNodes executes nodes in sequence
func executeSequence(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, metadata recipe.NodeMetadata, sequence []recipe.Node, nodeInputs map[string]interface{}) (map[string]interface{}, map[string]interface{}, error) {
	timeout := time.Duration(metadata.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	outputs, err := executeCompositeInEnvelope(ctx, ToTemporalRetryPolicy(metadata.Retry), timeout, func(inner workflow.Context) (map[string]interface{}, error) {
		return innerSequence(inner, activityRegistry, metadata, sequence, nodeInputs)
	})
	// Return outputs, modified inputs (with nodes), and error
	return outputs, nodeInputs, err
}

// executeCompositeInEnvelope executes a composite nodes in a retry/timeout envelope
func executeCompositeInEnvelope(ctx workflow.Context, retry *temporal.RetryPolicy, timeoutDuration time.Duration, fn func(inner workflow.Context) (map[string]interface{}, error)) (map[string]interface{}, error) {
	// If no retry policy, just execute once with timeout
	if retry == nil {
		if timeoutDuration > 0 {
			var cancel workflow.CancelFunc
			ctx, cancel = workflow.WithCancel(ctx)
			defer cancel()
			
			// Start timeout timer
			workflow.Go(ctx, func(ctx workflow.Context) {
				_ = workflow.Sleep(ctx, timeoutDuration)
				cancel()
			})
		}
		return fn(ctx)
	}
	
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
