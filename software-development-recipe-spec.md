# Software Development Recipe Specification

## Overview
A new recipe pattern that enables AI-assisted software development through iterative code generation, compilation, and testing cycles.

## Recipe Inputs
```yaml
input_schema:
  cell_name:
    type: string
    required: true
    description: "Name of the moon cell/project to build and test"
  
  working_directory:
    type: string
    required: true
    description: "Directory where the development activity will occur"
  
  context_directory:
    type: string
    required: true
    description: "Directory containing source files to provide as context"
  
  development_prompt:
    type: string
    required: true
    description: "Software development task description for the LLM"
  
  max_iterations:
    type: number
    default: 5
    description: "Maximum build/test fix iterations before failing"
```

## New Operations Required

### 1. `git_file_collector` Operation
Collects all git-tracked files from a repository and formats them as File objects compatible with the FileAdapter interface. This is distinct from simple file pattern matching because it:
- Only includes files tracked by git (respects .gitignore)
- Excludes build artifacts and temporary files
- Can include staged changes
- Provides clean repository context

```yaml
type GitFileCollectorOperation struct {
    ContextDir       string   `json:"context_dir" required:"true"`
    FilePatterns     []string `json:"file_patterns,omitempty"` // Optional glob patterns within git files
    ExcludePatterns  []string `json:"exclude_patterns,omitempty"`
    MaxFileSize      int      `json:"max_file_size,omitempty" default:"100000"`
    IncludeStaged    bool     `json:"include_staged,omitempty" default:"true"`
    IncludeUntracked bool     `json:"include_untracked,omitempty" default:"false"`
    AutoDetectType   bool     `json:"auto_detect_type,omitempty" default:"true"`
}

// Outputs:
// - files: []File (array of File objects compatible with FileAdapter)
```

Each File object contains:
- Path: relative path from context_dir
- Name: filename
- Content: file bytes
- MimeType: detected MIME type
- Type: FileTypeText, FileTypeImage, etc.
- Metadata: additional info (size, modified time, etc.)

### 2. Tool Execution Handler (Enhancement to llm_inference)
The enhanced `llm_inference` operation needs to handle tool execution internally when `execute_tools: true`.

When the LLM returns tool calls, the operation should:
1. Validate tool calls against the provided tool definitions
2. Execute the tools (write_file, delete_file, etc.) in the working directory
3. Collect results and errors
4. Optionally pass results back to the LLM for continuation

```yaml
// Tool execution outputs added to llm_inference Response:
// - tool_calls: []ToolCall (tools called by the LLM)
// - tool_results: []ToolResult (results from executed tools)
// - tool_execution_errors: []string (any errors during tool execution)
// - files_written: []string (paths of files written)
// - files_deleted: []string (paths of files deleted)
```

### 3. Enhanced `llm_inference` Operation
Extend existing LLM operation to leverage both the FileAdapter interface for file inputs and the tool calling interface for file outputs:

```yaml
additions to LLMInferenceOperation:
    Files            []File      `json:"files,omitempty"` // Files from context_collector
    Tools            []Tool      `json:"tools,omitempty"` // Tools the LLM can call
    FileHandling     string      `json:"file_handling,omitempty"` // "native", "text_fallback", "hybrid"
    ExecuteTools     bool        `json:"execute_tools,omitempty" default:"true"` // Auto-execute tool calls
```

The operation will:
1. Use the existing `FileAdapter.GenerateWithFiles()` to pass files as first-class inputs
2. Use the existing `Adapter.GenerateWithTools()` to provide tools like file writing
3. Automatically execute tool calls and return results

This leverages existing infrastructure:
- `FileAdapter` interface for native file handling by each provider
- `GenerateWithTools` method for tool/function calling
- Tool definitions following the JSON Schema format
- Automatic tool execution with results passed back to the LLM

## Recipe Workflow Structure

```yaml
name: software_development_cycle
version: "1.0"
description: "Iterative AI-assisted software development with build/test validation"

states:
  initial: collect_context
  
  collect_context:
    op: git_file_collector
    inputs:
      context_dir: "{{ .inputs.context_directory }}"
      include_staged: true
      include_untracked: false  # Only include committed/staged files
    transitions:
      - to: generate_code
  
  generate_code:
    op: llm_inference
    inputs:
      provider: "OpenAI"
      model: "gpt-4"
      file_handling: "native"  # Use native file handling for best results
      files: "{{ .collect_context.outputs.files }}"  # Pass File objects as first-class entities
      tools:
        - name: write_file
          description: Write or update a file in the working directory
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
        - name: delete_file
          description: Delete a file from the working directory
          parameters:
            type: object
            properties:
              path:
                type: string
                description: Relative path to the file to delete
            required: ["path"]
      execute_tools: true  # Automatically execute tool calls
      system_prompt: |
        You are a software developer. Generate or modify code based on the task.
        You have access to the current codebase files provided as context.
        Use the write_file tool to create or update files as needed.
        Make all necessary changes to implement the requested functionality.
      prompt: |
        Task: {{ .inputs.development_prompt }}
        
        Working directory: {{ .inputs.working_directory }}
        
        Previous errors (if any):
        {{ .build_errors }}
        {{ .test_errors }}
        
        Please implement the necessary changes using the available tools.
      max_tokens: 8000
    transitions:
      - when: "outputs.tool_execution_errors.size() > 0"
        to: handle_write_error
      - to: run_build
  
  run_build:
    op: command_execution
    inputs:
      run: "moon {{ .inputs.cell_name }}:build"
      working_directory: "{{ .inputs.working_directory }}"
      timeout: "5m"
      continue_on_error: true
    transitions:
      - when: "outputs.exit_code == 0"
        to: run_tests
      - when: "outputs.exit_code != 0 && iteration_count < inputs.max_iterations"
        to: generate_code
        with:
          build_errors: "{{ .outputs.stderr }}"
      - to: build_failed
  
  run_tests:
    op: command_execution
    inputs:
      run: "moon {{ .inputs.cell_name }}:test"
      working_directory: "{{ .inputs.working_directory }}"
      timeout: "10m"
      continue_on_error: true
    transitions:
      - when: "outputs.exit_code == 0"
        to: success
      - when: "outputs.exit_code != 0 && iteration_count < inputs.max_iterations"
        to: generate_code
        with:
          test_errors: "{{ .outputs.stderr }}"
      - to: test_failed
  
  success:
    op: command_execution
    inputs:
      run: |
        echo "Development task completed successfully!"
        echo "Modified files:"
        git diff --name-only
  
  build_failed:
    error: "Build failed after {{ .inputs.max_iterations }} attempts"
  
  test_failed:
    error: "Tests failed after {{ .inputs.max_iterations }} attempts"
  
  handle_write_error:
    error: "Failed to execute file operations: {{ .generate_code.outputs.tool_execution_errors }}"
```

## Implementation Requirements

### 1. New Op Implementations
- `GitFileCollectorOperation`: Use git commands (`git ls-files`, `git diff --staged`) to list tracked files, read content
- Enhanced LLM operation: Support for file inputs via FileAdapter and tool execution

### 2. Safety Features
- File write validation before applying changes
- Backup mechanism for modified files
- Sandboxing: Restrict file operations to specified directories
- Git integration: Only modify git-tracked files by default
- Size limits: Prevent excessive file operations

### 3. Context Management
- Efficient file content collection (respect .gitignore)
- Token-aware context truncation for LLM
- Incremental context updates between iterations

### 4. Error Handling
- Detailed error messages from build/test failures
- Structured error parsing for LLM consumption
- Iteration counting with configurable limits
- Graceful degradation on partial failures

### 5. State Persistence
- Track iteration count across state transitions
- Accumulate error history for LLM context
- Maintain file change history for rollback

## Usage Example

```yaml
# development-task.yaml
name: implement_feature
version: "1.0"

input_schema:
  feature_description:
    type: string
    required: true

op: recipe
inputs:
  recipe: "software_development_cycle"
  cell_name: "my-app"
  working_directory: "./server/my-app"
  context_directory: "./server/my-app/src"
  development_prompt: |
    Implement the following feature:
    {{ .inputs.feature_description }}
    
    Requirements:
    - Write idiomatic Go code
    - Include appropriate error handling
    - Add unit tests for new functionality
    - Update existing tests if needed
  max_iterations: 3
```

## Testing Strategy

### Unit Tests
- Test each new operation independently
- Mock file system operations
- Validate LLM response parsing
- Test error conditions

### Integration Tests
- Full recipe execution with mock LLM
- File system sandbox testing
- Moon build/test command mocking
- State machine transition validation

### E2E Tests
- Simple code generation tasks
- Build failure recovery scenarios
- Test failure recovery scenarios
- Multi-iteration convergence tests

## Security Considerations

1. **File System Access**
   - Restrict operations to working/context directories
   - Validate all file paths against directory traversal
   - Respect file permissions and ownership

2. **Code Execution**
   - Moon commands run in controlled environment
   - Timeout limits on all operations
   - Resource usage monitoring

3. **LLM Content**
   - Sanitize file content before LLM submission
   - Validate LLM responses against schema
   **- Filter sensitive information (API keys, secrets)

## Future Enhancements

1. **Incremental Development**
   - Support for partial file updates (diffs)
   - Preserve manual changes between iterations
   - Merge conflict resolution

2. **Multi-Language Support**
   - Language-specific build/test commands
   - Syntax validation before applying changes
   - IDE integration for real-time feedback

3. **Collaboration Features**
   - Review generated changes before applying
   - Human-in-the-loop approval steps
   - Change tracking and attribution

4. **Performance Optimization**
   - Parallel file operations**
   - Incremental compilation support
   - Test result caching

5. **Advanced LLM Features**
   - Multi-model consensus for critical changes
   - Code review by secondary LLM
   - Automatic prompt optimization based on success rates