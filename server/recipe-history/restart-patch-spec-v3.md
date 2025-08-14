# Recipe Restart and Patching Specification (v3)

## Overview

This specification defines how vibethis leverages Temporal's native workflow reset capability to enable restart and patching of recipe executions. The recipe compiler generates Temporal workflows that support restarting at any op with modified inputs.

## Core Concept

When a user requests a restart with patches:
1. We use Temporal's Reset API to rewind the workflow to a specific point
2. The reset creates a new workflow run that branches from the original
3. Patches are passed as part of the new run's input/context
4. The workflow applies patches during re-execution

## Architecture

### Cumulative Patch Handling

When restarting a previously patched workflow, we need to preserve existing patches and merge them with new ones:

```go
// RestartRequest includes patches directly
type RestartRequest struct {
    WorkflowID   string                    `json:"workflow_id"`
    RunID        string                    `json:"run_id"`
    RestartPoint string                    `json:"restart_point"`  // e.g., "workflowA.workflowB.opB2"
    Patches      map[string]InputPatch     `json:"patches"`
    Reason       string                    `json:"reason"`
}

type InputPatch struct {
    OpPath         string                 `json:"op_path"`
    InputOverrides map[string]interface{} `json:"input_overrides"`
    MergeStrategy  string                 `json:"merge_strategy"` // deep|shallow|replace
}

// Track cumulative patches across restarts
type CumulativePatches struct {
    Patches      map[string]InputPatch `json:"patches"`
    RestartChain []RestartInfo         `json:"restart_chain"`
}

type RestartInfo struct {
    RunID        string    `json:"run_id"`
    RestartPoint string    `json:"restart_point"`
    Timestamp    time.Time `json:"timestamp"`
    Reason       string    `json:"reason"`
}

func RestartWorkflow(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    temporalClient := client.Get()
    
    // Get existing patches from the workflow being reset
    existingPatches, restartChain := getExistingPatches(ctx, req.WorkflowID, req.RunID)
    
    // Merge new patches with existing ones
    cumulativePatches := mergePatchSets(existingPatches, req.Patches, req.RestartPoint)
    
    // Add this restart to the chain
    restartChain = append(restartChain, RestartInfo{
        RunID:        req.RunID,
        RestartPoint: req.RestartPoint,
        Timestamp:    time.Now(),
        Reason:       req.Reason,
    })
    
    // Find the event ID for the restart point
    eventID, err := findEventIDForOp(ctx, req.WorkflowID, req.RunID, req.RestartPoint)
    if err != nil {
        return nil, err
    }
    
    // Reset with cumulative patches
    resetResp, err := temporalClient.ResetWorkflowExecution(ctx, &workflowservice.ResetWorkflowExecutionRequest{
        Namespace: "default",
        WorkflowExecution: &common.WorkflowExecution{
            WorkflowId: req.WorkflowID,
            RunId:      req.RunID,
        },
        Reason:                    fmt.Sprintf("Restart: %s", req.Reason),
        WorkflowTaskFinishEventId: eventID,
        RequestId:                 uuid.New().String(),
        // Pass cumulative patches and restart chain
        ResetReapplyType: enumspb.RESET_REAPPLY_TYPE_SIGNAL,
        Memo: &common.Memo{
            Fields: map[string]*common.Payload{
                "cumulative_patches": encodePayload(CumulativePatches{
                    Patches:      cumulativePatches,
                    RestartChain: restartChain,
                }),
                "restart_point": encodeString(req.RestartPoint),
                "restart_reason": encodeString(req.Reason),
            },
        },
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

func getExistingPatches(ctx context.Context, workflowID, runID string) (map[string]InputPatch, []RestartInfo) {
    // Query the workflow to get its current patches
    response, err := temporalClient.QueryWorkflow(ctx, workflowID, runID, "get-patches")
    if err != nil {
        // First run, no existing patches
        return make(map[string]InputPatch), nil
    }
    
    var cumulative CumulativePatches
    if err := response.Get(&cumulative); err != nil {
        return make(map[string]InputPatch), nil
    }
    
    return cumulative.Patches, cumulative.RestartChain
}

func mergePatchSets(existing, new map[string]InputPatch, restartPoint string) map[string]InputPatch {
    result := make(map[string]InputPatch)
    
    // Copy existing patches
    for opPath, patch := range existing {
        // Only keep patches for ops before the restart point
        if isBeforeRestartPoint(opPath, restartPoint) {
            result[opPath] = patch
        }
    }
    
    // Apply new patches (may override existing ones at or after restart point)
    for opPath, patch := range new {
        if existingPatch, exists := result[opPath]; exists {
            // Merge patches for the same op
            result[opPath] = mergeTwoPatches(existingPatch, patch)
        } else {
            result[opPath] = patch
        }
    }
    
    return result
}

func mergeTwoPatches(existing, new InputPatch) InputPatch {
    // If new patch uses "replace" strategy, it completely overrides
    if new.MergeStrategy == "replace" {
        return new
    }
    
    // Otherwise merge the input overrides
    merged := InputPatch{
        OpPath:        new.OpPath,
        MergeStrategy: new.MergeStrategy,
        InputOverrides: deepMerge(existing.InputOverrides, new.InputOverrides),
    }
    
    return merged
}
```

### Compiler-Generated Workflows

The recipe compiler generates workflows that check for patches in their context:

```go
// Generated workflow code
func WorkflowA(ctx workflow.Context, input WorkflowAInput) (*WorkflowAOutput, error) {
    logger := workflow.GetLogger(ctx)
    
    // Check if this is a reset run with cumulative patches
    info := workflow.GetInfo(ctx)
    cumulative := getCumulativePatchesFromMemo(info.Memo)
    patches := cumulative.Patches
    isReset := len(patches) > 0
    
    // Register query handler to expose patches for future restarts
    workflow.SetQueryHandler(ctx, "get-patches", func() (CumulativePatches, error) {
        return cumulative, nil
    })
    
    // Op A1 - could be a simple op, state transition, or nested workflow
    var opA1Output OpA1Output
    opA1Input := input.OpA1Input
    
    // Apply patch if this op is patched
    if patch, exists := patches["workflowA.opA1"]; exists {
        opA1Input = applyPatch(opA1Input, patch)
        logger.Info("Applied patch to opA1", "patch", patch)
    }
    
    // Execute op (might be activity, child workflow, or state logic)
    err := executeOp(ctx, "opA1", opA1Input, &opA1Output)
    if err != nil {
        return nil, err
    }
    
    // Op A2 - similar pattern
    var opA2Output OpA2Output
    opA2Input := input.OpA2Input
    if patch, exists := patches["workflowA.opA2"]; exists {
        opA2Input = applyPatch(opA2Input, patch)
    }
    err = executeOp(ctx, "opA2", opA2Input, &opA2Output)
    if err != nil {
        return nil, err
    }
    
    // Nested workflow (appears as single op to user)
    var nestedOutput NestedWorkflowOutput
    nestedInput := prepareNestedInput(opA2Output)
    
    // Pass patches to nested workflow
    nestedPatches := filterPatchesForNested(patches, "workflowA.nestedOp")
    
    err = workflow.ExecuteChildWorkflow(
        workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
            WorkflowID: fmt.Sprintf("%s-nested", info.WorkflowExecution.ID),
            Memo: &common.Memo{
                Fields: map[string]*common.Payload{
                    "patches": encodePatches(nestedPatches),
                },
            },
        }),
        NestedWorkflow,
        nestedInput,
    ).Get(ctx, &nestedOutput)
    if err != nil {
        return nil, err
    }
    
    // Op A3 - uses output from nested workflow
    var opA3Output OpA3Output
    opA3Input := OpA3Input{FromNested: nestedOutput.Result}
    if patch, exists := patches["workflowA.opA3"]; exists {
        opA3Input = applyPatch(opA3Input, patch)
    }
    err = executeOp(ctx, "opA3", opA3Input, &opA3Output)
    
    return &WorkflowAOutput{
        OpA1Result: opA1Output,
        OpA3Result: opA3Output,
        NestedResult: nestedOutput,
    }, nil
}

// Helper to execute different op types
func executeOp(ctx workflow.Context, opID string, input, output interface{}) error {
    opConfig := getOpConfig(opID)
    
    switch opConfig.Type {
    case "activity":
        // Simple activity execution
        return workflow.ExecuteActivity(
            workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
                ActivityID: opID,
                StartToCloseTimeout: opConfig.Timeout,
            }),
            opConfig.Activity,
            input,
        ).Get(ctx, output)
        
    case "state":
        // State transition logic (inline, not special)
        return executeStateTransition(ctx, opConfig, input, output)
        
    case "nested":
        // Nested workflow (hidden from user)
        return workflow.ExecuteChildWorkflow(
            workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
                WorkflowID: fmt.Sprintf("%s-%s", workflow.GetInfo(ctx).WorkflowExecution.ID, opID),
            }),
            opConfig.Workflow,
            input,
        ).Get(ctx, output)
        
    default:
        return fmt.Errorf("unknown op type: %s", opConfig.Type)
    }
}
```

### Handling Nested Workflows

Nested workflows can be encapsulated as single ops from the user's perspective:

```go
// User sees: workflowA.dataProcessingOp
// Reality: dataProcessingOp is actually a complex nested workflow

func DataProcessingWorkflow(ctx workflow.Context, input DataProcessingInput) (*DataProcessingOutput, error) {
    // This workflow is hidden from the user - appears as single op
    
    // Check for patches passed from parent
    info := workflow.GetInfo(ctx)
    patches := getPatchesFromMemo(info.Memo)
    
    // Internal ops of the nested workflow
    var step1Output Step1Output
    step1Input := input.Step1Input
    
    // Patches use the full path including the encapsulated op name
    if patch, exists := patches["workflowA.dataProcessingOp.step1"]; exists {
        step1Input = applyPatch(step1Input, patch)
    }
    
    err := workflow.ExecuteActivity(ctx, Step1Activity, step1Input).Get(ctx, &step1Output)
    if err != nil {
        return nil, err
    }
    
    // More internal steps...
    
    return &DataProcessingOutput{Result: finalResult}, nil
}

// From user's perspective, they can restart at:
// - workflowA.dataProcessingOp (restart the entire nested workflow)
// - workflowA.dataProcessingOp.step1 (restart within the nested workflow)
```

### State Handling (Not Special)

States are just workflow logic, not a separate concept:

```go
func WorkflowWithStates(ctx workflow.Context, input WorkflowInput) (*WorkflowOutput, error) {
    // States are just ops with conditional logic
    patches := getPatchesFromMemo(workflow.GetInfo(ctx).Memo)
    
    currentState := "init"
    stateData := make(map[string]interface{})
    
    for currentState != "end" {
        switch currentState {
        case "init":
            var initOutput InitOutput
            initInput := InitInput{Data: stateData}
            
            // States can be patched like any op
            if patch, exists := patches["workflow.state.init"]; exists {
                initInput = applyPatch(initInput, patch)
            }
            
            err := workflow.ExecuteActivity(ctx, InitActivity, initInput).Get(ctx, &initOutput)
            if err != nil {
                return nil, err
            }
            
            stateData = mergeData(stateData, initOutput.Data)
            currentState = evaluateNextState(initOutput)
            
        case "process":
            // Similar pattern for other states
            var processOutput ProcessOutput
            processInput := ProcessInput{Data: stateData}
            
            if patch, exists := patches["workflow.state.process"]; exists {
                processInput = applyPatch(processInput, patch)
            }
            
            err := workflow.ExecuteActivity(ctx, ProcessActivity, processInput).Get(ctx, &processOutput)
            // ...
        }
    }
    
    return &WorkflowOutput{FinalData: stateData}, nil
}
```

## Op Path Resolution

The compiler generates metadata to map op paths to Temporal events:

```go
// Generated by compiler
func RegisterOpMetadata() {
    // Maps user-visible op paths to Temporal activity IDs
    OpRegistry.Register("workflowA.opA1", OpMetadata{
        ActivityID: "opA1",
        Type: "activity",
    })
    
    OpRegistry.Register("workflowA.dataProcessingOp", OpMetadata{
        Type: "nested_workflow",
        WorkflowType: "DataProcessingWorkflow",
        // Nested ops
        Children: []string{
            "workflowA.dataProcessingOp.step1",
            "workflowA.dataProcessingOp.step2",
        },
    })
    
    OpRegistry.Register("workflowA.state.init", OpMetadata{
        ActivityID: "state-init",
        Type: "state",
    })
}

func findEventIDForOp(ctx context.Context, workflowID, runID, opPath string) (int64, error) {
    // Look up the op metadata
    opMeta := OpRegistry.Get(opPath)
    if opMeta == nil {
        return 0, fmt.Errorf("unknown op: %s", opPath)
    }
    
    // Get workflow history from Temporal
    iter := temporalClient.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
    
    // Find the corresponding event based on op type
    switch opMeta.Type {
    case "activity", "state":
        return findActivityCompletedEvent(iter, opMeta.ActivityID)
    case "nested_workflow":
        return findChildWorkflowCompletedEvent(iter, opMeta.WorkflowType)
    default:
        return 0, fmt.Errorf("unsupported op type: %s", opMeta.Type)
    }
}
```

## Patch Application

Patches are applied based on merge strategy:

```go
func applyPatch(original interface{}, patch InputPatch) interface{} {
    switch patch.MergeStrategy {
    case "replace":
        // Complete replacement
        return patch.InputOverrides
        
    case "shallow":
        // Shallow merge - top-level keys only
        result := copyMap(original)
        for k, v := range patch.InputOverrides {
            result[k] = v
        }
        return result
        
    case "deep":
        // Deep merge - recursive
        return deepMerge(original, patch.InputOverrides)
        
    default:
        return original
    }
}

func getCumulativePatchesFromMemo(memo *common.Memo) CumulativePatches {
    if memo == nil || memo.Fields == nil {
        return CumulativePatches{
            Patches: make(map[string]InputPatch),
        }
    }
    
    // Check for cumulative patches (from restart)
    if cumulativePayload, exists := memo.Fields["cumulative_patches"]; exists {
        var cumulative CumulativePatches
        if err := converter.GetDefaultDataConverter().FromPayload(cumulativePayload, &cumulative); err == nil {
            return cumulative
        }
    }
    
    // Fallback to simple patches (first run)
    if patchPayload, exists := memo.Fields["patches"]; exists {
        var patches map[string]InputPatch
        if err := converter.GetDefaultDataConverter().FromPayload(patchPayload, &patches); err == nil {
            return CumulativePatches{
                Patches: patches,
            }
        }
    }
    
    return CumulativePatches{
        Patches: make(map[string]InputPatch),
    }
}
```

## User Interface

### REST API

```go
// POST /api/workflows/{workflowId}/runs/{runId}/restart
type RestartRequest struct {
    RestartPoint string                    `json:"restart_point"`  // e.g., "workflowA.opB2"
    Patches      map[string]InputPatch     `json:"patches,omitempty"`
    Reason       string                    `json:"reason,omitempty"`
}

type RestartResponse struct {
    WorkflowID    string    `json:"workflow_id"`
    RunID         string    `json:"run_id"`  // New run ID after reset
    RestartedAt   time.Time `json:"restarted_at"`
}
```

### CLI

```bash
# Restart at specific op with patch
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.dataProcessingOp.step2" \
  --patch '{"threshold": 0.8}' \
  --reason "Adjusting threshold"

# Restart at state with patch
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.state.process" \
  --patch '{"retry_count": 0, "mode": "enhanced"}'
```

## Cumulative Patch Example

### Scenario: Multiple Restarts with Patches

**Initial Run:**
```
workflowA:
  ✓ opA1 -> input: {threshold: 0.5}
  ✓ opA2 -> input: {mode: "basic"}
  ✓ opA3 -> input: {validate: false}
  ✓ opA4 -> input: {parallel: false}
```

**First Restart at opA2:**
```bash
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-original \
  --restart-point "workflowA.opA2" \
  --patch '{"mode": "advanced"}'
```

Result (run-456):
```
workflowA:
  ↻ opA1 -> (skipped, before restart point)
  ✓ opA2 -> input: {mode: "advanced"}  # PATCHED
  ✓ opA3 -> input: {validate: false}
  ✓ opA4 -> input: {parallel: false}
```

Cumulative patches: `{"workflowA.opA2": {mode: "advanced"}}`

**Second Restart at opA3 (from already patched run):**
```bash
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.opA3" \
  --patch '{"validate": true, "strict": true}'
```

Result (run-789):
```
workflowA:
  ↻ opA1 -> (skipped, before restart point)
  ↻ opA2 -> input: {mode: "advanced"}  # PRESERVED PATCH
  ✓ opA3 -> input: {validate: true, strict: true}  # NEW PATCH
  ✓ opA4 -> input: {parallel: false}
```

Cumulative patches: 
```json
{
  "workflowA.opA2": {"mode": "advanced"},  // Preserved
  "workflowA.opA3": {"validate": true, "strict": true}  // Added
}
```

**Third Restart at opA2 again (with different patch):**
```bash
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-789 \
  --restart-point "workflowA.opA2" \
  --patch '{"mode": "expert", "timeout": 30}'
```

Result (run-999):
```
workflowA:
  ↻ opA1 -> (skipped, before restart point)
  ✓ opA2 -> input: {mode: "expert", timeout: 30}  # REPLACED PATCH
  ✓ opA3 -> input: {validate: false}  # opA3 patch removed (after restart point)
  ✓ opA4 -> input: {parallel: false}
```

Cumulative patches:
```json
{
  "workflowA.opA2": {"mode": "expert", "timeout": 30}  // Updated
  // opA3 patch removed because it's after the restart point
}
```

## Example Scenarios

### Scenario 1: Restart Nested Workflow Op

User's view:
```
workflowA:
  ✓ opA1
  ✓ dataProcessingOp  ← User wants to restart here
  ✓ opA3
```

Internal structure (hidden):
```
workflowA:
  ✓ opA1 (activity)
  ✓ dataProcessingOp (nested workflow):
    ✓ step1 (activity)
    ✓ step2 (activity)  
    ✓ step3 (activity)
  ✓ opA3 (activity)
```

Restart command:
```bash
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.dataProcessingOp" \
  --patch '{"processing_mode": "enhanced"}'
```

Result:
- Temporal resets to before the nested workflow
- New run re-executes dataProcessingOp with patched input
- opA3 re-executes with new output from dataProcessingOp

### Scenario 2: Restart Within Nested Workflow

User wants to restart within the encapsulated op:
```bash
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.dataProcessingOp.step2" \
  --patch '{"algorithm": "v2"}'
```

Result:
- Temporal resets the parent workflow to before the nested workflow
- Nested workflow re-executes from beginning but with patch applied at step2
- Parent workflow continues with new nested workflow output

### Scenario 3: Restart at State Transition

```bash
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.state.process" \
  --patch '{"processing_config": {"parallel": true}}'
```

Result:
- Workflow resets to the process state
- State executes with patched input
- Subsequent states use updated state data

## Audit Trail

Simple audit logging without external dependencies:

```go
func (s *RestartService) RecordRestart(req RestartRequest, newRunID string) {
    // Log restart event
    logger.Info("Workflow restarted",
        "workflow_id", req.WorkflowID,
        "original_run_id", req.RunID,
        "new_run_id", newRunID,
        "restart_point", req.RestartPoint,
        "patches_count", len(req.Patches),
        "reason", req.Reason,
        "initiated_by", getUserFromContext(),
    )
    
    // Store in simple audit table if needed
    s.db.Exec(`
        INSERT INTO restart_audit (
            workflow_id, original_run_id, new_run_id,
            restart_point, patch_count, reason, initiated_by
        ) VALUES (?, ?, ?, ?, ?, ?, ?)
    `, req.WorkflowID, req.RunID, newRunID,
       req.RestartPoint, len(req.Patches), req.Reason, getUserFromContext())
}
```

## Testing Strategy

### Unit Tests
1. Patch application with different merge strategies
2. Op path to event ID resolution
3. Patch filtering for nested workflows
4. Cumulative patch merging logic
5. Patch preservation across restarts

### Integration Tests
1. End-to-end restart with Temporal
2. Nested workflow restart scenarios
3. State transition restart scenarios
4. Patch propagation through workflow hierarchy
5. Multiple restarts with cumulative patches
6. Patch override scenarios

## Success Criteria

- [ ] Workflows restart at any op using Temporal's reset
- [ ] Patches passed directly through reset (no external store)
- [ ] Cumulative patches preserved across multiple restarts
- [ ] Patches correctly merged when restarting previously patched workflows
- [ ] Nested workflows properly handle patches
- [ ] States treated as regular ops (no special handling)
- [ ] Op registry maps user paths to Temporal events
- [ ] Audit trail captures restart metadata and patch history
- [ ] Works with standard Temporal tooling