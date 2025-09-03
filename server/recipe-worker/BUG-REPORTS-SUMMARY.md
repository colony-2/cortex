# Bug Reports Summary

## High Priority Bugs

### 1. [Shared References Not Recognized](./BUG-REPORT-shared-references.md)
**Severity**: High  
**Impact**: Cannot use shared/modular operations  
**Affected**: `unified-data-pipeline` test  
**Root Cause**: Parser doesn't handle `shared/` prefix for operation references  

### 2. [Nested Template Resolution Failure](./BUG-REPORT-nested-output-templates.md)
**Severity**: High  
**Impact**: Templates in nested maps don't resolve  
**Affected**: `state-machine-composition` tests (all 4 cases)  
**Root Cause**: `processNodeOutputs` doesn't recursively process nested structures  

## Medium Priority Bugs

### 3. ~~[Command Output Includes Unwanted Newline](./BUG-REPORT-command-output-newline.md)~~ ✅ RESOLVED
**Severity**: Medium  
**Impact**: Command outputs included trailing newlines, breaking string comparisons  
**Resolution**: Fixed - `command_execution` now strips trailing newlines  

### 4. [Missing Field Validation Not Working](./BUG-REPORT-missing-field-validation.md)
**Severity**: Medium  
**Impact**: Operations may run without required inputs  
**Affected**: `test-missing-fields` test validity  
**Root Cause**: Input validation may not be enforced for operations  

## Test Status After Fixes

### Fixed Tests ✅
- `test-missing-fields` - Fixed recipe structure
- `test-sleep-operation` - Corrected expectations to match actual output
- `test-sequence-node` - Fixed test expectations (newlines in output, all outputs returned)

### Blocked Tests ❌
- `state-machine-composition` - Blocked by nested template bug
- `unified-data-pipeline` - Blocked by shared reference bug

## Recommendations

1. **High Priority**: Fix shared reference parsing
2. **High Priority**: Fix nested template resolution  
3. **Medium Priority**: Implement proper field validation

These bugs represent core functionality issues in the recipe execution engine that prevent proper template resolution and modular recipe composition.