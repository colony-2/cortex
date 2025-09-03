# Failing Tests Analysis

## Current Status
Most tests are now passing. Remaining issues are documented below.

## Key Patterns and Fixes

### 1. Output Mapping Syntax
**Pattern:** Remove `outputs` prefix in output field references
- Incorrect: `outputs.field`
- Correct: `.field`

### 2. CEL Function Usage
**Pattern:** Use correct CEL functions
- Use `size()` not `len()` for array/map length
- Use `has(inputs.field)` to check field existence before accessing
- Default values: `{{ has(inputs.field) ? inputs.field : "default" }}`

### 3. Recipe Structure Rules
- State machines use `state:` at root level, never `op: state_machine`
- `states:` was incorrect, use `state:` instead
- Recipe outputs defined at root level: `outputs:` as sibling of `state:`/`sequence:`
- State machine outputs in `state:` block are not implemented for individual states
- Operations can only be: `op:`, `sequence:`, `state:`, or shared reference

### 4. Activity Output Fields
Test activities return specific fields:
- `echo_activity`: Returns `output` field (not `result`)
- `command_execution`: Returns `result` and `stdout` (not `exit_code` or `stderr`)
- `batch_validator`: Returns `valid`, `count`, `items`
- `context_logger`: Returns `logged: true`
- Activities wrap outputs in step ID when in sequence

### 5. Template Resolution
- Templates in nested metadata structures may not resolve (known bug)
- State machine outputs defined at recipe level work
- Use `states.statename.outputs.field` to reference state outputs (not currently working)
- Sequence node references: `{{ sequence.node_id.outputs.field }}`

### 6. Parse-Time vs Runtime Errors
- Unknown operations fail at parse time, not runtime
- Parse errors tested in `basic_parser_test.go`, not in recipe test framework
- Test framework requires valid parseable recipes
- Don't register test activities with invalid names like `unknown_activity_type`

### 7. Default Input Handling
- Recipe inputs with defaults still need `has()` check in templates
- Pattern: `{{ has(inputs.message) ? inputs.message : "default" }}`

## Known Bugs (Documented)
1. **State machine terminal outputs** - Terminal state outputs not returned (`BUG-REPORT-state-machine-outputs.md`)
2. **Nested template resolution** - Templates in nested maps don't resolve (`BUG-REPORT-nested-output-templates.md`)

## Remaining Failing Tests

### test-missing-fields
- Tests required field validation
- May need adjustment for new validation rules

### test-sequence-node
- Tests sequence execution timing
- May have timeout issues

### test-sleep-operation  
- Tests sleep/delay operations
- May have timing expectations to adjust

## Test Categories

### Completed
- Simple recipes without complex features
- Basic sequences and operations
- State machines (with workarounds for output limitations)
- Template features (except nested resolution bug)
- Command execution (adjusted for test activity limitations)
- Invalid operation testing (moved to parse-time validation)

### Deleted/Moved
- Parallel execution tests (feature removed)
- Sub-recipe operations (moved to `/server/ops/test-recipes/`)

### Special Cases
- `test-invalid`: Converted to placeholder since unknown ops fail at parse time
- Default value handling uses `has()` function pattern
- State machine outputs use recipe-level outputs, not terminal state outputs