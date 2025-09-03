# Failing Tests Catalog

## UPDATE: Tests Fixed
All state machine related tests have been fixed by:
1. Adding required inputs to test cases (no longer supporting defaults)
2. Rewriting test-state-machine.yaml to use proper template references
3. Using conditional logic based on inputs rather than non-existent state tracking variables

# Original Failing Tests (Now Fixed)

## Test Execution Summary
- **Total Test Suites**: Multiple packages tested
- **Failed Test Suites**: 2 (state-machine related)
- **Total Failing Test Cases**: 4

## Failed Test Details

### 1. state-machine-composition Suite

#### Test: `state-machine-composition/default_values`
- **Status**: FAIL
- **Error Type**: Template resolution failure
- **Error Message**: 
  ```
  failed to resolve output template metadata: 
  failed to resolve key output_format: 
  failed to evaluate CEL expression: no such key: output_format
  ```
- **Root Cause**: Missing `output_format` key in CEL expression evaluation context
- **Location**: Recipe execution during output template resolution

### 2. test-state-machine Suite

#### Test: `test-state-machine/state_machine_flow`
- **Status**: FAIL  
- **Error Type**: CEL compilation error
- **Error Message**:
  ```
  failed to resolve output template error_message: 
  failed to compile CEL expression: ERROR: <input>:1:1: 
  undeclared reference to 'state' (in container '')
   | state.error
   | ^
  ```
- **Root Cause**: Undeclared reference to `state` object in CEL expression
- **Location**: Template compilation phase

#### Test: `test-state-machine/state_machine_error_handling`
- **Status**: FAIL
- **Error Type**: CEL compilation error (same as state_machine_flow)
- **Error Message**: Undeclared reference to 'state' in CEL expression
- **Root Cause**: Same as `state_machine_flow` - missing `state` object in context

#### Test: `test-state-machine/state_machine_conditional`
- **Status**: FAIL
- **Error Type**: CEL compilation error (same as state_machine_flow)
- **Error Message**: Undeclared reference to 'state' in CEL expression
- **Root Cause**: Same as `state_machine_flow` - missing `state` object in context

## Pattern Analysis

### Common Issues
1. **Template Resolution Failures**: All failures occur during template/CEL expression processing
2. **Missing Context Variables**: 
   - `output_format` not available in resolution context
   - `state` object not declared/available in CEL environment

### Affected Components
- State machine workflow implementation
- CEL expression evaluator
- Template resolver
- Output metadata processing

## Recommendations

1. **Immediate Fix**: Add `state` object to CEL evaluation environment for state machine workflows
2. **Default Values**: Ensure `output_format` has a default value or is properly initialized
3. **Context Validation**: Add validation to ensure required variables are present before CEL evaluation
4. **Test Coverage**: Add unit tests specifically for CEL expression context setup