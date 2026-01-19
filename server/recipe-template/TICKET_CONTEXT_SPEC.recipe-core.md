# Spec: Ticket Context (recipe-core)

## Scope
Add a new top-level `ticket` field in `contextual.JobContext` and `contextual.TaskExecutionContext` so all execution paths can carry ticket metadata.

## Data Model
Introduce a new type in `server/recipe-core/pkg/contextual/context.go`:

```go
// TicketCreatorContext captures the actor details behind a ticket.
type TicketCreatorContext struct {
    Type  string                     `json:"type,omitempty"`
    User  *TicketCreatorUserContext  `json:"user,omitempty"`
    Agent *TicketCreatorAgentContext `json:"agent,omitempty"`
}

// TicketContext represents ticket metadata available to recipes.
type TicketContext struct {
    ID          string               `json:"id,omitempty"`
    Title       string               `json:"title,omitempty"`
    Description string               `json:"description,omitempty"`
    Creator     TicketCreatorContext `json:"creator,omitempty"`
    CreatedAt   time.Time            `json:"created_at,omitempty"`
    UpdatedAt   time.Time            `json:"updated_at,omitempty"`
}
```

Add fields:
- `Ticket TicketContext `json:"ticket,omitempty"`` to `JobContext`.
- `Ticket TicketContext `json:"ticket,omitempty"`` to `TaskExecutionContext`.

## Creator Shape
Use a structured object mirroring ticket actor payloads (serializes as a JSON map):

```json
{
  "type": "user" | "agent",
  "user": { "email": "user@example.com" },
  "agent": {
    "cell": "cell-name",
    "workflow_name": "workflow",
    "execution_id": "execution-id",
    "invocation_hash": "hash"
  }
}
```

Fields not applicable to the actor type can be omitted.

## Notes
- `created_at`/`updated_at` use `time.Time` and serialize as RFC3339 in JSON.
- Existing `context.actor.ticket_id` remains supported.
