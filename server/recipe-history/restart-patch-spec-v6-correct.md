# Recipe Restart and Patching Specification (v6 - Correct Approach)

## The Problem with Previous Approaches

- **Can't change workflow input during reset** - Temporal replays with original input
- **Shouldn't use memo to change behavior** - Breaks determinism guarantees
- **Signals during replay are complex** - Timing and determinism issues

## The Correct Solution: Workflow Versioning

Temporal provides **workflow versioning** specifically for changing behavior during replay. This is the proper, supported way to handle patches.

### Core Pattern

```go
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    logger := workflow.GetLogger(ctx)
    
    // Use versioning to handle patches properly
    version := workflow.GetVersion(ctx, "patch-version", workflow.DefaultVersion, 1)
    
    var patches map[string]Patch
    
    if version == workflow.DefaultVersion {
        // Original execution - no patches
        patches = input.Patches // Will be empty on first run
    } else {
        // Version 1: Check for patches after reset
        // This is deterministic because version is part of history
        patches = getPatchesForVersion(ctx, version)
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

## The Actual Best Solution: Start New Workflow

Given the constraints, the **cleanest and most correct approach** is to start a new workflow:

```go
func (s *RestartService) RestartRecipeExecution(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    // 1. Get original execution details
    original, err := s.getOriginalExecution(ctx, req.WorkflowID, req.RunID)
    if err != nil {
        return nil, err
    }
    
    // 2. Determine what outputs to preserve based on restart point
    preservedOutputs := s.getOutputsBeforeRestartPoint(ctx, req.WorkflowID, req.RunID, req.RestartPoint)
    
    // 3. Start new workflow with patches and preserved outputs
    newWorkflowID := fmt.Sprintf("%s-restart-%s", req.WorkflowID, uuid.New().String()[:8])
    
    we, err := s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
        ID:        newWorkflowID,
        TaskQueue: original.TaskQueue,
        Memo: map[string]interface{}{
            "original_workflow_id": req.WorkflowID,
            "original_run_id":      req.RunID,
            "restart_point":        req.RestartPoint,
            "restart_reason":       req.Reason,
        },
    }, RecipeInterpreterWorkflow, RecipeWorkflowInput{
        Recipe:           original.Recipe,
        Inputs:           original.Inputs,
        Patches:          req.Patches,
        PreservedOutputs: preservedOutputs, // Outputs from ops before restart point
        RestartPoint:     req.RestartPoint,
    })
    
    if err != nil {
        return nil, err
    }
    
    // 4. Cancel original workflow
    err = s.temporalClient.CancelWorkflow(ctx, req.WorkflowID, req.RunID)
    if err != nil {
        logger.Warn("Failed to cancel original workflow", "error", err)
    }
    
    return &RestartResponse{
        WorkflowID:         newWorkflowID,
        RunID:              we.GetRunID(),
        OriginalWorkflowID: req.WorkflowID,
        RestartedAt:        time.Now(),
    }, nil
}
```

### Modified Interpreter to Handle Preserved Outputs

```go
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    logger := workflow.GetLogger(ctx)
    
    execCtx := &ExecutionContext{
        Inputs:  input.Inputs,
        Outputs: make(map[string]interface{}),
        Patches: input.Patches,
    }
    
    // Pre-populate outputs from before restart point
    if input.PreservedOutputs != nil {
        for opID, output := range input.PreservedOutputs {
            execCtx.Outputs[opID] = output
            logger.Info("Using preserved output", "op", opID)
        }
    }
    
    // Execute recipe ops
    for _, op := range input.Recipe.Ops {
        // Skip ops before restart point (we have their outputs)
        if input.RestartPoint != "" && isBeforeRestartPoint(op.ID, input.RestartPoint) {
            logger.Info("Skipping op before restart point", "op", op.ID)
            continue
        }
        
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
        // Pass patches to nested workflow
        return executeNestedRecipe(ctx, op, opInput, execCtx.Patches)
        
    default:
        return nil, fmt.Errorf("unknown op type: %s", op.Type)
    }
}

func executeNestedRecipe(ctx workflow.Context, op OpDefinition, input interface{}, parentPatches map[string]Patch) (interface{}, error) {
    // Extract patches for nested recipe
    nestedPatches := extractNestedPatches(parentPatches, op.ID)
    
    // Nested recipe as child workflow with patches
    var output interface{}
    err := workflow.ExecuteChildWorkflow(
        workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
            WorkflowID: fmt.Sprintf("%s-%s", workflow.GetInfo(ctx).WorkflowExecution.ID, op.ID),
        }),
        RecipeInterpreterWorkflow,
        RecipeWorkflowInput{
            Recipe:  op.NestedRecipe,
            Inputs:  input,
            Patches: nestedPatches, // Patches passed as normal input
        },
    ).Get(ctx, &output)
    
    return output, err
}
```

## Why This Approach is Correct

1. **No determinism violations** - New workflow starts fresh with patches as input
2. **Clear semantics** - Restart = new workflow with preserved outputs
3. **Temporal-native** - Uses workflows as designed
4. **Simple child handling** - Children always get patches via input
5. **Audit trail** - Original workflow linked via memo

## Alternative: Use Continue-As-New

If keeping the same workflow ID is critical:

```go
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    // Check if this is a restart continuation
    if input.IsRestart && input.RestartPoint != "" {
        // Execute from restart point with patches
        return executeFromRestartPoint(ctx, input)
    }
    
    // Normal execution
    execCtx := &ExecutionContext{
        Inputs:  input.Inputs,
        Outputs: make(map[string]interface{}),
        Patches: input.Patches,
    }
    
    for _, op := range input.Recipe.Ops {
        output, err := executeOp(ctx, op, execCtx)
        if err != nil {
            // Check if we need to restart at this point
            if shouldRestart(err) {
                // Continue-as-new with restart info
                return nil, workflow.NewContinueAsNewError(
                    ctx,
                    RecipeInterpreterWorkflow,
                    RecipeWorkflowInput{
                        Recipe:       input.Recipe,
                        Inputs:       input.Inputs,
                        Patches:      getPatchesForRestart(),
                        IsRestart:    true,
                        RestartPoint: op.ID,
                        PreservedOutputs: execCtx.Outputs,
                    },
                )
            }
            return nil, err
        }
        execCtx.Outputs[op.ID] = output
    }
    
    return &RecipeWorkflowOutput{
        Outputs: execCtx.Outputs,
    }, nil
}
```

## Summary

### The Right Way:
1. **Start new workflow** with patches as input and preserved outputs
2. **Cancel original workflow** to avoid confusion
3. **Link workflows via memo** for audit trail

### Why NOT to use memo for behavior:
- **Violates Temporal guarantees** - Memo is for metadata, not behavior
- **Could break in future** - Temporal could optimize memo handling
- **Not deterministic** - Replay behavior would differ

### Why NOT to use reset with patches:
- **Can't change input** - Reset uses original input
- **Complex versioning** - Would need versioning for every patch scenario
- **Not the intended use** - Reset is for retry, not behavior change

## Success Criteria

- [ ] New workflow started with patches and preserved outputs
- [ ] Original workflow cancelled or marked as superseded
- [ ] Child workflows receive patches via input (not memo/signals)
- [ ] No determinism violations
- [ ] Clear audit trail linking original and restart workflows
- [ ] Works with standard Temporal patterns