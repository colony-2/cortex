# SWF Migration - server/api

## Objectives
- Replace Temporal client usage with SWF engine interactions for recipe invocation, status, and input handling.

## Scope & Plan (Temporal usage to remove)
- `go.mod`/`go.sum` and test plans depend on Temporal SDK/server and `embeddedtemporal`; remove and swap to SWF engine.
- `cmd/testserver/main.go` wires embeddedtemporal + Temporal client; rewrite to use SWF harness and capability watchers.
- Docs (`EMBEDDED_TEMPORAL_MIGRATION_PLAN.md`, bug reports) reference embeddedtemporal/Temporal; replace with SWF equivalents.
- Any workflowctl/Temporal references in handlers need SWF job APIs instead.
- **Engine wiring**: initialize SWF engine (Postgres/Strata config) and inject into handlers/ops setup; share harness bootstrap for tests.
- **Endpoints**: start recipes as SWF jobs (return job IDs); status endpoints read `CheckJobStatus` + Strata chapters; input endpoints expose pending input tasks via `FindTasksWaitingForCapability` and allow completion via `TaskHandle.Finish`.
- **Testserver**: swap embeddedtemporal harness for SWF harness; wire capability watchers for input tasks.
- **Testing**: update integration tests to assert on job IDs and chapter content, not workflow/run IDs or search attributes.

## SWF Gaps Affecting This Project
- No query/search attributes; progress inferred from chapters + job status.
- Cancellation semantics limited to `CancelJob`; document lack of parent-close cascades.
