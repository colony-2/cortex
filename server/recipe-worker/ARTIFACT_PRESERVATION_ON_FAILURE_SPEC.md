# Artifact Preservation on Task Failure Specification

**Scope:** `/src/server/recipe-worker` module only

## Problem Statement

When a task fails during execution, all output artifacts that have been added to `OpDependencies` are discarded. Operations may produce valuable diagnostic artifacts (logs, stderr, partial results) before failing, but these are lost because:

1. **activity_registry.go**: Returns `nil` artifacts on error instead of calling `GetOutputArtifacts()`
2. **compiler.go**: Discards the `out` result when `ctx.DoTask()` fails
3. **job_worker.go**: Returns `nil` JobData when `ExecuteRecipe()` fails

The SWF framework saves artifacts when they are returned by ops, so we need to ensure artifacts are returned alongside errors at each level.

## Requirements

**FR-1: Artifact Retrieval on Failure**
- When a task fails, the system MUST retrieve all artifacts added to `OpDependencies` before the failure
- Retrieved artifacts MUST be included in the failure response

**FR-2: Artifact Cleanup on Failure**
- All artifacts retrieved on failure MUST have their cleanup callbacks invoked by SWF
- The system MUST NOT leak temporary files or resources when tasks fail

**FR-3: Error Context Preservation**
- The original error that caused the task failure MUST be preserved and returned
- Artifact retrieval failures MUST NOT mask the original error

## Required Changes

### activity_registry.go - Deferred Artifact Collection

**Location:** `recipe-worker/pkg/ops/activity_registry.go:withGitWorkspace()`

Use `defer` to always collect artifacts, even on error path:

```go
func (reg *ActivityRegistry) withGitWorkspace(
    ctx context.Context,
    req *gitWorkspaceActivityRequest,
) (_ OpOutput, _ []swf.Artifact, _ error) {
    // ... existing setup ...

    var outputArtifacts []swf.Artifact
    defer func() {
        artifacts := opDeps.GetOutputArtifacts()
        if len(artifacts) > 0 {
            outputArtifacts = append(outputArtifacts, artifacts...)
        }
    }()

    // ... existing git workspace setup ...

    outputData, err := reg.Step.Invoke(opDeps, ctx, req.Input)
    if err != nil {
        // Defer will collect artifacts
        return OpOutput{}, outputArtifacts, fmt.Errorf("operation failed: %w", err)
    }

    outputMetadata, outputThinPack, err := controller.Persist(context.Background(), &req.GitTaskContext)
    if err != nil {
        // Defer will collect artifacts from operation
        return OpOutput{}, outputArtifacts, fmt.Errorf("persist failed: %w", err)
    }

    if outputThinPack != nil {
        outputArtifacts = append(outputArtifacts, outputThinPack)
    }

    return OpOutput{Data: outputData, GitMetadata: outputMetadata}, outputArtifacts, nil
}
```

### compiler.go - Don't Discard Artifacts from Failed Tasks

**Location:** `recipe-worker/pkg/compiler/compiler.go:executeOp()`

Currently when `ctx.DoTask()` fails, the `out` result is discarded. Need to extract and propagate artifacts:

```go
out, err := ctx.DoTask(runPolicy, taskType, taskData)
if err != nil {
    // Don't discard artifacts from failed task
    // TODO: How to propagate artifacts through compiler?
    // Options:
    // 1. Store in resolution context
    // 2. Attach to error
    // 3. Store in workflow state
    return fmt.Errorf("task %s failed: %w", taskType, err)
}
```

**Open Question:** How should artifacts from failed tasks be propagated through the compiler layer?

### job_worker.go - Preserve Job-Level Artifacts

**Location:** `recipe-worker/pkg/compiler/job_worker.go:Run()`

Currently returns `nil` JobData on failure:

```go
out, err := ExecuteRecipe(wCtx, r, input.Inputs, runContext, contextual.GitCommitContext{ParentRef: input.GitRef})
if err != nil {
    return nil, err  // No JobData means artifacts are lost
}
```

**Open Question:** Should failed jobs return JobData with artifacts? How does SWF handle JobData returned alongside errors?

## Implementation Steps

1. Update `activity_registry.go` with deferred artifact collection
2. Resolve open questions about artifact propagation through compiler and job_worker layers
3. Update `compiler.go` and `job_worker.go` based on decisions
4. Add unit tests for artifact preservation on failure
5. Verify artifact cleanup is invoked by SWF for failed tasks

## Testing

### Unit Tests

**File:** `recipe-worker/pkg/ops/activity_registry_test.go`

```go
func TestWithGitWorkspace_OperationFailure_PreservesArtifacts(t *testing.T) {
    // Setup: Operation adds artifacts then fails
    // Assert: Artifacts are returned with error
}

func TestWithGitWorkspace_PersistFailure_PreservesOperationArtifacts(t *testing.T) {
    // Setup: Successful operation, failing persist
    // Assert: Operation artifacts returned, no git artifact
}

func TestWithGitWorkspace_RestoreFailure_ReturnsNoArtifacts(t *testing.T) {
    // Setup: Git restore fails before operation runs
    // Assert: Empty artifact list, error returned
}
```

### Integration Tests

**File:** `recipe-worker/pkg/ops/activity_integration_test.go`

```go
func TestCodexOperation_FailureWithStdout(t *testing.T) {
    // Setup: Codex operation produces stdout then fails
    // Assert: Stdout artifact is preserved and accessible
}
```

## Edge Cases

### Scenario 1: Operation Adds Artifacts Then Fails
- Both artifacts are returned alongside error
- Error is propagated with artifacts attached

### Scenario 2: Git Restore Fails
- No artifacts available (operation never ran)
- Return empty artifact list with error

### Scenario 3: Git Persist Fails After Successful Operation
- Operation artifacts are preserved
- Git thin pack artifact is NOT added (persist failed)
- Error returned with operation artifacts attached

## Success Criteria

1. ✅ Failed operations return all artifacts added before failure
2. ✅ Git persist failures preserve operation artifacts
3. ✅ Original errors are never masked by artifact handling
4. ✅ Artifact cleanup is invoked by SWF for all returned artifacts
5. ✅ No resource leaks from failed operations
6. ✅ All unit and integration tests pass
7. ✅ No performance regression in error handling paths

## References

### Code Locations

- `recipe-worker/pkg/ops/activity_registry.go:136-212` - Task execution (requires change)
- `recipe-worker/pkg/compiler/compiler.go:108-116` - Task invocation (requires change after open questions resolved)
- `recipe-worker/pkg/compiler/job_worker.go:69-72` - Job execution (requires change after open questions resolved)
- `recipe-core/pkg/ops/op_dependencies.go` - OpDependencies interface (no changes)

### Related Specifications

- `CODEX_ARTIFACT_MIGRATION_SPEC.md` - Codex artifact handling
- `GITSTATE_ARTIFACT_MIGRATION_SPEC.md` - Git state artifacts
