# Embedded Temporal Migration Plan for Input Route Tests

## Goals
- Execute integration tests against a real Temporal control plane using the `server/embeddedtemporal` package.
- Run the real recipe worker (from `server/recipe-worker`) so recipes, activities, and ops execute exactly as in production.
- Remove `suiteWorkflowCtl` summaries and rely on the actual Temporal client for `Describe` and `ListWorkflows`.
- Keep tests deterministic, isolated, and efficient.

## Approach Overview
1. **Provision Embedded Temporal Server per Test Suite**
   - Use `embeddedtemporal.NewServer` with SQLite backend and fast options.
   - Start the server in `TestMain` (or suite-level setup) for the handler package.
   - Register required namespaces (`default` + test-specific).
   - Ensure teardown stops the server and cleans up temp files.

2. **Run Real Recipe Worker**
   - Bootstrap the recipe worker component (`server/recipe-worker/cmd` or the library entry point) against the embedded server.
   - Register the full ops/recipes used in tests (including `input` op, `InputCollectionWorkflow`, etc.).
   - Manage worker lifecycle within the tests (start before executing scenarios, stop at teardown).

3. **Create Real WorkflowControl**
   - Reuse `server/recipe-worker/pkg/worker`'s `temporalWorkflowControl` by constructing a Temporal `client.Client` against the embedded server.
   - Wrap in a helper that simply provides namespace information; no summary caching.

4. **Test Harness Updates**
   - Replace `suiteWorkflowCtl` with a helper that:
     - Spins up embedded Temporal once per suite.
     - Starts the real recipe worker connected to that server.
     - Exposes a factory for `WorkflowControl` that hits the live Temporal client.
   - Update API handler tests to invoke recipes via the worker (e.g., calling the API to execute recipes, then asserting via real queries).
   - Update ops tests if they need the same harness; otherwise keep existing unit coverage.

5. **Recipe Execution in Tests**
   - Execute recipes through the Temporal client so that the worker runs the actual implementation (including `input` op, command execution stubs, etc.).
   - Wait for pending state using real `ListWorkflowExecutions` queries exposed via `WorkflowControl`.
   - Submit responses through the API under test, wait for completion, and assert SSE/pending outputs from the live Temporal state.

6. **Stability & Performance Considerations**
   - Use short timeouts and disable optional background services (scanners, etc.) to keep tests fast.
   - Provide per-test isolation by using unique workflow IDs / namespaces.
   - Ensure teardown propagation to avoid leaking worker goroutines or server processes.

7. **Incremental Rollout**
   - Introduce helper utilities in a new test support package (e.g., `internal/testtemporal`).
   - Port API handler tests first, ensuring they run via the real worker.
   - Verify via `go test ./server/api/internal/handlers`.
   - Remove unused `suiteWorkflowCtl` code.

8. **Risks & Mitigations**
- **Startup latency**: mitigate with cached binaries and minimal logging.
- **Resource contention**: randomize ports, ensure teardown cleans DB files and stops workers.
- **Flaky waits**: use Temporal SDK polling with explicit timeouts rather than arbitrary sleeps.

## Next Steps
- Implement shared embedded Temporal test harness.
- Refactor API tests to depend on the harness.
- Update ops tests if required.
- Remove legacy mocks and re-run full test suite.
