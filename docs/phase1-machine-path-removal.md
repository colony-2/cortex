# Phase 1: Remove Machine-Specific Paths from ActivityInvocationRequest

## Goal
Enable job mobility by removing machine-specific paths (WorktreePath, ThinPackPath) from ActivityInvocationRequest serialization.

## Problem
Currently, `ActivityInvocationRequest` contains `GitTaskContext` which includes:
- `WorktreePath` - machine-specific local filesystem path
- `ThinPackPath` - no longer used (moved to artifact storage)

This prevents jobs from moving between machines since these paths are hardcoded in the serialized request.

## Solution
1. Create `GlobalGitTaskContext` with only machine-independent fields
2. Use **sentinel value pattern** for template references to `worktree_path`
3. Move `WorktreePath` to `OpDependencies` (built locally per-machine)
4. **Hydrate sentinels** at execution time with actual local worktree path
5. Remove unused `ThinPackPath` field entirely

## Sentinel Value Flow

**The Challenge:**
Op inputs may reference worktree path via templates: `{{ environment.worktree_path }}`. Template resolution happens at **COMPILE TIME** (before serialization), but the actual worktree path must be determined at **EXECUTION TIME** on the target machine.

**The Solution:**
```
┌─────────────────────────────────────────────────────────────────────┐
│ COMPILE TIME (recipe-worker/compiler.go)                            │
│                                                                      │
│ 1. Op input template: {"path": "{{ environment.worktree_path }}"}  │
│ 2. Template resolution calls EnvironmentContext.GetWorktreePath()   │
│ 3. Returns sentinel: "__COLONY_WORKTREE_PATH__"                    │
│ 4. Resolved input: {"path": "__COLONY_WORKTREE_PATH__"}            │
│ 5. ActivityInvocationRequest created with sentinel in Input field   │
│ 6. Serialized to JSON (sentinel is machine-independent string)      │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ (Job sent to any worker)
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│ EXECUTION TIME (recipe-worker/activity_registry.go)                 │
│                                                                      │
│ 1. Deserialize ActivityInvocationRequest                            │
│ 2. Input still contains: {"path": "__COLONY_WORKTREE_PATH__"}      │
│ 3. Create actual local worktree: /tmp/recipe-worktree-abc123/      │
│ 4. replaceSentinels() walks Input and replaces sentinel             │
│ 5. Hydrated input: {"path": "/tmp/recipe-worktree-abc123/"}        │
│ 6. Execute op with hydrated input (real local path)                 │
└─────────────────────────────────────────────────────────────────────┘
```

**Key Benefits:**
- ✅ Templates can reference `{{ environment.worktree_path }}`
- ✅ Sentinel is machine-independent (safe to serialize)
- ✅ Actual path determined on target machine
- ✅ Ops receive real path in their inputs
- ✅ Works seamlessly with Phase 2 caching (sentinel → cached path)

---

## Implementation Steps

### 1. Create GlobalGitTaskContext
**File:** `/src/server/git/pkg/gitstate/git_task_context.go`

```go
// GlobalGitTaskContext contains machine-independent git context (serializable)
type GlobalGitTaskContext struct {
    BaseRepo         string
    BaseRef          string
    ResolvedBaseHash string
    PersistHash      string
    ParentHash       string
    // WorktreePath removed - stored in OpDependencies
    // ThinPackPath removed - no longer used
    TicketID         string
    CellName         string
    CellPath         string
    GitAuthor        string
    NodePath         string
    InvokeSeq        int64
    InvokeHash       string
}

// Constructor for GlobalGitTaskContext
func NewGlobalGitTaskContext(tec contextual.TaskExecutionContext) *GlobalGitTaskContext {
    return &GlobalGitTaskContext{
        BaseRepo:         tec.GitBase.BaseRepo,
        BaseRef:          tec.GitBase.BaseRef,
        ResolvedBaseHash: tec.GitBase.ResolvedBaseHash,
        GitAuthor:        tec.GitBase.GitAuthor,
        PersistHash:      tec.GitCommit.PersistHash,
        ParentHash:       tec.GitCommit.ParentHash,
        TicketID:         tec.Actor.TicketID,
        CellName:         tec.Workflow.CellName,
        CellPath:         tec.Workflow.CellPath,
        NodePath:         tec.Invocation.NodePath,
        InvokeSeq:        tec.Invocation.InvokeSeq,
        InvokeHash:       tec.Invocation.Hash(),
    }
}

// GitTaskContext is used internally by workspace_controller
// Uses POINTER embedding to share the global context
type GitTaskContext struct {
    *GlobalGitTaskContext  // Pointer embedding - shares data with global context
    WorktreePath string
}

// Constructor that preserves existing behavior
func NewGitTaskContext(tec contextual.TaskExecutionContext) *GitTaskContext {
    return &GitTaskContext{
        GlobalGitTaskContext: NewGlobalGitTaskContext(tec),
        WorktreePath:         tec.Environment.WorktreePath,
    }
}

// Keep ALL 14 existing getter methods - controller and other code depends on them
func (c *GitTaskContext) GetBaseRepo() string         { return c.GlobalGitTaskContext.BaseRepo }
func (c *GitTaskContext) GetBaseRef() string          { return c.GlobalGitTaskContext.BaseRef }
func (c *GitTaskContext) GetResolvedBaseHash() string { return c.GlobalGitTaskContext.ResolvedBaseHash }
func (c *GitTaskContext) GetPersistHash() string      { return c.GlobalGitTaskContext.PersistHash }
func (c *GitTaskContext) GetParentHash() string       { return c.GlobalGitTaskContext.ParentHash }
func (c *GitTaskContext) GetWorktreePath() string     { return c.WorktreePath }
// Note: GetThinPackPath() will be REMOVED as part of this refactor
func (c *GitTaskContext) GetTicketID() string         { return c.GlobalGitTaskContext.TicketID }
func (c *GitTaskContext) GetCellName() string         { return c.GlobalGitTaskContext.CellName }
func (c *GitTaskContext) GetCellPath() string         { return c.GlobalGitTaskContext.CellPath }
func (c *GitTaskContext) GetGitAuthor() string        { return c.GlobalGitTaskContext.GitAuthor }
func (c *GitTaskContext) GetNodePath() string         { return c.GlobalGitTaskContext.NodePath }
func (c *GitTaskContext) GetInvokeSeq() int64         { return c.GlobalGitTaskContext.InvokeSeq }
func (c *GitTaskContext) GetInvokeHash() string       { return c.GlobalGitTaskContext.InvokeHash }
```

**Critical Notes:**
- ⚠️ Must use **pointer embedding** (`*GlobalGitTaskContext`) not value embedding
- Controller.Restore() and Persist() update fields on context
- Pointer embedding shares data bidirectionally
- All 14 getter methods must be preserved for backward compatibility

---

### 2. Add WorktreePath to OpDependencies
**File:** `/src/server/recipe-core/pkg/ops/op_dependencies.go`

```go
type OpDependencies interface {
    Database() *gorm.DB
    WorkflowControl() workflowctl.WorkflowControl
    GetInputArtifacts() []swf.Artifact
    AddOutputArtifact(swf.Artifact) error
    GetOutputArtifacts() []swf.Artifact
    WorktreePath() string // NEW
}

type opDepImpl struct {
    db              *gorm.DB
    inputArtifacts  []swf.Artifact
    outputArtifacts []swf.Artifact
    workflowControl workflowctl.WorkflowControl
    worktreePath    string // NEW
}

func (o *opDepImpl) WorktreePath() string {
    return o.worktreePath
}

type OpDependenciesBuilder struct {
    db              *gorm.DB
    artifacts       []swf.Artifact
    workflowControl workflowctl.WorkflowControl
    worktreePath    string // NEW
}

func (b *OpDependenciesBuilder) WithWorktreePath(path string) *OpDependenciesBuilder {
    b.worktreePath = path
    return b
}

func (b *OpDependenciesBuilder) Build() OpDependencies {
    return &opDepImpl{
        db:              b.db,
        inputArtifacts:  b.artifacts,
        outputArtifacts: make([]swf.Artifact, 0),
        workflowControl: b.workflowControl,
        worktreePath:    b.worktreePath, // NEW
    }
}
```

---

### 3. Update ActivityInvocationRequest
**File:** `/src/server/recipe-worker/pkg/ops/activity_registry.go`

```go
type ActivityInvocationRequest struct {
    Input               map[string]interface{}        `json:"input"`
    GitTaskContext      gitstate.GlobalGitTaskContext `json:"context"` // Changed from GitTaskContext
    Deps                ops.OpDependencies             `json:"-"`
}
```

---

### 4. Add Sentinel Value for WorktreePath
**File:** `/src/server/recipe-core/pkg/contextual/context.go`

**Problem:** Templates like `{{ environment.worktree_path }}` get resolved at COMPILE TIME (before serialization), but the actual worktree path must be determined at EXECUTION TIME on the target machine.

**Solution:** Return a sentinel value during template resolution.

```go
// Define sentinel constant
const WorktreePathSentinel = "__COLONY_WORKTREE_PATH__"

// Modify EnvironmentContext to return sentinel when WorktreePath is empty/unresolved
type EnvironmentContext struct {
    WorktreePath string `json:"worktree_path,omitempty"`
}

// Add method that template resolution calls
// This is used by the template engine when accessing {{ environment.worktree_path }}
func (e EnvironmentContext) GetWorktreePath() string {
    if e.WorktreePath == "" {
        return WorktreePathSentinel  // Return sentinel for template resolution
    }
    return e.WorktreePath
}
```

**Alternative if templates use field access directly:**
Update template resolution in `/src/server/recipe-template/pkg/template/` to check for empty WorktreePath and substitute sentinel.

---

### 5. Rewrite withGitWorkspace with Sentinel Hydration
**File:** `/src/server/recipe-worker/pkg/ops/activity_registry.go`

```go
func withGitWorkspace(deps ops.ServiceDependencies2, reg ActivityRegistration, controller *gitstate.Controller) func(context.Context, ActivityInvocationRequest, []swf.Artifact) (ActivityInvocationOutput, []swf.Artifact, error) {
    if controller == nil {
        controller = gitstate.NewController(nil)
    }
    return func(ctx context.Context, req ActivityInvocationRequest, inputArtifacts []swf.Artifact) (output ActivityInvocationOutput, outputArtifacts []swf.Artifact, err error) {
        var zero ActivityInvocationOutput

        // Create temporary worktree directory for this invocation
        worktreePath, err := os.MkdirTemp("", "recipe-worktree-*")
        if err != nil {
            return zero, nil, fmt.Errorf("create temp worktree: %w", err)
        }
        defer os.RemoveAll(worktreePath)

        // Build full GitTaskContext for controller from global context + local worktree path
        fullContext := &gitstate.GitTaskContext{
            GlobalGitTaskContext: &req.GitTaskContext,
            WorktreePath:         worktreePath,
        }

        // Find and filter input thin pack artifact
        var thinPackArtifact swf.Artifact
        var nonThinPackArtifacts []swf.Artifact
        for _, art := range inputArtifacts {
            if art.Name() == "__git_state_thin_pack__" {
                thinPackArtifact = art
            } else {
                nonThinPackArtifacts = append(nonThinPackArtifacts, art)
            }
        }

        // Call Restore with full context (includes WorktreePath)
        if err := controller.Restore(context.Background(), fullContext, thinPackArtifact); err != nil {
            return zero, nil, err
        }

        // CRITICAL: Hydrate sentinel values in input with actual worktree path
        // Templates like {{ environment.worktree_path }} resolved to sentinel at compile time
        // Now replace with real local path
        hydratedInput := replaceSentinels(req.Input, worktreePath)

        // Build OpDependencies with WorktreePath
        db := deps.Database()
        if tx, ok := swf.TxFromCtx(ctx); ok && tx != nil {
            db = tx
        }
        opDeps := ops.NewOpDependenciesBuilder().
            WithArtifacts(nonThinPackArtifacts).
            WithDatabase(db).
            WithWorkflowControl(deps.WorkflowControl()).
            WithWorktreePath(fullContext.WorktreePath).
            Build()

        // Ensure artifacts are collected on both success and failure paths
        defer func() {
            artifacts := opDeps.GetOutputArtifacts()
            outputArtifacts = append(outputArtifacts, artifacts...)
        }()

        // Execute operation with HYDRATED input (sentinels replaced)
        outputData, err := reg.Step.Invoke(opDeps, ctx, hydratedInput)
        if err != nil {
            return zero, outputArtifacts, err
        }

        // Call PersistWithDiffs with full context
        _, persistArtifacts, err := controller.PersistWithDiffs(context.Background(), fullContext)
        if err != nil {
            return zero, outputArtifacts, err
        }

        // Handle artifact pass-through logic (unchanged)
        if len(persistArtifacts) > 0 {
            outputArtifacts = append(outputArtifacts, persistArtifacts...)
        } else if thinPackArtifact != nil {
            outputArtifacts = append(outputArtifacts, thinPackArtifact)
        }

        // Build response using fullContext (which has updated hashes from Persist)
        parentRef := ""
        if fullContext.PersistHash == "" {
            parentRef = fullContext.BaseRef
        }

        return ActivityInvocationOutput{
            OpOutput: outputData,
            GitResult: contextual.GitCommitContext{
                PersistHash: fullContext.PersistHash,
                ParentHash:  fullContext.ParentHash,
                ParentRef:   parentRef,
            },
            NextTask: reg.NextTaskType,
        }, outputArtifacts, nil
    }
}

// replaceSentinels recursively walks the input map and replaces sentinel values
func replaceSentinels(input map[string]interface{}, worktreePath string) map[string]interface{} {
    result := make(map[string]interface{})
    for k, v := range input {
        result[k] = replaceSentinelValue(v, worktreePath)
    }
    return result
}

// replaceSentinelValue handles different types recursively
func replaceSentinelValue(value interface{}, worktreePath string) interface{} {
    switch v := value.(type) {
    case string:
        if v == contextual.WorktreePathSentinel {
            return worktreePath
        }
        return v
    case map[string]interface{}:
        return replaceSentinels(v, worktreePath)
    case []interface{}:
        result := make([]interface{}, len(v))
        for i, item := range v {
            result[i] = replaceSentinelValue(item, worktreePath)
        }
        return result
    default:
        return v
    }
}
```

**Key Changes:**
- Creates temp worktree per invocation with `os.MkdirTemp()`
- Cleans up with `defer os.RemoveAll(worktreePath)`
- Builds full GitTaskContext from global + local path
- **NEW:** Hydrates sentinel values before executing op
- Passes hydrated input (not req.Input) to Invoke
- Passes worktree path to OpDependencies

---

### 6. Update Compiler
**File:** `/src/server/recipe-worker/pkg/compiler/compiler.go`

**Line 91:** Template resolution happens here - templates referencing `{{ environment.worktree_path }}` will resolve to the sentinel value.

**Line 116:** Change from `NewGitTaskContext()` to `NewGlobalGitTaskContext()`

```go
// Line 91: Template resolution (unchanged, but now returns sentinel for worktree_path)
resolvedNodeInputs, err := resCtx.ResolveMap(metadata.Inputs)
if err != nil {
    return fmt.Errorf("failed to resolve templates op inputs: %w", err)
}
// resolvedNodeInputs may now contain: {"path": "__COLONY_WORKTREE_PATH__"}

// Line 114: Create invocation with global context
invocation := workerops.ActivityInvocationRequest{
    Input:          resolvedNodeInputs,  // May contain sentinel values
    GitTaskContext: *gitstate.NewGlobalGitTaskContext(resCtx.TaskExecutionContext()), // Changed
}
```

**Flow:**
1. Template resolution happens at compile time
2. `{{ environment.worktree_path }}` → `"__COLONY_WORKTREE_PATH__"`
3. Sentinel gets embedded in `Input` field
4. Gets serialized and sent to worker (machine-independent)
5. Worker's `withGitWorkspace()` replaces sentinel with actual local path

---

### 7. Remove ThinPackPath
**File:** `/src/server/recipe-core/pkg/contextual/context.go`

**Line 13:** Remove ThinPackPath field

```go
// EnvironmentContext captures filesystem and storage locations relevant to execution.
type EnvironmentContext struct {
    WorktreePath string `json:"worktree_path,omitempty"`
    // REMOVED: ThinPackPath string `json:"thin_pack_path,omitempty"`
}
```

**Also remove from git_task_context.go:**
- Remove `GetThinPackPath()` getter method

---

### 8. Add Worktree Cleanup to Job Worker
**File:** `/src/server/recipe-worker/pkg/compiler/job_worker.go`

**Lines 60-66:** Add cleanup

```go
runContext := input.JobContext
var cleanupWorktree func()
if runContext.Environment.WorktreePath == "" {
    working, err := os.MkdirTemp("", "recipe-git-artifacts")
    if err != nil {
        return nil, err
    }
    runContext.Environment.WorktreePath = working
    cleanupWorktree = func() { os.RemoveAll(working) }
}
// Cleanup worktree on job completion
if cleanupWorktree != nil {
    defer cleanupWorktree()
}
```

**Note:** This job-level worktree is now only used for recipe resolution context, NOT for actual git operations (activities create their own).

---

## Testing

### Unit Tests

**File:** `/src/server/recipe-worker/pkg/ops/activity_registry_test.go`

```go
// Test that GlobalGitTaskContext serializes without machine paths
func TestGlobalGitTaskContextSerialization(t *testing.T) {
    testContext := contextual.TaskExecutionContext{
        // ... populate test context
    }
    ctx := gitstate.NewGlobalGitTaskContext(testContext)

    data, err := json.Marshal(ctx)
    require.NoError(t, err)

    // Verify WorktreePath and ThinPackPath are NOT in JSON
    require.NotContains(t, string(data), "worktree_path")
    require.NotContains(t, string(data), "thin_pack_path")
}

// Test that ActivityInvocationRequest is portable across machines
func TestActivityInvocationRequestPortability(t *testing.T) {
    req := workerops.ActivityInvocationRequest{
        Input: map[string]interface{}{"key": "value"},
        GitTaskContext: *gitstate.NewGlobalGitTaskContext(testContext),
    }

    data, err := json.Marshal(req)
    require.NoError(t, err)

    // Verify can deserialize on "different machine"
    var req2 workerops.ActivityInvocationRequest
    require.NoError(t, json.Unmarshal(data, &req2))

    // Verify machine-independent fields preserved
    require.Equal(t, req.GitTaskContext.BaseRepo, req2.GitTaskContext.BaseRepo)
    require.Equal(t, req.GitTaskContext.CellPath, req2.GitTaskContext.CellPath)
}

// Test that OpDependencies provides worktree path
func TestOpDependenciesWorktreePath(t *testing.T) {
    deps := ops.NewOpDependenciesBuilder().
        WithWorktreePath("/tmp/test-worktree").
        Build()

    require.Equal(t, "/tmp/test-worktree", deps.WorktreePath())
}

// Test that all 14 getter methods still work
func TestGitTaskContextGetters(t *testing.T) {
    ctx := gitstate.NewGitTaskContext(testContext)

    require.NotEmpty(t, ctx.GetBaseRepo())
    require.NotEmpty(t, ctx.GetWorktreePath())
    // ... test all 14 methods
}

// NEW: Test sentinel value replacement
func TestSentinelReplacement(t *testing.T) {
    input := map[string]interface{}{
        "repo_path": contextual.WorktreePathSentinel,
        "nested": map[string]interface{}{
            "path": contextual.WorktreePathSentinel,
        },
        "array": []interface{}{
            contextual.WorktreePathSentinel,
            "other",
        },
    }

    result := replaceSentinels(input, "/tmp/actual-path")

    require.Equal(t, "/tmp/actual-path", result["repo_path"])
    require.Equal(t, "/tmp/actual-path", result["nested"].(map[string]interface{})["path"])
    require.Equal(t, "/tmp/actual-path", result["array"].([]interface{})[0])
    require.Equal(t, "other", result["array"].([]interface{})[1])
}

// NEW: Test that sentinel can be serialized
func TestSentinelSerializable(t *testing.T) {
    req := workerops.ActivityInvocationRequest{
        Input: map[string]interface{}{
            "path": contextual.WorktreePathSentinel,
        },
        GitTaskContext: *gitstate.NewGlobalGitTaskContext(testContext),
    }

    // Serialize
    data, err := json.Marshal(req)
    require.NoError(t, err)

    // Verify sentinel is in JSON (as a string value)
    require.Contains(t, string(data), contextual.WorktreePathSentinel)

    // Deserialize
    var req2 workerops.ActivityInvocationRequest
    require.NoError(t, json.Unmarshal(data, &req2))

    // Verify sentinel preserved
    require.Equal(t, contextual.WorktreePathSentinel, req2.Input["path"])
}
```

### Integration Tests
- Test activity execution with new structure
- Test git operations (clone, restore, persist) still work
- Test multi-step task chains with git operations
- Test worktree cleanup (no temp directory leaks)

---

## Validation Checklist

### Serialization & Portability
- [ ] ActivityInvocationRequest serializes to JSON without `worktree_path` or `thin_pack_path` fields
- [ ] GlobalGitTaskContext contains no machine-specific fields
- [ ] Can deserialize ActivityInvocationRequest and execute on "different machine"
- [ ] Jobs can be scheduled on any worker node regardless of original worktree path

### Sentinel Values
- [ ] Template `{{ environment.worktree_path }}` resolves to sentinel at compile time
- [ ] Sentinel value is serializable (appears in ActivityInvocationRequest.Input)
- [ ] replaceSentinels() correctly hydrates all sentinel values
- [ ] Nested sentinels (in maps and arrays) are replaced
- [ ] Ops receive actual local path (not sentinel) in their inputs

### OpDependencies
- [ ] OpDependencies.WorktreePath() returns correct path accessible by ops
- [ ] WorktreePath passed to OpDependencies matches actual temp directory
- [ ] Ops can use deps.WorktreePath() to access git worktree

### Backward Compatibility
- [ ] All 14 GitTaskContext getter methods still work
- [ ] Pointer embedding correctly shares GlobalGitTaskContext data
- [ ] Controller operations (Restore, Persist) update shared context fields

### Git Operations
- [ ] Git operations (clone, restore, persist) work correctly with new structure
- [ ] Temp worktrees are created successfully
- [ ] Git state is properly restored from thin pack artifacts
- [ ] Changes are persisted and new thin packs created

### Cleanup
- [ ] No worktree path leaks - all temp directories cleaned up properly
- [ ] defer os.RemoveAll() called for activity-level worktrees
- [ ] Job-level worktree cleanup added (job_worker.go)

### ThinPackPath Removal
- [ ] ThinPackPath fully removed from codebase (verified via `grep -r "ThinPackPath" server/`)
- [ ] GetThinPackPath() method removed from GitTaskContext
- [ ] EnvironmentContext.ThinPackPath field removed

### Tests
- [ ] All existing integration tests pass
- [ ] New serialization portability tests pass
- [ ] New sentinel replacement tests pass
- [ ] Test that sentinels can be serialized/deserialized

---

## Critical Notes

### ⚠️ Pointer Embedding Required
Must use `*GlobalGitTaskContext` (pointer) not value embedding:
- Controller.Restore() and Persist() update fields on context
- Value embedding would create copies, losing updates
- Pointer embedding shares data bidirectionally

### ⚠️ Backward Compatibility
Changing `ActivityInvocationRequest.GitTaskContext` type will break in-flight jobs. Consider:
- Rolling deployment strategy
- Optional backward-compatible field temporarily
- Version check in deserialization

### ⚠️ Cleanup Responsibilities
- Each activity cleans up its own temp worktree via defer
- Job-level worktree (job_worker.go) must also be cleaned up (currently leaks)

### ⚠️ Template Resolution Investigation Required
Before implementation, verify where template resolution accesses `WorktreePath`:
- Check if templates use field access or getter method
- Might need to update template engine in `/src/server/recipe-template/pkg/template/`
- Ensure `GetWorktreePath()` method is called (not direct field access)
- If direct field access, need different approach (e.g., pre-populate EnvironmentContext with sentinel)

### ⚠️ Sentinel Value Must Be Unique
- Choose sentinel that won't appear in real paths: `"__COLONY_WORKTREE_PATH__"`
- Document that this is a reserved value
- Consider adding validation to reject recipes with this literal string

### ⚠️ ThinPackPath Removal Verified
- workspace_controller does NOT use task.GetThinPackPath()
- Safe to remove from all contexts
- Remove getter method
- Update any tests that reference it

### ⚠️ replaceSentinels Must Be Thorough
- Must handle all JSON-serializable types: string, map, array, numbers, booleans, null
- Only replace string values that exactly match sentinel
- Don't replace substrings (e.g., "path: __COLONY_WORKTREE_PATH__/file" should become "path: /tmp/worktree/file")
- Consider using strings.ReplaceAll() for partial path replacement if needed
