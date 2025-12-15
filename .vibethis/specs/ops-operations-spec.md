# Recipe Operations Specification

## Overview
This specification defines new self-registering operations for the recipe system (`server/ops/`) that leverage the enhanced LLM adapter capabilities. These operations follow the existing `RegisterableActivity` pattern and are automatically discoverable by the recipe-worker without requiring changes to the cortex schema.

## Design Principles
1. **Self-Registration**: Operations implement `RegisterableActivity` interface
2. **Auto-Discovery**: Recipe-worker automatically finds and registers operations
3. **No Schema Changes**: Operations work with existing YAML `op:` field
4. **Type Safety**: Strongly typed inputs/outputs with validation

## 1. Git File Collector Operation

### Operation Usage in YAML
```yaml
- id: collect_files
  op: git_file_collector
  inputs:
    context_dir: "/project/src"
    file_patterns:
      - "**/*.go"
      - "go.mod"
    exclude_patterns:
      - "**/*_test.go"
    include_staged: true
```

### Activity Implementation
```go
// server/ops/pkg/git/git_file_collector.go

package git

import (
    "context"
    "os/exec"
    "path/filepath"
    "github.com/colony2/server/ops/pkg/types"
    "github.com/colony2/server/llm/adapters"
)

// GitFileCollectorConfig provides configuration for the activity
type GitFileCollectorConfig struct {
    DefaultMaxFileSize  int
    DefaultMaxTotalSize int
}

// GitFileCollectorInput defines the input for git file collection
type GitFileCollectorInput struct {
    ContextDir       string   `json:"context_dir"`
    FilePatterns     []string `json:"file_patterns,omitempty"`
    ExcludePatterns  []string `json:"exclude_patterns,omitempty"`
    MaxFileSize      int      `json:"max_file_size,omitempty"`
    MaxTotalSize     int      `json:"max_total_size,omitempty"`
    IncludeStaged    bool     `json:"include_staged,omitempty"`
    IncludeUntracked bool     `json:"include_untracked,omitempty"`
    UseGitignore     bool     `json:"use_gitignore,omitempty"`
    AutoDetectType   bool     `json:"auto_detect_type,omitempty"`
}

// GitFileCollectorOutput defines the output structure
type GitFileCollectorOutput struct {
    Files      []adapters.File        `json:"files"`
    FileCount  int                    `json:"file_count"`
    TotalSize  int64                  `json:"total_size"`
    Repository GitRepositoryInfo      `json:"repository"`
}

type GitRepositoryInfo struct {
    Branch        string `json:"branch"`
    CommitHash    string `json:"commit_hash"`
    CommitMessage string `json:"commit_message"`
    IsDirty       bool   `json:"is_dirty"`
    RemoteURL     string `json:"remote_url,omitempty"`
}

// GitFileCollectorActivity implements RegisterableActivity
type GitFileCollectorActivity struct {
    logger Logger
}

// Ensure we implement the interface
var _ types.RegisterableActivity[GitFileCollectorConfig, GitFileCollectorInput, GitFileCollectorOutput] = (*GitFileCollectorActivity)(nil)

// NewGitFileCollectorActivity creates a new activity instance
func NewGitFileCollectorActivity() types.RegisterableActivity[GitFileCollectorConfig, GitFileCollectorInput, GitFileCollectorOutput] {
    return &GitFileCollectorActivity{}
}

// GetMetadata returns activity metadata for registration
func (a *GitFileCollectorActivity) GetMetadata() types.ActivityMetadata {
    return types.ActivityMetadata{
        Type:           "git_file_collector",
        Name:           "Git File Collector",
        Description:    "Collects files from a git repository with filtering",
        Version:        "1.0.0",
        DefaultTimeout: 30 * time.Second,
        RetryPolicy: &types.RetryPolicy{
            MaximumAttempts:    3,
            InitialInterval:    1 * time.Second,
            BackoffCoefficient: 2.0,
        },
    }
}

// Execute runs the git file collection
func (a *GitFileCollectorActivity) Execute(
    ctx context.Context,
    config GitFileCollectorConfig,
    input GitFileCollectorInput,
) (GitFileCollectorOutput, error) {
    // Apply defaults
    if input.MaxFileSize == 0 {
        input.MaxFileSize = config.DefaultMaxFileSize
    }
    if input.MaxTotalSize == 0 {
        input.MaxTotalSize = config.DefaultMaxTotalSize
    }
    
    // Validate git repository
    if err := a.validateGitRepo(input.ContextDir); err != nil {
        return GitFileCollectorOutput{}, fmt.Errorf("not a git repository: %w", err)
    }
    
    // Get repository info
    repoInfo, err := a.getRepositoryInfo(ctx, input.ContextDir)
    if err != nil {
        return GitFileCollectorOutput{}, fmt.Errorf("failed to get repository info: %w", err)
    }
    
    // List files using git
    filePaths, err := a.listGitFiles(ctx, input)
    if err != nil {
        return GitFileCollectorOutput{}, fmt.Errorf("failed to list files: %w", err)
    }
    
    // Collect file contents
    files, totalSize, err := a.collectFiles(ctx, input.ContextDir, filePaths, input)
    if err != nil {
        return GitFileCollectorOutput{}, fmt.Errorf("failed to collect files: %w", err)
    }
    
    return GitFileCollectorOutput{
        Files:      files,
        FileCount:  len(files),
        TotalSize:  totalSize,
        Repository: repoInfo,
    }, nil
}

// Private helper methods...
func (a *GitFileCollectorActivity) validateGitRepo(dir string) error {
    gitDir := filepath.Join(dir, ".git")
    _, err := os.Stat(gitDir)
    return err
}

func (a *GitFileCollectorActivity) listGitFiles(ctx context.Context, input GitFileCollectorInput) ([]string, error) {
    // Implementation using git ls-files, git diff --staged, etc.
    // ...
}
```

## 2. Enhanced LLM Inference Operation

### Operation Usage in YAML
```yaml
- id: generate_code
  op: llm_inference
  inputs:
    provider: "OpenAI"
    model: "gpt-4"
    prompt: "Implement the feature"
    files: "{{ .collect_files.outputs.files }}"
    tools:
      - name: write_file
        description: "Write a file"
        parameters:
          type: object
          properties:
            path: { type: string }
            content: { type: string }
          required: ["path", "content"]
    execute_tools: true
    tool_working_dir: "/project"
```

### Enhanced Activity Implementation
```go
// server/ops/pkg/llm/llm_inference_enhanced.go

package llm

import (
    "context"
    "github.com/colony2/server/ops/pkg/types"
    "github.com/colony2/server/llm/adapters"
)

// LLMInferenceConfig provides configuration
type LLMInferenceConfig struct {
    DefaultProvider string
    DefaultModel    string
    APIKeys         map[string]string
}

// LLMInferenceInput defines enhanced input
type LLMInferenceInput struct {
    // Standard fields
    Prompt         string                 `json:"prompt,omitempty"`
    SystemPrompt   string                 `json:"system_prompt,omitempty"`
    Temperature    float64                `json:"temperature,omitempty"`
    MaxTokens      int                    `json:"max_tokens,omitempty"`
    TopP           float64                `json:"top_p,omitempty"`
    StopSequences  []string               `json:"stop_sequences,omitempty"`
    ResponseSchema map[string]interface{} `json:"response_schema,omitempty"`
    Provider       string                 `json:"provider,omitempty"`
    Model          string                 `json:"model,omitempty"`
    
    // Enhanced fields
    Files          []adapters.File        `json:"files,omitempty"`
    FileHandling   string                 `json:"file_handling,omitempty"`
    Tools          []ToolDefinition       `json:"tools,omitempty"`
    ExecuteTools   bool                   `json:"execute_tools,omitempty"`
    ToolWorkingDir string                 `json:"tool_working_dir,omitempty"`
    MaxToolRounds  int                    `json:"max_tool_rounds,omitempty"`
}

type ToolDefinition struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
}

// LLMInferenceOutput defines enhanced output
type LLMInferenceOutput struct {
    Response            interface{}      `json:"response"`
    Model               string           `json:"model"`
    FinishReason        string           `json:"finish_reason"`
    Usage               map[string]interface{} `json:"usage"`
    ToolCalls           []ToolCallInfo   `json:"tool_calls,omitempty"`
    ToolResults         []ToolResult     `json:"tool_results,omitempty"`
    ToolExecutionErrors []string         `json:"tool_execution_errors,omitempty"`
    FilesWritten        []string         `json:"files_written,omitempty"`
    FilesDeleted        []string         `json:"files_deleted,omitempty"`
}

// EnhancedLLMInferenceActivity implements RegisterableActivity
type EnhancedLLMInferenceActivity struct {
    registry adapters.Registry
    logger   Logger
}

var _ types.RegisterableActivity[LLMInferenceConfig, LLMInferenceInput, LLMInferenceOutput] = (*EnhancedLLMInferenceActivity)(nil)

func NewEnhancedLLMInferenceActivity() types.RegisterableActivity[LLMInferenceConfig, LLMInferenceInput, LLMInferenceOutput] {
    return &EnhancedLLMInferenceActivity{}
}

func (a *EnhancedLLMInferenceActivity) GetMetadata() types.ActivityMetadata {
    return types.ActivityMetadata{
        Type:           "llm_inference",  // Replaces/enhances existing
        Name:           "LLM Inference",
        Description:    "Enhanced LLM inference with file and tool support",
        Version:        "2.0.0",
        DefaultTimeout: 5 * time.Minute,
    }
}

func (a *EnhancedLLMInferenceActivity) Execute(
    ctx context.Context,
    config LLMInferenceConfig,
    input LLMInferenceInput,
) (LLMInferenceOutput, error) {
    // Apply defaults
    if input.Provider == "" {
        input.Provider = config.DefaultProvider
    }
    if input.Model == "" {
        input.Model = config.DefaultModel
    }
    
    // Get adapter from registry
    adapter, err := a.registry.GetAdapter(input.Provider)
    if err != nil {
        return LLMInferenceOutput{}, err
    }
    
    // Configure tool executor if needed
    if input.ExecuteTools && len(input.Tools) > 0 {
        executor := adapters.NewFileToolExecutor(
            input.ToolWorkingDir,
            adapters.WithSandbox(true),
        )
        
        if execAdapter, ok := adapter.(adapters.ExecutableToolAdapter); ok {
            execAdapter.SetToolExecutor(executor)
        }
    }
    
    // Execute based on capabilities
    if len(input.Files) > 0 && len(input.Tools) > 0 {
        return a.executeWithFilesAndTools(ctx, adapter, input)
    } else if len(input.Files) > 0 {
        return a.executeWithFiles(ctx, adapter, input)
    } else if len(input.Tools) > 0 {
        return a.executeWithTools(ctx, adapter, input)
    } else {
        return a.executeBasic(ctx, adapter, input)
    }
}
```

## 3. Activity Registration

### Automatic Registration
```go
// server/ops/pkg/registry/auto_register.go

package registry

import (
    "github.com/colony2/server/ops/pkg/types"
    "github.com/colony2/server/ops/pkg/git"
    "github.com/colony2/server/ops/pkg/llm"
    "github.com/colony2/server/ops/pkg/command"
)

// GetAllActivities returns all available activities for registration
func GetAllActivities() []types.RegisterableActivity[any, any, any] {
    return []types.RegisterableActivity[any, any, any]{
        // Existing activities
        command.NewCommandExecutionActivity(),
        
        // New activities
        git.NewGitFileCollectorActivity(),
        llm.NewEnhancedLLMInferenceActivity(),
    }
}

// RegisterAll registers all activities with a worker
func RegisterAll(worker Worker) {
    for _, activity := range GetAllActivities() {
        metadata := activity.GetMetadata()
        worker.RegisterActivity(metadata.Type, activity.Execute)
    }
}
```

### Recipe Worker Integration
```go
// server/recipe-worker/pkg/worker/worker.go

package worker

import (
    "github.com/colony2/server/ops/pkg/registry"
)

func (w *Worker) Start() error {
    // Register all ops activities
    registry.RegisterAll(w.temporalWorker)
    
    // Start worker
    return w.temporalWorker.Start()
}
```

## 4. Built-in Tool Definitions

### Tool Helper Functions
```go
// server/ops/pkg/tools/file_tools.go

package tools

// GetFileOperationTools returns standard file operation tool definitions
func GetFileOperationTools() []ToolDefinition {
    return []ToolDefinition{
        {
            Name:        "write_file",
            Description: "Write or update a file",
            Parameters: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "path":        map[string]interface{}{"type": "string"},
                    "content":     map[string]interface{}{"type": "string"},
                    "create_dirs": map[string]interface{}{"type": "boolean", "default": true},
                },
                "required": []string{"path", "content"},
            },
        },
        {
            Name:        "read_file",
            Description: "Read a file's content",
            Parameters: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "path": map[string]interface{}{"type": "string"},
                },
                "required": []string{"path"},
            },
        },
        {
            Name:        "delete_file",
            Description: "Delete a file",
            Parameters: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "path": map[string]interface{}{"type": "string"},
                },
                "required": []string{"path"},
            },
        },
    }
}
```

## 5. Configuration

### Activity Configuration
```yaml
# server/ops/config/activities.yaml

activities:
  git_file_collector:
    default_max_file_size: 100000
    default_max_total_size: 10485760
    
  llm_inference:
    default_provider: OpenAI
    default_model: gpt-4
    api_keys:
      openai: ${OPENAI_API_KEY}
      anthropic: ${ANTHROPIC_API_KEY}
      gemini: ${GEMINI_API_KEY}
```

### Loading Configuration
```go
// server/ops/pkg/config/loader.go

package config

type ActivityConfig struct {
    GitFileCollector git.GitFileCollectorConfig `yaml:"git_file_collector"`
    LLMInference     llm.LLMInferenceConfig     `yaml:"llm_inference"`
}

func LoadConfig(path string) (*ActivityConfig, error) {
    // Load from YAML file
    // ...
}
```

## 6. Validation

### Input Validation
```go
// server/ops/pkg/validation/validators.go

package validation

func ValidateGitFileCollectorInput(input git.GitFileCollectorInput) error {
    if input.ContextDir == "" {
        return fmt.Errorf("context_dir is required")
    }
    
    // Check if directory exists
    if _, err := os.Stat(input.ContextDir); err != nil {
        return fmt.Errorf("context directory not found: %w", err)
    }
    
    // Validate patterns
    for _, pattern := range input.FilePatterns {
        if _, err := filepath.Match(pattern, "test"); err != nil {
            return fmt.Errorf("invalid file pattern %s: %w", pattern, err)
        }
    }
    
    return nil
}

func ValidateLLMInferenceInput(input llm.LLMInferenceInput) error {
    if input.Provider == "" {
        return fmt.Errorf("provider is required")
    }
    
    if input.ExecuteTools && input.ToolWorkingDir == "" {
        return fmt.Errorf("tool_working_dir required when execute_tools is true")
    }
    
    return nil
}
```

## 7. Testing

### Unit Tests
```go
// server/ops/pkg/git/git_file_collector_test.go

func TestGitFileCollectorActivity(t *testing.T) {
    activity := NewGitFileCollectorActivity()
    
    t.Run("GetMetadata", func(t *testing.T) {
        metadata := activity.GetMetadata()
        assert.Equal(t, "git_file_collector", metadata.Type)
        assert.Equal(t, "1.0.0", metadata.Version)
    })
    
    t.Run("Execute", func(t *testing.T) {
        // Setup test git repo
        tmpDir := setupTestRepo(t)
        defer os.RemoveAll(tmpDir)
        
        config := GitFileCollectorConfig{
            DefaultMaxFileSize: 100000,
        }
        
        input := GitFileCollectorInput{
            ContextDir: tmpDir,
            FilePatterns: []string{"*.go"},
        }
        
        output, err := activity.Execute(context.Background(), config, input)
        require.NoError(t, err)
        assert.Greater(t, output.FileCount, 0)
    })
}
```

### Integration Tests
```go
// server/ops/integration/recipe_test.go

func TestRecipeWithNewOperations(t *testing.T) {
    // Test YAML recipe using new operations
    recipe := `
name: test_recipe
version: "1.0"

sequence:
  - id: collect
    op: git_file_collector
    inputs:
      context_dir: "."
      
  - id: generate
    op: llm_inference
    inputs:
      prompt: "Analyze these files"
      files: "{{ .collect.outputs.files }}"
`
    
    // Execute recipe and verify results
    // ...
}
```

## 8. Monitoring

### Metrics
```go
// server/ops/pkg/metrics/activity_metrics.go

package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    ActivityDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "ops_activity_duration_seconds",
            Help: "Duration of activity execution",
        },
        []string{"activity_type", "status"},
    )
    
    GitFilesCollected = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "git_files_collected",
            Help: "Number of files collected from git",
            Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500},
        },
    )
    
    LLMToolExecutions = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "llm_tool_executions_total",
            Help: "Total number of LLM tool executions",
        },
        []string{"provider", "tool", "status"},
    )
)
```

## 9. Error Handling

### Error Types
```go
// server/ops/pkg/errors/errors.go

package errors

type ActivityError struct {
    ActivityType string
    ErrorType    string
    Message      string
    Details      map[string]interface{}
    Retryable    bool
}

func NewGitError(message string, details map[string]interface{}) *ActivityError {
    return &ActivityError{
        ActivityType: "git_file_collector",
        ErrorType:    "git_operation",
        Message:      message,
        Details:      details,
        Retryable:    false,
    }
}

func NewLLMError(message string, provider string, retryable bool) *ActivityError {
    return &ActivityError{
        ActivityType: "llm_inference",
        ErrorType:    "llm_provider",
        Message:      message,
        Details:      map[string]interface{}{"provider": provider},
        Retryable:    retryable,
    }
}
```

## 10. Security

### Sandboxing
```go
// server/ops/pkg/security/sandbox.go

package security

type ActivitySandbox struct {
    AllowedPaths    []string
    RestrictedPaths []string
    MaxMemory       int64
    MaxExecutionTime time.Duration
}

func (s *ActivitySandbox) ValidatePath(path string) error {
    absPath, err := filepath.Abs(path)
    if err != nil {
        return err
    }
    
    // Check allowed paths
    for _, allowed := range s.AllowedPaths {
        if strings.HasPrefix(absPath, allowed) {
            // Check not in restricted
            for _, restricted := range s.RestrictedPaths {
                if strings.HasPrefix(absPath, restricted) {
                    return fmt.Errorf("path is restricted: %s", path)
                }
            }
            return nil
        }
    }
    
    return fmt.Errorf("path not allowed: %s", path)
}
```

## Summary

This specification defines self-registering operations that:
1. **Follow existing patterns**: Implement `RegisterableActivity` interface
2. **Auto-register**: Recipe-worker discovers them automatically
3. **Work with existing YAML**: Use standard `op:` field
4. **Provide enhanced capabilities**: Git integration and advanced LLM features
5. **Maintain compatibility**: Can replace/enhance existing operations

The operations are ready to be used in recipes without any changes to the cortex schema or recipe-worker discovery mechanism.