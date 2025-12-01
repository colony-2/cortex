# Typed Activity Envelope (No Legacy Maps) Spec

## Goals
- Eliminate legacy map fallbacks in activity invocation handling.
- Separate op input/output payloads from contextual data (workspace/git) in the invocation envelope.
- Ensure all encoding/decoding happens at the registry boundary with typed structs; no map-based runtime logic.

## Target Envelope Types
- `ActivityInvocationRequest`:
  - `Invocation ops.Invocation` (metadata)
  - `OpInput json.RawMessage` (op-specific input payload; passed through untouched)
  - `Workspace gitstate.WorkspacePayload` (typed git/context payload)
- `ActivityInvocationOutput`:
  - `OpOutput json.RawMessage` (raw op output; registry must not decode/encode it)
  - `Workspace gitstate.WorkspaceResult` (typed git/context result)

## Required Changes
1) **Remove legacy map fallback**
   - Delete `LegacyPayloadFromInput` usage from the registry. `Workspace` must be supplied in `ActivityInvocationRequest`; if missing, return an error.
   - Remove map helper functions from the runtime path; keep only test utilities if needed.
2) **Registry request/response handling**
   - Update `withGitWorkspace` to:
     - Pass `OpInput` directly to `ExecuteV2` (no decoding/encoding in the registry).
     - Use `req.Workspace` directly to build git context (`ContextFromPayload`); error if absent.
     - Capture the raw `OpOutput` from `ExecuteV2` without interpretation.
     - Populate `ActivityInvocationOutput.Workspace` with the typed git persistence result; no map reinjection.
   - Adjust `taskWorker.Run` (and any wrappers) to marshal/unmarshal `ActivityInvocationOutput` instead of map outputs.
3) **Compiler/executor call sites**
   - Build `ActivityInvocationRequest` with `OpInput` (marshalled op input) and `Workspace` (typed `WorkspacePayload` derived from inputs).
   - Expect `ActivityInvocationOutput` from tasks; pass `OpOutput` raw to caller or decode only when needed by downstream logic.
4) **Tests**
   - Update registry/ops/compiler tests to construct requests with typed `Workspace` and to assert on `ActivityInvocationOutput`.
   - Remove legacy map-based tests; add cases for missing workspace (should fail), bad JSON in `OpInput`, and proper propagation of typed workspace results.
5) **Cleanup**
   - Delete `LegacyPayloadFromInput` and `LegacyOutputsFromResult` (or keep only in a deprecated test helper if absolutely needed).
   - Remove map helper functions from production code paths.

## Acceptance Criteria
- No map decoding/encoding in runtime paths for activity invocation; only typed structs and raw JSON for op payloads.
- `ActivityInvocationRequest` and `ActivityInvocationOutput` are the sole wire formats for tasks.
- All ops/gitstate/compiler tests pass with the new envelopes and without legacy fallbacks.
