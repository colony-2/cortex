# Bug: Embedded Temporal startup readiness flakiness/latency (>60s)

## Summary
In constrained environments (e.g., CI runners), the embedded Temporal server intermittently fails to become “ready” within 60 seconds even with background features disabled and with a fresh SQLite database. This causes `Server.Start()` to return an error before downstream code can initialize clients/workers, leading to test and server startup failures.

This report focuses solely on the startup readiness behavior (not shutdown/teardown latency).

## Affected Component
- Module: `server/embeddedtemporal`
- File: `pkg/temporal/server.go`
  - `Start()` calls `waitForServerReady()` which currently performs TCP+gRPC health probing until `Options.ReadinessTimeout` (defaults to 30s if unset).

## Environment & Options Used
- Backend: SQLite (file), fresh DB per run via `t.TempDir()`
- Host: `127.0.0.1`, ephemeral frontend port (avoid collisions)
- Options passed to `NewServer(Options{ ... })`:
  - `DisableScanners: true`
  - `DisableNexus: true`
  - `DisableParentClosePolicy: true`
  - `ReadinessTimeout: 60 * time.Second` (also reproduced at 30s; raising to 60s still flaked here)
  - `EnableInternalWorker: false` (default)

## Reproduction (via API test that uses embeddedtemporal)
Run a test that starts the embedded server, creates a client/worker, then exercises a workflow and HTTP endpoint:

```
cd server/api
# Run the Temporal‑dependent test repeatedly
go test ./internal/handlers -run TestInputRoutes_SimpleInputCycle -count=10 -v \
  | awk '/^=== RUN|^--- (PASS|FAIL):/'
```

Observed on a constrained runner (representative):

```
=== RUN   TestInputRoutes_SimpleInputCycle
--- FAIL: TestInputRoutes_SimpleInputCycle (60.70s)
... (repeated 10/10)
```

Log tail per run shows normal boot logs for history/matching/frontend (listeners created, schedulers started), then `Start()` returns with:

```
server failed to become ready: timeout waiting for server to be ready
```

Note: The test already uses an ephemeral frontend port and a clean DB dir per run to avoid port/DB reuse issues.

## Expected Behavior
With the above configuration and a minimal health probe, the server should report readiness well within 60s on modest CI runners, allowing clients to connect and tests to proceed.

## Actual Behavior
`waitForServerReady()` fails after 60s repeatedly, returning an error from `Start()`. Downstream code cannot initialize a Temporal client and the test/server fails to start.

## Analysis / Hypotheses
- Health probe requires gRPC `GetClusterInfo` (not just TCP). Early in warmup, TCP may succeed while gRPC remains unavailable. If service init + SQLite schema setup + subsystem warmup exceed 60s, readiness fails.
- First‑run SQLite schema creation cost may be significant on IO‑limited environments. However, timeouts also appear on repeated runs with fresh temp DBs, suggesting broader startup latency (e.g., internal service boot order or dependency readiness).
- Even with scanners/nexus/parent‑close disabled and internal worker off, the startup path may still block gRPC health for too long.

## Suggestions / Potential Fixes
1) Make readiness probing lighter, earlier:
   - Provide a dedicated light‑weight gRPC (or HTTP/TCP) readiness endpoint that becomes available sooner than `GetClusterInfo`.
   - Accept gRPC channel “READY” state (or a simple ping RPC) as readiness when disable flags are active.
2) Reduce critical path under test toggles:
   - When `DisableScanners`/`DisableNexus`/`DisableParentClosePolicy` are set, start only the minimum services required for basic client RPCs.
   - Consider deferring expensive background init until after readiness.
3) SQLite optimizations:
   - Pre‑init or cache a schema’d DB file for tests.
   - Tune pragmas further for first‑run throughput.
4) Observability:
   - Add structured timestamps for major phases (schema start/end, service graph init, frontend bind, health first success) to pinpoint bottlenecks.
5) Config knobs (temporary mitigation):
   - Allow higher defaults (e.g., 90–120s) in CI mode until startup path is optimized.

## Impact
- Temporal‑dependent API tests become flaky or fail outright on shared/CI runners.
- API servers that rely on embedded Temporal for management routes cannot register those routes; expected application logs (e.g., input management) are missing, complicating troubleshooting.

## Workarounds Tried
- Raised `ReadinessTimeout` to 60s, still failing 10/10 on this runner.
- Ephemeral ports, fresh DB per run, and disable toggles already in use.
- Temporary mitigation: set readiness to 90–120s in CI; however, underlying startup latency should be reduced.

