# Git File Collector Operation Specification

## Overview
This specification defines a new self-registering operation for collecting files from git repositories. The operation follows the `RegisterableActivity` pattern and is automatically discoverable by the recipe-worker.

## Purpose
Provide git-aware file collection that:
- Respects version control boundaries (.gitignore)
- Collects only relevant source files
- Excludes build artifacts and dependencies
- Excludes binary files (images, compiled binaries, etc.)
- Supports staged and untracked files optionally
- Formats files for LLM consumption
- Leverages existing git functionality from server/git packages

## Operation Details

### Operation Type
```yaml
op: git_file_collector
```

### Usage in Recipes
```yaml
# Basic usage
- id: collect_files
  op: git_file_collector
  inputs:
    context_dir: "/project/src"

# With filtering
- id: collect_go_files
  op: git_file_collector
  inputs:
    context_dir: "/project"
    file_patterns:
      - "**/*.go"
      - "go.mod"
      - "go.sum"
    exclude_patterns:
      - "**/*_test.go"
      - "vendor/**"
    include_staged: true
    include_untracked: false
    exclude_binary: true

# With size limits
- id: collect_with_limits
  op: git_file_collector
  inputs:
    context_dir: "."
    max_file_size: 500000  # 500KB per file
    max_total_size: 10485760  # 10MB total
```

## Implementation

### Activity Structure
```go
// server/git/pkg/gitcollector/git_file_collector.go

package gitcollector

import (
    "context"
    "path/filepath"
    "time"
    "github.com/vibethis/server/recipe-worker/pkg/worker"
    "github.com/vibethis/server/llm/adapters"
    "github.com/vibethis/server/git/pkg/common"
    "github.com/vibethis/server/git/internal/commands"
)

// GitFileCollectorConfig provides configuration for the activity
type GitFileCollectorConfig struct {
    // Default limits
    DefaultMaxFileSize  int `yaml:"default_max_file_size"`
    DefaultMaxTotalSize int `yaml:"default_max_total_size"`
    
    // Default behavior
    DefaultIncludeStaged    bool `yaml:"default_include_staged"`
    DefaultIncludeUntracked bool `yaml:"default_include_untracked"`
    DefaultUseGitignore     bool `yaml:"default_use_gitignore"`
    DefaultExcludeBinary    bool `yaml:"default_exclude_binary"`
}

// GitFileCollectorInput defines the input parameters
type GitFileCollectorInput struct {
    // Required
    ContextDir string `json:"context_dir" validate:"required,dir"`
    
    // File filtering
    FilePatterns    []string `json:"file_patterns,omitempty"`
    ExcludePatterns []string `json:"exclude_patterns,omitempty"`
    
    // Size limits
    MaxFileSize  int `json:"max_file_size,omitempty"`
    MaxTotalSize int `json:"max_total_size,omitempty"`
    
    // Git options
    IncludeStaged    bool `json:"include_staged,omitempty"`
    IncludeUntracked bool `json:"include_untracked,omitempty"`
    UseGitignore     bool `json:"use_gitignore,omitempty"`
    ExcludeBinary    bool `json:"exclude_binary,omitempty"`
    
    // Processing options
    AutoDetectType  bool `json:"auto_detect_type,omitempty"`
    IncludeMetadata bool `json:"include_metadata,omitempty"`
}

// GitFileCollectorOutput defines the output structure
type GitFileCollectorOutput struct {
    Files      []adapters.File   `json:"files"`
    FileCount  int               `json:"file_count"`
    TotalSize  int64             `json:"total_size"`
    Repository GitRepositoryInfo `json:"repository"`
    Statistics FileStatistics    `json:"statistics,omitempty"`
}

type GitRepositoryInfo struct {
    Branch        string    `json:"branch"`
    CommitHash    string    `json:"commit_hash"`
    CommitMessage string    `json:"commit_message"`
    CommitTime    time.Time `json:"commit_time"`
    Author        string    `json:"author"`
    IsDirty       bool      `json:"is_dirty"`
    RemoteURL     string    `json:"remote_url,omitempty"`
}

type FileStatistics struct {
    FilesByType      map[string]int `json:"files_by_type"`
    FilesByExtension map[string]int `json:"files_by_extension"`
    LargestFile      string         `json:"largest_file"`
    LargestFileSize  int64          `json:"largest_file_size"`
    SkippedFiles     int            `json:"skipped_files"`
    SkippedReasons   map[string]int `json:"skipped_reasons,omitempty"`
}
```

### Activity Implementation
```go
// GitFileCollectorActivity implements RegisterableActivity
type GitFileCollectorActivity struct {
    logger       Logger
    fileDetector FileTypeDetector
    gitRepo      *commands.Repository
}

// Ensure we implement the interface
var _ worker.RegisterableActivity[GitFileCollectorConfig, GitFileCollectorInput, GitFileCollectorOutput] = (*GitFileCollectorActivity)(nil)

// NewGitFileCollectorActivity creates a new activity instance
func NewGitFileCollectorActivity() worker.RegisterableActivity[GitFileCollectorConfig, GitFileCollectorInput, GitFileCollectorOutput] {
    return &GitFileCollectorActivity{
        fileDetector: NewFileTypeDetector(),
        gitRepo:      commands.New("", ""),
    }
}

// GetMetadata returns activity metadata for registration
func (a *GitFileCollectorActivity) GetMetadata() worker.ActivityMetadata {
    return worker.ActivityMetadata{
        Type:           "git_file_collector",
        Name:           "Git File Collector",
        Description:    "Collects files from a git repository with filtering and metadata",
        Version:        "1.0.0",
        DefaultTimeout: 30 * time.Second,
        RetryPolicy: &worker.RetryPolicy{
            MaximumAttempts:    3,
            InitialInterval:    1 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    10 * time.Second,
        },
    }
}

// Execute runs the git file collection
func (a *GitFileCollectorActivity) Execute(
    ctx context.Context,
    config GitFileCollectorConfig,
    input GitFileCollectorInput,
) (GitFileCollectorOutput, error) {
    // Apply defaults from config
    a.applyDefaults(&input, config)
    
    // Validate input
    if err := a.validateInput(input); err != nil {
        return GitFileCollectorOutput{}, err
    }
    
    // Validate git repository using common utils
    if err := common.ValidateRepository(input.ContextDir); err != nil {
        return GitFileCollectorOutput{}, fmt.Errorf("not a git repository: %w", err)
    }
    
    // Get repository information using git commands.Repository
    status, err := a.gitRepo.GetStatus(ctx, input.ContextDir)
    if err != nil {
        return GitFileCollectorOutput{}, fmt.Errorf("failed to get repository status: %w", err)
    }
    
    // Get latest commit info
    commits, err := a.gitRepo.GetHistory(ctx, input.ContextDir, 1)
    if err != nil || len(commits) == 0 {
        return GitFileCollectorOutput{}, fmt.Errorf("failed to get commit history: %w", err)
    }
    latestCommit := commits[0]
    
    // Build repository info from status and commit
    repoInfo := GitRepositoryInfo{
        Branch:        status.Branch,
        CommitHash:    latestCommit.Hash,
        CommitMessage: latestCommit.Message,
        CommitTime:    latestCommit.Date,
        Author:        latestCommit.Author,
        IsDirty:       !status.Clean,
        RemoteURL:     "", // Can be extracted if needed
    }
    
    // List files using git
    filePaths, err := a.listGitFiles(ctx, input)
    if err != nil {
        return GitFileCollectorOutput{}, fmt.Errorf("failed to list files: %w", err)
    }
    
    // Apply pattern filters
    filePaths = a.applyPatternFilters(filePaths, input)
    
    // Collect file contents
    files, stats, err := a.collectFiles(ctx, input.ContextDir, filePaths, input)
    if err != nil {
        return GitFileCollectorOutput{}, fmt.Errorf("failed to collect files: %w", err)
    }
    
    return GitFileCollectorOutput{
        Files:      files,
        FileCount:  len(files),
        TotalSize:  stats.TotalSize,
        Repository: repoInfo,
        Statistics: stats,
    }, nil
}
```

### Core Implementation Methods
```go
func (a *GitFileCollectorActivity) listGitFiles(ctx context.Context, input GitFileCollectorInput) ([]string, error) {
    var files []string
    
    // Get tracked files using common.ExecuteGitCommand
    output, err := common.ExecuteGitCommand(ctx, input.ContextDir, "ls-files")
    if err != nil {
        return nil, fmt.Errorf("git ls-files failed: %w", err)
    }
    
    for _, line := range strings.Split(string(output), "\n") {
        if line != "" {
            files = append(files, line)
        }
    }
    
    // Get staged files if requested
    if input.IncludeStaged {
        output, err = common.ExecuteGitCommand(ctx, input.ContextDir, "diff", "--staged", "--name-only")
        if err == nil && len(output) > 0 {
            for _, line := range strings.Split(string(output), "\n") {
                if line != "" {
                    files = append(files, line)
                }
            }
        }
    }
    
    // Get untracked files if requested
    if input.IncludeUntracked {
        args := []string{"ls-files", "--others"}
        if input.UseGitignore {
            args = append(args, "--exclude-standard")
        }
        
        output, err = common.ExecuteGitCommand(ctx, input.ContextDir, args...)
        if err == nil && len(output) > 0 {
            for _, line := range strings.Split(string(output), "\n") {
                if line != "" {
                    files = append(files, line)
                }
            }
        }
    }
    
    return a.deduplicateFiles(files), nil
}

func (a *GitFileCollectorActivity) collectFiles(
    ctx context.Context,
    baseDir string,
    filePaths []string,
    input GitFileCollectorInput,
) ([]adapters.File, FileStatistics, error) {
    var files []adapters.File
    var totalSize int64
    stats := FileStatistics{
        FilesByType:      make(map[string]int),
        FilesByExtension: make(map[string]int),
        SkippedReasons:   make(map[string]int),
    }
    
    for _, filePath := range filePaths {
        fullPath := filepath.Join(baseDir, filePath)
        
        // Check file size
        info, err := os.Stat(fullPath)
        if err != nil {
            stats.SkippedFiles++
            stats.SkippedReasons["not_found"]++
            continue
        }
        
        if input.MaxFileSize > 0 && info.Size() > int64(input.MaxFileSize) {
            stats.SkippedFiles++
            stats.SkippedReasons["too_large"]++
            continue
        }
        
        if input.MaxTotalSize > 0 && totalSize+info.Size() > int64(input.MaxTotalSize) {
            stats.SkippedFiles++
            stats.SkippedReasons["total_size_exceeded"]++
            continue
        }
        
        // Read file content
        content, err := os.ReadFile(fullPath)
        if err != nil {
            stats.SkippedFiles++
            stats.SkippedReasons["read_error"]++
            continue
        }
        
        // Check for binary files if exclusion is enabled
        if input.ExcludeBinary && isBinaryFile(content, filePath) {
            stats.SkippedFiles++
            stats.SkippedReasons["binary_file"]++
            continue
        }
        
        // Detect file type
        fileType := adapters.FileTypeText
        mimeType := "text/plain"
        if input.AutoDetectType {
            fileType = a.fileDetector.DetectType(content, filePath)
            mimeType = a.fileDetector.DetectMimeType(content, filePath)
        }
        
        // Create file object
        file := adapters.File{
            Path:     filePath,
            Name:     filepath.Base(filePath),
            Content:  content,
            MimeType: mimeType,
            Type:     fileType,
        }
        
        if input.IncludeMetadata {
            file.Metadata = map[string]interface{}{
                "size":         info.Size(),
                "modified":     info.ModTime(),
                "permissions":  info.Mode().String(),
                "is_symlink":   info.Mode()&os.ModeSymlink != 0,
            }
        }
        
        files = append(files, file)
        totalSize += info.Size()
        
        // Update statistics
        stats.FilesByType[string(fileType)]++
        ext := filepath.Ext(filePath)
        if ext != "" {
            stats.FilesByExtension[ext]++
        }
        
        if info.Size() > stats.LargestFileSize {
            stats.LargestFile = filePath
            stats.LargestFileSize = info.Size()
        }
    }
    
    stats.TotalSize = totalSize
    return files, stats, nil
}
```

## Registration

### Auto-Registration
```go
// server/git/pkg/activity/registry.go

package activity

import (
    "github.com/vibethis/server/git/pkg/gitcollector"
)

// RegisterGitActivities registers all git-related activities
func RegisterGitActivities() []RegisterableActivity {
    return []RegisterableActivity{
        gitcollector.NewGitFileCollectorActivity(),
        // Future git operations can be added here
    }
}
```

## Configuration

### Default Configuration
```yaml
# server/git/config/activities.yaml

git_file_collector:
  default_max_file_size: 100000      # 100KB
  default_max_total_size: 10485760   # 10MB
  default_include_staged: true
  default_include_untracked: false
  default_use_gitignore: true
  default_exclude_binary: true
```

## Validation

### Input Validation
```go
func (a *GitFileCollectorActivity) validateInput(input GitFileCollectorInput) error {
    // Check required fields
    if input.ContextDir == "" {
        return fmt.Errorf("context_dir is required")
    }
    
    // Check directory exists
    info, err := os.Stat(input.ContextDir)
    if err != nil {
        return fmt.Errorf("context directory not accessible: %w", err)
    }
    
    if !info.IsDir() {
        return fmt.Errorf("context_dir must be a directory")
    }
    
    // Validate patterns
    for _, pattern := range input.FilePatterns {
        if _, err := filepath.Match(pattern, "test"); err != nil {
            return fmt.Errorf("invalid file pattern %s: %w", pattern, err)
        }
    }
    
    for _, pattern := range input.ExcludePatterns {
        if _, err := filepath.Match(pattern, "test"); err != nil {
            return fmt.Errorf("invalid exclude pattern %s: %w", pattern, err)
        }
    }
    
    // Validate size limits
    if input.MaxFileSize < 0 {
        return fmt.Errorf("max_file_size must be non-negative")
    }
    
    if input.MaxTotalSize < 0 {
        return fmt.Errorf("max_total_size must be non-negative")
    }
    
    return nil
}

// isBinaryFile detects if a file is binary based on extension and content
func isBinaryFile(content []byte, path string) bool {
    // Check by extension first (similar to pre-commit config)
    binaryExtensions := []string{
        ".gif", ".png", ".jpg", ".jpeg", ".ico", ".pdf",
        ".zip", ".tar", ".gz", ".bz2", ".7z", ".rar",
        ".exe", ".dll", ".so", ".dylib", ".a", ".o",
        ".pyc", ".pyo", ".class", ".jar", ".war",
        ".woff", ".woff2", ".ttf", ".otf", ".eot",
        ".mp3", ".mp4", ".avi", ".mov", ".wmv",
        ".db", ".sqlite", ".rdb",
    }
    
    ext := strings.ToLower(filepath.Ext(path))
    for _, binExt := range binaryExtensions {
        if ext == binExt {
            return true
        }
    }
    
    // Check for null bytes in first 8192 bytes (common heuristic)
    checkLen := len(content)
    if checkLen > 8192 {
        checkLen = 8192
    }
    
    for i := 0; i < checkLen; i++ {
        if content[i] == 0 {
            return true
        }
    }
    
    return false
}
```

## Error Handling

### Error Types
```go
// server/git/pkg/gitcollector/errors.go

package gitcollector

type GitOperationError struct {
    Operation string
    Command   string
    ExitCode  int
    Stderr    string
    Message   string
}

func (e *GitOperationError) Error() string {
    return fmt.Sprintf("git %s failed: %s", e.Operation, e.Message)
}

type FileCollectionError struct {
    Path    string
    Reason  string
    Details map[string]interface{}
}

func (e *FileCollectionError) Error() string {
    return fmt.Sprintf("failed to collect %s: %s", e.Path, e.Reason)
}
```

## Testing

### Unit Tests
```go
// server/git/pkg/gitcollector/git_file_collector_test.go

func TestGitFileCollectorActivity(t *testing.T) {
    activity := NewGitFileCollectorActivity()
    
    t.Run("Metadata", func(t *testing.T) {
        metadata := activity.GetMetadata()
        assert.Equal(t, "git_file_collector", metadata.Type)
        assert.Equal(t, "1.0.0", metadata.Version)
        assert.NotNil(t, metadata.RetryPolicy)
    })
    
    t.Run("CollectTrackedFiles", func(t *testing.T) {
        tmpDir := setupTestGitRepo(t, map[string]string{
            "main.go":      "package main",
            "lib/util.go":  "package lib",
            "README.md":    "# Test",
        })
        defer os.RemoveAll(tmpDir)
        
        config := GitFileCollectorConfig{
            DefaultMaxFileSize: 100000,
        }
        
        input := GitFileCollectorInput{
            ContextDir: tmpDir,
        }
        
        output, err := activity.Execute(context.Background(), config, input)
        require.NoError(t, err)
        assert.Equal(t, 3, output.FileCount)
        assert.NotEmpty(t, output.Repository.CommitHash)
    })
    
    t.Run("FilePatterns", func(t *testing.T) {
        tmpDir := setupTestGitRepo(t, map[string]string{
            "main.go":      "package main",
            "main_test.go": "package main",
            "lib/util.go":  "package lib",
            "README.md":    "# Test",
        })
        defer os.RemoveAll(tmpDir)
        
        input := GitFileCollectorInput{
            ContextDir: tmpDir,
            FilePatterns: []string{"*.go"},
            ExcludePatterns: []string{"*_test.go"},
        }
        
        output, err := activity.Execute(context.Background(), GitFileCollectorConfig{}, input)
        require.NoError(t, err)
        assert.Equal(t, 2, output.FileCount) // main.go and lib/util.go
    })
}
```

### Integration Tests
```go
func TestGitFileCollectorIntegration(t *testing.T) {
    t.Run("WithStagedFiles", func(t *testing.T) {
        tmpDir := setupTestGitRepo(t, map[string]string{
            "committed.go": "package main",
        })
        defer os.RemoveAll(tmpDir)
        
        // Add a staged file
        stagePath := filepath.Join(tmpDir, "staged.go")
        os.WriteFile(stagePath, []byte("package staged"), 0644)
        exec.Command("git", "-C", tmpDir, "add", "staged.go").Run()
        
        input := GitFileCollectorInput{
            ContextDir:    tmpDir,
            IncludeStaged: true,
        }
        
        output, err := NewGitFileCollectorActivity().Execute(
            context.Background(),
            GitFileCollectorConfig{},
            input,
        )
        
        require.NoError(t, err)
        assert.Equal(t, 2, output.FileCount)
        assert.True(t, output.Repository.IsDirty)
    })
}
```

## Metrics

```go
// server/git/pkg/metrics/git_metrics.go

var (
    GitFilesCollected = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "git_files_collected",
            Help:    "Number of files collected from git repositories",
            Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
        },
        []string{"repository"},
    )
    
    GitCollectionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "git_collection_duration_seconds",
            Help:    "Duration of git file collection operations",
            Buckets: prometheus.DefBuckets,
        },
        []string{"status"},
    )
    
    GitCollectionSize = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "git_collection_size_bytes",
            Help:    "Total size of collected files in bytes",
            Buckets: []float64{1024, 10240, 102400, 1048576, 10485760, 104857600},
        },
    )
)
```

## Security Considerations

1. **Path Traversal Prevention**: All paths are validated to be within the context directory using common.ValidateRepository
2. **Git Command Injection**: Use common.ExecuteGitCommand which safely executes git commands
3. **Resource Limits**: Enforce file size and total size limits
4. **Binary File Exclusion**: Automatically detect and exclude binary files to prevent processing non-text content
5. **Sensitive File Detection**: Option to exclude files matching sensitive patterns
6. **Repository Access**: Respects git permissions and SSH keys

## Future Enhancements

1. **Incremental Collection**: Only collect files changed since last collection
2. **Branch Support**: Collect files from specific branches without switching
3. **Diff Collection**: Include git diff information in metadata using gitRepo.GetDiff
4. **Submodule Support**: Optionally include submodule files
5. **Performance Optimization**: Parallel file reading for large repositories
6. **Caching**: Cache collected files with invalidation on git changes
7. **Pre-commit Integration**: Apply pre-commit hooks to filter files
8. **Language Detection**: Auto-detect programming languages for better filtering