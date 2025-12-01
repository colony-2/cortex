# Typed Template Resolution and State Modeling Spec

## Goals
- Eliminate `map[string]interface{}` from op inputs/outputs and internal state tracking; allow maps only where recipe authors declare arbitrary outputs on sequences/states.
- Keep op inputs/outputs isolated to their declared shapes; execution metadata, git/context, and scope bookkeeping travel in separate typed envelopes.
- Provide a single, typed template/CEL view that matches the scoping rules in `NODE_SCOPE_SPEC.md` and the map-removal direction set in `compiler-typed-refactor-spec.md`.
- Remove legacy/double implementations (`template.go`, `template2.go`, `workflows/template_resolution.go`) and sentinel hacks like `__sequence_nodes__`.

## Current Pain Points (as of /pkg/compiler)
- `template_resolver.go` stores `TemplateData` as loose maps and mutates it during execution; `SetInputs` smuggles `context` out of user inputs.
- `statemachine_compiler.go`/`executeSequence`/`executeOp` merge maps repeatedly, so per-op payloads can pick up unrelated keys and lose type info before reaching the registry.
- `StateContext`/`ScopedContext` expose CEL/template data via nested maps, making run histories and state/sequence boundaries hard to validate.
- Multiple resolvers (`template.go`, `template2.go`, scoped resolver, and `pkg/workflows/template_resolution.go`) disagree on shape and error handling.
- Transition evaluation uses map-shaped sequence/state outputs plus the `__sequence_nodes__` sentinel instead of a typed view of the child scope.

## Target Runtime Data Model
- **WorkflowInputs** (compiler-level): typed struct that aggregates
  - `User` (raw recipe inputs JSON or a decoded struct),
  - `Invocation contextual.InvocationContext`,
  - `Actor contextual.ActorContext`,
  - `Environment contextual.EnvironmentContext`,
  - `Git gitstate.WorkspacePayload`,
  - optional `Envelope contextual.WorkflowEnvelope`.
- **OpCallEnvelope** (per invocation):
  - `Invocation ops.Invocation`,
  - `Git gitstate.WorkspacePayload`,
  - `InputPayload ops.RawMessageOrStruct` (either raw JSON or already-decoded struct shaped for the op’s declared input type),
  - `OutputPayload ops.RawMessageOrStruct` (either raw JSON or already-decoded struct for the op’s output type).
  - No workflow/sequence/state data is merged into this payload beyond what the recipe author mapped.
- **ScopeFrame** (sequence/state/state-machine):
  - `ID`, `Kind` (enum), `Parent *ScopeFrame`,
  - `InputPayload ops.RawMessageOrStruct` (resolved scope input, raw or decoded),
  - `Steps map[StepID]StepRecord`,
  - `States map[StateID]StateRecord`,
  - `Meta ScopeMeta` (execution id, timestamps, attempt counters),
  - `Context contextual.*` snapshots (invocation, actor, env, git) carried separately from user inputs.
- **StepRecord / StateRecord**
  - `Output` (single output value for the node):
    - For op nodes: the op’s declared `Out` struct (never `json.RawMessage`).
    - For sequence/state nodes with `outputs:`: the resolved `map[string]interface{}` produced by evaluating that outputs map.
    - For sequence/state without `outputs:`: no output (nil) — they do not implicitly expose internals.
  - `Runs []RunRecord` (for retries/loops),
  - `Attempt int`, `StartedAt/CompletedAt time.Time`.
- **TemplateScopeView** (fed to CEL/templates; no generic interface{}):
  - `Inputs` -> decoded view of the scope input struct (or `map[string]ops.RawMessageOrStruct` if arbitrary),
  - `Sequence` -> `map[string]TemplateNodeView` (`Outputs` is either an op output struct or `map[string]interface{}` from a sequence outputs block; `Runs []TemplateRunView`),
  - `States` -> `map[string]TemplateStateView` (same output rules),
  - `Scope ScopeMeta`, `Context contextual.*`.
  - Conversion to CEL uses adapters over the above typed structs/maps instead of ad-hoc `map[string]interface{}` cloning; op outputs stay typed, declared outputs stay as the resolved map.

## Template Resolution Pipeline
1) **Input shaping**  
   - Build a zero value of the target input struct (`op.GetInputType()` or scope input type) and hand it to `cel-go`’s dyn/adt adapters to expose a walkable shape—no conversion to `map[string]interface{}` and no manual reflection code.
   - Walk that dyn view, replacing templated strings via CEL evaluation over the `TemplateScopeView`. Non-string nodes stay typed; unknown fields are rejected early. Shape inspection relies on CEL’s native/dyn adapters, not on JSON round-trips.
   - Produce the resolved payload as `ops.RawMessageOrStruct` (raw or decoded). Decode into the target struct only when required by the consumer (`ExecuteV2` does the final decode).
2) **Output mapping**  
   - When a sequence/state defines `outputs:`, create a `map[string]interface{}` from the declared YAML, then recursively walk it, evaluating CEL/template expressions in place. The result remains an in-memory map (not serialized).
   - For CEL evaluation over outputs (sequence/state exports), surface op outputs as native structs and declared outputs as the resolved map; avoid `json.RawMessage` in this path.
3) **CEL environment**  
   - Register CEL variables `inputs`, `sequence`, `states`, `scope`, `context` backed by typed adapters; no direct map[string]interface{} injection.
   - Run-condition evaluation (`when`) uses the same adapters and typed `TemplateScopeView`; no sentinel fields (`__sequence_nodes__`) needed.
4) **Error handling**  
   - Missing references surface as validation errors tied to the target field path; interpolation returns raw types when the whole value is a single expression.

## Execution Flow Updates
- Op nodes produce their declared output struct; store that struct in `StepRecord/StateRecord` (no `json.RawMessage` in compiler state).
- Sequence/state nodes produce a single output: if `outputs:` is present, the resolved `map[string]interface{}`; otherwise nil (they do not implicitly expose child outputs).
- `executeOp` builds an `OpCallEnvelope` from the resolved input JSON or struct and hands it to the registry; registry/`ExecuteV2` performs the decode.
- Scope transitions read from the current `ScopeFrame` and child frames directly; removal of `__sequence_nodes__` and map-based run histories.
- Context propagation: git/actor/invocation/envelope ride along in `WorkflowInputs`/`ScopeFrame.Meta` and are never merged into user payloads.
- **RawMessageOrStruct at wire edges:** Use `RawMessageOrStruct` for ActivityInvocationRequest/Output and op input payloads flowing through the task boundary so callers can pass either raw JSON or decoded structs; decoding remains the responsibility of `ExecuteV2`. Compiler/runtime state keeps typed structs for op outputs and in-memory maps for declared sequence/state outputs.

## Cleanup and Migration Plan
1) Delete/retire `template.go`, `template2.go`, `ScopedTemplateResolver`, and `pkg/workflows/template_resolution.go` after introducing the unified typed resolver.
2) Introduce typed structs (`ScopeFrame`, `TemplateScopeView`, `StepRecord`, etc.) and migrate `template_resolver.go`, `statemachine_compiler.go`, `compiler.go`, and `scope.go` to them.
3) Replace map merges in `executeSequence`/`processNodeOutputs`/`executeStateNode` with typed input/output shaping against the target structs. For generic payloads at the task boundary (OpCallEnvelope, activity invocation envelopes), use `RawMessageOrStruct`; compiler-held outputs stay as typed structs or resolved maps.
4) Update tests to construct typed inputs/outputs, assert on JSON decode into concrete types, and validate CEL visibility of sequence/state scopes without map shortcuts. Add coverage that `RawMessageOrStruct` flows through without double marshal in ActivityInvocationRequest/Output, while compiler records keep concrete outputs/maps.
5) Drop legacy context hacks (`SetInputs` pulling `context` from user inputs); context is injected via typed envelopes only.
6) Introduce `type RawMessageOrStruct = any` (or similarly named alias) near the compiler runtime types to make the allowance explicit and document which fields use it: op inputs/outputs in envelopes (mirroring the alias already declared in `recipe-core/pkg/ops`) and any temporary storage before the registry boundary. Compiler-held outputs use concrete structs (ops) or resolved maps (sequence/state outputs), not the alias.

## Success Criteria
- No production structs in `pkg/compiler` or `pkg/workflows` expose `map[string]interface{}` except for the resolved outputs maps defined explicitly in recipe `outputs:` blocks; dynamic data elsewhere is typed structs or `ops.RawMessageOrStruct` only at task boundaries.
- Template/CEL evaluation operates over typed adapters, with op outputs as structs and declared outputs as in-memory maps; no JSON round-trips for compiler state.
- Op invocations receive only the resolved, decoded input struct; no extraneous workflow/state fields appear in op inputs or outputs.
- Transition logic and output mappings run without sentinel keys or map-based run histories.
