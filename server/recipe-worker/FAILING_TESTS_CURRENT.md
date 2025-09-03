# Current Failing Test Fixtures Status

## Summary
After fixing several test issues, here's the current status of test fixtures.

## Fixed Tests ✅
1. **test-missing-fields** - Fixed recipe structure, now passing
2. **test-sequence-node** - Fixed test expectations (removed newlines, corrected field names)
3. **test-sleep-operation** - Wrapped in sequence to control outputs

## Still Failing Tests ❌

### Currently Failing Recipe Tests:
1. **basic-test** - simple_passthrough
2. **echo-example** - default_echo, custom_message, multiple_repeats, empty_message
3. **inputs** - static_inputs, override_inputs  
4. **minimal_workflow** - basic_greeting, custom_name, empty_name, missing_name
5. **project** - project_definition
6. **recipe** - default_research, custom_topic, limited_sources
7. **simple-echo** - various echo tests
8. **simple-recipe** - basic execution tests
9. **state-machine-composition** - All 4 test cases (nested template resolution bug)
10. **test-command-execution** - command execution tests
11. **test-execute** - execution tests
12. **test-invalid** - validation tests
13. **test-state-machine** - state machine flow tests
14. **unified-data-pipeline** - shared reference not recognized

## Known Functional Bugs Blocking Tests

### High Priority
1. **Nested Template Resolution** - Templates in nested maps don't resolve
   - Blocking: state-machine-composition (all 4 cases)
   
2. **Shared References Not Recognized** - Parser doesn't handle `shared/` operations
   - Blocking: unified-data-pipeline

### Fixed
3. ~~Command Output Newlines~~ ✅ - Now properly strips trailing newlines

## Categories of Failures

### Likely Template/Output Issues
- echo-example tests
- simple-echo tests  
- inputs tests
- minimal_workflow tests

### Likely Missing Operations
- test-command-execution (may need specific command operations)
- test-execute (may need execute operation)

### Known Bugs
- state-machine-composition (nested templates)
- unified-data-pipeline (shared references)

### Need Investigation
- basic-test
- project
- recipe
- simple-recipe
- test-invalid
- test-state-machine

## Next Steps
1. Investigate common patterns in echo/input/workflow failures
2. Check if more operations need registration
3. Fix the two known functional bugs
4. Review each failing test category systematically