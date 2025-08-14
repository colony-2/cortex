# Recipe Restart and Patching Specification

## Overview

This specification defines how the vibethis recipe system enables restart and patching of workflow executions by leveraging Temporal's workflow versioning and replay capabilities. The system allows users to restart workflows at any point in the execution tree with modified inputs, while maintaining execution history and state consistency.

## Problem Statement

When executing complex nested workflows, users need the ability to:
1. Restart execution from any point in the workflow tree (including nested child workflows)
2. Apply patches (input modifications) at the restart point
3. Preserve execution history up to the restart point
4. Re-execute downstream ops with updated outputs from patched ops
5. Maintain audit trail of original execution and patches applied

## Core Concepts

### Execution Checkpoint Model

Every op execution creates a checkpoint that includes:
- **Execution ID**: Unique identifier for this execution instance
- **Op Path**: Full path in the execution tree (e.g., `workflowA.workflowB.opB2`)
- **Input Snapshot**: Complete input state at execution time
- **Output Snapshot**: Complete output state after execution
- **Temporal Context**: Workflow ID, run ID, activity ID for Temporal correlation
- **Metadata**: Timestamp, duration, status, error details if applicable

### Patch Definition

A patch represents modifications to be applied at a restart point:
```yaml
patch:
  target: "workflowA.workflowB.opB2"  # Op path to restart from
  input_overrides:                    # Input modifications
    field1: "new_value"
    field2: 
      nested: "modified"
  merge_strategy: "deep"              # How to apply overrides: replace|shallow|deep
  propagate: true                     # Whether to re-execute downstream ops
```

## Architecture

### Compiler Instrumentation

The recipe compiler injects checkpoint and restart logic into generated Temporal workflows:

```go
// Generated workflow code with restart instrumentation
func WorkflowA(ctx workflow.Context, input WorkflowAInput) (*WorkflowAOutput, error) {
    logger := workflow.GetLogger(ctx)
    
    // Check for restart context
    restartCtx := workflow.GetRestartContext(ctx)
    var checkpoint *ExecutionCheckpoint
    
    if restartCtx != nil {
        checkpoint = restartCtx.GetCheckpoint("workflowA")
    }
    
    // Op A1 (maps to Temporal activity)
    if checkpoint == nil || checkpoint.ShouldExecute("workflowA.opA1") {
        opA1Input := prepareOpInput(input.OpA1Input, checkpoint, "workflowA.opA1")
        
        // Record pre-execution checkpoint
        preCheckpoint := &ExecutionCheckpoint{
            Path:      "workflowA.opA1",
            Inputs:    opA1Input,
            Timestamp: workflow.Now(ctx),
        }
        workflow.RecordCheckpoint(ctx, preCheckpoint)
        
        // Execute op as Temporal activity
        var opA1Output OpA1Output
        err := workflow.ExecuteActivity(ctx, OpA1Activity, opA1Input).Get(ctx, &opA1Output)
        if err != nil {
            return nil, err
        }
        
        // Record post-execution checkpoint
        postCheckpoint := &ExecutionCheckpoint{
            Path:      "workflowA.opA1",
            Inputs:    opA1Input,
            Outputs:   opA1Output,
            Timestamp: workflow.Now(ctx),
            Duration:  workflow.Since(ctx, preCheckpoint.Timestamp),
        }
        workflow.RecordCheckpoint(ctx, postCheckpoint)
    } else {
        // Use checkpoint output for skipped execution
        opA1Output = checkpoint.GetOutput("workflowA.opA1").(OpA1Output)
    }
    
    // Op A2 (maps to Temporal activity)
    if checkpoint == nil || checkpoint.ShouldExecute("workflowA.opA2") {
        // Similar checkpoint logic...
        var opA2Output OpA2Output
        err := workflow.ExecuteActivity(ctx, OpA2Activity, opA2Input).Get(ctx, &opA2Output)
        // ...
    }
    
    // Child Workflow B
    if checkpoint == nil || checkpoint.ShouldExecute("workflowA.workflowB") {
        // Prepare child workflow input with potential patches
        wfBInput := prepareChildWorkflowInput(opA2Output, checkpoint, "workflowA.workflowB")
        
        // Pass restart context to child
        childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
            WorkflowID: fmt.Sprintf("%s-workflowB", workflow.GetInfo(ctx).WorkflowExecution.ID),
        })
        
        if restartCtx != nil {
            childCtx = workflow.WithRestartContext(childCtx, restartCtx.ForChild("workflowA.workflowB"))
        }
        
        var wfBOutput WorkflowBOutput
        err := workflow.ExecuteChildWorkflow(childCtx, WorkflowB, wfBInput).Get(ctx, &wfBOutput)
        if err != nil {
            return nil, err
        }
        
        // Record child workflow completion
        workflow.RecordCheckpoint(ctx, &ExecutionCheckpoint{
            Path:    "workflowA.workflowB",
            Inputs:  wfBInput,
            Outputs: wfBOutput,
            Type:    "child_workflow",
        })
    } else {
        wfBOutput = checkpoint.GetOutput("workflowA.workflowB").(WorkflowBOutput)
    }
    
    // Op A3 (uses output from workflow B)
    if checkpoint == nil || checkpoint.ShouldExecute("workflowA.opA3") {
        opA3Input := OpA3Input{
            FromWorkflowB: wfBOutput.Result,
            // other fields...
        }
        
        // Apply any patches for this op
        opA3Input = applyPatches(opA3Input, checkpoint, "workflowA.opA3")
        
        var opA3Output OpA3Output
        err := workflow.ExecuteActivity(ctx, OpA3Activity, opA3Input).Get(ctx, &opA3Output)
        // ...
    }
    
    return &WorkflowAOutput{
        OpA1Result: opA1Output,
        OpA3Result: opA3Output,
        WfBResult:  wfBOutput,
    }, nil
}
```

### Restart Context Management

```go
type RestartContext struct {
    OriginalRunID   string                       // Original workflow run
    RestartPoint    string                       // Path where restart begins
    Checkpoints     map[string]*ExecutionCheckpoint // Historical checkpoints
    Patches         map[string]*PatchDefinition     // Patches to apply
    ExecutionMode   RestartMode                     // full|selective|downstream
}

type RestartMode string

const (
    // Re-execute everything from restart point
    RestartModeFull RestartMode = "full"
    
    // Only execute ops with patches
    RestartModeSelective RestartMode = "selective"
    
    // Execute restart point and all downstream
    RestartModeDownstream RestartMode = "downstream"
)

type ExecutionCheckpoint struct {
    Path          string                 // Full op path
    Inputs        interface{}            // Input snapshot
    Outputs       interface{}            // Output snapshot
    Timestamp     time.Time              // Execution time
    Duration      time.Duration          // Execution duration
    Status        string                 // success|failure|skipped
    Error         *ErrorDetails          // Error info if failed
    TemporalMeta  *TemporalMetadata      // Temporal-specific metadata
}

type PatchDefinition struct {
    Target         string                 // Op path to patch
    InputOverrides map[string]interface{} // Input modifications
    MergeStrategy  MergeStrategy          // How to apply overrides
    Propagate      bool                   // Re-execute downstream
}
```

### Checkpoint Storage

Checkpoints are stored in a dedicated table for efficient querying and retrieval:

```sql
CREATE TABLE execution_checkpoints (
    id              UUID PRIMARY KEY,
    workflow_id     VARCHAR(255) NOT NULL,
    run_id          VARCHAR(255) NOT NULL,
    op_path         VARCHAR(500) NOT NULL,
    checkpoint_type VARCHAR(50) NOT NULL, -- pre|post|child_workflow
    inputs          JSONB,
    outputs         JSONB,
    status          VARCHAR(50),
    error_details   JSONB,
    temporal_meta   JSONB,
    created_at      TIMESTAMP NOT NULL,
    duration_ms     INTEGER,
    
    INDEX idx_workflow_run (workflow_id, run_id),
    INDEX idx_op_path (op_path),
    INDEX idx_created_at (created_at)
);

CREATE TABLE restart_patches (
    id              UUID PRIMARY KEY,
    workflow_id     VARCHAR(255) NOT NULL,
    original_run_id VARCHAR(255) NOT NULL,
    restart_run_id  VARCHAR(255) NOT NULL,
    restart_point   VARCHAR(500) NOT NULL,
    patches         JSONB NOT NULL,
    applied_by      VARCHAR(255),
    applied_at      TIMESTAMP NOT NULL,
    reason          TEXT,
    
    INDEX idx_original_run (original_run_id),
    INDEX idx_restart_run (restart_run_id)
);
```

### Restart Execution Flow

#### 1. Initiate Restart

```go
func InitiateRestart(ctx context.Context, req RestartRequest) (*RestartResponse, error) {
    // Load original execution checkpoints
    checkpoints, err := LoadCheckpoints(req.OriginalWorkflowID, req.OriginalRunID)
    if err != nil {
        return nil, err
    }
    
    // Validate restart point exists
    if !checkpoints.HasPath(req.RestartPoint) {
        return nil, fmt.Errorf("restart point %s not found in execution", req.RestartPoint)
    }
    
    // Build restart context
    restartCtx := &RestartContext{
        OriginalRunID: req.OriginalRunID,
        RestartPoint:  req.RestartPoint,
        Checkpoints:   checkpoints,
        Patches:       req.Patches,
        ExecutionMode: req.Mode,
    }
    
    // Determine root workflow to restart
    rootWorkflow := getRootWorkflow(req.RestartPoint)
    
    // Start new workflow execution with restart context
    workflowOptions := client.StartWorkflowOptions{
        ID:        fmt.Sprintf("%s-restart-%s", req.OriginalWorkflowID, uuid.New()),
        TaskQueue: req.TaskQueue,
        Memo: map[string]interface{}{
            "restart_context": restartCtx,
            "original_run":    req.OriginalRunID,
            "restart_point":   req.RestartPoint,
        },
    }
    
    // Execute workflow with restart context
    we, err := temporalClient.ExecuteWorkflow(ctx, workflowOptions, rootWorkflow, originalInput)
    if err != nil {
        return nil, err
    }
    
    // Record restart metadata
    err = RecordRestart(req, we.GetID(), we.GetRunID())
    if err != nil {
        // Log but don't fail
        logger.Error("failed to record restart", "error", err)
    }
    
    return &RestartResponse{
        WorkflowID: we.GetID(),
        RunID:      we.GetRunID(),
    }, nil
}
```

#### 2. Checkpoint Decision Logic

```go
func (c *ExecutionCheckpoint) ShouldExecute(opPath string) bool {
    restartCtx := c.RestartContext
    if restartCtx == nil {
        return true // No restart context, execute normally
    }
    
    switch restartCtx.ExecutionMode {
    case RestartModeFull:
        // Execute if at or after restart point
        return isAtOrAfter(opPath, restartCtx.RestartPoint)
        
    case RestartModeSelective:
        // Only execute if has patches
        return restartCtx.HasPatch(opPath)
        
    case RestartModeDownstream:
        // Execute if at restart point or downstream dependency
        return isAtOrDownstream(opPath, restartCtx.RestartPoint)
        
    default:
        return true
    }
}

func isAtOrAfter(path, restartPoint string) bool {
    // Check if path is the restart point or executes after it
    if path == restartPoint {
        return true
    }
    
    // Parse execution order from path
    pathOrder := getExecutionOrder(path)
    restartOrder := getExecutionOrder(restartPoint)
    
    return pathOrder >= restartOrder
}

func isAtOrDownstream(path, restartPoint string) bool {
    // Check if path depends on restart point output
    dependencies := getDependencies(path)
    return contains(dependencies, restartPoint) || path == restartPoint
}
```

#### 3. Input Patching

```go
func prepareOpInput(originalInput interface{}, checkpoint *ExecutionCheckpoint, opPath string) interface{} {
    if checkpoint == nil {
        return originalInput
    }
    
    // Check for patches
    patch := checkpoint.GetPatch(opPath)
    if patch == nil {
        // No patch, use original or checkpoint input
        if checkpoint.HasExecuted(opPath) {
            return checkpoint.GetInput(opPath)
        }
        return originalInput
    }
    
    // Apply patch based on merge strategy
    switch patch.MergeStrategy {
    case MergeStrategyReplace:
        return patch.InputOverrides
        
    case MergeStrategyShallow:
        return shallowMerge(originalInput, patch.InputOverrides)
        
    case MergeStrategyDeep:
        return deepMerge(originalInput, patch.InputOverrides)
        
    default:
        return originalInput
    }
}

func deepMerge(base, override interface{}) interface{} {
    // Deep merge implementation
    baseMap := toMap(base)
    overrideMap := toMap(override)
    
    result := make(map[string]interface{})
    
    // Copy base values
    for k, v := range baseMap {
        result[k] = v
    }
    
    // Apply overrides
    for k, v := range overrideMap {
        if existing, exists := result[k]; exists {
            // Recursively merge nested objects
            if isMap(existing) && isMap(v) {
                result[k] = deepMerge(existing, v)
            } else {
                result[k] = v
            }
        } else {
            result[k] = v
        }
    }
    
    return result
}
```

## User Interface

### Restart API

```go
type RestartRequest struct {
    OriginalWorkflowID string                    `json:"original_workflow_id"`
    OriginalRunID      string                    `json:"original_run_id"`
    RestartPoint       string                    `json:"restart_point"`
    Patches            map[string]*PatchDefinition `json:"patches,omitempty"`
    Mode               RestartMode                 `json:"mode"`
    Reason             string                      `json:"reason,omitempty"`
}

type RestartResponse struct {
    WorkflowID    string    `json:"workflow_id"`
    RunID         string    `json:"run_id"`
    RestartedAt   time.Time `json:"restarted_at"`
}

// REST API
// POST /api/workflows/{workflowId}/runs/{runId}/restart
```

### CLI Interface

```bash
# Restart at specific op with patch
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.workflowB.opB2" \
  --patch '{"field1": "new_value"}' \
  --mode downstream

# Restart with patch file
vibethis workflow restart \
  --workflow-id wf-123 \
  --run-id run-456 \
  --restart-point "workflowA.workflowB.opB2" \
  --patch-file patches.yaml \
  --reason "Correcting configuration error"

# View restart history
vibethis workflow restarts \
  --workflow-id wf-123 \
  --original-run-id run-456
```

### Patch File Format

```yaml
# patches.yaml
restart_point: workflowA.workflowB.opB2
mode: downstream
reason: "Fixing data processing error in opB2"

patches:
  workflowA.workflowB.opB2:
    input_overrides:
      processing_config:
        threshold: 0.8  # was 0.5
        enable_validation: true
    merge_strategy: deep
    
  workflowA.opA3:
    input_overrides:
      additional_context: "Updated based on new B2 output"
    merge_strategy: shallow
```

## Execution Examples

### Example 1: Simple Downstream Restart

Original execution completed:
```
workflowA:
  ✓ opA1 -> output: {data: "initial"}
  ✓ opA2 -> output: {processed: "v1"}
  ✓ workflowB:
    ✓ opB1 -> output: {step1: "done"}
    ✓ opB2 -> output: {step2: "result_v1"}  ← Restart Point
  ✓ opA3 -> output: {final: "based_on_result_v1"}
```

Restart with patch at opB2:
```yaml
restart_point: workflowA.workflowB.opB2
patches:
  workflowA.workflowB.opB2:
    input_overrides:
      algorithm: "v2"
mode: downstream
```

Restarted execution:
```
workflowA:
  ↻ opA1 -> (skipped, use checkpoint)
  ↻ opA2 -> (skipped, use checkpoint)
  ↻ workflowB:
    ↻ opB1 -> (skipped, use checkpoint)
    ✓ opB2 -> output: {step2: "result_v2"}  ← Re-executed with patch
  ✓ opA3 -> output: {final: "based_on_result_v2"}  ← Re-executed with new B output
```

### Example 2: Selective Patch Application

```yaml
restart_point: workflowA.opA1
mode: selective
patches:
  workflowA.opA2:
    input_overrides:
      config: "updated"
  workflowA.workflowB.opB2:
    input_overrides:
      threshold: 0.9
```

Execution:
```
workflowA:
  ↻ opA1 -> (skipped, no patch)
  ✓ opA2 -> (executed with patch)
  ↻ workflowB:
    ↻ opB1 -> (skipped, no patch)
    ✓ opB2 -> (executed with patch)
  ↻ opA3 -> (skipped, no patch)
```

## State Machine Integration

For workflows using CEL state machines, restart points can target specific states:

```yaml
restart_point: "workflowA.stateMachine.processState"
patches:
  workflowA.stateMachine.processState:
    input_overrides:
      retry_count: 0
      processing_mode: "enhanced"
```

The state machine resume from the specified state with checkpointed state context:

```go
func StateMachineWorkflow(ctx workflow.Context, input StateMachineInput) (*StateMachineOutput, error) {
    restartCtx := workflow.GetRestartContext(ctx)
    
    var currentState string
    var stateData map[string]interface{}
    
    if restartCtx != nil && restartCtx.HasCheckpoint("stateMachine") {
        // Resume from checkpoint
        checkpoint := restartCtx.GetCheckpoint("stateMachine")
        currentState = checkpoint.GetString("current_state")
        stateData = checkpoint.GetMap("state_data")
    } else {
        // Start from initial state
        currentState = "init"
        stateData = make(map[string]interface{})
    }
    
    // Execute state machine from current state
    for currentState != "end" {
        stateConfig := getStateConfig(currentState)
        
        // Check if this state should be executed
        statePath := fmt.Sprintf("stateMachine.%s", currentState)
        if restartCtx == nil || restartCtx.ShouldExecute(statePath) {
            // Execute state op (as Temporal activity)
            stateInput := prepareStateInput(stateData, restartCtx, statePath)
            
            var stateOutput StateOutput
            err := workflow.ExecuteActivity(ctx, stateConfig.Activity, stateInput).Get(ctx, &stateOutput)
            if err != nil {
                return nil, err
            }
            
            // Update state data
            stateData = mergeStateData(stateData, stateOutput.Data)
            
            // Determine next state
            currentState = evaluateTransition(stateConfig.Transitions, stateData)
            
            // Record state checkpoint
            workflow.RecordCheckpoint(ctx, &ExecutionCheckpoint{
                Path: statePath,
                Outputs: map[string]interface{}{
                    "current_state": currentState,
                    "state_data":    stateData,
                },
            })
        } else {
            // Skip to next state from checkpoint
            checkpoint := restartCtx.GetCheckpoint(statePath)
            currentState = checkpoint.GetString("next_state")
            stateData = checkpoint.GetMap("state_data")
        }
    }
    
    return &StateMachineOutput{
        FinalState: currentState,
        Data:       stateData,
    }, nil
}
```

## Monitoring and Observability

### Restart Metrics

```go
type RestartMetrics struct {
    TotalRestarts      int64
    SuccessfulRestarts int64
    FailedRestarts     int64
    AverageSkipCount   float64
    AverageReExecCount float64
    CommonRestartPoints []RestartPointStats
}

type RestartPointStats struct {
    Path         string
    RestartCount int64
    SuccessRate  float64
    CommonErrors []string
}
```

### Audit Trail

Every restart creates an audit record:

```json
{
  "event_type": "workflow_restart",
  "timestamp": "2024-01-15T10:30:00Z",
  "workflow_id": "wf-123",
  "original_run_id": "run-456",
  "restart_run_id": "run-789",
  "restart_point": "workflowA.workflowB.opB2",
  "patches_applied": [
    {
      "target": "workflowA.workflowB.opB2",
      "fields_modified": ["processing_config.threshold"]
    }
  ],
  "initiated_by": "user@example.com",
  "reason": "Correcting threshold configuration"
}
```

## Security Considerations

### Authorization

- Restart requests must be authorized based on workflow ownership
- Patch content is validated against schema before application
- Sensitive fields can be marked as non-patchable

```go
type OpConfig struct {
    Name           string
    PatchableFields []string  // Whitelist of patchable fields
    RestartPolicy   RestartPolicy
}

type RestartPolicy struct {
    AllowRestart   bool
    RequireApproval bool
    MaxRestarts    int
    RestartWindow  time.Duration
}
```

### Data Integrity

- Original execution data is never modified
- Patches are stored separately and applied at runtime
- Full lineage tracking from original to restarted executions

## Performance Optimizations

### Checkpoint Compression

Large checkpoints are compressed before storage:

```go
func StoreCheckpoint(checkpoint *ExecutionCheckpoint) error {
    if checkpoint.Size() > CompressionThreshold {
        checkpoint.Inputs = compress(checkpoint.Inputs)
        checkpoint.Outputs = compress(checkpoint.Outputs)
        checkpoint.Compressed = true
    }
    
    return db.StoreCheckpoint(checkpoint)
}
```

### Selective Loading

Only load checkpoints relevant to restart:

```go
func LoadCheckpoints(workflowID, runID, restartPoint string) (map[string]*ExecutionCheckpoint, error) {
    // Determine checkpoint range based on restart point
    startPath := getRestartRangeStart(restartPoint)
    
    // Only load checkpoints from restart point backwards
    checkpoints, err := db.Query(`
        SELECT * FROM execution_checkpoints 
        WHERE workflow_id = ? AND run_id = ?
        AND op_path <= ?
        ORDER BY created_at
    `, workflowID, runID, startPath)
    
    return checkpoints, err
}
```

## Future Enhancements

### Intelligent Restart Suggestions

- Analyze failed executions and suggest optimal restart points
- Detect patterns in restart behavior for optimization
- Automatic patch generation based on error analysis

### Checkpoint Versioning

- Support schema evolution in checkpoint data
- Automatic migration of checkpoint formats
- Backward compatibility for historical restarts

### Distributed Checkpointing

- Checkpoint storage in distributed systems (S3, GCS)
- Checkpoint sharing across environments
- Cross-region restart capability

## Testing Requirements

### Unit Tests

1. Checkpoint creation and storage
2. Restart context propagation
3. Patch application with different merge strategies
4. Skip/execute decision logic
5. State machine restart behavior

### Integration Tests

1. End-to-end restart with nested workflows
2. Patch propagation through workflow tree
3. Checkpoint recovery after failures
4. Concurrent restart handling
5. Large checkpoint handling

### Performance Tests

1. Checkpoint storage/retrieval latency
2. Restart overhead vs fresh execution
3. Memory usage with large checkpoint sets
4. Compression effectiveness

## Success Criteria

- [ ] Workflows can be restarted from any op or state
- [ ] Patches are correctly applied at restart points
- [ ] Downstream ops use updated outputs
- [ ] Original execution data is preserved
- [ ] Restart context propagates through nested workflows
- [ ] State machines can resume from checkpointed states
- [ ] Performance overhead is < 10% for normal execution
- [ ] Checkpoint storage scales to millions of executions
- [ ] Full audit trail of all restarts and patches
- [ ] Clear debugging visibility into restart behavior