# Step 2: Typed Refactor for pkg/ops

## Goals and Boundaries
- Prerequisite: gitstate must be refactored to typed structs first so the registry can consume the new git context types.
- Remove all `map[string]interface{}` usage from `pkg/ops`, including invocation envelopes, git decorators, and schema generation helpers.
- Centralize all marshal/unmarshal work (including CEL-resolved inputs) inside `activity_registry.go`; the registry is the only boundary that touches `json.RawMessage`/`swf.Data`.
- Ops must receive inputs decoded into their declared `GetInputType()` and return typed outputs only.
- No legacy map compatibility or shims.

## Target Design
- **Invocation envelope:** `ActivityInvocationRequest` holds invocation metadata, raw input (`json.RawMessage`), and a decoded `Input` field typed to the op’s `GetInputType()`. Decoding is done once in the registry before execution.
- **Git-aware wrapper:** `withGitWorkspace` operates on typed git context structs (from `pkg/gitstate`) and typed output envelopes; patch keys like `git_context_patch` disappear.
- **Schema generation:** `SchemaGenerator` derives schemas from concrete input/output types, including the typed git context structs referenced by ops, without map fallbacks.
- **Task wiring:** Conversion between Temporal task payloads (`swf.Data`) and the invocation envelope happens only in the registry. Ops and downstream code never touch raw JSON or maps.

## Implementation Steps
1) **Define typed envelope**
   - Introduce `ActivityInvocationRequest` with `{ Invocation ops.Invocation; RawInput json.RawMessage; Input <typed>; }`.
   - Update registry execution path to decode `RawInput` into `Input` via `op.GetInputType()`; fail fast on decode errors.
2) **Registry wrappers**
   - Rewrite `withGitWorkspace` to consume typed git context structs; propagate persist results via typed output fields (from gitstate) instead of map patches.
   - Keep `withDependencies` but ensure `Invocation.Deps` is set before decode so handlers see fully populated invocation data.
3) **Schema generation**
   - Point schema generation at `op.GetInputType()`/`GetOutputType()`; remove map-based schema fallbacks.
4) **Tests**
   - Update registry tests and integration tests to assert typed decode behavior, git workspace propagation via typed fields, and schema generation over concrete structs.
   - Add failure tests: bad JSON, missing required git context fields, mismatched types.

## Notes
- No support for legacy map inputs/outputs; callers must provide JSON that decodes into the op’s declared struct.
- The registry collaborates with the template resolver by accepting CEL-resolved JSON (handled before decode) and then decoding into the target type.
