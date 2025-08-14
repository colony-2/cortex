# Recipe Node Simplification Specification v20 - Implementation Ready

## Overview

This specification defines a **refactoring** of the existing recipe system to simplify user experience and improve maintainability. This is **NOT** introducing new functionality - we're reshaping how recipes are defined while preserving all current capabilities.

## Core Goals

1. **Simplify user mental model** - One consistent pattern for all composition
2. **Improve validation** - Type-specific schemas that enforce semantic correctness
3. **Maintain functionality** - No feature loss, only improved syntax
4. **Enable refactoring** - Clear migration path from current implementation

## Core Concepts

### Node
A **node** is the fundamental execution unit. A node contains **exactly one** of five options:
- `op: <operation_type>` - Executes an operation
- `sequence: [<nodes>]` - Executes nodes in order
- `parallel: [<nodes>]` - Executes nodes concurrently
- `states: <state_map>` - State machine with transitions
- `shared: <shared_name>` - References a shared node definition

### State
A **state** is a named location in a state machine with transition logic.

## Type-Specific Grammar for Validation

Rather than a shared `Node` struct with optional fields, we define specific types to enable proper schema validation:

```yaml
# Node types - these are the building blocks
# ===========================================

# Operation Node - no outputs allowed
OperationNode := {
  id?: string  # Optional at root, required in arrays
  desc?: string  # Human readable description
  op: OperationType
  inputs?: map
  timeout?: string
  retry?: RetryPolicy
  when?: string
  # NO outputs field - operations have inherent outputs
}

# Sequence Node - outputs allowed for encapsulation
SequenceNode := {
  id?: string
  desc?: string  # Human readable description
  sequence: [Node]
  inputs?: map
  outputs?: map  # Can export from child nodes
  timeout?: string
  retry?: RetryPolicy
  when?: string
}

# Parallel Node - outputs allowed for encapsulation
ParallelNode := {
  id?: string
  desc?: string  # Human readable description
  parallel: [Node]
  inputs?: map
  outputs?: map  # Can export from child nodes
  timeout?: string
  retry?: RetryPolicy
  when?: string
}

# State Machine Node - outputs allowed for encapsulation
StateMachineNode := {
  id?: string
  desc?: string  # Human readable description
  states: StateMap
  inputs?: map  # Including initial state
  outputs?: map  # Can export from any state
  timeout?: string
  retry?: RetryPolicy
  when?: string
}

# Shared Reference Node - no inputs/outputs (inherit from referenced node)
SharedNode := {
  id?: string
  desc?: string  # Human readable description
  shared: string  # References key in recipe.shared
  timeout?: string  # Override shared node timeout
  retry?: RetryPolicy  # Override shared node retry
  when?: string
  # NO inputs/outputs - both come from the shared node definition
}

# Polymorphic Node type
Node := OperationNode | SequenceNode | ParallelNode | StateMachineNode | SharedNode

# Recipe - just metadata plus a root node
# ========================================
Recipe := {
  name: string
  version: string
  description?: string
  shared?: map[string]Node  # Shared node definitions (declarations, not references)
  
  # Root is one of FOUR node types (no shared at root):
  op?: OperationType |
  sequence?: [Node] |
  parallel?: [Node] |
  states?: StateMap
  
  # All node properties apply at root:
  id?: string
  desc?: string
  inputs?: map
  outputs?: map  # Only valid for sequence/parallel/states at root
  timeout?: string
  retry?: RetryPolicy
}

# Supporting types
# ================

StateMap := {
  initial: string  # Which state to start with
  <state_name>: State
}

State := Node + {
  # A state is a node plus these additional fields:
  transitions?: [Transition]
  error?: string
}

# Or more explicitly:
State := {
  # All node fields apply:
  op?: OperationType |
  sequence?: [Node] |
  parallel?: [Node] |
  states?: StateMap |
  shared?: string
  
  desc?: string
  inputs?: map
  outputs?: map  # Only for composite types
  timeout?: string
  retry?: RetryPolicy
  
  # Plus state-specific additions:
  transitions?: [Transition]
  error?: string
}

RetryPolicy := {
  max_attempts: number
  initial_interval: string
  backoff_coefficient?: number
  max_interval?: string
}

Transition := {
  to: string
  when?: string
}

OperationType := "command_execution" | "llm_inference" | "git_shallow_clone" | "sleep" | "recipe"
```

## Key Design Decisions

1. **Root embedded directly** - No `root:` label, recipe IS the root node
2. **Five node types** - Added `shared:` as fifth option alongside the original four
3. **Type-specific schemas** - Each node type has its own struct for proper validation
4. **Outputs only where valid** - Operations can't have outputs, composites can
5. **Shared nodes are nodes** - Only complete node definitions can be shared

## Examples

### Simple Recipe (Root as Operation)
```yaml
name: hello_world
version: "1.0"

# Root node embedded directly - no 'root:' label
op: command_execution
inputs:
  run: "echo 'Hello World'"
timeout: "30s"
```

### Sequence at Root
```yaml
name: build_pipeline
version: "1.0"

# Root is a sequence with inputs/outputs
sequence:
  - id: clone
    op: git_shallow_clone
    inputs:
      url: "{{ .inputs.repo_url }}"
      depth: 1
  
  - id: test
    op: command_execution
    inputs:
      run: "npm test"
      working_directory: "./repo"
    timeout: "5m"
    retry:
      max_attempts: 3
      initial_interval: "1s"

# Root node properties
inputs:
  repo_url: "https://github.com/user/repo"  # Root node's inputs
outputs:
  test_results: "{{ .nodes.test.stdout }}"  # Root node's outputs
timeout: "10m"  # Overall timeout for the sequence
```

### State Machine at Root
```yaml
name: deployment_flow
version: "1.0"

# Root is a state machine
states:
  initial: validate  # Part of StateMap
  validate:
    op: command_execution
    inputs:
      run: "validate_env.sh {{ .inputs.environment }}"
    transitions:
      - to: deploy
        when: ".exit_code == 0"
      - to: failed
  
  deploy:
    op: command_execution
    inputs:
      run: "deploy.sh {{ .inputs.environment }}"
    timeout: "5m"
    transitions:
      - to: success
  
  success:  # Terminal - no transitions
    op: command_execution
    inputs:
      run: "echo 'Deployed successfully'"
  
  failed:  # Terminal
    error: "Deployment failed"

# Root node properties
inputs:
  environment: "production"  # Root node's inputs
outputs:
  deployment_id: "{{ .context.deployment_id }}"  # Root node's outputs
  status: "{{ .current_state }}"
```

### Shared Nodes
```yaml
name: data_processor
version: "1.0"

shared:
  fetch_with_retry:  # Shared node definition
    op: command_execution
    timeout: "30s"
    retry:
      max_attempts: 3
      initial_interval: "2s"
    inputs:
      run: "curl -f {{ .inputs.url }}"
  
  validate_json:
    sequence:
      - id: parse
        op: command_execution
        inputs:
          run: "echo '{{ .inputs.data }}' | jq empty"
      
      - id: schema_check
        op: command_execution
        inputs:
          run: "echo '{{ .inputs.data }}' | validate_schema.sh"
    timeout: "1m"

# Root uses sequence
sequence:
  - id: fetch_api1
    shared: fetch_with_retry  # Reference shared node
    inputs:
      url: "https://api1.example.com"  # Merge with shared inputs
  
  - id: fetch_api2
    shared: fetch_with_retry
    inputs:
      url: "https://api2.example.com"
    timeout: "60s"  # Override shared timeout
  
  - id: validate_api1
    shared: validate_json
    inputs:
      data: "{{ .nodes.fetch_api1.stdout }}"
  
  - id: process
    parallel:
      - id: transform1
        op: command_execution
        inputs:
          run: "echo '{{ .nodes.fetch_api1.stdout }}' | transform.sh"
      
      - id: transform2
        op: command_execution
        inputs:
          run: "echo '{{ .nodes.fetch_api2.stdout }}' | transform.sh"

outputs:
  api1_result: "{{ .nodes.process.nodes.transform1.stdout }}"
  api2_result: "{{ .nodes.process.nodes.transform2.stdout }}"
```

### Complex State Machine with Shared Nodes
```yaml
name: robust_processor
version: "1.0"

inputs:
  - name: data_url
    type: string

shared:
  robust_fetch:
    op: command_execution
    timeout: "30s"
    retry:
      max_attempts: 5
      initial_interval: "1s"
      backoff_coefficient: 2.0
    inputs:
      run: "curl -f {{ .inputs.url }}"
  
  log_event:
    op: command_execution
    inputs:
      run: "echo '[{{ .context.timestamp }}] {{ .inputs.level }}: {{ .inputs.message }}'"

# Root is state machine
states:
  initial: fetch  # Part of StateMap
  fetch:
    shared: robust_fetch  # States CAN use shared nodes
    transitions:
      - to: validate
        when: ".exit_code == 0"
      - to: fetch_failed
  
  validate:
    sequence:
      - id: check_size
        op: command_execution
        inputs:
          run: "test $(echo '{{ .states.fetch.stdout }}' | wc -c) -gt 10"
      
      - id: parse_json
        op: command_execution
        inputs:
          run: "echo '{{ .states.fetch.stdout }}' | jq empty"
      
      - id: log  # States can't use shared, nodes in sequences can
        shared: log_event
        inputs:
          level: "info"
          message: "Data validation completed"
    transitions:
      - to: process
        when: ".nodes.parse_json.exit_code == 0"
      - to: validation_failed
  
  process:
    parallel:
      - id: extract
        op: command_execution
        inputs:
          run: "echo '{{ .states.fetch.stdout }}' | jq '.data[]'"
        timeout: "2m"
      
      - id: metadata
        op: command_execution
        inputs:
          run: "echo '{{ .states.fetch.stdout }}' | extract_metadata.sh"
    transitions:
      - to: save
  
  save:
    op: command_execution
    inputs:
      run: "save_results.sh '{{ .states.process.nodes.extract.stdout }}'"
    retry:
      max_attempts: 3
      initial_interval: "500ms"
    transitions:
      - to: success
        when: ".exit_code == 0"
      - to: save_failed
  
  success:  # Terminal
    shared: log_event  # States can use shared nodes
  
  fetch_failed:  # Terminal
    error: "Failed to fetch data"
  
  validation_failed:  # Terminal
    error: "Data validation failed"
  
  save_failed:  # Terminal
    error: "Failed to save results"

# Root node properties
inputs:
  data_url: "https://api.example.com/data"  # Root node's inputs
outputs:
  extracted_data: "{{ .states.process.nodes.extract.stdout }}"
  metadata: "{{ .states.process.nodes.metadata.stdout }}"
  success: "{{ .current_state == 'success' }}"
```

## Implementation Strategy

### This is a Refactoring, Not New Features

**Critical:** This specification describes reshaping the user interface for defining recipes. All existing functionality is preserved:

- Timeouts, retries, shared definitions → Still supported
- State machines, parallel execution → Still supported  
- Error handling, conditional execution → Still supported
- Template syntax, output mapping → Still supported

**What changes:** How users write recipes (syntax)
**What doesn't change:** What recipes can do (semantics)

### Schema Validation Benefits

The type-specific approach enables better validation:

```go
// Instead of this (error-prone):
type Node struct {
    Op       *string       `yaml:"op,omitempty"`
    Sequence []*Node       `yaml:"sequence,omitempty"`
    Parallel []*Node       `yaml:"parallel,omitempty"`
    States   *StateMap     `yaml:"states,omitempty"`
    Shared   *string       `yaml:"shared,omitempty"`
    Outputs  map[string]any `yaml:"outputs,omitempty"` // Wrong! Not valid for ops
}

// Use this (type-safe):
type Node interface {
    isNode()
}

type OperationNode struct {
    Op      string            `yaml:"op"`
    Inputs  map[string]any    `yaml:"inputs,omitempty"`
    Timeout *string           `yaml:"timeout,omitempty"`
    // No Outputs field - compiler prevents misuse
}

type SequenceNode struct {
    Sequence []*Node          `yaml:"sequence"`
    Outputs  map[string]any   `yaml:"outputs,omitempty"` // Valid here
    Timeout  *string          `yaml:"timeout,omitempty"`
}

// etc.
```

### Migration Path

1. **Only support new format**
3. **Convert internal code** to use new representation
4. **Validate semantically** using type-specific schemas

### Validation Rules Enforced by Types

1. **Operations cannot have outputs** - Compiler error if attempted
3. **State machines must have initial state** - Is an additional static label used inside of the statemap (states).
4. **Nodes must be one of five types** - Union type enforcement
5. **States don't have outputs beyond what their inner node supports** 


## Migration Examples

### Old Format
```yaml
name: example
steps:
  - id: step1
    uses: command_execution
    config:
      timeout: "30s"
    inputs:
      run: "echo hello"
```

### New Format  
```yaml
name: example
sequence:
  - id: step1
    op: command_execution
    timeout: "30s"
    inputs:
      run: "echo hello"
```

### Old Parallel
```yaml
steps:
  - id: parallel_work
    parallel:
      steps:
        - id: task1
          uses: command_execution
```

### New Parallel
```yaml
parallel:
   - id: task1
     op: command_execution
```

The new format is more consistent, removes two layers of nesting, and eliminates the artificial `config` vs `inputs` distinction.