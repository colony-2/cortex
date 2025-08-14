# Recipe Restart and Patching Specification (v2 - Temporal-Native)

## Overview

This specification defines how vibethis leverages Temporal's native replay, versioning, and workflow reset capabilities to enable restart and patching of recipe executions. Instead of building custom checkpointing, we use Temporal's built-in features and focus only on the vibethis-specific abstractions.

## Key Insight

Temporal already provides:
- **Deterministic replay** - Workflows automatically replay from history
- **Event history** - Complete record of all activity executions
- **Workflow reset** - Native ability to reset workflows to any point
- **Versioning** - Built-in workflow versioning for non-breaking changes
- **Query handlers** - Ability to query workflow state
- **Search attributes** - Indexed metadata for workflow searching
- **Continue-as-new** - Ability to continue workflows with new inputs

We should leverage these instead of rebuilding them.

## Problem Statement

Users need to:
1. Restart recipe execution from any op with modified inputs
2. Apply patches without breaking deterministic replay
3. Maintain audit trail of patches applied
4. Handle vibethis-specific concepts (nested compositions, CEL state machines)

## Architecture

### Temporal-Native Approach

Instead of custom checkpointing, we use Temporal's capabilities:

1. **Workflow Reset** for restart functionality
2. **Workflow Versioning** for applying patches
3. **Search Attributes** for indexing op paths
4. **Query Handlers** for inspecting state
5. **Continue-as-new** for large state machines

### Compiler Strategy

The recipe compiler generates Temporal workflows that are restart-friendly:

```go
// Generated workflow with versioning for patches
func WorkflowA(ctx workflow.Context, input WorkflowAInput) (*WorkflowAOutput, error) {
    logger := workflow.GetLogger(ctx)
    
    // Version the workflow for potential patches
    v := workflow.GetVersion(ctx, "workflow-version", workflow.DefaultVersion, 1)
    
    // Op A1 - Direct Temporal activity execution
    var opA1Output OpA1Output
    opA1Input := input.OpA1Input
    
    // Check for patches via side effect (deterministic)
    var patchedInputA1 OpA1Input
    workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
        // This runs only on replay, can check external patch store
        if patch := getPatchForOp("workflowA.opA1"); patch != nil {
            return applyPatch(opA1Input, patch)
        }
        return opA1Input
    }).Get(&patchedInputA1)
    
    // Execute with Temporal's native activity tracking
    err := workflow.ExecuteActivity(
        workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
            StartToCloseTimeout: 10 * time.Minute,
            ActivityID:          "opA1", // Stable ID for reset
        }),
        OpA1Activity,
        patchedInputA1,
    ).Get(ctx, &opA1Output)
    if err != nil {
        return nil, err
    }
    
    // Op A2
    var opA2Output OpA2Output
    err = workflow.ExecuteActivity(
        workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
            ActivityID: "opA2",
        }),
        OpA2Activity,
        input.OpA2Input,
    ).Get(ctx, &opA2Output)
    if err != nil {
        return nil, err
    }
    
    // Child Workflow B - with deterministic ID
    var wfBOutput WorkflowBOutput
    err = workflow.ExecuteChildWorkflow(
        workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
            WorkflowID: fmt.Sprintf("%s-workflowB", workflow.GetInfo(ctx).WorkflowExecution.ID),
        }),
        WorkflowB,
        prepareChildInput(opA2Output),
    ).Get(ctx, &wfBOutput)
    if err != nil {
        return nil, err
    }
    
    // Op A3 - uses output from workflow B
    var opA3Output OpA3Output
    err = workflow.ExecuteActivity(
        workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
            ActivityID: "opA3",
        }),
        OpA3Activity,
        OpA3Input{FromWorkflowB: wfBOutput.Result},
    ).Get(ctx, &opA3Output)
    
    return &WorkflowAOutput{
        OpA1Result: opA1Output,
        OpA3Result: opA3Output,
        WfBResult:  wfBOutput,
    }, nil
}

// Query handler for workflow inspection
func WorkflowAQueries(ctx workflow.Context) {
    // Register query to get execution state
    workflow.SetQueryHandler(ctx, "execution-state", func() (ExecutionState, error) {
        return ExecutionState{
            CompletedOps: getCompletedOps(ctx),
            CurrentOp:    getCurrentOp(ctx),
            Outputs:      getOutputs(ctx),
        }, nil
    })
    
    // Register query to get op paths
    workflow.SetQueryHandler(ctx, "op-paths", func() ([]string, error) {
        return []string{
            "workflowA.opA1",
            "workflowA.opA2",
            "workflowA.workflowB",
            "workflowA.workflowB.opB1",
            "workflowA.workflowB.opB2",
            "workflowA.opA3",
        }, nil
    })
}
```

## Restart Implementation

### Using Temporal Reset API

Instead of custom checkpointing, use Temporal's Reset API:

```go
func RestartWorkflow(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    temporalClient := client.Get()
    
    // 1. Query the workflow to get its current state
    response, err := temporalClient.QueryWorkflow(ctx, req.WorkflowID, req.RunID, "execution-state")
    if err != nil {
        return nil, err
    }
    
    var state ExecutionState
    if err := response.Get(&state); err != nil {
        return nil, err
    }
    
    // 2. Find the event ID for the restart point
    eventID, err := findEventIDForOp(ctx, req.WorkflowID, req.RunID, req.RestartPoint)
    if err != nil {
        return nil, err
    }
    
    // 3. Store patches in external store (accessed via SideEffect)
    if len(req.Patches) > 0 {
        err = storePatchesForWorkflow(req.WorkflowID, req.Patches)
        if err != nil {
            return nil, err
        }
    }
    
    // 4. Use Temporal's Reset API
    resetResp, err := temporalClient.ResetWorkflowExecution(ctx, &workflowservice.ResetWorkflowExecutionRequest{
        Namespace: "default",
        WorkflowExecution: &common.WorkflowExecution{
            WorkflowId: req.WorkflowID,
            RunId:      req.RunID,
        },
        Reason:                    fmt.Sprintf("User restart: %s", req.Reason),
        WorkflowTaskFinishEventId: eventID,
        RequestId:                 uuid.New().String(),
    })
    if err != nil {
        return nil, err
    }
    
    return &RestartResponse{
        WorkflowID:  req.WorkflowID,
        RunID:       resetResp.RunId,
        RestartedAt: time.Now(),
    }, nil
}

func findEventIDForOp(ctx context.Context, workflowID, runID, opPath string) (int64, error) {
    // Parse op path to determine activity ID
    activityID := extractActivityID(opPath)
    
    // Get workflow history
    iter := temporalClient.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
    
    for iter.HasNext() {
        event, err := iter.Next()
        if err != nil {
            return 0, err
        }
        
        // Find the ActivityTaskCompleted event for our op
        if event.GetEventType() == enumspb.EVENT_TYPE_ACTIVITY_TASK_COMPLETED {
            attrs := event.GetActivityTaskCompletedEventAttributes()
            scheduledEvent := getEventByID(attrs.ScheduledEventId)
            scheduledAttrs := scheduledEvent.GetActivityTaskScheduledEventAttributes()
            
            if scheduledAttrs.ActivityId == activityID {
                // Return the workflow task completed event right before this activity
                return findPrecedingWorkflowTaskCompleted(event.EventId), nil
            }
        }
    }
    
    return 0, fmt.Errorf("op %s not found in workflow history", opPath)
}
```

### Patch Application via SideEffect

Patches are applied deterministically using SideEffect:

```go
// Patch storage (external, like Redis or DB)
type PatchStore interface {
    GetPatch(workflowID, opPath string) *Patch
    StorePatch(workflowID, opPath string, patch *Patch) error
    ClearPatches(workflowID string) error
}

// In the workflow, patches are applied via SideEffect
func applyPatchIfExists(ctx workflow.Context, workflowID, opPath string, originalInput interface{}) interface{} {
    var patchedInput interface{}
    
    workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
        // This is deterministic - same patch always returned for same workflow run
        patch := patchStore.GetPatch(workflowID, opPath)
        if patch == nil {
            return originalInput
        }
        
        // Apply patch based on merge strategy
        switch patch.MergeStrategy {
        case "replace":
            return patch.InputOverrides
        case "shallow":
            return shallowMerge(originalInput, patch.InputOverrides)
        case "deep":
            return deepMerge(originalInput, patch.InputOverrides)
        default:
            return originalInput
        }
    }).Get(&patchedInput)
    
    return patchedInput
}
```

## State Machine Handling

For CEL state machines, use Temporal's Continue-as-New:

```go
func StateMachineWorkflow(ctx workflow.Context, input StateMachineInput) (*StateMachineOutput, error) {
    // Use continue-as-new for long-running state machines
    const maxTransitions = 100
    
    currentState := input.CurrentState
    if currentState == "" {
        currentState = "init"
    }
    
    stateData := input.StateData
    if stateData == nil {
        stateData = make(map[string]interface{})
    }
    
    transitionCount := input.TransitionCount
    
    for currentState != "end" && transitionCount < maxTransitions {
        stateConfig := getStateConfig(currentState)
        
        // Execute state op
        var stateOutput StateOutput
        err := workflow.ExecuteActivity(
            workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
                ActivityID: fmt.Sprintf("state-%s", currentState),
            }),
            stateConfig.Activity,
            StateInput{Data: stateData},
        ).Get(ctx, &stateOutput)
        if err != nil {
            return nil, err
        }
        
        // Merge state data
        stateData = mergeStateData(stateData, stateOutput.Data)
        
        // Evaluate next state
        currentState = evaluateTransition(stateConfig.Transitions, stateData)
        transitionCount++
    }
    
    // Continue-as-new if we hit the limit
    if transitionCount >= maxTransitions && currentState != "end" {
        return nil, workflow.NewContinueAsNewError(
            ctx,
            StateMachineWorkflow,
            StateMachineInput{
                CurrentState:    currentState,
                StateData:       stateData,
                TransitionCount: transitionCount,
            },
        )
    }
    
    return &StateMachineOutput{
        FinalState: currentState,
        Data:       stateData,
    }, nil
}
```

## Visibility and Search

Use Temporal's Search Attributes for indexing:

```go
func WorkflowWithSearchAttributes(ctx workflow.Context, input WorkflowInput) error {
    // Set search attributes for visibility
    workflow.UpsertSearchAttributes(ctx, map[string]interface{}{
        "RecipeType":   input.RecipeType,
        "CurrentOp":    "opA1",
        "ExecutionPath": "workflowA",
    })
    
    // Update as execution progresses
    err := workflow.ExecuteActivity(ctx, OpA1Activity, input).Get(ctx, nil)
    if err != nil {
        return err
    }
    
    workflow.UpsertSearchAttributes(ctx, map[string]interface{}{
        "CurrentOp":    "opA2",
        "CompletedOps": []string{"opA1"},
    })
    
    // ... continue execution
}
```

## Audit Trail

Use Temporal's built-in history and add a lightweight audit service:

```go
type AuditService struct {
    db *sql.DB
}

func (a *AuditService) RecordRestart(ctx context.Context, restart RestartEvent) error {
    // Store restart metadata (patches, reason, user)
    // Temporal already has the execution history
    _, err := a.db.ExecContext(ctx, `
        INSERT INTO restart_audit (
            workflow_id, original_run_id, new_run_id,
            restart_point, patches, reason, initiated_by, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `, restart.WorkflowID, restart.OriginalRunID, restart.NewRunID,
       restart.RestartPoint, restart.Patches, restart.Reason,
       restart.InitiatedBy, time.Now())
    return err
}

func (a *AuditService) GetRestartHistory(workflowID string) ([]RestartEvent, error) {
    // Query restart history
    rows, err := a.db.Query(`
        SELECT * FROM restart_audit 
        WHERE workflow_id = ? 
        ORDER BY created_at DESC
    `, workflowID)
    // ... parse and return
}
```

## Key Differences from V1

### What We DON'T Need to Build

1. **Checkpoint Storage** - Temporal stores all activity results in history
2. **Execution Tracking** - Temporal tracks what has executed
3. **Replay Logic** - Temporal handles deterministic replay
4. **Skip/Execute Decision** - Temporal's reset handles this
5. **Output Preservation** - Temporal preserves outputs in history

### What We DO Need to Build

1. **Patch Store** - External store for input modifications
2. **Op Path Mapping** - Map vibethis op paths to Temporal events
3. **Restart API** - Wrapper around Temporal's reset
4. **Audit Service** - Track restart reasons and patches
5. **Query Handlers** - Expose vibethis-specific state

## Implementation Plan

### Phase 1: Core Restart
- Implement patch store (Redis/DB)
- Create restart API using Temporal reset
- Add SideEffect-based patching to generated workflows

### Phase 2: Visibility
- Add search attributes to workflows
- Implement query handlers
- Create audit service

### Phase 3: Advanced Features
- Continue-as-new for state machines
- Nested workflow handling
- Batch restart operations

## Benefits of This Approach

1. **Less Code** - Leverage Temporal instead of rebuilding
2. **Battle-tested** - Temporal's replay is production-proven
3. **Native Integration** - Works with Temporal UI and tctl
4. **Performance** - No custom checkpoint overhead
5. **Compatibility** - Works with existing Temporal tooling

## Example Usage

### Simple Restart

```bash
# Reset workflow to before opB2 with patches
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.workflowB.opB2" \
  --patch '{"processing_config": {"threshold": 0.8}}'

# Behind the scenes:
# 1. Find opB2 in Temporal history
# 2. Store patch in external store
# 3. Call Temporal Reset API
# 4. Workflow replays with patch applied via SideEffect
```

### Query Workflow State

```bash
# Get current execution state
tctl workflow query \
  --workflow-id wf-123 \
  --query-type execution-state

# Returns:
{
  "completed_ops": ["opA1", "opA2", "opB1"],
  "current_op": "opB2",
  "outputs": {...}
}
```

### Search Workflows

```bash
# Find workflows at specific op
tctl workflow list \
  --query 'CurrentOp="opB2" AND RecipeType="data-processing"'
```

## Migration from Custom Checkpointing

For existing workflows with custom checkpointing:

1. **Gradual Migration** - New workflows use Temporal-native approach
2. **Compatibility Layer** - Adapter to read old checkpoints if needed
3. **Cleanup** - Remove checkpoint tables after migration

## Testing Strategy

### Unit Tests
- Patch application logic
- Op path to event ID mapping
- Query handler responses

### Integration Tests
- End-to-end restart with Temporal
- Patch application during replay
- Continue-as-new for state machines

### Temporal-Specific Tests
- Deterministic replay with patches
- Reset API integration
- Search attribute updates

## Success Criteria

- [ ] Workflows restart using Temporal's native reset
- [ ] Patches apply deterministically via SideEffect
- [ ] No custom checkpoint storage needed
- [ ] Full visibility through Temporal UI
- [ ] Audit trail captures restart metadata
- [ ] State machines use continue-as-new
- [ ] Compatible with tctl and Temporal tools
- [ ] Reduced code complexity vs. custom solution