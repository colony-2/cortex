# Bug Report: State Machine Terminal State Outputs Not Implemented

**Date**: 2025-01-02  
**Reporter**: Claude Code Assistant  
**Severity**: High (Core feature missing)  
**Status**: Open  

## Summary

State machines with terminal states that define custom outputs are not returning those outputs. Instead, all state machines return a generic `"Executed state_machine"` result, regardless of their terminal state output definitions.

## Reproduction Steps

1. Create a state machine recipe with terminal states that define custom outputs
2. Execute the recipe  
3. Observe that the actual output is `{"result":"Executed state_machine", "status":"success"}` instead of the terminal state outputs

## Minimal Example

**Recipe** (`state-machine-composition.yaml`):
```yaml
id: basic_state_machine_test
op: state_machine
config:
  initial_state: intake
  states:
    final_state:
      terminal: true
      outputs:
        result: '{{ states.processing.outputs.validate_input.items }}'
        metadata:
          analysis_type: '{{ inputs.analysis_depth }}'
          output_format: '{{ inputs.output_format }}'
```

**Test Input**:
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
  "result": "Executed state_machine",
  "status": "success"
}
```

## Root Cause Analysis

### Evidence from Existing Tests

The issue is system-wide. Both test suites show the same pattern:

**simple-state-machine.test.yaml** expects:
```yaml
want:
  result: Executed state_machine
  status: success
```

**state-machine-composition.test.yaml** fails because it expects custom outputs but gets the same generic result.

This suggests the state machine execution engine is not processing terminal state outputs at all.

### Code Investigation Needed

The issue likely lies in the state machine execution logic in:
- `/Users/jnadeau/src/colony2/server/recipe-worker/pkg/compiler/compiler.go` - `executeStateMachine()` function
- State machine execution may not be checking for terminal states with custom outputs
- Terminal state output templates may not be resolved and returned

## Impact

- **Immediate**: `state-machine-composition` test suite fails (4 test cases)
- **Functional**: State machines cannot return custom results, limiting their usefulness
- **User Experience**: State machine recipes cannot provide meaningful output data

## Files Affected

- `/Users/jnadeau/src/colony2/server/recipe-worker/test-fixtures/recipes/state-machine-composition.yaml` - recipe with terminal outputs
- `/Users/jnadeau/src/colony2/server/recipe-worker/test-fixtures/recipes/state-machine-composition.test.yaml` - failing tests
- `/Users/jnadeau/src/colony2/server/recipe-worker/test-fixtures/recipes/simple-state-machine.test.yaml` - currently expects generic output

## Expected Fix

The state machine execution should:
1. Detect when a terminal state is reached
2. Process any `outputs:` definitions in that terminal state  
3. Resolve template expressions in those outputs
4. Return the resolved outputs as the recipe result instead of the generic message

## Testing

To verify the fix works:
```bash
go test -v ./... -run TestAllRecipes/state-machine-composition
```

Should pass with all 4 test cases returning the expected custom outputs.

## Related Documentation

State machine terminal outputs appear to be documented/expected functionality based on:
- Recipe YAML structure allowing `outputs:` in terminal states
- Test expectations for custom output structures
- Template resolution patterns used in terminal state outputs