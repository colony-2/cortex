# Ops Invocation Dependency Attachment

Goal: let every op execution access shared runtime services by attaching `ServiceDependencies2` to the existing `ops.Invocation` object. The execution signatures, registry structure, and schema tooling stay as they are today; ops simply read from `inv.Deps` when they require workflow control, git helpers, SSE brokers, or other injected services.

## Proposed Changes

1. **Invocation Struct**
   - Add a `Deps ServiceDependencies2` field to `ops.Invocation` in `server/recipe-core/pkg/ops/invocation.go`.
   - Mirror the field on `InvocationContext` if that helper is still used.
   - Document that `Deps` is transient and intentionally excluded from any hashing or serialization logic. The current `Hash()` implementation already relies only on deterministic fields, so no code changes are needed there beyond explanatory comments.

2. **Construction & Propagation**
   - Whenever an `Invocation` is built, populate `Deps` with the dependency container available in that runtime (API server, worker, CLI). Passing `nil` remains valid when no services are needed.
   - Ops that need injected services access them via `inv.Deps`; others ignore the field.
   - Runtime helpers such as the recipe-worker activity registry expose setters so dependency containers can be injected once and carried through every invocation.

3. **Lifecycle Considerations**
   - The dependency container remains owned by the runtime. It can be reused across invocations so long as it is safe for concurrent access.
   - Ops should guard against missing dependencies and report clear errors when required services are absent.

## Impacted Code Paths

Implementing this requires updates in multiple modules:

- `server/recipe-core/pkg/ops/invocation.go`: add the new field, ensure helpers like `InvocationContext()` and any copy constructors carry it forward, and comment on hash behavior.
- `server/recipe-core/pkg/ops/registerable_op.go`: adjust unit tests to assert that handlers can observe `inv.Deps`.
- `server/recipe-worker`
  - `pkg/ops/activity_registry.go`: set `Invocation.Deps` before calling `ExecuteV2`.
  - `pkg/compiler/compiler.go`: propagate dependencies during inline execution and nested recipe expansion.
  - Fixtures/tests that fabricate invocations.
- `server/api`
  - `internal/opssetup` or any management endpoints that execute ops must attach the server’s dependency container to the invocation.
- `server/ops`
  - Tests such as `pkg/input/keyed_routing_test.go` that call inline ops directly should set `Deps` (or pass `nil` explicitly).
- `server/recipe-history`
  - `pkg/history/integration_test.go`: update the test harness.
- `server/cortex` and other CLIs: propagate dependencies if they execute ops.
- Any additional utilities or tests that construct `Invocation` values will need similar tweaks.

## Migration Plan

1. Add `Deps ServiceDependencies2` to `Invocation` (and `InvocationContext` if necessary).
2. Adjust helper constructors and factory functions to accept and store the dependency container.
3. Rebuild the repo; fix compile-time errors by setting `Deps` at each construction site.
4. Update tests to supply either real dependency containers or `nil` when appropriate.
5. Document the new field in developer guides (`AGENTS.md`, worker specs, etc.).

## Expected Benefits

- Ops gain direct access to runtime services without altering their method signatures.
- Dependency wiring is localized to the point where invocations are created, offering a clear, consistent pattern across runtimes.
- Minimal disruption to existing registry and schema code paths, making the change straightforward to adopt.
