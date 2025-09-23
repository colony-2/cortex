**Temporal API Usage (server/recipe-history + server/recipe-worker)**

- Scope: Non-test Go code under `server/recipe-history` and `server/recipe-worker`.
- Goal: Enumerate every Temporal API (Go SDK and API protos) referenced, with purpose and where used.

**Go SDK Packages**

- `go.temporal.io/sdk/client`
  - `client.Client` — Shared Temporal client held by services/workers.
    - `WorkflowService()` — Exposes raw gRPC workflow service client for advanced queries.
      - Used in `server/recipe-history/pkg/history/history.go` to list workflow executions.
    - `DescribeWorkflowExecution(ctx, workflowID, runID)` — Fetches execution summary/details.
      - Used in `.../history.go` to build detailed Job view.
    - `GetWorkflowHistory(ctx, workflowID, runID, isLongPoll, filterType)` — Iterates history events.
      - Used in `.../history.go` to collect activity executions from history.

- `go.temporal.io/sdk/worker`
  - `worker.New(client, taskQueue, worker.Options)` — Creates a worker bound to a task queue.
    - Used in `server/recipe-worker/pkg/worker/worker_manager.go`.
  - `worker.Options` — Configures concurrency/pollers, etc.
    - Used to set `MaxConcurrent{Workflow,Activity}TaskPollers` in `.../worker_manager.go`.
  - `worker.Worker` — Worker instance with control/registration methods.
    - `RegisterWorkflowWithOptions(fn, workflow.RegisterOptions)` — Registers workflow by name.
      - Used in `.../worker_manager.go`.
    - `Start()` / `Stop()` — Controls worker lifecycle.
      - Used in `.../worker_manager.go`.

- `go.temporal.io/sdk/workflow`
  - Workflow runtime primitives used inside workflows/compilation layer:
    - `workflow.Context` — Workflow execution context type.
      - Pervasive across `server/recipe-worker/pkg/compiler/*.go` and `.../worker_manager.go`.
    - `workflow.ActivityOptions` + `workflow.WithActivityOptions(ctx, opts)` — Configure activity execution (timeouts, retries) and bind to context.
      - Used in `server/recipe-worker/pkg/compiler/compiler.go` when invoking activities.
    - `workflow.ExecuteActivity(ctx, name, input)` — Schedules an activity and returns a `Future`.
      - `.Get(ctx, &out)` to retrieve results. Used in `.../compiler.go`.
    - `workflow.GetLogger(ctx)` — Structured logging within workflow code.
      - Used in `.../statemachine_compiler.go` and `.../compiler.go` for debug logs.
    - `workflow.WithCancel(ctx)` — Derives a cancelable child workflow context.
      - Used in `.../compiler.go` to implement timeout/cancellation envelopes.
    - `workflow.Sleep(ctx, d)` — Deterministic timer for delays and retry backoff.
      - Used in `.../compiler.go` for timeout and retry handling.
    - `workflow.Go(ctx, func(ctx workflow.Context){ ... })` — Starts a workflow goroutine.
      - Used in `.../compiler.go` to run timeout watchdogs.
    - `workflow.RegisterOptions` — Name/alias for registered workflows.
      - Used in `.../worker_manager.go` and `.../executor/standalone.go`.
    - `workflow.ContextPropagator` — Type for context propagation hooks.
      - Used in `.../executor/standalone.go` to set propagators in test env.

- `go.temporal.io/sdk/activity`
  - `activity.RegisterOptions` — Options for registering activities (e.g., name).
    - Used in `server/recipe-worker/pkg/ops/activity_registry.go`.
  - `RegisterActivityWithOptions(func, activity.RegisterOptions)` — Registration API (consumed via interface to support both real worker and test env).
    - Invoked in `.../activity_registry.go` through an `ActivityRegisterable` abstraction.

- `go.temporal.io/sdk/temporal`
  - `temporal.RetryPolicy` — Retry policy configuration used for activities/composites.
    - Built/converted in `server/recipe-worker/pkg/compiler/retry.go` and applied in `.../compiler.go`.
  - `temporal.IsCanceledError(err)` — Detects cancellation (e.g., due to timeout watchdogs).
    - Used in `.../compiler.go` retry/timeout envelope.
  - `temporal.NewApplicationError(message, type, cause...)` — Creates typed application errors.
    - Used in `.../compiler.go` to signal timeout and max-retry conditions.
  - `temporal.ApplicationError` — Type-asserted to check `NonRetryable()`.
    - Used in `.../compiler.go` to respect non-retryable failures.

- `go.temporal.io/sdk/testsuite` (used for local, serverless execution)
  - `testsuite.WorkflowTestSuite{}` — Test suite harness.
  - `NewTestWorkflowEnvironment()` — In-memory workflow environment.
  - `SetOnActivityCompletedListener(nil)` — Suppress test env debug logs.
  - `RegisterWorkflowWithOptions(...)` — Register workflow handler in test env.
  - `SetContextPropagators([]workflow.ContextPropagator)` — Configure propagators.
  - `ExecuteWorkflow(name, inputs)` — Run workflow by registered name.
  - `GetWorkflowError()` / `GetWorkflowResult(&out)` — Retrieve results.
    - All used in `server/recipe-worker/pkg/executor/standalone.go` to execute recipes without a Temporal server.

**gRPC/API Proto Packages**

- `go.temporal.io/api/workflowservice/v1`
  - `ListWorkflowExecutions(ctx, *ListWorkflowExecutionsRequest)` — Lists workflows by query.
    - Called via `client.WorkflowService()` in `server/recipe-history/pkg/history/history.go`.
  - `DescribeWorkflowExecutionResponse` — Response message consumed when describing executions.
    - Transformed in `server/recipe-history/pkg/history/transformer.go`.

- `go.temporal.io/api/workflow/v1`
  - `WorkflowExecutionInfo` — Execution summary: IDs, type, status, times, search attributes.
    - Converted to domain jobs in `server/recipe-history/pkg/history/transformer.go`.
  - `PendingActivityInfo` — Pending activity details.
    - Mapped in `.../transformer.go` to activity execution model.

- `go.temporal.io/api/history/v1`
  - `History` — Container for ordered `HistoryEvent`s.
  - `HistoryEvent` — Event with `EventId`, `EventType`, `EventTime`, and typed attributes.
    - Attribute getters used in `.../transformer.go`:
      - `GetActivityTaskScheduledEventAttributes()`
      - `GetActivityTaskStartedEventAttributes()`
      - `GetActivityTaskCompletedEventAttributes()`
      - `GetActivityTaskFailedEventAttributes()`
      - `GetActivityTaskTimedOutEventAttributes()`
    - Purpose: reconstruct activity execution timeline/status.

- `go.temporal.io/api/enums/v1`
  - Workflow status enums: `WORKFLOW_EXECUTION_STATUS_{RUNNING,COMPLETED,FAILED,CANCELED,TERMINATED,CONTINUED_AS_NEW,TIMED_OUT}`
    - Mapped to internal job statuses in `.../transformer.go`.
  - Pending activity state enums: `PENDING_ACTIVITY_STATE_{SCHEDULED,STARTED,CANCEL_REQUESTED}`
    - Mapped to friendly strings in `.../transformer.go`.
  - History event type enums: `EVENT_TYPE_ACTIVITY_TASK_{SCHEDULED,STARTED,COMPLETED,FAILED,TIMED_OUT}`
    - Used in switch over events in `.../transformer.go`.

- `go.temporal.io/api/common/v1`
  - `Payload` — Serialized data blob (used here for `SearchAttributes` parsing and activity results).
    - Decoded in `server/recipe-history/pkg/history/transformer.go` (`parsePayload`).

**Where Used (Non‑Test Files)**

- server/recipe-history
  - `pkg/history/history.go` — Uses `sdk/client` for `DescribeWorkflowExecution`, `GetWorkflowHistory`, and `WorkflowService().ListWorkflowExecutions`.
  - `pkg/history/transformer.go` — Uses `api/{workflow,history,enums,common,workflowservice}` to transform Temporal structures to domain types.

- server/recipe-worker
  - `pkg/worker/worker_manager.go` — Uses `sdk/worker` and `sdk/workflow` to create workers, register workflows, and control lifecycle.
  - `pkg/worker/worker.go` — Holds `sdk/client.Client` for wiring into manager/registry.
  - `pkg/ops/activity_registry.go` — Uses `sdk/activity` to register activities with options (names) into a worker/test env.
  - `pkg/compiler/compiler.go` — Uses `sdk/workflow` for activity execution, logging, cancellation, timers; and `sdk/temporal` for retry policies and error typing.
  - `pkg/compiler/statemachine_compiler.go` — Uses `sdk/workflow` for logging within workflow execution.
  - `pkg/compiler/retry.go` — Converts between internal retry struct and `temporal.RetryPolicy`.
  - `pkg/executor/standalone.go` — Uses `sdk/testsuite` + `sdk/workflow` to run workflows locally without a Temporal server.

**Notes**

- All references above exclude `*_test.go` files per request.
- The `testsuite` package is used in non‑test code (`StandaloneExecutor`) to enable an embedded execution mode.

