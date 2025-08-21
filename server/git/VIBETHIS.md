# Git Module Documentation

## Overview

The Git module provides comprehensive Git repository operations for the vibethis system. It offers both direct repository management through the Repository interface and specialized activities for file collection, commit persistence, and shallow cloning operations that integrate with the recipe workflow system.

## Architecture

### Core Components

- **Repository Interface** (`pkg/git/repository.go`): Primary interface for Git operations including status, diff, history, commit, and file staging
- **Commands Implementation** (`internal/commands/commands.go`): Command-line Git wrapper providing concrete implementations
- **Activity System**: Registerable activities for workflow integration
  - **GitFileCollector** (`pkg/gitcollector/`): Git-aware file collection with filtering and metadata
  - **GitCommit Activities** (`pkg/gitcommit/`): Commit persistence and restoration using thin packs
  - **GitShallow** (`pkg/gitshallow/`): Shallow repository cloning operations
- **Common Utilities** (`pkg/common/`): Shared Git command execution, validation, and metadata parsing

### Key Relationships

```
Repository Interface
    ↓
Commands Implementation (git CLI wrapper)
    ↓
Common Utilities (validation, execution)

Activity System
    ↓
RegisterableActivity Interface
    ↓
Recipe Worker Integration
```

## Key Interfaces

### Repository Interface

```go
type Repository interface {
    GetStatus(ctx context.Context, nodePath string) (*Status, error)
    GetDiff(ctx context.Context, nodePath string, staged bool) (string, error)
    GetHistory(ctx context.Context, nodePath string, limit int) ([]Commit, error)
    CreateCommit(ctx context.Context, nodePath, message string) error
    StageFiles(ctx context.Context, nodePath string, files []string) error
    UnstageFiles(ctx context.Context, nodePath string, files []string) error
}

func NewRepository(config Config) Repository
```

### Git File Collector Activity

```go
type GitFileCollectorInput struct {
    ContextDir       string   `json:"context_dir" validate:"required,dir"`
    FilePatterns     []string `json:"file_patterns,omitempty"`
    ExcludePatterns  []string `json:"exclude_patterns,omitempty"`
    MaxFileSize      int      `json:"max_file_size,omitempty"`
    MaxTotalSize     int      `json:"max_total_size,omitempty"`
    IncludeStaged    bool     `json:"include_staged,omitempty"`
    IncludeUntracked bool     `json:"include_untracked,omitempty"`
    UseGitignore     bool     `json:"use_gitignore,omitempty"`
    ExcludeBinary    bool     `json:"exclude_binary,omitempty"`
    AutoDetectType   bool     `json:"auto_detect_type,omitempty"`
    IncludeMetadata  bool     `json:"include_metadata,omitempty"`
}

func NewGitFileCollectorActivity() RegisterableActivity[GitFileCollectorConfig, GitFileCollectorInput, GitFileCollectorOutput]
```

### Commit Persistence Activities

```go
type PersistCommitInput struct {
    RepoPath        string        `json:"repo_path"`
    StorageLocation string        `json:"storage_location"`
    RootHash        string        `json:"root_hash"`
    CommitMessage   string        `json:"commit_message,omitempty"`
    Author          string        `json:"author,omitempty"`
    Timeout         time.Duration `json:"timeout,omitempty"`
}

type RestoreCommitInput struct {
    RepoPath        string        `json:"repo_path"`
    TargetCommit    string        `json:"target_commit"`
    RootHash        string        `json:"root_hash"`
    StorageLocation string        `json:"storage_location"`
    Force           bool          `json:"force,omitempty"`
    Timeout         time.Duration `json:"timeout,omitempty"`
}

func NewPersistCommitActivity() RegisterableActivity[PersistCommitConfig, PersistCommitInput, PersistCommitOutput]
func NewRestoreCommitActivity() RegisterableActivity[RestoreCommitConfig, RestoreCommitInput, RestoreCommitOutput]
```

### Shallow Clone Activity

```go
type GitShallowInput struct {
    SourceDir  string `json:"source_dir"`
    TargetDir  string `json:"target_dir"`
    CommitHash string `json:"commit_hash"`
}

func NewGitShallowActivity() RegisterableActivity[GitShallowConfig, GitShallowInput, GitShallowOutput]
```

### Common Utilities

```go
func ExecuteGitCommand(ctx context.Context, repoPath string, args ...string) ([]byte, error)
func ValidateRepository(repoPath string) error
func FindGitRoot(path string) (string, error)
func GetCommitHash(ctx context.Context, repoPath, ref string) (string, error)
func CommitExists(ctx context.Context, repoPath, commitHash string) bool
```

## Usage Examples

### Basic Repository Operations

```go
// Create repository instance
config := git.Config{
    DefaultAuthor: "Bot User",
    DefaultEmail:  "bot@example.com",
}
repo := git.NewRepository(config)

// Get repository status
status, err := repo.GetStatus(ctx, "/path/to/repo")
if err != nil {
    return err
}

// Stage and commit files
err = repo.StageFiles(ctx, "/path/to/repo", []string{"file1.go", "file2.go"})
if err != nil {
    return err
}

err = repo.CreateCommit(ctx, "/path/to/repo", "Add new features")
if err != nil {
    return err
}
```

### Git File Collection Activity

```yaml
# Recipe YAML
activities:
  - type: git_file_collector
    inputs:
      context_dir: "/workspace/project"
      file_patterns: ["*.go", "*.yaml"]
      exclude_patterns: ["*_test.go", "vendor/**"]
      max_file_size: 100000
      include_staged: true
      exclude_binary: true
      auto_detect_type: true
```

```go
// Programmatic usage
activity := gitcollector.NewGitFileCollectorActivity()
input := gitcollector.GitFileCollectorInput{
    ContextDir:      "/workspace/project",
    FilePatterns:    []string{"*.go", "*.yaml"},
    ExcludePatterns: []string{"*_test.go"},
    MaxFileSize:     100000,
    IncludeStaged:   true,
    ExcludeBinary:   true,
    AutoDetectType:  true,
}

output, err := activity.Execute(ctx, config, input)
```

### Commit Persistence Workflow

```yaml
# Persist current state
- type: git_persist_commit
  inputs:
    repo_path: "/workspace/project"
    storage_location: "/data/thin-packs"
    root_hash: "abc123def456"
    commit_message: "Checkpoint: feature implementation"

# Later restore to specific commit
- type: git_restore_commit
  inputs:
    repo_path: "/workspace/project"
    target_commit: "def456abc789"
    root_hash: "abc123def456"
    storage_location: "/data/thin-packs"
```

### Shallow Repository Clone

```go
activity := gitshallow.NewGitShallowActivity()
input := gitshallow.GitShallowInput{
    SourceDir:  "/original/repo",
    TargetDir:  "/workspace/clone",
    CommitHash: "abc123def456",
}

output, err := activity.Execute(ctx, config, input)
if err != nil {
    return err
}
// Repository cloned to output.ClonedPath
```

## Configuration

### Git File Collector Configuration

Configuration file: `config/activities.yaml`

```yaml
git_file_collector:
  default_max_file_size: 100000      # 100KB per file
  default_max_total_size: 10485760   # 10MB total
  default_include_staged: true       # Include staged files
  default_include_untracked: false   # Exclude untracked files
  default_use_gitignore: true        # Respect .gitignore
  default_exclude_binary: true       # Skip binary files
```

### Activity Registration

```go
// Get all available git activities
activities := activity.GetAll()

// Individual activity registration
gitFileCollector := gitcollector.NewGitFileCollectorActivity()
persistCommit := gitcommit.NewPersistCommitActivity()
restoreCommit := gitcommit.NewRestoreCommitActivity()
shallowClone := gitshallow.NewGitShallowActivity()
```

### Common Git Configuration

```go
type GitConfig struct {
    Author  string        `json:"author,omitempty"`
    Email   string        `json:"email,omitempty"`
    Timeout time.Duration `json:"timeout,omitempty"`
}
```