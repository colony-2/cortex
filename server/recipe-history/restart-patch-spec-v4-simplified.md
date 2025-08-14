# Recipe Restart and Patching Specification (v4 - Simplified)

## Core Insight

We should leverage Temporal's native capabilities instead of building custom logic:
1. **Always reset the root workflow** - Temporal handles replay automatically
2. **Pass patches as child workflow inputs** - No special memo/query handling needed
3. **Child workflows naturally create restart boundaries** - Each can be independently versioned

## Architecture

### Simple Pattern: Reset Root, Patch Inputs

```go
// The compiler generates workflows that accept patches as normal inputs
func RootWorkflow(ctx workflow.Context, input RootWorkflowInput) (*RootWorkflowOutput, error) {
    logger := workflow.GetLogger(ctx)
    
    // Patches are just part of the input after reset
    patches := input.Patches // Will be nil on first run, populated on restart
    
    // Op A1 - Execute with potential patch
    var opA1Output OpA1Output
    opA1Input := input.OpA1Input
    if patch, exists := patches["opA1"]; exists {
        opA1Input = applyPatch(opA1Input, patch)
    }
    
    err := workflow.ExecuteActivity(ctx, OpA1Activity, opA1Input).Get(ctx, &opA1Output)
    if err != nil {
        return nil, err
    }
    
    // Op A2 - Child workflow (natural restart boundary)
    var opA2Output OpA2Output
    opA2Input := prepareChildInput(opA1Output)
    
    // Pass relevant patches to child
    childPatches := filterPatchesWithPrefix(patches, "opA2.")
    
    err = workflow.ExecuteChildWorkflow(
        ctx,
        OpA2Workflow,
        OpA2WorkflowInput{
            Data:    opA2Input,
            Patches: childPatches, // Child handles its own patches
        },
    ).Get(ctx, &opA2Output)
    if err != nil {
        return nil, err
    }
    
    // Op A3 - Uses child output
    var opA3Output OpA3Output
    opA3Input := OpA3Input{FromChild: opA2Output.Result}
    if patch, exists := patches["opA3"]; exists {
        opA3Input = applyPatch(opA3Input, patch)
    }
    
    err = workflow.ExecuteActivity(ctx, OpA3Activity, opA3Input).Get(ctx, &opA3Output)
    if err != nil {
        return nil, err
    }
    
    return &RootWorkflowOutput{
        OpA1Result: opA1Output,
        OpA2Result: opA2Output,
        OpA3Result: opA3Output,
    }, nil
}

// Child workflows follow the same pattern
func OpA2Workflow(ctx workflow.Context, input OpA2WorkflowInput) (*OpA2WorkflowOutput, error) {
    patches := input.Patches
    
    // Internal step 1
    var step1Output Step1Output
    step1Input := input.Data.Step1Input
    if patch, exists := patches["step1"]; exists {
        step1Input = applyPatch(step1Input, patch)
    }
    
    err := workflow.ExecuteActivity(ctx, Step1Activity, step1Input).Get(ctx, &step1Output)
    if err != nil {
        return nil, err
    }
    
    // Internal step 2
    var step2Output Step2Output
    step2Input := Step2Input{FromStep1: step1Output.Result}
    if patch, exists := patches["step2"]; exists {
        step2Input = applyPatch(step2Input, patch)
    }
    
    err = workflow.ExecuteActivity(ctx, Step2Activity, step2Input).Get(ctx, &step2Output)
    if err != nil {
        return nil, err
    }
    
    return &OpA2WorkflowOutput{Result: step2Output.Data}, nil
}
```

### Restart Implementation

```go
func RestartWorkflow(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    temporalClient := client.Get()
    
    // Always reset the root workflow
    // Temporal will replay everything up to the reset point
    
    // Get the original input to preserve it
    originalInput, err := getOriginalWorkflowInput(ctx, req.WorkflowID, req.RunID)
    if err != nil {
        return nil, err
    }
    
    // Find where to reset based on the op path
    resetEventID, err := findResetPoint(ctx, req.WorkflowID, req.RunID, req.RestartPoint)
    if err != nil {
        return nil, err
    }
    
    // Reset the workflow
    resetResp, err := temporalClient.ResetWorkflowExecution(ctx, &workflowservice.ResetWorkflowExecutionRequest{
        Namespace: "default",
        WorkflowExecution: &common.WorkflowExecution{
            WorkflowId: req.WorkflowID,
            RunId:      req.RunID,
        },
        Reason:                    fmt.Sprintf("Restart at %s: %s", req.RestartPoint, req.Reason),
        WorkflowTaskFinishEventId: resetEventID,
        RequestId:                 uuid.New().String(),
    })
    if err != nil {
        return nil, err
    }
    
    // The reset creates a new run that continues from the reset point
    // We need to signal it with the patches
    err = temporalClient.SignalWorkflow(ctx, req.WorkflowID, resetResp.RunId, "apply-patches", req.Patches)
    if err != nil {
        return nil, err
    }
    
    return &RestartResponse{
        WorkflowID:  req.WorkflowID,
        RunID:       resetResp.RunId,
        RestartedAt: time.Now(),
    }, nil
}
```

### Even Simpler: Workflow Versioning

For patches that change behavior, use Temporal's versioning:

```go
func VersionedWorkflow(ctx workflow.Context, input WorkflowInput) (*WorkflowOutput, error) {
    // Version for handling patches
    v := workflow.GetVersion(ctx, "patch-v1", workflow.DefaultVersion, 1)
    
    var opA1Output OpA1Output
    
    if v == workflow.DefaultVersion {
        // Original behavior
        err := workflow.ExecuteActivity(ctx, OpA1Activity, input.OpA1Input).Get(ctx, &opA1Output)
        if err != nil {
            return nil, err
        }
    } else {
        // Patched behavior (v1)
        patchedInput := applyHardcodedPatch(input.OpA1Input)
        err := workflow.ExecuteActivity(ctx, OpA1Activity, patchedInput).Get(ctx, &opA1Output)
        if err != nil {
            return nil, err
        }
    }
    
    // Continue with rest of workflow...
}
```

## Why This Works Better

### 1. Natural Boundaries with Child Workflows

```go
// Each recipe op that's complex becomes a child workflow
func RecipeWorkflow(ctx workflow.Context, input RecipeInput) (*RecipeOutput, error) {
    // Simple ops are activities
    var prepOutput PrepOutput
    err := workflow.ExecuteActivity(ctx, PrepActivity, input.PrepInput).Get(ctx, &prepOutput)
    
    // Complex ops are child workflows (natural restart boundaries)
    var processOutput ProcessOutput
    err = workflow.ExecuteChildWorkflow(ctx, ProcessWorkflow, prepOutput).Get(ctx, &processOutput)
    
    // State machines are child workflows too
    var stateOutput StateOutput
    err = workflow.ExecuteChildWorkflow(ctx, StateMachineWorkflow, processOutput).Get(ctx, &stateOutput)
    
    return &RecipeOutput{Result: stateOutput.Final}, nil
}
```

### 2. Restart Simply Resets Root

When user wants to restart at `recipe.process.step2`:
1. Reset root workflow to before ProcessWorkflow child
2. ProcessWorkflow re-executes with patches
3. Temporal handles all the replay logic

### 3. No Custom Code Needed

- No checkpoint storage (Temporal has history)
- No query handlers (patches are just inputs)
- No memo complexity (use signals or inputs)
- No patch merging (each run is independent)

## Handling Cumulative Patches

If we need cumulative patches, use Temporal's built-in patterns:

```go
func WorkflowWithSignals(ctx workflow.Context, input WorkflowInput) (*WorkflowOutput, error) {
    // Patches accumulate via signals
    var patches map[string]Patch
    patchChannel := workflow.GetSignalChannel(ctx, "apply-patches")
    
    // Non-blocking receive of patches
    patchChannel.Receive(ctx, &patches)
    
    // Continue with patched execution
    // ...
}
```

Or use Continue-As-New:

```go
func RestartableWorkflow(ctx workflow.Context, input RestartableInput) error {
    // After reset, continue with new input that includes patches
    if input.IsRestart {
        // Apply patches and continue
        input = applyPatches(input, input.Patches)
    }
    
    // Execute workflow...
    
    // If we need to restart again, use continue-as-new
    if needsAnotherRestart {
        return workflow.NewContinueAsNewError(ctx, RestartableWorkflow, newInput)
    }
    
    return nil
}
```

## Recipe Compiler Strategy

The compiler should:

1. **Generate child workflows for complex ops** - Natural restart boundaries
2. **Accept patches as normal inputs** - No special handling
3. **Use workflow versioning for behavioral changes** - Temporal's native pattern

Example compiler output:

```go
// User writes: recipe with ops [prep, process, validate]
// Compiler generates:

func RecipeWorkflow(ctx workflow.Context, input RecipeInput) (*RecipeOutput, error) {
    // Patches come from input (populated on restart)
    patches := input.Patches
    
    // Each op can be activity or child workflow
    for _, op := range []string{"prep", "process", "validate"} {
        switch op {
        case "prep":
            // Simple op -> activity
            var prepOut PrepOutput
            prepIn := input.PrepInput
            if p, ok := patches[op]; ok {
                prepIn = applyPatch(prepIn, p)
            }
            err := workflow.ExecuteActivity(ctx, PrepActivity, prepIn).Get(ctx, &prepOut)
            
        case "process":
            // Complex op -> child workflow
            var processOut ProcessOutput
            processIn := ProcessInput{Data: prepOut}
            if p, ok := patches[op]; ok {
                processIn.Patches = p // Pass to child
            }
            err := workflow.ExecuteChildWorkflow(ctx, ProcessWorkflow, processIn).Get(ctx, &processOut)
            
        case "validate":
            // Another activity
            var validateOut ValidateOutput
            validateIn := ValidateInput{Data: processOut}
            if p, ok := patches[op]; ok {
                validateIn = applyPatch(validateIn, p)
            }
            err := workflow.ExecuteActivity(ctx, ValidateActivity, validateIn).Get(ctx, &validateOut)
        }
    }
    
    return &RecipeOutput{...}, nil
}
```

## Benefits

1. **Minimal custom code** - Leverage Temporal's native patterns
2. **Natural restart boundaries** - Child workflows are designed for this
3. **Simple mental model** - Reset root, pass patches as inputs
4. **Works with Temporal tools** - tctl, UI, etc. all work normally
5. **No state management** - Temporal handles everything

## Success Criteria

- [ ] Workflows restart using Temporal's reset API
- [ ] Patches passed as normal workflow inputs
- [ ] Child workflows create natural restart boundaries
- [ ] No custom checkpoint or state management
- [ ] Works with standard Temporal tooling
- [ ] Compiler generates restart-friendly workflows