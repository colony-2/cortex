# Codex Output Artifact Migration to SWF Artifacts

## Overview

Migrate codex stdout and stderr storage from the blob store pattern to SWF artifacts. The codex.exec operation will produce two output artifacts (stdout and stderr) instead of uploading to a blob store and embedding stderr in the response, eliminating the dependency on `BlobstoreURI` and `BlobStore` for output capture management.

This implementation uses `swf.NewArtifact()` with a cleanup callback to enable lazy file-based artifacts with automatic temporary directory cleanup.

## Background

Currently, the codex.exec operation:
1. Captures stdout to a temporary file during execution (`execute.go`)
2. Captures stderr to an in-memory buffer during execution
3. Uses `BlobStore.Put()` to upload the stdout file to a blob store location
4. Returns `StdoutBlobURI` in the result, pointing to the blob store location
5. Returns `Stderr` as a string field in the result
6. Requires `context.blobstore` (or `context.git.blob_store_uri`) to be provided in the input

With SWF's artifact system, operations can produce `[]swf.Artifact` outputs via `OpDependencies.AddOutputArtifact()`. We can leverage this to return both stdout and stderr as artifacts without external storage.

**Key advantage:** The SWF artifact cleanup API allows us to create lazy file-based artifacts that stream directly from temporary files without loading into memory. The cleanup callback ensures temporary directories are automatically removed after SWF finishes consuming the artifact.

## Goals

1. **Eliminate blob store dependency** for stdout/stderr capture in codex operations
2. **Use SWF artifacts** to return both stdout and stderr as output artifacts
3. **Simplify architecture** by removing `BlobStore` usage and inline stderr from codex execution
4. **Remove all deprecated fields** - clean break from old implementation
5. **Efficient resource usage** via lazy file streaming and automatic cleanup (using `swf.NewArtifact()` with cleanup callback)
6. **Consistent output handling** - both stdout and stderr stored the same way (as artifacts)

## Non-Goals

- Backwards compatibility with existing workflows (breaking change - all old data will be deleted)
- Changes outside `/src/server/ops/pkg/codex/`
- Migrating other blob store use cases (only codex stdout/stderr)

## Design

### Artifact Naming Convention

Codex will produce two artifacts per execution:

**Stdout artifact:**
```
codex_stdout_<timestamp>_<uuid>.jsonl
```

**Stderr artifact:**
```
codex_stderr_<timestamp>_<uuid>.txt
```

**Rationale:**
- Descriptive prefix `codex_stdout_` or `codex_stderr_` clearly indicates purpose
- Same timestamp and UUID used for both artifacts from the same execution (for easy correlation)
- `.jsonl` extension for stdout (structured format)
- `.txt` extension for stderr (unstructured text)
- Unlike git state (which uses a fixed sentinel name), each codex execution produces new artifacts

**Example:**
- `codex_stdout_20260105T143022Z_a1b2c3d4-e5f6-7890-abcd-ef1234567890.jsonl`
- `codex_stderr_20260105T143022Z_a1b2c3d4-e5f6-7890-abcd-ef1234567890.txt`

### Operation Integration

The `runCodexActivity()` function in `op.go` will manage artifact creation:

**Before execution:**
1. Extract `OpDependencies` from invocation context
2. Remove validation requirement for `blobstore` URI (make it optional)
3. Continue with normal codex execution

**After execution:**
4. If execution successful and outputs captured:
   - Create lazy stdout artifact from stdout file using `swf.NewArtifact()` with cleanup callback
   - Create lazy stderr artifact from stderr file using `swf.NewArtifact()` with cleanup callback
   - Add both artifacts to `OpDependencies` via `AddOutputArtifact()`
5. Return result with artifacts

### Execute Function Changes

#### Modified Signature and Implementation

The `Execute()` function in `execute.go` will change to:

**Current behavior:**
- Captures stdout to file
- Captures stderr to in-memory buffer
- Uploads stdout to blob store
- Returns `StdoutBlobURI` and `Stderr` string in result
- Cleans up temp directory immediately after upload

**New behavior:**
- Captures both stdout and stderr to files in temp directory
- Returns file paths and temp directory to caller
- Caller responsible for creating artifacts with cleanup
- No longer interacts with blob store

**Signature change:**
```go
// Execute runs Codex in non-interactive mode and returns the normalized result.
// The caller is responsible for managing the returned stdoutPath, stderrPath, and tempDir.
// If creating artifacts, use the cleanup callback to remove tempDir.
// If not creating artifacts, caller must clean up tempDir immediately.
func Execute(ctx context.Context, opts Options) (Result, string, string, string, error)
```

**Return values:**
1. `Result` - The execution result (without StdoutBlobURI or Stderr)
2. `stdoutPath` - Path to the stdout capture file
3. `stderrPath` - Path to the stderr capture file
4. `tempDir` - Temporary directory that needs cleanup
5. `error` - Any error that occurred

#### Implementation Changes

**In `Execute()` (`execute.go`):**

Change stderr capture from in-memory buffer to file:
```go
// CHANGE collector to write stderr to file instead of buffer
stderrPath := opts.stderrPath(tempDir)
stderrFile, err := os.Create(stderrPath)
if err != nil {
    return Result{}, "", "", "", fmt.Errorf("create stderr capture: %w", err)
}

collector := newOutputCollector(stdoutFile, stderrFile)
```

Update collector to close stderr file:
```go
if closeErr := stderrFile.Close(); closeErr != nil {
    if runErr == nil {
        runErr = fmt.Errorf("close stderr capture: %w", closeErr)
    }
}
```

Remove the blob store upload section (lines 89-95):
```go
// REMOVE THIS:
stdoutID := uuid.NewString()
timestamp := opts.Clock.Now().Format("20060102T150405Z")
relPath := opts.stdoutRelativePath(timestamp, stdoutID)
if _, err := opts.BlobStore.Put(ctx, opts.BlobstoreURI, relPath, stdoutPath); err != nil {
    return Result{}, fmt.Errorf("store stdout: %w", err)
}
result.StdoutBlobURI = joinBlobURI(opts.BlobstoreURI, relPath)
```

Remove stderr string from result building:
```go
// REMOVE from buildResultFromParse:
result := Result{
    Stderr: stderr, // DELETE THIS LINE
}
```

Remove the immediate temp directory cleanup:
```go
// REMOVE THIS:
defer os.RemoveAll(tempDir)
```

Return the paths for caller to manage:
```go
// At the end of Execute():
return result, stdoutPath, stderrPath, tempDir, nil
```

**In `runCodexActivity()` (`op.go`):**

Change how `Execute` is called and handle artifacts:
```go
result, stdoutPath, stderrPath, tempDir, err := executeLibrary(actx, opts)
if err != nil {
    // Clean up immediately on error
    if tempDir != "" {
        os.RemoveAll(tempDir)
    }
    return ExecOpOutput{}, err
}

// Create timestamp and ID for artifact naming (same for both artifacts)
timestamp := time.Now().UTC().Format("20060102T150405Z")
executionID := uuid.NewString()

// Track cleanup state - only cleanup once when all artifacts are consumed
cleanupCalled := &atomic.Bool{}

cleanupFunc := func() error {
    if cleanupCalled.CompareAndSwap(false, true) {
        return os.RemoveAll(tempDir)
    }
    return nil
}

// Create stdout artifact
stdoutArtifact := swf.NewArtifact(
    fmt.Sprintf("codex_stdout_%s_%s.jsonl", timestamp, executionID),
    func() (io.ReadCloser, int64, error) {
        f, err := os.Open(stdoutPath)
        if err != nil {
            return nil, 0, fmt.Errorf("open stdout: %w", err)
        }
        info, err := f.Stat()
        if err != nil {
            f.Close()
            return nil, 0, fmt.Errorf("stat stdout: %w", err)
        }
        return f, info.Size(), nil
    },
    cleanupFunc,
)

// Create stderr artifact
stderrArtifact := swf.NewArtifact(
    fmt.Sprintf("codex_stderr_%s_%s.txt", timestamp, executionID),
    func() (io.ReadCloser, int64, error) {
        f, err := os.Open(stderrPath)
        if err != nil {
            return nil, 0, fmt.Errorf("open stderr: %w", err)
        }
        info, err := f.Stat()
        if err != nil {
            f.Close()
            return nil, 0, fmt.Errorf("stat stderr: %w", err)
        }
        return f, info.Size(), nil
    },
    cleanupFunc,
)

// Add both artifacts to output
if err := inv.AddOutputArtifact(stdoutArtifact); err != nil {
    os.RemoveAll(tempDir) // Clean up on artifact error
    return ExecOpOutput{}, fmt.Errorf("add stdout artifact: %w", err)
}
if err := inv.AddOutputArtifact(stderrArtifact); err != nil {
    os.RemoveAll(tempDir) // Clean up on artifact error
    return ExecOpOutput{}, fmt.Errorf("add stderr artifact: %w", err)
}

output := ExecOpOutput{
    Status:              string(result.Status),
    SessionID:           safeString(result.SessionID),
    AssistantSummary:    safeString(result.AssistantSummary),
    IncompleteReason:    safeString(result.IncompleteReason),
    IncompleteCategory:  safeString(result.IncompleteCategory),
    PendingDependencies: copyDependencies(result.PendingDependencies),
    ErrorMessage:        safeString(result.ErrorMessage),
    // StdoutBlobURI field removed - use output artifacts
    // Stderr field removed - use output artifacts
}
return output, nil
// Temp dir cleaned up by artifact cleanup callback (after both artifacts consumed)
```

### Options Validation Changes

**In `options.go`:**

The `validate()` function currently requires `BlobstoreURI`:
```go
if strings.TrimSpace(o.BlobstoreURI) == "" {
    return errMissingBlobstore
}
```

This validation should be removed:
```go
// REMOVE validation for BlobstoreURI - no longer required
// Keep the field in the struct for transition period but don't require it
```

Also remove the default `BlobStore` assignment since it's no longer used:
```go
// REMOVE THIS:
if o.BlobStore == nil {
    o.BlobStore = fileBlobStore{}
}
```

### Type Changes

**In `types.go`:**

Remove `BlobstoreURI` and `BlobStore` from `Options` struct:
```go
type Options struct {
    Prompt    string
    SessionID string
    Model     string
    ExtraEnv  map[string]string

    WorktreeRoot     string
    CellRelativePath string
    // BlobstoreURI field REMOVED - no longer needed

    Clock            Clock
    RunnerFactory    RunnerFactory
    // BlobStore field REMOVED - no longer needed
    StructuredSchema []byte
}
```

Remove `StdoutBlobURI` and `Stderr` from `Result` struct:
```go
type Result struct {
    Status              Status       `json:"status"`
    SessionID           string       `json:"sessionId"`
    AssistantSummary    string       `json:"assistantSummary,omitempty"`
    IncompleteReason    string       `json:"incompleteReason,omitempty"`
    IncompleteCategory  string       `json:"incompleteCategory,omitempty"`
    PendingDependencies []Dependency `json:"pendingDependencies,omitempty"`
    ErrorMessage        string       `json:"errorMessage,omitempty"`
    // StdoutBlobURI field REMOVED - use artifacts instead
    // Stderr field REMOVED - use artifacts instead
}
```

Remove `BlobStore` interface - no longer needed:
```go
// REMOVE this entire interface:
// type BlobStore interface {
//     Put(ctx context.Context, baseURI, relativePath, sourcePath string) (string, error)
// }
```

### Temporary File Management

The operation manages stdout and stderr file cleanup via shared artifact cleanup callback:

**Lifecycle:**
1. `Execute()` creates temp directory and captures both stdout and stderr to files
2. `Execute()` returns temp dir path, stdout file path, and stderr file path to caller
3. `runCodexActivity()` creates both artifacts with shared cleanup callback
4. Both artifacts added to OpDependencies output
5. SWF uploads both artifacts to storage backend
6. SWF calls cleanup callback (from either artifact) → removes temp directory once

**Shared cleanup:** Both artifacts share the same cleanup callback using atomic bool to ensure temp directory is only removed once, after both artifacts are consumed by SWF.

**Cleanup timing:** Temp directories live for the duration of uploads (typically seconds to minutes), which is acceptable for output files (typically < 1MB each).

**Error handling:** If artifact creation fails or execution errors, caller must clean up temp directory immediately.

## Implementation Plan

### Phase 1: Update `execute.go`

#### 1.1 Modify `Execute()` function

**Changes:**
- Change signature to return `(Result, string, string, error)` - add stdout path and temp dir path
- Remove `defer os.RemoveAll(tempDir)` at the top
- Remove blob store upload section (lines 89-95):
  - Remove `opts.BlobStore.Put()` call
  - Remove `result.StdoutBlobURI = ...` assignment
- Return stdout path and temp dir path to caller:
  ```go
  return result, stdoutPath, tempDir, nil
  ```

**New signature:**
```go
func Execute(ctx context.Context, opts Options) (Result, string, string, error)
```

### Phase 2: Update `op.go`

#### 2.1 Modify `runCodexActivity()` function

**Changes:**
- Remove validation for `blobstore` URI (lines 80-88) completely
- Update `executeLibrary` call to handle new return signature (4 return values)
- Add temp directory cleanup on error path
- Create TWO artifacts using `swf.NewArtifact()` with shared cleanup callback
- Add both artifacts to OpDependencies via `inv.AddOutputArtifact()`
- Remove `StdoutBlobURI` and `Stderr` from output struct
- **Do NOT add defer cleanup** - managed by artifact cleanup callback
- Use atomic bool to ensure cleanup only happens once after both artifacts consumed

**Detailed implementation:**
1. Remove blobstore URI extraction and validation
2. After calling `executeLibrary`:
   ```go
   result, stdoutPath, stderrPath, tempDir, err := executeLibrary(actx, opts)
   if err != nil {
       if tempDir != "" {
           os.RemoveAll(tempDir)
       }
       return ExecOpOutput{}, err
   }
   ```
3. Create shared cleanup callback:
   ```go
   timestamp := time.Now().UTC().Format("20060102T150405Z")
   executionID := uuid.NewString()

   cleanupCalled := &atomic.Bool{}
   cleanupFunc := func() error {
       if cleanupCalled.CompareAndSwap(false, true) {
           return os.RemoveAll(tempDir)
       }
       return nil
   }
   ```
4. Create stdout artifact:
   ```go
   stdoutArtifact := swf.NewArtifact(
       fmt.Sprintf("codex_stdout_%s_%s.jsonl", timestamp, executionID),
       func() (io.ReadCloser, int64, error) {
           f, err := os.Open(stdoutPath)
           if err != nil {
               return nil, 0, fmt.Errorf("open stdout: %w", err)
           }
           info, err := f.Stat()
           if err != nil {
               f.Close()
               return nil, 0, fmt.Errorf("stat stdout: %w", err)
           }
           return f, info.Size(), nil
       },
       cleanupFunc,
   )
   ```
5. Create stderr artifact:
   ```go
   stderrArtifact := swf.NewArtifact(
       fmt.Sprintf("codex_stderr_%s_%s.txt", timestamp, executionID),
       func() (io.ReadCloser, int64, error) {
           f, err := os.Open(stderrPath)
           if err != nil {
               return nil, 0, fmt.Errorf("open stderr: %w", err)
           }
           info, err := f.Stat()
           if err != nil {
               f.Close()
               return nil, 0, fmt.Errorf("stat stderr: %w", err)
           }
           return f, info.Size(), nil
       },
       cleanupFunc,
   )
   ```
6. Add both artifacts to output:
   ```go
   if err := inv.AddOutputArtifact(stdoutArtifact); err != nil {
       os.RemoveAll(tempDir)
       return ExecOpOutput{}, fmt.Errorf("add stdout artifact: %w", err)
   }
   if err := inv.AddOutputArtifact(stderrArtifact); err != nil {
       os.RemoveAll(tempDir)
       return ExecOpOutput{}, fmt.Errorf("add stderr artifact: %w", err)
   }
   ```
7. Build output without `StdoutBlobURI` and `Stderr`:
   ```go
   output := ExecOpOutput{
       Status:              string(result.Status),
       SessionID:           safeString(result.SessionID),
       AssistantSummary:    safeString(result.AssistantSummary),
       IncompleteReason:    safeString(result.IncompleteReason),
       IncompleteCategory:  safeString(result.IncompleteCategory),
       PendingDependencies: copyDependencies(result.PendingDependencies),
       ErrorMessage:        safeString(result.ErrorMessage),
       // StdoutBlobURI field removed
       // Stderr field removed
   }
   ```

### Phase 3: Update `options.go`

#### 3.1 Remove blobstore validation and defaults

**Changes:**
- Remove the validation check for `BlobstoreURI` (lines 24-26)
- Remove the default `BlobStore` assignment (lines 36-38)
- Remove `errMissingBlobstore` error variable (no longer needed)

**Remove:**
```go
if strings.TrimSpace(o.BlobstoreURI) == "" {
    return errMissingBlobstore
}
```

**Remove:**
```go
if o.BlobStore == nil {
    o.BlobStore = fileBlobStore{}
}
```

**Remove:**
```go
var (
    errEmptyPrompt      = errors.New("codex: prompt is required")
    errMissingWorktree  = errors.New("codex: worktree root is required")
    errMissingBlobstore = errors.New("codex: blobstore URI is required") // DELETE THIS
)
```

### Phase 4: Update `types.go`

#### 4.1 Remove blobstore-related fields and types

**Changes:**
- Remove `BlobstoreURI` field from `Options` struct
- Remove `BlobStore` field from `Options` struct
- Remove `BlobStore` interface definition
- Remove `StdoutBlobURI` field from `Result` struct
- Remove `Stderr` field from `Result` struct
- Remove `ExecOpOutput.StdoutBlobURI` field from `op.go`
- Remove `ExecOpOutput.Stderr` field from `op.go`

**In `types.go`, remove from `Options`:**
```go
BlobstoreURI     string // DELETE THIS LINE
BlobStore        BlobStore // DELETE THIS LINE
```

**In `types.go`, remove interface:**
```go
// DELETE THIS ENTIRE INTERFACE:
// BlobStore provides persistence for stdout artifacts.
type BlobStore interface {
    Put(ctx context.Context, baseURI, relativePath, sourcePath string) (string, error)
}
```

**In `types.go`, remove from `Result`:**
```go
StdoutBlobURI       string       `json:"stdoutBlobUri"` // DELETE THIS LINE
Stderr              string       `json:"stderr,omitempty"` // DELETE THIS LINE
```

**In `op.go`, remove from `ExecOpOutput`:**
```go
StdoutBlobURI       string       `json:"stdoutBlobUri"` // DELETE THIS LINE
Stderr              string       `json:"stderr"` // DELETE THIS LINE
```

### Phase 5: Delete unused files

#### 5.1 Delete `blobstore.go`

**Changes:**
- Delete the entire `blobstore.go` file
- Remove all blobstore-related helper functions

**File to delete:**
- `pkg/codex/blobstore.go` - Entire file deleted

### Phase 6: Update Tests

#### 6.1 `execute_test.go`

Update all test cases to:
- Handle new `Execute()` signature with stdout path, stderr path, and temp dir returns
- Remove all blob store mocks and expectations
- Clean up temp directories in test teardown

New test cases:
- `TestExecute_ReturnsStdoutPath` - verify stdout path is returned
- `TestExecute_ReturnsStderrPath` - verify stderr path is returned
- `TestExecute_ReturnsTempDir` - verify temp dir path is returned
- `TestExecute_StderrCapturedToFile` - verify stderr written to file, not buffer
- `TestExecute_TempDirNotCleanedByExecute` - verify Execute doesn't clean up (caller's responsibility)

#### 6.2 `op_test.go`

Update all test cases to:
- Remove blobstore URI from context inputs entirely
- Verify TWO output artifacts are created (stdout and stderr)
- Verify artifacts have correct name format (matching timestamp and ID)
- Verify artifacts stream from files (lazy reading)
- Verify temp directory cleanup callback is set and shared
- Remove any assertions on `StdoutBlobURI` or `Stderr` fields (fields no longer exist)

New test cases:
- `TestRunCodexActivity_CreatesBothArtifacts` - verify both stdout and stderr artifacts created
- `TestRunCodexActivity_ArtifactNameFormat` - verify naming convention for both
- `TestRunCodexActivity_ArtifactsShareTimestampAndID` - verify same timestamp/ID used
- `TestRunCodexActivity_ArtifactLazyReading` - verify file streaming (not loaded in memory)
- `TestRunCodexActivity_SharedCleanup` - verify cleanup callback shared and only called once
- `TestRunCodexActivity_NoBlobstoreRequired` - verify works without blobstore URI
- `TestRunCodexActivity_CleanupOnError` - verify temp dir cleaned immediately on execution error
- `TestRunCodexActivity_CleanupOnArtifactError` - verify temp dir cleaned if AddOutputArtifact fails

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Temp directory creation fails | `Execute()` returns error immediately |
| Stdout file creation fails | `Execute()` returns error, defer cleanup handles temp dir |
| Stderr file creation fails | `Execute()` returns error, defer cleanup handles temp dir |
| Codex execution fails | `Execute()` returns error, caller cleans up temp dir |
| Artifact creation fails | Clean up temp dir immediately, return error |
| `AddOutputArtifact()` fails for stdout | Clean up temp dir immediately, return error |
| `AddOutputArtifact()` fails for stderr | Clean up temp dir immediately, return error |
| File open fails in artifact opener | Error returned when SWF attempts to open, logged by SWF |
| Cleanup callback called twice | Atomic bool prevents double cleanup |
| Cleanup callback fails | Error logged by SWF, does not fail workflow |

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Empty stdout output | Stdout artifact created with 0-byte file |
| Empty stderr output | Stderr artifact created with 0-byte file |
| Large stdout (>100MB) | Lazy streaming prevents memory issues |
| Large stderr (>100MB) | Lazy streaming prevents memory issues |
| Concurrent executions | Each creates isolated temp directory with unique timestamp/ID |
| Execution error before files created | Temp dir cleaned immediately by caller |
| SWF consumes stdout before stderr | Cleanup waits until both consumed |
| SWF consumes stderr before stdout | Cleanup waits until both consumed |
| Cleanup callback error | SWF logs error, OS eventually cleans `/tmp` |

## Scope Confirmation

### Changes Required in `/src/server/ops/pkg/codex/`

1. `execute.go` - Update `Execute()` signature, change stderr to file capture, remove blob store
2. `op.go` - Update `runCodexActivity()` to create TWO artifacts, remove `StdoutBlobURI` and `Stderr` from `ExecOpOutput`
3. `options.go` - Remove blobstore validation and error variable, add `stderrPath()` helper
4. `types.go` - Remove `BlobstoreURI`, `BlobStore`, `StdoutBlobURI`, `Stderr` fields and `BlobStore` interface
5. `runner.go` or collector file - Update output collector to write stderr to file
6. `blobstore.go` - **DELETE entire file**
7. `execute_test.go` - Update tests for new signature and stderr file capture
8. `op_test.go` - Update tests to verify TWO artifact creation

### No External Changes Required

✅ **Confirmed:** All changes can be completed within `/src/server/ops/pkg/codex/` package. No changes needed to:
- External APIs
- Database schemas
- SWF workflow definitions
- Other services or modules

The SWF artifact system is already in place and being used by the operation system via `OpDependencies`.

## Migration Path

1. **Delete all existing data** - Clear any in-flight workflows and old blob store data
2. **Implement changes** in feature branch
3. **Run tests** to ensure correctness
4. **Update documentation** to reflect artifact-based stdout
5. **Deploy** (breaking change - all old workflows incompatible)
6. **Monitor** for issues

**Note:** This is a **complete breaking change**. All existing workflows will fail and must be restarted. No backwards compatibility is maintained.

## Benefits

1. **Simpler architecture** - No blob store abstraction needed
2. **Better encapsulation** - Both stdout and stderr travel with task execution as artifacts
3. **Reduced I/O** - No external blob store round trips, lazy file streaming
4. **Clearer ownership** - SWF manages artifact lifecycle including cleanup
5. **Easier testing** - No need to mock blob store
6. **Efficient resource usage** - Lazy file reading (no memory duplication), automatic cleanup
7. **Consistent pattern** - Matches git state artifact migration pattern
8. **Consistent output handling** - Stdout and stderr handled identically (both as artifacts)
9. **Better scalability** - Large stderr no longer bloats response payload

## Risks

1. **Complete breaking change** - All existing workflows will fail and must be restarted
2. **Data loss** - All old stdout in blob store will become inaccessible
3. **Artifact size limits** - SWF may have size limits for artifacts (stdout typically small, < 1MB)
4. **Cleanup timing** - Temp directories held until SWF finishes with artifact (typically seconds, acceptable)
5. **No rollback path** - Once deployed, cannot roll back without data loss

## Implementation Notes

### Cleanup API Usage

The cleanup callback is called by SWF after the artifact has been uploaded to storage. Typical lifecycle:

```
1. Execute() creates temp dir and captures stdout
2. Execute() returns stdout path and temp dir path
3. runCodexActivity() creates artifact with cleanup callback
4. Artifact added to OpDependencies output
5. SWF uploads artifact to storage backend
6. SWF calls artifact.Cleanup() → removes temp directory
```

**Cleanup timing:** Temp directories live for the duration of the upload (typically seconds to minutes for < 1MB files), which is acceptable.

**Error handling:** Cleanup errors are logged by SWF but do not fail the task execution. The temp directory lifecycle is "best effort" - if cleanup fails, the OS will eventually clean up `/tmp`.

### Artifact Naming

Unlike git state (which uses a fixed sentinel name `__git_state_thin_pack__` because only one thin pack is active per invocation), codex stdout uses descriptive unique names because:
- Each execution produces new stdout content
- Multiple codex operations may run in sequence
- Timestamp + UUID provides traceability and uniqueness
- `.jsonl` extension indicates structured format

### Input Artifacts

Currently, codex.exec does not consume input artifacts. If future versions need to read artifacts (e.g., previous execution stdout), they can use:
```go
inputArtifacts := inv.GetInputArtifacts()
// Filter and process as needed
```

## Success Criteria

- [ ] `Execute()` no longer calls `BlobStore.Put()`
- [ ] `Execute()` returns stdout path, stderr path, and temp directory path
- [ ] `Execute()` captures stderr to file instead of buffer
- [ ] `runCodexActivity()` creates TWO artifacts using `swf.NewArtifact()` with shared cleanup callback
- [ ] Both artifacts use lazy file reading (no memory copy)
- [ ] Shared cleanup callback removes temp directory only once
- [ ] Atomic bool prevents double cleanup
- [ ] `StdoutBlobURI` field removed from all structs
- [ ] `Stderr` field removed from all structs
- [ ] `BlobstoreURI` field removed from `Options`
- [ ] `BlobStore` interface and field removed
- [ ] `blobstore.go` file deleted
- [ ] Operation works without any blobstore configuration
- [ ] All existing tests pass with updated implementation
- [ ] New tests verify TWO artifact creation and cleanup
- [ ] No blob store calls made during codex operations
- [ ] Temp directories cleaned up via artifact cleanup callbacks
- [ ] No memory duplication (both outputs streamed from files)
- [ ] Artifact naming follows conventions:
  - Stdout: `codex_stdout_<timestamp>_<uuid>.jsonl`
  - Stderr: `codex_stderr_<timestamp>_<uuid>.txt`
- [ ] Both artifacts from same execution share timestamp and UUID

## Future Enhancements

After successful migration, consider:

1. **Artifact compression** - Compress stdout/stderr before creating artifacts if size is concern
2. **Artifact metadata** - Add metadata to artifacts (execution time, model used, etc.)
3. **Additional output artifacts** - Separate other outputs (e.g., traces, metrics) into their own artifacts
4. **Artifact retention policies** - Configure how long SWF keeps output artifacts
5. **Conditional stderr** - Only create stderr artifact if non-empty (optimization)
