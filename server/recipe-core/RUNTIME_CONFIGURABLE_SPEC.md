# Runtime Configurable Ops Spec

## Context
Recipe ops currently rely on `Invocation` (for deterministic identity) and ad-hoc constructors to obtain runtime dependencies. Management services historically used string-key lookups to obtain shared resources, forcing callers to coordinate on magic strings and making it difficult to wire structured dependencies (e.g. database pools) safely. Execution handlers have no standard path to receive dependencies, leading to globals or attempts to push runtime state into `Invocation`, both of which violate Temporal replay guarantees.

## Goals
- Provide a first-class hook for ops to receive runtime dependencies assembled by recipe-worker or other hosts.
- Replace the string-key registry with typed accessor methods so dependency wiring is discoverable and type-safe.
- Allow multiple ops to participate in shared resources (e.g. pooled databases, caches) without channeling those resources through `Invocation` or activity payloads.
- Keep exposed HTTP routes aligned with the new pattern so both execution and management surfaces draw from the same dependency context with minimal bootstrap logic.

## Non-Goals
- Changing handler signatures (activity or inline) or modifying `Invocation`.
- Requiring every op to expose HTTP routes; the pattern should work for ops without web endpoints.
- Mandating a specific dependency implementation—the provider simply needs to satisfy the typed interface.

## Proposed Design

### 1. Typed Dependency Carrier Only
Retire the legacy `ServiceDependencies` interface and define a single typed surface in `server/recipe-core/pkg/ops/service_deps2.go`:

```go
type ServiceDependencies interface {
    WorkflowControl() (workflowctl.WorkflowControl, bool)
    DatabaseProvider() (db.Provider, bool)
    // Future accessors, e.g. Cache(), Logger(), etc.
}
```

All bootstrap code is updated to provide an implementation of this interface (or a superset). No key-based `Get(name)` remains in the API; new accessors are added explicitly as methods when additional dependencies are required. Ops call these methods during configuration to obtain strongly typed resources.

### 2. RuntimeConfigurable Interface
Introduce a lightweight interface in `server/recipe-core/pkg/ops/registerable_op.go`:

```go
type RuntimeConfigurable interface {
    ConfigureRuntime(deps ServiceDependencies) error
}
```

Any `RegisterableOp` that needs runtime dependencies implements this interface (either on the op wrapper or via embedding). `ConfigureRuntime` is invoked by bootstrap code whenever the dependency graph changes—at startup, during hot reload, or whenever new dependency instances are supplied. Ops treat each call as authoritative and rebuild internal state from the provided typed accessors.

### 3. Runtime Binder Utility
Provide a helper in `pkg/ops`:

```go
func ConfigureOpsRuntime(ops []RegisterableOp, deps ServiceDependencies) error
```

The helper iterates the slice, type-asserts `RuntimeConfigurable`, and invokes `ConfigureRuntime`. Because dependencies are typed, ops can immediately retrieve providers without fragile string switches. Repeated calls simply reconfigure ops with the latest object graph; errors bubble up to the caller.

### 4. HTTP Route Providers Build on Runtime Configuration
Ops expose HTTP routes through a capability interface once they have been runtime-configured. Rename the method on `RegisterableOp` and the backing interface to highlight their role:

```go
type APIRoutesProvider interface {
    Routes() []Route
}
```

- Replace `GetManagementService()` on `RegisterableOp` with `GetAPIRoutesProvider() APIRoutesProvider` (returning `nil` when no routes exist).
- During `ConfigureRuntime`, the op injects dependencies—obtained via typed accessors—into any internal route provider so it is ready when `GetAPIRoutesProvider()` is called. Because configuration may run multiple times, ops ensure their route providers refresh internal references when new dependency instances arrive.
- Bootstrap code simply iterates ops, calls `GetAPIRoutesProvider()`, and registers returned routes. If the provider implements `io.Closer` (or another shutdown interface), bootstrap code records cleanup callbacks.

Provide a helper:

```go
func CollectAPIRoutes(ops []RegisterableOp) (routes []Route, cleanup []func())
```

### 5. Lifecycle Expectations
- `ConfigureRuntime` may run multiple times; ops treat each invocation as a full refresh using the latest typed dependencies. They drop cached handles that are no longer valid and replace them with newly supplied instances.
- Ops should not retain the dependency carrier reference beyond configuration—capture concrete dependencies instead to avoid accidental mutation of the container.
- Route providers are expected to be ready immediately after each configuration. Cleanup remains the caller’s responsibility: if a provider implements `io.Closer`, bootstrap code tracks it and calls `Close()` during shutdown.

## Implementation Plan
1. Remove the legacy `ServiceDependencies` interface and update call sites to the new typed-only `ServiceDependencies` surface.
2. Add `RuntimeConfigurable` interface and helper functions to server/recipe-core in `pkg/ops`.
3.`GetAPIRoutesProvider()` and introduce the `APIRoutesProvider` interface; provide migration shims if absolutely necessary, but eliminate key-based management services.
4. Update existing ops (e.g. input forms) to implement `RuntimeConfigurable`, pull dependencies via typed accessors, and expose their HTTP handlers via `GetAPIRoutesProvider()`.
5. Modify worker/bootstrap code (recipe-worker, API gateway, cortex) to instantiate typed dependency carriers, call `ConfigureOpsRuntime` whenever dependencies are built or refreshed, then `CollectAPIRoutes` to register HTTP routes.
6. Update documentation (`VIBETHIS.md`, AGENTS.md) to describe the new pattern, including guidance for adding new typed accessors.
7. Add tests covering:
   - Configuring a mock op that implements `RuntimeConfigurable` and uses typed accessors.
   - Ensuring reconfiguration replaces prior dependency instances.
   - Verifying route providers are fully initialised without additional bootstrap calls.

## Open Questions
- Which dependencies should be surfaced first (database pool, cache, metrics, logging)?
- Should route providers embed `io.Closer`, or should cleanup remain an optional interface check?
- How should we handle ops that need optional dependencies—return `(nil, false)` or provide explicit “not configured” errors?

