# SWF Migration - server/nucleus

## Objectives
- Update CLI to drive SWF jobs instead of Temporal workflows: start, status, logs/story rendering, and input completion.

## Scope & Plan (Temporal usage to remove)
- `go.mod`/`go.sum` depend on Temporal SDK/server and `embeddedtemporal`; remove.
- `internal/client` uses Temporal client creation; replace with SWF client wrapper.
- `internal/testutil/embedded.go` spins embeddedtemporal; swap to SWF harness.
- CLI flags/docs (`temporal-server`) must change to SWF endpoints.
- **Client layer**: build SWF client wrapper to start jobs, poll `FindTasksWaitingForCapability`, finish tasks, and fetch Strata chapters.
- **Commands**: `run` starts a job and optionally tails chapters; `status/logs` read job status + chapters; input commands list/complete pending input tasks.
- **Workflowctl compatibility**: provide shims where workflowctl is used in tests/scripts, delegating to SWF operations.
- **Testing**: port integration tests to SWF harness; assert on job IDs/chapter content rather than workflow/run IDs.

## SWF Gaps Affecting This Project
- No query API for arbitrary filters; CLI must filter via chapters or external indices.
- Cancellation differs; `cancel` maps to `CancelJob` without parent-close cascade.
