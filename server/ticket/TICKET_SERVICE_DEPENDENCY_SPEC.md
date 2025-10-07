# ServiceDependencies2 Refactor & Database Accessor

## Goals
1. Refactor `ops.ServiceDependencies2` so that only recipe-core constructs compliant instances via a fluent builder. This removes the need for every consumer to hand-roll structs whenever a new dependency is added.
2. Extend the interface with a generic `Database()` accessor for the shared GORM handle used by ticket operations (and future services).

## Current Pain Points
- Multiple packages (`server/api/internal/opssetup`, `server/cortex/internal/setup`, `server/recipe-worker/pkg/...`) each define their own struct implementing `ServiceDependencies2`. Adding a new dependency requires touching every implementation, increasing the risk of regression.
- Nothing prevents external packages from creating ad-hoc implementations; changes to the interface risk silent breakage when callers forget to wire new fields.
- Ticket-related code (and future services) lacks a typed way to request the common database connection from the dependency container.

## Proposed Refactor
### Interface Shape
```go
type ServiceDependencies2 interface {
    ServiceDependencies
    WorkflowControl() (workflowctl.WorkflowControl, bool)
    Database() (*gorm.DB, bool)
    serviceDependenciesMarker()
}
```
- `serviceDependenciesMarker()` is an unexported method. Only recipe-core types can satisfy the interface, forcing callers to use the provided builder.
- `Database()` returns the shared GORM handle and a boolean indicating availability.

### Builder API
Implement a fluent builder under `server/recipe-core/pkg/ops`:
```go
builder := ops.NewServiceDepsBuilder().
    WithWorkflowControl(ctl).
    WithDatabase(db).
    Build()
```
- Builder stores optional fields internally; new dependencies (e.g., `WithBlobStore`) can be added later without touching existing call sites.
- `Build()` returns the concrete `serviceDependencies` struct (kept internal) that satisfies `ServiceDependencies2`.
- To avoid double-builds, the builder can panic or return an error if `Build()` is called more than once.

### Behavioural Expectations
- `WithDatabase(nil)` is treated as "not provided"; `Database()` will return `(nil, false)`.
- Absent setters leave the corresponding accessor returning `(nil, false)` so existing call sites continue working until they opt-in to new dependencies.
- The builder should be cheap to copy/pointer for simple chaining.

## Migration Plan
1. **Introduce Builder & Interface Changes**
   - Add `serviceDependencies` struct inside recipe-core implementing the new interface, plus `NewServiceDepsBuilder()` and fluent setters.
   - Update interface definition with `Database()` + marker method and doc comments describing each accessor.

2. **Update Call Sites**
   Replace manual struct construction with the builder in all non-test packages that currently produce dependency containers:
   - `server/api/internal/opssetup`
   - `server/cortex/internal/setup`
   - `server/cortex/internal/shared/registry`
   - `server/recipe-worker/pkg/worker/deps.go`
   - `server/recipe-worker/pkg/worker/worker_manager.go`
   - `server/recipe-worker/pkg/ops/activity_registry.go`
   - `server/recipe-worker/pkg/compiler/invocation_tracker.go`
   - Any other package wiring dependencies (verify via `git grep "NewServiceDeps"` / `ServiceDependencies2`).

   Each call site becomes something like:
   ```go
   deps := ops.NewServiceDepsBuilder().
       WithWorkflowControl(ctl).
       WithDatabase(db).
       WithSSEManager(sse).
       Build()
   ```
   (Future `WithSSEManager`/`WithFoo` methods can be added as needed.)

3. **Adjust Delegates & Shims (Keep Only Where Necessary)**
   - Remove thin wrapper types that only existed to satisfy the interface—most callers can now hold the builder-produced value directly.
   - Retain `server/api/internal/opssetup.shimDeps` because it injects HTTP management routes and needs to continue translating extension routes while deferring dependency resolution to the underlying ServiceDeps. Update it to accept a builder-produced instance and forward typed accessors without additional logic.
   - Review other wrappers; if they merely re-export methods without adding behaviour (e.g., some legacy adapters in worker code), replace them with direct use of the builder output.

4. **Ticket Op Initialization**
   - `server/ticket/pkg/op` will obtain dependencies via the builder-produced container and should call `deps.Database()` during setup. If the DB is missing, registration must return an explicit error (e.g., `errors.New("ticket op requires database dependency")`).

5. **Testing Updates**
   - Refresh existing test doubles to either:
     - Use the builder directly, or
     - Embed the internal `serviceDependencies` while remaining in recipe-core test packages.
   - Add builder-focused unit tests covering default values, chaining behaviour, and the new `Database()` accessor.

6. **Documentation & Tooling**
   - Update internal design docs (`workflowctl-depre.md`, `OPS_EXECUTE_DEPS_EXTENSION.md`, etc.) to describe the builder pattern and the new accessor.
   - Add short usage examples to `server/recipe-core/AGENTS.md` or a new README section.

## Rollout Considerations
- This change breaks existing interface implementations until they migrate to the builder. Plan to land in a single PR/CL that touches all dependent packages plus their tests.
- Call sites that do not need a database can skip `WithDatabase`; downstream accessors must check the boolean return value.
- Future dependencies (blobstore, caching, etc.) will only require new `WithFoo` methods plus optional consumer updates; existing builder usage remains source-compatible.

## Open Questions
- Do we want separate `WithPrimaryDatabase` vs `WithDatabase` in the future if we introduce multiple handles? For now, a single `Database()` accessor is sufficient and keeps the API simple.
- Should management services receive the builder output directly or continue to use wrappers? Initial plan keeps existing wrappers but they can adopt the builder later for consistency.
# test
