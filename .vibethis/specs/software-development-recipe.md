# Software Development Recipe Specification

## Overview
A recipe pattern that enables AI-assisted software development through iterative code generation, compilation, and testing cycles. This recipe leverages the `git_file_collector` and enhanced `llm_inference` operations to create a powerful development workflow.

## Purpose
Automate common software development tasks by:
- Collecting repository context using git-aware file collection
- Generating or modifying code using LLMs with file context
- Iteratively fixing build and test failures
- Maintaining code quality through automated testing

## Recipe Metadata
```yaml
name: software_development_cycle
version: "1.0"
description: "Iterative AI-assisted software development with build/test validation"
tags: ["development", "ai", "automation", "testing"]
```

## Input Schema
```yaml
input_schema:
  cell_name:
    type: string
    required: true
    description: "Name of the moon cell/project to build and test"
    example: "my-app"
  
  working_directory:
    type: string
    required: true
    description: "Directory where the development activity will occur"
    example: "./server/my-app"
  
  context_directory:
    type: string
    required: true
    description: "Directory containing source files to provide as context"
    example: "./server/my-app/src"
  
  development_prompt:
    type: string
    required: true
    description: "Software development task description for the LLM"
    example: "Add input validation to the user registration endpoint"
  
  max_iterations:
    type: number
    default: 5
    description: "Maximum build/test fix iterations before failing"
    minimum: 1
    maximum: 10
  
  provider:
    type: string
    default: "OpenAI"
    enum: ["OpenAI", "Anthropic", "Gemini"]
    description: "LLM provider to use"
  
  model:
    type: string
    default: "gpt-4"
    description: "Model identifier for the chosen provider"
```

## Workflow Structure

### State Machine Design
```yaml
states:
  initial: collect_context
  
  collect_context:
    op: git_file_collector
    inputs:
      context_dir: "{{ .inputs.context_directory }}"
      include_staged: true
      include_untracked: false
      file_patterns:
        - "**/*.go"
        - "**/*.js"
        - "**/*.ts"
        - "**/*.py"
        - "**/go.mod"
        - "**/package.json"
        - "**/requirements.txt"
      exclude_patterns:
        - "**/node_modules/**"
        - "**/vendor/**"
        - "**/.git/**"
        - "**/dist/**"
        - "**/build/**"
    transitions:
      - to: generate_code
  
  generate_code:
    op: llm_inference
    inputs:
      provider: "{{ .inputs.provider }}"
      model: "{{ .inputs.model }}"
      file_handling: "native"
      files: "{{ .collect_context.outputs.files }}"
      tools:
        - name: write_file
          description: Write or update a file in the working directory
          parameters:
            type: object
            properties:
              path:
                type: string
                description: Relative path to the file from working directory
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
        
        - name: read_file
          description: Read additional file not in initial context
          parameters:
            type: object
            properties:
              path:
                type: string
                description: Relative path to the file to read
            required: ["path"]
      
      execute_tools: true
      tool_working_dir: "{{ .inputs.working_directory }}"
      
      system_prompt: |
        You are an expert software developer. You have access to the current codebase files.
        Your task is to implement the requested changes while:
        - Maintaining code style and conventions
        - Ensuring backward compatibility
        - Following best practices for the language/framework
        - Writing clean, maintainable code
        - Adding appropriate error handling
        - Updating or adding tests as needed
        
        Use the provided tools to read, write, and delete files as necessary.
        Make all changes required to complete the task successfully.
      
      prompt: |
        Task: {{ .inputs.development_prompt }}
        
        Working directory: {{ .inputs.working_directory }}
        
        Context: You have access to the project files. The project uses moon for build management.
        Build command: moon {{ .inputs.cell_name }}:build
        Test command: moon {{ .inputs.cell_name }}:test
        
        {{ if .build_errors }}
        Previous build failed with errors:
        {{ .build_errors }}
        
        Please fix these build errors.
        {{ end }}
        
        {{ if .test_errors }}
        Previous tests failed with errors:
        {{ .test_errors }}
        
        Please fix these test failures.
        {{ end }}
        
        {{ if not .build_errors and not .test_errors }}
        Please implement the requested changes. Ensure the code compiles and tests pass.
        {{ end }}
      
      max_tokens: 8000
      temperature: 0.7
      
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
      - when: "outputs.exit_code != 0 && state.iteration_count < inputs.max_iterations"
        to: generate_code
        with:
          build_errors: "{{ .outputs.stderr }}"
          iteration_count: "{{ state.iteration_count + 1 }}"
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
      - when: "outputs.exit_code != 0 && state.iteration_count < inputs.max_iterations"
        to: generate_code
        with:
          test_errors: "{{ .outputs.stderr }}"
          iteration_count: "{{ state.iteration_count + 1 }}"
      - to: test_failed
  
  success:
    op: command_execution
    inputs:
      run: |
        echo "✅ Development task completed successfully!"
        echo "Files modified:"
        git diff --name-only
        echo ""
        echo "Summary of changes:"
        git diff --stat
    outputs:
      success: true
      message: "Development task completed successfully"
      modified_files: "{{ .outputs.stdout }}"
  
  build_failed:
    error: "Build failed after {{ .state.iteration_count }} attempts. Last error: {{ .run_build.outputs.stderr }}"
  
  test_failed:
    error: "Tests failed after {{ .state.iteration_count }} attempts. Last error: {{ .run_tests.outputs.stderr }}"
  
  handle_write_error:
    error: "Failed to execute file operations: {{ .generate_code.outputs.tool_execution_errors }}"
```

## Usage Examples

### Basic Usage
```yaml
name: add_validation
version: "1.0"

op: recipe
inputs:
  recipe: "software_development_cycle"
  cell_name: "api-server"
  working_directory: "./server/api"
  context_directory: "./server/api/src"
  development_prompt: |
    Add input validation to the POST /users endpoint:
    - Validate email format
    - Ensure password is at least 8 characters
    - Check username is unique
    - Return appropriate error messages
```

### Feature Implementation
```yaml
name: implement_feature
version: "1.0"

input_schema:
  feature_description:
    type: string
    required: true

op: recipe
inputs:
  recipe: "software_development_cycle"
  cell_name: "web-app"
  working_directory: "./web/app"
  context_directory: "./web/app/src"
  development_prompt: |
    Implement the following feature:
    {{ .inputs.feature_description }}
    
    Requirements:
    - Follow existing code patterns
    - Add unit tests
    - Update documentation
    - Ensure accessibility compliance
  max_iterations: 3
  provider: "Anthropic"
  model: "claude-3-opus"
```

### Bug Fix Workflow
```yaml
name: fix_bug
version: "1.0"

sequence:
  - id: reproduce_bug
    op: command_execution
    inputs:
      run: "./scripts/reproduce_bug.sh"
      continue_on_error: true
  
  - id: fix_bug
    op: recipe
    inputs:
      recipe: "software_development_cycle"
      cell_name: "service"
      working_directory: "./server/service"
      context_directory: "./server/service"
      development_prompt: |
        Fix the following bug:
        
        Error output:
        {{ .reproduce_bug.outputs.stderr }}
        
        The bug occurs in the data processing pipeline.
        Ensure the fix handles edge cases properly.
      max_iterations: 5
  
  - id: verify_fix
    op: command_execution
    inputs:
      run: "./scripts/reproduce_bug.sh"
```

## Iteration Flow

### Success Path
1. Collect git-tracked files from context directory
2. LLM generates/modifies code using file context and tools
3. Run build command - succeeds
4. Run test command - succeeds
5. Report success with file changes

### Failure Recovery Path
1. Collect git-tracked files from context directory
2. LLM generates/modifies code
3. Run build command - fails
4. Pass build errors back to LLM (iteration 2)
5. LLM fixes build errors
6. Run build command - succeeds
7. Run test command - fails
8. Pass test errors back to LLM (iteration 3)
9. LLM fixes test failures
10. Run build command - succeeds
11. Run test command - succeeds
12. Report success

### Maximum Iterations
- Prevents infinite loops
- Configurable per use case
- Returns detailed error on exhaustion
- Preserves partial progress

## Output Schema
```yaml
success:
  success: true
  message: string
  modified_files: string[]
  iterations_used: number
  build_output: string
  test_output: string

failure:
  success: false
  error: string
  last_build_error: string
  last_test_error: string
  iterations_attempted: number
  partial_changes: string[]
```

## Best Practices

### Context Management
- Include only relevant source files
- Use file patterns to filter by language/framework
- Exclude generated code and dependencies
- Keep context under token limits

### Prompt Engineering
- Provide clear, specific instructions
- Include build/test command information
- Reference coding standards and conventions
- Specify error handling requirements

### Error Recovery
- Set appropriate max_iterations (3-5 typical)
- Include detailed error context
- Allow LLM to read additional files if needed
- Preserve working changes between iterations

### Testing Strategy
- Always run tests after successful build
- Include test writing in development prompt
- Validate edge cases
- Ensure backward compatibility

## Monitoring and Observability

### Metrics to Track
- Success rate per iteration
- Average iterations to success
- Token usage per task
- File modification patterns
- Common error categories

### Logging
- Log all LLM interactions
- Record tool executions
- Capture build/test outputs
- Track iteration progression

### Debugging
- Enable verbose mode for tool execution
- Inspect intermediate file states
- Review LLM reasoning
- Analyze error patterns

## Security Considerations

### Code Execution
- Commands run in specified working directory only
- Timeout limits prevent infinite loops
- Resource usage monitoring
- No arbitrary command execution

### File Operations
- Restricted to working directory
- Path traversal prevention
- File size limits
- Binary file handling

### LLM Interactions
- Sensitive data filtering
- API key protection
- Rate limiting
- Token usage controls

## Performance Optimization

### Caching
- Cache git file collection
- Reuse LLM context when possible
- Cache build artifacts
- Store test results

### Parallelization
- Run independent tests in parallel
- Batch file operations
- Concurrent LLM requests for different modules

### Resource Management
- Limit context size
- Optimize file reading
- Clean up temporary files
- Monitor memory usage

## Error Handling

### Common Errors and Solutions

#### Build Failures
- Syntax errors: LLM fixes in next iteration
- Missing dependencies: Add to context or prompt
- Type errors: Include type definitions
- Import errors: Ensure proper module structure

#### Test Failures
- Unit test failures: LLM updates test logic
- Integration test failures: Check service dependencies
- Timeout errors: Increase test timeout
- Flaky tests: Add retry logic

#### Tool Execution Errors
- File not found: Create parent directories
- Permission denied: Check file permissions
- Disk full: Clean up temporary files
- Invalid paths: Validate before execution

## Extension Points

### Custom Tools
Add domain-specific tools:
- Database migrations
- API client generation
- Documentation updates
- Deployment scripts

### Custom Validators
- Lint checking
- Security scanning
- Performance analysis
- Code coverage

### Integration Hooks
- Pre-build hooks
- Post-test hooks
- Success callbacks
- Failure handlers

## Limitations

### Current Limitations
- Requires moon build system
- Git repository required
- Single working directory
- Synchronous execution

### Planned Improvements
- Support for other build systems
- Multi-repository support
- Distributed execution
- Incremental builds

## Migration Guide

### From Manual Development
1. Set up moon build configuration
2. Ensure git repository
3. Create test suites
4. Define development prompts
5. Run recipe with small tasks first

### From Other Automation
1. Map existing build commands to moon
2. Convert test runners
3. Adapt file structures
4. Update CI/CD pipelines

## Conclusion
This recipe provides a powerful pattern for AI-assisted software development, combining git-aware context collection, intelligent code generation, and iterative error correction to automate complex development tasks while maintaining code quality and test coverage.