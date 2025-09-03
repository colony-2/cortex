# Bug Report: Nested Template Resolution in Recipe Outputs

**Date**: 2025-09-03  
**Reporter**: Claude Code Assistant  
**Severity**: High  
**Status**: Open  
**Type**: Functional Bug  

## Summary

Template expressions inside nested structures in recipe outputs are not being resolved. While top-level templates work correctly, templates within nested maps remain as literal strings.

## Reproduction

**Recipe** (`state-machine-composition.yaml`):
```yaml
outputs:
  result:
  - '{{ inputs.message }}'          # This resolves correctly
  metadata:
    analysis_type: '{{ inputs.analysis_depth }}'  # This doesn't resolve
    output_format: '{{ inputs.output_format }}'   # This doesn't resolve
```

**Input**:
```yaml
inputs:
  message: 'Test message'
  analysis_depth: medium
  output_format: json
```

**Expected Output**:
```json
{
  "result": ["Test message"],
  "metadata": {
    "analysis_type": "medium",
    "output_format": "json"
  }
}
```

**Actual Output**:
```json
{
  "result": ["Test message"],
  "metadata": {
    "analysis_type": "{{ inputs.analysis_depth }}",
    "output_format": "{{ inputs.output_format }}"
  }
}
```

## Analysis

The template resolution in `processNodeOutputs` appears to handle top-level templates and arrays correctly, but doesn't recursively resolve templates in nested map structures.

## Files Affected

- `/Users/jnadeau/src/vibethis/server/recipe-worker/test-fixtures/recipes/state-machine-composition.yaml`
- Template resolution logic in `pkg/compiler/compiler.go` - `processNodeOutputs` function

## Test Cases Affected

The `state-machine-composition` test suite fails because of this issue. All 4 test cases show the same problem:
- `medium_depth_analysis` - expects "medium", gets "{{ inputs.analysis_depth }}"
- `deep_analysis` - expects "deep", gets "{{ inputs.analysis_depth }}"
- `quick_analysis` - expects "quick", gets "{{ inputs.analysis_depth }}"
- `default_values` - expects default values, gets unresolved templates

## Root Cause Analysis

The `processNodeOutputs` function in the compiler likely:
1. Processes top-level fields correctly
2. Handles arrays with template resolution
3. **Missing**: Recursive descent into nested map structures to resolve templates

## Proposed Solution

1. Update `processNodeOutputs` to recursively process nested maps
2. Apply template resolution at all levels of the output structure
3. Ensure type preservation (don't stringify non-string values)

## Priority

High - This breaks a common pattern of structured outputs with metadata, affecting recipe composability and data organization.