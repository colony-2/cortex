**Temporal-Agnostic Orchestration Roadmap**

- Purpose: Define a provider-neutral orchestration surface that matches every Temporal feature used across `server/recipe-worker`, `server/ops`, `server/recipe-core`, `server/recipe-history`, `server/cortex`, `server/nucleus`, and `workflowctl`, enabling us to swap orchestration engines without invasive refactors.
- Scope: Workflow execution/runtime semantics, local and remote activity invocation, child workflows, signals and external workflow control, deterministic concurrency primitives, search attributes, workflow info/metadata, worker lifecycle, execution history/introspection, local testing, and embedded orchestration services.

**Temporal Usage Inventory (Feb 2025 audit)**

- `server/ops/pkg/recipe/op.go`: schedules child workflows, inspects `workflow.Execution`, and relies on Temporal retry/parent-close options.
- `server/ops/pkg/input/activity.go` & `workflow.go`: use typed search attributes, signal channels, external workflow signaling, cancellable contexts, and deterministic goroutines.
- `server/recipe-worker/pkg/compiler`: consumes `workflow.SideEffect`, `workflow.GetInfo`, `workflow.WithValue`, and Temporal application errors for validation (`execution_context.go`, `compiler.go`).
- `server/recipe-worker/pkg/gitstate`: depends on local activities (`workflow.ExecuteLocalActivity`), inline workspace helpers, and `workflow.WithLocalActivityOptions`.
- `server/recipe-core/pkg/ops` and tests: inline op DSL takes `workflow.Context`, Temporal retry policies, typed channels/selectors (`registerable_op.go`, `registerable_op_test.go`).
- `server/recipe-history/pkg/history` and `pkg/storybuilder`: use raw gRPC service clients, Temporal proto enums (search attributes, events), and history pagination.
- `server/cortex/internal/shared` & `server/nucleus/internal/client`: depend on Temporal workers/clients and the workflow testsuite harness.
- `server/embeddedtemporal`: packages a development Temporal server/client and dynamic config helpers that today leak Temporal options to consumers.

**Abstraction Layers**

- `orchestration/runtime`: workflow execution primitives, context propagation, deterministic concurrency, side effects, activities (remote & local), child workflows, signals, search attributes, and error helpers.
- `orchestration/worker`: worker factories, task queue bindings, registration metadata, lifecycle controls, interceptors, and telemetry hooks.
- `orchestration/history`: execution listing, description, history streaming, marker decoding, and payload conversion to provider-neutral types.
- `orchestration/control`: control-plane helpers for external signaling/cancellation (`workflowctl` replacement) and introspection utilities used by API handlers.
- `orchestration/testing`: test environment abstraction that replaces `go.temporal.io/sdk/testsuite` with provider-pluggable deterministic runners.
- `providers/<vendor>`: adapters that translate the neutral interfaces to Temporal, in-memory/local, or future engines (e.g. Cadence, Argo Workflows runtime shim).

**Runtime Interfaces (proposed)**

```go
package runtime

type FlowContext interface {
    Logger() Logger
    Now() time.Time
    Info() WorkflowInfo

    WithCancel() (FlowContext, CancelFunc)
    WithDeadline(time.Time) (FlowContext, CancelFunc)
    WithValue(key any, val any) FlowContext

    Sleep(d time.Duration) error
    Go(fn func(FlowContext))
    SideEffect(fn func(FlowContext) (any, error)) (Future, error)

    WithActivityOptions(ActivityOptions) FlowContext
    ExecuteActivity(name string, input any) (Future, error)

    WithLocalActivityOptions(LocalActivityOptions) FlowContext
    ExecuteLocalActivity(name string, input any) (Future, error)

    WithChildOptions(ChildWorkflowOptions) FlowContext
    ExecuteChildWorkflow(name string, input any) (ChildWorkflowFuture, error)

    GetSignal(name string) SignalChannel
    SignalExternal(ref ExecutionRef, signal string, payload any) (Future, error)

    NewChannel(capacity int) Channel
    Selector() Selector

    UpsertSearchAttributes(attrs ...SearchAttribute) error
}

type Future interface {
    Get(out any) error
}

// ChildWorkflowFuture mirrors Temporal's handle for child execution metadata.
type ChildWorkflowFuture interface {
    Future
    GetChildExecution(out *WorkflowExecution) error
}

type Channel interface {
    Send(ctx FlowContext, val any) error
    Receive(ctx FlowContext, out any) error
    Close()
}

type SignalChannel interface {
    Receive(ctx FlowContext, out any) error
}

type Selector interface {
    AddFuture(Future, func(FlowContext) error)
    AddReceive(Channel, func(FlowContext, any) error)
    Select(ctx FlowContext) error
}

// Activity, local activity, and child workflow configuration mirrors Temporal semantics.
type ActivityOptions struct {
    StartToClose time.Duration
    Retry        *RetryPolicy
    Heartbeat    time.Duration
}

type LocalActivityOptions struct {
    StartToClose time.Duration
    Retry        *RetryPolicy
}

type ChildWorkflowOptions struct {
    ID                string
    TaskQueue         string
    RunTimeout        time.Duration
    ExecutionTimeout  time.Duration
    Retry             *RetryPolicy
    ParentClosePolicy ParentClosePolicy
    IDReusePolicy     IDReusePolicy
}

type RetryPolicy struct {
    InitialInterval    time.Duration
    BackoffCoefficient float64
    MaximumInterval    time.Duration
    MaximumAttempts    int32
    NonRetryableErrors []string
}

// SearchAttribute represents our typed helper (replaces temporal.NewSearchAttributeKey*).
type SearchAttribute struct {
    Key   string
    Type  SearchAttributeType
    Value any
}

type SearchAttributeType string

const (
    SearchAttributeKeyword  SearchAttributeType = "keyword"
    SearchAttributeString   SearchAttributeType = "string"
    SearchAttributeInt      SearchAttributeType = "int"
    SearchAttributeDouble   SearchAttributeType = "double"
    SearchAttributeBool     SearchAttributeType = "bool"
    SearchAttributeDatetime SearchAttributeType = "datetime"
)

type ParentClosePolicy string

const (
    ParentClosePolicyTerminate      ParentClosePolicy = "terminate"
    ParentClosePolicyAbandon        ParentClosePolicy = "abandon"
    ParentClosePolicyRequestCancel  ParentClosePolicy = "request-cancel"
)

type IDReusePolicy string

const (
    IDReusePolicyAllowDuplicate       IDReusePolicy = "allow-duplicate"
    IDReusePolicyAllowDuplicateFailed IDReusePolicy = "allow-duplicate-failed"
    IDReusePolicyRejectDuplicate      IDReusePolicy = "reject-duplicate"
)

type ExecutionRef struct {
    WorkflowID string
    RunID      string
}

type WorkflowExecution struct {
    WorkflowID string
    RunID      string
}

type WorkflowInfo struct {
    WorkflowID   string
    RunID        string
    Attempt      int
    TaskQueue    string
    Namespace    string
    CronSchedule string
    Memo         map[string]any
}
```

Helpers retained from the original draft (`Logger`, `IsCanceled`, `NewError`, `IsNonRetryable`, `EncodedData`) continue to apply but now work across the expanded surface.

**History & Introspection Interfaces**

Reuse the neutral types from the earlier draft (`ExecutionService`, `ExecutionsPage`, `ExecutionDescription`, `HistoryIterator`, `HistoryEvent`, etc.) and extend event coverage to include signal markers and workflow metadata fields surfaced by `recipe-history` (`server/recipe-history/pkg/history/transformer.go`). Side effect markers remain represented via `EncodedData` so `server/recipe-core/pkg/story/markers.go` can deserialize provider-specific payloads.

**Worker & Control Plane Interfaces**

- `WorkerFactory`, `Worker`, `WorkflowRegistration`, and `ActivityRegistration` remain but add hooks for typed interceptors and middleware (used today by `server/recipe-worker/pkg/worker/worker_integration_test.go`).
- `ExecutionRef` in `orchestration/control` provides `{WorkflowID, RunID}` handles for external signaling/cancellation, matching `workflowctl.ExecutionRef` usage in `server/ops/pkg/input/management.go`.
- Control plane also exposes `Signal(ctx, ExecutionRef, signal string, payload any)` and `Cancel(ctx, ExecutionRef)` for API handlers.

**Testing & Local Execution**

- `LocalRuntime` / `LocalEnv` abstraction stays, but gains deterministic channel/signal mocks and the ability to pre-register search attribute factories so that tests such as `server/ops/pkg/input/keyed_routing_test.go` and `server/recipe-worker/pkg/compiler/sequence_integration_test.go` run without the Temporal testsuite.
- Provide a `FakeClock` option to advance timers deterministically, mirroring how the Temporal testsuite drives time.

**Provider Adapters**

- `TemporalAdapter`: implements the expanded interfaces by delegating to `workflow.Context`, `workflow.ExecuteActivity`, `workflow.ExecuteLocalActivity`, `workflow.ExecuteChildWorkflow`, `workflow.GetSignalChannel`, `workflow.UpsertTypedSearchAttributes`, selectors/channels, and search attribute builders. It also maps `workflow.SideEffect` futures (without context) into the `Future` contract.
- `LocalInMemoryAdapter`: offers an in-memory deterministic runtime for unit tests and CLI simulations. Supports signals, child workflow stubs, and typed search attribute storage.
- Future adapters (e.g. Cadence) must implement the same contracts; everything beyond the adapter remains provider-neutral.

**Migration Plan**

1. Introduce the new `orchestration` packages along with Temporal and in-memory adapters. Ship behind experimental build tags but maintain Temporal as the default provider.
2. Update `workflowctl` (`server/recipe-core/pkg/workflowctl`) to use `orchestration/control` so API layers stop importing Temporal client types directly.
3. Migrate `server/ops` inline ops and workflows:
   - Replace `workflow.Context` usage with `runtime.FlowContext`.
   - Swap `temporal.NewSearchAttributeKey*` with neutral `SearchAttribute` helpers.
   - Rewrite signal handling to use `SignalChannel`, `Selector`, and `ExecutionRef` interfaces.
4. Port `server/recipe-worker` compiler/runtime:
   - Replace direct calls to `workflow.SideEffect`, local activities, and child workflows with wrapper methods defined above.
   - Update `ActivityRegistry` to register via `orchestration/worker.Worker`.
5. Migrate `server/recipe-core` DSL and tests to depend on interfaces only, removing `go.temporal` imports from production code.
6. Refactor `server/recipe-history` to consume `orchestration/history.ExecutionService` instead of `sdk/client` + proto enums; ensure converters cover markers, search attributes, child workflow events, and signal history.
7. Switch `server/cortex` and `server/nucleus` CLIs to request workers/clients through the neutral factories. Delete Temporal-specific configuration structs once adapters land.
8. Rework `server/embeddedtemporal` so it implements the neutral provider contract (exposing an adapter plus dev server utilities) instead of leaking Temporal APIs.
9. Once consumers compile against the neutral surface, gate the Temporal adapter behind a feature flag and validate alternative runtimes.

**Open Questions / Follow-Ups**

- Do we need a typed payload codec abstraction for custom data converters (`server/recipe-history/pkg/storybuilder` currently depends on Temporal's converter API)?
- Should selectors expose a strongly typed API or stick with `any` payloads? Audit usage to decide (currently only used in tests).
- Assess performance implications of wrapping search attributes and futures; benchmark before flipping defaults.
- Investigate whether we need explicit support for Temporal features not yet used (signals with headers, queries) to future-proof the design.

This roadmap reflects the Feb 2025 repository inventory; revisit the usage audit whenever new Temporal features are introduced so the neutral surface stays aligned with real-world needs.
