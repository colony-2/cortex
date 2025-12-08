# Activity Registry Multi-Task Chains Spec

## Goals and Constraints
- Allow a single registerable op to expand into N sequential tasks (one task type name, called repeatedly).
- Preserve today’s recipe authoring surface (`op:` remains the same) and resolution context semantics (inputs resolved once per node; no new scopes).
- Minimize compiler churn: only touch the op execution path; no compiler-wide context/model rewrites.
- Keep backward compatibility: ops that return a single task spec behave exactly as today.

## Non-Goals
- No child workflows / nested Temporal workers.
- No new recipe DSL constructs or template scopes.
- No parallel fan-out; sequencing only.

## Proposed Types (ops / registry)
- Keep existing `NewActivity*` single-step helpers intact for compatibility; introduce a new builder interface for multi-step (reflection-enforced):
  ```go
  // Step is an internal interface; authors obtain it via the generic constructor.
  type Step interface{ isStep() InType() reflect.Type OutType() reflect.Type Handler() interface{} }

  func NewStep[In, Out any](fn func(ctx context.Context, in In) (Out, error)) Step

  type OpBuilder interface {
      // Op metadata setters (expose OpMetadata fields individually).
      WithType(t string) OpBuilder
      WithDescription(desc string) OpBuilder
      WithVersion(ver string) OpBuilder
      WithDefaultTimeout(d time.Duration) OpBuilder

      AddStep(name string, step Step) OpBuilder
      WithManagementService(svc ManagementService) OpBuilder
      Build() (RegisterableOp, error)
  }

  func NewOp() OpBuilder
  ```
  - `AddStep` accepts any handler; Build performs reflection to ensure:
    - Handler kind: `func(ctx context.Context, in TIn) (TOut, error)` (wrapped internally to inject deps if needed).
    - Chaining: `TOut(i) == TIn(i+1)` for all adjacent steps.
    - At least one step is present.
  - Step names are provided explicitly (`name string`) for docs/metrics and capability discovery; dispatch uses the step name on the wire (not just index).
  - Single-step chain: `NewOp().WithType(...).AddStep(...).Build()` behaves like today; legacy `NewActivity*` helpers delegate to the builder to avoid drift.
- `RegisterableOp` shifts to a chain-first interface:
  - Remove stepless methods (`GetInputType`, `GetOutputType`, `GetInputStruct`, single `ExecuteV2`).
  - New requirements (illustrative):
    ```go
    type RegisterableOp interface {
        GetMetadata() OpMetadata // assembled from builder setters
        GetName() string
        TaskChain() []TaskStep // TaskStep: {Name, InputType, OutputType, Handler, DisallowAsTask}
        GetManagementService() ManagementService // optional, unchanged
        isOpSpec()
    }
    ```
- `TaskStep` remains internal; op authors only interact with the builder/handlers.
- Task type per step is explicit: `opType:stepName` (not just the op type).
- One retry/timeout policy: derived from recipe node metadata (no per-step overrides).

## Registry Behavior
- `Register` reads `TaskChain()`; if length == 0 it errors, if length == 1 it behaves like today.
- Schema generation: produce input/output schemas for every step (used for docs/validation), but still register exactly one `TaskWorker` name per step: `task type = opType:stepName`.
  - Invocation envelope stays simple: `ActivityInvocationRequest` carries resolved `Input` and `GitTaskContext`. Dispatch uses the SWF task type (`opType:stepName`) so no step selector is needed in the payload.
  - Task worker dispatch: the wrapper is registered per step (`opType:stepName`), so payload-driven routing is unnecessary.
- Output->Input threading: after executing a step handler, the wrapper marshals the output map and includes it in the response; no custom `ResultMerge` needed because the compiler will treat the step output as the next step input.
- Retry/timeouts: unchanged; registry does not apply per-step options.
- Step-level `DisallowAsTask`: respected when building the TaskWorker list; steps flagged as disallowed are not exposed as tasks (while still usable within recipes).

## Compiler Execution Flow (minimal changes)
1) Resolve node inputs once with the existing `ResolutionContext` (unchanged).
2) Fetch `TaskChain` (default single-step) from the registry.
3) Initialize `stepInput` = resolved node inputs.
4) Loop using next-step hints from the step response:
   - Invoke `ctx.DoTask(taskType=opType:stepName, ActivityInvocationRequest{Input: stepInput, GitTaskContext: ...})` with the usual retry/timeout from node metadata.
   - Unmarshal `ActivityInvocationOutput`; update git state as today.
   - Set `stepInput = OpOutput`. If `NextTaskType` is present, set the next task name to that and continue; otherwise exit.
5) After the last step, call `AddExecution` with the final `stepInput` (which is the last step’s output) so outer scopes observe the same shape as a single-step op.

## Input Shaping (without new resolution scopes)
- Template resolution happens once on the node’s `inputs`. Step 0 receives that object as-is.
- Each subsequent step receives the previous step’s output as its input. No additional selectors or merges are needed.

## Naming and Compatibility
- Task type per step = `opType:stepName` (explicit in SWF); routing is implicit via task name. Steps carry `NextStepName`/`NextTaskType` so the compiler can walk the chain without registry lookups during execution.
- Recipes remain unchanged; single-task ops are unaffected.
- Schema/docs generation can expose per-step schemas while keeping the public op name the same.

## Rollout Plan
1) Add `TaskChain()` to `RegisterableOp` and implement internal `TaskStep` + `NewOp`/builder helpers in `recipe-core/pkg/ops` (runtime validation of output->next input, step-level `DisallowAsTask`).
2) Extend `ActivityRegistry` to dispatch by `StepName`/`StepIndex` while keeping one registered task name and generating per-step schemas.
3) Update compiler `executeOp` to loop over the chain, feeding outputs to inputs, preserving current resolution context and git updates (pass step name in invocation).
4) Tests:
   - Registry: rejects mismatched chains; dispatches correct handler by step index.
   - Compiler: multi-step op runs both steps in order, uses one retry policy, and exposes only the final output to the parent scope.
5) Document the pattern for op authors (how to define chained handlers).
