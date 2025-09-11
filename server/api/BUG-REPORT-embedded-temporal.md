Note: This document tracks two separate issues observed with the embedded Temporal server: (1) shutdown/teardown latency and (2) startup readiness flakiness. The current blocker for input-management logs is the startup readiness flakiness. The shutdown section is included for completeness from prior investigations.

## Bug A: Embedded Temporal emits fatal during/after shutdown in tests

### Summary
When starting an embedded Temporal server for API integration tests and then stopping it, the process intermittently emits fatal logs about failing to create an SDK client (connection refused). The same behavior reproduces when running the embedded Temporal package tests directly. Functionally, the server starts quickly (1–2s) and serves clients, but shutdown triggers repeated warnings and a fatal in background services.

The larger issue is a long delay (~60 seconds) during/after shutdown before the process fully exits. The fatal is noisy but tolerable; the prolonged teardown is the primary concern because it slows tests and CI significantly.

### Affected Area
- Module: `server/embeddedtemporal`
- Consumers: `server/api` tests that initialize management services with a real Temporal client (e.g., input management service);
- Downstream UI uses the testserver successfully; issue is observed specifically in test shutdown path.

### Environment
- Go: as pinned by workspace
- Temporal Server (embedded): go.temporal.io/server v1.28.0
- SDK: go.temporal.io/sdk v1.35.0 (varies by module)
- Static hosts for service discovery; frontend bound to `127.0.0.1:<fixed-port>`

### Expected Behavior (Shutdown)
- `NewServer(...).Start()` should return when the server is ready; a client can connect and operate.
- `Stop()` should cleanly stop services without long-poll warnings or fatals; tests should complete without a 60s stall/fatal during teardown.

### Actual Behavior (Shutdown)
- Server starts and is ready (DescribeNamespace on `temporal-system` works).
- During/after test teardown, there is a prolonged delay (~60s) accompanied by repeated warnings and ending with a fatal, e.g.:

```
{"level":"warn","msg":"error creating sdk client","service":"worker","error":"failed reaching server: connection error: desc = \"transport: Error while dialing: dial tcp 127.0.0.1:<frontend-port>: connect: connection refused\"","logging-call-at":".../common/sdk/factory.go:98"}
{"level":"fatal","msg":"error creating sdk client","service":"worker","error":"failed reaching server: connection error: desc = \"transport: Error while dialing: dial tcp 127.0.0.1:<frontend-port>: connect: connection refused\"","logging-call-at":".../common/sdk/factory.go:109","stacktrace":"... service/worker/scanner ..."}
```

Additional logs often include:

```
{"level":"info","msg":"matching client encountered error","service":"frontend","error":"connection error: desc = \"transport: Error while dialing: dial tcp 127.0.0.1:<matching-port>: connect: connection refused\"","service-error-type":"serviceerror.Unavailable"}
{"level":"error","msg":"long poll to refresh Nexus endpoints returned error","error":"connection error: desc = \"transport: Error while dialing ... connect: connection refused\""}
```

This suggests background components (e.g., worker scanner, endpoint registry) attempt to create SDK clients or long-poll after the server has begun shutting down, eventually hitting connection refused and logging a fatal.

### Reproduction Steps (Shutdown)

1) Reproduce in embeddedtemporal module itself:

```
cd server/embeddedtemporal
go test ./... -v
```

Observed at end of `TestServerLifecycle`/others: repeated warnings and a fatal similar to the above, with a ~60s delay before the process fully exits.

2) Reproduce in API tests that use embedded Temporal (server/api):

- Tests located at `server/api/internal/handlers/input_routes_test.go`:
  - `TestInputRoutes_SSEConnect` and `TestInputRoutes_SimpleInputCycle`
  - Both start embedded Temporal with static hosts and fixed ports (e.g., 17237, 17238), create a real SDK client, run a minimal workflow/worker (for the simple cycle), then stop the server.

Command:

```
cd server/api
go test ./... -run TestInputRoutes_SimpleInputCycle -v
```

Result: functional assertions pass (server starts, SSE connects, respond endpoint works), but during teardown logs the warnings/fatal mentioned above.

### Notes on Setup Used (Shutdown)
- Static hosts (as implemented in `server/embeddedtemporal/pkg/temporal/server.go`) are respected: frontend/matching/history/worker all bind to 127.0.0.1 with deterministic ports.
- We avoid port 0 to prevent dynamic discovery and adhere to the static host setup.
- We use `NewServer(opts).Start()`; the server blocks until `waitForServerReady()` verifies readiness by connecting and describing the system namespace.
- For API E2E, we register only the necessary workflow (`InputCollectionWorkflow`) and start a minimal worker on a dedicated task queue.

### Hypotheses (Shutdown)
- The embedded Temporal server’s `Stop()` may not orchestrate a clean shutdown of all background goroutines (scanner, endpoint registry) that depend on creating SDK clients or long-polling endpoints, leading to retries that outlive the RPC listeners and eventually fatal.
- The worker service tries to create a “system client” late in shutdown (via `common/sdk/factory.GetSystemClient`) after frontend sockets are closed, causing fatal logging.
- A race between service shutdown order and SDK client factory initialization/teardown.

### Potential Directions to Investigate (Shutdown)
- Review shutdown sequence in embedded server: ensure components that perform client creation/long-poll (scanner, nexus endpoint registry, etc.) are stopped before RPC listeners are torn down.
- Consider disabling optional background services in the embedded server config for tests (e.g., internal worker scanner) if feasible.
- Provide a graceful shutdown path that cancels long-polls and prevents SDK client creation attempts after shutdown begins.
- Verify port binding/lifecycle in the static host setup during Stop(); ensure listeners remain available until all services have been signaled to stop and exited.

### References (Shutdown)
- Implementation: `server/embeddedtemporal/pkg/temporal/server.go` (static hosts via `temporal.WithStaticHosts`, readiness checks, namespace initialization).
- Repro tests: `server/embeddedtemporal/pkg/temporal/server_test.go` and `server/api/internal/handlers/input_routes_test.go`.

### Impact (Shutdown)
- Functional flows work; server starts quickly and API-level tests that use embedded Temporal can pass their assertions.
- The main problem is the ~60s teardown delay which slows test runs and CI pipelines substantially.
- The fatal/error logs on shutdown are noisy but not as critical as the prolonged teardown time.

### Request (Shutdown)
Please investigate the shutdown ordering in the embedded server and background services that attempt SDK client creation during/after Stop(). We need a clean Stop() path that avoids long-poll or client creation after listeners are closed so API integration tests remain quiet and reliable.

---

## Bug B: Embedded Temporal times out during startup (10s readiness window too short)

### Summary
The embedded Temporal server frequently fails to start in constrained environments due to a hard‑coded 10s readiness timeout. This causes `Start()` to return an error: `server failed to become ready: timeout waiting for server to be ready`. As a result, services (e.g., `server/api` testserver) that correctly wait for a Temporal client cannot initialize ops and exit.

### Affected Area
- Module: `server/embeddedtemporal`
- File: `server/embeddedtemporal/pkg/temporal/server.go`
- Function: `waitForServerReady()` (contains `timeout := time.After(10 * time.Second)`)

### Reproduction
Using the API testserver which exercises embedded Temporal:

```
cd server/api
timeout 45s go run ./cmd/testserver -p 8087 > /tmp/testserver_wait.log 2>&1 || true
tail -n 80 /tmp/testserver_wait.log
```

Observed (intermittent on shared/CI machines):

```
Using in-memory storage
… Temporal services boot logs …
Error: failed to start embedded Temporal server: server failed to become ready: timeout waiting for server to be ready
exit status 1
```

### Expected
`embeddedtemporal.Server.Start()` should wait long enough for services to accept connections and respond to a simple gRPC call (e.g., up to 30s by default).

### Actual
`Start()` returns after 10s with a timeout, even though services typically become ready shortly thereafter.

### Analysis
- `waitForServerReady()` enforces a 10s ceiling:
  ```go
  func (s *Server) waitForServerReady() error {
      backoff := 200 * time.Millisecond
      maxBackoff := 1 * time.Second
      timeout := time.After(10 * time.Second)
      // ... poll TCP + GetClusterInfo until timeout ...
  }
  ```
- First‑time DB schema initialization and CPU‑limited environments can push Temporal startup just beyond 10s.
- Upstream callers cannot extend this window because `Start()` returns the error before they can attempt client dial retries.

### Proposed Fix
- Make readiness timeout configurable via `Options` (e.g., `ReadinessTimeout`), default 30s; or raise the hard‑coded timeout from 10s to 30s.
- Optionally add a TCP‑only warmup phase before gRPC to reduce early failures while services are not yet handling gRPC.

### Impact (Startup - 10s window)
- Flaky or failed startup for E2E/integration tests on CI; API servers depending on embedded Temporal fail to register routes and emit expected logs.

---

## Bug C: Embedded Temporal startup flakiness/latency (>60s) in constrained environments

### Summary
Even with `ReadinessTimeout` increased to 60s and with background features disabled, the embedded Temporal server intermittently fails to report readiness in time on constrained runners. In repeated runs (10x), startup consistently times out at ~60.7s. This causes `TestInputRoutes_SimpleInputCycle` to fail before a client/worker can be created, and blocks API routes that depend on a Temporal client.

### Preconditions in our tests
- Fresh SQLite DB per test (`t.TempDir()/temporal-e2e.db`), avoiding reuse side‑effects.
- Ephemeral frontend port (bind to `127.0.0.1:0`), avoiding port collisions.
- Background features disabled to reduce load:
  - `DisableScanners: true`
  - `DisableNexus: true`
  - `DisableParentClosePolicy: true`
- `ReadinessTimeout: 60 * time.Second`

### Reproduction
Command (run multiple times):

```
cd server/api
go test ./internal/handlers -run TestInputRoutes_SimpleInputCycle -count=10 -v \
  | awk '/^=== RUN|^--- (PASS|FAIL):/'
```

Observed on this runner (10/10 failures):

```
=== RUN   TestInputRoutes_SimpleInputCycle
--- FAIL: TestInputRoutes_SimpleInputCycle (60.70s)
… (repeated 10x)
```

Log tail (representative): server emits normal service boot logs (history/matching/frontend binding, schedulers starting), then `Start()` returns error: `server failed to become ready: timeout waiting for server to be ready`.

### Expected
Temporal becomes reachable for a lightweight health check well within 60s in a CI‑like environment when using SQLite and minimal services.

### Actual
Readiness regularly exceeds 60s on this runner, causing `Start()` to fail. This is with schema initialization, dynamic ports for non‑frontend services, and reduced background features.

### Analysis and hypotheses
- Readiness gate performs both TCP dial and a gRPC `GetClusterInfo` call. Early in warmup, TCP may succeed while gRPC remains unavailable. If service startup plus schema work exceeds 60s, readiness fails.
- First‑time SQLite schema setup may be heavy on this environment (IO constrained). However, timeouts also occur on subsequent runs using a fresh temp DB, suggesting broader startup latency.
- Internal service initialization order may delay frontend’s ability to serve minimal APIs even after listeners bind.
- Dynamic config and background components are already reduced, but further reductions (e.g., starting only the minimal set of services necessary for health) may be needed.

### Suggestions / potential fixes
1) Make readiness health lighter:
   - Allow a TCP‑only readiness path (or gRPC channel state "READY") before requiring `GetClusterInfo`.
   - Expose a dedicated health endpoint in frontend for early readiness.
2) Reduce startup critical path when `Disable*` toggles are set:
   - Option to start only `frontend` service for readiness, then lazily bring up `history`/`matching`.
   - Further lower background initialization when `EnableInternalWorker=false`.
3) Optimize SQLite path:
   - Provide an in‑memory visibility option or tuned pragmas for first‑run schema creation.
   - Optionally pre‑generate schema or cache initialized DB for tests.
4) Observability:
   - Add structured timestamps for major init phases (DB schema start/end, service graph init, RPC listeners bound, health probe first success) to pinpoint bottlenecks.
5) Config knobs:
   - While optimizing, consider raising default readiness to 90–120s under `DisableScanners` mode to avoid CI flakiness.

### Impact
- Blocks API tests that require an embedded Temporal client (routes not registered; input management logs absent).
- Adds significant flakiness and runtime to CI pipelines.

### Workarounds attempted
- Increased `ReadinessTimeout` to 60s: still failing 10/10 here.
- Ephemeral ports + fresh DB + feature disables: still failing.
- Recommendation until fixed: set readiness to 90–120s in CI. However, the underlying startup latency should be reduced; 60s+ is unexpected for a minimal dev server.
