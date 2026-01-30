# Spec 1: recipeExecutor Interface

## Goal
Extract the workflow execution logic into a `recipeExecutor` interface so we can wrap/extend execution without altering core behavior or `swf.JobContext`.

## Interface
```go
type recipeExecutor interface {
  ExecuteRecipe(ctx context.Context, jobCtx swf.JobContext, rec *recipe.Recipe, inputs map[string]interface{}) error
  ExecuteStateMachine(ctx context.Context, jobCtx swf.JobContext, sm recipe.StateMachine, inputs map[string]interface{}) error
  ExecuteState(ctx context.Context, jobCtx swf.JobContext, state recipe.State, inputs map[string]interface{}) error
  ExecuteOp(ctx context.Context, jobCtx swf.JobContext, op recipe.Op, inputs map[string]interface{}) error
  ExecuteSequence(ctx context.Context, jobCtx swf.JobContext, seq recipe.Sequence, inputs map[string]interface{}) error
}
```

## Steps
1) Create a `defaultRecipeExecutor` struct that implements the interface by moving current free functions onto methods.
2) Update the local call sites (where `ExecuteRecipe` is invoked) to accept a `recipeExecutor`, defaulting to `defaultRecipeExecutor` so no upstream package changes are required.
3) Keep `swf.JobContext` untouched; no signature changes to ops.
4) Ensure current behavior is preserved: default executor is functionally identical to existing code.

## Considerations
- Keep the interface narrow (only the methods we already expose).
- Avoid public surface changes beyond the new interface type and default implementation.
- Validate that error propagation and retry semantics remain identical after refactor.

## Testing
- Golden-path regression: run existing executor tests against `defaultRecipeExecutor`.
- Interface conformance: compile-time check that default executor implements `recipeExecutor`.
- Smoke test: start a recipe via existing service and ensure no behavior change.
