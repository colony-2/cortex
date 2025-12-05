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
- Public surface stays minimal and mirrors today’s constructors:
  - **New**: `NewActivityMappedOpV2(metadata OpMetadata, handlers ...ActivityHandlerV2[any, any]) RegisterableOp`
    - Accepts 1+ handlers; single handler preserves current behavior.
    - Constructor inspects concrete generic types, ensuring `Out(i) == In(i+1)`; fails fast otherwise.
    - No exposed `TaskChainSpec`/`TaskStepSpec` to op authors.
- `RegisterableOp` shifts to a chain-first interface:
  - Remove stepless methods (`GetInputType`, `GetOutputType`, `GetInputStruct`, single `ExecuteV2`).
  - New requirements (illustrative):
    ```go
    type RegisterableOp interface {
        GetMetadata() OpMetadata
        GetName() string
        TaskChain() []TaskStep // TaskStep: {InputType, OutputType, Handler}
        GetManagementService() ManagementService // optional, unchanged
        isOpSpec()
    }
    ```
  - Removed: `ExecuteV2`, `GetInputType`, `GetOutputType`, `GetInputStruct` on the interface. Execution happens per step via the stored handlers.
  - `TaskStep` remains internal; op authors only supply handlers via `NewActivityMappedOpV2`.
- Single task name: `metadata.Type` is used for every step invocation; `StepIndex` in the invocation payload drives dispatch.
- One retry/timeout policy: derived from recipe node metadata (no per-step overrides).

## Registry Behavior
- `Register` reads `TaskChain()`; if length == 0 it errors, if length == 1 it behaves like today.
- Schema generation: produce input/output schemas for every step (used for docs/validation), but still register exactly one `TaskWorker` name = `metadata.Type`.
- Invocation envelope: `ActivityInvocationRequest` gains `StepIndex` (int) alongside `Input` and `GitTaskContext`; dispatch uses it to pick the handler.
- Task worker dispatch: the wrapper receives `StepIndex` in `ActivityInvocationRequest` and routes to `Steps[StepIndex]`.
- Output->Input threading: after executing a step handler, the wrapper marshals the output map and includes it in the response; no custom `ResultMerge` needed because the compiler will treat the step output as the next step input.
- Retry/timeouts: unchanged; registry does not apply per-step options.

## Compiler Execution Flow (minimal changes)
1) Resolve node inputs once with the existing `ResolutionContext` (unchanged).
2) Fetch `TaskChain` (default single-step) from the registry.
3) Initialize `stepInput` = resolved node inputs.
4) For step index i over the chain:
   - Invoke `ctx.DoTask(metadata.Type, ActivityInvocationRequest{StepIndex: i, Input: stepInput, GitTaskContext: ...})` with the usual retry/timeout from node metadata.
   - Unmarshal `ActivityInvocationOutput`; update git state as today.
   - Set `stepInput = OpOutput` for the next iteration (output type is guaranteed to match the next input type by construction).
5) After the last step, call `AddExecution` with the final `stepInput` (which is the last step’s output) so outer scopes observe the same shape as a single-step op.

## Input Shaping (without new resolution scopes)
- Template resolution happens once on the node’s `inputs`. Step 0 receives that object as-is.
- Each subsequent step receives the previous step’s output as its input. No additional selectors or merges are needed.

## Naming and Compatibility
- Single task name = `opType`; called repeatedly with `StepIndex` to select the handler.
- Recipes remain unchanged; single-task ops are unaffected.
- Schema/docs generation can expose per-step schemas while keeping the public op name the same.

## Rollout Plan
1) Add `TaskChain()` to `RegisterableOp` and implement internal `TaskStep` + `NewChainedOp` helpers in `recipe-core/pkg/ops` (runtime validation of output->next input).
2) Extend `ActivityRegistry` to dispatch by `StepIndex` while keeping one registered task name and generating per-step schemas.
3) Update compiler `executeOp` to loop over the chain, feeding outputs to inputs, preserving current resolution context and git updates.
4) Tests:
   - Registry: rejects mismatched chains; dispatches correct handler by step index.
   - Compiler: multi-step op runs both steps in order, uses one retry policy, and exposes only the final output to the parent scope.
5) Document the pattern for op authors (how to define chained handlers).
