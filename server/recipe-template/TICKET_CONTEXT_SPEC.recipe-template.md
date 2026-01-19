# Spec: Ticket Context (recipe-template)

## Scope
Expose `context.ticket` in template resolution (CEL + interpolation) so recipes can read ticket metadata.

## CEL Environment
Update `server/recipe-template/pkg/template/template_resolver.go` to register the new type:
- Add `contextual.TicketContext{}` to `ext.NativeTypes`.

The `context` binding already uses `contextual.TaskExecutionContext`, so `context.ticket` becomes visible after the struct addition.

## Template Example
```yaml
inputs:
  ticket_id: "{{ context.ticket.id }}"
  ticket_title: "{{ context.ticket.title }}"
  ticket_creator_email: "{{ context.ticket.creator.user.email }}"
  created_at: "{{ context.ticket.created_at }}"
```

## Validation
- Add/extend a resolver test to assert `context.ticket.*` resolves.
