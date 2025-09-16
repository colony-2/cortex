# Deterministic Input Key + Keyed Signal Routing (Final State)

This spec defines a deterministic key for input coordination and a keyed-signal routing model across workflow, ops, and API layers. This is the final, non‑backward‑compatible state.

## Overview

- Each input invocation in a workflow is assigned a deterministic, compact key (`id`).
- The workflow waits on a signal channel derived from the key: `user-response:<id>`.
- The API requires the same `id` when posting responses (and cancel requests).
- SSE events include the `id` so UIs can correlate prompts and responses.
- Typed search attributes store the `id` and other metadata for querying.

## Deterministic Key

- Inputs must be deterministically reproducible across replay; no random UUIDs.
- Construct a base tuple from deterministic components:
  - `recipeId`: Stable logical recipe identifier (or run’s recipe metadata id).
  - `nodePath`: Stable path to this node in the recipe tree.
    - Prefer concatenated `NodeMetadata.ID` along the path.
    - For nodes lacking IDs, use structural indices: e.g., `seq[2]`, `state[approved]`.
  - `invokeSeq`: Deterministic per-path monotonic counter incremented each time this node path invokes an input within the workflow execution.
  - Optional qualifiers that are stable in inputs: `boxId`, `activityId`.
- Compute `id` as a short hash (base32 lowercase) over the tuple string:

```
inputKeyMaterial := recipeId + "|" + nodePath + "|" + strconv.Itoa(invokeSeq) + "|" + boxId + "|" + activityId
sha := SHA-256(inputKeyMaterial)
id := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sha[:16]) // 128-bit truncation
```

- Determinism notes:
  - `nodePath` and `invokeSeq` are derived only from the recipe structure and workflow-local state.
  - No non-deterministic inputs (time/uuid) are used to generate `id`.

## Workflow Behavior

- Before waiting:
  - Upsert typed search attributes:
    - `InputKey` (Keyword) = `id`
    - `InputStatus` (Keyword) = `pending`
    - `InputCreatedAt` (Time) = `workflow.Now()`
    - `InputExpiresAt` (Time) = `workflow.Now().Add(timeout)`
    - `InputBoxID` (String) = input.BoxID
    - `InputActivityID` (String) = input.ActivityID
- Wait on a per-key signal channel:
  - Channel name: `user-response:<id>`
  - Fallback single-channel listen is removed.
- On timeout:
  - Upsert `InputStatus=timeout` (typed) and return a Temporal application error.
- On response:
  - Upsert `InputStatus=completed`, `InputRespondedBy` (String), `InputRespondedAt` (Time).

## API Management Service

- Endpoints (final shape, all require `id`):
  - `POST /api/user-inputs/{workflowID}/pending`
    - Purpose: Record a new pending prompt in external systems and broadcast SSE.
    - Request:
      - `application/json`
      - `{ "id": string, "title"?: string, "box_id"?: string, "expires_at"?: string(ISO-8601), "metadata"?: object }`
    - Response: `201 Created` `{ "ok": true }`
    - SSE: `input_pending` with `{ workflow_id, id, title?, box_id?, expires_at?, metadata? }`.
  - `POST /api/user-inputs/{workflowID}/respond`
    - Purpose: Deliver end user response to the waiting workflow.
    - Request:
      - `application/json`
      - `{ "id": string, "user_id": string, "fields": object, "metadata"?: object }`
    - Behavior: Sends signal to `user-response:<id>` with payload `{ fields, user_id, responded_at, metadata }`.
    - Response: `200 OK` `{ "ok": true }`
    - SSE: `input_completed` with `{ workflow_id, id, user_id, fields }`.
  - `POST /api/user-inputs/{workflowID}/cancel`
    - Purpose: Cancel a pending prompt.
    - Request:
      - `application/json`
      - `{ "id": string, "reason"?: string }`
    - Behavior: Cancel via typed workflow control; status upsert handled by workflow code or cancel handler where applicable.
    - Response: `200 OK` `{ "ok": true }`
    - SSE: `input_cancelled` with `{ workflow_id, id, reason }`.
  - `GET /api/user-inputs/{workflowID}` (id)
    - Include current search attributes, notably `id` and `InputStatus`.
  - `GET /api/user-inputs/pending`
    - Future: Query by search attributes (requires control list API); not mandatory for the keyed routing to function.

## SSE Events

- `input_pending`: `{ workflow_id, id, title?, box_id?, expires_at?, metadata? }`
- `input_completed`: `{ workflow_id, id, user_id, fields }`
- `input_cancelled`: `{ workflow_id, id, reason }`
- `heartbeat` and `connected` remain for stream lifecycles.

## Security and Validation

- Respond and cancel endpoints validate presence of `id` and reject if missing.
- Signals are only sent to `user-response:<id>`; no generic channels.
- `id` is unguessable (128-bit hash) yet deterministic; do not treat as secret. Use auth middleware for user identity.

## Testing Matrix

- Two concurrent inputs in one workflow → two distinct `id`s → two signals routed to correct waiter.
- Loop/iteration → increment `invokeSeq` deterministically → distinct `id`s across iterations.
- Reset to before signal → step waits again with same `id`; only new signal completes.

## Repository Changes (by Project)

This section enumerates concrete code changes required across `server/*` projects to implement the final keyed model.

### server/ops

- Input op (pkg/input)
  - Compute deterministic `id` and upsert typed search attributes when awaiting input.
  - Switch wait to keyed channel: `user-response:<id>`.
  - Remove legacy single-channel listening and any fallback.
  - Emit SSE via management service:
    - `input_pending` on prompt creation (via `POST /pending`), include `id`.
    - `input_completed` on response, include `id` and `fields`.
    - `input_cancelled` on cancel, include `id`.
  - Respond and cancel endpoints must require `id` in the request body.
- Utilities (new, package‑internal or shared)
  - Hash helper to produce `id` from `(recipeId, nodePath, invokeSeq, boxId, activityId)` using SHA‑256 → base32 (no padding, 128‑bit truncation).
  - Encapsulate typed search attribute upserts for readability.

### server/recipe-worker

- Populate deterministic invocation context for ops using recipe‑core APIs (no direct dependency on server/ops):
  - Compute:
    - `nodePath`: stable path for the current node (`NodeMetadata.ID` chain; fallback to structural indices like `seq[2]`, `state[approved]`).
    - `invokeSeq`: deterministic per‑`nodePath` counter in workflow‑local state.
    - `recipeId`: from recipe metadata.
  - Create `ops.InvocationContext` (from recipe‑core) and attach to `workflow.Context` via `ops.WithInvocationContext(ctx, inv)` before invoking the op’s inline handler.
  - Optionally precompute `id` via `ops.ComputeDeterministicKey(...)` and set it on the invocation context to avoid duplication at the op.
  - This is a general mechanism for all ops, not input‑specific.

### server/recipe-core

- Define shared deterministic invocation context (consumed by both recipe‑worker and server/ops):
  - `type InvocationContext struct { RecipeID string; NodePath string; InvokeSeq int; ID string; BoxID string; ActivityID string }`
  - Accessors for Temporal workflow context:
    - `func WithInvocationContext(ctx workflow.Context, inv *InvocationContext) workflow.Context`
    - `func GetInvocationContext(ctx workflow.Context) (*InvocationContext, bool)`
  - Accessors for standard context (activities), for completeness:
    - `func WithInvocationContextStd(ctx context.Context, inv *InvocationContext) context.Context`
    - `func GetInvocationContextStd(ctx context.Context) (*InvocationContext, bool)`
- Provide a reusable helper for key generation:
  - `ops.ComputeDeterministicKey(recipeId, nodePath string, invokeSeq int, boxId, activityId string) string` → base32(SHA‑256) over tuple (128‑bit truncation).
  - This centralizes hashing/encoding so worker and ops never diverge on the formula.

### server/api

- Management service endpoints already exist; update to final keyed contract:
  - Require `id` in bodies for `POST /respond` and `POST /cancel`.
  - `POST /pending` must accept `id` and broadcast `input_pending` including `id`.
  - Ensure tests cover keyed routing (no single‑channel fallbacks).
- Integration tests
  - Update WorkflowTestSuite‑based tests to include `id` in respond/cancel bodies and verify signal routing to `user-response:<id>`.
  - Cover concurrent prompts to validate correct correlation.

### Dependency Boundaries

- server/recipe-core exposes neutral APIs and types (InvocationContext, key helper) used by both server/recipe-worker and server/ops.
- server/recipe-worker is responsible for computing deterministic values and attaching them to workflow.Context; it does not depend on server/ops.
- server/ops consumes the context via recipe-core accessors in its inline handler to get `ID` (or compute it once if `ID` is not prefilled), and routes signals / SAs accordingly.

### OpenAPI generation (api/openapi, server/openapi, be-openapi, fe-openapi)

- Update schemas for the three endpoints to include `id` and new response shapes.
- Regenerate server and frontend clients.

### Optional: Querying pending

- If we add a control/list API in `workflowctl`, implement `GET /pending` by querying typed search attributes for `InputStatus=pending` and return `{ workflow_id, id, title?, box_id?, expires_at? }`.
