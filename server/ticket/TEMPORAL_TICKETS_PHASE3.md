# Ticket Temporal Storage – Phase 3: Reset & Point-in-Time Events

## Goals
- Replace `ResetEvents` with `ResetTicket`, coordinating ticket temporal slices and event log updates in one transaction.
- Emit a dedicated `ticket_reset` event describing the rewind.
- Extend `TicketEventFilter` / `ListEvents` with point-in-time support and ensure queries return a consistent snapshot alongside temporal tickets.

## Reset Semantics
- `ResetTicket(ctx, id, input)` (supersedes `ResetEvents`) performs:
  1. Insert into `ticket_resets` (table is retained for bookkeeping and indexing).
  2. Mark affected events by setting `reset_id` (existing behaviour).
  3. Determine `reset_time` (defaults to now or the anchor event time).
  4. Close the current ticket slice (`valid_until = reset_time`).
  5. Reconstruct the ticket state as of the anchor and insert a new slice with `valid_from = reset_time`, `valid_until = 'infinity'`.
  6. Emit a `ticket_reset` event containing the reset metadata (ID, actor, reason, anchor event) so the log reflects the action.

## Event Log Adjustments
- `TicketEventFilter` gains `At *time.Time` (TIMESTAMP WITHOUT TIME ZONE).
- When `At` is specified:
  - Include events where `event_time <= At` and either `reset_id IS NULL` or the reset occurs after `At`.
  - Ticket queries already use temporal ranges; this keeps projections aligned.
- `ListEvents` accepts the updated filter, enabling consumers to replay the event stream at a chosen instant.

## Consistency & Snapshot Queries
- All reset operations execute in one transaction, ensuring the newly inserted ticket slice, the reset event, and the updated event records are atomically visible.
- Consumers can query tickets and events with the same `At` value to obtain a consistent historical view.

## Testing Strategy – Phase 3
- **Unit tests**: cover `ResetTicket` logic (event marking, emitted reset event, ticket slice reconstruction) and validation of the new inputs.
- **Integration tests**: run embedded Postgres scenarios that reset tickets at various anchors, query states before/after using `At`, and ensure consistent snapshots of tickets + events.
- **Regression**: ensure existing point-in-time queries remain stable and reset metadata is still accessible via `ticket_resets`.

## Open Questions
- Should the reconstructed ticket slice emit an additional ticket event for clarity? (Worth confirming during implementation.)
- How should clients consume the reset event in UIs/workflows (e.g. special styling or summarisation)?

