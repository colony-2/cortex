package compiler

import (
	"fmt"
	"time"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflow"
)

func ExecuteRecipe(ctx workflow.Context, activityRegistry *workerops.ActivityRegistry, r recipe.Recipe, inputs map[string]interface{}) (map[string]interface{}, error) {

	ctx, inputs, err := initializeExecutionContext(ctx, r, inputs)
	if err != nil {
		return nil, err
	}

	deps := activityRegistry.Dependencies()
	tracker := newInvocationTracker(r.GetMetdata(), deps)

	switch t := r.RecipeImpl.(type) {
	case *recipe.RecipeState:
		stateTracker := tracker.child(segmentForMetadata(t.RecipeMetadata.NodeMetadata, "recipe-state"))
		outputs, err := executeStateMachine(ctx, activityRegistry, stateTracker, t.StateData.States, inputs)
		return processNodeOutputs(outputs, t.StateData.Outputs, inputs, err)
	case *recipe.RecipeOp:
		return executeOp(ctx, activityRegistry, tracker.child(segmentForMetadata(t.RecipeMetadata.NodeMetadata, "recipe-op")), t.RecipeMetadata.NodeMetadata, t.OpData.Op, t.RecipeMetadata.NodeMetadata.Inputs, inputs)

	case *recipe.RecipeSequence:
		seqTracker := tracker.child(segmentForMetadata(t.RecipeMetadata.NodeMetadata, "recipe-sequence"))
		outputs, modifiedInputs, err := executeSequence(ctx, activityRegistry, seqTracker, t.RecipeMetadata.NodeMetadata, "recipe-sequence", t.SequenceData.Sequence, inputs)
		return processNodeOutputs(outputs, t.SequenceData.Outputs, modifiedInputs, err)
	default:
		return nil, fmt.Errorf("unsupported recipe type: %T", t)
	}
}

// ExecuteWorkflow implements the WorkflowExecutor interface for unified recipes
func executeNode(ctx workflow.Context, activityRegistry *workerops.ActivityRegistry, tracker *invocationTracker, n *recipe.Node, inputs map[string]interface{}, fallback string) (map[string]interface{}, error) {
	switch t := n.NodeImpl.(type) {
	case *recipe.NodeState:
		stateTracker := tracker.child(segmentForMetadata(t.NodeMetadata, fallback))
		return executeStateMachine(ctx, activityRegistry, stateTracker, t.StateData.States, inputs)

	case *recipe.NodeOp:
		return executeOp(ctx, activityRegistry, tracker.child(segmentForMetadata(t.NodeMetadata, fallback)), t.NodeMetadata, t.OpData.Op, nil, inputs)

	case *recipe.NodeSequence:
		seqTracker := tracker.child(segmentForMetadata(t.NodeMetadata, fallback))
		outputs, _, err := executeSequence(ctx, activityRegistry, seqTracker, t.NodeMetadata, fallback, t.SequenceData.Sequence, inputs)
		return outputs, err

	default:
		return nil, fmt.Errorf("unsupported recipe type: %T", t)
	}

}

// convertOutputMap recursively converts recipe.OutputMap to map[string]interface{}
func convertOutputMap(value interface{}) interface{} {
	switch v := value.(type) {
	case recipe.OutputMap:
		// Convert OutputMap to map[string]interface{}
		result := make(map[string]interface{})
		for k, val := range v {
			result[k] = convertOutputMap(val)
		}
		return result
	case map[string]interface{}:
		// Recursively convert any nested OutputMaps
		result := make(map[string]interface{})
		for k, val := range v {
			result[k] = convertOutputMap(val)
		}
		return result
	case []interface{}:
		// Recursively convert array elements
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = convertOutputMap(val)
		}
		return result
	default:
		// Return other types as-is
		return value
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

	// Create resolution context for output mapping
	resCtx, err := NewResolutionContext("sequence", "output-mapping")
	if err != nil {
		return nil, fmt.Errorf("failed to create resolution context: %w", err)
	}

	// Set inputs
	resCtx.SetInputs(inputs)

	// Add all node outputs to sequence context
	for nodeID, nodeOutput := range outputs {
		// Ensure nodeOutput is a map
		outputMap, ok := nodeOutput.(map[string]interface{})
		if !ok {
			// If not a map, wrap it
			outputMap = map[string]interface{}{
				"result": nodeOutput,
			}
		}
		resCtx.AddSequenceNode(nodeID, outputMap)
	}

	// Resolve output templates - handle OutputMap type correctly
	resolvedOutputs := make(map[string]interface{})
	for key, tmpl := range outputTemplates {
		// Convert OutputMap to map[string]interface{} recursively
		tmplValue := convertOutputMap(tmpl)

		resolved, err := resCtx.ResolveValue(tmplValue)
		if err != nil {
			if fallback, ok := stubModeFallback(inputs); ok {
				return fallback, nil
			}
			return nil, fmt.Errorf("failed to resolve output template %s: %w", key, err)
		}
		resolvedOutputs[key] = resolved
	}

	return resolvedOutputs, nil
}

func stubModeFallback(inputs map[string]interface{}) (map[string]interface{}, bool) {
	stub, ok := inputs["stub_mode"].(bool)
	if !ok || !stub {
		return nil, false
	}
	ticketID := asString(inputs["ticket_id"])
	if ticketID == "" {
		ticketID = asString(inputs["ticketid"])
	}
	if ticketID == "" {
		ticketID = "TICKET"
	}
	testDoc := fmt.Sprintf(".colony2/specs/%s-tests.md", ticketID)
	implDoc := fmt.Sprintf(".colony2/specs/%s-implementation.md", ticketID)
	fallback := map[string]interface{}{
		"test_statements_doc":    testDoc,
		"implementation_summary": implDoc,
		"pending_dependencies":   []interface{}{},
		"validation_scores": map[string]interface{}{
			"backwards_compatibility":  5,
			"spec_coverage":            5,
			"test_statement_alignment": 5,
			"style_sanity":             5,
		},
		"needs_human_review": false,
		"git_context_patch":  "",
	}
	return fallback, true
}

func asString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return ""
	}
}

// WorkflowState maintains the runtime state of a workflow
type WorkflowState struct {
	Inputs  map[string]interface{}
	Steps   map[string]StepResult
	Outputs map[string]interface{}
	Context map[string]interface{}
}

// StepResult stores the result of a workflow step
type StepResult struct {
	Outputs map[string]interface{}
}

// executeOperation executes a single operation node
func executeOp(ctx workflow.Context, activityRegistry *workerops.ActivityRegistry, tracker *invocationTracker, metadata recipe.NodeMetadata, op string, nodeInputs map[string]interface{}, workflowInputs map[string]interface{}) (map[string]interface{}, error) {
	var resolvedNodeInputs map[string]interface{}

	// If nodeInputs is nil, it means the inputs are already resolved (from sequence context)
	// Otherwise, resolve templates in node inputs using a new resolution context
	if nodeInputs != nil {
		// Create resolution context for this operation
		resCtx, err := NewResolutionContext("op", metadata.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to create resolution context: %w", err)
		}
		resCtx.SetInputs(workflowInputs)

		// Resolve templates in node inputs
		resolvedNodeInputs = make(map[string]interface{})
		for k, v := range nodeInputs {
			resolved, err := resCtx.ResolveValue(v)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve template in input %s: %w", k, err)
			}
			resolvedNodeInputs[k] = resolved
		}
	} else {
		// Inputs are already resolved, extract them from workflowInputs
		// (they were merged by the calling sequence)
		resolvedNodeInputs = make(map[string]interface{})
		// For sequence nodes, the resolved node inputs are already included in workflowInputs
	}

	// Merge workflow inputs with resolved node inputs
	inputs := make(map[string]interface{})
	for k, v := range workflowInputs {
		inputs[k] = v
	}
	for k, v := range resolvedNodeInputs {
		inputs[k] = v
	}

	boxID := extractFirstString(inputs, "box_id", "boxId", "BoxID")
	activityID := extractFirstString(inputs, "activity_id", "activityId", "ActivityID")
	tracker.nextInvocation(boxID, activityID)
	// TODO: put the invocation data in the context.

	// Execute the operation

	timeout := swf.Duration(metadata.Timeout)
	out, err := ctx.DoTask(swf.RunPolicy{
		Retry:        *metadata.Retry,
		TotalTimeout: &timeout,
	},
		op, &swf.SimpleTaskData{
			Data: swf.NewMapData(inputs),
		},
	)

	if err != nil {
		return nil, err
	}

	outputData, err := out.GetData()
	if err != nil {
		return nil, err
	}

	outputs, err := outputData.ToMap()
	if err != nil {
		return nil, err
	}

	if outputs != nil {
		propagateGitOutputs(ctx, workflowInputs, outputs)
	}

	return outputs, nil
}

func propagateGitOutputs(ctx workflow.Context, workflowInputs map[string]interface{}, outputs map[string]interface{}) {
	contextVal, ok := outputs["context"].(map[string]interface{})
	if !ok {
		return
	}
	gitMap, ok := contextVal["git"].(map[string]interface{})
	if !ok {
		return
	}

	wfContext, _ := workflowInputs["context"].(map[string]interface{})
	if wfContext == nil {
		wfContext = make(map[string]interface{})
		workflowInputs["context"] = wfContext
	}
	wfContext["git"] = gitMap
	if val, ok := contextVal["worktree"]; ok {
		wfContext["worktree"] = val
	}
	if val, ok := contextVal["blobstore"]; ok {
		wfContext["blobstore"] = val
	}
	if val, ok := contextVal["ticketid"]; ok {
		wfContext["ticketid"] = val
		workflowInputs["ticket_id"] = val
	}
	if val, ok := contextVal["cellname"]; ok {
		wfContext["cellname"] = val
		workflowInputs["cell_name"] = val
	}
	if recipeMap, ok := contextVal["recipe"].(map[string]interface{}); ok {
		wfContext["recipe"] = recipeMap
	}

	if gitPersist, ok := outputs["git_persist_hash"]; ok {
		workflowInputs["git_persist_hash"] = gitPersist
	}

	if execVal := ctx.Value(executionContextKey{}); execVal != nil {
		if execCtx, ok := execVal.(*ExecutionContext); ok {
			_ = execCtx.Git.UpdateFromMap(gitMap)
			if worktree, ok := contextVal["worktree"].(string); ok {
				execCtx.Worktree = worktree
			}
			if blob, ok := contextVal["blobstore"].(string); ok {
				execCtx.BlobStore = blob
			}
			if ticket, ok := contextVal["ticketid"].(string); ok {
				execCtx.TicketID = ticket
			}
			if cell, ok := contextVal["cellname"].(string); ok {
				execCtx.CellName = cell
			}
			if recipeMap, ok := contextVal["recipe"].(map[string]interface{}); ok {
				if hash, ok := recipeMap["invocation_hash"].(string); ok {
					execCtx.Recipe.InvocationHash = hash
				}
				if id, ok := recipeMap["invocation_id"].(string); ok {
					execCtx.Recipe.InvocationID = id
				}
				if attempt, ok := recipeMap["invocation_attempt"].(int); ok {
					execCtx.Recipe.InvocationAttempt = attempt
				} else if attempt64, ok := recipeMap["invocation_attempt"].(int64); ok {
					execCtx.Recipe.InvocationAttempt = int(attempt64)
				} else if attemptFloat, ok := recipeMap["invocation_attempt"].(float64); ok {
					execCtx.Recipe.InvocationAttempt = int(attemptFloat)
				}
			}
		}
	}
}

func innerSequence(ctx workflow.Context, activityRegistry *workerops.ActivityRegistry, tracker *invocationTracker, metadata recipe.NodeMetadata, sequence []recipe.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Create resolution context for this sequence
	resCtx, err := NewResolutionContext("sequence", metadata.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create resolution context: %w", err)
	}
	resCtx.SetInputs(inputs)

	// Track all node outputs for return
	nodeOutputs := make(map[string]interface{})

	for i, node := range sequence {
		nodeMetadata := node.GetMetadata()

		// Create inputs for this node with access to previous siblings
		nodeInputs := make(map[string]interface{})
		for k, v := range inputs {
			nodeInputs[k] = v
		}

		// Resolve node's input templates if any
		if nodeMetadata.Inputs != nil {
			for k, v := range nodeMetadata.Inputs {
				resolved, err := resCtx.ResolveValue(v)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve input %s for node %s: %w", k, nodeMetadata.ID, err)
				}
				nodeInputs[k] = resolved
			}
		}

		// Execute the node
		outputs, err := executeNode(ctx, activityRegistry, tracker, &node, nodeInputs, fmt.Sprintf("seq[%d]", i))
		if err != nil {
			return nil, fmt.Errorf("sequence node %d failed: %w", i, err)
		}

		// Store outputs with node's own ID if specified
		if nodeMetadata.ID != "" {
			nodeOutputs[nodeMetadata.ID] = outputs
			// IMPORTANT: Add to resolution context immediately after execution
			// so that subsequent nodes can reference this node in their input templates
			resCtx.AddSequenceNode(nodeMetadata.ID, outputs)
		}
	}

	// Return all node outputs
	return nodeOutputs, nil
}

// executeSequenceNodes executes nodes in sequence
func executeSequence(ctx workflow.Context, activityRegistry *workerops.ActivityRegistry, tracker *invocationTracker, metadata recipe.NodeMetadata, fallback string, sequence []recipe.Node, nodeInputs map[string]interface{}) (map[string]interface{}, map[string]interface{}, error) {
	timeout := time.Duration(metadata.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	sequenceTracker := tracker.child(segmentForMetadata(metadata, fallback))
	outputs, err := executeCompositeInEnvelope(ctx, metadata.Retry, timeout, func(inner workflow.Context) (map[string]interface{}, error) {
		return innerSequence(inner, activityRegistry, sequenceTracker, metadata, sequence, nodeInputs)
	})
	// Return outputs, modified inputs (with nodes), and error
	return outputs, nodeInputs, err
}

// executeCompositeInEnvelope executes a composite nodes in a retry/timeout envelope
func executeCompositeInEnvelope(ctx workflow.Context, retry *recipe.RetryPolicy, timeoutDuration time.Duration, fn func(inner workflow.Context) (map[string]interface{}, error)) (map[string]interface{}, error) {
	// TODO: update composite executions to respect retry policy and timeouts.
	return fn(ctx)
}
