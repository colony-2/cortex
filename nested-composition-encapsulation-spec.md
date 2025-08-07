# Nested Composition Encapsulation Specification

## Overview

This specification defines strict encapsulation boundaries for nested compositions (sequential, parallel, conditional) in recipes. The goal is to enforce clean interfaces between nested and outer contexts, preventing direct access to internal implementation details while enabling explicit data flow through well-defined inputs and outputs.

## Problem Statement

The current implementation allows unrestricted access between nested and outer contexts:
- Nested steps can reference `.States.<previous_state>` from outside their scope
- Outer steps can access internal details like `.Steps.stage1.outputs.metadata.enrich_meta`
- No clear interface boundaries between composition levels
- Difficult to reason about data dependencies and side effects
- Makes refactoring and testing complex nested compositions challenging

## Design Principles

1. **Strict Encapsulation**: Each composition level operates in an isolated context
2. **Explicit Interfaces**: Data flows only through declared inputs and outputs
3. **Hierarchical Scoping**: Inner contexts cannot access outer context data directly
4. **Immutable Inputs**: Inputs to a composition are immutable within that scope
5. **Structured Outputs**: Compositions must explicitly declare their output structure

## Proposed Solution

### 1. Composition Interface Definition

Every nested composition (sequential, parallel, conditional) must declare its interface:

```yaml
# Sequential composition with explicit interface
sequential:
  inputs:
    - name: raw_data
      from: "{{ .Inputs.document }}"
    - name: config
      from: "{{ .Steps.previous_step.config }}"
  
  outputs:
    processed_data: "{{ .Steps.transform.result }}"
    metadata: "{{ .Steps.analyze.metadata }}"
    success: "{{ .Steps.validate.is_valid }}"
  
  steps:
    - id: transform
      uses: data_transformer
      inputs:
        # Can only access declared inputs, not outer context
        data: "{{ .Inputs.raw_data }}"
        settings: "{{ .Inputs.config }}"
    
    - id: analyze
      uses: analyzer
      inputs:
        # Can access previous steps within same composition
        data: "{{ .Steps.transform.output }}"
    
    - id: validate
      uses: validator
      inputs:
        data: "{{ .Steps.analyze.result }}"
```

### 2. Access Patterns and Context Sharing

#### Context Boundaries
- **State Machine Context**: All states within a state machine share the same execution context and can access each other's outputs via `.States.<state_name>`
- **Flat Composition Context**: Steps within the same sequential or parallel composition (without additional nesting) share context and can reference each other via `.Steps.<step_id>`
- **Nested Composition Boundaries**: When a step contains a nested sequential, parallel, or conditional composition, that creates a new isolated context requiring explicit interfaces
- **Subtree Passing**: Entire context subtrees can be explicitly passed as inputs to nested compositions

#### Within a Nested Composition
- ✅ Access declared inputs: `{{ .Inputs.<input_name> }}`
- ✅ Access previous steps in same level: `{{ .Steps.<step_id>.outputs }}`
- ✅ Access explicitly passed context: `{{ .Inputs.parent_steps.<step_id> }}` (if passed)
- ❌ Access outer context directly: `{{ .States.<state_name> }}` - **BLOCKED** (unless explicitly passed)
- ❌ Access parent steps directly: `{{ .Parent.Steps.<step_id> }}` - **BLOCKED** (unless explicitly passed)

#### From Outer Context  
- ✅ Access composition outputs: `{{ .Steps.<composition_id>.outputs.<output_name> }}`
- ❌ Access internal steps: `{{ .Steps.<comp_id>.steps.<internal_step> }}` - **BLOCKED**
- ❌ Access composition inputs: `{{ .Steps.<comp_id>.inputs }}` - **BLOCKED**

#### Passing Context Subtrees
```yaml
# Example: Passing parent context to nested composition
sequential:
  inputs:
    - name: current_data
      from: "{{ .Inputs.data }}"
    - name: parent_steps
      from: "{{ .Steps }}"  # Pass entire Steps subtree
    - name: previous_states
      from: "{{ .States }}"  # Pass entire States subtree
  
  outputs:
    result: "{{ .Steps.process.output }}"
  
  steps:
    - id: process
      uses: processor
      inputs:
        data: "{{ .Inputs.current_data }}"
        # Can now access parent context through inputs
        previous_result: "{{ .Inputs.parent_steps.previous_step.output }}"
        state_data: "{{ .Inputs.previous_states.init.data }}"
```

### 3. Conditional Composition Special Rules

Conditional branches inherit the same encapsulation rules but with branch-specific outputs:

```yaml
conditional:
  inputs:
    - name: priority
      from: "{{ .Inputs.priority }}"
    - name: data
      from: "{{ .Steps.fetch.result }}"
  
  outputs:
    # Outputs depend on which branch executes
    result: "{{ .Branch.outputs.result }}"
    branch_taken: "{{ .Branch.name }}"
  
  branches:
    - name: high_priority
      when: ".Inputs.priority == 'high'"
      sequential:
        outputs:
          result: "{{ .Steps.fast_process.output }}"
        steps:
          - id: fast_process
            uses: fast_processor
            inputs:
              data: "{{ .Inputs.data }}"
    
    - name: normal_priority
      default: true
      uses: standard_processor
      inputs:
        data: "{{ .Inputs.data }}"
      outputs:
        result: "{{ .Activity.output }}"
```

### 4. Parallel Composition with Dependencies

```yaml
parallel:
  inputs:
    - name: document
      from: "{{ .States.fetch.document }}"
  
  outputs:
    analysis: "{{ .Steps.analyze.result }}"
    extraction: "{{ .Steps.extract.data }}"
    validation: "{{ .Steps.validate.status }}"
  
  steps:
    - id: extract
      uses: extractor
      inputs:
        doc: "{{ .Inputs.document }}"
    
    - id: analyze
      uses: analyzer
      inputs:
        doc: "{{ .Inputs.document }}"
      depends_on: [extract]  # Can depend on parallel siblings
    
    - id: validate
      uses: validator
      inputs:
        # Access sibling outputs through depends_on
        extracted: "{{ .Steps.extract.output }}"
        analysis: "{{ .Steps.analyze.output }}"
      depends_on: [extract, analyze]
```

## Implementation Details

### Type Definitions

```go
// CompositionInterface defines the contract for nested compositions
type CompositionInterface struct {
    Inputs  []InputDefinition  `yaml:"inputs,omitempty"`
    Outputs map[string]string  `yaml:"outputs"`  // Required for compositions
}

// InputDefinition maps external data to internal names
type InputDefinition struct {
    Name string `yaml:"name"`
    From string `yaml:"from"`  // Template expression from outer context
    Type string `yaml:"type,omitempty"`  // Optional type hint
    
    // Type can be: string, number, boolean, object, array
    // Special types for context passing:
    // - "steps": Step outputs subtree
    // - "states": State outputs subtree  
    // - "context": Full context object
}

// Updated composition types
type SequentialComposition struct {
    CompositionInterface `yaml:",inline"`
    Steps []CompositionStep `yaml:"steps"`
}

type ParallelComposition struct {
    CompositionInterface `yaml:",inline"`
    Steps []CompositionStep `yaml:"steps"`
}

type ConditionalComposition struct {
    CompositionInterface `yaml:",inline"`
    Branches []ConditionalBranch `yaml:"branches"`
}
```

### Context Management

```go
// ExecutionContext maintains the scoped environment for a composition
type ExecutionContext struct {
    // Inputs passed to this composition (read-only)
    Inputs map[string]interface{}
    
    // Step outputs within current composition level only
    StepOutputs map[string]interface{}
    
    // Parent context reference (not directly accessible)
    parent *ExecutionContext
    
    // Composition metadata
    Level int
    Type  string // "sequential", "parallel", "conditional"
}

// CreateChildContext creates an isolated context for nested composition
func (ctx *ExecutionContext) CreateChildContext(inputs map[string]interface{}) *ExecutionContext {
    return &ExecutionContext{
        Inputs:      inputs,  // Copy of resolved inputs
        StepOutputs: make(map[string]interface{}),
        parent:      ctx,
        Level:       ctx.Level + 1,
    }
}

// ResolveOutputs evaluates output templates in current context
func (ctx *ExecutionContext) ResolveOutputs(outputs map[string]string) (map[string]interface{}, error) {
    result := make(map[string]interface{})
    for key, template := range outputs {
        // Only allow access to current context's Steps and Inputs
        value, err := resolveTemplate(template, ctx)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve output %s: %w", key, err)
        }
        result[key] = value
    }
    return result, nil
}
```

### Validation Rules

1. **Input Validation**: All `from` expressions in inputs must reference the parent context
2. **Output Validation**: All output expressions must reference only the current context
3. **Step Reference Validation**: Steps can only reference `.Inputs` and `.Steps` within their context
4. **No Cross-Context Access**: Any attempt to access `.States`, `.Parent`, or other contexts should fail

### CEL Expression Scoping

```go
// BuildCELEnvironment creates a scoped CEL environment for a context
func BuildCELEnvironment(ctx *ExecutionContext) (*cel.Env, error) {
    return cel.NewEnv(
        cel.Variable("Inputs", cel.MapType(cel.StringType, cel.DynType)),
        cel.Variable("Steps", cel.MapType(cel.StringType, cel.DynType)),
        // Notably missing: States, Parent, Context
    )
}
```

## Migration Strategy

### Phase 1: Backward Compatibility Mode
- Add support for new interface syntax
- Detect legacy workflows without interfaces
- Auto-generate interfaces based on usage analysis
- Emit deprecation warnings

### Phase 2: Strict Mode (Opt-in)
- Add workflow-level flag: `strict_encapsulation: true`
- Enforce all encapsulation rules
- Fail on cross-context access attempts

### Phase 3: Default Strict Mode
- Make strict mode the default
- Require explicit `legacy_mode: true` for old behavior
- Provide migration tooling

## State Machine Context Sharing

State machines have special context sharing rules since all states operate within the same logical workflow:

```yaml
states:
  init:
    uses: data_fetcher
    outputs:
      data: "{{ .Activity.result }}"
    transitions:
      - to: process
  
  process:
    # States can access other states' outputs directly
    uses: processor
    inputs:
      data: "{{ .States.init.data }}"  # ALLOWED: States share context
    transitions:
      - to: nested_work
  
  nested_work:
    # But nested compositions still require explicit interfaces
    sequential:
      inputs:
        - name: processed_data
          from: "{{ .States.process.output }}"
        - name: all_states  
          from: "{{ .States }}"  # Can pass entire state context
      
      outputs:
        final: "{{ .Steps.complete.result }}"
      
      steps:
        - id: validate
          uses: validator
          inputs:
            data: "{{ .Inputs.processed_data }}"
            # Access other states through passed context
            original: "{{ .Inputs.all_states.init.data }}"
        
        - id: complete
          uses: finalizer
          inputs:
            validated: "{{ .Steps.validate.output }}"
```

## Examples

### Before (Current Implementation)
```yaml
states:
  process:
    sequential:
      - id: nested
        conditional:
          - when: ".States.previous.data_size > 1000"  # BAD: Direct outer state access
            uses: large_processor
        
      - id: next
        uses: finalizer
        inputs:
          # BAD: Accessing internals of conditional
          data: "{{ .Steps.nested.branches[0].output }}"
```

### After (With Encapsulation)
```yaml
states:
  process:
    sequential:
      - id: nested
        conditional:
          inputs:
            - name: data_size
              from: "{{ .States.previous.data_size }}"
            - name: all_steps
              from: "{{ .Steps }}"  # Pass parent steps if needed
          
          outputs:
            result: "{{ .Branch.outputs.result }}"
            metadata: "{{ .Branch.outputs.metadata }}"
          
          branches:
            - when: ".Inputs.data_size > 1000"  # GOOD: Using declared input
              uses: large_processor
              outputs:
                result: "{{ .Activity.output }}"
                metadata: "{{ .Activity.metadata }}"
      
      - id: next
        uses: finalizer
        inputs:
          # GOOD: Accessing declared output
          data: "{{ .Steps.nested.outputs.result }}"
          metadata: "{{ .Steps.nested.outputs.metadata }}"
```

### Flat vs Nested Composition Context
```yaml
states:
  example:
    # This is a flat parallel composition - all steps share context
    parallel:
      - id: step1
        uses: activity1
        inputs:
          data: "{{ .Inputs.data }}"
      
      - id: step2
        uses: activity2
        inputs:
          # Can directly reference sibling in flat composition
          sibling_data: "{{ .Steps.step1.output }}"
        depends_on: [step1]
      
      - id: step3
        # This nested sequential creates a new context boundary
        sequential:
          inputs:
            - name: parent_data
              from: "{{ .Steps.step1.output }}"  # Must explicitly pass
            - name: sibling_ref
              from: "{{ .Steps.step2 }}"  # Can pass whole step reference
          
          outputs:
            result: "{{ .Steps.process.final }}"
          
          steps:
            - id: process
              uses: processor
              inputs:
                # Must use inputs, not direct parent access
                data: "{{ .Inputs.parent_data }}"
                sibling: "{{ .Inputs.sibling_ref.output }}"
```

### Complex Example with Context Passing
```yaml
states:
  orchestrate:
    parallel:
      - id: stream_a
        sequential:
          inputs:
            - name: config
              from: "{{ .Inputs.stream_config }}"
            - name: parent_context
              from: "{{ .Steps }}"  # Pass entire parent step context
          
          outputs:
            processed: "{{ .Steps.final.data }}"
            stats: "{{ .Steps.analyze.statistics }}"
          
          steps:
            - id: fetch
              uses: fetcher
              inputs:
                config: "{{ .Inputs.config }}"
            
            - id: analyze  
              uses: analyzer
              inputs:
                data: "{{ .Steps.fetch.output }}"
                # Can reference parent context through inputs
                baseline: "{{ .Inputs.parent_context.baseline.metrics }}"
            
            - id: final
              uses: finalizer
              inputs:
                analyzed: "{{ .Steps.analyze.output }}"
      
      - id: stream_b
        uses: direct_processor
        inputs:
          config: "{{ .Inputs.stream_config }}"
      
      - id: baseline
        uses: baseline_calculator
        inputs:
          historical_data: "{{ .States.init.historical }}"
```

## Benefits

1. **Clarity**: Clear data flow and dependencies
2. **Testability**: Nested compositions can be tested in isolation
3. **Reusability**: Compositions with clear interfaces can be extracted and reused
4. **Maintainability**: Changes to internal implementation don't affect consumers
5. **Type Safety**: Potential for future type checking on interfaces
6. **Debugging**: Easier to trace data flow and identify issues

## Testing Requirements

1. **Unit Tests**:
   - Validate input resolution from parent context
   - Validate output resolution in current context
   - Test access violations are properly blocked
   - Test each composition type with interfaces

2. **Integration Tests**:
   - Complex nested workflows with proper encapsulation
   - Migration from legacy to strict mode
   - Error handling for invalid access patterns

3. **Validation Tests**:
   - Schema validation for interface definitions
   - CEL expression scoping validation
   - Template resolution boundary checks

## Open Questions

1. Should we allow read-only access to parent context through explicit declaration?
2. How should we handle context data like timestamps, request IDs that might be needed at all levels?
3. Should parallel steps with dependencies have special access rules?
4. What's the best way to handle error context propagation with strict boundaries?