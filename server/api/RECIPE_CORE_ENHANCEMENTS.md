# Recipe-Core Integration Enhancements (Typed Workflow Control + Input Flow)

This spec documents recent enhancements that align recipe-core, ops, and the API management layer around a typed workflow control pattern and a clean input collection flow.

## Goals

- Remove string-key dependency lookups; use a typed accessor for workflow control.
- Make the input op record “pending” state and then wait for a real user response.
- Provide REST endpoints and SSE events to connect UI → management service → workflows.

## Typed Workflow Control (recipe-core)

- `ServiceDependencies2`: extends `ServiceDependencies` with a typed accessor `WorkflowControl() (workflowctl.WorkflowControl, bool)`.
- All components now retrieve workflow control via the typed accessor; no string keys.
- `workflowctl.WorkflowControl` is the portable interface (Describe/Signal/Cancel) implemented by runtimes or adapters (e.g., Temporal WorkflowTestSuite environments in tests).

## Input Op Execution Model (ops/input)

- The input operation now runs inline in a workflow (not as a standalone activity):
  - On start, it records “pending” via workflow search attributes: `InputStatus=pending`, plus form title, box ID, created/expires timestamps.
  - It waits for a `user-response` workflow signal with a timeout.
  - On timeout: updates `InputStatus=timeout` and returns a Temporal app error.
  - On response: updates `InputStatus=completed` and returns `Output{Fields, UserID, Metadata}`.
- Direct activity execution of the input op is disabled and returns an error to enforce the inline pattern.

## Management Service (ops/input)

### Endpoints

- `POST /api/user-inputs/{workflowID}/pending`
  - Records a newly created input request via SSE (see below).
  - Optional JSON body: `{ "title": string, "box_id": string, "expires_at": string, "metadata": object }`.
  - Emits `input_pending` SSE with available fields.

- `POST /api/user-inputs/{workflowID}/respond`
  - Existing endpoint to deliver a user response.
  - Accepts JSON: `{ "id": string, "fields": object, "metadata": object }`.
  - Uses typed workflow control to signal `user-response` to the workflow.
  - Emits `input_completed` SSE including submitted `fields` and `workflow_id`/`user_id`.

- `POST /api/user-inputs/{workflowID}/cancel`
  - Cancels the pending request (typed control if present, otherwise Temporal client).
  - Emits `input_cancelled` SSE.

- `GET /api/user-inputs/pending`
  - Placeholder list for pending inputs (will be backed by a control-level query when available).

- `GET /api/user-inputs/{workflowID}`
  - Returns a normalized status/summary via typed workflow control or Temporal client.

### SSE Events

- `input_pending`
  - Emitted by `POST /pending` to indicate a new input was created.
  - Data: `{ workflow_id, title?, box_id?, expires_at?, metadata? }`.

- `input_completed`
  - Emitted by `POST /respond` after signaling the workflow.
  - Data: `{ workflow_id, user_id, fields }`.

- `input_cancelled`
  - Emitted by `POST /cancel` after cancelling the execution.
  - Data: `{ workflow_id, reason }`.

## API/Testing

- The API integration tests now run against Temporal’s `WorkflowTestSuite` and use a suite-backed `workflowctl.WorkflowControl` adapter.
- Tests POST to the REST endpoints to complete the workflow, validating the full loop: workflow wait → management REST → workflow signal → completion.

## Migration Notes

- Direct activity execution of the input op was deprecated/removed; execute inline in a workflow and deliver responses through the management service.
- All string-key references (e.g., `Get("workflowctl")`) have been removed in favor of `ServiceDependencies2.WorkflowControl()`.
