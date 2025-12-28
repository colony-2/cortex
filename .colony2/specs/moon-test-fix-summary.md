# Moon Integration Test Fix Summary

## Issue
The Moon integration tests (`moon run cortex:test`) were failing after implementing the new automated schema generation system.

## Root Cause
The test `TestGenerateCompleteSchema` in `cmd/cortex/schema_test.go` was expecting operation definitions like `SleepOperation` and `CommandExecutionOperation` to be present in the schema's `definitions` section. The new automated generator wasn't adding these operation definitions to maintain backward compatibility.

## Solution
Modified the `generateAllDefinitions()` function in `/server/cortex/pkg/schema/generator.go` to explicitly add operation schemas to the definitions:

```go
// Operation types - add these for backward compatibility
g.definitions["SleepOperation"] = g.generateOperationSchema(&SleepOperation{})
g.definitions["CommandExecutionOperation"] = g.generateOperationSchema(&CommandExecutionOperation{})
g.definitions["LLMInferenceOperation"] = g.generateOperationSchema(&LLMInferenceOperation{})
g.definitions["GitShallowCloneOperation"] = g.generateOperationSchema(&GitShallowCloneOperation{})
g.definitions["RecipeOperation"] = g.generateOperationSchema(&RecipeOperation{})
g.definitions["InputOperation"] = g.generateOperationSchema(&InputOperation{})
```

Also updated `generateNodeOperationSchema()` to reference these definitions instead of creating duplicates.

## Verification
- All Go tests pass: `go test ./...` ✅
- Moon integration tests pass: `moon run cortex:test` ✅
- Schema generation works correctly:
  - Full recipe schema: `./cortex schema --format json` ✅
  - Activity-specific schema: `./cortex schema --activity command_execution` ✅
  - Operation definitions are present in output ✅

## Files Modified
- `/server/cortex/pkg/schema/generator.go` - Added operation definitions for backward compatibility

## Impact
The fix maintains backward compatibility while preserving all the benefits of the new automated schema generation system. Tests that depend on the presence of operation definitions in the schema now pass without requiring any changes to the test code itself.