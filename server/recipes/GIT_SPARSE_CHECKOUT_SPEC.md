# Git Package: Sparse Checkout Support

**Status:** Draft
**Author:** System
**Date:** 2026-01-05
**Target Package:** `/src/server/git`

## Overview

Add sparse checkout functionality to the `server/git` package to enable selective checkout of specific directories/files from a git repository. This feature is needed by the recipe service to efficiently clone only the `.c2/recipes` directory instead of the entire repository.

## Background

### What is Sparse Checkout?

Sparse checkout allows you to populate your working directory with only a subset of files from the repository. This is particularly useful when:
- Working with large repositories where you only need specific directories
- Reducing disk space usage
- Speeding up checkout operations
- Minimizing network transfer when combined with shallow clones

### Git Sparse Checkout Modes

Git supports two modes:

1. **Cone Mode (Modern, Recommended)**
   - Simpler, more performant
   - Works with directory paths (e.g., `.c2/recipes`)
   - Automatically includes parent directories
   - Better performance for large repositories
   - Introduced in Git 2.25 (Feb 2020)

2. **Pattern Mode (Legacy)**
   - More flexible but complex
   - Uses gitignore-style patterns
   - Can include/exclude individual files
   - More error-prone

**This spec implements cone mode only** as it's simpler and covers the recipe service use case.

### How It Works

```bash
# Clone repository (normal or shallow)
git clone --depth=1 <url> <path>

# Initialize sparse checkout (cone mode)
git sparse-checkout init --cone

# Set paths to checkout
git sparse-checkout set .c2/recipes

# Result: Only .c2/recipes/ directory is in working tree
# Other files remain in git database but not on filesystem
```

## Current State

### Existing Clone Support

The git package currently supports:

```go
type CloneOptions struct {
    Branch       string // Specific branch to clone
    Depth        *int   // Shallow clone depth (nil = full clone)
    SingleBranch bool   // Clone only the specified branch
    Bare         bool   // Create a bare repository
}
```

**Limitations:**
- No sparse checkout support
- All files are checked out to working directory
- Large repositories consume unnecessary disk space

### Implementation Location

**Public API:** `/src/server/git/pkg/git/repository.go`
**Internal Implementation:** `/src/server/git/internal/commands/commands.go`

## Proposed Changes

### 1. Add SparseCheckoutOptions Type

**Location:** `pkg/git/repository.go`

Add new type definition after `CloneOptions` (around line 43):

```go
// SparseCheckoutOptions configures sparse checkout during clone operations.
type SparseCheckoutOptions struct {
    // Cone enables cone mode sparse checkout (recommended).
    // When true, Paths are treated as directory paths.
    // When false, sparse checkout is disabled.
    Cone bool

    // Paths specifies the directories to checkout in cone mode.
    // Example: []string{".c2/recipes", "docs"}
    // Empty slice disables sparse checkout.
    Paths []string
}
```

### 2. Update CloneOptions

**Location:** `pkg/git/repository.go` (line 36-42)

**Before:**
```go
type CloneOptions struct {
    Branch       string // Specific branch to clone (empty = default branch)
    Depth        *int   // Shallow clone depth (nil = full clone)
    SingleBranch bool   // Clone only the specified branch
    Bare         bool   // Create a bare repository
}
```

**After:**
```go
type CloneOptions struct {
    Branch         string                  // Specific branch to clone (empty = default branch)
    Depth          *int                    // Shallow clone depth (nil = full clone)
    SingleBranch   bool                    // Clone only the specified branch
    Bare           bool                    // Create a bare repository
    SparseCheckout *SparseCheckoutOptions  // Sparse checkout config (nil = full checkout)
}
```

### 3. Update Clone Method Signature

**Location:** `pkg/git/repository.go` (Repository interface, line 182)

**Before:**
```go
Clone(ctx context.Context, url string, localPath string, options CloneOptions) error
```

**After:**
```go
// No change to interface method signature
// SparseCheckout is now part of CloneOptions
```

### 4. Update repoAdapter.Clone Implementation

**Location:** `pkg/git/repository.go` (line 321-323)

**Before:**
```go
func (a *repoAdapter) Clone(ctx context.Context, url string, localPath string, options CloneOptions) error {
    return a.repo.Clone(ctx, url, localPath, options.Branch, options.Depth, options.SingleBranch, options.Bare)
}
```

**After:**
```go
func (a *repoAdapter) Clone(ctx context.Context, url string, localPath string, options CloneOptions) error {
    return a.repo.Clone(ctx, url, localPath, options.Branch, options.Depth, options.SingleBranch, options.Bare, options.SparseCheckout)
}
```

### 5. Update commands.Repository.Clone Implementation

**Location:** `internal/commands/commands.go` (line 288-315)

**Before:**
```go
func (r *Repository) Clone(ctx context.Context, url string, localPath string, branch string, depth *int, singleBranch bool, bare bool) error {
    args := []string{"clone"}

    if branch != "" {
        args = append(args, "--branch", branch)
    }

    if depth != nil {
        args = append(args, "--depth", fmt.Sprintf("%d", *depth))
    }

    if singleBranch {
        args = append(args, "--single-branch")
    }

    if bare {
        args = append(args, "--bare")
    }

    args = append(args, url, localPath)

    cmd := exec.CommandContext(ctx, "git", args...)
    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to clone repository: %w\nOutput: %s", err, output)
    }

    return nil
}
```

**After:**
```go
func (r *Repository) Clone(ctx context.Context, url string, localPath string, branch string, depth *int, singleBranch bool, bare bool, sparseCheckout *SparseCheckoutOptions) error {
    // Build clone arguments
    args := []string{"clone"}

    if branch != "" {
        args = append(args, "--branch", branch)
    }

    if depth != nil {
        args = append(args, "--depth", fmt.Sprintf("%d", *depth))
    }

    if singleBranch {
        args = append(args, "--single-branch")
    }

    if bare {
        args = append(args, "--bare")
    }

    // Sparse checkout requires --no-checkout to avoid initial full checkout
    if sparseCheckout != nil && sparseCheckout.Cone && len(sparseCheckout.Paths) > 0 {
        args = append(args, "--no-checkout")
    }

    args = append(args, url, localPath)

    // Execute clone
    cmd := exec.CommandContext(ctx, "git", args...)
    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to clone repository: %w\nOutput: %s", err, output)
    }

    // Configure sparse checkout if requested
    if sparseCheckout != nil && sparseCheckout.Cone && len(sparseCheckout.Paths) > 0 {
        if err := r.configureSparseCheckout(ctx, localPath, sparseCheckout.Paths); err != nil {
            return fmt.Errorf("failed to configure sparse checkout: %w", err)
        }
    }

    return nil
}
```

### 6. Add configureSparseCheckout Helper

**Location:** `internal/commands/commands.go` (add after `Clone` method, around line 316)

```go
// configureSparseCheckout initializes and configures sparse checkout in cone mode.
// This must be called after clone with --no-checkout flag.
func (r *Repository) configureSparseCheckout(ctx context.Context, repoPath string, paths []string) error {
    // Initialize sparse checkout in cone mode
    cmd := exec.CommandContext(ctx, "git", "sparse-checkout", "init", "--cone")
    cmd.Dir = repoPath
    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to init sparse checkout: %w\nOutput: %s", err, output)
    }

    // Set sparse checkout paths
    args := append([]string{"sparse-checkout", "set"}, paths...)
    cmd = exec.CommandContext(ctx, "git", args...)
    cmd.Dir = repoPath
    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to set sparse checkout paths: %w\nOutput: %s", err, output)
    }

    // Checkout the files (this populates working directory with sparse paths)
    cmd = exec.CommandContext(ctx, "git", "checkout")
    cmd.Dir = repoPath
    if output, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to checkout sparse paths: %w\nOutput: %s", err, output)
    }

    return nil
}
```

### 7. Add SparseCheckoutOptions to Internal Package

**Location:** `internal/commands/commands.go` (add type definition near top, around line 37)

```go
// SparseCheckoutOptions configures sparse checkout during clone operations.
type SparseCheckoutOptions struct {
    Cone  bool
    Paths []string
}
```

## Usage Examples

### Example 1: Basic Sparse Checkout

```go
// Clone only .c2/recipes directory
depth := 1
cloneOpts := git.CloneOptions{
    Depth:        &depth,
    SingleBranch: true,
    SparseCheckout: &git.SparseCheckoutOptions{
        Cone:  true,
        Paths: []string{".c2/recipes"},
    },
}

err := gitRepo.Clone(ctx, "https://github.com/example/repo.git", "/tmp/workspace", cloneOpts)
```

**Result:**
- Only `.c2/recipes/` directory exists in working tree
- Shallow clone with depth=1 (only latest commit)
- Single branch cloned
- Minimal disk usage and network transfer

### Example 2: Multiple Sparse Paths

```go
// Clone multiple directories
cloneOpts := git.CloneOptions{
    SparseCheckout: &git.SparseCheckoutOptions{
        Cone:  true,
        Paths: []string{".c2/recipes", "docs", "config"},
    },
}

err := gitRepo.Clone(ctx, repoURL, workspacePath, cloneOpts)
```

**Result:**
- Working tree contains: `.c2/recipes/`, `docs/`, `config/`
- All other files remain in git database but not on filesystem

### Example 3: Recipe Service Usage

```go
func (s *service) createEphemeralWorkspace(ctx context.Context, projectID project.ID) (string, func(), error) {
    proj, err := s.projectSvc.GetProject(ctx, projectID)
    if err != nil {
        return "", nil, err
    }

    tempDir, err := os.MkdirTemp("", fmt.Sprintf("recipe-workspace-%s-*", projectID))
    if err != nil {
        return "", nil, err
    }

    cleanup := func() {
        os.RemoveAll(tempDir)
    }

    // Shallow clone with sparse checkout of .c2/recipes only
    depth := 1
    cloneOpts := git.CloneOptions{
        Depth:        &depth,
        SingleBranch: true,
        SparseCheckout: &git.SparseCheckoutOptions{
            Cone:  true,
            Paths: []string{".c2/recipes"},
        },
    }

    if err := s.gitRepo.Clone(ctx, proj.GitRepoPath, tempDir, cloneOpts); err != nil {
        cleanup()
        return "", nil, err
    }

    return tempDir, cleanup, nil
}
```

## Git Command Sequence

When sparse checkout is enabled, the following git commands are executed:

```bash
# Step 1: Clone without checking out files
git clone --depth=1 --single-branch --no-checkout <url> <path>

# Step 2: Initialize sparse checkout in cone mode
cd <path>
git sparse-checkout init --cone

# Step 3: Configure sparse paths
git sparse-checkout set .c2/recipes

# Step 4: Checkout configured paths
git checkout
```

## Backward Compatibility

### No Breaking Changes

**Existing code continues to work unchanged:**

```go
// Old code - still works (no sparse checkout)
cloneOpts := git.CloneOptions{
    Branch: "main",
    Depth:  &depth,
}
gitRepo.Clone(ctx, url, path, cloneOpts)
```

**New field is optional:**
- `SparseCheckout` field defaults to `nil`
- When `nil`, normal full checkout is performed
- Existing behavior preserved

## Edge Cases and Error Handling

### Case 1: Empty Paths Array

```go
SparseCheckout: &git.SparseCheckoutOptions{
    Cone:  true,
    Paths: []string{}, // Empty!
}
```

**Behavior:** Treated as if `SparseCheckout` is `nil`, performs full checkout

### Case 2: Cone = false

```go
SparseCheckout: &git.SparseCheckoutOptions{
    Cone:  false,
    Paths: []string{".c2/recipes"},
}
```

**Behavior:** Treated as if `SparseCheckout` is `nil`, performs full checkout

### Case 3: Invalid Paths

```go
SparseCheckout: &git.SparseCheckoutOptions{
    Cone:  true,
    Paths: []string{"nonexistent/path"},
}
```

**Behavior:** Git command fails, error returned to caller

### Case 4: Bare Repository

```go
CloneOptions{
    Bare: true,
    SparseCheckout: &git.SparseCheckoutOptions{...},
}
```

**Behavior:** Sparse checkout is ignored (bare repos have no working tree)

## Testing Strategy

### Unit Tests

**Location:** `internal/commands/commands_test.go`

```go
func TestClone_WithSparseCheckout(t *testing.T) {
    // Test sparse checkout is configured correctly
}

func TestClone_WithoutSparseCheckout(t *testing.T) {
    // Test normal clone still works (backward compatibility)
}

func TestClone_SparseCheckoutEmptyPaths(t *testing.T) {
    // Test empty paths array is handled gracefully
}
```

### Integration Tests

```go
func TestRepository_Clone_SparseCheckout_Integration(t *testing.T) {
    // Create real git repository with multiple directories
    // Clone with sparse checkout
    // Verify only specified paths exist in working tree
    // Verify git operations (commit, push) still work
}
```

## Performance Characteristics

### Disk Space Comparison

**Example:** Repository with 1000 files, 100MB total, `.c2/recipes` = 5MB

| Clone Type | Disk Usage | Notes |
|------------|------------|-------|
| Full clone | ~100MB | All files in working tree |
| Shallow clone (depth=1) | ~100MB | Latest commit, all files |
| **Shallow + Sparse** | **~5MB** | Latest commit + sparse working tree |

**Best practice:** Always combine sparse checkout with shallow clone

## Git Version Requirements

**Required:** Git 2.25.0 or later (February 2020)

**Rationale:** Cone mode introduced in Git 2.25

## Documentation Updates

### Public API Documentation

Update `CloneOptions` godoc:

```go
// CloneOptions configures repository clone operations.
//
// Sparse Checkout:
//   To clone only specific directories, use SparseCheckout option:
//
//     depth := 1
//     opts := CloneOptions{
//         Depth: &depth,
//         SparseCheckout: &SparseCheckoutOptions{
//             Cone: true,
//             Paths: []string{".c2/recipes"},
//         },
//     }
//
//   Requires git 2.25.0 or later.
type CloneOptions struct {
    // ... fields
}
```

## Implementation Checklist

- [ ] Add `SparseCheckoutOptions` type to `pkg/git/repository.go`
- [ ] Update `CloneOptions` struct with `SparseCheckout` field
- [ ] Add `SparseCheckoutOptions` type to `internal/commands/commands.go`
- [ ] Update `commands.Repository.Clone` method signature
- [ ] Implement `configureSparseCheckout` helper method
- [ ] Update `Clone` implementation to handle sparse checkout
- [ ] Update `repoAdapter.Clone` to pass sparse checkout options
- [ ] Add unit tests for sparse checkout logic
- [ ] Add integration tests with real repositories
- [ ] Update API documentation (godoc)

## Success Criteria

1. **Functionality:** Clone with sparse checkout creates working tree with only specified paths
2. **Backward Compatibility:** Existing code continues to work without changes
3. **Performance:** Shallow + sparse clone uses <10% disk space of full clone
4. **Reliability:** Error handling for edge cases, no panics

## References

- Git Sparse Checkout Documentation: https://git-scm.com/docs/git-sparse-checkout
- Git 2.25 Release Notes: https://github.com/git/git/blob/master/Documentation/RelNotes/2.25.0.txt
- Cone Mode Blog: https://github.blog/2020-01-17-bring-your-monorepo-down-to-size-with-sparse-checkout/
