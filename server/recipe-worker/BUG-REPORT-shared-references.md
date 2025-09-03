# Bug Report: Shared References Not Recognized

**Date**: 2025-09-03  
**Reporter**: Claude Code Assistant  
**Severity**: High  
**Status**: Open  

## Summary

Shared operation references (e.g., `shared/data-validator`) are not being recognized by the recipe parser, causing parse errors even though the recipe structure follows documented patterns.

## Reproduction

**Recipe** (`unified-data-pipeline.yaml`):
```yaml
id: unified-data-pipeline
desc: Unified data processing pipeline
version: '1.0'
sequence:
- id: validate
  op: shared/data-validator  # This fails
  inputs:
    data: '{{ inputs.raw_data }}'
```

**Error**:
```
unknown op: [shared/data-validator] at [11:3]
```

## Expected Behavior

According to recipe structure rules, nodes can be:
- `op:` (for operations)
- `sequence:` (for sequences)
- `state:` (for state machines)
- **Shared references** (like `shared/operation-name`)

Shared references should be recognized and resolved by the parser.

## Actual Behavior

The parser throws an "unknown op" error when encountering shared references, treating them as invalid operations rather than references to shared/reusable operations.

## Impact

- Cannot use shared/reusable operations in recipes
- Recipes that follow modular design patterns fail to parse
- Test `unified-data-pipeline` cannot run

## Root Cause Analysis

The parser appears to validate operation names against a fixed list of known operations and doesn't have logic to handle the `shared/` prefix or resolve shared operation references.

## Workaround

None available - shared references are a core feature for recipe modularity.

## Files Affected

- `/Users/jnadeau/src/vibethis/server/recipe-worker/test-fixtures/recipes/unified-data-pipeline.yaml`
- Recipe parser logic that validates operation names
- Shared operation resolution logic (missing or not integrated)

## Suggested Fix

1. Update parser to recognize `shared/` prefix as a special case
2. Implement shared operation resolution logic
3. Load shared operations from a registry or definition file
4. Validate that referenced shared operations exist