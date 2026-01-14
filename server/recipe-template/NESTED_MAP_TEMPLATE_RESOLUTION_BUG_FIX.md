# Bug Fix: Template Resolution for recipe.InputMap

## Issue Summary

**Bug Confirmed**: Templates inside `recipe.InputMap` were NOT being resolved.

### The Problem

When calling `ResolveMap` with an input containing `recipe.InputMap`:

```go
map[string]interface{}{
    "form": recipe.InputMap{
        "question": "{{ inputs.prompt }}",
    },
}
```

The template `{{ inputs.prompt }}` inside the `InputMap` was **not resolved** and remained as the literal string.

### Root Cause

`recipe.InputMap` is defined as a type alias:
```go
// In recipe-core/pkg/recipe/shared.go
type InputMap map[string]interface{}
```

Even though `InputMap` has the same underlying type as `map[string]interface{}`, Go's type switch treats them as **different types**. The code in `template_interpolate.go` only had a case for `map[string]interface{}`:

```go
func (rc *ResolutionContext) ResolveValueWithMode(value interface{}, mode ResolutionMode) (interface{}, error) {
    switch v := value.(type) {
    case string:
        return rc.ResolveTemplateWithMode(v, mode)
    case map[string]interface{}:
        // This case matches map[string]interface{} but NOT recipe.InputMap
        // ...
    case []interface{}:
        // ...
    default:
        // recipe.InputMap fell through to here and was returned as-is!
        return value, nil
    }
}
```

When a `recipe.InputMap` was passed, it didn't match the `map[string]interface{}` case and fell through to `default`, which returned it unchanged.

## The Fix

Added a specific case for `recipe.InputMap` in `/src/server/recipe-template/pkg/template/template_interpolate.go`:

```go
case recipe.InputMap:
    // Handle recipe.InputMap specifically (type alias for map[string]interface{})
    // This ensures templates inside InputMap are resolved
    result := make(map[string]interface{})
    for key, val := range v {
        resolved, err := rc.ResolveValueWithMode(val, mode)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve key %s: %w", key, err)
        }
        result[key] = resolved
    }
    return result, nil
```

## Test Coverage

Added comprehensive test in `template_interpolate_test.go`:

### `TestResolveValueWithMode_CustomMapTypes`
1. **Direct InputMap resolution**: Tests resolving templates directly inside a `recipe.InputMap`
2. **Nested InputMap**: Tests the exact reported scenario with `map[string]interface{}` containing `recipe.InputMap`

### Test Results

**Before Fix:**
```
Resolved InputMap: map[question:{{ inputs.prompt }}]
❌ Template NOT resolved
```

**After Fix:**
```
Resolved InputMap: map[question:hello world]
✅ Template resolved correctly
```

## Files Changed

1. **`pkg/template/template_interpolate.go`**
   - Added import for `recipe` package
   - Added case for `recipe.InputMap` in `ResolveValueWithMode`

2. **`pkg/template/template_interpolate_test.go`**
   - Added import for `recipe` package
   - Added `TestResolveValueWithMode_CustomMapTypes` with 2 test cases

## Verification

All existing tests continue to pass:
```bash
go test ./pkg/template/
PASS
ok  	github.com/colony-2/colony2/server/recipe-template/pkg/template	0.018s
```

## Impact

This fix ensures that templates in `recipe.InputMap` values are properly resolved when:
- Passing inputs to sequences
- Passing inputs to state machines
- Passing inputs to operations
- Any scenario where `ResolveMap` is called with nested `recipe.InputMap` values

The fix is backward compatible and doesn't change behavior for other map types.
