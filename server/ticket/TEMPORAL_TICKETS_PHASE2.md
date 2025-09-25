# Ticket Temporal Storage – Phase 2: Service & API Updates

## Goals
- Update service/store logic to operate on temporal ticket slices.
- Modify `UpdateTicket` to close the current slice and insert a new one in a transaction.
- Emit internal ticket mutation events and remove the generic `AppendEvent` from the public interface.
- Introduce strongly typed append methods (`AppendWorkflowEvent`, `AppendMarkdownEvent`, `AppendChangeSetEvent`).
- Add point-in-time support for ticket searches (`SearchFilter.At`) and expose `GetTicketAt` service method.

## Tasks
1. Adjust ticket store to select current rows (`valid_until = 'infinity'`) by default and expose `GetAt(ctx, id, at)`.
2. Update service `CreateTicket`/`UpdateTicket` flows to manage temporal ranges (close old slice, insert new slice, emit ticket event).
3. Remove public `AppendEvent`; add per-payload append service methods with validation.
4. Extend search filters and tests to respect the temporal improvements.
5. Update unit/integration tests to verify temporal behaviour and new APIs.

Phase 2 builds on Phase 1 schema work and prepares for reset integration in Phase 3.

