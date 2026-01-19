# Spec: Ticket Context (be-api)

## Scope
Populate `JobContext.Ticket` when starting workflows from API-facing entry points so templates can access ticket metadata.

## Responsibilities
- When a workflow is started from a ticket, map ticket data into `JobContext.Ticket`:
  - `ticket.id -> JobContext.Ticket.ID`
  - `ticket.title -> JobContext.Ticket.Title`
  - `ticket.description -> JobContext.Ticket.Description`
  - `ticket.creator -> JobContext.Ticket.Creator` (object per creator shape)
  - `ticket.created_at -> JobContext.Ticket.CreatedAt`
  - `ticket.updated_at -> JobContext.Ticket.UpdatedAt`

## Notes
- This is additive to existing `context.actor.ticket_id`.
- Ensure any API path that constructs a `workflowctl.StartJob` includes the ticket context if a ticket is present.
