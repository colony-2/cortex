# Spec 2: Execution Analysis via Executor Decorator

## Goal
Produce a structured `ExecutionAnalysisArtifact` (ops, state machine transitions, when matches, sequences, runtime) without changing core execution signatures. Use a decorator over `recipeExecutor` plus an internal recording job context wrapper.

## Components
- **analysisExecutor**: implements `recipeExecutor`, wraps an inner executor (defaults to `defaultRecipeExecutor`), intercepts execution methods to record semantic events, then delegates to the inner executor.
- **RecordingJobContext**: lightweight wrapper around `swf.JobContext` used only inside analysis executor to capture annotations (transition taken, when matched, op start/end, runtime waits/leases, sequence progress). Default executor continues using plain `swf.JobContext`.
- **ExecutionAnalysisArtifact**: structured in-memory object returned by `analysisExecutor.ExportAnalysis()`; serialization (if needed) is handled at API/service layer, not inside executor.

## Flow
1) Service chooses executor:
   - Default path: `defaultRecipeExecutor` with plain `swf.JobContext` (no recording).
   - Analysis path: `analysisExecutor{inner: defaultRecipeExecutor}` with `RecordingJobContext`.
2) Run `ExecuteRecipe` through the chosen executor.
3) The analysis executor records semantic events while delegating.
4) After completion, call `ExportAnalysis()` to get the artifact; persist or return via `/analysis` API.

## Recording surface (examples)
- `RecordTransition(from, to, on, conditionMatched, at)`
- `RecordWhen(op, expr, matched, reason?, at)`
- `RecordOpStart(op, attemptOrdinal, at)` / `RecordOpEnd(op, attemptOrdinal, status, error?, at)`
- `RecordRuntime(op, state, leaseOwner?, leaseExpiresAt?, waitFor?, availableAt?)`
- `RecordSequenceProgress(seq, index, at)`

## Service/API notes
- Add `GetWorkflowAnalysis` that:
  - fetches `GetJobRun` if replaying,
  - runs through `analysisExecutor`,
  - returns or persists the `ExecutionAnalysisArtifact`.
- Backward compatibility: existing `GetWorkflowRun` unchanged.

## Testing
- Unit: recording happens under analysis executor and is no-op under default executor.
- Mapping: transitions, when matches, sequence progress reconstructed correctly for multi-attempt and retry scenarios.
- Integration stub: fake swf engine + recipe to ensure artifact fields populate as expected.
