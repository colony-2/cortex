# New Operations Specification

## Overview
This specification defines new operations and enhancements to existing operations to support advanced file handling and tool execution in the recipe system.

## 1. Git File Collector Operation

### Purpose
Collects files from a git repository and converts them into File objects compatible with the LLM FileAdapter interface. This operation provides git-aware file collection that respects version control boundaries and ignore patterns.

### Operation Definition
```yaml
type: git_file_collector
```

### Input Schema
```go
type GitFileCollectorOperation struct {
    // Base directory of the git repository
    ContextDir string `json:"context_dir" required:"true" description:"Git repository directory to collect files from"`
    
    // Optional glob patterns to filter collected files (applied after git filtering)
    FilePatterns []string `json:"file_patterns,omitempty" description:"Glob patterns to include (e.g., '**/*.go', 'src/**/*')"`
    
    // Glob patterns for files to exclude
    ExcludePatterns []string `json:"exclude_patterns,omitempty" description:"Glob patterns to exclude (e.g., '*_test.go', 'vendor/**')"`
    
    // Maximum size for individual files (in bytes)
    MaxFileSize int `json:"max_file_size,omitempty" default:"100000" description:"Maximum file size in bytes"`
    
    // Include files that are staged but not committed
    IncludeStaged bool `json:"include_staged,omitempty" default:"true" description:"Include staged changes"`
    
    // Include untracked files
    IncludeUntracked bool `json:"include_untracked,omitempty" default:"false" description:"Include untracked files"`
    
    // Automatically detect file types from content and extension
    AutoDetectType bool `json:"auto_detect_type,omitempty" default:"true" description:"Auto-detect file MIME types"`
}
```

### Output Schema
```go
type GitFileCollectorOutput struct {
    // Array of File objects compatible with LLM FileAdapter
    Files []File `json:"files" description:"Collected files as File objects"`
}

// File structure (matching llm/adapters/file_adapter.go)
type File struct {
    Path     string                 `json:"path"`      // Relative path from context_dir
    Name     string                 `json:"name"`      // Filename
    Content  []byte                 `json:"content"`   // File contents
    MimeType string                 `json:"mime_type"` // Detected MIME type
    Type     FileType               `json:"type"`      // text, image, pdf, etc.
    Metadata map[string]interface{} `json:"metadata"`  // Additional metadata
}
```

### Implementation Details

#### Git Integration
- Uses `git ls-files` to list tracked files
- Respects `.gitignore` patterns automatically
- Optionally includes staged changes via `git diff --staged --name-only`
- Optionally includes untracked files via `git ls-files --others --exclude-standard`

#### File Processing
1. Enumerate files using git commands
2. Apply additional glob patterns if specified
3. Read file content (up to MaxFileSize)
4. Detect MIME type and file type
5. Create File objects with metadata

#### File Type Detection
- Text files: Source code, configuration, documentation
- Images: PNG, JPEG, GIF, SVG
- Documents: PDF, Word, Excel
- Data: JSON, XML, CSV, YAML
- Binary: Executables, archives

### Usage Examples

#### Basic Usage
```yaml
- id: collect_files
  op: git_file_collector
  inputs:
    context_dir: "/project/src"
```

#### With Filters
```yaml
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
```

#### Including Untracked Files
```yaml
- id: collect_all
  op: git_file_collector
  inputs:
    context_dir: "/project"
    include_staged: true
    include_untracked: true
    max_file_size: 500000
```

### Error Handling
- Returns error if context_dir is not a git repository
- Skips files larger than MaxFileSize with warning
- Handles binary files gracefully
- Reports permission errors for unreadable files

---

## 2. Enhanced LLM Inference Operation

### Purpose
Extends the existing `llm_inference` operation to support file inputs through the FileAdapter interface and tool execution capabilities.

### Enhancements to Existing Operation

#### New Input Fields
```go
// Additional fields for LLMInferenceOperation
type LLMInferenceOperationEnhanced struct {
    // Existing fields...
    
    // Files to pass to the LLM (from git_file_collector or other sources)
    Files []File `json:"files,omitempty" description:"Files to include as context"`
    
    // Tools the LLM can call
    Tools []Tool `json:"tools,omitempty" description:"Available tools for the LLM"`
    
    // How to handle files (native, text_fallback, hybrid)
    FileHandling string `json:"file_handling,omitempty" default:"native" description:"File handling mode"`
    
    // Whether to automatically execute tool calls
    ExecuteTools bool `json:"execute_tools,omitempty" default:"false" description:"Auto-execute tool calls"`
    
    // Working directory for tool execution
    ToolWorkingDir string `json:"tool_working_dir,omitempty" description:"Working directory for tools"`
}
```

#### Tool Definition Schema
```go
type Tool struct {
    Name        string          `json:"name" required:"true"`
    Description string          `json:"description" required:"true"`
    Parameters  json.RawMessage `json:"parameters"` // JSON Schema for parameters
}
```

#### Enhanced Output Schema
```go
type LLMInferenceOutputEnhanced struct {
    // Existing fields...
    Response     interface{} `json:"response"`
    Model        string      `json:"model"`
    FinishReason string      `json:"finish_reason"`
    Usage        map[string]interface{} `json:"usage"`
    
    // New fields for tool calling
    ToolCalls           []ToolCall   `json:"tool_calls,omitempty"`
    ToolResults         []ToolResult `json:"tool_results,omitempty"`
    ToolExecutionErrors []string     `json:"tool_execution_errors,omitempty"`
    
    // File operation tracking
    FilesWritten []string `json:"files_written,omitempty"`
    FilesDeleted []string `json:"files_deleted,omitempty"`
    FilesRead    []string `json:"files_read,omitempty"`
}
```

### Tool Execution Flow

1. **File Input Processing**
   - If Files provided, use `FileAdapter.GenerateWithFiles()`
   - Apply FileHandling strategy (native, text_fallback, hybrid)
   - Provider-specific optimizations applied automatically

2. **Tool Calling**
   - If Tools provided, use `Adapter.GenerateWithTools()`
   - LLM can invoke tools through function/tool calling
   - Tools are validated against their JSON Schema

3. **Tool Execution** (when ExecuteTools=true)
   - Parse tool calls from LLM response
   - Validate arguments against tool schema
   - Execute tools in ToolWorkingDir
   - Collect results and errors
   - Optionally continue conversation with results

### Built-in Tools

#### write_file
```yaml
name: write_file
description: Write or update a file
parameters:
  type: object
  properties:
    path:
      type: string
      description: Relative path to the file
    content:
      type: string
      description: Complete file content
  required: ["path", "content"]
```

#### delete_file
```yaml
name: delete_file
description: Delete a file
parameters:
  type: object
  properties:
    path:
      type: string
      description: Relative path to the file
  required: ["path"]
```

#### read_file
```yaml
name: read_file
description: Read a file's content
parameters:
  type: object
  properties:
    path:
      type: string
      description: Relative path to the file
  required: ["path"]
```

### Usage Examples

#### With Files and Tools
```yaml
- id: generate_with_context
  op: llm_inference
  inputs:
    provider: "OpenAI"
    model: "gpt-4"
    files: "{{ .collect_files.outputs.files }}"
    file_handling: "native"
    tools:
      - name: write_file
        description: Write a file
        parameters:
          type: object
          properties:
            path: { type: string }
            content: { type: string }
          required: ["path", "content"]
    execute_tools: true
    tool_working_dir: "/project/src"
    prompt: "Refactor the authentication module"
```

#### File Handling Modes

**native**: Use provider's native file handling
- OpenAI: Vision API for images, text embedding for code
- Gemini: Native file upload API
- Anthropic: Text embedding

**text_fallback**: Convert all files to text representation
- Images: Base64 or description
- PDFs: Text extraction
- Binary: Hex representation or skip

**hybrid**: Use native for supported types, fallback for others
- Optimal balance of functionality and compatibility

### Security Considerations

#### Tool Execution Sandboxing
- Tools execute in specified ToolWorkingDir only
- Path traversal prevention
- File operations restricted to working directory
- Resource limits enforced

#### Sensitive Data Handling
- Automatic filtering of common secret patterns
- Environment variable masking
- API key detection and removal

### Error Handling
- Provider-specific error mapping
- Graceful degradation for unsupported file types
- Tool execution timeout protection
- Detailed error reporting in output

---

## Integration with Existing Systems

### Compatibility
- Leverages existing `FileAdapter` interface from `server/llm/adapters/`
- Uses standard `GenerateWithTools()` method
- Compatible with all supported LLM providers (OpenAI, Anthropic, Gemini)
- Follows existing operation schema patterns

### Dependencies
- Git command-line tools for `git_file_collector`
- LLM adapter implementations
- File system access permissions
- JSON Schema validation library

### Performance Considerations
- File collection is I/O bound - consider caching
- Large file sets may exceed LLM context limits
- Tool execution adds latency - batch when possible
- Provider rate limits apply to file uploads

---

## Testing Requirements

### Unit Tests
- Git file collection with various repository states
- File type detection accuracy
- Tool schema validation
- Tool execution sandboxing

### Integration Tests
- End-to-end file collection and LLM processing
- Tool execution with real file operations
- Provider-specific file handling
- Error recovery scenarios

### Performance Tests
- Large repository handling
- Multiple file upload efficiency
- Tool execution overhead
- Concurrent operation handling

---

## Future Enhancements

### Planned Features
1. **Incremental File Updates**
   - Diff-based file modifications
   - Partial file updates
   - Merge conflict resolution

2. **Advanced Git Integration**
   - Branch operations
   - Commit creation
   - Pull request generation

3. **Tool Library**
   - Database operations
   - API calls
   - Testing tools
   - Linting and formatting

4. **Caching Layer**
   - File content caching
   - LLM response caching
   - Tool result caching

### Extensibility Points
- Custom file type handlers
- Plugin-based tool system
- Provider-specific optimizations
- Custom validation rules