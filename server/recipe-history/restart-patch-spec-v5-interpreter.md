# Recipe Restart and Patching Specification (v5 - Interpreter Pattern)

## Overview

The recipe-worker interprets recipe definitions at runtime and executes them as Temporal workflows. This spec defines how the interpreter handles restarts and patches using Temporal's native capabilities.

## Core Architecture

### Recipe Interpreter Pattern

```go
// The recipe-worker has a generic workflow that interprets any recipe
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    logger := workflow.GetLogger(ctx)
    
    // Load the recipe definition
    recipe := input.Recipe
    
    // Patches passed as input (nil on first run, populated on restart)
    patches := input.Patches
    
    // Execute context maintains state across ops
    execCtx := &ExecutionContext{
        Inputs:  input.Inputs,
        Outputs: make(map[string]interface{}),
        Patches: patches,
    }
    
    // Interpret and execute each op in the recipe
    for _, op := range recipe.Ops {
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

// Generic op executor that handles different op types
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
        // Composition ops
        return executeComposition(ctx, op, opInput, execCtx)
        
    case "conditional":
        // Conditional execution (includes state machines)
        return executeConditional(ctx, op, opInput, execCtx)
        
    default:
        return nil, fmt.Errorf("unknown op type: %s", op.Type)
    }
}
```

### Nested Recipe Execution

Complex ops and nested recipes become child workflows:

```go
func executeNestedRecipe(ctx workflow.Context, op OpDefinition, input interface{}, parentPatches map[string]Patch) (interface{}, error) {
    // Extract patches for this nested recipe
    nestedPatches := extractNestedPatches(parentPatches, op.ID)
    
    // Execute nested recipe as child workflow
    var output interface{}
    err := workflow.ExecuteChildWorkflow(
        workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
            WorkflowID: fmt.Sprintf("%s-%s", workflow.GetInfo(ctx).WorkflowExecution.ID, op.ID),
        }),
        RecipeInterpreterWorkflow,
        RecipeWorkflowInput{
            Recipe:  op.NestedRecipe,
            Inputs:  input,
            Patches: nestedPatches,
        },
    ).Get(ctx, &output)
    
    return output, err
}

func extractNestedPatches(patches map[string]Patch, prefix string) map[string]Patch {
    // Extract patches that apply to nested ops
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

### State Machine Interpretation

States are just conditional ops with transitions:

```go
func executeStateMachine(ctx workflow.Context, stateMachine StateMachineDefinition, execCtx *ExecutionContext) (interface{}, error) {
    currentState := stateMachine.InitialState
    stateData := make(map[string]interface{})
    
    for currentState != stateMachine.FinalState {
        state := stateMachine.States[currentState]
        
        // Execute state op
        stateInput := map[string]interface{}{
            "state_data": stateData,
            "inputs":     execCtx.Inputs,
        }
        
        // Apply patches to state if present
        statePath := fmt.Sprintf("state.%s", currentState)
        if patch, exists := execCtx.Patches[statePath]; exists {
            stateInput = applyPatch(stateInput, patch)
        }
        
        stateOutput, err := executeOp(ctx, state.Op, &ExecutionContext{
            Inputs:  stateInput,
            Outputs: execCtx.Outputs,
            Patches: execCtx.Patches,
        })
        if err != nil {
            return nil, err
        }
        
        // Update state data
        if outputMap, ok := stateOutput.(map[string]interface{}); ok {
            for k, v := range outputMap {
                stateData[k] = v
            }
        }
        
        // Evaluate transitions
        currentState = evaluateTransitions(state.Transitions, stateData)
    }
    
    return stateData, nil
}
```

## Restart Implementation

### Recipe Worker Restart Handler

```go
type RestartService struct {
    temporalClient client.Client
    recipeStore    RecipeStore
}

func (s *RestartService) RestartRecipeExecution(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    // The restart point is an op path like "recipe.dataProcessor.step2"
    
    // 1. Get the original recipe and inputs
    originalExecution, err := s.getOriginalExecution(ctx, req.WorkflowID, req.RunID)
    if err != nil {
        return nil, err
    }
    
    // 2. Merge patches with any existing patches (for cumulative restarts)
    cumulativePatches := s.mergePatches(originalExecution.Patches, req.Patches, req.RestartPoint)
    
    // 3. Find the Temporal event to reset to
    // We need to reset BEFORE any point where patches need to be applied
    resetEventID, err := s.findResetPoint(ctx, req.WorkflowID, req.RunID, req.RestartPoint, cumulativePatches)
    if err != nil {
        return nil, err
    }
    
    // 4. Create new workflow input with patches
    newInput := RecipeWorkflowInput{
        Recipe:  originalExecution.Recipe,
        Inputs:  originalExecution.Inputs,
        Patches: cumulativePatches,  // Patches passed as workflow input!
    }
    
    // 5. Reset and restart with new input
    // Note: Temporal's reset creates a new run that continues from the reset point
    // but we need to ensure the workflow has the patches from the start
    resetResp, err := s.temporalClient.ResetWorkflowExecution(ctx, &workflowservice.ResetWorkflowExecutionRequest{
        Namespace: "default",
        WorkflowExecution: &common.WorkflowExecution{
            WorkflowId: req.WorkflowID,
            RunId:      req.RunID,
        },
        Reason:                    fmt.Sprintf("Restart at %s: %s", req.RestartPoint, req.Reason),
        WorkflowTaskFinishEventId: resetEventID,
        // The key insight: Reset replays with original input
        // So we need to restart the entire workflow with new input containing patches
    })
    if err != nil {
        return nil, err
    }
    
    // 6. Since reset replays with original input, we actually need to start a new workflow
    // with patches included from the beginning
    // OR use workflow versioning to check for patches in memo
    
    // Alternative approach: Store patches in a way the workflow can retrieve them
    // during replay (e.g., in workflow memo which persists across reset)
    
    return &RestartResponse{
        WorkflowID:  req.WorkflowID,
        RunID:       resetResp.RunId,
        RestartedAt: time.Now(),
        Note:        "Patches embedded in workflow for replay",
    }, nil
}

func (s *RestartService) findResetPoint(ctx context.Context, workflowID, runID, opPath string) (int64, error) {
    // Map op path to Temporal history event
    // This requires understanding how the interpreter maps ops to activities/child workflows
    
    // Get workflow history
    iter := s.temporalClient.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
    
    // Parse op path (e.g., "recipe.dataProcessor.step2")
    pathParts := strings.Split(opPath, ".")
    
    // Find the event that corresponds to this op
    // For nested ops, we need to find the parent child workflow start
    targetOpID := pathParts[len(pathParts)-1]
    
    for iter.HasNext() {
        event, err := iter.Next()
        if err != nil {
            return 0, err
        }
        
        // Look for activity or child workflow that matches our op
        switch event.GetEventType() {
        case enumspb.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
            attrs := event.GetActivityTaskScheduledEventAttributes()
            if attrs.ActivityId == targetOpID {
                // Find the workflow task completed event before this
                return findPrecedingWorkflowTask(event.EventId), nil
            }
            
        case enumspb.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED:
            attrs := event.GetStartChildWorkflowExecutionInitiatedEventAttributes()
            // Check if this matches our nested op
            if strings.Contains(attrs.WorkflowId, targetOpID) {
                return findPrecedingWorkflowTask(event.EventId), nil
            }
        }
    }
    
    return 0, fmt.Errorf("op %s not found in workflow history", opPath)
}
```

### Key Insight: Reset with Patches

The critical insight is that **Temporal's reset replays with the original workflow input**. However, we have a few options:

#### Option 1: Start New Workflow (Clearest)
```go
func (s *RestartService) RestartRecipeExecution(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    // Get original execution
    original, err := s.getOriginalExecution(ctx, req.WorkflowID, req.RunID)
    if err != nil {
        return nil, err
    }
    
    // Start completely new workflow with patches
    newWorkflowID := fmt.Sprintf("%s-restart-%d", req.WorkflowID, time.Now().Unix())
    
    we, err := s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
        ID:        newWorkflowID,
        TaskQueue: original.TaskQueue,
    }, RecipeInterpreterWorkflow, RecipeWorkflowInput{
        Recipe:  original.Recipe,
        Inputs:  original.Inputs,
        Patches: req.Patches,  // Patches in input from the start
    })
    
    return &RestartResponse{
        WorkflowID: newWorkflowID,
        RunID:      we.GetRunID(),
    }, nil
}
```

#### Option 2: Reset with Updated Memo (THIS WORKS!)
```go  
func (s *RestartService) RestartWithMemo(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    // KEY INSIGHT: Temporal's reset API allows updating the memo!
    // The memo is NOT part of the deterministic execution, so it can be changed
    
    resetResp, err := s.temporalClient.ResetWorkflowExecution(ctx, &workflowservice.ResetWorkflowExecutionRequest{
        Namespace: "default",
        WorkflowExecution: &common.WorkflowExecution{
            WorkflowId: req.WorkflowID,
            RunId:      req.RunID,
        },
        WorkflowTaskFinishEventId: resetEventID,
        // This REPLACES the memo during reset!
        Memo: &common.Memo{
            Fields: map[string]*common.Payload{
                "patches": encodePayload(req.Patches),
                "restart_metadata": encodePayload(map[string]interface{}{
                    "restart_point": req.RestartPoint,
                    "restart_time":  time.Now(),
                    "reason":        req.Reason,
                }),
            },
        },
    })
    
    return &RestartResponse{
        WorkflowID: req.WorkflowID,
        RunID:      resetResp.RunId,
    }, nil
}

// Workflow checks memo for patches
func RecipeInterpreterWorkflow(ctx workflow.Context, input RecipeWorkflowInput) (*RecipeWorkflowOutput, error) {
    // Patches can come from input OR memo (for reset case)
    patches := input.Patches
    if patches == nil {
        // Check memo for patches (persists across reset)
        info := workflow.GetInfo(ctx)
        if memoPatches := getPatchesFromMemo(info.Memo); memoPatches != nil {
            patches = memoPatches
        }
    }
    
    // Execute with patches...
    execCtx := &ExecutionContext{
        Inputs:  input.Inputs,
        Outputs: make(map[string]interface{}),
        Patches: patches,
    }
    
    // No signals needed - patches are static data!
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

## Op Path Registry

The recipe-worker maintains a registry of op paths for restart resolution:

```go
// Built when recipe is loaded
type OpRegistry struct {
    paths map[string]OpMetadata
}

type OpMetadata struct {
    RecipeID   string
    OpID       string
    Type       string   // activity, workflow, sequential, parallel, conditional
    ParentPath string   // For nested ops
    Children   []string // For composition ops
}

// Build registry when interpreting recipe
func buildOpRegistry(recipe Recipe, parentPath string) map[string]OpMetadata {
    registry := make(map[string]OpMetadata)
    
    for _, op := range recipe.Ops {
        opPath := fmt.Sprintf("%s.%s", parentPath, op.ID)
        
        registry[opPath] = OpMetadata{
            RecipeID:   recipe.ID,
            OpID:       op.ID,
            Type:       op.Type,
            ParentPath: parentPath,
        }
        
        // Register nested ops
        if op.Type == "workflow" && op.NestedRecipe != nil {
            nestedRegistry := buildOpRegistry(*op.NestedRecipe, opPath)
            for path, meta := range nestedRegistry {
                registry[path] = meta
            }
        }
        
        // Register composition children
        if op.Type == "sequential" || op.Type == "parallel" {
            for _, childOp := range op.Steps {
                childPath := fmt.Sprintf("%s.%s", opPath, childOp.ID)
                registry[childPath] = OpMetadata{
                    OpID:       childOp.ID,
                    Type:       childOp.Type,
                    ParentPath: opPath,
                }
            }
        }
    }
    
    return registry
}
```

## Example: Restart with Nested Recipe

### Recipe Definition
```yaml
id: data-pipeline
ops:
  - id: fetch
    type: activity
    config:
      activity: FetchDataActivity
      
  - id: process
    type: workflow  # Nested recipe
    config:
      recipe:
        id: data-processor
        ops:
          - id: validate
            type: activity
            config:
              activity: ValidateActivity
              
          - id: transform
            type: activity
            config:
              activity: TransformActivity
              
          - id: enrich
            type: activity
            config:
              activity: EnrichActivity
              
  - id: store
    type: activity
    config:
      activity: StoreActivity
```

### Restart Scenarios

**Scenario 1: Restart at nested op**
```bash
vibethis recipe restart \
  --workflow-id pipeline-123 \
  --run-id run-456 \
  --restart-point "data-pipeline.process.transform" \
  --patch '{"algorithm": "v2"}'
```

The interpreter:
1. Resets root workflow to before "process" child workflow
2. Process child workflow re-executes from beginning
3. Transform op gets patched input when executed
4. Store op re-executes with new process output

**Scenario 2: Restart at root-level op**
```bash
vibethis recipe restart \
  --workflow-id pipeline-123 \
  --run-id run-456 \
  --restart-point "data-pipeline.store" \
  --patch '{"batch_size": 1000}'
```

The interpreter:
1. Resets root workflow to before "store" activity
2. Store activity executes with patched input
3. Previous ops (fetch, process) outputs preserved from history

## Summary: The Simple Truth

After exploring options, here's what actually works with Temporal's reset:

1. **You CANNOT change workflow input during reset** - Reset replays with original input
2. **You CAN update the memo during reset** - Memo is not part of deterministic execution
3. **Best approach: Reset with updated memo** containing patches
4. **Child workflows automatically get relevant patches** - Parent filters and passes them
5. **No custom restart code in children** - They just execute with patches if provided

### Why Option 2 (Reset with Memo) is Best:

- **Keeps same workflow ID** - Better for tracking and UI
- **Temporal-native** - Uses reset as intended
- **Memo survives reset** - Perfect for patches
- **No new workflow needed** - Simpler operationally

The key insight: **We don't need any custom code to handle restarts within child workflows**. If the parent always passes the relevant patches when spawning children, everything just works:

```go
// Parent always passes patches to children
err = workflow.ExecuteChildWorkflow(
    ctx,
    RecipeInterpreterWorkflow,
    RecipeWorkflowInput{
        Recipe:  op.NestedRecipe,
        Inputs:  input,
        Patches: extractNestedPatches(execCtx.Patches, op.ID), // Always pass relevant patches!
    },
).Get(ctx, &output)
```

When we reset and replay:
- Parent workflow replays with patches (from input or memo)
- Parent passes patches to child when spawning it
- Child executes normally with patches
- No special restart handling needed anywhere!

## Benefits of Interpreter Pattern

1. **Single generic workflow** - One interpreter handles all recipes
2. **Dynamic op resolution** - Op paths resolved at runtime
3. **Natural nesting** - Nested recipes are just child workflows
4. **Simple patching** - Patches flow through workflow inputs
5. **Temporal-native** - Uses reset, memo, child workflows
6. **No signals** - Patches are static data, passed as input

## Success Criteria

- [ ] Recipe interpreter handles restarts via Temporal reset
- [ ] Patches passed through execution context
- [ ] Nested recipes create child workflow boundaries
- [ ] Op registry maps paths to Temporal events
- [ ] State machines handled as conditional ops
- [ ] No custom checkpointing needed
- [ ] Works with standard Temporal tooling