# Recipe Validation Mode Spec

## Goal

Provide a "validation mode" for executing recipes that resolves all CEL templates and validates that they compile and evaluate correctly, without running any activities or causing side effects.

## Non-Goals

- No Temporal task execution.
- No external effects (filesystem, network, git, containers).
- No dependency on real activity outputs.

## Public API

Add an execution mode and options to the execution entry points, but keep the main
execution path clean by pushing behavior into injected contexts/resolvers.

```go
type ExecutionMode string

const (
	ExecutionModeRun      ExecutionMode = "run"
	ExecutionModeValidate ExecutionMode = "validate"
)

type ValidationMode string

const (
	ValidateAll      ValidationMode = "all"
	ValidatePathOnly ValidationMode = "path_only"
)

type ValidationOptions struct {
	Mode       ValidationMode
	CollectAll bool
}

type ExecutionOptions struct {
	Mode       ExecutionMode
	Validation ValidationOptions
}

func ExecuteRecipe(
	ctx workflow.Context,
	r recipe.Recipe,
	rawRecipeInputs map[string]interface{},
	execCtx contextual.JobContext,
	commitContext contextual.GitCommitContext,
	opts ...ExecutionOptions,
) (map[string]interface{}, error)
```

Also expose the same options on `StandaloneExecutor.Execute(...)`.

## Resolver Options

Introduce resolver-side options so template behavior can change without adding
branches in `ExecuteRecipe` or node executors:

```go
type ResolutionOptions struct {
	Mode                 ExecutionMode
	ValidationMode       ValidationMode
	ClampSliceIndex      bool
	AllowFutureStepRefs  bool
}
```

`NewRecipeResolutionContext(...)` and `NewChildContext(...)` should accept (or
inherit) `ResolutionOptions`, and `EvaluateCEL` / `ResolveMap` should use them.
In validation mode, default to `ClampSliceIndex=true` and
`AllowFutureStepRefs=true` unless explicitly overridden.

## Design Constraints

- Prefer pushing validation behavior into injected context/resolver objects, not
  branches inside `ExecuteRecipe` and node executors.
- Use a validation `workflow.Context` implementation that no-ops tasks and
  returns deterministic zero outputs.
- Pass `ExecutionOptions` (or a derived `ResolutionOptions`) into template
  resolution to handle validation-only behaviors like slice index handling.

## Validation Workflow Context

Implement a validation `swf.JobContext` that plugs into `workflow.Context`:

- `ExecuteRecipe` continues to accept a `workflow.Context` value.
- For validation, provide a `workflow.Context` whose `JobContext` implements
  `DoTask` with deterministic zero outputs.
- `DoTask` behavior:
  - Parse `taskType` as `opType:stepName`.
  - Lookup the op in `ops.Get(opType)` and find the matching `TaskStep` by name.
  - Generate a zero output map for the step `OutputType` (same rules as above).
  - Create a `workerops.ActivityInvocationOutput` with:
    - `OpOutput`: zero output map
    - `GitResult`: unchanged (pass through from the current context)
    - `NextTask`: non-empty for non-final steps, empty for the final step
  - Return `swf.NewTaskData(envelope)`.

This makes `executeOp` walk the full `TaskChain()` without branching on
`ExecutionMode` and without calling real activities.

## Validation Semantics

- Input validation should accept a `nil` input map in validation mode:
  - If inputs are provided, validate via `RecipeMetadata.ValidateInputShapeAndFillDefaults`.
  - If inputs are `nil`, generate a map from `RecipeMetadata.InputSchema`:
    - Use `default_value` when present.
    - Otherwise use type-based zero values: `string` -> `""`, `number` -> `0`,
      `boolean` -> `false`.
    - If `InputSchema.Type` is unsupported, return a validation error.
    - Do not error on missing required fields when inputs are `nil` in
      validation mode; treat them as zero-valued placeholders.
- Templates are always resolved using existing resolution context helpers.
- Any CEL compilation or evaluation error is a validation failure.
- `ExecutionModeValidate` must never call `ctx.DoTask` (enforced by a validation
  `workflow.Context` implementation that returns an activity envelope with
  zero outputs).
- Validation should tolerate slice indexing by resolving any slice index to the
  first element when operating in validation mode (see "Slice Index Handling").

## Zero Output Generation

When in validation mode, each op yields "zero outputs" instead of real execution:

- All ops are schema-backed, so output zeros should be derived from the op schema:
  - Use the last `TaskStep.OutputType` from `TaskChain()` (same source as
    `checkOpInputs` in `recipe/node.go`).
  - Produce a zero value for that type with reflect and map it using JSON tags.
  - Use the same struct-to-map rules as the op runtime (`structs` with
    `TagName = "json"`).

## Execution Rules (Validation)

### Op Nodes (`executeOp`)

- Resolve inputs via `ResolveMap` (resolver uses validation options).
- The injected validation `workflow.Context` returns zero outputs for tasks and
  forces the `executeOp` loop to walk the full `TaskChain()`:
  - For non-final steps, set `NextTask` to any non-empty string.
  - For the final step, set `NextTask` to `""`.
- Call `AddExecution(zeroOutputs)` using the task output envelope.

### Sequences (`executeSequence`)

- Resolve sequence inputs.
- Execute child nodes in order (using validation mode).
- Resolve sequence outputs and add to parent context.

### State Machines

Validation mode supports two behaviors:

1) **ValidateAll** (recommended default)
   - Validate every state in deterministic order (map key order is unstable, so
     use sorted keys for `StateMap.States`).
   - Resolve state inputs.
   - Evaluate all `when` conditions and retry conditions for CEL validity using
     `evaluateTransitionsWithContext` and `ResolutionContext.EvaluateCEL`.
   - Execute contained node(s) in validation mode and `AddExecution`.

2) **ValidatePathOnly**
   - Evaluate conditions with zero outputs and follow the chosen transition.
   - This matches the runtime path, but may skip templates in other branches.

## Error Reporting

Return structured validation errors when possible:

```go
type ValidationError struct {
	Path  string // e.g. "sequence[2].inputs.user_id" or "states.review.transitions[0].when"
	Expr  string // raw CEL expression
	Scope template.ScopeType
	Err   error
}
```

If `CollectAll` is true, collect and return all errors in one response. Otherwise, return the first error.

## Output

On success, return the resolved recipe outputs map, with zero values where outputs originate from ops.

## Why This Works

- Templates are evaluated in the same scoped context as runtime.
- Outputs only exist where they would at runtime, so invalid references are caught.
- `ValidateAll` ensures branches and states that are not executed still get validated.

## Slice Index Handling

Validation must allow expressions that index into run/attempt slices (or any slice)
to resolve even when only one element exists. In validation mode, this behavior
is implemented in the template resolver using options passed from execution.

- Any slice index access should resolve to index 0 if the slice is non-empty.
- If the slice is empty, keep current behavior (validation error).

Concrete implementation:

- Add a CEL type adapter or list wrapper in `template.NewResolutionContext` that
  intercepts list indexing (`x[i]`) when `ResolutionOptions.ClampSliceIndex` is
  enabled.
- Use `cel.CustomTypeAdapter(...)` with a wrapper that:
  - Detects native Go slices/arrays in `NativeToValue`.
  - Wraps them in a `clampedList` that implements `ref.Val` plus
    `traits.Lister`, `traits.Indexer`, and `traits.Sizer`.
  - `Get(i)` and `Index(i)` must always return element 0 when the list is not
    empty, regardless of `i`.
  - When the list is empty, `Index(i)` returns an error (keep current behavior).
- Keep the existing `embeddedStructAdapter` behavior; either compose the new
  adapter to delegate non-slice values or wrap the adapter and forward.

Examples:

- `sequence.api_call.runs[3].outputs.error` resolves against `runs[0]`.
- `states.review.runs[1].outputs.status` resolves against `runs[0]`.

This ensures templates that use "latest" or specific-index semantics remain valid
when validation runs with only a single synthesized entry.

## Future-Populated Data

Validation should tolerate references to steps that could be populated in later
iterations (state loops or retries). The concrete behavior should follow the
actual data structures in `template/template_resolver.go`:

- `templateData.Sequence` and `templateData.States` are `map[string]StepOutput`.
- `StepOutput` contains `Outputs map[string]interface{}` and `Runs []RunOutput`.
- `RunOutput` contains `Outputs map[string]interface{}`.

Concrete rules in validation mode:

1) Pre-seed `TemplateData.Sequence` with placeholders for every node id in the
   current sequence. The key must match `scopeId(...)`:
   - `NodeMetadata.ID` when present.
   - For ops, fallback to the op name (the `executeOp` call passes `op` as the
     fallback).
   - For sequences, fallback to `"sequence"` if no `NodeMetadata.ID` is set.

2) Pre-seed `TemplateData.States` with placeholders for every state name in the
   current `StateMap.States` (the `runState` call passes `stateName` as the
   fallback to `NewChildContext`).

3) Each placeholder is a `StepOutput` with:
   - `Outputs` pre-filled by node type:
     - Op nodes: zero map derived from the op output schema (`TaskStep.OutputType`).
     - Sequence nodes: use `recipe.SequenceData.Outputs` to get keys and seed
       a zero-valued map for those keys. Zeroing rules:
       - `string` (including template strings) -> `""` (do not evaluate)
       - `number`/`int` literal -> `0`
       - `boolean` literal -> `false`
       - `map[string]interface{}` -> recursively apply zeroing
       - `[]interface{}` -> empty slice
     - State machine nodes: use `recipe.StateData.Outputs` to get keys and seed
       a zero-valued map for those keys (same zeroing rules as sequences).
   - `Runs` containing a single `RunOutput` whose `Outputs` match the same
     placeholder shape so that `runs[0]` access is always valid.

4) Unknown root variables (`inputs`, `sequence`, `states`, `scope`, `context`)
   still error; only missing keys inside `sequence`/`states` are tolerated in
   validation mode.

Implementation note: use a `recipe.NodeWalker` to collect node ids for each
sequence scope and state names for each state machine scope, then apply those
placeholders when creating the corresponding `ResolutionContext` in validation
mode. The hooks are:

- `executeSequence` -> right after `NewChildContext(template.ScopeSequence, ...)`
  when `ResolutionOptions.AllowFutureStepRefs` is enabled.
- `executeStateMachine` -> right after `NewChildContext(template.ScopeStateMachine, ...)`
  for the state map itself, and in `runState` for per-state placeholders.

This keeps the main execution path unchanged while making future-populated
references resolvable.
