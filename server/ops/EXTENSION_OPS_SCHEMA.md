**Extension Ops Schema + Validation**

- Goal: Make `cortex schema` and `cortex validate` include and validate runtime‑discovered extension ops using recipe-core’s existing schema/validate framework.

**Requirements**

- Extension ops declare input/output JSON Schemas in `.colony2/ops/<op>/op.yaml`:
  - `input_schema` (object): JSON Schema (Draft 2020) for `inputs`.
  - `output_schema` (object): JSON Schema for the op’s outputs (used for docs; optional for runtime validation).

Example `op.yaml` fragment:

```yaml
name: python_sum
description: Summation op
run: |
  python3 sum.py
timeout: 30s
input_schema:
  type: object
  required: [numbers]
  properties:
    numbers:
      type: array
      items: { type: number }
output_schema:
  type: object
  required: [sum]
  properties:
    sum: { type: number }
```

**Integration Strategy**

- Discovery: `server/ops/pkg/extensions` already discovers extension ops and builds `RegisterableOp` instances.
- Schema injection (no recipe-core changes):
  - Implement a small struct wrapper around a map with inline YAML to represent dynamic inputs while remaining a struct for reflection guards.
  - The wrapper implements `JSONSchema() *jsonschema.Schema` so invopop’s reflector emits the exact `input_schema` from `op.yaml`.
  - The wrapper also implements `UnmarshalYAML` to validate YAML inputs against the same JSON Schema at parse time.
  - Return an instance of this wrapper (preloaded with the JSON Schema) from `RegisterableOp.GetInputStruct()` so both schema generation and YAML validation work seamlessly.
  - If `input_schema` is missing, default to a permissive object: `{ "type": "object", "additionalProperties": true }`.
- Outputs:
  - `output_schema` is carried in op metadata for documentation and potential future validation of node output mappings. It does not affect `cortex validate` (which checks recipe structure, not runtime output).

**Why This Works with Cortex**

- `cortex schema`:
  - Calls `shared.RegisterOps()` which pulls ops from `server/ops/pkg/export.GetAll()`.
  - `export.GetAll()` already appends discovered extension ops.
  - `recipe-core.GenerateSchemaString()` enumerates registered ops and reflects `GetInputStruct()`. With the wrapper’s `JSONSchema()` method, the generated JSON Schema includes the extension op’s `inputs` shape.
- `cortex validate`:
  - Validates recipe YAML against the generated schema (`recipe-core/pkg/recipe/validate.go`). Recipes that use extension ops are validated using the injected `input_schema`.
  - No changes required to cortex beyond ensuring ops are registered before schema/validation (already done via `shared.RegisterOps()`).

**Implementation Notes (server/ops)**

- Add to `server/ops/pkg/extensions` a struct that wraps a dynamic map and carries schema:
  - Example shape:
    - `type ExtInputs struct { Data map[string]interface{} ` + "`yaml:\",inline\" json:\"-\"`" + `; Schema *jsonschema.Schema }`
    - `func (e ExtInputs) JSONSchema() *jsonschema.Schema { return e.Schema }`
    - `func (e *ExtInputs) UnmarshalYAML(unmarshal func(any) error) error { var m map[string]interface{}; if err := unmarshal(&m); err != nil; /* compile and validate against e.Schema using jsonschema/v6 */; e.Data = m; return nil }`
  - During discovery:
    - Parse `input_schema` from `op.yaml` into `*jsonschema.Schema` (or a permissive default) and stash it in the wrapper instance returned by `GetInputStruct()`.
    - Keep execution handler unchanged (activity-only external command).
- Backwards compatibility:
  - If an extension does not specify `input_schema`, we still register it with a permissive schema so `cortex schema` and `validate` continue to work.

**Operational Flow**

- API server: In ops setup (`server/api/internal/opssetup`), call normal registration (already wired) so extension ops are visible to clients.
- CLI:
  - `cortex schema`: prints schema that includes extension ops and their input schemas.
  - `cortex validate -f recipe.yaml`: validates recipes using extension ops’ schemas.
- Project root detection: Discovery starts from CWD and walks up for `.colony2/ops`; set `VIBETHIS_PROJECT_ROOT` to pin a non-standard working dir when running `cortex` outside the repo root.

**Edge Cases**

- Invalid `input_schema`: Fail registration of that op with a clear error referencing the `op.yaml` path; other ops still register.
- Name collisions: Directory name vs `name` in `op.yaml` — `name` wins; schema discriminator (`op`) uses the resolved name.
- Inline execution: Not supported for extension ops; they always run as activities. This does not impact schema/validation.

**Minimal Test Plan**

- Place a sample op under `.colony2/ops/python_sum` with `op.yaml` including `input_schema`.
- Run `cortex schema` from project root and verify the `Node` oneOf includes `op: "python_sum"` and `inputs` with required `numbers`.
- Create a recipe using `op: python_sum`:
  - Validate passes when inputs conform to the schema.
  - Validate fails with helpful errors when `numbers` is missing or wrong type.
