# Recipe Writing Guide for LLMs

## Quick Start
Recipes are YAML workflows that orchestrate ops. Use `cortex` CLI to validate and test.

## Recipe Structure
```yaml
name: recipe_name
description: What this recipe does
version: "1.0"

inputs:
  - name: param1
    type: string      # string, number, boolean, array, object
    required: true
    default: "value"  # optional

outputs:
  - name: result
    value: "{{ .Steps.stepId.outputs.fieldName }}"
    type: string

steps:
  - id: step1
    uses: op_type       # See available ops below
    config:             # Op-specific config (optional)
      key: value
    inputs:
      param: "{{ .Inputs.param1 }}"  # Template syntax
    outputs:
      fieldName: varName  # Map activity output to variable
```

## Available Ops

### command_execution
Runs shell commands:
```yaml
- id: run_cmd
  uses: command_execution
  inputs:
    run: "echo 'Hello'"  # Required: command to run
    working_directory: "/tmp"  # Optional
    timeout: "30s"  # Optional
    continue_on_error: true  # Optional
```

### llm_inference
LLM API calls (requires API keys):
```yaml
- id: generate
  uses: llm_inference
  config:
    provider: "openai"  # openai, anthropic, gemini
    model: "gpt-4"
  inputs:
    prompt: "{{ .Inputs.user_prompt }}"
    max_tokens: 100
    temperature: 0.7
```

### git_shallow_clone
Clone Git repositories:
```yaml
- id: clone
  uses: git_shallow_clone
  inputs:
    url: "https://github.com/user/repo"
    depth: 1
```

### sleep
Delay execution:
```yaml
- id: wait
  uses: sleep
  inputs:
    duration: "5s"
```

### recipe
Invoke child recipes:
```yaml
- id: child
  uses: recipe
  config:
    recipe_name: "other_recipe"
  inputs:
    param: "value"
```

## Template Syntax
Access data using Go template syntax:
- `{{ .Inputs.paramName }}` - Input parameters
- `{{ .Steps.stepId.outputs.field }}` - Step outputs
- `{{ index .array 0 }}` - Array access
- `{{ .object.field }}` - Object field access

## Parallel Execution
```yaml
- id: parallel_block
  parallel:
    steps:
      - id: task1
        uses: command_execution
        inputs:
          run: "task1.sh"
      - id: task2
        uses: command_execution
        inputs:
          run: "task2.sh"
```

## State Machines
For complex control flow:
```yaml
name: deployment_pipeline
initialState: validate
states:
  - name: validate
    steps:
      - name: check_files
        type: command_execution
        input:
          run: "test -f config.yaml && test -f deploy.sh"
        output: validation
    transitions:
      - condition: 'validation.exit_code == 0'
        target: deploy
      - condition: 'validation.exit_code != 0'
        target: failed
  
  - name: deploy
    steps:
      - name: run_deployment
        type: command_execution
        input:
          run: "./deploy.sh"
          timeout: "5m"
        output: deploy_result
    transitions:
      - condition: 'deploy_result.exit_code == 0'
        target: verify
      - condition: 'deploy_result.exit_code != 0'
        target: rollback
  
  - name: verify
    steps:
      - name: health_check
        type: command_execution
        input:
          run: "curl -f http://localhost:8080/health"
        output: health
    transitions:
      - condition: 'health.exit_code == 0'
        target: success
      - condition: 'health.exit_code != 0'
        target: rollback
  
  - name: rollback
    steps:
      - name: restore_previous
        type: command_execution
        input:
          run: "./rollback.sh"
      - name: notify_failure
        type: command_execution
        input:
          run: "echo 'Deployment failed' | mail -s 'Alert' ops@example.com"
    transitions:
      - target: failed
  
  - name: success
    terminal: true
    steps:
      - name: notify_success
        type: command_execution
        input:
          run: "echo 'Deployed successfully'"
  
  - name: failed
    terminal: true
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
steps:
  - id: prepare
    uses: recipe
    config:
      recipe_name: "data_prep"
    inputs:
      source: "{{ .Inputs.data }}"
    outputs:
      processed: data
  
  - id: analyze
    uses: recipe
    config:
      recipe_name: "analysis"
    inputs:
      data: "{{ .Steps.prepare.outputs.processed }}"
```

### Conditional Execution
```yaml
steps:
  - id: check
    uses: command_execution
    inputs:
      run: "test -f data.json"
    outputs:
      exit_code: file_check
  
  - id: process
    when: "{{ .Steps.check.outputs.exit_code }} == 0"
    uses: command_execution
    inputs:
      run: "jq '.items[]' data.json"
```

### Error Recovery
```yaml
steps:
  - id: try_primary
    uses: command_execution
    inputs:
      run: "primary.sh"
      continue_on_error: true
    outputs:
      exit_code: primary_exit
  
  - id: fallback
    when: "{{ .Steps.try_primary.outputs.exit_code }} != 0"
    uses: command_execution
    inputs:
      run: "fallback.sh"
```

## Common Patterns

### Sequential Processing
```yaml
steps:
  - id: fetch
    uses: command_execution
    inputs:
      run: "curl -s https://api.example.com/data"
    outputs:
      stdout: raw_data
  
  - id: process
    uses: command_execution
    inputs:
      run: "echo '{{ .Steps.fetch.outputs.raw_data }}' | jq '.items[]'"
```

### Parallel Git Clones
```yaml
- id: clone_repos
  parallel:
    steps:
      - id: repo1
        uses: git_shallow_clone
        inputs:
          url: "https://github.com/org/repo1"
      - id: repo2
        uses: git_shallow_clone
        inputs:
          url: "https://github.com/org/repo2"
```

### LLM Processing Pipeline
```yaml
steps:
  - id: generate_code
    uses: llm_inference
    config:
      provider: "openai"
      model: "gpt-4"
    inputs:
      prompt: "Write a Python function to {{ .Inputs.task }}"
      max_tokens: 500
  
  - id: save_code
    uses: command_execution
    inputs:
      run: |
        cat > generated.py << 'EOF'
        {{ .Steps.generate_code.outputs.response }}
        EOF
  
  - id: test_code
    uses: command_execution
    inputs:
      run: "python -m pytest generated.py"
      continue_on_error: true
```

## Debugging Tips

1. **Check Schema**: `cortex schema | jq '.definitions.ops.command_execution'`
2. **Verbose Logging**: `cortex execute recipe.yaml -l debug`
3. **Template Issues**: Missing values show as `<no value>`
4. **Command Output**: stdout, stderr, exit_code are standard outputs

## Quick Reference

| Command | Purpose |
|---------|---------|
| `cortex schema` | View all op schemas |
| `cortex validate file.yaml` | Check recipe syntax |
| `cortex execute file.yaml -i key=value` | Run recipe |
| `cortex execute file.yaml -l debug` | Debug execution |