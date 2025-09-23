**Recipe-Core Change Proposal: Input Provider for Ops (Schema + YAML Validation)**

Overview
- Add new constructors to RegisterableOp that accept a provider function for the op’s input instance. This allows ops to surface a per-op input wrapper which:
  - Implements JSONSchema() so schema generation includes the exact per-op input schema.
  - Implements UnmarshalYAML for parse-time validation of recipe YAML inputs.
- This keeps recipe-core generic and avoids any special-casing of extension ops.

Goals
- Let ops inject a concrete input instance dynamically (e.g., a wrapper carrying per-op compiled JSON Schema) for:
  - JSON Schema generation in `pkg/recipe/schema.go`.
  - YAML parsing/validation in `pkg/recipe/node.go` (with the already accepted change to unmarshal into a concrete pointer).
- Preserve all current behavior when no provider is supplied.

API Changes (additive)
1) New constructors (mirroring existing ones):
- `NewActivityMappedOpWithProviderV2[In any, Out any](metadata OpMetadata, handler func(ops.Invocation, context.Context, In) (Out, error), getInputStruct func() interface{}) RegisterableOp`
- `NewInlineOpWithProviderV2[In any, Out any](metadata OpMetadata, handler func(ops.Invocation, workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error), getInputStruct func() interface{}) RegisterableOp`

2) Internals: extend opSpecImpl
- Add field: `inputProvider func() interface{}`
- Modify methods:
  - `GetInputStruct() interface{}` → if `inputProvider != nil`, return `inputProvider()`. Else, current behavior (`reflect.New(GetInputType()).Elem().Interface()`).
  - `GetInputType() reflect.Type` → if `inputProvider != nil`, return `reflect.TypeOf(inputProvider())` (if it’s a pointer, return `Elem()`); else current behavior based on handler signature.

3) No changes to Execute/ExecuteInline
- These continue to decode into the generic `In` as today. The provider is used only for schema reflection and YAML parsing/validation.

Illustrative Diff
```diff
diff --git a/server/recipe-core/pkg/ops/registerable_op.go b/server/recipe-core/pkg/ops/registerable_op.go
index 1111111..2222222 100644
--- a/server/recipe-core/pkg/ops/registerable_op.go
+++ b/server/recipe-core/pkg/ops/registerable_op.go
@@
 type opSpecImpl[In any, Out any] struct {
     metadata          OpMetadata
     handler           func(context.Context, In) (Out, error)
     inlineHandler     func(workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error)
     managementService ManagementService
+    inputProvider     func() interface{}
 }

@@
 func (c *opSpecImpl[In, Out]) GetInputStruct() interface{} {
-    return reflect.New(c.GetInputType()).Elem().Interface()
+    if c.inputProvider != nil {
+        return c.inputProvider()
+    }
+    return reflect.New(c.GetInputType()).Elem().Interface()
 }

@@
 func (c *opSpecImpl[In, Out]) GetInputType() reflect.Type {
-    if c.handler != nil {
-        return reflect.ValueOf(c.handler).Type().In(1)
-    } else {
-        // Inline handler signature: func(workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error)
-        // The input type is the 4th parameter (index 3)
-        return reflect.ValueOf(c.inlineHandler).Type().In(3)
-    }
+    if c.inputProvider != nil {
+        t := reflect.TypeOf(c.inputProvider())
+        if t.Kind() == reflect.Ptr { return t.Elem() }
+        return t
+    }
+    if c.handler != nil {
+        return reflect.ValueOf(c.handler).Type().In(1)
+    }
+    // Inline handler signature: func(workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error)
+    // The input type is the 4th parameter (index 3)
+    return reflect.ValueOf(c.inlineHandler).Type().In(3)
 }

@@
func NewActivityMappedOpWithProviderV2[In any, Out any](metadata OpMetadata, handler func(ops.Invocation, context.Context, In) (Out, error), getInputStruct func() interface{}) RegisterableOp {
    return &opSpecImpl[In, Out]{
        metadata:      metadata,
        handler:       handler,
        inputProvider: getInputStruct,
    }
}

@@
func NewInlineOpWithProviderV2[In any, Out any](metadata OpMetadata, handler func(ops.Invocation, workflow.Context, time.Duration, *temporal.RetryPolicy, In) (Out, error), getInputStruct func() interface{}) RegisterableOp {
    return &opSpecImpl[In, Out]{
        metadata:      metadata,
        inlineHandler: handler,
        inputProvider: getInputStruct,
    }
}
```

Companion Change (already proposed and accepted)
- In `pkg/recipe/node.go`, `checkOpInputs` now unmarshals into a pointer to the concrete value returned by `GetInputStruct()`, enabling wrapper-based `UnmarshalYAML` to validate YAML inputs.

Backwards Compatibility
- No breaking API changes; existing constructors keep working.
- When the provider is omitted, behavior is identical to today.

Testing Plan (recipe-core)
1) New constructor behavior
- Create a dummy op using `NewActivityMappedOpWithProviderV2` with a provider returning a struct `W` that implements `JSONSchema()`.
- Verify `GetInputStruct()` returns a `*W` and `GetInputType()` matches `W`.
- Ensure schema generation (`recipe.GenerateSchemaString()`) reflects `W.JSONSchema()`.

2) YAML validation via wrapper
- With the previously accepted unmarshal change in `checkOpInputs`, define `W` with `UnmarshalYAML` that sets a flag and enforces a field.
- Register the op with provider returning `&W{}` and unmarshal a recipe node that uses it; confirm validation triggers and passes/fails as expected.

3) Regression tests for existing ops
- Ops constructed with existing constructors should continue to:
  - Produce identical schema shapes as before.
  - Validate YAML inputs as before.

Server/Ops Integration (reference)
- Extension ops will pass an input provider that returns a wrapper instance carrying per-op schemas:
```go
op := ops.NewActivityMappedOpWithProviderV2[map[string]interface{}, map[string]interface{}](
    md,
    func(inv ops.Invocation, ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
        // handler logic (inv carries recipe/node identifiers for tracing)
        return runExtension(inv, ctx, input)
    },
    func() interface{} {
        return &extInputsWrapper{
            schemaDoc:      rctx.inputSchemaDoc,
            compiledSchema: rctx.compiledInput,
        }
    },
)
```
- This makes per-op input schemas visible in the generated JSON Schema and enforces YAML parse-time validation via `UnmarshalYAML` on the wrapper.
