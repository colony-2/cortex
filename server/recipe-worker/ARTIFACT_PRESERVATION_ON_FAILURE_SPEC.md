# Artifact Preservation on Task Failure Specification

**Scope:** `/src/server/recipe-worker` module only

## Problem Statement

When a task fails during execution, all output artifacts that have been added to `OpDependencies` are discarded. Operations may produce valuable diagnostic artifacts (logs, stderr, partial results) before failing, but these are lost because `activity_registry.go` returns `nil` artifacts on error instead of calling `GetOutputArtifacts()`.

The SWF framework saves artifacts when TaskWorker.Run() returns TaskData with artifacts, even on errors. We just need to ensure artifacts are returned at the activity level.

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

## Implementation Steps

1. Update `activity_registry.go` with deferred artifact collection
2. Add unit tests for artifact preservation on failure
3. Verify artifact cleanup is invoked by SWF for failed tasks

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
- `recipe-core/pkg/ops/op_dependencies.go` - OpDependencies interface (no changes)

### Related Specifications

- `CODEX_ARTIFACT_MIGRATION_SPEC.md` - Codex artifact handling
- `GITSTATE_ARTIFACT_MIGRATION_SPEC.md` - Git state artifacts
