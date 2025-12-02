# Compiler Typed Template & Map-Free Refactor Specification

## Goals
- Eliminate map coercion in compiler runtime paths; allow maps only where recipe authors explicitly declare them (e.g., `outputs:` YAML) or as the op input envelope payload.
- Preserve op inputs/outputs as typed structs; `ops.RawMessageOrStruct` is used only for dynamic inputs/contexts, never for stored op outputs in state records.
- Provide typed/dynamic template/CEL resolution without normalizing structs to maps.
- Enforce that sequences/states emit only declared `outputs:`; ops emit only their declared `Out` type; no implicit bubbling of internals.
- Keep `ActivityInvocationRequest.OpInput` as `map[string]interface{}` but validate/deny any disallowed/system fields leaking into op inputs.

## Problem Statement (Current Issues)
1) **Template resolver map coercion**
   - `TemplateData.Inputs` and `SetInputs` assume maps; context is extracted by map indexing.
   - CEL adapters convert everything (inputs, sequence/state outputs) into maps.
   - `TemplateResolver` (`template.go`) builds template data by converting inputs/step outputs to maps.
   - Tests index outputs as maps, reinforcing the coercion.

2) **Output normalization**
   - `materializeOpOutput` converts op outputs into maps (JSON round-trip).
   - `WorkflowState`, `StepResult`, `TemplateScope` store map outputs.
   - `__sequence_nodes__` sentinel leaks internal sequence state.

3) **Execution path map merges**
   - `executeSequence`, `executeStateNode`, `executeOp` merge/clobber `map[string]interface{}` inputs and pass them down.
   - State machine fallback logic returns aggregated state outputs as maps.

4) **Registry integration gap**
   - Compiler constructs op inputs via map merges/resolution; registry doesn’t provide an input resolver that yields the op’s `In` type from the current execution context.

5) **Tests tied to maps**
   - Compiler/template tests expect map indexing of outputs; op registration uses global registry with map-typed handlers.

## Target Data Model
### Op IO
- Each `RegisterableOp` defines `In`/`Out` types.
- Runtime carries op inputs/outputs as `ops.RawMessageOrStruct` (raw JSON or struct); no coercion to maps.

### Workflow/State Records
- `WorkflowState`:
  - `Inputs ops.RawMessageOrStruct`
  - `Steps map[string]StepResult`
  - `Outputs ops.RawMessageOrStruct`
  - `Context map[string]interface{}`
- `StepResult.Outputs`: concrete op `Out` struct (no `json.RawMessage`) so it can be referenced/resolved later without serialization.
- `StateRecord.Output` (single value):
  - Op node: op `Out` struct (never `json.RawMessage`).
  - Sequence/state with `outputs:`: resolved declared outputs (as authored, often a map) kept in-memory; not derived by coercing child outputs.
  - Sequence/state without `outputs:`: nil/empty (no implicit bubbling).

### TemplateData / ResolutionContext
- `Inputs ops.RawMessageOrStruct`
- `Sequence map[string]NodeOutput` where `NodeOutput.Outputs ops.RawMessageOrStruct`, `Runs` maintain same type.
- `States map[string]StateOutput` with same pattern.
- `Context` only set if provided as a map in inputs.
- CEL/templating view uses native/dyn adapters (`cel.NativeToValue` or custom `TypeAdapter`) over raw structs/RawMessage; no map normalization.

### Declared Outputs (`outputs:` on sequences/states)
- Arbitrary author-declared shape. Parse YAML → Go value (often `map[string]any`); resolve templates in place; keep in-memory. Do not coerce op outputs to map to satisfy this.
- Store as `ops.RawMessageOrStruct` (the resolved value), not serialized; not derived from child outputs unless explicitly referenced in templates.

### Registry Input Resolver
- Activity registry exposes an input resolver: given current execution context (typed `WorkflowInputs`/ResolutionContext), resolve templates and produce the op’s `In` value (struct) or raw JSON for `ExecuteV2`.
- Compiler no longer manually merges maps; it uses the registry’s resolver per op.

## Detailed Changes

### 1) Template Resolver Rewrite (No Map Coercion)
- `TemplateData.Inputs`: keep as `ops.RawMessageOrStruct`; remove map defaults.
- `SetInputs`: assign inputs; if inputs is a `map[string]interface{}` and has `context`, set `TemplateData.Context`; otherwise empty map.
- Remove `convertRawToMap` map coercion for inputs/outputs; pass `Inputs`, `Sequence`, `States` directly into CEL via adapters.
- CEL adapter/value factory:
  - Use `cel.NativeToValue` for native structs/slices.
  - For `json.RawMessage`, lazily decode to Go value for CEL only (cache per payload), without mutating stored payloads.
  - Do not marshal structs to maps; do not clone into `map[string]interface{}`.
- Interpolation/ResolveValue:
  - Strings resolved via CEL; other types returned as-is.
  - For collections, walk without converting elements to maps unless they already are maps.
- Tests:
  - Update to assert on typed structs/Raw payloads; only index as map when the underlying value is a map (e.g., declared outputs).
- Introduce a `ScopeType` enum for resolver scopes (`root`, `sequence`, `state_machine`, `state`, `op`) to replace freeform strings.
- Use the existing typed execution context (`compiler.ExecutionContext`) in `TemplateData.Context`; `SetInputs` should look for a `context` key in the incoming inputs map, decode that map into `compiler.ExecutionContext` (e.g., via mapstructure), and if absent or invalid leave `TemplateData.Context` as the zero-value. Do not introduce any new context struct in the resolver.
- `SetInputs` is valid for all resolver scopes (`root`, `sequence`, `state_machine`, `state`, `op`) at the moment the scope is created; it should not mutate parent scopes. Subsequent calls should be rare—prefer creating a new child context when scope inputs change.

### 2) Typed Workflow State
- `WorkflowState.Inputs/Outputs`: `ops.RawMessageOrStruct`.
- `StepResult.Outputs`: `ops.RawMessageOrStruct`.
- Remove `materializeOpOutput` normalization; wherever a map is truly needed for legacy compatibility, handle at a clear boundary and mark for removal.

### 3) Sequence/State Output Semantics
- Sequence/state emit only declared `outputs:`; no implicit exposure of child outputs when `outputs:` is absent (nil/empty).
- Remove `__sequence_nodes__` sentinel and any fallback that aggregates child outputs.
- Declared outputs resolution:
  - Parse YAML outputs to Go value (likely `map[string]any`); walk recursively, resolve templated strings via CEL/template using typed resolver.
  - Store resolved declared outputs as `ops.RawMessageOrStruct` (the resolved value), not serialized, not coercing op outputs.

### 4) Activity Registry Input Handling (Envelope with Map Payload)
- Keep op input template resolution in the compiler: resolve templates against workflow context to produce an input payload.
- `ActivityInvocationRequest.OpInput` remains a `map[string]interface{}`; this is the resolved input map passed to the registry/task boundary.
- Add a wrapper/validator hook that inspects `OpInput` maps and *actively denies* (errors) any disallowed/system fields (e.g., git/context blobs) instead of silently stripping. Validate against the op’s `In` type/schema; fail fast on unexpected keys.
- Validation should happen before task dispatch; if denied keys are found, return an error rather than mutating the payload.
- Compiler `executeOp`:
  - Build ResolutionContext with typed inputs (`ops.RawMessageOrStruct`).
  - Resolve node input templates to a `map[string]interface{}` payload.
  - Validate the payload to ensure no bleeding of workflow/system context into op inputs; error on violations.
  - Marshal to JSON only at the envelope/task boundary as needed; registry decodes into the op’s `In` type.

### 5) Compiler Execution Paths
- `executeSequence`/`executeStateNode`/`executeStateMachine`:
  - Pass `ops.RawMessageOrStruct` down; only type-assert to map when explicitly dealing with declared outputs that are maps.
  - Remove map merges for inputs; rely on typed input resolver.
- Op outputs: carry as returned (`ops.RawMessageOrStruct`), not converted to maps; update consumers/tests accordingly.

### 6) Tests Adjustments
- Compiler/template tests:
  - Assert on typed outputs or raw payloads; avoid map indexing unless value is a map.
  - Replace global registry clears with registry-scoped registrations; avoid duplicate registration errors by checking registry state.
- Sequence/state tests:
  - Expect nil/empty outputs when no `outputs:` is declared.
  - For declared outputs, expect the resolved map exactly as authored (no child output injection unless templated).
- Registry tests:
  - Add coverage for input resolver: given context + raw/struct payload, obtain op `In`; ensure no map coercion.

## Implementation Steps (Suggested Order)
1) **Data structures**: Update `WorkflowState`, `TemplateData`, `NodeOutput`, `StateOutput` to use `ops.RawMessageOrStruct` where dynamic is needed; `StepResult`/`StateRecord` must store concrete op output structs (no `json.RawMessage`). Remove map defaults in TemplateData Inputs and wire `TemplateData.Context` to the existing typed execution context.
2) **Resolver**: Rewrite `template_resolver.go` to drop `convertRawToMap` for CEL/template; add CEL adapters for `ops.RawMessageOrStruct` (native/dyn, lazy RawMessage decode). Fix `SetInputs` to only extract context from map inputs.
3) **Execution**: Refactor `executeOp` to use registry input resolver; refactor sequence/state execution to avoid map merges; remove `materializeOpOutput`.
4) **Outputs**: Enforce declared outputs-only for sequences/states; remove `__sequence_nodes__`; adjust state machine terminal behavior.
5) **Tests**: Update compiler/template tests for typed expectations; register ops via registry, no global clears; adjust assertions away from map indexing except when the value is actually a map (e.g., declared outputs).
6) **Cleanup**: Delete `raw_util.go` and any residual map coercion helpers; ensure no production path normalizes structs to maps except for explicitly declared outputs maps.

## Success Criteria
- No production path coerces op inputs/outputs to `map[string]interface{}` except when the recipe author declares a map in `outputs:` or in the op input envelope; templating/CEL uses typed/dyn adapters.
- Sequences/states do not expose internals without declared outputs; `__sequence_nodes__` removed.
- Compiler uses registry-provided input resolver to build op `In` without map merges.
- `go test ./pkg/compiler` passes with typed expectations; template tests no longer depend on map coercion.
