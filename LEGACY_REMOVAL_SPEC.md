# Legacy Code Removal Specification

## Overview
This specification outlines the systematic removal of all legacy code dependencies introduced for backwards compatibility with the old recipe format. The migration will be done component by component, with testing at each step to ensure no functionality is broken.

## Migration Principles
1. **Component Isolation:** Update one component at a time
2. **Test After Each Change:** Run `moon :test` and `moon :integration` after each component
3. **No Breaking Changes:** Maintain functionality throughout the migration
4. **Remove Dead Code:** Delete legacy code only after all dependencies are removed

## Phase 1: State Machine Compiler Migration

### Component: `/server/recipe-worker/pkg/compiler/statemachine/`

#### Current State
- Uses `Steps []Step` field
- Expects `StateMachineConfig` type
- Processes `Transition` structures

#### Migration Steps

1. **Update State Machine Compiler to Use `States` Node Type**
   ```go
   // Old: Process recipe.Steps with StateMachineConfig
   // New: Process recipe.States directly
   
   func (c *StateMachineCompiler) CompileStateMachine(recipe *yamlpkg.RecipeDefinition) {
       if recipe.States != nil {
           // Process States node type
           initial := recipe.States.Initial
           states := recipe.States.States
           // ... compile state machine
       }
   }
   ```

2. **Update State Processing**
   - Change from `StateMachineState` to `State` type
   - Update transition handling to use new `Transition` structure
   - Remove dependency on `Step` type

3. **Testing Checkpoint**
   ```bash
   cd /Users/jnadeau/src/vibethis
   moon run recipe-worker:test
   # Verify: All state machine tests pass
   ```

## Phase 2: Worker Manager Migration

### Component: `/server/recipe-worker/pkg/worker/worker_manager.go`

#### Current State
- Converts `Node` to `SharedActivity`
- Uses compatibility layer for activity registration

#### Migration Steps

1. **Update Activity Registration to Use Node Directly**
   ```go
   // Old: Convert Node to SharedActivity
   // New: Register Node directly
   
   func (m *WorkerManager) RegisterSharedNode(name string, node *yamlpkg.Node) {
       m.activityRegistry.RegisterNode(name, node)
       m.activityRegistry.RegisterNode("shared/"+name, node)
   }
   ```

2. **Update Activity Type Registry**
   - Modify `ActivityTypeRegistry` to work with `Node` instead of `SharedActivity`
   - Update metadata extraction to use `node.Op` instead of `activity.Uses`

3. **Testing Checkpoint**
   ```bash
   moon run recipe-worker:test
   # Verify: Worker tests pass with new Node-based registration
   ```

## Phase 3: Compiler Migration

### Component: `/server/recipe-worker/pkg/compiler/compiler.go`

#### Current State
- `ExecuteWorkflow` expects `Steps` field
- Falls back to legacy step processing

#### Migration Steps

1. **Update ExecuteWorkflow to Process New Format**
   ```go
   func (c *Compiler) ExecuteWorkflow(ctx workflow.Context, def *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
       // Remove Steps processing
       // Add support for all node types
       if def.Op != "" {
           return c.executeOperation(ctx, def, inputs)
       } else if len(def.Sequence) > 0 {
           return c.executeSequence(ctx, def.Sequence, inputs)
       } else if len(def.Parallel) > 0 {
           return c.executeParallel(ctx, def.Parallel, inputs)
       } else if def.States != nil {
           return c.executeStateMachine(ctx, def.States, inputs)
       }
   }
   ```

2. **Remove Step Processing Methods**
   - Delete `processStep()` method
   - Delete `executeStep()` method
   - Update to use `executeNode()` pattern

3. **Testing Checkpoint**
   ```bash
   moon run recipe-worker:test
   # Verify: Compiler tests pass with new format
   ```

## Phase 4: Test Fixture Migration

### Component: All test files using legacy format

#### Migration Steps

1. **Identify Tests Using Legacy Format**
   ```bash
   grep -r "uses:" --include="*.go" /Users/jnadeau/src/vibethis/server/recipe-worker/
   grep -r "config:" --include="*.go" /Users/jnadeau/src/vibethis/server/recipe-worker/
   ```

2. **Update Test Recipes**
   - Change `uses:` to `op:`
   - Change `config:` to merge into `inputs:`
   - Update assertions to expect new format

3. **Testing Checkpoint**
   ```bash
   moon :test
   # Verify: All tests still pass after test updates
   ```

## Phase 5: Recipe Definition Cleanup

### Component: `/server/recipe-core/pkg/yaml/types.go`

#### Migration Steps

1. **Remove Steps Field from RecipeDefinition**
   ```go
   type RecipeDefinition struct {
       // Metadata
       Name        string `yaml:"name"`
       Description string `yaml:"description,omitempty"`
       Version     string `yaml:"version"`
       
       // Shared node definitions
       Shared map[string]Node `yaml:"shared,omitempty"`
       
       // Root node (one of these four)
       Op       string      `yaml:"op,omitempty"`
       Sequence []Node      `yaml:"sequence,omitempty"`
       Parallel []Node      `yaml:"parallel,omitempty"`
       States   *StateMap   `yaml:"states,omitempty"`
       
       // Node properties
       ID          string                 `yaml:"id,omitempty"`
       Desc        string                 `yaml:"desc,omitempty"`
       Inputs      map[string]interface{} `yaml:"inputs,omitempty"`
       Outputs     map[string]interface{} `yaml:"outputs,omitempty"`
       InputSchema map[string]InputDef    `yaml:"input_schema,omitempty"`
       Timeout     string                 `yaml:"timeout,omitempty"`
       Retry       *RetryPolicy           `yaml:"retry,omitempty"`
       
       // Steps field REMOVED
   }
   ```

2. **Testing Checkpoint**
   ```bash
   moon run recipe-core:test
   # Verify: Core tests pass without Steps field
   ```

## Phase 6: Legacy File Removal

### Component: `/server/recipe-core/pkg/yaml/legacy_compat.go`

#### Prerequisites
- All previous phases must be complete
- All tests must be passing

#### Migration Steps

1. **Verify No Legacy Type Usage**
   ```bash
   # Check for any remaining usage of legacy types
   grep -r "SharedActivity" --include="*.go" /Users/jnadeau/src/vibethis/server/
   grep -r "StateMachineConfig" --include="*.go" /Users/jnadeau/src/vibethis/server/
   grep -r "ParallelSpec" --include="*.go" /Users/jnadeau/src/vibethis/server/
   ```

2. **Delete Legacy File**
   ```bash
   rm /Users/jnadeau/src/vibethis/server/recipe-core/pkg/yaml/legacy_compat.go
   ```

3. **Final Testing**
   ```bash
   moon :test
   moon :integration
   # Verify: All tests pass without legacy types
   ```

## Phase 7: Documentation Update

### Components: All documentation files

#### Migration Steps

1. **Update Recipe Examples**
   - Remove all examples using old format
   - Ensure all documentation uses new format

2. **Update Migration Guide**
   - Mark old format as fully deprecated
   - Remove backwards compatibility mentions

3. **Delete Backwards Compatibility Doc**
   ```bash
   rm /Users/jnadeau/src/vibethis/BACKWARDS_COMPATIBILITY.md
   ```

## Validation Checklist

After each phase, verify:

- [ ] Component builds successfully: `go build ./...`
- [ ] Unit tests pass: `go test ./...`
- [ ] Integration tests pass: `moon :integration`
- [ ] Full test suite passes: `moon :test`
- [ ] No regression in functionality

YOU MUST NOT proceed to next phase until previous phase is fully validated

## Success Criteria

The migration is complete when:

1. ✅ All components use the new unified node format
2. ✅ No references to legacy types remain
3. ✅ `legacy_compat.go` is deleted
4. ✅ All tests pass without legacy code
5. ✅ Documentation reflects only the new format
