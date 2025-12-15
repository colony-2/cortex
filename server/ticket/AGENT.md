# VibeThis Ticket Agent Overview

The `server/ticket` project encapsulates the ticketing domain for VibeThis. It exposes a Go package (`pkg/ticket`) that other services can depend on to create, update, and search tickets tied to dependency graph cells. Tickets carry stage/state metadata, capture the current actor (user or automation), and enforce optimistic concurrency so that workflows can coordinate safely.

Key capabilities provided here:

- **Data model**: Types for stages, states, actors, and `Ticket` records, plus the event log schema (`TicketEvent`, `TicketReset`) with discriminator-backed payloads and per-kind embedded structures.
- **Storage layer**: GORM-backed repositories for both tickets and ticket events (`internal/store/tickets`, `internal/store/events`) that handle migrations, transactional updates/resets, and iterator-based search queries.
- **Service layer**: Business logic (`internal/service`) for ticket lifecycle operations and append/list/reset semantics on the event log, including validator-backed payload enforcement, automatic base58 ID generation, and helper constructors for common actors.
- **Public package**: `pkg/ticket` re-exports the domain types, error constants, service interfaces, and event utilities for downstream consumers.
- **Test harness**: Shared embedded Postgres utilities (`internal/testutil`) powering integration coverage for both ticket CRUD and event log scenarios.
Consumers should import `github.com/colony-2/colony2/server/ticket/pkg/ticket` to interact with the service and event APIs, relying on the provided `Service` interface, iterators, and helper constructors.

## Reset-aware API surfaces

Phase 3 introduced temporal ticket slices and reset-aware event handling. The public API now exposes last-reset metadata alongside each ticket and accepts append calls that safely coordinate with concurrent resets.

### Ticket fields
- `LastResetID *ticket.TicketResetID`: `nil` when the ticket has never been reset; otherwise the most recent reset identifier for this ticket slice.
- `LastResetAt *time.Time`: UTC timestamp of the latest reset. Populate on `CreateTicket`, `UpdateTicket`, `SearchTickets`, and `GetTicketAt` responses.

### Service behaviours
- `AppendWorkflowEvent`, `AppendMarkdownEvent`, `AppendChangeSetEvent` run inside a transaction when available. If a reset is committed between the read and append, the new event is created with `ResetID` set to the latest reset—callers still receive the event ID but it will be filtered out of default listings (`IncludeReset=false`).
- `ResetTicket` replaces `ResetEvents`, closing the active slice, inserting the rewind slice, and emitting a `ticket_reset` event with reset metadata.
- `SearchTickets` returns an iterator that lazily hydrates `LastResetID/At` so callers do not need extra queries.

### Example: listing tickets with reset metadata
```go
iter, err := svc.SearchTickets(ctx, ticket.SearchFilter{Cells: []core.CellName{"api"}})
defer ticket.MustCloseIterator(iter)
for {
    tkt, err := iter.Next(ctx)
    if errors.Is(err, ticket.ErrIteratorDone) {
        break
    }
    if tkt.LastResetID != nil {
        log.Printf("ticket %s reset at %s", tkt.ID, tkt.LastResetAt.Format(time.RFC3339))
    }
}
```

### Example: appending while handling resets
```go
evt, err := svc.AppendWorkflowEvent(ctx, ticketID, ticket.WorkflowEventInput{
    Actor:     ticket.NewAgentActor(cell, workflow, execID, hash),
    EventTime: time.Now(),
    Payload: ticket.WorkflowEventPayload{
        Type:       ticket.WorkflowEventRunning,
        WorkflowID: ticket.WorkflowID("wf-123"),
        RunID:      ticket.WorkflowRunID("run-1"),
    },
})
if err != nil {
    return err
}
if evt.ResetID != nil {
    // The append raced with a reset; the event is recorded but already marked inactive.
    log.Printf("workflow event auto-reset by %s", *evt.ResetID)
}
```

### Example: concurrent reset protection (Transactional)
Consumers coordinating larger sequences can ensure they share the transaction:
```go
err := svc.Store().WithTx(ctx, func(ctx context.Context, tx ticket.Store) error {
    // mutate ticket slice...
    _, err := svcInternal.AppendMarkdownEventInTx(ctx, tx, snapshot, ticketID, actor, body, time.Now())
    return err
})
```
Public clients normally rely on the high-level methods; the internal helper exists for service composition where the transactional store is available.
