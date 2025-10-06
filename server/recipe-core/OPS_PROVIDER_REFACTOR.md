# Ops Provider Refactor Plan

This note captures the work required to move the recipe-core operation registry from storing concrete `RegisterableOp` instances to storing provider factories that can materialize ops with access to `ServiceDependencies2`. The change removes the shared global instance pattern so that each runtime (API management, workers, tests) can obtain a fresh op tied to the dependencies it actually has available.

## Why We Need Providers
- `ops.RegisterableOp` instances are currently created during process init and pushed into a global registry. Management services are only initialized if `SetupOps` is called, so most ops never see `ServiceDependencies2`.
- The management service lifecycle is tied to a single shared instance; we cannot create per-runtime state or instantiate ops that expect to read dependencies during construction.
- Future ops (and several existing ones) will need real dependencies (e.g., workflow control, git access controllers, SSE brokers) during op creation, not only during execution.
- Registering providers instead of concrete ops lets API/server components pass in the right dependency container and receive both an op and a purpose-built management service instance.

## Provider and Op Interfaces
Introduce a new provider abstraction in `server/recipe-core/pkg/ops`:

```go
// OpProvider carries static metadata and can materialize an op on demand.
type OpProvider interface {
    GetName() string
    GetMetadata() OpMetadata
    ExecuteAsActivity() bool
    GetInputStruct() interface{}
    GetInputType() reflect.Type
    GetOutputType() reflect.Type

    // Returns a concrete op bound to the provided dependencies and an optional
    // management service instance. The op must never be nil, and any management
    // service must already be fully initialized when returned.
    GetOpAndManagementService(ServiceDependencies2) (RegisterableOp, ManagementService, error)
}
```

`RegisterableOp` keeps only execution concerns and exposes a way to recover provider metadata:

```go
type RegisterableOp interface {
    ExecuteV2(inv Invocation, ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
    ExecuteInlineV2(inv Invocation, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input map[string]interface{}) (map[string]interface{}, error)

    // Allows runtime code to recover provider metadata when only the op instance is held.
    GetProvider() OpProvider
}
```

`ManagementService` can drop `Initialize` because providers will perform any setup before returning the service:

```go
type ManagementService interface {
    GetRoutes() []Route
    Close()
}
```

Key points:
- Providers expose the static metadata the registry and schema builders need, so recipe parsing and validation remain dependency-free.
- `GetOpAndManagementService` receives `ServiceDependencies2`, builds a fresh op instance that is fully wired for the current runtime, and returns an already-initialized management service (when applicable). The method returns an error when required dependencies are missing so callers can fail fast instead of silently registering a broken op.
- Keeping a back-reference via `RegisterableOp.GetProvider()` means downstream code that only sees concrete ops can still read metadata (type, version, descriptions) without needing to duplicate fields on the op itself.

## Registry API Changes
The registry in `pkg/ops` will manage providers only (no compatibility shims):
- Replace `Register(ops ...RegisterableOp)` with `RegisterProviders(providers ...OpProvider)`.
- `List()` becomes `ListProviders()` and returns `[]OpProvider`. Update schema generation and YAML helpers to iterate providers.
- Add `GetProvider(name string) (OpProvider, bool)` for static lookups by type or alias.
- Update `Clear()` and `Size()` to operate on the provider map.

Internal helpers such as `ops.GetWorkflowControl` stay untouched; they operate on the dependency container supplied to the provider.

## Recipe-Core Surface Adjustments
- `pkg/recipe/node.go` and `pkg/recipe/schema.go` should query providers via `GetProvider`/`ListProviders`. `checkOpInputs` can grab the provider, call `GetInputStruct`, and avoid instantiating the op during YAML unmarshalling.
- Tests under `pkg/recipe` and `pkg/ops` need to register lightweight test providers (wrapping existing inline/activity constructors) instead of concrete ops.
- Update documentation (`AGENTS.md`, schema specs, visitor docs) to describe provider registration and the `GetProvider` back-reference.

## Runtime Initialization Flow
- `server/api/internal/opssetup.SetupOps` should iterate providers via a new `opsexport.GetAllProviders()`. For each provider, call `GetOpAndManagementService(deps)` to receive a concrete op plus an already-initialized management service, register the op with runtime registries, and wire up the returned routes.
- Worker registries (recipe-worker) should call `GetOpAndManagementService` with the worker’s dependency container and register the resulting ops/activities directly—no separate registry `Instantiate` helper is required.

## Impacted Go Projects & Required Changes
The following Go modules must be updated to honor the provider model in a single coordinated change:

- `server/recipe-core`
  - Define `OpProvider`, adjust `RegisterableOp`, update `ManagementService`, and rewrite the registry APIs.
  - Update YAML unmarshalling, schema generation, and tests to consume providers.
  - Refresh developer docs to explain provider registration and the `GetProvider` link.

- `server/ops`
  - Convert exported helpers (`pkg/export`, `pkg/input`, `pkg/llm`, `pkg/recipe`, etc.) to return providers instead of concrete ops. Each provider’s `GetOpAndManagementService` should build the op (likely via existing constructors) and create/initialize any management service before returning.
  - Update extension discovery (`pkg/extensions`) to load provider factories from disk and register them with the new registry API.
  - Adjust tests to work with providers and the updated registry signatures.

- `server/git`
  - Update `pkg/export` and individual ops (`gitcollector`, `gitshallow`, `gitcommit`, `squashrebasemerge`, `thinpackrebase`) to expose providers that materialize ops on demand and surface the provider metadata through `GetProvider`.

- `server/recipe-worker`
  - Change `pkg/export` to publish providers.
  - Update `pkg/ops/activity_registry.go` to iterate providers, call `GetOpAndManagementService` with worker dependencies, and register the returned ops. Use `GetProvider` when metadata is needed during schema generation or validation.
  - Migrate tests and fixtures that call `recipeops.Register` to register providers instead.
  - Ensure worker-only ops that were previously constructed via `init()` now provide dependency-aware providers (e.g., command execution, sleep, HTTP helpers).

- `server/api`
  - Modify `internal/opssetup` to consume provider lists, instantiate ops with the HTTP server’s dependency container, and register management routes from the provider result instead of from the shared singleton.
  - Update any direct `ops.Register` usage to the new provider API.

- `server/cortex`
  - Mirror the API changes inside `internal/shared/registry.go` and CLI setup code: fetch providers, call provider factories with the CLI’s dependency container, and register/cleanup management services using the returned instances.

- `server/recipe-history` (tests only)
  - Adjust integration tests to register providers when exercising history translators that relied on `coreops.Register`.

## Provider Metadata Consumers
The current metadata consumers that motivated the old registry helpers are:
- **Schema generation (`server/recipe-core/pkg/recipe/schema.go`)**: enumerates ops to build JSON Schema for the UI-driven recipe editor.
- **YAML parsing (`server/recipe-core/pkg/recipe/node.go`)**: validates op inputs with provider metadata during unmarshalling.
- **Worker registration (`server/recipe-worker/pkg/ops/activity_registry.go`)**: inspects metadata to validate struct tags, build JSON schemas, and surface activity catalogs for tooling.
- **CLI validation (`server/cortex/internal/shared/validator.go`)**: relies on recipe metadata (and, transitively, provider metadata) to enforce required inputs when guiding users through prompts.

Because `RegisterableOp.GetProvider()` returns the provider instance responsible for an op, runtime code that only has the concrete op can still reach back to metadata without duplicating fields on the op itself.

## Migration Notes
- Perform the provider refactor in a single update rather than staging compatibility layers.
- Remove `ManagementService.Initialize`; providers must perform any necessary setup before returning the service.
- Ensure every provider returns a non-nil op from `GetOpAndManagementService`. If dependencies are missing, return an error instead of a partially constructed result.

