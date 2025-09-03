# LLM Adapter Enhancements Specification

## Overview
This specification defines enhancements to the LLM adapter layer (`server/llm/adapters/`) to support tool execution and improved file handling capabilities for recipe operations.

## Background
The current LLM adapter infrastructure already provides:
- `FileAdapter` interface with `GenerateWithFiles()` method
- `Adapter` interface with `GenerateWithTools()` method
- File type definitions and validation
- Tool calling support for OpenAI, Anthropic, and Gemini

This specification defines enhancements to make these capabilities more suitable for automated recipe execution.

## 1. Tool Execution Framework

### Purpose
Extend the LLM adapters to support automatic execution of tool calls, enabling autonomous operation within recipes.

### New Interface: `ExecutableToolAdapter`
```go
// server/llm/adapters/executable_tool_adapter.go

package adapters

import (
    "context"
)

// ExecutableToolAdapter extends the base Adapter with tool execution capabilities
type ExecutableToolAdapter interface {
    Adapter
    
    // GenerateAndExecuteTools generates a response and automatically executes any tool calls
    GenerateAndExecuteTools(
        ctx context.Context,
        prompt string,
        tools []Tool,
        config ExecutableToolConfig,
    ) (ExecutableToolResponse, error)
    
    // SetToolExecutor sets the executor for handling tool calls
    SetToolExecutor(executor ToolExecutor)
}

// ExecutableToolConfig extends Config with execution settings
type ExecutableToolConfig struct {
    Config
    
    // Whether to automatically execute tool calls
    AutoExecute bool `json:"auto_execute"`
    
    // Maximum number of tool execution rounds
    MaxToolRounds int `json:"max_tool_rounds,omitempty"`
    
    // Working directory for tool execution
    WorkingDirectory string `json:"working_directory,omitempty"`
    
    // Timeout for individual tool executions
    ToolTimeout Duration `json:"tool_timeout,omitempty"`
    
    // Whether to continue conversation after tool execution
    ContinueAfterTools bool `json:"continue_after_tools,omitempty"`
}

// ExecutableToolResponse extends Response with execution details
type ExecutableToolResponse struct {
    Response
    
    // Tool execution results
    ToolResults []ToolResult `json:"tool_results,omitempty"`
    
    // Execution errors (non-fatal)
    ExecutionErrors []ToolExecutionError `json:"execution_errors,omitempty"`
    
    // Execution metadata
    ExecutionMetadata map[string]interface{} `json:"execution_metadata,omitempty"`
}
```

### Tool Executor Interface
```go
// server/llm/adapters/tool_executor.go

package adapters

import (
    "context"
    "encoding/json"
)

// ToolExecutor handles the execution of tool calls
type ToolExecutor interface {
    // ExecuteTool executes a single tool call
    ExecuteTool(ctx context.Context, call ToolCall, config ToolExecutionConfig) (ToolResult, error)
    
    // ValidateTool validates a tool call against its schema
    ValidateTool(call ToolCall, tool Tool) error
    
    // GetAvailableTools returns the list of executable tools
    GetAvailableTools() []Tool
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
    ToolCallID string          `json:"tool_call_id"`
    ToolName   string          `json:"tool_name"`
    Result     json.RawMessage `json:"result,omitempty"`
    Error      string          `json:"error,omitempty"`
    Success    bool            `json:"success"`
    Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ToolExecutionConfig configures individual tool execution
type ToolExecutionConfig struct {
    WorkingDirectory string        `json:"working_directory"`
    Timeout          time.Duration `json:"timeout"`
    Environment      map[string]string `json:"environment,omitempty"`
    Sandbox          bool          `json:"sandbox,omitempty"`
}

// ToolExecutionError provides detailed error information
type ToolExecutionError struct {
    ToolCallID string `json:"tool_call_id"`
    ToolName   string `json:"tool_name"`
    Error      string `json:"error"`
    Timestamp  int64  `json:"timestamp"`
}
```

### Built-in Tool Executors
```go
// server/llm/adapters/file_tool_executor.go

package adapters

// FileToolExecutor handles file operation tools
type FileToolExecutor struct {
    baseDir     string
    maxFileSize int64
    allowDelete bool
}

// NewFileToolExecutor creates a new file tool executor
func NewFileToolExecutor(baseDir string, opts ...FileToolOption) *FileToolExecutor

// Supported tools:
// - write_file: Write or update a file
// - read_file: Read a file's content
// - delete_file: Delete a file
// - list_files: List files in a directory
// - create_directory: Create a directory
```

## 2. Enhanced File Handling

### File Collection Integration
```go
// server/llm/adapters/file_collector.go

package adapters

// FileCollector provides file collection capabilities for LLM context
type FileCollector interface {
    // CollectFiles gathers files based on patterns
    CollectFiles(patterns []string, opts CollectionOptions) ([]File, error)
    
    // CollectGitFiles gathers git-tracked files
    CollectGitFiles(repoPath string, opts GitCollectionOptions) ([]File, error)
}

// CollectionOptions configures file collection
type CollectionOptions struct {
    MaxFileSize      int64    `json:"max_file_size"`
    ExcludePatterns  []string `json:"exclude_patterns"`
    IncludeHidden    bool     `json:"include_hidden"`
    FollowSymlinks   bool     `json:"follow_symlinks"`
    AutoDetectType   bool     `json:"auto_detect_type"`
}

// GitCollectionOptions extends CollectionOptions for git
type GitCollectionOptions struct {
    CollectionOptions
    IncludeStaged    bool `json:"include_staged"`
    IncludeUntracked bool `json:"include_untracked"`
    Branch           string `json:"branch,omitempty"`
}
```

### File Type Detection Enhancements
```go
// server/llm/adapters/file_type_detector.go

package adapters

// FileTypeDetector provides enhanced file type detection
type FileTypeDetector interface {
    // DetectType determines the FileType from content and metadata
    DetectType(content []byte, path string) FileType
    
    // DetectMimeType determines the MIME type
    DetectMimeType(content []byte, path string) string
    
    // IsTextFile checks if content is text-based
    IsTextFile(content []byte) bool
    
    // GetLanguage detects programming language for source files
    GetLanguage(content []byte, path string) string
}

// Enhanced FileType constants
const (
    // Existing types...
    FileTypeText     FileType = "text"
    FileTypeImage    FileType = "image"
    FileTypePDF      FileType = "pdf"
    
    // New types
    FileTypeCode     FileType = "code"      // Source code files
    FileTypeConfig   FileType = "config"    // Configuration files
    FileTypeData     FileType = "data"      // Data files (JSON, XML, CSV)
    FileTypeMarkdown FileType = "markdown"  // Documentation
    FileTypeBinary   FileType = "binary"    // Generic binary
)
```

## 3. Provider-Specific Enhancements

### OpenAI Adapter
```go
// server/llm/adapters/openai_adapter_enhanced.go

// Additional methods for OpenAIAdapter
func (a *OpenAIAdapter) GenerateAndExecuteTools(
    ctx context.Context,
    prompt string,
    tools []Tool,
    config ExecutableToolConfig,
) (ExecutableToolResponse, error) {
    // Implementation:
    // 1. Call existing GenerateWithTools
    // 2. If AutoExecute && tool_calls present:
    //    - Execute tools using executor
    //    - Format results
    //    - If ContinueAfterTools, make follow-up call
    // 3. Return combined response
}

// Support for parallel tool calls (GPT-4 feature)
func (a *OpenAIAdapter) EnableParallelToolCalls(enabled bool)
```

### Anthropic Adapter
```go
// server/llm/adapters/anthropic_adapter_enhanced.go

// Additional methods for AnthropicAdapter
func (a *AnthropicAdapter) GenerateAndExecuteTools(
    ctx context.Context,
    prompt string,
    tools []Tool,
    config ExecutableToolConfig,
) (ExecutableToolResponse, error) {
    // Implementation using Claude's tool use
    // Handle multiple tool calls in sequence
    // Support for chain-of-thought with tools
}

// Support for Claude's enhanced tool descriptions
func (a *AnthropicAdapter) SetToolDescriptionMode(mode string) // "concise" | "detailed"
```

### Gemini Adapter
```go
// server/llm/adapters/gemini_adapter_enhanced.go

// Additional methods for GeminiAdapter
func (a *GeminiAdapter) GenerateAndExecuteTools(
    ctx context.Context,
    prompt string,
    tools []Tool,
    config ExecutableToolConfig,
) (ExecutableToolResponse, error) {
    // Implementation using Gemini's function calling
    // Native file handling optimization
    // Multi-modal tool support
}

// Support for Gemini's code execution
func (a *GeminiAdapter) EnableCodeExecution(enabled bool)
```

## 4. Combined File and Tool Operations

### Unified Interface
```go
// server/llm/adapters/unified_adapter.go

package adapters

// UnifiedAdapter combines file and tool capabilities
type UnifiedAdapter interface {
    FileAdapter
    ExecutableToolAdapter
    
    // GenerateWithFilesAndTools combines both capabilities
    GenerateWithFilesAndTools(
        ctx context.Context,
        prompt string,
        files []File,
        tools []Tool,
        config UnifiedConfig,
    ) (UnifiedResponse, error)
}

// UnifiedConfig combines all configuration options
type UnifiedConfig struct {
    ExecutableToolConfig
    FileHandling FileHandlingMode `json:"file_handling"`
}

// UnifiedResponse combines all response types
type UnifiedResponse struct {
    ExecutableToolResponse
    FilesProcessed []string `json:"files_processed"`
    FilesCreated   []string `json:"files_created,omitempty"`
    FilesModified  []string `json:"files_modified,omitempty"`
    FilesDeleted   []string `json:"files_deleted,omitempty"`
}
```

## 5. Safety and Security

### Sandboxing
```go
// server/llm/adapters/sandbox.go

package adapters

// ToolSandbox provides execution isolation
type ToolSandbox interface {
    // Execute runs a tool in a sandboxed environment
    Execute(ctx context.Context, call ToolCall, config SandboxConfig) (ToolResult, error)
    
    // Validate checks if an operation is allowed
    Validate(call ToolCall) error
}

// SandboxConfig configures the sandbox
type SandboxConfig struct {
    AllowedPaths     []string          `json:"allowed_paths"`
    BlockedPaths     []string          `json:"blocked_paths"`
    MaxFileSize      int64             `json:"max_file_size"`
    MaxExecutionTime time.Duration     `json:"max_execution_time"`
    MaxMemory        int64             `json:"max_memory"`
    Environment      map[string]string `json:"environment"`
    NetworkAccess    bool              `json:"network_access"`
}
```

### Secret Filtering
```go
// server/llm/adapters/secret_filter.go

package adapters

// SecretFilter removes sensitive information
type SecretFilter interface {
    // FilterContent removes secrets from text
    FilterContent(content string) string
    
    // FilterFile removes secrets from file content
    FilterFile(file *File) *File
    
    // AddPattern adds a secret pattern to filter
    AddPattern(pattern string)
}

// Common patterns to filter:
// - API keys (various formats)
// - JWT tokens
// - Private keys
// - Database connection strings
// - Environment variables with SECRET/KEY/TOKEN
```

## 6. Observability

### Metrics
```go
// server/llm/adapters/metrics.go

package adapters

// MetricsCollector tracks adapter usage
type MetricsCollector interface {
    // RecordGeneration tracks basic generation metrics
    RecordGeneration(provider string, model string, duration time.Duration, tokens int)
    
    // RecordToolExecution tracks tool execution
    RecordToolExecution(toolName string, success bool, duration time.Duration)
    
    // RecordFileOperation tracks file operations
    RecordFileOperation(operation string, fileType FileType, size int64)
}

// Metrics to track:
// - Request latency by provider/model
// - Token usage by operation type
// - Tool execution success rate
// - File processing statistics
// - Error rates and types
```

### Logging
```go
// server/llm/adapters/logging.go

package adapters

// StructuredLogger provides detailed logging
type StructuredLogger interface {
    // LogRequest logs LLM request details
    LogRequest(ctx context.Context, prompt string, config Config)
    
    // LogResponse logs LLM response
    LogResponse(ctx context.Context, response Response)
    
    // LogToolExecution logs tool execution details
    LogToolExecution(ctx context.Context, call ToolCall, result ToolResult)
    
    // LogError logs errors with context
    LogError(ctx context.Context, err error, metadata map[string]interface{})
}
```

## 7. Testing Support

### Mock Implementations
```go
// server/llm/adapters/mock_adapter.go

package adapters

// MockAdapter for testing
type MockAdapter struct {
    // Configurable responses
    Responses []Response
    
    // Tool execution simulation
    ToolResults map[string]ToolResult
    
    // Error injection
    ErrorOn string
}

// MockToolExecutor for testing
type MockToolExecutor struct {
    // Predefined results
    Results map[string]interface{}
    
    // Execution tracking
    ExecutedCalls []ToolCall
}
```

## 8. Configuration

### Adapter Configuration
```yaml
# Example configuration for enhanced adapters
adapters:
  openai:
    api_key: ${OPENAI_API_KEY}
    tool_execution:
      enabled: true
      max_rounds: 3
      timeout: 30s
      sandbox: true
    file_handling:
      mode: native
      max_file_size: 1048576
      auto_detect_type: true
    
  anthropic:
    api_key: ${ANTHROPIC_API_KEY}
    tool_execution:
      enabled: true
      description_mode: detailed
      continue_after_tools: true
    
  gemini:
    api_key: ${GEMINI_API_KEY}
    tool_execution:
      enabled: true
      code_execution: true
      parallel_calls: true
```

## 9. Migration Path

### Backward Compatibility
- All existing adapter methods remain unchanged
- New interfaces extend existing ones
- Optional features via configuration
- Graceful degradation for unsupported features

### Migration Steps
1. Update adapter implementations to support new interfaces
2. Add tool executor implementations
3. Configure sandbox and security settings
4. Enable metrics and logging
5. Update tests with new capabilities

## 10. Performance Considerations

### Optimization Strategies
- Tool result caching
- Parallel tool execution where supported
- File content deduplication
- Streaming for large responses
- Connection pooling for API calls

### Resource Limits
- Maximum file context size: 10MB total
- Maximum tool execution time: 60s per tool
- Maximum tool rounds: 5
- Maximum parallel tools: 10
- Rate limiting per provider

## Conclusion
These enhancements to the LLM adapter layer provide the foundation for sophisticated recipe execution with file handling and tool execution capabilities, while maintaining security, observability, and performance.