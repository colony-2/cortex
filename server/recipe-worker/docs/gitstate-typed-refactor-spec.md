# Step 1: Typed Refactor for pkg/gitstate

## Goals and Boundaries
- Eliminate map-based git context handling; rely exclusively on typed structs for context, recipe metadata, patches, and persist results.
- Accept only typed inputs produced by the activity registry; no legacy map bridges.
- Ensure detached/inline workspace flows, controller lifecycle, and persist/restore paths operate on typed data.
- Reuse shared contexts from `pkg/contextual` (InvocationContext, ActorContext, EnvironmentContext, GitSnapshotContext, WorkflowEnvelope) instead of redefining fields.

## Target Design
- **Context struct:** Replace `gitstate.Context` fields with an embedded `GitExecutionContext` that itself embeds shared `contextual` structs. Add gitstate-only flags sparingly (e.g., inline markers).
- **ContextFromRequest:** Accept typed invocation envelope from the registry; decode directly into `GitExecutionContext` (or a thin DTO that maps to it) without map lookups.
- **Persist propagation:** `InjectPersistResult` returns typed context output structs (`GitPersistResult` carrying updated `GitSnapshotContext` and related data); consumers embed them without map mutation.
- **Workspace flows:** `PlanDetachedWorkspace` and `WithDetachedWorkspace` accept/return `GitExecutionContext` and typed outputs; no `map[string]interface{}` cloning or duplicated metadata fields.

## Implementation Steps
1) **Struct definitions**
   - Replace `gitstate.Context` with `GitExecutionContext` embedding shared contexts; add gitstate-only fields sparingly.
   - Define `GitPersistResult` and `GitWorkspacePlan` using shared types; remove duplicated fields.
2) **Parsing and validation**
   - Rewrite `ContextFromRequest` to consume typed payloads; enforce required fields and derive defaults without map helpers.
3) **Lifecycle helpers**
   - Update detached/inline workspace helpers and controller glue to pass typed structs end-to-end (inputs, outputs, patches, persist data).
4) **Output propagation**
   - Replace `InjectPersistResult` map mutation with functions returning typed output envelopes that the compiler/ops can serialize without map edits.
5) **Tests**
   - Refresh tests to build typed payloads; add coverage for missing fields, bad types, patch application, detached workspace derivation, and persist/restore correctness.

## Notes
- No shims or map adapters; failure on map-shaped payloads is acceptable.
- Keep JSON tags stable to preserve over-the-wire compatibility while internally staying fully typed.
