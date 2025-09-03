# Bug Report: Template Interpolation Failure in State Machine Outputs

## Date: 2025-09-03

## Affected Component
Recipe Worker - State Machine Output Processing

## Affected Test
`state-machine-composition` (all 4 test cases fail)

## Severity
High - Core functionality broken

## Description
Template expressions in the `outputs:` section of state machine recipes are not being evaluated. The raw template strings are being returned instead of interpolated values.

Note: `outputs:` is only valid at root level when the root is a `sequence:` or `state:`, not when it's an `op:`.

## Reproduction

### Recipe Definition (state-machine-composition.yaml)
```yaml
id: basic_state_machine_test
state:
  initial_state: processing
  states:
    # ... state definitions ...
outputs:
  result:
  - '{{ inputs.message }}'
  metadata:
    analysis_type: '{{ inputs.analysis_depth }}'
    output_format: '{{ inputs.output_format }}'
```

### Test Input
```yaml
inputs:
  message: 'Test message'
  analysis_depth: medium
  output_format: json
```

### Expected Output
```json
{
  "result": ["Test message"],
  "metadata": {
    "analysis_type": "medium",
    "output_format": "json"
  }
}
```

### Actual Output
```json
{
  "result": ["Test message"],
  "metadata": {
    "analysis_type": "{{ inputs.analysis_depth }}",
    "output_format": "{{ inputs.output_format }}"
  }
}
```

## Impact
- All state machine recipes with templated outputs are broken
- Cannot dynamically set output values based on inputs
- Affects 4 test cases in state-machine-composition

## Root Cause Analysis
The template engine is not processing expressions in the outputs section of recipes with state machines. The `result` array correctly evaluates `{{ inputs.message }}` but nested object properties in `metadata` are not being processed.

## Suggested Fix
Ensure the template engine recursively processes all template expressions in the outputs section, including nested objects and arrays, when the root is a state machine.

## Verification
After fix, run:
```bash
go test ./test-fixtures/... -v -run "TestAllRecipes/state-machine-composition"
```

All 4 test cases should pass:
- medium_depth_analysis
- deep_analysis  
- quick_analysis
- default_values