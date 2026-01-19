# Spec: Ticket Context (recipe-worker)

## Scope
Ensure the recipe worker preserves and forwards `JobContext.Ticket` through execution and template resolution.

## Integration Points
- `server/recipe-core/pkg/workflowctl/types.go` already carries `JobContext` in `StartJob`; no wire format change needed beyond the struct update.
- `server/recipe-worker/pkg/compiler/job_worker.go` should continue to pass `input.JobContext` into `ExecuteRecipe` without dropping the new `Ticket` field.

## Test Coverage
- Add a recipe-worker test fixture that sets `JobContext.Ticket` and resolves a template referencing `context.ticket.*`.
