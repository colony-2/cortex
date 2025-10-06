# Ops Execution Dependency Injection Enhancement

This proposal enhances the existing recipe-core execution pipeline by threading `ServiceDependencies2` through op execution. Many ops need access to runtime services (workflow control, git helpers, SSE brokers, etc.) but today only management services see those dependencies. By extending the execution signatures, we give ops the ability to read from the same dependency container without reworking the registry.

## Objective

Allow every op execution—activity or inline—to access shared runtime services via `ServiceDependencies2`, while keeping the registration model, schema tooling, and metadata flow unchanged.

## API Changes

Update `ops.RegisterableOp` in `server/recipe-core/pkg/ops/registerable_op.go`:

```go
type RegisterableOp interface {
    ExecuteV2(inv Invocation, deps ServiceDependencies2, ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
    ExecuteInlineV2(inv Invocation, deps ServiceDependencies2, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, input map[string]interface{}) (map[string]interface{}, error)

    GetMetadata() OpMetadata
    GetName() string
    ExecuteAsActivity() bool
    GetInputStruct() interface{}
    GetInputType() reflect.Type
    GetOutputType() reflect.Type
    GetManagementService() ManagementService

    isOpSpec()
}
```

Existing callers add one more argument, typically the dependency container already available in scope. Ops that do not need shared services can ignore the new parameter.

## Handler Type Updates

Extend the handler aliases so op constructors receive the dependency container as well:

```go
type ActivityHandlerV2[In any, Out any] func(inv Invocation, deps ServiceDependencies2, ctx context.Context, in In) (Out, error)

type InlineHandlerV2[In any, Out any] func(inv Invocation, deps ServiceDependencies2, wctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, in In) (Out, error)
```

All invocations of `NewActivityMappedOpV2`, `NewActivityMappedOpWithManagementV2`, `NewActivityMappedOpWithProviderV2`, `NewInlineOpV2`, and `NewInlineOpWithManagementV2` must be updated so their handler functions accept the additional `deps ServiceDependencies2` argument. Most existing handlers can ignore it; they just add `_` to their parameter list.

## Implementation Details

1. **`opSpecImpl` execution methods**
   - Change method signatures to accept `deps ServiceDependencies2`.
   - When calling the stored handler, pass the dependency argument through. No other structural changes are required.

2. **Handler Definitions**
   - Update every handler implementation to match the new alias signatures. Handlers that require dependencies can now read from `deps`; handlers that do not can disregard it.

3. **Management Services**
   - No changes; they already receive `ServiceDependencies2` during initialization. Ops now have parity at execution time.

## Call Site Updates

All places that call `ExecuteV2` or `ExecuteInlineV2` must pass a dependency container:

- `server/recipe-worker/pkg/ops/activity_registry.go`: supply the worker’s dependency bundle when scheduling activities.
- `server/recipe-worker/pkg/compiler/compiler.go`: ensure inline execution and nested recipe steps forward dependencies.
- `server/ops/pkg/input/keyed_routing_test.go`: adjust tests invoking inline ops directly.
- `server/recipe-core/pkg/ops/registerable_op_test.go`: update unit tests to use `nil` or stub dependencies.
- `server/recipe-history/pkg/history/integration_test.go`: extend the history test harness with a suitable container.
- Any other utilities or tests that call the execution methods directly should be updated during the compile-error sweep.

## Dependency Container Guidance

- Runtimes should keep a single dependency container (implementing `ServiceDependencies2`) and pass it through each op invocation.
- For environments without real services (most unit tests), pass `nil` or a minimal stub that implements `ServiceDependencies2` but returns `(nil,false)` for optional resources.
- Ops that require specific services should validate them early and fail with actionable errors.

## Migration Plan

1. Modify the interface, handler aliases, and `opSpecImpl` signatures.
2. Rebuild and fix compile errors by threading dependency containers through all execution call sites and handler implementations.
3. Update helper constructors or test utilities to accept the new parameter.
4. Refresh documentation (`AGENTS.md`, worker specs, this file) to mention the extended signature and provide examples.

## Expected Outcomes

- Ops gain direct access to shared runtime services during execution without altering JSON schema generation, registration, or management service setup.
- Ops remain singletons; they read dependencies from the container instead of storing global state.
- The change is incremental: existing behavior is preserved, and ops that do not need dependencies simply ignore the new argument.
