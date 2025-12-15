# Bug Report: Missing Required Field Validation Not Working

**Date**: 2025-09-03  
**Reporter**: Claude Code Assistant  
**Severity**: Medium  
**Status**: Open  

## Summary

The test `test-missing-fields` is designed to verify that missing required fields in operations cause validation errors, but the validation doesn't appear to be working as expected at runtime.

## Background

The test was originally failing with a parse error because the recipe structure was invalid (node with `id` and `inputs` but no `op`). After fixing the structure, the test now passes, but this raises questions about whether required field validation is actually being performed.

## Expected Behavior

Operations should validate their required inputs and fail with appropriate error messages when required fields are missing:
- `echo_activity` requires a `message` input
- `command_execution` requires a `run` input

**Expected test recipe**:
```yaml
sequence:
- id: step1
  op: echo_activity
  # Missing required 'message' input - should fail
- id: step2
  op: command_execution
  # Missing required 'run' input - should fail
```

**Expected error**: "missing required fields" or similar validation error

## Actual Behavior

The test passes without any validation errors when required inputs are missing from operations.

## Test Case Analysis

**Current test expectations**:
```yaml
tests:
- name: missing_required_fields
  description: Test that missing required fields cause failure
  inputs: {}
  want: {}
  wantErr: true
  expectedError: missing required fields
```

The test expects an error but may not be getting one if validation is not enforced.

## Impact

- Required field validation may not be working
- Operations could run with missing critical inputs
- Potential runtime failures instead of validation-time failures
- Reduced reliability of recipe execution

## Investigation Needed

1. Check if operations actually validate their required inputs
2. Verify if `echo_activity` works without a `message` field
3. Verify if `command_execution` works without a `run` field
4. Determine if validation is compile-time or runtime
5. Check if test framework properly captures validation errors

## Files Affected

- `/Users/jnadeau/src/colony2/server/recipe-worker/test-fixtures/recipes/test-missing-fields.yaml`
- `/Users/jnadeau/src/colony2/server/recipe-worker/test-fixtures/recipes/test-missing-fields.test.yaml`
- Operation validation logic in activities
- Test framework validation error handling

## Suggested Fix

1. Implement proper input validation for all operations
2. Validate required fields at parse/compile time when possible
3. Ensure validation errors are properly propagated
4. Update test to verify the specific validation behavior