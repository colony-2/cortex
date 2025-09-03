# Bug Report: Command Execution Output Includes Unwanted Newline

**Date**: 2025-09-03  
**Reporter**: Claude Code Assistant  
**Severity**: Medium  
**Status**: RESOLVED  
**Type**: Functional Bug  
**Resolution**: Fixed - command_execution now strips trailing newlines using `strings.TrimRight(stdout.String(), "\r\n")`  

## Summary

The `command_execution` operation returns stdout with trailing newline characters included, which is unexpected behavior for output processing. When executing `echo 'first step'`, the output is `"first step\n"` instead of `"first step"`.

## Reproduction

**Recipe**:
```yaml
sequence:
- id: step1
  op: command_execution
  inputs:
    run: echo 'first step'
    shell: bash
outputs:
  command_output: '{{ sequence.step1.outputs.stdout }}'
```

**Expected Output**:
```json
{
  "command_output": "first step"
}
```

**Actual Output**:
```json
{
  "command_output": "first step\n"
}
```

## Impact

1. **String Comparisons Fail**: Tests and conditional logic expecting `"first step"` fail when the actual value is `"first step\n"`
2. **Template Concatenation Issues**: When combining outputs, unwanted newlines appear in the middle of strings
3. **Data Processing**: Downstream operations may not expect trailing whitespace
4. **Test Brittleness**: Tests must account for platform-specific line endings

## Root Cause Analysis

The `command_execution` activity appears to capture the raw stdout from the shell command, including any trailing newlines that commands like `echo` produce. The activity doesn't strip trailing whitespace before returning the output.

## Expected Behavior

Command outputs should have trailing newlines stripped by default, similar to how most command substitution works in shells:
- Bash `$(echo 'test')` strips the newline
- Most programming languages' command execution utilities strip trailing newlines
- Users expect the content, not the formatting characters

## Current Workaround

Tests must expect the newline:
```yaml
want:
  command_output: "first step\n"  # Must include \n
```

Or use string manipulation in templates:
```yaml
outputs:
  command_output: '{{ strings.TrimSpace(sequence.step1.outputs.stdout) }}'
```

## Proposed Solution

1. **Option A**: Strip trailing newlines from stdout by default in `command_execution`
   - Pros: Matches user expectations, cleaner outputs
   - Cons: Breaking change for existing recipes that expect newlines

2. **Option B**: Add a `strip_newlines` option to command_execution
   ```yaml
   op: command_execution
   inputs:
     run: echo 'first step'
     strip_newlines: true  # Default could be true or false
   ```

3. **Option C**: Provide both `stdout` and `stdout_stripped` fields
   ```yaml
   outputs:
     stdout: "first step\n"        # Raw output
     stdout_stripped: "first step" # Trimmed output
   ```

## Affected Components

- `command_execution` activity implementation
- Any recipes using command outputs in string comparisons
- Test fixtures expecting command outputs

## Test Cases

```go
// Should pass after fix
assert.Equal(t, "first step", commandOutput)

// Should not need to include newline
assert.NotEqual(t, "first step\n", commandOutput)
```

## Priority

Medium - This is a quality-of-life issue that makes recipes more complex than necessary and causes confusion in test expectations. While workarounds exist, the current behavior is unintuitive and doesn't match common conventions.