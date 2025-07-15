# server/git

Git version control integration for vibethis boxes. Provides read-only Git operations on node directories without initializing new repositories.

## Architecture

- **pkg/git/repository.go**: Public interface and types for Git operations
- **internal/commands/**: Implementation using command-line git executable

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
```

## Operations

### Status Tracking
- Current branch detection
- File status (modified, added, deleted, untracked)
- Clean/dirty state
- Remote tracking (ahead/behind counts)

### Diff Generation
- Unstaged changes (`staged=false`)
- Staged changes (`staged=true`)
- Full diff output with context

### History
- Commit log with configurable limit
- Includes hash, author, date, and message
- Short hash generation for display

### Staging/Committing
- Stage specific files for commit
- Unstage files
- Create commits with custom messages
- Supports default author/email configuration

## Error Handling

- Returns "not a git repository" for non-git directories
- No automatic repository initialization
- Validates staged changes before committing
- Command execution errors wrapped with context

## Integration Notes

- Used by API handlers for Git operations on boxes
- All operations require existing Git repositories
- Context-aware for cancellation support
- Thread-safe command execution

## Configuration

```go
type Config struct {
    DefaultAuthor string  // Used when no Git user configured
    DefaultEmail  string  // Used when no Git email configured
}
```

## Implementation Details

- Uses exec.CommandContext for Git CLI operations
- Repository detection via `git rev-parse --git-dir`
- Status parsing from `git status --porcelain`
- Commit formatting with pipe-delimited fields
- Environment variable injection for author info