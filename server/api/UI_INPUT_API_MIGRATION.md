# UI Migration: Input API (Old → New with Deterministic Keys)

This document explains the required UI changes to migrate from the legacy input API to the new keyed, deterministic input model.

## Summary of Changes

- Every input prompt carries a deterministic `id` provided by the backend.
- All user responses and cancellations must include this `id`.
- SSE events include `id` so the UI can correlate UI state and API calls.

## SSE Stream

- Old:
  - `input_pending` did not include a unique identifier; correlation was implicit.
  - `input_completed` did not include fields.
- New:
  - `input_pending` payload:
    - `{ workflow_id, id, title?, box_id?, expires_at?, metadata? }`
  - `input_completed` payload:
    - `{ workflow_id, id, user_id, fields }`
  - `input_cancelled` payload:
    - `{ workflow_id, id, reason }`

UI action:
- When receiving `input_pending`, create a prompt entry keyed by `id`.
- Use this `id` for the respond/cancel actions below.

## Respond Endpoint

- Old:
  - `POST /api/user-inputs/{workflowID}/respond`
  - Body: `{ fields: object, metadata?: object }`
  - Server inferred the target input (ambiguous when multiple pending).
- New:
  - `POST /api/user-inputs/{workflowID}/respond`
  - Body (required):
    - `{ id: string, user_id: string, fields: object, metadata?: object }`
  - Returns: `200 OK` `{ ok: true }`

UI action:
- Include `id` from `input_pending` SSE when posting.
- Provide `user_id` (as available from auth/user context).

## Cancel Endpoint

- Old:
  - `POST /api/user-inputs/{workflowID}/cancel`
  - Body: `{ reason?: string }` (implied target)
- New:
  - `POST /api/user-inputs/{workflowID}/cancel`
  - Body: `{ id: string, reason?: string }`
  - Returns: `200 OK` `{ ok: true }`

UI action:
- Include `id` from `input_pending` SSE when cancelling.

## Pending Registration (optional for orchestration)

- New endpoint emits `input_pending` explicitly when needed:
  - `POST /api/user-inputs/{workflowID}/pending`
  - Body: `{ id: string, title?: string, box_id?: string, expires_at?: string, metadata?: object }`
  - Returns: `201 Created` `{ ok: true }`

UI action:
- Typically not called by end-user UI; it’s for orchestration tools. If the UI is responsible for surfacing pre-existing prompts, listen to SSE and query details as needed.

## Details Endpoint

- `GET /api/user-inputs/{workflowID}` returns normalized status + attributes including `id` and `InputStatus`.

UI action:
- Use this to refresh a prompt’s state if reconciling after reconnect.

## Error Scenarios

- Missing `id` → server rejects with `400 Bad Request`.
- Unknown `id` → server returns `404 Not Found` or `409 Conflict` if already completed/cancelled.
- Timeout → prompt transitions to completed with error on the workflow side; UI should remove the prompt after receiving completion or after details show `InputStatus=timeout`.

## Quick Reference (New Contracts)

- SSE:
  - `input_pending`: `{ workflow_id, id, title?, box_id?, expires_at?, metadata? }`
  - `input_completed`: `{ workflow_id, id, user_id, fields }`
  - `input_cancelled`: `{ workflow_id, id, reason }`

- Respond:
  - `POST /api/user-inputs/{workflowID}/respond`
  - Body: `{ id, user_id, fields, metadata? }`

- Cancel:
  - `POST /api/user-inputs/{workflowID}/cancel`
  - Body: `{ id, reason? }`

- Pending (orchestration only):
  - `POST /api/user-inputs/{workflowID}/pending`
  - Body: `{ id, title?, box_id?, expires_at?, metadata? }`

- Details:
  - `GET /api/user-inputs/{workflowID}`
