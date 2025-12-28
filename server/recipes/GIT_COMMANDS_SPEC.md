# Git Commands Specification

## Overview

This document specifies additional git operations required by the recipe service and other components that need remote repository capabilities. These operations extend the existing `server/git/pkg/git` interface.

## Design Principles

1. **Context-Aware**: All operations support cancellation via context
2. **Error Transparency**: Return clear, actionable error messages
3. **Path-Based**: Operate on repository paths, not repository objects
4. **Stateless**: No persistent connection state between operations
5. **Safe Defaults**: Conservative behavior that prevents data loss

## Required Operations

### 1. Clone Repository

**Purpose**: Create a local copy of a remote repository

**Signature**:
```go
Clone(ctx context.Context, url string, localPath string, options CloneOptions) error
```

**Parameters**:
- `url`: Remote repository URL (https:// or git@)
- `localPath`: Local filesystem path where repository should be created
- `options`: Configuration for clone operation

**Options**:
```go
type CloneOptions struct {
    Branch      string  // Specific branch to clone (empty = default branch)
    Depth       *int    // Shallow clone depth (nil = full clone)
    SingleBranch bool   // Clone only the specified branch
    Bare        bool    // Create a bare repository
}
```

**Behavior**:
- Creates parent directories if they don't exist
- Fails if `localPath` already exists and is not empty
- Uses authentication configured at repository level
- Returns immediately if context is cancelled

**Errors**:
- Repository URL not accessible (network, authentication)
- Local path creation failed (permissions, disk space)
- Invalid URL format
- Branch does not exist (when specified)
- Context cancelled

**Use Case**: Initial setup when first accessing a remote recipe repository

---

### 2. Fetch from Remote

**Purpose**: Download objects and refs from remote repository without merging

**Signature**:
```go
Fetch(ctx context.Context, nodePath string, options FetchOptions) error
```

**Parameters**:
- `nodePath`: Local repository path
- `options`: Configuration for fetch operation

**Options**:
```go
type FetchOptions struct {
    Remote   string   // Remote name (default: "origin")
    Branch   string   // Specific branch to fetch (empty = all branches)
    Prune    bool     // Remove remote-tracking references that no longer exist
    Tags     bool     // Fetch tags
}
```

**Behavior**:
- Updates remote-tracking branches
- Does not modify working directory or current branch
- Silent success if already up-to-date
- Uses authentication configured at repository level

**Errors**:
- Not a git repository
- Remote does not exist
- Network failure
- Authentication failure
- Context cancelled

**Use Case**: Update local cache of remote state before checking for conflicts

---

### 3. Pull from Remote

**Purpose**: Fetch from remote and integrate into current branch

**Signature**:
```go
Pull(ctx context.Context, nodePath string, options PullOptions) (*PullResult, error)
```

**Parameters**:
- `nodePath`: Local repository path
- `options`: Configuration for pull operation

**Options**:
```go
type PullOptions struct {
    Remote       string      // Remote name (default: "origin")
    Branch       string      // Remote branch (empty = tracking branch)
    FastForward  bool        // Only allow fast-forward merges
    Rebase       bool        // Rebase instead of merge
}
```

**Result**:
```go
type PullResult struct {
    Updated      bool        // Whether local branch was updated
    OldCommit    string      // Commit hash before pull
    NewCommit    string      // Commit hash after pull
    ConflictFiles []string   // Files with conflicts (if any)
}
```

**Behavior**:
- Fetches from remote
- Integrates changes into current branch
- Fails if there are uncommitted changes (unless clean)
- With `FastForward=true`: fails if merge would create merge commit

**Errors**:
- Not a git repository
- Uncommitted changes in working directory
- Merge conflicts
- Not fast-forward (when `FastForward=true`)
- Authentication failure
- Context cancelled

**Use Case**: Sync local recipe repository with remote changes

---

### 4. Push to Remote

**Purpose**: Upload local commits to remote repository

**Signature**:
```go
Push(ctx context.Context, nodePath string, options PushOptions) (*PushResult, error)
```

**Parameters**:
- `nodePath`: Local repository path
- `options`: Configuration for push operation

**Options**:
```go
type PushOptions struct {
    Remote      string   // Remote name (default: "origin")
    Branch      string   // Local branch to push (empty = current branch)
    Force       bool     // Force push (overwrite remote)
    SetUpstream bool     // Set upstream tracking branch
}
```

**Result**:
```go
type PushResult struct {
    Pushed       bool     // Whether any commits were pushed
    Rejected     bool     // Whether push was rejected
    CommitsPushed int     // Number of commits pushed
}
```

**Behavior**:
- Uploads local commits not present on remote
- Updates remote branch reference
- Fails if not fast-forward (unless `Force=true`)
- Sets upstream tracking if requested

**Errors**:
- Not a git repository
- Remote does not exist
- Not fast-forward (when `Force=false`)
- Authentication failure
- Network failure
- Permission denied (protected branch)
- Context cancelled

**Use Case**: Publish recipe changes to remote repository

---

### 5. Check Commit Ancestry

**Purpose**: Determine if one commit is an ancestor of another

**Signature**:
```go
IsAncestor(ctx context.Context, nodePath string, ancestor string, descendant string) (bool, error)
```

**Parameters**:
- `nodePath`: Local repository path
- `ancestor`: Commit hash that might be the ancestor
- `descendant`: Commit hash that might be the descendant

**Returns**:
- `true` if `ancestor` is reachable from `descendant` in commit history
- `false` if there is no path from `descendant` to `ancestor`

**Behavior**:
- Checks local git history only
- Works with commit hashes or refs (branch names, tags)
- Returns `false` if either commit does not exist (not an error)

**Errors**:
- Not a git repository
- Context cancelled

**Use Case**: Verify fast-forward is possible before attempting push/merge

---

### 6. Get Remote HEAD

**Purpose**: Retrieve the commit hash of a remote branch without fetching

**Signature**:
```go
GetRemoteHead(ctx context.Context, nodePath string, remote string, branch string) (string, error)
```

**Parameters**:
- `nodePath`: Local repository path
- `remote`: Remote name (e.g., "origin")
- `branch`: Branch name on remote

**Returns**:
- Commit hash (full 40-character SHA-1) of remote branch HEAD

**Behavior**:
- Queries remote without downloading objects
- Uses authentication configured at repository level
- Returns cached value if remote refs were recently fetched

**Errors**:
- Not a git repository
- Remote does not exist
- Branch does not exist on remote
- Network failure
- Authentication failure
- Context cancelled

**Use Case**: Check if local branch is behind remote before fetch

---

### 7. Get Current Commit

**Purpose**: Get the commit hash of the current HEAD

**Signature**:
```go
GetCurrentCommit(ctx context.Context, nodePath string) (string, error)
```

**Parameters**:
- `nodePath`: Local repository path

**Returns**:
- Commit hash (full 40-character SHA-1) of current HEAD

**Behavior**:
- Returns commit hash even in detached HEAD state
- Fast operation (no network access)

**Errors**:
- Not a git repository
- Empty repository (no commits yet)
- Context cancelled

**Use Case**: Track which commit a recipe version corresponds to

---

### 8. Checkout Commit/Branch

**Purpose**: Switch to a different branch or commit

**Signature**:
```go
Checkout(ctx context.Context, nodePath string, ref string, options CheckoutOptions) error
```

**Parameters**:
- `nodePath`: Local repository path
- `ref`: Branch name, tag, or commit hash
- `options`: Configuration for checkout operation

**Options**:
```go
type CheckoutOptions struct {
    CreateBranch  string   // Create new branch at ref (empty = no creation)
    Force         bool     // Discard local changes
    Detach        bool     // Detach HEAD (create detached HEAD state)
}
```

**Behavior**:
- Updates working directory to match ref
- Fails if there are uncommitted changes (unless `Force=true`)
- Can create new branch if `CreateBranch` is specified

**Errors**:
- Not a git repository
- Ref does not exist
- Uncommitted changes (when `Force=false`)
- Context cancelled

**Use Case**: Access historical recipe versions or switch between branches

---

### 9. Initialize Repository

**Purpose**: Create a new git repository

**Signature**:
```go
InitRepository(ctx context.Context, path string, options InitOptions) error
```

**Parameters**:
- `path`: Directory to initialize as repository
- `options`: Configuration for init operation

**Options**:
```go
type InitOptions struct {
    Bare          bool     // Create bare repository
    DefaultBranch string   // Name of default branch (empty = "main")
}
```

**Behavior**:
- Creates `.git` directory (or bare repository structure)
- Creates parent directories if needed
- Idempotent: safe to call on existing repository

**Errors**:
- Path creation failed (permissions)
- Already a git repository (when strict checking desired)
- Context cancelled

**Use Case**: Create local recipe repository for new project

---

### 10. Add Remote

**Purpose**: Add a remote repository reference

**Signature**:
```go
AddRemote(ctx context.Context, nodePath string, name string, url string) error
```

**Parameters**:
- `nodePath`: Local repository path
- `name`: Remote name (e.g., "origin", "upstream")
- `url`: Remote repository URL

**Behavior**:
- Adds remote reference to repository config
- Fails if remote with same name already exists
- Does not validate URL or test connectivity

**Errors**:
- Not a git repository
- Remote already exists
- Invalid remote name
- Context cancelled

**Use Case**: Configure remote for newly initialized repository

---

### 11. List Remotes

**Purpose**: Get all configured remotes

**Signature**:
```go
ListRemotes(ctx context.Context, nodePath string) ([]Remote, error)
```

**Returns**:
```go
type Remote struct {
    Name      string
    FetchURL  string
    PushURL   string
}
```

**Behavior**:
- Returns all configured remotes
- Returns empty slice if no remotes configured

**Errors**:
- Not a git repository
- Context cancelled

**Use Case**: Determine if repository has remote configured

---

### 12. Get File at Commit

**Purpose**: Retrieve file content from a specific commit

**Signature**:
```go
GetFileAtCommit(ctx context.Context, nodePath string, commit string, filePath string) ([]byte, error)
```

**Parameters**:
- `nodePath`: Local repository path
- `commit`: Commit hash or ref
- `filePath`: Path to file within repository

**Returns**:
- Raw file content as bytes

**Behavior**:
- Does not modify working directory
- Works with any commit in repository history
- File path is relative to repository root

**Errors**:
- Not a git repository
- Commit does not exist
- File does not exist at commit
- Context cancelled

**Use Case**: Retrieve recipe content from specific version without checking out

---

### 13. List Files at Commit

**Purpose**: List all files in a directory at a specific commit

**Signature**:
```go
ListFilesAtCommit(ctx context.Context, nodePath string, commit string, dirPath string) ([]FileInfo, error)
```

**Parameters**:
- `nodePath`: Local repository path
- `commit`: Commit hash or ref
- `dirPath`: Directory path within repository (empty = root)

**Returns**:
```go
type FileInfo struct {
    Path      string
    IsDir     bool
    Size      int64
}
```

**Behavior**:
- Does not modify working directory
- Returns empty slice for empty directory
- Recursive listing not included (only direct children)

**Errors**:
- Not a git repository
- Commit does not exist
- Directory does not exist at commit
- Context cancelled

**Use Case**: Browse available recipes at a specific version

---

## Authentication

All remote operations (Clone, Fetch, Pull, Push, GetRemoteHead) require authentication configuration:

```go
type AuthConfig struct {
    Type         AuthType
    SSHKeyPath   string        // Path to SSH private key
    SSHPassword  string        // SSH key passphrase
    HTTPSUsername string       // HTTPS username
    HTTPSPassword string       // HTTPS password/token
}

type AuthType int

const (
    AuthTypeNone AuthType = iota
    AuthTypeSSH
    AuthTypeHTTPS
)
```

**Behavior**:
- Authentication configured at Repository level, not per-operation
- SSH preferred for automation (no token expiration)
- HTTPS with Personal Access Token for GitHub/GitLab
- Agent-based SSH authentication supported
- Credentials never logged or exposed in errors

---

## Error Handling

All operations return errors that can be inspected:

```go
type GitError struct {
    Op      string    // Operation that failed (e.g., "clone", "push")
    Path    string    // Repository path
    Err     error     // Underlying error
    Stderr  string    // Git command stderr output (if applicable)
}

func (e *GitError) Error() string
func (e *GitError) Unwrap() error
```

**Standard Error Variables**:
```go
var (
    ErrNotRepository      = errors.New("not a git repository")
    ErrNotFound          = errors.New("not found")
    ErrNotFastForward    = errors.New("not a fast-forward")
    ErrConflict          = errors.New("merge conflict")
    ErrAuthFailed        = errors.New("authentication failed")
    ErrUncommittedChanges = errors.New("uncommitted changes")
    ErrNetworkFailure    = errors.New("network failure")
)
```

---

## Concurrency

**Thread Safety**:
- All operations are safe to call concurrently
- Operations on the same repository path are serialized internally
- Operations on different repository paths run in parallel

**Context Cancellation**:
- All operations respect context cancellation
- Long-running operations (clone, fetch) check context periodically
- Cancelled operations may leave repository in incomplete state
- Callers should verify repository state after cancellation

---

## Performance Considerations

**Caching**:
- Remote ref information cached for short duration (configurable)
- Credentials cached in memory (never persisted)

**Optimization Hints**:
- `Fetch` before `Push` to detect conflicts early
- Use `IsAncestor` to avoid unnecessary fetch operations
- Shallow clones for large repositories when full history not needed
- Single-branch clones when working with specific branch

**Resource Limits**:
- Maximum clone size (configurable, default: 1GB)
- Operation timeout (from context deadline)
- Concurrent operations limit (configurable, default: 10)

---

## Use Cases

### Recipe Service Workflow

1. **Initial Setup**:
   ```
   InitRepository() → AddRemote() → Clone()
   ```

2. **Create/Update Recipe**:
   ```
   Fetch() → IsAncestor() → [modify files] → Push()
   ```

3. **Activate Recipe Version**:
   ```
   GetFileAtCommit() → [validate and store]
   ```

4. **Browse Recipe History**:
   ```
   GetHistory() → GetFileAtCommit()
   ```

5. **Conflict Detection**:
   ```
   GetCurrentCommit() → Fetch() → GetRemoteHead() → IsAncestor()
   ```

### Multi-User Collaboration

1. **User A creates recipe**:
   ```
   [write file] → StageFiles() → CreateCommit() → Push()
   Success
   ```

2. **User B updates different recipe (concurrent)**:
   ```
   Fetch() → [write file] → StageFiles() → CreateCommit() → Push()
   Success (different files)
   ```

3. **User C updates same recipe as A (conflict)**:
   ```
   Fetch() → IsAncestor() → returns false → ErrNotFastForward
   Must pull and re-apply changes
   ```

---

## Testing Requirements

Implementations must support:

1. **Unit Testing**: Mock repository interface
2. **Integration Testing**: Real git operations with temporary directories
3. **Remote Testing**: Mock git server for testing remote operations
4. **Error Simulation**: Inject failures (network, auth, conflicts)
5. **Performance Testing**: Benchmark operations on large repositories

---

## Future Considerations

Operations that may be needed in the future:

1. **Merge**: Merge branches (beyond fast-forward)
2. **Rebase**: Rebase current branch onto another
3. **Cherry-Pick**: Apply specific commits
4. **Tag Operations**: Create, list, delete tags
5. **Submodules**: Initialize, update submodules
6. **Worktrees**: Multiple working directories
7. **Stash**: Save and restore working directory changes
8. **Blame**: Track line-level history
9. **Bisect**: Binary search for regression
10. **Garbage Collection**: Optimize repository size

These should follow the same design principles when added.
