# Test Fixtures Fix Summary

## Tests Fixed

### 1. test-missing-fields ✅
**Issue**: Recipe had invalid structure - node with `id` and `inputs` but no `op`
**Fix**: Added proper `op` fields to each node in the sequence
**Status**: PASSING

### 2. test-sleep-operation ✅  
**Issue**: 
- Duplicate `inputs` key in YAML
- Test expectations didn't match actual sleep operation output
**Fix**: 
- Removed duplicate inputs
- Updated test expectations to match actual output (`slept: "1s"`)
**Status**: PASSING

## Functional Bugs Identified

### 1. Nested Template Resolution Bug
**File**: `BUG-REPORT-nested-output-templates.md`
**Issue**: Templates in nested map structures don't resolve
**Example**: `metadata.analysis_type: '{{ inputs.analysis_depth }}'` stays as literal string
**Affected Tests**: state-machine-composition (all 4 test cases)
**Status**: BLOCKED - Requires compiler fix

### 2. Sequence Output Reference Bug
**File**: `BUG-REPORT-sequence-output-reference.md`
**Issue**: Cannot reference sequence node outputs using documented syntax
**Example**: `sequence.step1.outputs.stdout` fails with CEL compilation error
**Affected Tests**: test-sequence-node
**Status**: BLOCKED - Requires compiler fix

## Summary

- **Fixed**: 2 test fixture issues (test-missing-fields, test-sleep-operation)
- **Blocked**: 2 tests due to functional bugs in the compiler
  - state-machine-composition: nested template resolution
  - test-sequence-node: sequence output references

Both blocked issues are compiler-level bugs that need to be fixed in the template resolution logic, not test fixture problems.