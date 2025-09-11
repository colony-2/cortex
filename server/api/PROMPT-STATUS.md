# Troubleshooting Notes — Input Management Logs and Embedded Temporal Readiness

## Context / Goal
- We needed to see logs for the input management component while running the API server and when executing tests (especially `TestInputRoutes_SimpleInputCycle`).
- Temporal server logs are very noisy, so we filtered them to surface only relevant `input_mgmt.*`, `HANDLER_LOG`, and route logs.

## What We Learned
- The input management service panicked on initialization when `temporal_client` was a typed-nil. This prevented any `input_mgmt.*` logs from appearing.
  - Fix in API wiring: stop injecting typed-nil clients; initialize ops only with a real Temporal client.
  - Fix in opssetup: treat missing/nil dependencies as errors; do not mask with typed-nil.
- Middleware bug: `RouteLogging` could dereference a nil route and trigger a 500; fixed with a nil guard. `TestLogVisibilityInHandlers` now passes and handler logs are visible in test output.
- Tests using embedded Temporal:
  - We switched tests to use an ephemeral frontend port and a fresh SQLite DB per run to avoid collisions or reuse.
  - `TestInputRoutes_SimpleInputCycle` sets `ReadinessTimeout: 60s`. On this runner, startup still times out at ~60.7s (10/10 attempts).
- Embedded Temporal startup readiness is the current blocker. Even with scanners/nexus/parent-close disabled and fresh DBs, readiness can exceed 60s in constrained environments.

## Current Status
- Passing tests: `TestInputRoutes_Pending`, `TestInputRoutes_SSEConnect`, `TestLogVisibilityInHandlers`.
- Failing/flaky here: `TestInputRoutes_SimpleInputCycle` due to embedded Temporal readiness timeouts at 60s.
- Separate bug report filed dedicated to startup readiness flakiness:
  - `server/api/BUG-REPORT-startup-readiness.md` (repro steps, analysis, suggestions).
- The earlier shutdown/teardown latency investigation remains in `server/api/BUG-REPORT-embedded-temporal.md` (now clearly labeled as shutdown-focused).

## Practical Run Notes
- Filter noisy Temporal logs when running the server:
  - Pipe output through `grep -Evi 'temporal|ringpop|frontend|matching|history|rpc|tchannel|etcd|cluster|scanner|cassandra|sql|shard|visibility|blobject|membership'`.
- Input management logs to expect:
  - `input_mgmt.get_details: entry ...` on GET details
  - `input_mgmt.submit_response: pre_signal ...` on POST respond

## Next Steps / Recommendations
- For CI stability now:
  - Increase embedded Temporal readiness to 90–120s in tests (we used 60s; still timing out here).
  - Keep using ephemeral ports and fresh DB per run.
- For embedded Temporal improvements (longer-term):
  - Provide a lighter readiness probe (e.g., dedicated RPC) that becomes available earlier than `GetClusterInfo`.
  - Reduce the startup critical path when disable flags are set; consider deferring heavy init until after readiness.
  - Add timestamps for major init phases to pinpoint bottlenecks.
- Optional: make testserver wait (bounded) for a real Temporal client before ops initialization, and fail fast if not ready. This ensures input routes and logs are present whenever the server starts successfully.

## Quick Commands
- Run readiness-sensitive test 1x with output:
  - `cd server/api && timeout 120s go test ./internal/handlers -run TestInputRoutes_SimpleInputCycle -v`
- Run log visibility test:
  - `cd server/api && go test ./internal/handlers -run TestLogVisibilityInHandlers -v`

