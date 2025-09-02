# Template Resolution Specification

## Overview

This document specifies the correct implementation of template resolution for the recipe execution system using a hybrid approach:
- **Input/Output fields**: Go templates (text/template) with Sprig functions + custom `cel` function
- **When conditions**: Pure CEL expressions

The current implementation has significant gaps in scope isolation, sibling visibility, and attempt tracking that need to be addressed.

## Core Principles

1. **Hybrid Evaluation**: Go templates for inputs/outputs with CEL for conditions and complex expressions
2. **Consistent Syntax**: No leading dots anywhere - same variable paths in both Go templates and CEL
3. **Scope Isolation**: Each composite node (State Machine, Sequence) creates an isolated scope boundary
4. **Hierarchical Resolution**: Inner scopes can see parent scope inputs/outputs through explicit mapping
5. **Sibling Visibility**: Nodes within the same scope can reference each other based on execution order
6. **Attempt Tracking**: Multiple execution attempts are tracked and accessible via explicit indexing
7. **Fail-Fast Validation**: Invalid template references should be detected before execution

## Scope Rules

### Node Types

- **Leaf Nodes**: `NodeOp`, `RecipeOp` - Execute operations, no scope creation
- **Composite Nodes**: `NodeSequence`, `RecipeSequence`, `NodeState`, `RecipeState` - Create new scopes
- **Shared Nodes**: Pre-resolved references, ignored during template resolution

### Scope Boundaries

Each composite node creates a new scope that:
- Inner nodes cannot directly access sibling scopes
- Can access parent scope through explicit input/output mappings
- Maintains its own local context for child nodes

### Template Variable Structure

**Key Design Decision**: To maintain consistency between Go templates and CEL expressions, the template data is structured as a map with named keys (inputs, outputs, sequence, states, scope) rather than passing data as root context. This allows identical syntax in both contexts without leading dots.

#### Within Sequences
```
sequence.<node_id>.outputs.<output_name>  # Reference to sibling node output
sequence.<node_id>.attempts[<n>].outputs.<output_name>  # Specific attempt
inputs.<input_name>  # Sequence inputs
outputs.<output_name>  # Current node outputs (in output mapping only)
```

#### Within State Machines
```
states.<state_id>.outputs.<output_name>  # State output (completed states only)
inputs.<input_name>  # State machine inputs

# In transitions, can access current state's nodes:
sequence.<node_id>.outputs.<output_name>  # If current state is a sequence
outputs.<output_name>  # Current state's op outputs (if leaf op)
```

#### Global Context
```
scope.attempts.current  # Current attempt number
scope.attempts.max  # Maximum attempts configured
scope.timestamp  # Current timestamp
scope.execution_id  # Unique execution identifier
```

#### Template Types and Usage

**Go Templates (inputs/outputs fields):**
```yaml
inputs:
  # Direct variable access (no leading dot)
  data: "{{ inputs.raw_data }}"
  
  # With Sprig functions
  formatted: "{{ upper inputs.name }}"
  
  # Complex expression via cel function
  computed: "{{ cel \"sequence.node1.outputs.count + 10\" }}"
```

**Pure CEL (when conditions):**
```yaml
transitions:
  - when: "sequence.transform.outputs.success == true"
  - when: "outputs.valid && outputs.score > 0.8"
```

## Template and CEL Integration

### Hybrid Evaluation Model

The system uses two evaluation contexts:

1. **Go Templates** (for `inputs` and `outputs` fields):
   - Standard Go text/template with Sprig functions
   - Custom `cel` function for complex expressions
   - Template data structured as map for consistent syntax

2. **Pure CEL** (for `when` conditions):
   - Direct CEL expression evaluation
   - No template delimiters needed

### Template Data Structure

Both Go templates and CEL receive the same data structure to ensure consistency:

```go
// TemplateData is the root context for both Go templates and CEL
type TemplateData struct {
    inputs   map[string]interface{}  // Current scope inputs
    outputs  map[string]interface{}  // Current outputs (context-dependent)
    sequence map[string]NodeOutput   // Sibling nodes in sequence
    states   map[string]StateOutput  // Completed states in state machine
    scope    ScopeMetadata          // Execution metadata
}

type NodeOutput struct {
    outputs  map[string]interface{}
    attempts []AttemptOutput  // If retries occurred
}

type StateOutput struct {
    outputs  map[string]interface{}
    attempts []AttemptOutput  // If retries occurred
}

type ScopeMetadata struct {
    attempts     AttemptInfo
    execution_id string
    timestamp    time.Time
}

type AttemptInfo struct {
    current int
    max     int
}
```

### Resolution Context Structure

```go
type ResolutionContext struct {
    // Scope type: "root", "sequence", "state_machine", "state"
    ScopeType string
    
    // Parent context (nil for root)
    Parent *ResolutionContext
    
    // Template data for current scope
    TemplateData TemplateData
    
    // Go template with Sprig + cel function
    GoTemplate *template.Template
    
    // CEL environment for when expressions
    CELEnv *cel.Env
    
    // Metadata
    ScopeID string
    AttemptNumber int
    MaxAttempts int
}

// ResolveTemplate handles Go template evaluation for inputs/outputs
func (rc *ResolutionContext) ResolveTemplate(expr string) (interface{}, error) {
    // Execute Go template with TemplateData as root context
    // This allows "inputs.field" syntax without leading dot
    return rc.GoTemplate.Execute(expr, rc.TemplateData)
}

// EvaluateCEL handles pure CEL evaluation for when conditions
func (rc *ResolutionContext) EvaluateCEL(expr string) (bool, error) {
    // Compile and evaluate CEL expression with same TemplateData
    program, err := rc.CELEnv.Compile(expr)
    if err != nil {
        return false, err
    }
    
    // Pass TemplateData fields as CEL variables
    result, _, err := program.Eval(map[string]interface{}{
        "inputs":   rc.TemplateData.inputs,
        "outputs":  rc.TemplateData.outputs,
        "sequence": rc.TemplateData.sequence,
        "states":   rc.TemplateData.states,
        "scope":    rc.TemplateData.scope,
    })
    
    return result.Value().(bool), err
}
```

### Template Functions

The Go template environment includes:
- All Sprig functions
- Custom `cel` function for evaluating CEL expressions within templates:

```go
func celFunction(rc *ResolutionContext) func(string) (interface{}, error) {
    return func(expr string) (interface{}, error) {
        // Evaluate CEL expression with current scope's data
        return rc.EvaluateCEL(expr)
    }
}

## Implementation Changes Required

### 1. Template Resolver Refactoring

**Current Issues:**
- `template.go` and `template2.go` have overlapping, inconsistent implementations
- No proper scope isolation
- Missing sibling reference support
- No attempt tracking

**Required Changes:**
- Consolidate into single `template_resolver.go`
- Implement `ResolutionContext` hierarchy
- Add path validation before execution
- Support attempt indexing

### 2. Execution Context Updates

**Current Issues:**
- `StateContext` mixes global and local state
- `WorkflowState` doesn't track scope boundaries
- No attempt tracking in context

**Required Changes:**
- Add `ExecutionScope` to track current scope
- Maintain scope stack during execution
- Track attempts per node/state
- Pass proper context to template resolver

### 3. Compiler Integration

**Current Issues:**
- `compiler.go` doesn't maintain proper scope during execution
- Template resolution happens at wrong scope levels
- No validation of template references

**Required Changes:**
- Create new scope for each composite node
- Pass scoped context to child executions
- Validate all templates before execution starts
- Properly merge outputs when exiting scope

## Examples

### Example 1: Simple Sequence

```yaml
sequence:
  - id: fetch_data
    op: http_get
    inputs:
      url: "https://api.example.com/data"
    
  - id: process_data
    op: transform
    inputs:
      data: "{{ sequence.fetch_data.outputs.body }}"  # Reference previous node (CEL)
      
  - id: save_result
    op: database_save
    inputs:
      record: "{{ sequence.process_data.outputs.transformed }}"

outputs:
  final_result: "{{ sequence.save_result.outputs.id }}"
```

**CEL Variables at `process_data`:**
```
{
  "inputs": { /* sequence inputs */ },
  "sequence": {
    "fetch_data": {
      "outputs": { "body": {...}, "status": 200 }
    }
  },
  "scope": {
    "attempts": { "current": 1, "max": 1 },
    "execution_id": "exec-123",
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

### Example 2: State Machine with Nested Sequence

```yaml
states:
  initial: processing
  processing:
    sequence:
      - id: validate
        op: validator
        inputs:
          data: "{{ inputs.raw_data }}"  # Go template accessing state machine input
          
      - id: transform
        op: transformer
        inputs:
          valid_data: "{{ sequence.validate.outputs.cleaned }}"  # Go template
    
    outputs:
      result: "{{ sequence.transform.outputs.processed }}"
      status: "{{ sequence.transform.outputs.success }}"
    
    transitions:
      - to: complete
        when: "sequence.transform.outputs.success == true"  # CEL expression
      - to: error
        when: "sequence.transform.outputs.success == false"
  
  complete:
    op: finalize
    inputs:
      # Can reference previous state outputs
      processed: "{{ states.processing.outputs.result }}"
```

**Template Data at `transform` node (Go template context):**
```
{
  "inputs": { "raw_data": {...} },  // From state machine
  "sequence": {
    "validate": {
      "outputs": { "cleaned": {...}, "valid": true }
    }
  },
  "scope": {
    "attempts": { "current": 1, "max": 3 },
    "execution_id": "exec-123",
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

**Variables at transition evaluation (CEL context):**
```
{
  "inputs": { "raw_data": {...} },
  "sequence": {  // The state's sequence nodes
    "validate": { "outputs": {...} },
    "transform": { "outputs": { "processed": {...}, "success": true } }
  },
  "states": {}, // No completed states yet
  "scope": { ... }
}
```

### Example 3: Retry with Attempt References

```yaml
sequence:
  - id: api_call
    op: http_post
    retry:
      max_attempts: 3
    inputs:
      url: "{{ inputs.endpoint }}"
      
  - id: process
    op: processor
    inputs:
      # Reference latest attempt by default
      latest: "{{ sequence.api_call.outputs.response }}"
      # Reference specific attempt
      first_attempt: "{{ sequence.api_call.attempts[0].outputs.response }}"
      # Check current attempt number
      attempt_info: "{{ scope.attempts.current }}"
```

### Example 4: Complex State Machine

```yaml
states:
  initial: validate
  
  validate:
    op: validator
    inputs:
      data: "{{ inputs.payload }}"
    transitions:
      - to: process
        when: "outputs.valid == true"
      - to: error
        when: "outputs.valid == false"
  
  process:
    sequence:
      - id: enrich
        op: enricher
        inputs:
          # Can see state machine input
          original: "{{ inputs.payload }}"
          # Can see previous state output via parent scope
          validation: "{{ states.validate.outputs.metadata }}"
          
      - id: transform
        op: transformer
        inputs:
          # Can see sibling in sequence
          enriched: "{{ sequence.enrich.outputs.data }}"
    
    outputs:
      final: "{{ sequence.transform.outputs.result }}"
    
    transitions:
      - to: complete
        when: "outputs.final != null"
```

**CEL Variables at `transform` in `process` state sequence:**
```
{
  "inputs": { "payload": {...} },  // State machine inputs
  "states": {  // Previous state outputs accessible via parent
    "validate": {
      "outputs": { "valid": true, "metadata": {...} }
    }
  },
  "sequence": {  // Sibling nodes in current sequence
    "enrich": {
      "outputs": { "data": {...} }
    }
  },
  "scope": {
    "attempts": { "current": 1, "max": 1 },
    "execution_id": "exec-123"
  }
}
```

## Invalid Reference Examples

These should fail during validation:

### 1. Scope Boundary Violations

```yaml
# INVALID: Trying to access sibling sequence internals
sequence:
  - id: seq1
    sequence:
      - id: inner1
        op: op1
        
  - id: seq2  
    sequence:
      - id: inner2
        op: op2
        inputs:
          # INVALID: Cannot see inside sibling sequence
          invalid: "{{ sequence.seq1.sequence.inner1.outputs.data }}"
          # VALID: Can see sequence output
          valid: "{{ sequence.seq1.outputs.result }}"
```

### 2. Temporal Violations

```yaml
# INVALID: Accessing future nodes
sequence:
  - id: node1
    op: op1
    inputs:
      # INVALID: node2 hasn't executed yet
      invalid: "{{ sequence.node2.outputs.data }}"
      
  - id: node2
    op: op2
```

### 3. Non-Existent References

```yaml
sequence:
  - id: fetch
    op: http_get
    inputs:
      url: "https://api.example.com"
      
  - id: process
    op: processor
    inputs:
      # INVALID: Non-existent node
      data1: "{{ sequence.nonexistent.outputs.data }}"
      
      # INVALID: Non-existent output field
      data2: "{{ sequence.fetch.outputs.nonexistent_field }}"
      
      # INVALID: Wrong path structure
      data3: "{{ sequence.fetch.nonexistent.outputs }}"
      
      # INVALID: Non-existent input
      data4: "{{ inputs.undefined_input }}"
```

### 4. State Machine Invalid References

```yaml
states:
  initial: state1
  
  state1:
    op: op1
    transitions:
      - to: state2
        # INVALID: Cannot reference non-existent state
        when: "states.nonexistent_state.outputs.result == true"
  
  state2:
    op: op2
    inputs:
      # INVALID: Cannot reference state that may not have executed
      data: "{{ states.state3.outputs.data }}"
    transitions:
      - to: state3
        # INVALID: Wrong output reference
        when: "outputs.nonexistent_field == true"
  
  state3:
    op: op3
```

### 5. Invalid Attempt References

```yaml
sequence:
  - id: retriable
    op: http_post
    retry:
      max_attempts: 3
      
  - id: consumer
    op: processor
    inputs:
      # INVALID: Attempt index out of bounds
      data: "{{ sequence.retriable.attempts[5].outputs.result }}"
      
      # INVALID: Attempt reference on non-retryable node
      data2: "{{ sequence.consumer.attempts[1].outputs.data }}"
```

### 6. Type Mismatches in CEL

```yaml
sequence:
  - id: fetch
    op: fetcher
    # Returns outputs.count as integer
    
  - id: process
    op: processor
    inputs:
      # INVALID: String concatenation with number
      message: "{{ 'Count: ' + sequence.fetch.outputs.count }}"
      
      # VALID: Proper string conversion
      message_valid: "{{ 'Count: ' + string(sequence.fetch.outputs.count) }}"
```

### 7. Cross-Scope State Access

```yaml
states:
  initial: parallel_state
  
  parallel_state:
    sequence:
      - id: step1
        state:
          initial: inner_state1
          inner_state1:
            op: op1
            transitions:
              - to: inner_state2
          inner_state2:
            op: op2
            inputs:
              # INVALID: Cannot access parent sequence sibling's internal state
              data: "{{ sequence.step1.states.inner_state1.outputs.data }}"
              
      - id: step2
        op: op3
        inputs:
          # VALID: Can access step1's outputs
          data: "{{ sequence.step1.outputs.result }}"
```

### 8. Invalid Scope Metadata Access

```yaml
sequence:
  - id: node1
    op: op1
    inputs:
      # INVALID: Wrong scope metadata path
      attempt: "{{ scope.attempt }}"  # Should be scope.attempts.current
      
      # INVALID: Non-existent scope field
      user: "{{ scope.user_id }}"
      
      # VALID: Correct scope access
      current_attempt: "{{ scope.attempts.current }}"
```

## Implementation Plan

### Phase 1: Template Resolution Core
1. Create new `template_resolver.go` with hybrid Go template + CEL approach
2. Structure template data as map (no root context) for consistent syntax
3. Implement custom `cel` function for Go templates
4. Add validation for both template types

### Phase 2: Execution Integration
1. Update `compiler.go` to build proper `TemplateData` structure
2. Modify `statemachine_compiler.go` to track state outputs correctly
3. Update sequence execution to maintain sibling visibility
4. Ensure transitions evaluate against correct context (current state's nodes, not outputs map)

### Phase 3: Testing & Validation
1. Add comprehensive unit tests for each scope type
2. Test attempt tracking and references
3. Validate all error cases shown in invalid examples
4. Test Go template functions (Sprig + cel)

### Phase 4: Cleanup
1. Remove old `template.go` and `template2.go`
2. Consolidate all resolution through new hybrid resolver

## Success Criteria

1. **Consistent Syntax**: No leading dots in any context - same paths work in Go templates and CEL
2. **Hybrid Evaluation**: Go templates for inputs/outputs, CEL for conditions
3. **Scope Isolation**: Child scopes cannot access parent siblings
4. **Sibling Visibility**: Nodes can reference prior siblings in sequence
5. **State Transitions**: Transitions correctly evaluate current state's node outputs
6. **Attempt Tracking**: All attempts are accessible via indexing
7. **Validation**: Invalid references fail during template/CEL compilation before execution
8. **Performance**: Template resolution adds < 5ms overhead per node
9. **Error Messages**: Clear error messages for both Go template and CEL failures