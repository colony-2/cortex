# Recipe Writing Guide for LLMs

## Quick Start
Recipes are YAML workflows that orchestrate ops. Use `cortex` CLI to validate and test.

## Recipe Structure
```yaml
name: recipe_name
description: What this recipe does
version: "1.0"

input_schema:  # Define input types and requirements
  param1:
    type: string      # string, number, boolean, array, object
    required: true
    default: "value"  # optional
    description: "Description of the parameter"

# Root node can be one of: op, sequence, parallel, or states
sequence:  # Sequential execution example
  - id: step1
    op: operation_type  # operation type (e.g., command_execution, llm_inference)
    inputs:
      param: "{{ .inputs.param1 }}"  # Template syntax
    outputs:
      fieldName: "{{ .outputs.result }}"  # Map to output
```

## Available Ops

### command_execution
Runs shell commands:
```yaml
- id: run_cmd
  op: command_execution
  inputs:
    run: "echo 'Hello'"  # Required: command to run
    working_directory: "/tmp"  # Optional (or working_dir)
    timeout: "30s"  # Optional
    continue_on_error: true  # Optional
    shell: "bash"  # Optional: shell to use
    env:  # Optional: environment variables
      KEY: "value"
```

### llm_inference
LLM API calls (requires API keys):
```yaml
- id: generate
  op: llm_inference
  inputs:
    provider: "OpenAI"  # OpenAI, Anthropic, Gemini
    model: "gpt-4"  # Model identifier
    prompt: "{{ .inputs.user_prompt }}"
    system_prompt: "You are a helpful assistant"  # Optional
    max_tokens: 100
    temperature: 0.7
    top_p: 0.9  # Optional
    stop_sequences: ["\n\n"]  # Optional
    response_schema:  # Optional: structured output schema
      type: object
      properties:
        result:
          type: string
```

### git_shallow_clone
Clone Git repositories:
```yaml
- id: clone
  op: git_shallow_clone
  inputs:
    source_dir: "/path/to/source/repo"  # Source git repository directory
    target_dir: "/path/to/target"  # Target directory for the clone
    commit_hash: "abc123"  # Optional: specific commit to clone
```

### sleep
Delay execution:
```yaml
- id: wait
  op: sleep
  inputs:
    duration: "5s"  # Duration to sleep (e.g., '5s', '2m')
```

### recipe
Invoke child recipes:
```yaml
- id: child
  op: recipe
  inputs:
    recipe: "other_recipe"  # Name of the recipe to invoke
    version: "1.0"  # Optional: version of the recipe
    timeout: "5m"  # Optional: timeout for recipe execution
    retry_policy:  # Optional: retry configuration
      max_attempts: 3
      initial_interval: "1s"
    # Additional inputs for the child recipe
    param: "value"
```

### input
Interactive user input:
```yaml
- id: get_input
  op: input
  inputs:
    question: "Enter your choice"  # For single question
    type: "short_answer"  # short_answer, paragraph_text, multiple_choice, etc.
    timeout: 300  # seconds (max 3600)
    default_on_timeout: "default value"
    # For forms with multiple fields:
    title: "User Information"
    fields:
      - id: name
        type: short_answer
        question: "What is your name?"
        required: true
      - id: age
        type: linear_scale
        question: "Rate your experience"
        scale:
          min: 1
          max: 10
          min_label: "Poor"
          max_label: "Excellent"
```

## Template Syntax
Access data using CEL (Common Expression Language) syntax:
- `inputs.paramName` - Input parameters
- `outputs.field` - Output fields
- `node_id.outputs.field` - Outputs from specific nodes (when using id)
- Array and object access follows standard CEL syntax

Note: Templates in inputs use `{{ .inputs.paramName }}` format

## Parallel Execution
```yaml
parallel:  # Root-level parallel execution
  - id: task1
    op: command_execution
    inputs:
      run: "task1.sh"
  - id: task2
    op: command_execution
    inputs:
      run: "task2.sh"

# Or as a nested node:
- id: parallel_block
  parallel:
    - id: subtask1
      op: command_execution
      inputs:
        run: "subtask1.sh"
    - id: subtask2
      op: command_execution
      inputs:
        run: "subtask2.sh"
```

## State Machines
For complex control flow:
```yaml
name: deployment_pipeline
version: "1.0"

states:
  initial: validate  # Required: initial state
  validate:
    op: command_execution
    inputs:
      run: "test -f config.yaml && test -f deploy.sh"
    transitions:
      - when: "outputs.exit_code == 0"  # CEL expression
        to: deploy
      - when: "outputs.exit_code != 0"
        to: failed
  
  deploy:
    op: command_execution
    inputs:
      run: "./deploy.sh"
      timeout: "5m"
    transitions:
      - when: "outputs.exit_code == 0"
        to: verify
      - when: "outputs.exit_code != 0"
        to: rollback
  
  verify:
    op: command_execution
    inputs:
      run: "curl -f http://localhost:8080/health"
    transitions:
      - when: "outputs.exit_code == 0"
        to: success
      - when: "outputs.exit_code != 0"
        to: rollback
  
  rollback:
    sequence:
      - op: command_execution
        inputs:
          run: "./rollback.sh"
      - op: command_execution
        inputs:
          run: "echo 'Deployment failed' | mail -s 'Alert' ops@example.com"
    transitions:
      - to: failed
  
  success:
    op: command_execution
    inputs:
      run: "echo 'Deployed successfully'"
    # Terminal state - no transitions
  
  failed:
    error: "Deployment failed"  # Terminal error state
```

## Testing Recipes

### 1. Validate Recipe
```bash
cortex validate recipe.yaml
```

### 2. Execute Recipe
```bash
# With inline inputs
cortex execute recipe.yaml -i param1="value1" -i param2=123

# With input file
cortex execute recipe.yaml -f inputs.json

# With debug output
cortex execute recipe.yaml -l debug -i param="value"
```

### 3. Create Test File
Create `recipe.test.yaml` alongside your recipe:
```yaml
tests:
  - name: "test_case_name"
    inputs:
      param1: "test value"
    want:
      result: "expected output"
    wantErr: false
  
  - name: "error_case"
    inputs:
      invalid: "data"
    wantErr: true
    wantErrContains: "error message fragment"
```

**To run tests:**
```bash
# Place files in server/recipe-worker/test-fixtures/recipes/
cd server/recipe-worker
go test ./test-fixtures -v
```

## Recipe Composition

### Child Recipes
```yaml
sequence:
  - id: prepare
    op: recipe
    inputs:
      recipe: "data_prep"
      source: "{{ .inputs.data }}"
  
  - id: analyze
    op: recipe
    inputs:
      recipe: "analysis"
      data: "{{ .prepare.outputs.processed }}"
```

### Conditional Execution
```yaml
sequence:
  - id: check
    op: command_execution
    inputs:
      run: "test -f data.json"
  
  - id: process
    when: "check.outputs.exit_code == 0"  # CEL expression
    op: command_execution
    inputs:
      run: "jq '.items[]' data.json"
```

### Error Recovery
```yaml
sequence:
  - id: try_primary
    op: command_execution
    inputs:
      run: "primary.sh"
      continue_on_error: true
  
  - id: fallback
    when: "try_primary.outputs.exit_code != 0"  # CEL expression
    op: command_execution
    inputs:
      run: "fallback.sh"
```

## Common Patterns

### Sequential Processing
```yaml
sequence:
  - id: fetch
    op: command_execution
    inputs:
      run: "curl -s https://api.example.com/data"
  
  - id: process
    op: command_execution
    inputs:
      run: "echo '{{ .fetch.outputs.stdout }}' | jq '.items[]'"
```

### Parallel Git Clones
```yaml
- id: clone_repos
  parallel:
    - id: repo1
      op: git_shallow_clone
      inputs:
        source_dir: "/source/repo1"
        target_dir: "/target/repo1"
    - id: repo2
      op: git_shallow_clone
      inputs:
        source_dir: "/source/repo2"
        target_dir: "/target/repo2"
```

### LLM Processing Pipeline
```yaml
sequence:
  - id: generate_code
    op: llm_inference
    inputs:
      provider: "OpenAI"
      model: "gpt-4"
      prompt: "Write a Python function to {{ .inputs.task }}"
      max_tokens: 500
  
  - id: save_code
    op: command_execution
    inputs:
      run: |
        cat > generated.py << 'EOF'
        {{ .generate_code.outputs.response }}
        EOF
  
  - id: test_code
    op: command_execution
    inputs:
      run: "python -m pytest generated.py"
      continue_on_error: true
```

## Additional Features

### Shared Nodes
Define reusable node configurations:
```yaml
shared:
  common_setup:
    op: command_execution
    inputs:
      run: "./setup.sh"
      timeout: "2m"

sequence:
  - shared: common_setup  # Reference shared node
    id: setup1
    inputs:
      env:
        MODE: "test"
  - shared: common_setup
    id: setup2
    inputs:
      env:
        MODE: "prod"
```

### Retry Policies
Configure retry behavior for any node:
```yaml
- id: flaky_operation
  op: command_execution
  inputs:
    run: "./unstable-script.sh"
  retry:
    max_attempts: 3
    initial_interval: "1s"
    backoff_coefficient: 2.0  # Optional: exponential backoff
    max_interval: "30s"  # Optional: cap on retry interval
```

## Debugging Tips

1. **Check Schema**: `cortex schema | jq '.definitions.CommandExecutionOperation'`
2. **Verbose Logging**: `cortex execute recipe.yaml -l debug`
3. **Template Issues**: Missing values show as `<no value>`
4. **Command Output**: Standard outputs include stdout, stderr, exit_code, success, timed_out, error_message

## Quick Reference

| Command | Purpose |
|---------|---------|
| `cortex schema` | View all op schemas |
| `cortex validate file.yaml` | Check recipe syntax |
| `cortex execute file.yaml -i key=value` | Run recipe |
| `cortex execute file.yaml -l debug` | Debug execution |