# Recipe Restart and Patching Specification (Final)

## Overview

The recipe-worker interprets recipes at runtime as Temporal workflows. This spec defines how restarts and patches work using Temporal's native capabilities with a signal-based approach.

## Core Pattern: Signal-Based Patches

All workflows start by waiting for a patches signal. This provides consistent behavior between normal execution and restart execution.

### Recipe Interpreter Workflow

```go
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    logger := workflow.GetLogger(ctx)
    
    // ALWAYS wait for patches signal at start
    var patches map[string]Patch
    patchChannel := workflow.GetSignalChannel(ctx, "patches")
    patchChannel.Receive(ctx, &patches) // Blocks until signal received
    
    logger.Info("Received patches signal", "count", len(patches))
    
    // Execute with patches (could be empty for normal execution)
    execCtx := &ExecutionContext{
        Inputs:  input.Inputs,
        Outputs: make(map[string]interface{}),
        Patches: patches,
    }
    
    // Execute recipe ops
    for _, op := range input.Recipe.Ops {
        output, err := executeOp(ctx, op, execCtx)
        if err != nil {
            return nil, fmt.Errorf("op %s failed: %w", op.ID, err)
        }
        execCtx.Outputs[op.ID] = output
    }
    
    return &RecipeWorkflowOutput{
        Outputs: execCtx.Outputs,
    }, nil
}

func executeOp(ctx workflow.Context, op OpDefinition, execCtx *ExecutionContext) (interface{}, error) {
    // Resolve inputs from context
    opInput := resolveOpInput(op, execCtx)
    
    // Apply patches if present
    if patch, exists := execCtx.Patches[op.ID]; exists {
        opInput = applyPatch(opInput, patch)
        workflow.GetLogger(ctx).Info("Applied patch to op", "op", op.ID)
    }
    
    // Execute based on op type
    switch op.Type {
    case "activity":
        return executeActivityOp(ctx, op, opInput)
        
    case "workflow":
        // Nested recipes become child workflows
        return executeNestedRecipe(ctx, op, opInput, execCtx.Patches)
        
    case "sequential", "parallel":
        return executeComposition(ctx, op, opInput, execCtx)
        
    case "conditional":
        return executeConditional(ctx, op, opInput, execCtx)
        
    default:
        return nil, fmt.Errorf("unknown op type: %s", op.Type)
    }
}
```

### Child Workflow Handling

Child workflows follow the same pattern - they wait for patches from their parent:

```go
func executeNestedRecipe(ctx workflow.Context, op OpDefinition, input interface{}, parentPatches map[string]Patch) (interface{}, error) {
    logger := workflow.GetLogger(ctx)
    
    // Extract patches for this nested recipe
    nestedPatches := extractNestedPatches(parentPatches, op.ID)
    
    // Start child workflow
    childID := fmt.Sprintf("%s-%s", workflow.GetInfo(ctx).WorkflowExecution.ID, op.ID)
    childFuture := workflow.ExecuteChildWorkflow(
        workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
            WorkflowID: childID,
        }),
        RecipeInterpreterWorkflow,
        RecipeWorkflowInput{
            Recipe: op.NestedRecipe,
            Inputs: input,
        },
    )
    
    // Immediately signal child with patches
    signalFuture := workflow.SignalExternalWorkflow(
        ctx,
        childID,
        "", // Empty run ID means latest run
        "patches",
        nestedPatches,
    )
    
    // Wait for signal to be sent
    err := signalFuture.Get(ctx, nil)
    if err != nil {
        logger.Warn("Failed to signal child workflow", "error", err)
    }
    
    // Wait for child to complete
    var output interface{}
    err = childFuture.Get(ctx, &output)
    return output, err
}

func extractNestedPatches(patches map[string]Patch, prefix string) map[string]Patch {
    // Extract patches for nested ops
    // E.g., "dataProcessor.step1" -> "step1" for nested recipe
    nested := make(map[string]Patch)
    prefixDot := prefix + "."
    
    for path, patch := range patches {
        if strings.HasPrefix(path, prefixDot) {
            nestedPath := strings.TrimPrefix(path, prefixDot)
            nested[nestedPath] = patch
        }
    }
    
    return nested
}
```

## Normal Execution

For normal recipe execution, start with an empty patches signal:

```go
func (s *RecipeService) ExecuteRecipe(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
    // Start workflow with signal containing empty patches
    wfOptions := client.StartWorkflowOptions{
        ID:        req.WorkflowID,
        TaskQueue: "recipe-queue",
    }
    
    we, err := s.temporalClient.SignalWithStartWorkflow(
        ctx,
        req.WorkflowID,
        "patches",
        map[string]Patch{}, // Empty patches for normal execution
        wfOptions,
        RecipeInterpreterWorkflow,
        RecipeWorkflowInput{
            Recipe: req.Recipe,
            Inputs: req.Inputs,
        },
    )
    
    if err != nil {
        return nil, err
    }
    
    return &ExecuteResponse{
        WorkflowID: we.GetID(),
        RunID:      we.GetRunID(),
    }, nil
}
```

## Restart Execution

For restart with patches, reset without signal replay and send new patches:

```go
func (s *RestartService) RestartRecipeExecution(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    // 1. Find where to reset based on restart point
    resetEventID, err := s.findResetPoint(ctx, req.WorkflowID, req.RunID, req.RestartPoint)
    if err != nil {
        return nil, err
    }
    
    // 2. Merge any cumulative patches if this is a re-restart
    cumulativePatches := s.mergePatchesIfNeeded(ctx, req.WorkflowID, req.RunID, req.Patches)
    
    // 3. Reset workflow WITHOUT replaying signals
    resetResp, err := s.temporalClient.ResetWorkflowExecution(ctx, &workflowservice.ResetWorkflowExecutionRequest{
        Namespace: "default",
        WorkflowExecution: &common.WorkflowExecution{
            WorkflowId: req.WorkflowID,
            RunId:      req.RunID,
        },
        Reason:                    fmt.Sprintf("Restart at %s: %s", req.RestartPoint, req.Reason),
        WorkflowTaskFinishEventId: resetEventID,
        RequestId:                 uuid.New().String(),
        // CRITICAL: Don't replay the original empty patches signal
        ResetReapplyType: enumspb.RESET_REAPPLY_TYPE_NONE,
    })
    if err != nil {
        return nil, err
    }
    
    // 4. Immediately send patches signal to the reset workflow
    err = s.temporalClient.SignalWorkflow(
        ctx,
        req.WorkflowID,
        resetResp.RunId,
        "patches",
        cumulativePatches,
    )
    if err != nil {
        return nil, fmt.Errorf("failed to signal patches: %w", err)
    }
    
    // 5. Record restart for audit
    s.recordRestart(req, resetResp.RunId)
    
    return &RestartResponse{
        WorkflowID:  req.WorkflowID,
        RunID:       resetResp.RunId,
        RestartedAt: time.Now(),
    }, nil
}

func (s *RestartService) findResetPoint(ctx context.Context, workflowID, runID, restartPoint string) (int64, error) {
    // We need to reset to BEFORE the first op that needs patches
    // This ensures the op can be re-executed with patches
    
    iter := s.temporalClient.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
    
    // Find the activity/child workflow that corresponds to the restart point
    // Then find the workflow task completed event right before it
    targetOpID := extractOpID(restartPoint)
    
    for iter.HasNext() {
        event, err := iter.Next()
        if err != nil {
            return 0, err
        }
        
        switch event.GetEventType() {
        case enumspb.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
            attrs := event.GetActivityTaskScheduledEventAttributes()
            if attrs.ActivityId == targetOpID {
                // Find the workflow task completed before this activity
                return findPrecedingWorkflowTaskCompleted(event.EventId), nil
            }
            
        case enumspb.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED:
            attrs := event.GetStartChildWorkflowExecutionInitiatedEventAttributes()
            if strings.Contains(attrs.WorkflowId, targetOpID) {
                // Find the workflow task completed before this child workflow
                return findPrecedingWorkflowTaskCompleted(event.EventId), nil
            }
        }
    }
    
    return 0, fmt.Errorf("op %s not found in workflow history", restartPoint)
}
```

## Cumulative Patches

For handling multiple restarts (restart of an already-restarted workflow):

```go
func (s *RestartService) mergePatchesIfNeeded(ctx context.Context, workflowID, runID string, newPatches map[string]Patch) map[string]Patch {
    // Query the workflow to see if it already has patches
    response, err := s.temporalClient.QueryWorkflow(ctx, workflowID, runID, "get-patches")
    if err != nil {
        // First restart, use new patches as-is
        return newPatches
    }
    
    var existingPatches map[string]Patch
    if err := response.Get(&existingPatches); err != nil {
        return newPatches
    }
    
    // Merge patches (new patches override existing for same op)
    merged := make(map[string]Patch)
    for k, v := range existingPatches {
        merged[k] = v
    }
    for k, v := range newPatches {
        merged[k] = v // Override or add new
    }
    
    return merged
}

// Add query handler to workflow
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    var patches map[string]Patch
    
    // Register query handler for patches
    workflow.SetQueryHandler(ctx, "get-patches", func() (map[string]Patch, error) {
        return patches, nil
    })
    
    // Wait for patches signal
    patchChannel := workflow.GetSignalChannel(ctx, "patches")
    patchChannel.Receive(ctx, &patches)
    
    // ... rest of execution
}
```

## Important Limitation: Child Workflow Re-execution

**When resetting a parent workflow, child workflows are COMPLETELY RE-EXECUTED from the beginning.** They do not preserve any execution history. This means:

- If you restart at `parent.child.opX`, the entire child workflow runs again
- Only the parent workflow benefits from history preservation
- Child workflows cannot skip ops - they always start fresh

### Why This Happens

When the parent workflow replays after reset:
1. It reaches the `ExecuteChildWorkflow` call
2. Temporal creates a **NEW child workflow instance** 
3. This new instance has no history - it starts from scratch
4. The child receives patches and executes everything (with patches applied where relevant)

## Example Scenarios

### Normal Execution
```bash
vibethis recipe execute \
  --recipe data-pipeline \
  --inputs '{"source": "prod"}'
```

Result:
1. Workflow starts with SignalWithStart (empty patches)
2. All ops execute normally
3. Child workflows receive empty patches

### Restart at Child Workflow (LIMITATION)
```bash
vibethis recipe restart \
  --workflow-id pipeline-123 \
  --run-id run-456 \
  --restart-point "data-pipeline.process.transform" \
  --patch '{"algorithm": "v2"}'
```

What Actually Happens:
1. Reset parent to before "process" child workflow
2. Signal with patches: `{"process.transform": {"algorithm": "v2"}}`
3. Parent replay preserves results before "process"
4. **"process" child workflow starts fresh and executes ALL ops:**
   - `validate` - executes (even though we wanted to skip)
   - `transform` - executes with patch
   - `enrich` - executes
5. Parent continues with new child output

### Workaround: Flatten Critical Paths

If you need fine-grained restart control, avoid deep nesting:

```yaml
# Instead of:
ops:
  - id: process
    type: workflow
    recipe:
      ops:
        - id: validate
        - id: transform  
        - id: enrich

# Consider:
ops:
  - id: process_validate
    type: activity
  - id: process_transform
    type: activity
  - id: process_enrich
    type: activity
```

### Alternative Solution: Child Workflow Skip Logic

Modify child workflows to support skipping:

```go
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    var patches map[string]Patch
    patchChannel := workflow.GetSignalChannel(ctx, "patches")
    patchChannel.Receive(ctx, &patches)
    
    // Check if we should skip ops before a certain point
    skipBefore := ""
    if input.RestartContext != nil {
        skipBefore = input.RestartContext.SkipBefore
    }
    
    execCtx := &ExecutionContext{
        Inputs:  input.Inputs,
        Outputs: make(map[string]interface{}),
        Patches: patches,
    }
    
    for _, op := range input.Recipe.Ops {
        // Skip ops if requested
        if skipBefore != "" && isBeforePoint(op.ID, skipBefore) {
            // Try to get cached output from parent
            if cachedOutput, exists := input.RestartContext.CachedOutputs[op.ID]; exists {
                execCtx.Outputs[op.ID] = cachedOutput
                continue
            }
        }
        
        output, err := executeOp(ctx, op, execCtx)
        if err != nil {
            return nil, err
        }
        execCtx.Outputs[op.ID] = output
    }
    
    return &RecipeWorkflowOutput{
        Outputs: execCtx.Outputs,
    }, nil
}
```

### Second Restart (Cumulative)
```bash
vibethis recipe restart \
  --workflow-id pipeline-123 \
  --run-id run-789 \
  --restart-point "data-pipeline.store" \
  --patch '{"batch_size": 1000}'
```

Result:
1. Query finds existing patches: `{"process.transform": {"algorithm": "v2"}}`
2. Merge with new: `{"process.transform": {"algorithm": "v2"}, "store": {"batch_size": 1000}}`
3. Reset to before "store"
4. Signal with cumulative patches
5. "store" executes with patch, previous patches preserved

## Key Benefits

1. **Deterministic** - Signal receive is part of workflow history
2. **Consistent** - All workflows follow same pattern (wait for patches)
3. **No external dependencies** - No need for patch store
4. **Preserves workflow ID** - Reset keeps same workflow identity
5. **Leverages Temporal** - Uses reset and signals as designed
6. **Handles nesting** - Parent signals children with filtered patches

## Implementation Considerations

### Signal Timing
The signal must be sent immediately after reset to ensure the workflow receives it before proceeding. This is generally reliable since the workflow blocks on receive.

### Child Workflow Signals
Since `SignalWithStartChildWorkflow` doesn't exist, we start the child and immediately signal it. The child blocks on receive, so timing is not an issue.

### Error Handling
If signaling fails (rare), the workflow will block forever. Consider adding a timeout:

```go
// Add timeout to patch receive
var patches map[string]Patch
patchChannel := workflow.GetSignalChannel(ctx, "patches")

ctx, cancel := workflow.WithCancel(ctx)
workflow.Go(ctx, func(ctx workflow.Context) {
    workflow.Sleep(ctx, 30*time.Second)
    cancel()
})

if !patchChannel.ReceiveWithTimeout(ctx, 30*time.Second, &patches) {
    // Timeout - assume empty patches for recovery
    patches = make(map[string]Patch)
    logger.Warn("Timeout waiting for patches signal, continuing with empty patches")
}
```

## Success Criteria

- [x] Workflows always wait for patches signal at start
- [x] Normal execution uses SignalWithStart with empty patches
- [x] Restart uses Reset with NONE reapply + patches signal
- [x] Child workflows receive patches via parent signals
- [x] Cumulative patches handled via query and merge
- [x] No external patch store needed
- [x] Preserves workflow ID through restarts
- [x] Deterministic execution maintained
- [x] Works with Temporal's native patterns