# Bug Report: Template Interpolation Fails for Nested Output Values

## Date: 2025-09-03

## Affected Component
Recipe Worker - Template Engine in Output Processing

## Severity
High - Partial functionality broken

## Description
Template expressions in nested objects within the `outputs:` section are not being evaluated. Top-level string values and array string elements are correctly interpolated, but values inside nested objects remain as raw template strings.

## Minimal Reproduction

### Recipe Definition (test-template-nested.yaml)
```yaml
id: test-template-nested
desc: Minimal test for nested template interpolation
version: 1.0.0
inputs:
  value1: default1
  value2: default2
sequence:
- id: echo_step
  op: echo_activity
  inputs:
    message: 'Testing nested templates'
outputs:
  simple: '{{ inputs.value1 }}'          # ✓ Works
  nested:
    level1: '{{ inputs.value1 }}'        # ✗ Doesn't work
    level2:
      deep: '{{ inputs.value2 }}'        # ✗ Doesn't work
  array:
  - '{{ inputs.value1 }}'                # ✓ Works
  - item: '{{ inputs.value2 }}'          # ✗ Doesn't work (object in array)
```

### Test Input
```yaml
inputs:
  value1: 'test1'
  value2: 'test2'
```

### Expected Output
```json
{
  "simple": "test1",
  "nested": {
    "level1": "test1",
    "level2": {
      "deep": "test2"
    }
  },
  "array": [
    "test1",
    {
      "item": "test2"
    }
  ]
}
```

### Actual Output
```json
{
  "simple": "test1",                        // ✓ Correctly interpolated
  "nested": {
    "level1": "{{ inputs.value1 }}",        // ✗ Not interpolated
    "level2": {
      "deep": "{{ inputs.value2 }}"         // ✗ Not interpolated
    }
  },
  "array": [
    "test1",                                 // ✓ Correctly interpolated
    {
      "item": "{{ inputs.value2 }}"         // ✗ Not interpolated
    }
  ]
}
```

## Pattern Analysis
Template interpolation works for:
- Top-level string values in outputs
- Direct string elements in arrays

Template interpolation fails for:
- String values inside nested objects
- String values inside objects that are array elements
- Any string value that is not a direct child of `outputs:` or a direct array element

## Impact
- Cannot use dynamic values in structured/hierarchical outputs
- Affects any recipe using nested output structures
- Breaks test cases in `state-machine-composition` and potentially other recipes

## Root Cause
The template engine is not recursively processing template expressions in nested objects within the outputs section. It appears to only process:
1. Direct children of the outputs map (if they're strings)
2. Direct string elements of arrays

But it skips processing for:
1. Values inside nested objects
2. Values inside objects that are array elements

## Suggested Fix
The template engine needs to recursively traverse the entire output structure and evaluate all string values containing template expressions, regardless of nesting depth.

## Verification
After fix, run:
```bash
go test ./test-fixtures/... -v -run "TestAllRecipes/test-template-nested"
```

The test should pass with all nested values correctly interpolated.