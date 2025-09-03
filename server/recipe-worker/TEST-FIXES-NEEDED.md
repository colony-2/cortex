# Test Fixes Needed

## Date: 2025-09-03

## Summary
Several test failures are due to test design issues rather than functional bugs.

## Test Design Issues

### 1. test-state-machine
**Problem**: Test expects state machine execution metadata that doesn't exist
- Expects: `final_state`, `transitions_executed`, `error_message`, `path_taken`
- Reality: These aren't exposed by the state machine implementation

**Fix Options**:
1. Redesign test to use actual state outputs:
   ```yaml
   outputs:
     end_result: "{{ states.end.outputs.stdout }}"
     sleep_completed: "{{ states.sleep_state.outputs.completed }}"
   ```
2. Or implement state tracking using variables in the state machine
3. Or if state machine metadata becomes available (e.g., via `scope.state_machine.*`), use that

### 2. test-invalid  
**Problem**: Test expects trailing newline that echo no longer produces
- Expected: `"test\n"`
- Actual: `"test"`

**Fix**: Update test expectations to remove `\n`

### 3. unified-data-pipeline
**Problem**: Incorrect shared operation syntax
- Wrong: `op: shared/data-validator`
- Correct: `shared: data-validator`

**Fix**: Update recipe to use correct syntax for referencing shared operations defined in `defs:`

### 4. error-handling
**Problem**: Malformed YAML indentation
- All state definitions starting at line 17 have incorrect indentation

**Fix**: Properly indent all state definitions under `states:`

## Functional Bug

### Template Interpolation in Nested Outputs
**Problem**: Template expressions in nested objects within outputs are not evaluated
- See BUG-REPORT-template-interpolation-nested-outputs.md for details
- Affects state-machine-composition and potentially other tests

## Next Steps

1. Fix the test design issues listed above
2. Fix the error-handling.yaml indentation
3. The template interpolation bug needs to be fixed in the recipe-worker code itself

## Verification
After fixes:
```bash
go test ./test-fixtures/... -v
```