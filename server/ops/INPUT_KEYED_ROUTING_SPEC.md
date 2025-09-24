# Deterministic Input Key + Keyed Signal Routing (Final State)

This spec defines a deterministic key for input coordination and a keyed-signal routing model across workflow, ops, and API layers. This corrects the current problem that inputs are not keyed.

## Overview

- Each input invocation in a workflow is assigned a deterministic, compact key (`id`).
- The workflow waits on a signal channel derived from the key: `user-response:<id>`.
- The API requires the same `id` when posting responses (and cancel requests).
- SSE events include the `id` so UIs can correlate prompts and responses.
- Typed search attributes store the `id` and other metadata for querying.

## Deterministic Key

- Inputs must be deterministically reproducible across replay; no random UUIDs.
- Use Invocation.Hash() from server/recipe-core as the input key (deterministically reproducible across replay)

- Determinism notes:
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
  - Encapsulate typed search attribute upserts for readability.


### server/api

- Integration tests
  - Update WorkflowTestSuite‑based tests to include `id` in respond/cancel bodies and verify signal routing to `user-response:<id>`.
  - Cover concurrent prompts to validate correct correlation.

### OpenAPI generation (api/openapi, server/openapi, be-openapi, fe-openapi)

- Update schemas for the three endpoints to include `id` and new response shapes.
- Regenerate server and frontend clients.

### Optional: Querying pending

- Implement `GET /pending` by querying typed search attributes for `InputStatus=pending` via workflowctrl and return `{ workflow_id, id, title?, box_id?, expires_at? }`.
