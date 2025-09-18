**Recipe-Core Change Proposal: Enable Wrapper-Based YAML Validation for Op Inputs**

Summary
- Minimal change in recipe-core to allow op-specific input wrappers (defined in server/ops) to validate recipe YAML at parse time via `UnmarshalYAML` without any recipe-core special casing.

Context
- Extension ops discovered at runtime provide JSON Schemas and want to validate their `inputs:` during YAML parsing.
- In server/ops we’ll return a wrapper type from `RegisterableOp.GetInputStruct()` that:
  - embeds a `map[string]interface{}` via `yaml:",inline"` to accept arbitrary keys,
  - implements `UnmarshalYAML` to validate against the op’s schema (which it can fetch from the global ops registry), and
  - optionally implements `JSONSchema()` for schema generation.

Problem
- Current recipe-core logic unmarshals op inputs into a pointer to an interface value, which causes the YAML decoder to materialize a generic map and bypass any `UnmarshalYAML` on the concrete type. As a result, wrapper-based validation cannot run.

Proposed Change (one function, few lines)
- In `server/recipe-core/pkg/recipe/node.go`, in `checkOpInputs`, unmarshal into a pointer to the concrete input value returned by `op.GetInputStruct()` so that, when the type implements `UnmarshalYAML`, it is invoked by the YAML decoder.
- No other changes to recipe-core are required. Schema generation, parsing, and execution semantics remain unchanged.

Rationale
- This preserves recipe-core’s generic design and avoids special-casing extension ops.
- It enables input wrappers to own all schema lookup/validation logic (they can obtain their op name from context or the registry if needed).
- Maintains full backward compatibility: existing ops without custom `UnmarshalYAML` keep working as before.

Diff (illustrative)
```diff
diff --git a/server/recipe-core/pkg/recipe/node.go b/server/recipe-core/pkg/recipe/node.go
index abcdef0..1234567 100644
--- a/server/recipe-core/pkg/recipe/node.go
+++ b/server/recipe-core/pkg/recipe/node.go
@@
-import (
-    "fmt"
-
-    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"
-    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
-    "github.com/invopop/jsonschema"
-    yamlv3 "gopkg.in/yaml.v3"
-)
+import (
+    "fmt"
+    "reflect"
+
+    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"
+    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
+    "github.com/invopop/jsonschema"
+    yamlv3 "gopkg.in/yaml.v3"
+)
@@
 func checkOpInputs(opName string, inputs map[string]interface{}, line int, col int) error {
     op, exists := ops.Get(opName)
     if !exists {
         return fmt.Errorf("unknown op: [%s] at [%d:%d]", opName, line, col)
     }
-    concreteInputType := op.GetInputStruct()
+    // Unmarshal into a pointer to the concrete input value so that any
+    // type-provided UnmarshalYAML (e.g., validation wrappers) is invoked.
+    inputVal := op.GetInputStruct()
+    var dest interface{}
+    rv := reflect.ValueOf(inputVal)
+    if rv.Kind() == reflect.Ptr && !rv.IsNil() {
+        dest = inputVal
+    } else {
+        dest = reflect.New(rv.Type()).Interface()
+    }

     data, err := yamlv3.Marshal(inputs)
     if err != nil {
         return err
     }
-    if err := yamlv3.Unmarshal(data, &concreteInputType); err != nil {
+    if err := yamlv3.Unmarshal(data, dest); err != nil {
         return fmt.Errorf("invalid inputs for op [%s] at [%d:%d]: %w", opName, line, col, err)
     }
     return nil
 }
```

Backward Compatibility
- Existing ops (struct types or maps) continue to decode as before; only the destination pointer changes to ensure proper `UnmarshalYAML` dispatch.
- No public API changes in recipe-core.

Testing Guidance
- Add/keep unit tests that:
  - Validate a normal op’s inputs still decode successfully.
  - Validate an invalid input still produces a useful error.
  - New: define a test op whose input type implements `UnmarshalYAML`; confirm its method is invoked and can reject invalid YAML.

Impact on Other Components
- Cortex: no changes needed. It already registers ops and uses recipe-core’s schema/validation.
- Server/ops: can now return wrapper input types that run their own schema-based validation during YAML unmarshal.
