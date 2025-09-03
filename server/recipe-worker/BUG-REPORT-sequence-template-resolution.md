# Bug Report: Sequence Template Resolution Failure

**Date**: 2025-09-02  
**Reporter**: Claude Code Assistant  
**Severity**: High (Breaks documented functionality)  
**Status**: Unresolved  

## Summary

The documented template pattern `{{ sequence.node_id.outputs.field }}` fails with "no such key: node_id" when used in sequence node input templates, despite being explicitly documented as supported in `TEMPLATE_REFERENCE_CHEATSHEET.md`.

## Reproduction Steps

1. Create a recipe with a sequence containing multiple nodes
2. Have the second node reference the first node's output using `{{ sequence.first_node.outputs.field }}`
3. Execute the recipe
4. Observe "no such key: first_node" error

## Minimal Example

**Recipe** (`simple_workflow.yaml`):
```yaml
id: simple_research
sequence:
- id: search
  op: quick_search_activity
  inputs:
    query: '{{ inputs.query }}'
- id: summarize
  op: summarize_activity
  inputs:
    data: '{{ sequence.search.outputs.result }}'  # FAILS HERE
```

**Error**:
```
sequence node 1 failed: failed to resolve template in input data: 
failed to evaluate CEL expression: no such key: search
```

## Expected Behavior

According to `TEMPLATE_REFERENCE_CHEATSHEET.md` line 33:
```yaml
# Sequence node output (sibling within same sequence)
value: "{{ sequence.node_id.outputs.field_name }}"
```

This pattern should work for referencing completed sibling nodes within the same sequence.

## Actual Behavior

Template resolution fails because the `search` node is not available in the CEL evaluation context when the `summarize` node tries to resolve its input templates.

## Root Cause Analysis

### Investigation Results

1. **Unit tests pass**: The pattern works in isolated unit tests (`TestEvaluateCEL_Conditions` passes)
2. **Integration tests fail**: The pattern fails in actual recipe execution
3. **Context issue**: The resolution context is not properly shared/maintained during sequence execution

### Code Analysis

**Location**: `/Users/jnadeau/src/vibethis/server/recipe-worker/pkg/compiler/compiler.go`  
**Function**: `innerSequence()` (lines 185-230)

**Issue**: In the sequence execution loop:
1. Line 209: Template resolution happens for node N
2. Line 218: Node N executes  
3. Line 228: `resCtx.AddSequenceNode()` adds node N to context
4. Loop continues to node N+1
5. Line 209: Template resolution for node N+1 fails because node N wasn't in context during step 1

**Attempted Fix**: Moved `AddSequenceNode()` call immediately after execution (line 228) but error persists, suggesting deeper context isolation issues.

## Impact

- **Immediate**: `simple_workflow` test fails
- **Broader**: All sequences using inter-node references fail
- **Documentation**: Creates gap between documented and actual functionality

## Evidence

### Working Unit Test
```go
// From template_resolver_test.go:155
ctx.AddSequenceNode("validate", map[string]interface{}{
    "valid": true,
    "score": 0.9,
})
// This works:
expression: "sequence.validate.outputs.valid == true"
```

### Failing Integration
```
# Error from test execution:
workflow execution error: failed to evaluate CEL expression: no such key: search
```

### Context Structure
```go
// From template_resolver.go:14
type TemplateData struct {
    Sequence map[string]NodeOutput  `json:"sequence"` // Should contain node outputs
    // ...
}

// AddSequenceNode correctly populates this, but context seems isolated
```

## Additional Investigation Needed

1. **Context Sharing**: How is `ResolutionContext` passed between sequence execution and template resolution?
2. **Workflow Isolation**: Does `executeCompositeInEnvelope()` create context isolation that prevents sharing?
3. **Alternative Patterns**: Are there other working patterns for inter-node references?

## Workaround

Currently, sequences must use static values instead of inter-node references:

```yaml
# Instead of:
data: '{{ sequence.search.outputs.result }}'
# Use:
data: static_value
```

## Related Files

- `/Users/jnadeau/src/vibethis/server/recipe-worker/TEMPLATE_REFERENCE_CHEATSHEET.md` (Documents the pattern)
- `/Users/jnadeau/src/vibethis/server/recipe-worker/pkg/compiler/compiler.go` (Sequence execution)  
- `/Users/jnadeau/src/vibethis/server/recipe-worker/pkg/compiler/template_resolver.go` (Context management)
- `/Users/jnadeau/src/vibethis/server/recipe-worker/test-fixtures/recipes/simple_workflow.yaml` (Failing test case)

## Next Steps

1. Investigate context isolation in `executeCompositeInEnvelope()`
2. Verify `ResolutionContext` sharing between template resolution and sequence execution
3. Consider if sequence execution creates new contexts that don't inherit node data
4. Add integration tests specifically for sequence template resolution
5. Update documentation if pattern is intentionally unsupported

## Testing

To verify fix works:
```bash
go test -v ./... -run TestAllRecipes/simple_workflow/basic_research
```

Should pass without "no such key" errors.