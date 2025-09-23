**Temporal-Agnostic Orchestration API (Proposal)**

- Purpose: Define a provider-neutral API surface that covers every Temporal API currently used by non-test code in `server/recipe-history` and `server/recipe-worker` so we can swap providers without invasive changes.
- Scope: Runtime workflow execution, activity invocation, worker lifecycle, retries/errors, context propagation, and execution history/inspection.

**Design Goals**

- Minimal surface: only what we actively use today.
- Deterministic workflow semantics preserved (sleep, timers, cancellation, logging, goroutines).
- Pluggable provider with clean adapters (Temporal, local/in-memory, other engines).
- Smooth migration: 1:1 mapping from current usages -> new interfaces.

**Modules**

- `orchestration/runtime` — Workflow runtime primitives (context, activities, timers, logging, error/retry types).
- `orchestration/worker` — Worker lifecycle and registration.
- `orchestration/history` — Execution listing, description, and history streaming.

**Core Types & Interfaces**

- `type FlowContext interface {`
  - `Logger() Logger` — Structured logger.
  - `WithCancel() (FlowContext, CancelFunc)` — Derive cancelable child context.
  - `Sleep(d Duration) error` — Deterministic timer.
  - `Go(fn func(FlowContext))` — Deterministic goroutine/spawn.
  - `WithActivityOptions(opts ActivityOptions) FlowContext` — Bind activity policy.
  - `ExecuteActivity(name string, input any) (Future, error)` — Schedule activity; returns future.
`}`

- `type Logger interface { Debug(msg string, kv ...any); Info(msg string, kv ...any); Warn(msg string, kv ...any); Error(msg string, kv ...any) }`

- `type Future interface { Get(ctx FlowContext, out any) error }`

- `type ActivityOptions struct {`
  - `StartToClose Timeout`
  - `Retry       *RetryPolicy`
`}`

- `type RetryPolicy struct {`
  - `InitialInterval Duration`
  - `BackoffCoefficient float64`
  - `MaximumInterval Duration`
  - `MaximumAttempts int32`
  - `NonRetryableErrorTypes []string`
`}`

- `func IsCanceled(err error) bool`
- `func NewError(kind, message string, cause error, opts ...ErrorOption) error`
- `func IsNonRetryable(err error) bool`

The above cover: activity execution, timers, cancellation, retries, and logging used by the compiler and state machine.

**Worker Lifecycle**

- `type Worker interface {`
  - `RegisterWorkflow(name string, fn any, opts WorkflowRegistration)`
  - `RegisterActivity(name string, fn any, opts ActivityRegistration)`
  - `Start() error`
  - `Stop()`
`}`

- `type WorkerOptions struct {`
  - `MaxWorkflowPollers int`
  - `MaxActivityPollers int`
`}`

- `type WorkflowRegistration struct { Name string }`
- `type ActivityRegistration struct { Name string }`

- `type WorkerFactory interface {`
  - `NewWorker(taskQueue string, opts WorkerOptions) Worker`
`}`

These cover: `worker.New`, `RegisterWorkflowWithOptions`, `RegisterActivityWithOptions`, `Start/Stop`, and poller limits.

**Local Execution (Standalone)**

- `type LocalRuntime interface {`
  - `NewEnvironment() LocalEnv`
`}`

- `type LocalEnv interface {`
  - `RegisterWorkflow(name string, fn any, opts WorkflowRegistration)`
  - `RegisterActivity(name string, fn any, opts ActivityRegistration)`
  - `SetContextPropagators(props []ContextPropagator)`
  - `ExecuteWorkflow(name string, input any)`
  - `GetWorkflowError() error`
  - `GetWorkflowResult(out any) error`
`}`

- `type ContextPropagator interface {`
  - `Inject(ctx context.Context, carrier map[string]string) error`
  - `Extract(ctx context.Context, carrier map[string]string) (context.Context, error)`
`}`

This replaces `sdk/testsuite` usage for `StandaloneExecutor` and preserves context propagation.

**Execution History and Introspection**

- `type ExecutionService interface {`
  - `ListExecutions(ctx context.Context, query ExecutionQuery) (ExecutionsPage, error)`
  - `DescribeExecution(ctx context.Context, workflowID, runID string) (ExecutionDescription, error)`
  - `History(ctx context.Context, workflowID, runID string, opts HistoryOptions) (HistoryIterator, error)`
`}`

- `type ExecutionQuery struct {`
  - `TaskQueue string`
  - `Status    ExecutionStatusFilter` // running|completed|failed|all
  - `PageSize  int32`
`}`

- `type ExecutionsPage struct { Executions []ExecutionInfo; NextPageToken []byte }`

- `type ExecutionInfo struct {`
  - `WorkflowID string`
  - `RunID      string`
  - `Type       string`
  - `Status     ExecutionStatus`
  - `StartTime  time.Time`
  - `CloseTime  *time.Time`
  - `HistoryLength int64`
  - `Metadata   map[string]any` // generalized search attributes
`}`

- `type ExecutionDescription struct {`
  - `Info              ExecutionInfo`
  - `PendingActivities []PendingActivity`
`}`

- `type PendingActivity struct {`
  - `ActivityType string`
  - `ActivityID   string`
  - `State        PendingActivityState` // scheduled|running|canceling
  - `ScheduledTime time.Time`
  - `Attempt      int`
`}`

- `type HistoryOptions struct { LongPoll bool; Filter EventFilter }`
- `type HistoryIterator interface { HasNext() bool; Next() (HistoryEvent, error) }`

- `type HistoryEvent struct {`
  - `ID        int64`
  - `Type      EventType`
  - `Time      time.Time`
  - `Attributes any` // one of ActivityScheduled|Started|Completed|Failed|TimedOut
`}`

- `type ActivityScheduled struct { ActivityType string }`
- `type ActivityStarted struct { ScheduledEventID int64 }`
- `type ActivityCompleted struct { ScheduledEventID int64; Result EncodedData }`
- `type ActivityFailed struct { ScheduledEventID int64; Message string }`
- `type ActivityTimedOut struct { ScheduledEventID int64 }`

- `type EncodedData struct { Raw []byte; Encoding string }` // provider-neutral payload

- `type ExecutionStatus string` with values: `Running|Completed|Failed|Canceled|Terminated|ContinuedAsNew|TimedOut|Unknown`
- `type PendingActivityState string` with values: `Scheduled|Running|CancelRequested|Unknown`
- `type EventType string` with values: `ActivityTaskScheduled|ActivityTaskStarted|ActivityTaskCompleted|ActivityTaskFailed|ActivityTaskTimedOut`

These cover the exact history/introspection data path used by `server/recipe-history`.

**Provider Adapters**

- TemporalAdapter implements:
  - `WorkerFactory` by wrapping `worker.New` and mapping `WorkerOptions`/registrations.
  - `FlowContext` by delegating to `workflow.Context` for timers, cancellation, futures, logging.
  - `Future` wrapping Temporal future `Get`.
  - `ExecutionService` by delegating to `client.Client` methods (`ListWorkflowExecutions`, `DescribeWorkflowExecution`, `GetWorkflowHistory`) and converting enums/structs to neutral types.
  - Error mapping between `temporal.ApplicationError` and `NewError` + `IsNonRetryable`.

- LocalInMemoryAdapter implements:
  - `LocalRuntime` for serverless execution (replacement for `sdk/testsuite`).
  - Optionally a `WorkerFactory` for embedded, test-only workers.

**Migration Map (Current Usage -> Proposed)**

- Runtime (compiler, state machine):
  - `workflow.Context` -> `FlowContext`
  - `workflow.ActivityOptions` + `workflow.WithActivityOptions` -> `ActivityOptions` + `FlowContext.WithActivityOptions`
  - `workflow.ExecuteActivity(...).Get(...)` -> `FlowContext.ExecuteActivity(...).Get(...)`
  - `workflow.GetLogger` -> `FlowContext.Logger()`
  - `workflow.WithCancel` -> `FlowContext.WithCancel()`
  - `workflow.Sleep` -> `FlowContext.Sleep()`
  - `workflow.Go` -> `FlowContext.Go()`
  - `temporal.RetryPolicy` -> `RetryPolicy`
  - `temporal.IsCanceledError` -> `IsCanceled`
  - `temporal.NewApplicationError`/`ApplicationError.NonRetryable()` -> `NewError(..., NonRetryable())` + `IsNonRetryable`

- Worker lifecycle:
  - `worker.New(client, taskQueue, worker.Options)` -> `WorkerFactory.NewWorker(taskQueue, WorkerOptions)`
  - `RegisterWorkflowWithOptions` -> `Worker.RegisterWorkflow(name, fn, WorkflowRegistration)`
  - `RegisterActivityWithOptions` -> `Worker.RegisterActivity(name, fn, ActivityRegistration)`
  - `Start/Stop` -> `Start/Stop`

- Local execution (StandaloneExecutor):
  - `testsuite.*` calls -> `LocalRuntime.NewEnvironment()` + `LocalEnv.*` methods
  - `SetContextPropagators([]workflow.ContextPropagator)` -> `SetContextPropagators([]ContextPropagator)`

- History/inspection (recipe-history):
  - `client.WorkflowService().ListWorkflowExecutions` -> `ExecutionService.ListExecutions`
  - `client.DescribeWorkflowExecution` -> `ExecutionService.DescribeExecution`
  - `client.GetWorkflowHistory` -> `ExecutionService.History`
  - `api enums/types` -> provider-neutral `ExecutionInfo`, `ExecutionStatus`, `PendingActivity`, `HistoryEvent`, etc.

**Incremental Adoption Plan**

1) Introduce new packages with adapters but keep Temporal as the backing provider.
2) Replace `activity_registry` registration signatures to accept `Worker` instead of the SDK-specific interface (it already abstracts via `ActivityRegisterable`).
3) Wrap `workflow.*` calls in small shim helpers (or directly switch to `FlowContext` where touched):
   - Start with `compiler` package: migrate timers, activities, retries.
4) Swap `worker_manager` to use `WorkerFactory` and `WorkerOptions`.
5) Introduce `ExecutionService` and migrate `recipe-history` client usages.
6) Keep `StandaloneExecutor` but implement it via `LocalRuntime` instead of `sdk/testsuite`.

**Non-Goals (for now)**

- Signals/queries, child workflows, cron schedules: not used today.
- Cross-namespace routing or sticky execution semantics: out of scope.

**Why This Covers Everything We Use Today**

- Activity execution and options: covered by `ActivityOptions`, `ExecuteActivity`, and `Future`.
- Timers/concurrency/cancellation/logging: covered by `FlowContext` methods and `Logger`.
- Retries/errors: covered by `RetryPolicy`, `IsCanceled`, `NewError`, `IsNonRetryable`.
- Worker creation/registration/lifecycle: covered by `WorkerFactory`, `Worker`, and registration structs.
- History/introspection: covered by `ExecutionService`, neutral enums/types, and `HistoryIterator`.
- Local/in-memory execution: covered by `LocalRuntime` and `LocalEnv`.

**Appendix: Example Handler Shapes**

- Workflow handler: `func(ctx FlowContext, inputs map[string]any) (map[string]any, error)`
- Activity handler: `func(ctx context.Context, input any) (any, error)`

