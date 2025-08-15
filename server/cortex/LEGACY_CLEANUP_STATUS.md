# Legacy Cleanup Status - Remaining TODOs and Deprecations

This document catalogs all remaining TODOs, deprecations, and legacy references found after the legacy removal implementation.

## Active TODOs That Need Implementation

### 1. Output Template Resolution
**Location**: `/Users/jnadeau/src/vibethis/server/recipe-worker/pkg/compiler/compiler.go:75-77`
```go
// TODO: Implement output template resolution for new node format
// For now, return outputs as-is
```
**Status**: 🟡 Pending Implementation  
**Description**: The `processNodeOutputs` function needs to implement template resolution for output mappings in the new node format.

### 2. CEL Expression Evaluation for Conditional Execution
**Location**: `/Users/jnadeau/src/vibethis/server/recipe-worker/pkg/compiler/compiler.go:237-239`
```go
// TODO: Implement CEL evaluation for conditional execution
// For now, always execute
```
**Status**: 🟡 Pending Implementation  
**Description**: The `executeNode` function needs to implement CEL (Common Expression Language) evaluation for the `when` field to support conditional node execution.

### 3. Type Validation
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/internal/shared/validator.go:70`
```go
// TODO: Add type validation when needed
```
**Status**: 🟡 Low Priority  
**Description**: Enhanced type validation for recipe inputs/outputs.

### 4. Output Reference Validation
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/internal/shared/validator.go:83`
```go
// TODO: Validate that output references valid step outputs
```
**Status**: 🟡 Low Priority  
**Description**: Validation to ensure output templates reference valid step outputs.

## Deprecated Functions (Marked for Removal)

### 1. Schema Generation Functions
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/cmd/cortex/schema.go`

```go
// Line 85-87: DEPRECATED: This function is kept for backward compatibility but is no longer used
func generateCompleteSchemaLegacy(registry *worker.ActivityRegistry, filterActivity string, includeVersion bool, includeExamples bool) (map[string]interface{}, error)

// Line 275-276: DEPRECATED: Moved to shared package  
func convertSchemaLegacy(schema interface{}) map[string]interface{}
```
**Status**: 🔴 Should be removed  
**Description**: These legacy schema generation functions should be removed as they're no longer used.

### 2. Input Validation Function
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/cmd/cortex/execute.go:468-470`
```go
// DEPRECATED: validateInputs is now handled by shared.RecipeValidator
func validateInputsLegacy(recipe *yamlpkg.RecipeDefinition, inputs map[string]interface{}) error
```
**Status**: 🔴 Should be removed  
**Description**: Legacy input validation function replaced by shared.RecipeValidator.

## Legacy Format Test Files and Examples

### 1. Test Recipe File
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/test-recipe.yaml:23-38`
```yaml
steps:
  - id: echo_step
    uses: command_execution
    # ... rest of step definition
```
**Status**: 🟡 Needs Migration  
**Description**: This test file still uses the old `steps` format and should be updated to use the new unified format (`sequence`, `parallel`, `states`, or `op`).

### 2. Schema Fix Specification
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/schema-fix-spec-v2.md`
- Contains references to old `Steps` field validation
- Lines 124-135 describe step validation logic that's no longer applicable

**Status**: 🟡 Documentation Update Needed  
**Description**: This specification document contains outdated validation logic and should be updated to reflect the new unified node format.

## Functions Still Calling Legacy Code

### 1. Schema Test Functions
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/cmd/cortex/schema_test.go`
- Lines 19, 49, 69, 77, 88: All call `generateCompleteSchemaLegacy`
- Line 144: Calls `convertSchemaLegacy`

**Status**: 🔴 Needs Update  
**Description**: These test functions should be updated to use the new schema generation methods instead of the deprecated legacy functions.

### 2. Execute Test Function
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/cmd/cortex/execute_test.go:334`
```go
err := validateInputsLegacy(tt.recipe, tt.inputs)
```
**Status**: 🔴 Needs Update  
**Description**: This test should use the new validation method instead of the deprecated legacy function.

## Critical Command Infrastructure Issues

### 1. Schema Command - Broken for New Format
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/cmd/cortex/schema.go` and `/Users/jnadeau/src/vibethis/server/cortex/internal/shared/schema.go`

**Current Issue**: The schema command currently only generates activity schemas, not the recipe schema structure. The shared schema manager is generating schemas for individual activities but not the overall recipe structure that supports the new unified format.

**Required Fix**: The schema command should generate a proper JSON schema for recipes that:
- ✅ Has a `oneOf` for the different node types (`op`, `sequence`, `parallel`, `states`) 
- ✅ Includes proper schemas for each node type
- ✅ References activity types within the `op` nodes
- ❌ **NOT** treat recipes as activities (there is no such thing as a "recipe activity")

**Current Broken Output**: Only shows activity definitions
```json
{
  "definitions": {
    "activities": {
      "command_execution": { ... }
    }
  }
}
```

**Expected Output**: Should include recipe structure schema
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Recipe Schema",
  "type": "object",
  "properties": {
    "name": { "type": "string" },
    "version": { "type": "string" },
    "oneOf": [
      { "$ref": "#/definitions/OpNode" },
      { "$ref": "#/definitions/SequenceNode" },
      { "$ref": "#/definitions/ParallelNode" },
      { "$ref": "#/definitions/StatesNode" }
    ]
  },
  "definitions": {
    "OpNode": { ... },
    "SequenceNode": { ... },
    // etc.
  }
}
```

**Status**: 🔴 **CRITICAL - Command completely broken for new format**

### 2. Validate Command - Partially Updated
**Location**: `/Users/jnadeau/src/vibethis/server/cortex/cmd/cortex/validate.go`

**Status Assessment Needed**: The validate command may have similar issues to the schema command. Investigation needed to confirm if:
- ✅ Validation works with new unified format (`sequence`, `parallel`, `states`, `op`)
- ❌ Still validates against old `steps` format
- ❌ Schema validation uses outdated schemas

**Required Investigation**: Check if the validate command properly:
1. Validates the new node structure
2. Rejects old `steps` format 
3. Uses updated schemas for validation
4. Properly validates nested nodes

**Status**: 🟡 **Needs Investigation**

## Summary

### Immediate Actions Required:
1. **🔴 CRITICAL: Fix schema command** - Generate proper recipe schema with unified format support
2. **🟡 Investigate validate command** - Ensure it works with new format and rejects old format
3. **🔴 Remove deprecated functions** - Clean up the 3 deprecated functions and update their callers
4. **🔴 Update test files** - Migrate `test-recipe.yaml` to new format and update test function calls
5. **🟡 Implement missing features** - CEL evaluation and output template resolution for complete functionality

### Priority Ranking:
1. **CRITICAL**: Fix schema command (core infrastructure broken)
2. **High**: Investigate/fix validate command (core infrastructure)
3. **High**: Remove deprecated code and update tests (technical debt cleanup)
4. **Medium**: Implement output template resolution (functionality gap)
5. **Medium**: Implement CEL evaluation for conditional execution (functionality gap)
6. **Low**: Enhanced validation features (nice-to-have improvements)

### Migration Status:
- ✅ Core legacy removal completed
- ✅ Template expansion working
- ✅ All integration tests passing  
- 🔴 **Schema command broken** - doesn't generate recipe schemas for new format
- 🟡 Validate command status unknown
- 🟡 Some deprecated code remains
- 🟡 Some test files need format updates
- 🟡 Two TODO features pending implementation

The legacy removal implementation is functionally complete for workflow execution, but **critical command infrastructure (schema/validate) needs immediate attention** to support the new unified format.