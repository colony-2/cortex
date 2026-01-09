# Recipe Service: Ephemeral Workspace Refactoring

**Status:** Draft
**Author:** System
**Date:** 2026-01-05

## Overview

Refactor the recipe service to eliminate persistent workspace directories and use ephemeral temporary workspaces with shallow, sparse checkouts. This addresses two critical issues:

1. **Concurrency safety**: Each operation gets its own isolated temporary workspace, eliminating race conditions
2. **Resource management**: Automatic cleanup of workspaces prevents disk space accumulation

## Current Architecture Problems

### Problem 1: Persistent Workspace Root
- Workspaces persist indefinitely at `./nodes/.recipe-workspaces/{projectID}/`
- Full git clones consume significant disk space (entire history)
- No cleanup mechanism on service shutdown or restart
- Multiple service instances create duplicate clones

### Problem 2: No Concurrency Protection
- Concurrent recipe operations for the same project share a single workspace directory
- Race conditions in workspace creation, git operations, and file writes
- No mutex or locking mechanisms
- Database-level optimistic locking doesn't protect git workspace operations

### Problem 3: Unnecessary Data Transfer
- Full git history cloned when only `.c2/recipes/` directory is needed
- All branches and tags fetched when only working branch is needed

## Proposed Architecture

### Core Principle: Ephemeral, Isolated Workspaces

Each recipe operation (create, update, delete, publish, sync) gets its own temporary workspace that:
1. Lives only for the duration of the operation
2. Contains only the `.c2/recipes` directory (sparse checkout)
3. Has minimal git history (shallow clone with depth=1)
4. Is automatically cleaned up when operation completes or fails

### Workspace Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│ Recipe Operation (Create/Update/Delete/Publish/Sync)       │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. Create Temp Directory                                    │
│    tempDir := os.MkdirTemp("", "recipe-workspace-*")       │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Shallow Clone with Sparse Checkout                       │
│    - Clone with depth=1, singleBranch=true                  │
│    - Enable sparse checkout (cone mode)                     │
│    - Checkout only .c2/recipes/ path                        │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Perform Recipe Operation                                 │
│    - Read/Write files in .c2/recipes/                       │
│    - Stage changes                                          │
│    - Create commit                                          │
│    - Push to origin                                         │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Cleanup (defer in function)                              │
│    os.RemoveAll(tempDir)                                    │
└─────────────────────────────────────────────────────────────┘
```

### Git Sparse Checkout Pattern

**Initial Setup (after clone):**
```bash
cd <workspace>
git sparse-checkout init --cone
git sparse-checkout set .c2/recipes
```

This results in:
- Only `.c2/recipes/` directory checked out in working tree
- All other files remain in git database but not in filesystem
- Significantly reduced disk usage
- Faster checkout operations

**Combined with Shallow Clone:**
```bash
git clone --depth=1 --single-branch --branch=main <url> <workspace>
cd <workspace>
git sparse-checkout init --cone
git sparse-checkout set .c2/recipes
```

Benefits:
- Only latest commit (no history)
- Only one branch
- Only `.c2/recipes` directory in working tree
- Minimal network transfer and disk usage

## Implementation Changes

### 1. Remove WorkspaceRoot Configuration

**Before:**
```go
type ServiceConfig struct {
    Store         store.Store
    GitRepo       git.Repository
    ProjectSvc    project.Service
    WorkspaceRoot string // ← REMOVE THIS
    IDGen         id.Generator
    Clock         clock.Clock
}
```

**After:**
```go
type ServiceConfig struct {
    Store      store.Store
    GitRepo    git.Repository
    ProjectSvc project.Service
    IDGen      id.Generator
    Clock      clock.Clock
}
```

**Validation Removal:**
```go
// REMOVE this validation
if cfg.WorkspaceRoot == "" {
    return nil, errors.New("recipe service: workspace root is required")
}

// REMOVE this directory creation
if err := os.MkdirAll(cfg.WorkspaceRoot, 0755); err != nil {
    return nil, fmt.Errorf("failed to create workspace root: %w", err)
}
```

### 2. New Helper: createEphemeralWorkspace

**Location:** `internal/service/helpers.go`

```go
// createEphemeralWorkspace creates a temporary workspace with sparse checkout
// for the given project. Returns the workspace path and a cleanup function.
// The caller MUST defer the cleanup function to ensure workspace removal.
func (s *service) createEphemeralWorkspace(
    ctx context.Context,
    projectID project.ID,
) (workspacePath string, cleanup func(), err error) {
    // Get project details
    proj, err := s.projectSvc.GetProject(ctx, projectID)
    if err != nil {
        return "", nil, fmt.Errorf("failed to get project: %w", err)
    }

    // Create temporary directory
    tempDir, err := os.MkdirTemp("", fmt.Sprintf("recipe-workspace-%s-*", projectID))
    if err != nil {
        return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
    }

    // Cleanup function to remove temp directory
    cleanup = func() {
        if err := os.RemoveAll(tempDir); err != nil {
            // Log error but don't fail - best effort cleanup
            fmt.Fprintf(os.Stderr, "warning: failed to cleanup workspace %s: %v\n", tempDir, err)
        }
    }

    // If any error occurs after this point, cleanup temp dir
    defer func() {
        if err != nil {
            cleanup()
        }
    }()

    // Perform shallow clone with sparse checkout
    depth := 1
    cloneOpts := git.CloneOptions{
        Depth:         &depth,
        SingleBranch:  true,
        SparseCheckout: &git.SparseCheckoutOptions{
            Cone:  true,
            Paths: []string{".c2/recipes"},
        },
    }

    if err := s.gitRepo.Clone(ctx, proj.GitRepoPath, tempDir, cloneOpts); err != nil {
        return "", nil, fmt.Errorf("failed to clone repository: %w", err)
    }

    // Configure git user for commits
    if err := s.gitRepo.ConfigureUser(ctx, tempDir, "Colony Recipe Service", "recipes@colony.local"); err != nil {
        return "", nil, fmt.Errorf("failed to configure git user: %w", err)
    }

    // Ensure .c2/recipes directory exists
    recipesDir := filepath.Join(tempDir, ".c2", "recipes")
    if _, err := os.Stat(recipesDir); os.IsNotExist(err) {
        if err := os.MkdirAll(recipesDir, 0755); err != nil {
            return "", nil, fmt.Errorf("failed to create recipes directory: %w", err)
        }

        // Create .gitkeep to ensure directory is tracked
        gitkeepPath := filepath.Join(recipesDir, ".gitkeep")
        if err := os.WriteFile(gitkeepPath, []byte(""), 0644); err != nil {
            return "", nil, fmt.Errorf("failed to create .gitkeep: %w", err)
        }

        // Stage, commit, and push initial structure
        if err := s.gitRepo.StageFiles(ctx, tempDir, []string{".c2/recipes/.gitkeep"}); err != nil {
            return "", nil, fmt.Errorf("failed to stage .gitkeep: %w", err)
        }

        if err := s.gitRepo.CreateCommit(ctx, tempDir, "Initialize recipe repository"); err != nil {
            return "", nil, fmt.Errorf("failed to create initial commit: %w", err)
        }

        pushOpts := git.PushOptions{Remote: "origin"}
        if _, err := s.gitRepo.Push(ctx, tempDir, pushOpts); err != nil {
            return "", nil, fmt.Errorf("failed to push initial structure: %w", err)
        }
    }

    return tempDir, cleanup, nil
}
```

### 3. Update CreateRecipe Implementation

**Location:** `internal/service/service.go:110-170`

**Before:**
```go
func (s *service) CreateRecipe(ctx context.Context, input model.CreateInput) (*model.RecipeVersion, error) {
    // Get or create workspace
    gitWorkspace, err := s.getOrCreateGitWorkspace(ctx, input.ProjectID)
    if err != nil {
        return nil, err
    }

    // ... rest of implementation uses gitWorkspace
}
```

**After:**
```go
func (s *service) CreateRecipe(ctx context.Context, input model.CreateInput) (*model.RecipeVersion, error) {
    // Create ephemeral workspace
    workspace, cleanup, err := s.createEphemeralWorkspace(ctx, input.ProjectID)
    if err != nil {
        return nil, err
    }
    defer cleanup() // CRITICAL: Always cleanup

    // ... rest of implementation uses workspace
    // No changes needed to core logic - just use workspace instead of gitWorkspace
}
```

### 4. Update All Recipe Operations

Apply the same pattern to:
- `UpdateRecipe` (service.go:172-232)
- `DeleteRecipe` (service.go:234-294)
- `PublishRecipe` (service.go:420-465)
- `SyncFromRemote` (service.go:542-582)
- Any other method that uses `getOrCreateGitWorkspace`

### 5. Remove Old Helper Functions

Delete these functions from `internal/service/helpers.go`:
- `getOrCreateGitWorkspace` (lines 30-101)
- `syncWorkspace` (lines 103-115)
- `ensureOriginRemote` (lines 117-138)

These are no longer needed because:
- Each operation creates its own workspace (no "get or create")
- Shallow clone always gets latest (no "sync" needed)
- Clone automatically sets up origin remote (no manual remote config)

### 6. Update Service Struct

**Before:**
```go
type service struct {
    store         store.Store
    gitRepo       git.Repository
    projectSvc    project.Service
    workspaceRoot string // ← REMOVE THIS
    idGen         id.Generator
    clock         clock.Clock
}
```

**After:**
```go
type service struct {
    store      store.Store
    gitRepo    git.Repository
    projectSvc project.Service
    idGen      id.Generator
    clock      clock.Clock
}
```

## Migration Impact

### Breaking Changes

**Configuration:**
- `ServiceConfig.WorkspaceRoot` field removed
- Callers must remove this from service initialization

**Example - Before:**
```go
recipeSvc, err := service.New(service.ServiceConfig{
    Store:         recipeStore,
    GitRepo:       gitRepo,
    ProjectSvc:    projectSvc,
    WorkspaceRoot: filepath.Join(nodesPath, ".recipe-workspaces"), // ← REMOVE
    IDGen:         idgen.New(),
    Clock:         clock.New(),
})
```

**Example - After:**
```go
recipeSvc, err := service.New(service.ServiceConfig{
    Store:      recipeStore,
    GitRepo:    gitRepo,
    ProjectSvc: projectSvc,
    IDGen:      idgen.New(),
    Clock:      clock.New(),
})
```

### Existing Workspace Directories

**What happens to existing `.recipe-workspaces/` directories?**

They become orphaned and should be manually cleaned up:

```bash
# Safe to remove after deploying new version
rm -rf ./nodes/.recipe-workspaces
```

These are NOT automatically removed because:
1. Service doesn't know about them anymore (no WorkspaceRoot config)
2. Safe to leave them (won't interfere with new system)
3. Manual cleanup allows for inspection if needed

## Benefits

### 1. Concurrency Safety
- **No shared state**: Each operation has isolated workspace
- **No race conditions**: No checking "does workspace exist?"
- **No locking needed**: Natural isolation via temp directories
- **Parallel operations**: Multiple recipes in same project can be edited simultaneously

### 2. Resource Management
- **Automatic cleanup**: `defer cleanup()` ensures removal even on panic
- **No disk accumulation**: Workspaces removed immediately after use
- **Minimal disk usage**: Only `.c2/recipes` checked out, only latest commit
- **Reduced network**: Shallow clone transfers minimal data

### 3. Operational Simplicity
- **No persistent state**: Service restarts don't need to manage old workspaces
- **Stateless service**: No initialization/shutdown lifecycle complexity
- **Multi-instance safe**: Multiple service instances don't conflict
- **Easy debugging**: Each operation is isolated and independent

### 4. Performance
- **Faster clones**: Shallow + sparse = minimal data transfer
- **Faster checkouts**: Only `.c2/recipes` files written to disk
- **No sync overhead**: Always start from latest (no pull/merge needed)
- **Parallel I/O**: Each workspace is independent, no serialization

## Performance Comparison

### Before (Persistent Workspace)

**First Operation:**
```
Clone (full history, all files): ~500MB, 10-30s
Total: ~500MB, 10-30s
```

**Subsequent Operations (best case - no conflicts):**
```
Sync (git pull): ~1-5s
Perform operation: ~1s
Total: ~2-6s per operation
```

**Subsequent Operations (worst case - conflicts or stale workspace):**
```
Detect stale/conflict: ~1s
Delete workspace: ~1-2s
Clone (full history, all files): ~500MB, 10-30s
Total: ~12-33s per operation
```

### After (Ephemeral Workspace)

**Every Operation (consistent):**
```
Create temp dir: <0.1s
Shallow sparse clone: ~5-50MB, 1-5s
Perform operation: ~1s
Cleanup: <0.1s
Total: ~2-7s per operation
```

**Key Differences:**
- Consistent performance (no "best case" vs "worst case")
- 10-100x less data transfer per operation
- No state accumulation or cache invalidation issues
- Parallelizable (multiple operations don't wait for workspace lock)

## Testing Strategy

### Unit Tests
- Mock `os.MkdirTemp` to verify temp directory creation
- Verify cleanup function is always called (even on error paths)
- Test error handling when clone fails (cleanup still happens)
- Verify sparse checkout paths are correctly configured

### Integration Tests
- Verify actual temp directories are created and removed
- Confirm only `.c2/recipes` directory exists in workspace
- Test concurrent recipe operations on same project (should succeed)
- Verify git operations work correctly with sparse checkout
- Confirm cleanup happens on context cancellation

### Regression Tests
- All existing recipe service tests should pass with minimal changes
- Only change: Remove `WorkspaceRoot` from test service configs

## Dependencies

This spec depends on:
- **Git Package Enhancement Spec**: Add sparse checkout support to `server/git` package
  - See `GIT_SPARSE_CHECKOUT_SPEC.md` for details

## Rollout Plan

### Phase 1: Git Package Enhancement
1. Implement sparse checkout support in `server/git`
2. Add unit tests for sparse checkout functionality
3. Add integration tests with real git repositories

### Phase 2: Recipe Service Refactoring
1. Implement `createEphemeralWorkspace` helper
2. Update `CreateRecipe` to use ephemeral workspaces
3. Add integration tests to verify behavior
4. Update remaining operations (Update, Delete, Publish, Sync)
5. Remove old helper functions and WorkspaceRoot field

### Phase 3: Deployment
1. Deploy new version
2. Verify no issues in production
3. Manually cleanup old `.recipe-workspaces` directories

## Alternative Approaches Considered

### Alternative 1: Add Locking to Existing System
**Approach:** Keep persistent workspaces but add per-project mutexes

**Rejected because:**
- Doesn't solve disk space accumulation
- Requires distributed locking for multi-instance deployments
- Still requires cleanup logic for old workspaces
- Adds complexity without addressing root causes

### Alternative 2: Workspace Pool with Reuse
**Approach:** Create pool of workspaces, reuse them across operations

**Rejected because:**
- Adds complexity (pool management, lifecycle, eviction)
- Still has concurrency issues (locking, state management)
- Doesn't significantly improve performance vs ephemeral approach
- Risk of state pollution between operations

### Alternative 3: In-Memory Git Operations
**Approach:** Use libgit2 or similar to avoid filesystem entirely

**Rejected because:**
- Significant implementation complexity
- Requires different git library (current uses CLI)
- Harder to debug and test
- Not compatible with existing git package abstraction

## Open Questions

1. **Temp directory location**: Should we use system temp (`os.MkdirTemp("")`) or configurable location?
   - **Recommendation**: System temp is fine, but could add optional `TempDirRoot` config for advanced users

2. **Cleanup on crash**: What happens if service crashes before cleanup?
   - **Answer**: OS eventually cleans system temp, but could add startup cleanup of old temp dirs

3. **Large recipe files**: What if `.c2/recipes` directory is very large (>1GB)?
   - **Answer**: Sparse checkout still helps (doesn't checkout other files), consider adding size warnings

4. **Git LFS**: Does sparse checkout work with Git LFS?
   - **Answer**: Yes, but LFS files are still downloaded. May need `git lfs fetch --include=".c2/recipes/**"`

## Success Metrics

1. **Concurrency**: Multiple recipe operations on same project succeed in parallel
2. **Disk usage**: No growth in disk usage over time from orphaned workspaces
3. **Performance**: Recipe operations complete in consistent time (2-7s regardless of operation count)
4. **Reliability**: Zero race condition errors in production

## References

- Git Sparse Checkout Documentation: https://git-scm.com/docs/git-sparse-checkout
- Git Shallow Clone Documentation: https://git-scm.com/docs/git-clone#Documentation/git-clone.txt---depthltdepthgt
- Go os.MkdirTemp: https://pkg.go.dev/os#MkdirTemp
