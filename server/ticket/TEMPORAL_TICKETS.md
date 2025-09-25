# Ticket Temporal Storage & Versioning Spec

## Goals
- Store tickets using temporal versioning with `valid_from` / `valid_until` **timestamp without time zone** columns so we can query historical snapshots exactly.
- Ensure a single active ticket row per `ticket_id` by leveraging PostgreSQL `btree_gist` exclusion constraints.
- Make `CreateTicket`/`UpdateTicket` write temporal slices, automatically emitting ticket mutation events on updates.
- Offer point-in-time (`At`) queries for both tickets and events, yielding consistent state views.
- Replace `ResetEvents` with `ResetTicket`, extending the reset workflow to also materialise the ticket state as of the reset anchor in a new temporal row.
- Provide payload-specific append APIs while reserving ticket-mutation events for the service itself.

## Temporal Table Design
- Add `valid_from` / `valid_until` columns of type `TIMESTAMP WITHOUT TIME ZONE` to `tickets`.
- Enable `btree_gist` and add `EXCLUDE USING gist (ticket_id WITH =, tsrange(valid_from, valid_until) WITH &&)` so temporal slices cannot overlap for the same ticket.
- Treat rows with `valid_until = 'infinity'` as the current state.

## Service Workflow Updates

### Create Ticket
- Insert new row with `valid_from = now()`, `valid_until = 'infinity'`.

### Update Ticket
- Run inside a transaction:
  1. Select current row (FOR UPDATE) and confirm optimistic lock `version` matches.
  2. Set current row’s `valid_until = now()`.
  3. Insert new row with merged fields, `valid_from = now()`, `valid_until = 'infinity'`.
  4. Emit a **ticket event** capturing the field changes automatically (no public API exposure) using the existing event log, with the same transaction timestamp.

### Append Event APIs
- Introduce explicit methods per payload type, e.g. `AppendWorkflowEvent`, `AppendMarkdownEvent`, `AppendChangeSetEvent`. Each accepts a strongly-typed payload and wraps the lower-level call.
- The generic `AppendEvent` disappears from the public surface; ticket mutation events are generated internally by `UpdateTicket`.

### Reset Event Entry
- When `ResetTicket` runs (see below), emit a `ticket_reset` event containing:
  - the `reset_id` (referencing `ticket_resets`)
  - actor & reason
  - anchor event pointers (`last_valid_event_id`, optional)
- This event stays visible in the log and provides the narrative context for the rewind; downstream projections can still filter superseded events by `reset_id`.

## Event Log & Point-in-Time Queries
- Extend `TicketEventFilter` (used by `ListEvents`) and ticket `SearchFilter` with `At *time.Time` (timestamp without time zone).
- When `At` is supplied:
  - Events: `event_time <= At` AND (`reset_id IS NULL` OR reset occurs after `At`).
  - Tickets: select rows where `valid_from <= At` AND `At < valid_until`.
- This guarantees a consistent snapshot across tickets and events.

## Reset Semantics (`ResetTicket`)
- New signature replaces `ResetEvents` and accepts the same payload (plus optional `LastValidEvent`).
- Within a single transaction:
  1. Record reset metadata (`ticket_resets`).
  2. Mark `ticket_events.reset_id` for affected events as today (same as current logic).
  3. Determine reset instant (`reset_time`, defaulting to `now()` or the last event time).
  4. Close the current ticket temporal row by setting `valid_until = reset_time`.
  5. Reconstruct ticket state at the reset anchor (by replaying events up to `LastValidEvent` or the last non-reset event) and insert a new ticket row with `valid_from = reset_time`, `valid_until = 'infinity'`.
  6. Emit the `ticket_reset` event described above so the log captures the reset action itself.
- This ensures historical queries using `At` align across the event log and ticket table, even after resets.

## Consistency Guarantees
- All ticket updates, event mutations, and resets occur within a single transaction, guaranteeing atomicity.
- Using `TIMESTAMP WITHOUT TIME ZONE` avoids time-zone drift when comparing `At` filters across tickets and events.
- Point-in-time reads simply apply the `At` constraint to both tables, yielding the correct projection of rows & events.

## API Surface Additions
- `SearchTickets` / `SearchEvents` accept `At *time.Time`.
- New service method `GetTicketAt(ctx, id, at)` returns the ticket row whose temporal range contains `at`.
- Public service methods for payload-specific events: `AppendWorkflowEvent`, `AppendMarkdownEvent`, `AppendChangeSetEvent`. The generic `AppendEvent` is removed from the public interface in favour of these strongly-typed entry points and the internal ticket-mutation event emitted by `UpdateTicket`.

## Migration Plan
1. Enable `btree_gist` extension in the ticket database.
2. Add `valid_from`/`valid_until` columns, backfill current tickets with `valid_from = now()`, `valid_until = 'infinity'`.
3. Create exclusion constraint preventing overlapping ranges per `ticket_id`.
4. Update GORM models, service logic, and tests to use per-slice insertions.
5. Introduce the new append APIs and rename `ResetEvents` ➜ `ResetTicket` with the extended behaviour.

## Testing Strategy
- **Unit Tests**: extend service/store suites to verify temporal insertions (`UpdateTicket` closes prior slice, emits ticket event, and inserts new row) and payload-specific append helpers. Validate validator errors for invalid payloads.
- **Integration Tests**: add embedded Postgres scenarios that
  - Create, update, and query tickets at different `At` instants, confirming the correct temporal slice is returned.
  - Exercise `ResetTicket`, asserting events receive `reset_id`, the old ticket slice closes, and a new slice with reconstructed state exists.
  - Query events and tickets at arbitrary timestamps to ensure consistent snapshots before and after resets.
- **Regression Coverage**: ensure existing iterator/pagination flows still succeed with the new `At` filters and temporal constraints.

## Open Questions
- Decide whether the new ticket version inserted by `ResetTicket` should emit its own ticket event (likely yes, for audit symmetry).
- Clarify how clients obtain the reconstructed ticket state for the reset: via service helpers or by returning the new version directly.
- Ensure pagination strategies still work when filtering by `At` across temporal ranges.
