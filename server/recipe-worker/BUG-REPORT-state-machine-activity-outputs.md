# Bug Report: State Machine Loses Terminal State Activity Outputs

**Date**: 2025-01-03  
**Reporter**: Investigation via test suite  
**Severity**: High (Data loss in state machine execution)  
**Status**: Confirmed  

## Summary

State machines with terminal states that execute activities (NodeOp) lose the activity output. The `executeStateMachine()` function returns an empty map instead of the activity's execution result.

## Reproduction Steps

1. Create a state machine recipe with a terminal state that executes an activity:
```yaml
id: state_outputs_test
desc: Test state machine outputs functionality
version: 1.0.0
inputs:
  message: 'test'
state:
  initial: simple_state
  states:
    simple_state:
      op: echo_activity
      inputs:
        message: 'Hello {{ inputs.message }}'
outputs:
  result: '{{ inputs.message }}'
```

2. Execute the recipe
3. Observe that `executeStateMachine()` returns `{}` instead of the activity output

## Expected Behavior

The state machine should return the terminal state's activity output, e.g.:
```json
{
  "message": "Hello test",
  "status": "success"
}
```

## Actual Behavior

The state machine returns an empty map: `{}`

## Root Cause Analysis

### Location
`pkg/compiler/statemachine_compiler.go:73-107` in the `executeStateMachine()` function

### Code Flow
1. State machine executes the terminal state activity successfully
2. The activity output is stored in `resCtx.TemplateData.States[currentState].Outputs`
3. When determining what to return (lines 73-107):
   - Lines 76-81: Check if terminal state is `NodeState` or `NodeSequence` (it's not - it's `NodeOp`)
   - Line 84: Check for output templates - none exist on `NodeOp`
   - Lines 97-99: Try to return `resCtx.TemplateData.States[currentState].Outputs`
   - **BUG**: This returns `nil` because the activity outputs weren't properly stored

### The Problem
The activity execution result is stored but not retrieved correctly:
- Line 57: `resCtx.AddStateOutput(currentState, stateOutputs)` stores the output
- Line 98: `return lastState.Outputs, nil` tries to retrieve it
- But `lastState.Outputs` is the stored value, which may be empty or improperly structured

## Impact

### Working Scenarios
- Recipe-level outputs that reference `inputs` work (due to `processNodeOutputs()` compensation)
- State machines with `NodeState` or `NodeSequence` terminal states work

### Broken Scenarios  
- Recipe outputs that reference state outputs fail: `{{ states.simple_state.outputs.result }}`
- Direct state machine execution returns empty results
- Activity outputs are lost and cannot be accessed in recipe-level templates

## Test Evidence

Test output from `TestStateOutputsDebug`:
```
Test 1: Direct state machine execution
executeStateMachine returns: map[]
❌ PROBLEM: State machine returns empty map!
   Expected: The echo_activity output or something meaningful

Test 2: Full recipe execution (through ExecuteRecipe)
ExecuteRecipe returns: map[result:test]
✓ Recipe outputs work: result = test
  This is because processNodeOutputs() processes the recipe-level outputs
```

## Proposed Fix

In `executeStateMachine()`, when the terminal state is a `NodeOp`:
1. Check if the state output was stored correctly in `resCtx.TemplateData.States`
2. Return the actual activity execution output
3. Ensure the output is properly structured for downstream processing

### Specific Code Location to Fix
```go
// Line 97-99 in statemachine_compiler.go
if lastState, ok := resCtx.TemplateData.States[currentState]; ok {
    return lastState.Outputs, nil  // This returns nil/empty
}
```

Should properly return the activity outputs that were stored at line 57.

## Workaround

Currently, users can only reference `inputs` in recipe-level outputs, not state outputs:
- ✅ Works: `outputs: { result: '{{ inputs.message }}' }`  
- ❌ Broken: `outputs: { result: '{{ states.simple_state.outputs.result }}' }`

## Test Files

- `/pkg/compiler/state_outputs_debug_test.go` - Demonstrates the issue
- `/pkg/compiler/state_machine_outputs_test.go` - Test suite for state outputs

## Priority

High - This breaks a fundamental expectation that state machine outputs are accessible, limiting the ability to chain state outputs or use them in recipe-level templates.