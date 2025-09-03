# Enhanced LLM Inference Operation Specification

## Overview
This specification defines enhancements to the existing LLM inference operation to support file inputs and tool execution. The enhanced operation maintains backward compatibility while adding powerful new capabilities for autonomous task execution.

## Purpose
Enhance the LLM inference operation to:
- Accept files as first-class input (leveraging FileAdapter)
- Support tool/function calling with automatic execution
- Enable autonomous file operations
- Maintain backward compatibility with existing recipes
- Provide sandboxed execution environment

## Operation Details

### Operation Type
```yaml
op: llm_inference  # Same as existing, with enhanced capabilities
```

### Usage in Recipes

#### Basic Usage (Backward Compatible)
```yaml
- id: generate_text
  op: llm_inference
  inputs:
    provider: "OpenAI"
    model: "gpt-4"
    prompt: "Write a hello world program"
    temperature: 0.7
    max_tokens: 500
```

#### With File Context
```yaml
- id: analyze_code
  op: llm_inference
  inputs:
    provider: "OpenAI"
    model: "gpt-4"
    prompt: "Analyze this codebase and suggest improvements"
    files: "{{ .collect_files.outputs.files }}"  # From git_file_collector
    file_handling: "native"  # Use provider's native file handling
```

#### With Tool Execution
```yaml
- id: generate_with_tools
  op: llm_inference
  inputs:
    provider: "OpenAI"
    model: "gpt-4"
    prompt: "Create a REST API endpoint for user management"
    files: "{{ .collect_files.outputs.files }}"
    tools:
      - name: write_file
        description: "Write or update a file"
        parameters:
          type: object
          properties:
            path: { type: string, description: "File path relative to working directory" }
            content: { type: string, description: "File content" }
          required: ["path", "content"]
      
      - name: read_file
        description: "Read a file's content"
        parameters:
          type: object
          properties:
            path: { type: string, description: "File path to read" }
          required: ["path"]
    
    execute_tools: true
    tool_working_dir: "/project/src"
    max_tool_rounds: 3
```

## Implementation

### Enhanced Activity Structure
```go
// server/ops/pkg/llm/llm_inference_enhanced.go

package llm

import (
    "context"
    "encoding/json"
    "github.com/vibethis/server/ops/pkg/types"
    "github.com/vibethis/server/llm/adapters"
)

// LLMInferenceConfig provides configuration
type LLMInferenceConfig struct {
    // Provider configurations
    DefaultProvider string            `yaml:"default_provider"`
    DefaultModel    string            `yaml:"default_model"`
    APIKeys         map[string]string `yaml:"api_keys"`
    
    // Tool execution settings
    EnableToolExecution bool   `yaml:"enable_tool_execution"`
    DefaultWorkingDir   string `yaml:"default_working_dir"`
    MaxToolRounds       int    `yaml:"max_tool_rounds"`
    
    // File handling settings
    DefaultFileHandling string `yaml:"default_file_handling"`
    MaxFileContextSize  int    `yaml:"max_file_context_size"`
    
    // Safety settings
    EnableSandbox    bool     `yaml:"enable_sandbox"`
    AllowedPaths     []string `yaml:"allowed_paths"`
    RestrictedPaths  []string `yaml:"restricted_paths"`
}

// LLMInferenceInput defines enhanced input (backward compatible)
type LLMInferenceInput struct {
    // Standard fields (existing)
    Prompt         string                 `json:"prompt,omitempty"`
    SystemPrompt   string                 `json:"system_prompt,omitempty"`
    Temperature    float64                `json:"temperature,omitempty"`
    MaxTokens      int                    `json:"max_tokens,omitempty"`
    TopP           float64                `json:"top_p,omitempty"`
    StopSequences  []string               `json:"stop_sequences,omitempty"`
    ResponseSchema map[string]interface{} `json:"response_schema,omitempty"`
    Provider       string                 `json:"provider,omitempty"`
    Model          string                 `json:"model,omitempty"`
    
    // Enhanced fields (new)
    Files          []adapters.File  `json:"files,omitempty"`
    FileHandling   string           `json:"file_handling,omitempty"` // native, text_fallback, hybrid
    Tools          []ToolDefinition `json:"tools,omitempty"`
    ExecuteTools   bool             `json:"execute_tools,omitempty"`
    ToolWorkingDir string           `json:"tool_working_dir,omitempty"`
    MaxToolRounds  int              `json:"max_tool_rounds,omitempty"`
    
    // Advanced options
    ContinueOnToolError bool                   `json:"continue_on_tool_error,omitempty"`
    ToolTimeout         string                 `json:"tool_timeout,omitempty"`
    Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

// ToolDefinition defines a tool for LLM
type ToolDefinition struct {
    Name        string                 `json:"name" validate:"required"`
    Description string                 `json:"description" validate:"required"`
    Parameters  map[string]interface{} `json:"parameters" validate:"required"`
}

// LLMInferenceOutput defines enhanced output (backward compatible)
type LLMInferenceOutput struct {
    // Standard fields (existing)
    Response     interface{}            `json:"response"`
    Model        string                 `json:"model"`
    FinishReason string                 `json:"finish_reason"`
    Usage        map[string]interface{} `json:"usage"`
    
    // Tool execution fields (new)
    ToolCalls           []ToolCallInfo       `json:"tool_calls,omitempty"`
    ToolResults         []ToolResult         `json:"tool_results,omitempty"`
    ToolExecutionErrors []string             `json:"tool_execution_errors,omitempty"`
    ToolRoundsUsed      int                  `json:"tool_rounds_used,omitempty"`
    
    // File operation tracking (new)
    FilesWritten []string               `json:"files_written,omitempty"`
    FilesRead    []string               `json:"files_read,omitempty"`
    FilesDeleted []string               `json:"files_deleted,omitempty"`
    
    // Enhanced metadata
    ExecutionTime   int64                  `json:"execution_time_ms,omitempty"`
    ProviderMetadata map[string]interface{} `json:"provider_metadata,omitempty"`
}

type ToolCallInfo struct {
    ID        string                 `json:"id"`
    Name      string                 `json:"name"`
    Arguments map[string]interface{} `json:"arguments"`
    Timestamp int64                  `json:"timestamp"`
}

type ToolResult struct {
    ToolCallID string      `json:"tool_call_id"`
    ToolName   string      `json:"tool_name"`
    Success    bool        `json:"success"`
    Result     interface{} `json:"result,omitempty"`
    Error      string      `json:"error,omitempty"`
    Duration   int64       `json:"duration_ms"`
}
```

### Enhanced Activity Implementation
```go
// EnhancedLLMInferenceActivity implements RegisterableActivity
type EnhancedLLMInferenceActivity struct {
    registry     adapters.Registry
    toolExecutor ToolExecutor
    sandbox      SecuritySandbox
    logger       Logger
}

var _ types.RegisterableActivity[LLMInferenceConfig, LLMInferenceInput, LLMInferenceOutput] = (*EnhancedLLMInferenceActivity)(nil)

func NewEnhancedLLMInferenceActivity() types.RegisterableActivity[LLMInferenceConfig, LLMInferenceInput, LLMInferenceOutput] {
    return &EnhancedLLMInferenceActivity{
        registry: adapters.NewRegistry(),
    }
}

func (a *EnhancedLLMInferenceActivity) GetMetadata() types.ActivityMetadata {
    return types.ActivityMetadata{
        Type:           "llm_inference",  // Same type, enhanced implementation
        Name:           "LLM Inference",
        Description:    "Enhanced LLM inference with file and tool support",
        Version:        "2.0.0",
        DefaultTimeout: 5 * time.Minute,
        RetryPolicy: &types.RetryPolicy{
            MaximumAttempts:        3,
            InitialInterval:        2 * time.Second,
            BackoffCoefficient:     2.0,
            MaximumInterval:        30 * time.Second,
            NonRetryableErrorTypes: []string{"InvalidAPIKey", "QuotaExceeded"},
        },
    }
}

func (a *EnhancedLLMInferenceActivity) Execute(
    ctx context.Context,
    config LLMInferenceConfig,
    input LLMInferenceInput,
) (LLMInferenceOutput, error) {
    startTime := time.Now()
    
    // Apply defaults
    a.applyDefaults(&input, config)
    
    // Validate input
    if err := a.validateInput(input, config); err != nil {
        return LLMInferenceOutput{}, fmt.Errorf("validation failed: %w", err)
    }
    
    // Get adapter from registry
    adapter, err := a.registry.GetAdapter(input.Provider)
    if err != nil {
        return LLMInferenceOutput{}, fmt.Errorf("failed to get adapter: %w", err)
    }
    
    // Configure adapter with API key
    if apiKey, ok := config.APIKeys[input.Provider]; ok {
        adapter.SetAPIKey(apiKey)
    }
    
    // Setup tool executor if needed
    if input.ExecuteTools && len(input.Tools) > 0 {
        a.setupToolExecutor(input, config)
    }
    
    // Execute based on capabilities
    var output LLMInferenceOutput
    
    if len(input.Files) > 0 && len(input.Tools) > 0 {
        output, err = a.executeWithFilesAndTools(ctx, adapter, input)
    } else if len(input.Files) > 0 {
        output, err = a.executeWithFiles(ctx, adapter, input)
    } else if len(input.Tools) > 0 {
        output, err = a.executeWithTools(ctx, adapter, input)
    } else {
        output, err = a.executeBasic(ctx, adapter, input)
    }
    
    if err != nil {
        return output, err
    }
    
    output.ExecutionTime = time.Since(startTime).Milliseconds()
    return output, nil
}
```

### Execution Methods
```go
func (a *EnhancedLLMInferenceActivity) executeWithFilesAndTools(
    ctx context.Context,
    adapter adapters.Adapter,
    input LLMInferenceInput,
) (LLMInferenceOutput, error) {
    // Check if adapter supports unified operations
    unifiedAdapter, ok := adapter.(adapters.UnifiedAdapter)
    if !ok {
        // Fall back to sequential execution
        return a.executeSequential(ctx, adapter, input)
    }
    
    // Convert tools to adapter format
    tools := a.convertTools(input.Tools)
    
    // Build unified config
    unifiedConfig := adapters.UnifiedConfig{
        ExecutableToolConfig: adapters.ExecutableToolConfig{
            Config: adapters.Config{
                Model:          input.Model,
                Temperature:    input.Temperature,
                MaxTokens:      input.MaxTokens,
                TopP:           input.TopP,
                SystemPrompt:   input.SystemPrompt,
                ResponseSchema: input.ResponseSchema,
            },
            AutoExecute:      input.ExecuteTools,
            MaxToolRounds:    input.MaxToolRounds,
            WorkingDirectory: input.ToolWorkingDir,
        },
        FileHandling: adapters.FileHandlingMode(input.FileHandling),
    }
    
    // Execute with retry logic for tool rounds
    var response adapters.UnifiedResponse
    var toolRounds int
    
    for toolRounds < input.MaxToolRounds {
        response, err := unifiedAdapter.GenerateWithFilesAndTools(
            ctx,
            input.Prompt,
            input.Files,
            tools,
            unifiedConfig,
        )
        
        if err != nil {
            return LLMInferenceOutput{}, fmt.Errorf("generation failed: %w", err)
        }
        
        // If no tool calls or tool execution disabled, we're done
        if len(response.ToolCalls) == 0 || !input.ExecuteTools {
            break
        }
        
        // Execute tools
        toolResults, err := a.executeToolCalls(ctx, response.ToolCalls, input)
        if err != nil && !input.ContinueOnToolError {
            return a.convertResponse(response, toolResults, toolRounds), err
        }
        
        // Continue conversation with tool results if needed
        if response.FinishReason == "tool_calls" {
            // Update prompt with tool results for next round
            input.Prompt = a.formatToolResults(toolResults)
            toolRounds++
        } else {
            break
        }
    }
    
    return a.convertResponse(response, nil, toolRounds), nil
}

func (a *EnhancedLLMInferenceActivity) executeToolCalls(
    ctx context.Context,
    toolCalls []adapters.ToolCall,
    input LLMInferenceInput,
) ([]ToolResult, error) {
    var results []ToolResult
    
    for _, call := range toolCalls {
        startTime := time.Now()
        
        // Validate tool call
        tool := a.findTool(call.Name, input.Tools)
        if tool == nil {
            results = append(results, ToolResult{
                ToolCallID: call.ID,
                ToolName:   call.Name,
                Success:    false,
                Error:      "unknown tool",
            })
            continue
        }
        
        // Validate arguments against schema
        if err := a.validateToolArguments(call.Arguments, tool.Parameters); err != nil {
            results = append(results, ToolResult{
                ToolCallID: call.ID,
                ToolName:   call.Name,
                Success:    false,
                Error:      fmt.Sprintf("invalid arguments: %v", err),
            })
            continue
        }
        
        // Execute tool in sandbox
        result, err := a.toolExecutor.Execute(ctx, ToolExecutionRequest{
            Name:       call.Name,
            Arguments:  call.Arguments,
            WorkingDir: input.ToolWorkingDir,
            Timeout:    a.parseTimeout(input.ToolTimeout),
            Sandbox:    a.sandbox,
        })
        
        results = append(results, ToolResult{
            ToolCallID: call.ID,
            ToolName:   call.Name,
            Success:    err == nil,
            Result:     result,
            Error:      a.errorString(err),
            Duration:   time.Since(startTime).Milliseconds(),
        })
    }
    
    return results, nil
}
```

## Tool Executor Implementation

### File Tool Executor
```go
// server/ops/pkg/llm/tool_executor.go

package llm

type FileToolExecutor struct {
    workingDir string
    sandbox    SecuritySandbox
    logger     Logger
}

func NewFileToolExecutor(workingDir string, sandbox SecuritySandbox) *FileToolExecutor {
    return &FileToolExecutor{
        workingDir: workingDir,
        sandbox:    sandbox,
    }
}

func (e *FileToolExecutor) Execute(
    ctx context.Context,
    request ToolExecutionRequest,
) (interface{}, error) {
    switch request.Name {
    case "write_file":
        return e.executeWriteFile(ctx, request)
    case "read_file":
        return e.executeReadFile(ctx, request)
    case "delete_file":
        return e.executeDeleteFile(ctx, request)
    case "list_files":
        return e.executeListFiles(ctx, request)
    default:
        return nil, fmt.Errorf("unknown tool: %s", request.Name)
    }
}

func (e *FileToolExecutor) executeWriteFile(
    ctx context.Context,
    request ToolExecutionRequest,
) (interface{}, error) {
    // Extract arguments
    path, _ := request.Arguments["path"].(string)
    content, _ := request.Arguments["content"].(string)
    createDirs, _ := request.Arguments["create_dirs"].(bool)
    
    // Build full path
    fullPath := filepath.Join(e.workingDir, path)
    
    // Validate path in sandbox
    if err := e.sandbox.ValidatePath(fullPath); err != nil {
        return nil, fmt.Errorf("path validation failed: %w", err)
    }
    
    // Create directories if needed
    if createDirs {
        dir := filepath.Dir(fullPath)
        if err := os.MkdirAll(dir, 0755); err != nil {
            return nil, fmt.Errorf("failed to create directories: %w", err)
        }
    }
    
    // Write file
    if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
        return nil, fmt.Errorf("failed to write file: %w", err)
    }
    
    return map[string]interface{}{
        "path":  path,
        "bytes": len(content),
    }, nil
}
```

## Registration

### Auto-Registration
```go
// server/ops/pkg/registry/llm_activities.go

package registry

import (
    "github.com/vibethis/server/ops/pkg/llm"
)

// RegisterLLMActivities registers all LLM-related activities
func RegisterLLMActivities() []RegisterableActivity {
    return []RegisterableActivity{
        llm.NewEnhancedLLMInferenceActivity(),
        // Future LLM operations can be added here
    }
}
```

### Backward Compatibility
The enhanced activity automatically handles both old and new input formats:
- If no `files` or `tools` are provided, it behaves exactly like the original
- The `Type` in metadata remains "llm_inference" for compatibility
- All existing recipes continue to work without modification

## Configuration

### Default Configuration
```yaml
# server/ops/config/activities.yaml

llm_inference:
  # Provider settings
  default_provider: OpenAI
  default_model: gpt-4
  api_keys:
    openai: ${OPENAI_API_KEY}
    anthropic: ${ANTHROPIC_API_KEY}
    gemini: ${GEMINI_API_KEY}
  
  # Tool execution settings
  enable_tool_execution: true
  default_working_dir: "."
  max_tool_rounds: 5
  
  # File handling settings
  default_file_handling: native
  max_file_context_size: 10485760  # 10MB
  
  # Safety settings
  enable_sandbox: true
  allowed_paths:
    - /tmp
    - /workspace
  restricted_paths:
    - /etc
    - /usr
    - /bin
    - /sbin
```

## Built-in Tools

### Standard File Operations
```go
// server/ops/pkg/llm/builtin_tools.go

func GetBuiltinTools() []ToolDefinition {
    return []ToolDefinition{
        {
            Name:        "write_file",
            Description: "Write or update a file",
            Parameters: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "path": map[string]interface{}{
                        "type":        "string",
                        "description": "File path relative to working directory",
                    },
                    "content": map[string]interface{}{
                        "type":        "string",
                        "description": "Complete file content",
                    },
                    "create_dirs": map[string]interface{}{
                        "type":        "boolean",
                        "description": "Create parent directories if needed",
                        "default":     true,
                    },
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
                    "path": map[string]interface{}{
                        "type":        "string",
                        "description": "File path to read",
                    },
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
                    "path": map[string]interface{}{
                        "type":        "string",
                        "description": "File path to delete",
                    },
                },
                "required": []string{"path"},
            },
        },
        {
            Name:        "list_files",
            Description: "List files in a directory",
            Parameters: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "path": map[string]interface{}{
                        "type":        "string",
                        "description": "Directory path",
                        "default":     ".",
                    },
                    "pattern": map[string]interface{}{
                        "type":        "string",
                        "description": "Optional glob pattern to filter files",
                    },
                    "recursive": map[string]interface{}{
                        "type":        "boolean",
                        "description": "List files recursively",
                        "default":     false,
                    },
                },
            },
        },
    }
}
```

## Validation

### Input Validation
```go
func (a *EnhancedLLMInferenceActivity) validateInput(
    input LLMInferenceInput,
    config LLMInferenceConfig,
) error {
    // Validate provider
    if input.Provider == "" {
        return fmt.Errorf("provider is required")
    }
    
    if !a.registry.HasProvider(input.Provider) {
        return fmt.Errorf("unsupported provider: %s", input.Provider)
    }
    
    // Validate model
    if input.Model == "" {
        return fmt.Errorf("model is required")
    }
    
    // Validate prompt
    if input.Prompt == "" && len(input.Files) == 0 {
        return fmt.Errorf("prompt or files required")
    }
    
    // Validate tool configuration
    if input.ExecuteTools {
        if input.ToolWorkingDir == "" {
            return fmt.Errorf("tool_working_dir required when execute_tools is true")
        }
        
        if !config.EnableToolExecution {
            return fmt.Errorf("tool execution is disabled in configuration")
        }
        
        for _, tool := range input.Tools {
            if err := a.validateToolDefinition(tool); err != nil {
                return fmt.Errorf("invalid tool %s: %w", tool.Name, err)
            }
        }
    }
    
    // Validate file handling
    if len(input.Files) > 0 {
        totalSize := 0
        for _, file := range input.Files {
            totalSize += len(file.Content)
        }
        
        if totalSize > config.MaxFileContextSize {
            return fmt.Errorf("total file size %d exceeds limit %d", 
                totalSize, config.MaxFileContextSize)
        }
    }
    
    return nil
}
```

## Error Handling

### Error Types
```go
// server/ops/pkg/llm/errors.go

type LLMOperationError struct {
    Provider  string
    Model     string
    ErrorType string
    Message   string
    Retryable bool
}

func (e *LLMOperationError) Error() string {
    return fmt.Sprintf("%s/%s: %s", e.Provider, e.Model, e.Message)
}

type ToolExecutionError struct {
    ToolName  string
    Arguments map[string]interface{}
    Error     error
    Duration  time.Duration
}

func (e *ToolExecutionError) Error() string {
    return fmt.Sprintf("tool %s failed: %v", e.ToolName, e.Error)
}
```

## Testing

### Unit Tests
```go
// server/ops/pkg/llm/llm_inference_enhanced_test.go

func TestEnhancedLLMInferenceActivity(t *testing.T) {
    activity := NewEnhancedLLMInferenceActivity()
    
    t.Run("BackwardCompatibility", func(t *testing.T) {
        // Test that old format still works
        input := LLMInferenceInput{
            Provider: "OpenAI",
            Model:    "gpt-3.5-turbo",
            Prompt:   "Hello, world!",
        }
        
        config := LLMInferenceConfig{
            APIKeys: map[string]string{
                "OpenAI": "test-key",
            },
        }
        
        // Should work without files or tools
        output, err := activity.Execute(context.Background(), config, input)
        require.NoError(t, err)
        assert.NotEmpty(t, output.Response)
    })
    
    t.Run("WithFiles", func(t *testing.T) {
        input := LLMInferenceInput{
            Provider: "OpenAI",
            Model:    "gpt-4",
            Prompt:   "Analyze these files",
            Files: []adapters.File{
                {
                    Path:    "main.go",
                    Content: []byte("package main"),
                    Type:    adapters.FileTypeCode,
                },
            },
            FileHandling: "native",
        }
        
        // Test file handling
        output, err := activity.Execute(context.Background(), LLMInferenceConfig{}, input)
        require.NoError(t, err)
        assert.NotEmpty(t, output.Response)
    })
    
    t.Run("WithTools", func(t *testing.T) {
        tmpDir := t.TempDir()
        
        input := LLMInferenceInput{
            Provider: "OpenAI",
            Model:    "gpt-4",
            Prompt:   "Create a hello.txt file with 'Hello, World!' content",
            Tools:    GetBuiltinTools(),
            ExecuteTools: true,
            ToolWorkingDir: tmpDir,
        }
        
        output, err := activity.Execute(context.Background(), LLMInferenceConfig{
            EnableToolExecution: true,
        }, input)
        
        require.NoError(t, err)
        assert.Contains(t, output.FilesWritten, "hello.txt")
        
        // Verify file was created
        content, err := os.ReadFile(filepath.Join(tmpDir, "hello.txt"))
        require.NoError(t, err)
        assert.Contains(t, string(content), "Hello")
    })
}
```

## Metrics

```go
// server/ops/pkg/metrics/llm_metrics.go

var (
    LLMInferenceRequests = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "llm_inference_requests_total",
            Help: "Total number of LLM inference requests",
        },
        []string{"provider", "model", "with_files", "with_tools"},
    )
    
    LLMInferenceDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "llm_inference_duration_seconds",
            Help:    "Duration of LLM inference operations",
            Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
        },
        []string{"provider", "model", "status"},
    )
    
    LLMToolExecutions = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "llm_tool_executions_total",
            Help: "Total number of tool executions",
        },
        []string{"provider", "tool", "status"},
    )
    
    LLMTokenUsage = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "llm_token_usage",
            Help:    "Token usage per request",
            Buckets: []float64{100, 500, 1000, 2000, 4000, 8000, 16000, 32000},
        },
        []string{"provider", "model", "type"}, // type: prompt, completion, total
    )
)
```

## Security Considerations

1. **API Key Management**: Keys stored securely in config, never logged
2. **Tool Sandboxing**: All tool executions are sandboxed with path restrictions
3. **Input Validation**: Schema validation for all tool arguments
4. **Resource Limits**: Token limits, file size limits, execution timeouts
5. **Audit Logging**: All tool executions are logged for audit
6. **Secret Filtering**: Automatic detection and filtering of secrets in outputs

## Migration Guide

### From v1 to v2
1. **No Changes Required**: Existing recipes work without modification
2. **Optional Enhancements**: Add files/tools to leverage new features
3. **Configuration Update**: Add new config sections for tools/files
4. **Testing**: Verify backward compatibility with existing workflows

## Future Enhancements

1. **Streaming Support**: Stream responses for long-running generations
2. **Multi-Modal Tools**: Support image/audio manipulation tools
3. **Custom Tool Plugins**: Allow users to register custom tools
4. **Result Caching**: Cache LLM responses for identical inputs
5. **Advanced Retry Logic**: Provider-specific retry strategies
6. **Cost Tracking**: Track and report API usage costs