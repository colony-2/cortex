# Backwards Compatibility Documentation

## Overview
This document lists all backwards compatibility functionality that exists in the codebase after implementing the recipe-node-simplification-spec-v20. This compatibility layer ensures that existing systems continue to function while we migrate to the new unified node format.

## Legacy Types and Fields

### 1. `Steps` Field in RecipeDefinition
**Location:** `/server/recipe-core/pkg/yaml/types.go`

**What it is:**
```yaml
type RecipeDefinition struct {
    // ...
    Steps []Step `yaml:"steps,omitempty"` // Legacy: use sequence or parallel instead
}
```

**Why it exists:**
- The state machine compiler (`/server/recipe-worker/pkg/compiler/statemachine/`) still expects the `Steps` field
- Allows gradual migration of existing recipes that use the old `steps:` format
- Prevents breaking existing workflows that haven't been converted yet

### 2. Legacy Types in `legacy_compat.go`
**Location:** `/server/recipe-core/pkg/yaml/legacy_compat.go`

**What it contains:**
- `Step` - The old step structure with `uses`, `config`, etc.
- `SharedActivity` - The old shared activity definition
- `ParallelSpec` - The old parallel execution specification
- `LoopSpec` - The old loop specification
- `ConditionalSpec` - The old conditional execution
- `StateMachineConfig` - The old state machine configuration
- `StateMachineState` - Individual state definitions
- `Transition` - State transitions
- `StepRetryPolicy` - Retry configuration for steps

**Why it exists:**
- The state machine compiler requires these types to compile workflows
- Components like `worker_manager.go` need to convert between new `Node` types and old `SharedActivity` types
- Allows the system to process both old and new recipe formats during the transition period

### 3. SharedActivity Conversion in WorkerManager
**Location:** `/server/recipe-worker/pkg/worker/worker_manager.go`

**What it does:**
Converts new `Node` format to `SharedActivity` for compatibility:
```go
sharedActivity := &yamlpkg.SharedActivity{
    Uses:   sharedNode.Op,
    Config: sharedNode.Inputs,
}
```

**Why it exists:**
- The activity registry and worker system were built expecting `SharedActivity` types
- Allows new recipes using `op:` and `inputs:` to work with the existing worker infrastructure
- Provides a bridge between new and old formats without rewriting the entire worker system

### 4. Compiler Compatibility
**Location:** `/server/recipe-worker/pkg/compiler/`

**What it handles:**
- The compiler's `ExecuteWorkflow` method expects `Steps` field
- `processOutputs` was updated to handle both old `[]OutputDefinition` and new `map[string]interface{}`

**Why it exists:**
- The compiler is used by the state machine workflow execution
- Many existing recipes rely on the compiler's workflow execution logic
- Gradual migration path - allows using either compiled workflows or dynamic workflows

### 5. Input Validation Compatibility
**Location:** `/server/cortex/internal/shared/validator.go` and `/server/cortex/cmd/cortex/execute.go`

**What changed:**
- Added `InputSchema` field to support input definitions with the new format
- `InputDef` type provides schema information (required, default, type)
- Validation functions updated to check both old and new formats

**Why it exists:**
- Input validation is critical for recipe execution
- Default values need to be applied for missing inputs
- Required input checking prevents runtime errors
- Maintains the same validation capabilities as the old system

## Migration Path

### Current State
The system currently supports both formats:

**Old Format:**
```yaml
steps:
  - id: step1
    uses: activity-name
    config:
      type: function
    inputs:
      data: value
```

**New Format:**
```yaml
sequence:
  - id: step1
    op: activity-name
    inputs:
      type: function
      data: value
```

### Why Both Formats Work
1. **Parser Flexibility:** The YAML parser accepts both structures
2. **Runtime Detection:** The workflow executor checks for both `Steps` (old) and `Sequence/Op/Parallel/States` (new)
3. **Automatic Conversion:** Where needed, old structures are converted to new ones internally

## Components Still Using Legacy Format

### 1. State Machine Compiler
- **Location:** `/server/recipe-worker/pkg/compiler/statemachine/`
- **Status:** Still requires `Steps` and related legacy types
- **Impact:** Recipes using state machines must maintain legacy compatibility

### 2. Some Test Fixtures
- **Location:** Various test files
- **Status:** Converted to new format but some still test legacy compatibility
- **Impact:** Ensures both formats continue to work

## Removal Timeline

The backwards compatibility layer should be removed when:

1. **State Machine Compiler is Updated:** Convert to use the new `States` node type
2. **All Production Recipes Migrated:** Ensure all recipes in production use the new format
3. **Worker System Updated:** Refactor to work directly with `Node` types instead of `SharedActivity`
4. **Documentation Updated:** All examples and docs use the new format

## Testing Backwards Compatibility

To ensure backwards compatibility:

1. **Mixed Format Tests:** Tests exist that use both old and new formats
2. **Integration Tests:** Verify that old recipes still execute correctly
3. **Conversion Tests:** Ensure proper conversion between formats

## Benefits of This Approach

1. **Zero Downtime Migration:** Systems can continue running during the transition
2. **Gradual Adoption:** Teams can migrate recipes at their own pace
3. **Rollback Safety:** If issues arise, old format recipes still work
4. **Testing Confidence:** Both formats are tested, ensuring stability

## Next Steps for Full Migration

1. Update state machine compiler to use new `States` node type
2. Create migration tool to automatically convert old recipes
3. Update all internal tooling to generate new format
4. Set deprecation timeline for old format
5. Remove legacy code once all systems are migrated