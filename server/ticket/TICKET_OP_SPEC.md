# Ticket Ops Specification

## Goals
- Provide a first-class recipe operation for managing tickets without forcing workflows to call the REST API directly.
- Support common lifecycle actions (create, update, stage/state transitions, document attachment) via a single op surface that maps cleanly onto `server/ticket` service methods.
- Ensure automation can run idempotently by leaning on optimistic versioning and event append semantics already implemented by `server/ticket`.

## Non-Goals
- Replacing the REST API or exposing every ticket capability on day one (e.g. resets, history queries). Those stay in dedicated endpoints for now.
- Building UI flows or Temporal workflows; this spec only covers the reusable op that recipes can embed.
- Introducing new ticket service features. We reuse existing service contracts.

## New Op Overview
- **Type:** `ticket.manage`
- **Location:** `server/ticket/pkg/op`
- **Registration:** exported via `ticketop.GetOp()` and appended to the list returned by `server/ops/pkg/export.GetAll()`.
- **Execution mode:** activity (`ops.NewActivityMappedOpV2`) because we talk to the database and emit events.
- **Timeout:** default 2 minutes (overridable by recipes).
- **Dependencies:** a `ticket.Service` plus its backing `ticket.Store`/`ticket.EventStore`. The op keeps a package-level reference initialized during worker/API startup.
- **Context propagation:** every invocation returns a `context_patch` map that merges into the recipe context under `context.ticket.*`.
- **Batch semantics:** a single invocation accepts an ordered list of actions applied sequentially to the same logical ticket. Later actions can depend on state created by earlier ones (e.g., create → update → attach doc).

### Dependency Wiring
1. Add `ticketop.SetService(s ticket.Service)` / `ticketop.Service()` helpers guarded by `sync.RWMutex`. Panic if the service is missing at invocation time to catch deployment misconfigurations early.
2. Extend `server/api/internal/opssetup.SetupOps` to construct the ticket service once (using the existing DB handles) and call `ticketop.SetService` before registering ops.
3. Provide `ticketop.TestingStub` so tests can swap in a fake implementation without leaking globals across runs.

## Input Envelope
```go
type Input struct {
    TicketID        string          `json:"ticket_id,omitempty"`
    Defaults        Defaults        `json:"defaults,omitempty"`
    Actions         []Action        `json:"actions"`
}

type Defaults struct {
    TicketActor     *Actor          `json:"ticket_actor,omitempty"`
    EventActor      *Actor          `json:"event_actor,omitempty"`
    EventTime       *time.Time      `json:"event_time,omitempty"`
    Workflow        *WorkflowPayload `json:"workflow,omitempty"`
}

type Action struct {
    Type            ActionType      `json:"type"`
    // Type-specific fields live alongside Type in the JSON payload.
}

type ActionType string

const (
    ActionCreateTicket        ActionType = "create_ticket"
    ActionUpdateTicket        ActionType = "update_ticket"
    ActionAppendTicketNote    ActionType = "append_ticket_note"
    ActionLinkMarkdown        ActionType = "link_markdown_doc"
    ActionOverrideMarkdown    ActionType = "override_markdown_doc"
    ActionRemoveMarkdown      ActionType = "remove_markdown_doc"
)
```
- `TicketID` identifies the target ticket when known. When omitted, the first `create_ticket` action generates the ID and subsequent actions in the batch reference that in-memory value.
- `Defaults` provide shared actor/time/workflow data; individual actions can override any field.
- `Actions` must be non-empty. The op executes them in order, short-circuiting on the first failure.

### Action Shapes
Each array element uses a discriminated-union schema based on `type`:

| Type | Additional Fields | Notes |
|------|-------------------|-------|
| `create_ticket` | `cell` (string, required), `title` (string, required), `stage` (string, required), `state` (string, required), `description` (string), `actor` (Actor) | Must be the first action if present. Produces a ticket slice and assigns `TicketID` for later actions. Stage normalized and state validated via service helpers. |
| `update_ticket` | `expected_version` (int64, required), `stage` (string), `state` (string), `description` (string), `actor` (Actor) | Requires an existing ticket (from input or prior create). At least one of `stage`, `state`, `description`, or `actor` must be supplied. Uses optimistic locking via `expected_version`. |
| `append_ticket_note` | `note` (string, required), `actor` (Actor), `event_time` (RFC3339 string), `workflow` (WorkflowPayload) | Appends a ticket event with note payload. `event_time` defaults using `Defaults.EventTime` or current time. `workflow` falls back to defaults. |
| `link_markdown_doc` | `name` (string, required), `path` (string, required), `reason` (string), `actor`, `event_time`, `workflow` | Emits `MarkdownDocAttached`. |
| `override_markdown_doc` | same fields as link | Emits `MarkdownDocOverridden`. |
| `remove_markdown_doc` | `name` (string, required), `path` (string, required), `actor`, `event_time`, `workflow` | Emits `MarkdownDocRemoved`. |

### Actor Handling & Defaults
- **Ticket mutations** (`create_ticket`, `update_ticket`) use the action-level `actor` when provided, otherwise `Defaults.TicketActor`, otherwise a generated automation actor derived from `ops.Invocation` metadata (cell, workflow name, node path hash).
- **Event actions** pick the action-level `actor`, then `Defaults.EventActor`, then fall back to the ticket-actor resolution above. This keeps creators distinct from artifact contributors when recipes specify both.

### Event Association Semantics
- The ticket service decides which log slice receives an event by inspecting `EventTime` relative to reset metadata. The op forwards `event_time` from the action (or defaults/current clock) without enforcing additional version constraints so concurrent stage updates do not block documentation events.
- `workflow` metadata, when present, maps to the workflow payload embedded in the resulting ticket event so downstream consumers can correlate artifacts with Temporal runs.

## Output Contract
```go
type Output struct {
    Ticket        *ticket.Ticket   `json:"ticket,omitempty"`
    Results       []ActionResult   `json:"results"`
    ContextPatch  map[string]any   `json:"context_patch,omitempty"`
}

type ActionResult struct {
    Type          ActionType           `json:"type"`
    Ticket        *ticket.Ticket       `json:"ticket,omitempty"`
    Event         *ticket.TicketEvent  `json:"event,omitempty"`
    EffectiveTime time.Time            `json:"effective_time"`
}
```
- `Results` align index-for-index with the input `actions` array, making it simple for recipes to correlate outputs.
- `Ticket` at the top level reflects the latest slice after the final action that mutated the ticket row (if any).
- `ContextPatch` merges into the recipe context (`context.ticket.*`), capturing final ticket state and the most recent event metadata.

### Context Propagation Details
- The patch always includes `ticket.id`.
- After ticket mutations we set `ticket.stage`, `ticket.state`, `ticket.version`, and `ticket.updated_at` based on the returned slice.
- After event actions we set `ticket.last_event_id`, `ticket.last_event_kind`, and `ticket.last_event_time`. If a ticket mutation already updated stage/state earlier in the batch we retain those values and avoid re-reading.
- Recipe steps can chain multiple batches; new context patches overwrite previous values in Temporal inline execution.

## Handler Flow
1. **Resolve Service & Defaults:** ensure a ticket service has been injected; normalize default actors/time/workflow.
2. **Bootstrap Ticket ID:** if the batch starts with `create_ticket`, capture the generated ID; otherwise require `TicketID` in the input and fetch the current slice when needed (for validation or context enrichment).
3. **Iterate Actions:** for each action in order:
   - Decode into the appropriate struct, applying defaults and validation rules.
   - Dispatch to the corresponding service method, tracking the resulting ticket slice or event. On failure return immediately with partial results populated for completed actions.
   - Append an `ActionResult` capturing the returned payload and chosen effective time.
4. **Update Context Patch:** merge ticket/event metadata after each action so later actions and the final output share consistent state.
5. **Logging & Metrics:** emit structured logs per action (action, ticket_id, actor_type, duration) and increment `ticket_manage_action_total{action, status}` counters. Aggregate latency across the batch for overall op timing.

## Error Mapping
- `ticket.ErrVersionConflict` → Temporal non-retryable, reason `VERSION_CONFLICT` (surfaced by relevant action result index).
- `ticket.ErrInvalidState`, `ErrInvalidActor`, `ErrEmptyTitle`, `ErrEmptyStage` → non-retryable, reason `BAD_REQUEST`.
- `context.Canceled`, `context.DeadlineExceeded` propagate untouched.
- All other errors wrap into retryable Temporal application errors, allowing the default retry policy to apply.

## Observability
- Structured logs per action and per batch with fields: `action`, `ticket_id`, `actor_type`, `effective_time`, `duration_ms`, `batch_index`.
- Metrics: latency histogram `ticket_manage_duration_ms` (batch duration) and counter `ticket_manage_action_total{action,status}`.
- Consider future trace spans (`ticket.action`) if we adopt OpenTelemetry instrumentation.

## Testing Plan
1. **Unit tests (`server/ticket/pkg/op/op_test.go`):**
   - Validate schema decoding for each action type and ensure defaults apply correctly.
   - Confirm mixed batches (create → update → markdown) execute in order and stop on first failure.
   - Verify event actions honour explicit `event_time` and `actor` without blocking on concurrent updates.
   - Assert context patch contents after multi-action batches.
2. **Integration tests (`server/ticket/pkg/op/op_integration_test.go`):**
   - Use the embedded Postgres harness to run end-to-end batches, checking ticket slices, event ordering, and reset-aware appends (by supplying backdated `event_time`).
   - Exercise workflows with and without `TicketID` on input to ensure create-first batches behave correctly.
3. **Recipe-worker schema tests:** extend `ActivityRegistry` coverage to ensure JSON Schema generation captures the discriminated-union action array (each branch with full `json` tags).

## Example Usage
```yaml
steps:
  - id: bootstrap-ticket
    op: ticket.manage
    input:
      defaults:
        ticket_actor:
          type: agent
          agent:
            workflow: build-ticket
        event_actor:
          type: agent
          agent:
            workflow: docs-sync
      actions:
        - type: create_ticket
          cell: core.build
          title: "Set up Buildkite pipeline"
          stage: triage
          state: waiting_capacity
        - type: update_ticket
          expected_version: 1
          stage: execution
          state: working
        - type: link_markdown_doc
          name: "Design Doc"
          path: docs/design.md
          reason: "Initial draft"
```

## Follow-on Items (Non-blocking)
- Add additional action types for change-set attachments, workflow status updates, or reset coordination once consumers require them.
- Evaluate exposing a read-only `list_events` op for automation auditing.
- Consider per-action retry policy overrides (e.g., `create_ticket` as non-retryable) once we have better idempotency markers.

