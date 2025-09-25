# Temporal Tickets – Reset-Safe Event Appends

## Overview
Phase 3 introduced transactional `ResetTicket` flows that rewind ticket slices and emit `ticket_reset` events. The append helpers (`AppendWorkflowEvent`, `AppendMarkdownEvent`, `AppendChangeSetEvent`) still operate on the legacy optimistic read ➜ write pattern: they resolve the *current* ticket slice, validate payloads, and persist events using the standalone event store connection. While this works for sequential traffic, concurrent resets can interleave between the read and write, leaving misclassified events (e.g. an event applied to the slice that is being closed or to a timeline that is immediately reset). This spec proposes reset-aware guards so append operations either serialize with the reset or fail fast.

## Goals
- Ensure event appends are associated with the correct temporal slice even when a reset is racing.
- Guarantee that events landing after a committed reset are immediately linked to the new slice (or flagged as reset) without relying on client retries.
- Preserve existing public APIs and behaviour—callers continue to rely on `ResetID` rather than `EventTime` to identify active events.
- Scope ordering guarantees to a single ticket; cross-ticket ordering remains unchanged.

## Non-Goals
- Changing public append inputs or requiring callers to pass ticket versions.
- Altering the Temporal reset semantics introduced in Phase 3.
- Introducing per-event deduplication or idempotency tokens.

## Current Behaviour
1. `Append*Event` calls `store.Get` (non-transactional) to ensure the ticket exists and to hydrate the actor.
2. The helper writes the event via `events.Append`, which creates a new DB session.
3. `ResetTicket` runs within `store.WithTx`, updating ticket slices, creating the reset row, marking superseded events, and emitting the reset event using a transaction-scoped event store.

Race window: an append acquires the current slice (`valid_until = infinity`). Before the insert, `ResetTicket` closes this slice and commits. The append now persists an event that logically occurred *after* the reset, but its lifetime still points to the old slice (since it was read before the reset). Clients end up with an event immediately tagged by a reset, or — depending on timing — a validation that passes even though the slice moved backwards.

## Proposed Changes
### 1. Append-on-reset reconciliation
- Execute append logic inside `store.WithTx` to share a snapshot with the latest ticket slice.
- Select the ticket using `SELECT ... FOR UPDATE` to anchor the append on the active slice.
- Retrieve the most recent reset metadata for the ticket within the same transaction. Suitable “version key”: `(ticket_id, last_reset_created_at)` where `last_reset_created_at` is the `CreatedAt` of the latest `ticket_reset` row (or zero timestamp if none).
- Persist the event via the transaction-scoped event store (`eventstore.NewWithDB(txDB)`).
- If the latest reset’s `CreatedAt` (or the `ticket_reset` event’s `EventTime`) is strictly greater than the append’s effective ticket version (i.e. when the reset committed after the append code read the ticket), set the new event’s `ResetID` to that reset. This keeps `ResetID` authoritative even when `EventTime` is later than the reset.
- Do **not** reject the append—clients observe the event, but because `ResetID` is immediately populated it is filtered out under default `IncludeReset=false` semantics.

### 2. Version key definition
- Introduce a helper `loadTicketVersion(ctx, tx, ticketID)` that returns:
  - `sliceValidFrom` / `sliceValidUntil` for the current row.
  - `lastResetID` and `lastResetCreatedAt` (zero values when no reset exists).
- Treat `lastResetCreatedAt` as the guard value; any append operating inside the transaction compares it after all writes. If `lastResetCreatedAt` changes during the transaction (another reset commits first), the current transaction is serialized by Postgres row locks ensuring we observe the latest values before the insert.
- Using `CreatedAt` (rather than `EventTime`) ensures we never rely on client-supplied `EventTime` ordering.

### 3. Event association rules
- When inserting the event:
  - Set `event.ResetID = lastResetID` if `lastResetID` is non-nil and the event’s target slice `ValidFrom` predates the reset (which holds for the active slice immediately after reset).
  - Otherwise, leave `ResetID` null.
- The rest of the append behaviour (payload validation, actor normalization, ID generation) stays unchanged.

### 4. Store & service adjustments
- Extend the tickets store with an internal `GetCurrentForUpdate` helper returning the current slice and last reset metadata (via a join/subquery).
- Add an internal event-store helper to tag events with a supplied `ResetID` during insert.
- Maintain public interfaces; `Append*Event` signatures and return values do not change.
- Extend public ticket representations (service + API gateway) to embed the latest reset metadata so clients can reason about stream continuity without extra calls. Suggested fields: `LastResetID`, `LastResetAt` (timestamp), and optional `LastResetActor` for audit context. Ensure these fields are populated in `CreateTicket`, `UpdateTicket`, `GetTicketAt`, and search results.

### 5. Metrics & observability
- Emit debug logs when an append is auto-reset (i.e. `ResetID` assigned at insert time) to aid diagnosis.
- Optionally track a counter of “append-after-reset” occurrences.

## Testing Strategy
- **Unit tests**: simulate append operations that observe resets mid-transaction and ensure events are inserted with the latest `ResetID` without errors.
- **Integration tests**: orchestrate concurrent reset + append flows confirming the appended event appears with `ResetID` set and that consumers filtering on `IncludeReset=false` do not see it. Validate that the event `EventTime` can be greater than the reset’s event time while still being treated as reset.
- **Store tests**: verify the `GetCurrentForUpdate` helper locks the current slice and exposes the latest reset metadata, preventing cross-ticket contamination.

## Rollout
1. Land store lock support and guard helper behind unit tests.
2. Update append methods to use the transactional guard; maintain existing API surface.
3. Roll integration tests in CI to confirm no regressions.
4. Communicate new error contract to API consumers (e.g. API handlers should map `ErrTicketResetInProgress` to 409 Conflict).

## Open Questions
- Should we proactively set the appended event’s `EventTime` to `max(provided, lastResetCreatedAt)` to preserve chronological ordering, or simply document that `ResetID` is authoritative?
- How do we surface the “auto-reset” outcome to API consumers (e.g. additional metadata in the response)?
- Do we need background reconciliation to catch historic events inserted before this change?
