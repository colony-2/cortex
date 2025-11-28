# SWF Migration - server/ticket

## Objectives
- Remove Temporal error dependencies and ensure ticket ops run under the SWF task model with proper cancellation/timeouts.

## Scope & Plan (Temporal usage to remove)
- `pkg/op/op.go` uses `temporal.ApplicationError`; replace with local error taxonomy.
- `go.mod`/`go.sum` depend on Temporal SDK; remove.
- Docs (`TEMPORAL_TICKETS*.md`) are unrelated to Temporal.io but retain the word “temporal” (time-based); keep or clarify to avoid confusion.
- **Error handling**: replace `temporal.ApplicationError` with local errors encoding retryability for recipe-worker envelopes.
- **Execution context**: accept standard contexts from SWF job/task workers; enforce DB timeouts via context.
- **Dependencies**: verify ticket DB/service wiring in `ServiceDependencies2` without Temporal client assumptions.
- **Testing**: run op tests inside SWF harness; validate retryability flags and context cancellation behavior.

## SWF Gaps Affecting This Project
- No activity heartbeats; long DB operations must honor caller timeouts.
