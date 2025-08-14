# Recipe Step Consolidation Specification

## Overview

This document outlines the consolidation of `Step` and `CompositionStep` types into a single unified `CompositionStep` type, eliminating unnecessary duplication while maintaining backward compatibility with simplified YAML syntax.

## Current State

We currently have two separate step types:
- `Step` (in types.go): Simple activity execution
- `CompositionStep` (in statemachine.go): Complex compositions with activities

This creates:
- Code duplication
- Maintenance overhead  
- Confusion about which type to use
- Inconsistent features (retry, conditions only on some steps)

## Proposed Change

### 1. Single Unified Type

Consolidate to only `CompositionStep`:

```go
type CompositionStep struct {
    ID       string                 `yaml:"id" json:"id"`
    Name     string                 `yaml:"name,omitempty" json:"name,omitempty"`
    
    // Simple activity execution
    Uses     string                 `yaml:"uses,omitempty" json:"uses,omitempty"`
    Config   map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
    
    // Complex compositions (mutually exclusive with Uses)
    Sequential  []CompositionStep   `yaml:"sequential,omitempty" json:"sequential,omitempty"`
    Parallel    []CompositionStep   `yaml:"parallel,omitempty" json:"parallel,omitempty"`
    Conditional []ConditionalBranch `yaml:"conditional,omitempty" json:"conditional,omitempty"`
    
    // Common configuration
    Inputs    map[string]interface{} `yaml:"inputs,omitempty" json:"inputs,omitempty"`
    Outputs   map[string]string      `yaml:"outputs,omitempty" json:"outputs,omitempty"`
    DependsOn []string               `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
    When      string                 `yaml:"when,omitempty" json:"when,omitempty"`
    Retry     *RetryPolicy           `yaml:"retry,omitempty" json:"retry,omitempty"`
}
```

### 2. Unified Retry Policy

Use a single `RetryPolicy` type:

```go
type RetryPolicy struct {
    When               string        `yaml:"when,omitempty" json:"when,omitempty"`
    MaxAttempts        int           `yaml:"max_attempts" json:"max_attempts"`
    InitialInterval    time.Duration `yaml:"initial_interval,omitempty" json:"initial_interval,omitempty"`
    BackoffCoefficient float64       `yaml:"backoff_coefficient,omitempty" json:"backoff_coefficient,omitempty"`
    MaximumInterval    time.Duration `yaml:"maximum_interval,omitempty" json:"maximum_interval,omitempty"`
}
```

## YAML Syntax Support

### Simple Form (Unchanged)

Users can continue writing simple steps without wrapper syntax:

```yaml
steps:
  - id: validate_input
    uses: validation_activity
    config:
      schema: user_schema
    inputs:
      data: "{{ .Inputs.user_data }}"
    outputs:
      result: validation_result
```

### Complex Form (Already Supported)

```yaml
steps:
  - id: process_batch
    sequential:
      - id: validate
        uses: validation_activity
        config:
          schema: batch_schema
      - id: transform
        uses: data_transformer
        when: "{{ .Steps.validate.outputs.valid == true }}"
        inputs:
          data: "{{ .Inputs.batch_data }}"
```

### Parser Behavior

The YAML parser should:
1. Parse all steps as `CompositionStep`
2. Validate that `Uses` and composition fields (Sequential/Parallel/Conditional) are mutually exclusive
3. Apply defaults appropriately
4. No changes needed to existing YAML files

## Benefits

1. **Simplification**: One type to understand and maintain
2. **Feature Parity**: All steps get retry, conditions, dependencies
3. **Evolution Path**: Simple steps can grow into compositions without restructuring
4. **Consistency**: Uniform handling throughout the system
5. **Backward Compatible**: No changes to existing YAML recipes

## Migration Plan

### Phase 1: Internal Consolidation
1. Update `CompositionStep` to include `Config` and `Outputs` fields
2. Add type alias: `type Step = CompositionStep` for compatibility
3. Update parser to use `CompositionStep` everywhere

### Phase 2: Code Updates
1. Replace all `Step` references with `CompositionStep`
2. Update activity registry and compiler
3. Ensure all tests pass

### Phase 3: Cleanup
1. Remove the `Step` type alias
2. Remove `StepRetryPolicy` (use unified `RetryPolicy`)
3. Update documentation

## Validation Rules

1. **Mutual Exclusivity**: A step must have either:
   - `Uses` (for simple activity)
   - One of `Sequential`, `Parallel`, or `Conditional` (for composition)
   - Not both

2. **Config Scope**: 
   - `Config` is only valid when `Uses` is specified
   - Composition steps pass configuration through their child steps

3. **Output Mapping**:
   - Simple steps: `Outputs` maps activity outputs
   - Composition steps: `Outputs` maps from child step outputs

## Example Conversions

### Before (Two Types)
```go
// Simple step
step := Step{
    ID:     "fetch_data",
    Uses:   "http_client",
    Config: map[string]interface{}{"url": "https://api.example.com"},
}

// Composition step  
comp := CompositionStep{
    ID:         "process", 
    Sequential: []CompositionStep{...},
    Retry:      &StepRetryPolicy{MaxAttempts: 3},
}
```

### After (Unified)
```go
// Simple step (same functionality)
step := CompositionStep{
    ID:     "fetch_data",
    Uses:   "http_client",
    Config: map[string]interface{}{"url": "https://api.example.com"},
}

// Composition step (unchanged)
comp := CompositionStep{
    ID:         "process",
    Sequential: []CompositionStep{...},
    Retry:      &RetryPolicy{MaxAttempts: 3},
}
```

## Testing Strategy

1. **Compatibility Tests**: Ensure existing YAML recipes parse correctly
2. **Feature Tests**: Verify retry/conditions work on all step types
3. **Validation Tests**: Check mutual exclusivity rules
4. **Migration Tests**: Test type alias during transition

## Timeline

- Week 1: Implement core changes and type alias
- Week 2: Update parsers and compilers
- Week 3: Testing and validation
- Week 4: Documentation and cleanup

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking existing recipes | Type alias ensures compatibility during transition |
| Performance impact | Nil pointers for unused fields minimize overhead |
| Learning curve | Document that "everything is a composition" simplifies mental model |

## Conclusion

Consolidating to a single `CompositionStep` type simplifies the codebase while maintaining full backward compatibility. The unified model better reflects the reality that all steps are compositions—some just happen to contain a single activity.