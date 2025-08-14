# Recipe Restart and Patching Specification (v7 - Final Understanding)

## Key Insight: How Temporal Reset Actually Works

When you reset a workflow:
1. **History is preserved** up to the reset point
2. **Activities/child workflows before reset point are NOT re-executed** - their results come from history
3. **Execution continues from reset point** with new behavior

This means we CAN use reset effectively, but we need to handle patches correctly without violating determinism.

## The Solution: Versioned Patches with External Store

Since we can't change workflow input or use memo for behavior, we need:
1. **External patch store** - Store patches outside Temporal
2. **Workflow versioning** - Deterministically check for patches
3. **Reset to replay** - Let Temporal handle the history

### Architecture

```go
// External patch store (Redis, DB, etc.)
type PatchStore interface {
    // Store patches for a workflow run
    StorePatchesForRun(workflowID, runID string, patches map[string]Patch) error
    
    // Get patches for a workflow run (returns nil if none)
    GetPatchesForRun(workflowID, runID string) (map[string]Patch, error)
}

// Recipe interpreter workflow
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    logger := workflow.GetLogger(ctx)
    info := workflow.GetInfo(ctx)
    
    // Use versioning to check for patches deterministically
    v := workflow.GetVersion(ctx, "patches-v1", workflow.DefaultVersion, 1)
    
    var patches map[string]Patch
    if v >= 1 {
        // Version 1+: Check external store for patches
        // This is deterministic because version is part of history
        err := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
            // SideEffect is deterministic - same input always returns same output
            p, err := patchStore.GetPatchesForRun(info.WorkflowExecution.ID, info.WorkflowExecution.RunID)
            if err != nil {
                return nil
            }
            return p
        }).Get(&patches)
        
        if err != nil {
            logger.Warn("Failed to get patches", "error", err)
        }
    }
    
    // Merge with any patches from input (for new workflows)
    if input.Patches != nil {
        if patches == nil {
            patches = input.Patches
        } else {
            // Merge input patches with stored patches
            for k, v := range input.Patches {
                patches[k] = v
            }
        }
    }
    
    // Execute with patches
    execCtx := &ExecutionContext{
        Inputs:  input.Inputs,
        Outputs: make(map[string]interface{}),
        Patches: patches,
    }
    
    for _, op := range input.Recipe.Ops {
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

### Restart Implementation

```go
func (s *RestartService) RestartRecipeExecution(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    // 1. Store patches for the new run
    // The reset will create a new run ID, but we can predict it
    err := s.patchStore.StorePatchesForRun(req.WorkflowID, req.RunID+"-reset", req.Patches)
    if err != nil {
        return nil, err
    }
    
    // 2. Find where to reset to
    // We need to reset to BEFORE the first patched op
    resetPoint := s.findEarliestResetPoint(req.RestartPoint, req.Patches)
    resetEventID, err := s.findEventIDForOp(ctx, req.WorkflowID, req.RunID, resetPoint)
    if err != nil {
        return nil, err
    }
    
    // 3. Reset the workflow
    resetResp, err := s.temporalClient.ResetWorkflowExecution(ctx, &workflowservice.ResetWorkflowExecutionRequest{
        Namespace: "default",
        WorkflowExecution: &common.WorkflowExecution{
            WorkflowId: req.WorkflowID,
            RunId:      req.RunID,
        },
        Reason:                    fmt.Sprintf("Restart at %s: %s", req.RestartPoint, req.Reason),
        WorkflowTaskFinishEventId: resetEventID,
    })
    if err != nil {
        return nil, err
    }
    
    // 4. Update patch store with actual run ID
    err = s.patchStore.MovePatchesToRun(req.WorkflowID, req.RunID+"-reset", resetResp.RunId)
    if err != nil {
        logger.Warn("Failed to update patch store", "error", err)
    }
    
    return &RestartResponse{
        WorkflowID:  req.WorkflowID,
        RunID:       resetResp.RunId,
        RestartedAt: time.Now(),
    }, nil
}
```

## How This Works

### Example: Restart with Patch

Original execution:
```
opA1 (activity) -> ✓ completed
opA2 (activity) -> ✓ completed  
opA3 (child workflow) -> ✓ completed
opA4 (activity) -> ✓ completed
```

User requests: Restart at opA3 with patch

What happens:
1. Store patches in external store for new run
2. Reset workflow to before opA3
3. Workflow replays:
   - opA1: **NOT re-executed** - result from history
   - opA2: **NOT re-executed** - result from history
   - opA3: **Re-executed** with patch (version check finds patches)
   - opA4: **Re-executed** with new opA3 output

### Child Workflow Handling

```go
func executeNestedRecipe(ctx workflow.Context, op OpDefinition, input interface{}, parentPatches map[string]Patch) (interface{}, error) {
    // Extract patches for nested recipe
    nestedPatches := extractNestedPatches(parentPatches, op.ID)
    
    // Child workflow ALWAYS gets patches via input
    // This ensures deterministic execution
    var output interface{}
    err := workflow.ExecuteChildWorkflow(
        workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
            WorkflowID: fmt.Sprintf("%s-%s", workflow.GetInfo(ctx).WorkflowExecution.ID, op.ID),
        }),
        RecipeInterpreterWorkflow,
        RecipeWorkflowInput{
            Recipe:  op.NestedRecipe,
            Inputs:  input,
            Patches: nestedPatches, // Passed as input, not from external store
        },
    ).Get(ctx, &output)
    
    return output, err
}
```

## Why This Works

1. **Temporal reset preserves history** - Activities before reset point not re-executed
2. **Versioning ensures determinism** - Version check is part of replayed history
3. **SideEffect is deterministic** - Same patches always returned for same run
4. **Child workflows get patches via input** - Clean, deterministic

## Alternative: Simpler Approach Without External Store

If you don't want an external store, the simplest correct approach is:

```go
func (s *RestartService) RestartRecipeExecution(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    // Just start a new workflow with skipBefore parameter
    
    we, err := s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
        ID:        fmt.Sprintf("%s-restart-%d", req.WorkflowID, time.Now().Unix()),
        TaskQueue: "recipe-queue",
    }, RecipeInterpreterWorkflow, RecipeWorkflowInput{
        Recipe:     original.Recipe,
        Inputs:     original.Inputs,
        Patches:    req.Patches,
        SkipBefore: req.RestartPoint, // Skip execution before this point
    })
    
    return &RestartResponse{
        WorkflowID: we.GetID(),
        RunID:      we.GetRunID(),
    }, nil
}

func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    execCtx := &ExecutionContext{
        Inputs:  input.Inputs,
        Outputs: make(map[string]interface{}),
        Patches: input.Patches,
    }
    
    for _, op := range input.Recipe.Ops {
        // Skip ops before restart point
        if input.SkipBefore != "" && isBeforePoint(op.ID, input.SkipBefore) {
            // Use a "fast-forward" activity to get previous output
            var previousOutput interface{}
            err := workflow.ExecuteActivity(ctx, GetPreviousOpOutput, input.OriginalRunID, op.ID).Get(ctx, &previousOutput)
            if err != nil {
                return nil, err
            }
            execCtx.Outputs[op.ID] = previousOutput
            continue
        }
        
        // Normal execution with patches
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

## Recommendation

**For production use**: Use the external patch store approach with reset
- Preserves workflow ID
- Leverages Temporal's history
- Clean deterministic execution

**For simpler cases**: Start new workflow with skipBefore
- No external dependencies
- Clear semantics
- Slightly less efficient (fetches previous outputs)

## Success Criteria

- [ ] Activities before reset point not re-executed (use history)
- [ ] Patches applied deterministically via versioning
- [ ] Child workflows receive patches via input
- [ ] No memo abuse or determinism violations
- [ ] Works with Temporal's native reset capability