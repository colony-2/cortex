# Map Removal Checklist for gitstate & ops (Typed Payload/Result Path)

## Current State
- `gitstate` uses typed `WorkspacePayload`/`WorkspaceResult` internally for inline/detached flows; legacy map handling is isolated to `LegacyPayloadFromInput`/`LegacyOutputsFromResult`.
- `activity_registry` is the only boundary that converts legacy map inputs/outputs into typed payloads/results and back.
- Ops tests and registry tests are aligned with the current handler signature; gitstate tests updated to typed flows.
- Residual map usage exists only for backward compatibility at the registry boundary.

## Remaining Work
1) **Registry boundary cleanup**
   - Remove `LegacyPayloadFromInput`/`LegacyOutputsFromResult` once upstream callers provide typed payloads; switch registry to decode `json.RawMessage` directly into `WorkspacePayload` and expect typed activity outputs.
   - Delete map helper functions (`mapFromAny*`, `stringFromMap`, etc.) after boundary removal.
2) **Inline/Detached function signatures**
   - Update external call sites (e.g., compiler/executor) to call `WithInlineWorkspace`/`WithDetachedWorkspace` with typed `WorkspacePayload`/`WorkspaceResult` instead of map-shaped inputs/outputs.
   - Drop the legacy `ContextFromRequest` shim; rely solely on `ContextFromPayload`.
3) **Activity outputs**
   - Require activities to return typed structs; remove `git_context_patch` handling and map-based merging once all ops emit structured results.
   - Simplify `withGitWorkspace` to skip map marshaling/unmarshaling when outputs are already typed.
4) **Schema generation**
   - Ensure schema generation in `activity_registry` derives from typed input/output structs (including git payload/result types) and remove any map-based schema fallbacks.
5) **Tests**
   - Add/refresh tests that validate the fully typed path: registry decoding of typed payloads, gitstate persistence propagation via typed results, and no legacy map usage.
   - Remove or rewrite legacy map tests once upstream is fully migrated.
