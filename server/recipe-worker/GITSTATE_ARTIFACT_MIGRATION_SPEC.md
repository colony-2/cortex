# Git State Thin Pack Migration to SWF Artifacts

## Overview

Migrate git thin pack storage from the blob store adapter pattern to SWF artifacts. Each task invocation will receive thin packs as input artifacts and produce them as output artifacts, eliminating the dependency on `StorageAdapter` and `BlobStoreURI` for git state management.

This implementation uses the new `swf.Artifact` cleanup API (`swf.NewArtifactWithCleanup()`) to enable lazy file-based artifacts with automatic temporary directory cleanup.

## Background

Currently, `workspace_controller.go` uses a `StorageAdapter` interface to save and retrieve thin packs from a blob store. The controller calls:
- `adapter.EnsureLocation()` to prepare storage
- `adapter.ListBlobs()` to find existing thin packs
- `adapter.PutBlob()` to save new thin packs

With SWF's artifact system, each task invocation now carries `[]swf.Artifact` inputs and produces `[]swf.Artifact` outputs. We can leverage this to pass git state between task executions without external storage.

**Key advantage:** The SWF artifact cleanup API allows us to create lazy file-based artifacts that stream directly from temporary files without loading into memory. The cleanup callback ensures temporary directories are automatically removed after SWF finishes consuming the artifact.

## Goals

1. **Eliminate blob store dependency** for git state management in `workspace_controller.go`
2. **Use SWF artifacts** to pass thin packs between task executions
3. **Simplify architecture** by removing `StorageAdapter` usage from git operations
4. **Maintain correctness** of git restore/persist operations
5. **Efficient resource usage** via lazy file streaming and automatic cleanup (using `swf.NewArtifactWithCleanup()`)

## Non-Goals

- Backwards compatibility with existing blob store-based workflows (breaking change acceptable)
- Removing `BlobStoreURI` or `ThinPackPath` fields from `GitTaskContext` (kept for future cleanup)
- Changes outside `/src/server/recipe-worker/` and `/src/server/git/`
- Migrating other blob store use cases (only git state)

## Design

### Artifact Naming Convention

Thin pack artifacts will use a reserved sentinel name:

```
__git_state_thin_pack__
```

**Rationale:**
- Simple, unambiguous name that won't conflict with user artifacts
- Prefix `__` indicates system-reserved artifact
- Descriptive enough to understand purpose
- Single fixed name (not hash-based) since only one thin pack is active per invocation

### Activity Registry Integration

The `withGitWorkspace()` function in `activity_registry.go` will manage artifact filtering:

**Before operation execution:**
1. Find `__git_state_thin_pack__` artifact in `inputArtifacts` (if present)
2. Pass thin pack artifact to `controller.Restore()` (or nil if not present)
3. Filter out thin pack artifact from `inputArtifacts` before passing to operation

**After operation execution:**
4. Call `controller.Persist()` to get output thin pack artifact
5. Determine which thin pack to output:
   - If `Persist()` returned a new thin pack → use it (changes were made)
   - If `Persist()` returned nil AND input had thin pack → use input thin pack (pass through, no changes)
   - If `Persist()` returned nil AND no input thin pack → output nil (no git state)
6. Append thin pack artifact to operation's output artifacts (if any)

### Workspace Controller Changes

#### Modified Signatures

```go
// Restore now takes a thin pack artifact instead of using blob store
// If thinPack is nil, skips thin pack restore
func (c *Controller) Restore(ctx context.Context, task *GitTaskContext, thinPack swf.Artifact) error

// Persist now returns a thin pack artifact instead of using blob store
// Returns nil if no changes (HasChanges=false)
func (c *Controller) Persist(ctx context.Context, task *GitTaskContext) (swf.Artifact, error)
```

#### Implementation Changes

**In `Restore()`:**
- Remove `adapterFor()` and `EnsureLocation()` calls
- Remove `ListBlobs()` call
- Accept `thinPack swf.Artifact` parameter (nil means no thin pack available)
- **Lazy optimization:** Check if workspace already at target hash BEFORE touching artifact
- If artifact needed:
  - Create temporary directory: `os.MkdirTemp("", "thin-pack-restore-*")`
  - Extract artifact to temp directory
  - Pass temp directory to `gitcommit.RestoreCommit` as `StorageLocation`
  - Clean up temp directory via defer
- This enables lazy artifact loading - if workspace already at target, artifact never extracted

**In `Persist()`:**
- Remove `adapterFor()` and `EnsureLocation()` calls
- Remove `PutBlob()` call after `PersistCommit`
- Remove `task.ThinPackPath = ...` assignment
- Create temporary directory internally for persist operation
- If `HasChanges=false` or no thin pack written, clean up temp directory and return nil
- If thin pack was written:
  - **Create lazy artifact with cleanup callback** (no memory copy needed)
  - Artifact uses file opener for lazy reading
  - Cleanup callback removes temp directory when SWF is done
  - Return artifact
- Temp directory lifecycle managed by artifact cleanup callback

**In `prepareWorkspace()`:**
- Remove `adapterFor()` and `EnsureLocation()` calls for blob store
- Keep workspace cloning and setup logic

### Temporary File Management

The controller manages its own temporary directories:

**In `Restore()`:**
```go
func (c *Controller) Restore(ctx context.Context, task *GitTaskContext, thinPack swf.Artifact) error {
    // ... prepare workspace ...

    // LAZY OPTIMIZATION: Check if already at target before touching artifact
    current, err := common.GetCommitHash(ctx, task.GetWorktreePath(), "HEAD")
    if err != nil {
        return err
    }
    if hashesEqual(current, targetHash) {
        // Already at target - never touch the artifact!
        return c.ensureCleanAfterRestore(ctx, task)
    }

    // Now we need the thin pack
    if thinPack == nil {
        return nil // Can't restore without thin pack
    }

    // Create temp dir for this restore operation
    thinPackDir, err := os.MkdirTemp("", "thin-pack-restore-*")
    if err != nil {
        return err
    }
    defer os.RemoveAll(thinPackDir)

    // Extract artifact to temp directory
    if err := extractArtifactToDir(thinPack, thinPackDir); err != nil {
        return err
    }

    // Restore using thin pack
    restoreInput := gitcommit.RestoreCommitActivity{
        RepoPath:        task.GetWorktreePath(),
        TargetCommit:    task.GetPersistHash(),
        RootHash:        rootHash,
        StorageLocation: thinPackDir,
        Force:           true,
    }
    if _, err := gitcommit.RestoreCommit(ctx, restoreInput); err != nil {
        return err
    }

    return c.ensureCleanAfterRestore(ctx, task)
}
```

**In `Persist()`:**
```go
func (c *Controller) Persist(ctx context.Context, task *GitTaskContext) (swf.Artifact, error) {
    // ... prepare ...

    thinPackDir, err := os.MkdirTemp("", "thin-pack-persist-*")
    if err != nil {
        return nil, err
    }

    // ... call PersistCommit ...

    if !output.HasChanges || output.ThinPackPath == "" {
        os.RemoveAll(thinPackDir) // Clean up immediately if no artifact
        return nil, nil
    }

    // Create lazy artifact with cleanup callback
    // Temp directory will be cleaned up by SWF after artifact is consumed
    artifact := swf.NewArtifactWithCleanup(
        "__git_state_thin_pack__",
        func() (io.ReadCloser, int64, error) {
            f, err := os.Open(output.ThinPackPath)
            if err != nil {
                return nil, 0, err
            }
            info, err := f.Stat()
            if err != nil {
                f.Close()
                return nil, 0, err
            }
            return f, info.Size(), nil
        },
        func() error {
            return os.RemoveAll(thinPackDir)
        },
    )

    return artifact, nil
    // Temp dir stays alive, cleaned up by artifact cleanup callback
}
```

### Artifact Visibility

- **Input artifacts:** Thin pack artifact is **hidden** from the operation (filtered out before creating `OpDependencies`)
- **Output artifacts:** Operation-produced artifacts + thin pack artifact (new or passed through)
- **No conflicts:** Operations cannot create artifacts named `__git_state_thin_pack__` (reserved)

This ensures operations remain unaware of git state management.

### Thin Pack Pass-Through Logic

Thin packs represent the current git state. Even if a task makes no changes, subsequent tasks need to know what state to restore.

**Pass-through behavior:**
- If `Persist()` returns a new thin pack (changes made) → output the new one
- If `Persist()` returns nil (no changes) and we had input → output the **same input artifact** (by reference)
- If `Persist()` returns nil and no input → output nil (no git state in workflow)

**Why pass through the same artifact reference?**
- Avoids re-uploading the same data to SWF storage
- SWF can detect artifact identity and reuse existing storage
- Maintains git state continuity across the entire workflow chain

**Example workflow:**
```
Task A (changes) → Task B (no changes) → Task C (changes)
```
- Task A creates thin pack → outputs it (uploaded to SWF storage)
- Task B makes no changes → outputs **same artifact reference** (no re-upload)
- Task C tries to restore → succeeds (has Task A's thin pack from storage)

## Implementation Plan

### Phase 1: Update `workspace_controller.go` in `/src/server/git/pkg/gitstate/`

#### 1.1 Modify `Restore()` method

**Changes:**
- Change signature to accept `thinPack swf.Artifact` parameter instead of using blob store
- Remove lines calling `c.adapterFor(task.GetBlobStoreURI())`
- Remove lines calling `adapter.EnsureLocation()`
- Remove lines calling `adapter.ListBlobs()`
- Add early return check: if workspace already at target hash, return without touching artifact (lazy optimization)
- If restore needed and `thinPack != nil`:
  - Create temp directory: `os.MkdirTemp("", "thin-pack-restore-*")`
  - Add defer cleanup: `defer os.RemoveAll(thinPackDir)`
  - Extract artifact to temp directory
  - Pass temp directory to `gitcommit.RestoreCommit()` as `StorageLocation`
- If `thinPack == nil`, skip thin pack restore (return nil or appropriate error)

**Signature change:**
```go
func (c *Controller) Restore(ctx context.Context, task *GitTaskContext, thinPack swf.Artifact) error
```

#### 1.2 Modify `Persist()` method

**Changes:**
- Change signature to return `swf.Artifact` instead of using blob store
- Remove lines calling `c.adapterFor(task.GetBlobStoreURI())`
- Remove lines calling `adapter.EnsureLocation()`
- Create temp directory internally: `os.MkdirTemp("", "thin-pack-persist-*")`
- **Do NOT add defer cleanup** - temp dir managed by artifact cleanup callback
- Remove `adapter.PutBlob()` call after `gitcommit.PersistCommit()`
- Remove `task.ThinPackPath = ...` assignment
- If `!output.HasChanges` or `output.ThinPackPath == ""`:
  - Clean up temp directory immediately: `os.RemoveAll(thinPackDir)`
  - Return `nil, nil`
- If thin pack written:
  - Create lazy artifact with cleanup: `swf.NewArtifactWithCleanup(...)`
  - File opener returns `os.Open(output.ThinPackPath)` for lazy reading
  - Cleanup callback removes temp directory: `os.RemoveAll(thinPackDir)`
  - Return artifact

**Signature change:**
```go
func (c *Controller) Persist(ctx context.Context, task *GitTaskContext) (swf.Artifact, error)
```

#### 1.3 Update `prepareWorkspace()` method

**Changes:**
- Remove `c.adapterFor()` call
- Remove `adapter.EnsureLocation()` call
- Keep workspace cloning logic

#### 1.4 Add helper function

```go
// extractArtifactToDir extracts an artifact to a directory
func extractArtifactToDir(art swf.Artifact, destDir string) error {
    reader, err := art.Open()
    if err != nil {
        return fmt.Errorf("open artifact: %w", err)
    }
    defer reader.Close()

    // Use a fixed filename since artifact name is sentinel
    // Could also parse filename from Content-Disposition or metadata if available
    destPath := filepath.Join(destDir, "thin-pack.pack")

    f, err := os.Create(destPath)
    if err != nil {
        return fmt.Errorf("create file: %w", err)
    }
    defer f.Close()

    if _, err := io.Copy(f, reader); err != nil {
        return fmt.Errorf("copy artifact data: %w", err)
    }

    return nil
}
```

### Phase 2: Update `activity_registry.go` in `/src/server/recipe-worker/pkg/ops/`

#### 2.1 Modify `withGitWorkspace()` function

**Current signature:**
```go
func withGitWorkspace(deps ops.ServiceDependencies2, reg ActivityRegistration, controller *gitstate.Controller) func(context.Context, ActivityInvocationRequest, []swf.Artifact) (ActivityInvocationOutput, []swf.Artifact, error)
```

**Implementation changes:**

1. **Find and filter input thin pack artifact:**
   ```go
   var thinPackArtifact swf.Artifact
   var nonThinPackArtifacts []swf.Artifact

   for _, art := range inputArtifacts {
       if art.Name() == "__git_state_thin_pack__" {
           thinPackArtifact = art
       } else {
           nonThinPackArtifacts = append(nonThinPackArtifacts, art)
       }
   }
   ```

2. **Call Restore with artifact (or nil):**
   ```go
   if err := controller.Restore(context.Background(), &req.GitTaskContext, thinPackArtifact); err != nil {
       return zero, nil, err
   }
   ```

3. **Build OpDependencies with filtered artifacts:**
   ```go
   db := deps.Database()
   if tx, ok := swf.TxFromCtx(ctx); ok && tx != nil {
       db = tx
   }
   opDeps := ops.NewOpDependenciesBuilder().
       WithArtifacts(nonThinPackArtifacts). // Thin pack hidden from operation
       WithDatabase(db).
       WithWorkflowControl(deps.WorkflowControl()).
       Build()
   ```

4. **Execute operation:**
   ```go
   outputData, err := reg.Step.Invoke(opDeps, ctx, req.Input)
   if err != nil {
       return zero, nil, err
   }
   ```

5. **Call Persist (returns artifact or nil):**
   ```go
   outputThinPack, err := controller.Persist(context.Background(), &req.GitTaskContext)
   if err != nil {
       return zero, nil, err
   }
   ```

6. **Determine final thin pack to output (with pass-through logic):**
   ```go
   var finalThinPack swf.Artifact
   if outputThinPack != nil {
       // Persist created a new thin pack (changes were made)
       finalThinPack = outputThinPack
   } else if thinPackArtifact != nil {
       // No changes, but we had an input thin pack - pass through the SAME artifact
       // This avoids re-uploading; SWF can reuse the existing artifact
       // Next task will need the git state
       finalThinPack = thinPackArtifact
   }
   // else: no input, no output - finalThinPack stays nil
   ```

7. **Append output thin pack artifact:**
   ```go
   outputArtifacts := opDeps.GetOutputArtifacts()

   if finalThinPack != nil {
       outputArtifacts = append(outputArtifacts, finalThinPack)
   }
   ```

8. **Return with artifacts:**
   ```go
   parentRef := ""
   if req.GitTaskContext.PersistHash == "" {
       parentRef = req.GitTaskContext.BaseRef
   }

   return ActivityInvocationOutput{
       OpOutput: outputData,
       GitResult: contextual.GitCommitContext{
           PersistHash: req.GitTaskContext.PersistHash,
           ParentHash:  req.GitTaskContext.ParentHash,
           ParentRef:   parentRef,
       },
       NextTask: reg.NextTaskType,
   }, outputArtifacts, nil
   ```

#### 2.2 No additional helper functions needed

The controller now manages all artifact<->file conversions internally.

### Phase 3: Update Tests

#### 3.1 `workspace_controller_test.go`

Update all test cases to:
- Pass `swf.Artifact` parameter to `Restore()` calls (create mock artifacts)
- Update `Persist()` calls to expect `swf.Artifact` return value
- Verify no blob store calls are made

New test cases:
- `TestRestore_NilThinPackArtifact` - verify graceful handling when artifact is nil
- `TestRestore_WorkspaceAlreadyAtTarget` - verify artifact not opened when already at target (lazy optimization)
- `TestPersist_ReturnsNilWhenNoChanges` - verify nil artifact returned when HasChanges=false, temp dir cleaned
- `TestPersist_ReturnsArtifactWithCleanup` - verify artifact created with cleanup callback
- `TestPersist_ArtifactLazyReading` - verify artifact streams from file (not loaded in memory)
- `TestPersist_CleanupRemovesTempDir` - verify cleanup callback removes temp directory
- `TestPersist_CleanupCalledBySWF` - verify SWF engine calls cleanup after consuming artifact

#### 3.2 `activity_registry_test.go`

New test cases:
- `TestWithGitWorkspace_ThinPackArtifactFiltering` - verify thin pack filtered from input artifacts
- `TestWithGitWorkspace_ThinPackArtifactPassedToRestore` - verify artifact passed to Restore()
- `TestWithGitWorkspace_NewThinPackAppended` - verify new thin pack appended when Persist creates one
- `TestWithGitWorkspace_SameThinPackPassedThrough` - **CRITICAL: verify the EXACT SAME artifact object (pointer equality) is returned when no changes, not a copy. Use `inputArt == outputArt` to verify identity.**
- `TestWithGitWorkspace_NoThinPackWhenNoInput` - verify nil output when no input and Persist returns nil
- `TestWithGitWorkspace_NewThinPackReplacesInput` - verify new thin pack used instead of input when changes made
- `TestWithGitWorkspace_ThinPackHiddenFromOperation` - verify operation doesn't see thin pack artifact
- `TestWithGitWorkspace_NonThinPackArtifactsPassThrough` - verify other artifacts passed to operation unchanged

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Temp directory creation fails (in controller) | Return error immediately |
| Artifact extraction fails (in controller) | Return error, defer cleanup handles temp dir |
| Artifact read fails (in controller) | Return error, defer cleanup handles temp dir |
| Restore fails | Return error to caller, controller cleanup handled |
| Operation execution fails | Return error to caller |
| Persist fails | Return error to caller, controller cleanup handled (if temp dir created) |
| Nil thin pack artifact in Restore | Skip thin pack restore gracefully |
| File open fails in Persist cleanup | Error logged by SWF, does not fail workflow |

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| No input artifacts | Pass nil to Restore, skip thin pack restore. If Persist creates one, output it |
| No changes with input thin pack | **Pass through same input artifact by reference** (no re-upload) |
| No changes without input thin pack | Output nil (no git state), temp dir cleaned immediately |
| Changes made with input thin pack | Persist creates new thin pack, output it (replaces input) |
| Changes made without input thin pack | Persist creates new thin pack, output it |
| Multiple input artifacts | Filter out `__git_state_thin_pack__`, pass rest to operation |
| Thin pack files | Lazy file reading via artifact opener (no memory duplication) |
| Workspace already at target | Restore never opens artifact (lazy optimization) |
| Concurrent task executions | Each Restore/Persist creates isolated temp directory |
| Temp directory cleanup | Managed by artifact cleanup callback after SWF consumption |

## Scope Confirmation

### Changes Required in `/src/server/recipe-worker/`

1. `pkg/ops/activity_registry.go` - Update `withGitWorkspace()` function
2. `pkg/ops/activity_registry_test.go` - Add new test cases

### Changes Required in `/src/server/git/pkg/gitstate/`

1. `workspace_controller.go` - Update `Restore()`, `Persist()`, and `prepareWorkspace()` methods
2. `workspace_controller_test.go` - Update tests for new signatures

### No External Changes Required

✅ **Confirmed:** All changes can be completed within `/src/server/recipe-worker/` and `/src/server/git/` packages. No changes needed to:
- External APIs
- Database schemas
- SWF workflow definitions
- Other services or modules

The SWF artifact system is already in place and being used by the activity invocation system, so we're simply leveraging it for git state.

## Migration Path

1. **Implement changes** in feature branch
2. **Run tests** to ensure correctness
3. **Deploy** (breaking change - in-flight workflows may fail)
4. **Monitor** for issues

**Note:** This is a breaking change. Existing in-flight workflows will fail and need to be restarted after deployment.

## Benefits

1. **Simpler architecture** - One less abstraction layer (StorageAdapter)
2. **Better encapsulation** - Git state travels with task execution
3. **Reduced I/O** - No external blob store round trips, lazy file streaming
4. **Clearer ownership** - SWF manages artifact lifecycle including cleanup
5. **Easier testing** - No need to mock blob store adapters
6. **Efficient resource usage** - Lazy file reading (no memory duplication), automatic cleanup
7. **Lazy optimizations** - Input artifacts not touched if workspace already at target

## Risks

1. **Breaking change** - Existing workflows will fail (acceptable per requirements)
2. **Artifact size limits** - SWF may have size limits for artifacts (thin packs are typically small, few KB-MB)
3. **Cleanup timing** - Temp directories held until SWF finishes with artifact (typically seconds to minutes, acceptable)

## Implementation Notes

### Cleanup API Usage

The cleanup callback is called by SWF after the artifact has been uploaded to storage. Typical lifecycle:

```
1. Persist() creates temp dir and calls PersistCommit()
2. Persist() returns artifact with cleanup callback
3. Activity registry appends artifact to output
4. SWF uploads artifact to storage backend
5. SWF calls artifact.Cleanup() → removes temp directory
```

**Cleanup timing:** Temp directories live for the duration of the upload (typically seconds to minutes), which is acceptable for thin packs (typically < 10MB).

**Error handling:** Cleanup errors are logged by SWF but do not fail the task execution. The temp directory lifecycle is "best effort" - if cleanup fails, the OS will eventually clean up `/tmp`.

## Success Criteria

- [ ] All `workspace_controller.go` methods work without `StorageAdapter`
- [ ] `withGitWorkspace()` correctly filters and passes through thin pack artifacts
- [ ] `Persist()` uses `swf.NewArtifactWithCleanup()` for lazy file streaming
- [ ] All existing tests pass with updated signatures
- [ ] New tests verify artifact handling and cleanup behavior
- [ ] **Test verifies same artifact reference returned when no changes (not re-uploaded)**
- [ ] No blob store calls made during git operations
- [ ] Thin pack artifacts flow between task executions with pass-through support
- [ ] Temporary directories cleaned up via artifact cleanup callbacks
- [ ] Operations remain unaware of thin pack artifacts
- [ ] No memory duplication (thin packs streamed from files)
- [ ] Lazy optimizations work (artifacts not opened if workspace already at target)
