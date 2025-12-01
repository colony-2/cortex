# Context Model Plan (Shared and Package-Specific)

## Goals
- Reduce duplicated metadata across gitstate, ops, and compiler by introducing shared context objects.
- Respect Go package directionality: lower-level context packages must not import higher-level modules (compiler and ops should depend on shared contexts, not vice versa).
- Keep contexts small and purpose-specific; compose where needed rather than creating monolithic structs.

## Proposed Package Layout
- `pkg/contextual` (new): shared package holding common context structs used across multiple packages. It may import recipe-core types and `swf` types (e.g., `swf.JobId`) but should stay otherwise dependency-light.
- Package-specific subcontexts remain within their domains (`pkg/gitstate`, `pkg/compiler`, `pkg/ops`) and embed or reference shared types from `pkg/contextual`.

## Shared Contexts (in pkg/contextual)
- **InvocationContext**: ties together runtime invocation metadata used by ops and compiler.
  - Fields (JSON tagged): `InvocationID`, `InvocationHash`, `InvocationAttempt`, `ActivityID`, `BoxID`, `NodePath`, `RecipeID`, `JobID` (use `swf.JobId`).
- **ActorContext**: ticket/actor identity.
  - Fields: `TicketID`, `ActorName`, `ActorEmail`, `CellName`.
- **EnvironmentContext**: execution environment hints.
  - Fields: `WorktreePath`, `BlobStoreURI`, `ThinPackPath`.
- **GitSnapshotContext**: immutable git state snapshot info.
  - Fields: `BaseRepo`, `BaseHash`, `PersistHash`, `PreviousHash`, `WorkspacePrepared`, `GitAuthor`.
- **WorkflowEnvelope**: coarse workflow/session identifiers (Temporal or SWF).
  - Fields (JSON tagged): `JobID` (use `swf.JobId`), `Namespace` (optional).

## Package-Specific Contexts
- **pkg/gitstate**
  - `GitExecutionContext`: composes `InvocationContext`, `ActorContext`, `EnvironmentContext`, `GitSnapshotContext`; adds gitstate-only fields if needed (e.g., `Inline` flags).
  - `GitPersistResult`: persist outcome and updated `GitSnapshotContext`.
  - `GitWorkspacePlan`: derives detached workspace paths, includes child recipe metadata.
- **pkg/ops**
  - `OpInvocationEnvelope`: wraps `InvocationContext`, `ActorContext`, op-specific `Input` (typed), and optional `WorkflowEnvelope` reference.
  - `OpOutputEnvelope`: typed op outputs plus updated `GitExecutionContext`/`WorkflowEnvelope` if mutated.
- **pkg/compiler**
  - `WorkflowInputs`: typed inputs plus `ActorContext`, `GitExecutionContext`, and `WorkflowEnvelope`.
  - `StepOutput`: typed op output plus `InvocationContext` snapshot.
  - `WorkflowState`: aggregation of `WorkflowInputs`, step outputs, and resolved contexts for CEL/template use.

## Directionality Rules
- `pkg/contextual` imports stdlib and may import recipe-core types; no dependencies on compiler/ops/gitstate.
- `pkg/gitstate`, `pkg/ops`, and `pkg/compiler` may import `pkg/contextual` but not each other (to avoid cycles).
- Data flows:
  - Registry decodes inputs into `OpInvocationEnvelope` (ops) which includes `GitExecutionContext` from gitstate contexts.
  - Compiler builds `WorkflowInputs` using shared contexts and passes them into ops; gitstate updates return via typed persist structs.

## Migration Notes
- All context structs must carry JSON tags for serialization/deserialization.
- Start by extracting shared fields from `gitstate.Context` into the above shared structs.
- Update gitstate types to embed shared structs; then adjust ops registry and compiler to consume the shared types instead of bespoke map fields.
- Remove duplicated fields once consumers adopt the shared contexts.
