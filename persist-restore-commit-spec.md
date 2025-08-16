# Git Persist and Restore Commit Recipe Operations

## Overview
This specification defines two recipe operations for the `server/ops` package that manage Git commits with idempotent access using shallow clones and thin packs. These operations follow the established patterns for Temporal recipe activities.

## Package Location
`server/ops/pkg/gitcommit/`

## Recipe Operations

### 1. PersistCommit Activity

#### Purpose
Capture a Git commit and generate a portable thin pack for external storage. This is a Temporal activity that can be invoked as part of recipe workflows.

#### Activity Definition
```go
package gitcommit

import (
    "context"
    "time"
)

// PersistCommitActivity defines the recipe operation for persisting Git commits
type PersistCommitActivity struct {
    // Required inputs
    RepoPath        string `json:"repo_path"`         // Path to the local Git repository
    StorageLocation string `json:"storage_location"`  // Directory path where thin packs will be stored
    RootHash        string `json:"root_hash"`         // Base commit hash this set was built upon
    
    // Optional configuration
    CommitMessage   string `json:"commit_message,omitempty"` // Message for the commit
    Author          string `json:"author,omitempty"`         // Author name and email
    Timeout         time.Duration `json:"timeout,omitempty"`  // Operation timeout
}

// PersistCommitOutput represents the output from persist operation
type PersistCommitOutput struct {
    CommitHash      string    `json:"commit_hash"`       // SHA-1 hash of created commit
    ParentHash      string    `json:"parent_hash"`       // SHA-1 hash of parent commit
    ThinPackPath    string    `json:"thin_pack_path"`    // Full path to generated thin pack
    ThinPackSize    int64     `json:"thin_pack_size"`    // Size of thin pack in bytes
    CreatedAt       time.Time `json:"created_at"`        // Timestamp of operation
}
```

#### Activity Function
```go
// PersistCommit performs the Git commit and thin pack generation
func PersistCommit(ctx context.Context, input PersistCommitActivity) (*PersistCommitOutput, error)
```

#### Process
1. Validate input parameters and repository state
2. Create a Git commit in the specified local repository
3. Get parent commit hash from current HEAD
4. Generate a Git thin pack containing the commit and its changes
5. Name the thin pack: `{commit_hash}-{parent_hash}-{root_hash}.pack`
6. Store the thin pack at the provided storage location
7. Return commit metadata

#### Error Conditions
- Repository path does not exist or is not a Git repository
- Storage location is not writable
- Commit creation fails
- Thin pack generation fails
- Invalid root hash

---

### 2. RestoreCommit Activity

#### Purpose
Restore a specific commit state, rebuilding from thin packs if necessary. This is a Temporal activity that can be invoked as part of recipe workflows.

#### Activity Definition
```go
// RestoreCommitActivity defines the recipe operation for restoring Git commits
type RestoreCommitActivity struct {
    // Required inputs
    RepoPath        string `json:"repo_path"`         // Path to the local Git repository
    TargetCommit    string `json:"target_commit"`     // Commit hash to restore to
    RootHash        string `json:"root_hash"`         // Root commit hash for this set
    StorageLocation string `json:"storage_location"`  // Directory containing thin packs
    
    // Optional configuration
    Force           bool   `json:"force,omitempty"`   // Force checkout even with uncommitted changes
    Timeout         time.Duration `json:"timeout,omitempty"` // Operation timeout
}

// RestoreCommitOutput represents the output from restore operation
type RestoreCommitOutput struct {
    Success         bool      `json:"success"`           // Whether restore succeeded
    CurrentCommit   string    `json:"current_commit"`    // Current commit hash after restore
    RestoredFrom    string    `json:"restored_from"`     // Source of restore: "repository" or "thin_packs"
    ThinPacksApplied []string `json:"thin_packs_applied,omitempty"` // List of applied thin packs
    RestoredAt      time.Time `json:"restored_at"`       // Timestamp of operation
}
```

#### Activity Function
```go
// RestoreCommit restores repository to a specific commit state
func RestoreCommit(ctx context.Context, input RestoreCommitActivity) (*RestoreCommitOutput, error)
```

#### Process
1. Validate input parameters
2. Check if the target commit exists in the current repository
   - If yes: checkout the target commit directly
   - If no: proceed to rebuild from thin packs
3. Reset repository to the root hash (must be available)
4. Build commit graph from thin pack filenames
5. Identify chain of thin packs from root to target
6. Apply thin packs sequentially
7. Verify final state matches target commit
8. Return restore metadata

#### Error Conditions
- Target commit cannot be found in repository or thin packs
- Root hash is not available in the repository
- Missing thin packs in the chain from root to target
- Thin pack application fails
- Verification fails (restored state doesn't match target)

---

## Thin Pack Naming Convention

### Format
```
{commit_hash}-{parent_hash}-{root_hash}.pack
```

### Components
- `commit_hash`: The SHA-1 hash of the commit contained in this pack
- `parent_hash`: The SHA-1 hash of the parent commit
- `root_hash`: The SHA-1 hash of the root commit for this entire set

### Example
```
a3f8b2c-d4e9f1a-b1c2d3e.pack
```

---

## Integration with Recipe System

### Workflow Usage

These activities can be used in Temporal workflows alongside other recipe operations:

```go
package workflows

import (
    "time"
    "go.temporal.io/sdk/workflow"
    "github.com/vibethis/server/ops/pkg/gitcommit"
    "github.com/vibethis/server/ops/pkg/gitshallow"
)

func ProcessCodeWorkflow(ctx workflow.Context, input WorkflowInput) error {
    // 1. Shallow clone to working directory
    shallowInput := gitshallow.GitShallowCloneInput{
        SourceDir:  input.SourceRepo,
        TargetDir:  input.WorkingDir,
        CommitHash: input.BaseCommit,
    }
    
    var shallowOutput gitshallow.GitShallowCloneOutput
    err := workflow.ExecuteActivity(ctx, gitshallow.GitShallowClone, shallowInput).Get(ctx, &shallowOutput)
    if err != nil {
        return err
    }
    
    // 2. Perform code modifications...
    // (other recipe operations)
    
    // 3. Persist the commit with thin pack
    persistInput := gitcommit.PersistCommitActivity{
        RepoPath:        shallowOutput.ClonedPath,
        StorageLocation: "/shared/thin-packs",
        RootHash:        input.BaseCommit,
        CommitMessage:   "Automated changes from recipe workflow",
    }
    
    var persistOutput gitcommit.PersistCommitOutput
    err = workflow.ExecuteActivity(ctx, gitcommit.PersistCommit, persistInput).Get(ctx, &persistOutput)
    if err != nil {
        return err
    }
    
    // 4. Later restore if needed
    restoreInput := gitcommit.RestoreCommitActivity{
        RepoPath:        input.WorkingDir,
        TargetCommit:    persistOutput.CommitHash,
        RootHash:        input.BaseCommit,
        StorageLocation: "/shared/thin-packs",
    }
    
    var restoreOutput gitcommit.RestoreCommitOutput
    err = workflow.ExecuteActivity(ctx, gitcommit.RestoreCommit, restoreInput).Get(ctx, &restoreOutput)
    
    return err
}
```

### Activity Registration

Register the activities with your Temporal worker:

```go
package main

import (
    "go.temporal.io/sdk/worker"
    "github.com/vibethis/server/ops/pkg/gitcommit"
)

func main() {
    // Create worker
    w := worker.New(temporalClient, "recipe-task-queue", worker.Options{})
    
    // Register Git commit activities
    w.RegisterActivity(gitcommit.PersistCommit)
    w.RegisterActivity(gitcommit.RestoreCommit)
    
    // Register other recipe activities...
    
    w.Run(worker.InterruptCh())
}
```

---

## Implementation Details

### Storage Structure
```
storage_location/
├── a3f8b2c-d4e9f1a-b1c2d3e.pack
├── d4e9f1a-e5f0a2b-b1c2d3e.pack
└── e5f0a2b-b1c2d3e-b1c2d3e.pack
```

### Idempotency Guarantees
- Multiple persist operations on the same commit produce identical thin packs
- Restore operations always result in the exact same repository state for a given commit
- Operations can be safely retried without side effects (Temporal activity retry compatible)

### Performance Optimizations
- Cache thin pack metadata to avoid repeated filesystem scans
- Use Git's built-in thin pack generation (`git pack-objects --thin`)
- Implement parallel thin pack application where possible
- Compatible with Temporal's activity heartbeating for long operations

### Shallow Clone Compatibility
- Designed to work with `gitshallow` package operations
- Root hash must be deep enough to be available in shallow clones
- Thin packs generated with awareness of shallow clone limitations
- Consider using `--shallow` flag when generating packs

---

## Testing

### Real Git Repository Tests

These tests work with actual Git repositories and operations, not mocks. They should be placed in `server/ops/pkg/gitcommit/gitcommit_test.go`.

#### Test Setup Helper
```go
package gitcommit

import (
    "context"
    "os"
    "os/exec"
    "path/filepath"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// setupTestRepo creates a real Git repository with commit history for testing
func setupTestRepo(t *testing.T) (repoPath string, rootCommit string, cleanup func()) {
    // Create temporary directory for test repo
    tmpDir, err := os.MkdirTemp("", "gitcommit-test-*")
    require.NoError(t, err)
    
    repoPath = filepath.Join(tmpDir, "repo")
    
    // Initialize Git repository
    err = os.MkdirAll(repoPath, 0755)
    require.NoError(t, err)
    
    // Git init
    cmd := exec.Command("git", "init")
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    // Configure git user for commits
    cmd = exec.Command("git", "config", "user.email", "test@example.com")
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    cmd = exec.Command("git", "config", "user.name", "Test User")
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    // Create initial commit (root commit)
    testFile := filepath.Join(repoPath, "README.md")
    err = os.WriteFile(testFile, []byte("# Test Repository\n"), 0644)
    require.NoError(t, err)
    
    cmd = exec.Command("git", "add", ".")
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    cmd = exec.Command("git", "commit", "-m", "Initial commit")
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    // Get root commit hash
    cmd = exec.Command("git", "rev-parse", "HEAD")
    cmd.Dir = repoPath
    output, err := cmd.Output()
    require.NoError(t, err)
    rootCommit = strings.TrimSpace(string(output))
    
    // Create a few more commits for testing
    for i := 1; i <= 3; i++ {
        fileName := fmt.Sprintf("file%d.txt", i)
        filePath := filepath.Join(repoPath, fileName)
        content := fmt.Sprintf("Content for file %d\n", i)
        err = os.WriteFile(filePath, []byte(content), 0644)
        require.NoError(t, err)
        
        cmd = exec.Command("git", "add", fileName)
        cmd.Dir = repoPath
        err = cmd.Run()
        require.NoError(t, err)
        
        cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("Add file %d", i))
        cmd.Dir = repoPath
        err = cmd.Run()
        require.NoError(t, err)
    }
    
    cleanup = func() {
        os.RemoveAll(tmpDir)
    }
    
    return repoPath, rootCommit, cleanup
}

// getCommitHash returns the current HEAD commit hash
func getCommitHash(t *testing.T, repoPath string) string {
    cmd := exec.Command("git", "rev-parse", "HEAD")
    cmd.Dir = repoPath
    output, err := cmd.Output()
    require.NoError(t, err)
    return strings.TrimSpace(string(output))
}

// verifyThinPack verifies that a thin pack file is valid
func verifyThinPack(t *testing.T, packPath string, repoPath string) {
    // Verify pack file exists and has content
    info, err := os.Stat(packPath)
    require.NoError(t, err)
    assert.Greater(t, info.Size(), int64(0), "Thin pack should not be empty")
    
    // Verify pack format using git verify-pack
    cmd := exec.Command("git", "verify-pack", "-v", packPath)
    cmd.Dir = repoPath
    output, err := cmd.CombinedOutput()
    require.NoError(t, err, "Pack verification failed: %s", string(output))
}
```

#### PersistCommit Real Tests
```go
func TestPersistCommit_RealGit(t *testing.T) {
    // Setup real Git repository
    repoPath, rootCommit, cleanup := setupTestRepo(t)
    defer cleanup()
    
    // Create storage directory for thin packs
    storageDir, err := os.MkdirTemp("", "thin-packs-*")
    require.NoError(t, err)
    defer os.RemoveAll(storageDir)
    
    // Make a new change to commit
    newFile := filepath.Join(repoPath, "newfile.txt")
    err = os.WriteFile(newFile, []byte("New content for persist test\n"), 0644)
    require.NoError(t, err)
    
    cmd := exec.Command("git", "add", "newfile.txt")
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    // Test PersistCommit
    input := PersistCommitActivity{
        RepoPath:        repoPath,
        StorageLocation: storageDir,
        RootHash:        rootCommit,
        CommitMessage:   "Test commit for persist operation",
        Author:          "Test User <test@example.com>",
    }
    
    output, err := PersistCommit(context.Background(), input)
    require.NoError(t, err)
    
    // Verify output
    assert.NotEmpty(t, output.CommitHash)
    assert.NotEmpty(t, output.ParentHash)
    assert.NotEmpty(t, output.ThinPackPath)
    assert.Greater(t, output.ThinPackSize, int64(0))
    
    // Verify commit was created in repo
    currentCommit := getCommitHash(t, repoPath)
    assert.Equal(t, output.CommitHash, currentCommit)
    
    // Verify thin pack file
    expectedPackName := fmt.Sprintf("%s-%s-%s.pack", 
        output.CommitHash[:7], output.ParentHash[:7], rootCommit[:7])
    expectedPackPath := filepath.Join(storageDir, expectedPackName)
    assert.Equal(t, expectedPackPath, output.ThinPackPath)
    assert.FileExists(t, output.ThinPackPath)
    
    verifyThinPack(t, output.ThinPackPath, repoPath)
}

func TestPersistCommit_MultipleCommits(t *testing.T) {
    // Test creating a chain of commits with thin packs
    repoPath, rootCommit, cleanup := setupTestRepo(t)
    defer cleanup()
    
    storageDir, err := os.MkdirTemp("", "thin-packs-*")
    require.NoError(t, err)
    defer os.RemoveAll(storageDir)
    
    var commits []string
    
    // Create chain of 5 commits
    for i := 1; i <= 5; i++ {
        // Make a change
        fileName := fmt.Sprintf("chain-file-%d.txt", i)
        filePath := filepath.Join(repoPath, fileName)
        content := fmt.Sprintf("Chain content %d\n", i)
        err = os.WriteFile(filePath, []byte(content), 0644)
        require.NoError(t, err)
        
        cmd := exec.Command("git", "add", fileName)
        cmd.Dir = repoPath
        err = cmd.Run()
        require.NoError(t, err)
        
        // Persist commit
        input := PersistCommitActivity{
            RepoPath:        repoPath,
            StorageLocation: storageDir,
            RootHash:        rootCommit,
            CommitMessage:   fmt.Sprintf("Chain commit %d", i),
        }
        
        output, err := PersistCommit(context.Background(), input)
        require.NoError(t, err)
        
        commits = append(commits, output.CommitHash)
        
        // Verify thin pack
        assert.FileExists(t, output.ThinPackPath)
        verifyThinPack(t, output.ThinPackPath, repoPath)
    }
    
    // Verify we have 5 different commits
    assert.Len(t, commits, 5)
    
    // Verify all thin packs exist
    packFiles, err := filepath.Glob(filepath.Join(storageDir, "*.pack"))
    require.NoError(t, err)
    assert.Len(t, packFiles, 5)
}
```

#### RestoreCommit Real Tests
```go
func TestRestoreCommit_FromRepository(t *testing.T) {
    // Test restoring a commit that exists in the repository
    repoPath, rootCommit, cleanup := setupTestRepo(t)
    defer cleanup()
    
    storageDir, err := os.MkdirTemp("", "thin-packs-*")
    require.NoError(t, err)
    defer os.RemoveAll(storageDir)
    
    // Get current commit before making changes
    targetCommit := getCommitHash(t, repoPath)
    
    // Make additional commits
    for i := 1; i <= 3; i++ {
        fileName := fmt.Sprintf("extra-file-%d.txt", i)
        filePath := filepath.Join(repoPath, fileName)
        err = os.WriteFile(filePath, []byte(fmt.Sprintf("Extra %d\n", i)), 0644)
        require.NoError(t, err)
        
        cmd := exec.Command("git", "add", fileName)
        cmd.Dir = repoPath
        err = cmd.Run()
        require.NoError(t, err)
        
        cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("Extra commit %d", i))
        cmd.Dir = repoPath
        err = cmd.Run()
        require.NoError(t, err)
    }
    
    // Current commit should be different
    currentCommit := getCommitHash(t, repoPath)
    assert.NotEqual(t, targetCommit, currentCommit)
    
    // Restore to target commit
    input := RestoreCommitActivity{
        RepoPath:        repoPath,
        TargetCommit:    targetCommit,
        RootHash:        rootCommit,
        StorageLocation: storageDir,
    }
    
    output, err := RestoreCommit(context.Background(), input)
    require.NoError(t, err)
    
    // Verify restoration
    assert.True(t, output.Success)
    assert.Equal(t, targetCommit, output.CurrentCommit)
    assert.Equal(t, "repository", output.RestoredFrom)
    assert.Empty(t, output.ThinPacksApplied)
    
    // Verify repo state
    restoredCommit := getCommitHash(t, repoPath)
    assert.Equal(t, targetCommit, restoredCommit)
}

func TestRestoreCommit_FromThinPacks(t *testing.T) {
    // Test restoring from thin packs when commit doesn't exist in repo
    repoPath, rootCommit, cleanup := setupTestRepo(t)
    defer cleanup()
    
    storageDir, err := os.MkdirTemp("", "thin-packs-*")
    require.NoError(t, err)
    defer os.RemoveAll(storageDir)
    
    // Create a commit and persist it
    testFile := filepath.Join(repoPath, "test-restore.txt")
    err = os.WriteFile(testFile, []byte("Content to restore\n"), 0644)
    require.NoError(t, err)
    
    cmd := exec.Command("git", "add", "test-restore.txt")
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    persistInput := PersistCommitActivity{
        RepoPath:        repoPath,
        StorageLocation: storageDir,
        RootHash:        rootCommit,
        CommitMessage:   "Commit to restore from thin pack",
    }
    
    persistOutput, err := PersistCommit(context.Background(), persistInput)
    require.NoError(t, err)
    targetCommit := persistOutput.CommitHash
    
    // Reset repo to root (simulating shallow clone scenario)
    cmd = exec.Command("git", "reset", "--hard", rootCommit)
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    // Verify target commit is not in repo
    cmd = exec.Command("git", "cat-file", "-e", targetCommit)
    cmd.Dir = repoPath
    err = cmd.Run()
    assert.Error(t, err, "Target commit should not exist in repo")
    
    // Restore from thin pack
    restoreInput := RestoreCommitActivity{
        RepoPath:        repoPath,
        TargetCommit:    targetCommit,
        RootHash:        rootCommit,
        StorageLocation: storageDir,
    }
    
    output, err := RestoreCommit(context.Background(), restoreInput)
    require.NoError(t, err)
    
    // Verify restoration
    assert.True(t, output.Success)
    assert.Equal(t, targetCommit, output.CurrentCommit)
    assert.Equal(t, "thin_packs", output.RestoredFrom)
    assert.Len(t, output.ThinPacksApplied, 1)
    
    // Verify file was restored
    assert.FileExists(t, testFile)
    content, err := os.ReadFile(testFile)
    require.NoError(t, err)
    assert.Equal(t, "Content to restore\n", string(content))
}

func TestRestoreCommit_ChainOfThinPacks(t *testing.T) {
    // Test restoring through a chain of thin packs
    repoPath, rootCommit, cleanup := setupTestRepo(t)
    defer cleanup()
    
    storageDir, err := os.MkdirTemp("", "thin-packs-*")
    require.NoError(t, err)
    defer os.RemoveAll(storageDir)
    
    var commits []string
    
    // Create chain of commits with thin packs
    for i := 1; i <= 4; i++ {
        fileName := fmt.Sprintf("chain-%d.txt", i)
        filePath := filepath.Join(repoPath, fileName)
        err = os.WriteFile(filePath, []byte(fmt.Sprintf("Chain %d\n", i)), 0644)
        require.NoError(t, err)
        
        cmd := exec.Command("git", "add", fileName)
        cmd.Dir = repoPath
        err = cmd.Run()
        require.NoError(t, err)
        
        input := PersistCommitActivity{
            RepoPath:        repoPath,
            StorageLocation: storageDir,
            RootHash:        rootCommit,
            CommitMessage:   fmt.Sprintf("Chain commit %d", i),
        }
        
        output, err := PersistCommit(context.Background(), input)
        require.NoError(t, err)
        commits = append(commits, output.CommitHash)
    }
    
    targetCommit := commits[len(commits)-1]
    
    // Reset to root
    cmd := exec.Command("git", "reset", "--hard", rootCommit)
    cmd.Dir = repoPath
    err = cmd.Run()
    require.NoError(t, err)
    
    // Restore through chain
    restoreInput := RestoreCommitActivity{
        RepoPath:        repoPath,
        TargetCommit:    targetCommit,
        RootHash:        rootCommit,
        StorageLocation: storageDir,
    }
    
    output, err := RestoreCommit(context.Background(), restoreInput)
    require.NoError(t, err)
    
    // Verify restoration
    assert.True(t, output.Success)
    assert.Equal(t, targetCommit, output.CurrentCommit)
    assert.Equal(t, "thin_packs", output.RestoredFrom)
    assert.Len(t, output.ThinPacksApplied, 4)
    
    // Verify all files exist
    for i := 1; i <= 4; i++ {
        fileName := fmt.Sprintf("chain-%d.txt", i)
        filePath := filepath.Join(repoPath, fileName)
        assert.FileExists(t, filePath)
    }
}
```

### Integration Tests with Temporal
```go
func TestGitCommitWorkflow_RealGit(t *testing.T) {
    testSuite := &testsuite.WorkflowTestSuite{}
    env := testSuite.NewTestWorkflowEnvironment()
    
    // Setup real Git repository
    repoPath, rootCommit, cleanup := setupTestRepo(t)
    defer cleanup()
    
    storageDir, err := os.MkdirTemp("", "workflow-thin-packs-*")
    require.NoError(t, err)
    defer os.RemoveAll(storageDir)
    
    // Register real activities (not mocks)
    env.RegisterActivity(gitcommit.PersistCommit)
    env.RegisterActivity(gitcommit.RestoreCommit)
    
    // Run workflow with real Git operations
    input := WorkflowInput{
        RepoPath:        repoPath,
        StorageLocation: storageDir,
        RootHash:        rootCommit,
    }
    
    env.ExecuteWorkflow(ProcessCodeWorkflow, input)
    
    assert.True(t, env.IsWorkflowCompleted())
    assert.NoError(t, env.GetWorkflowError())
    
    // Verify actual Git state and thin packs
    packFiles, err := filepath.Glob(filepath.Join(storageDir, "*.pack"))
    assert.NoError(t, err)
    assert.Greater(t, len(packFiles), 0, "Should have created thin packs")
}
```

---

## Security Considerations
- Validate all commit hashes before operations
- Ensure thin packs are not corrupted (use checksums)
- Implement access controls on the storage location
- Sanitize file paths to prevent directory traversal attacks
- Follow Temporal security best practices for activity execution

---

## Future Enhancements
- Support for remote storage locations (S3, GCS, etc.) as recipe configuration
- Compression of thin packs for storage efficiency
- Garbage collection of orphaned thin packs via scheduled workflows
- Incremental thin pack generation for large commits
- Parallel restore operations for multiple commits
- Integration with other Git-related recipe operations in `server/ops`