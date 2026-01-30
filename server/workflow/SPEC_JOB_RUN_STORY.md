# Spec 3: JobRunStory Replay API

## Goal
Expose a new `JobRunStory` API that tells the narrative of a workflow run (recipe attempts, state machines, sequences, ops, transitions) by replaying recorded outcomes from `swf.GetJobRun` instead of reading raw Strata chapters.

## Scope & Non-Goals
- In scope: build story data in-memory from `GetJobRunResponse`, add a story-building executor/context, and surface a workflow service API + Go models for callers. Replace Strata lookups for run reconstruction and artifact metadata in this package.
- Out of scope: UI rendering, persistence of the story, or changing engine-side recording. Artifact bytes delivery stays as-is (still fetched via engine `GetArtifact`).

## Data Model (service side)
- `JobRunStory` (new in `internal/model`):
  - `WorkflowID`, `RunID`, `RecipeName`, `Status`, `StartedAt`, `CompletedAt?`, `AttemptCount`.
  - `RecipeAttempts[]`: ordered by job attempt; holds start/end, outcome, worker id, and `Events[]` for that attempt.
  - `Events[]` (chronological within attempt): unified timeline items with `Type` + payload. Event types include `SequenceEntered/Exited`, `StateMachineStarted/Finished`, `StateEntered/Exited`, `StateTransition`, `OpAttemptStarted/Finished`, `OpOutput`, `OpError`, `RuntimeWait/Lease`, `JobAttemptResult`.
  - `Ops[]`: denormalized per-op summary (name, attempts, status, inputs/outputs/artifacts/errors) for quick UI tables.
- `GetJobRunStoryRequest`: `{ProjectID, WorkflowID, IncludeArtifacts bool, IncludeOpIO bool}` (defaults true for artifacts, false for full IO if size is a concern).

## Components
- **StoryBuildingContext**: custom `swf.JobContext` implementation that replays rather than executes.
  - Holds the parsed `GetJobRunResponse` grouped by task type and attempt ordinal.
  - `DoTask` pops the next recorded `TaskAttempt` for the requested `taskType`; returns its output (or error) without side effects; asserts input hash/task type alignment; records event hooks for start/end/runtime info.
  - `AwaitDuration/AwaitJobs/SpawnAsync` become no-ops that record timeline markers using runtime data from `TaskAttempt.Runtime` or `JobAttempt` when present.
- **StoryBuildingExecutor**: lightweight `recipeExecutor` implementation that walks the recipe structure and emits semantic events.
  - Wraps/extends the existing executor shape (from Spec 1) but uses `StoryBuildingContext` so ops do not re-run.
  - Emits events on: entering/exiting sequences, state machines, states, and on transitions taken (including `when` matches); associates them with attempt ordinals and timestamps from the replayed attempts.
  - Provides `ExportStory()` to produce the final `JobRunStory` assembled from recorded events plus aggregated op summaries.
- **StoryBuilder orchestration**: service-layer helper that
  1) calls `engine.GetJobRun` with `IncludeInputs=true`, `IncludeOutputs=true`, `IncludeArtifacts=true`, `IncludeAttemptInputs=true` to obtain the canonical run timeline;
  2) loads the recipe via existing `RecipeProvider` to know the logical graph;
  3) instantiates `StoryBuildingContext` with the fetched run data;
  4) runs `StoryBuildingExecutor.ExecuteRecipe` to generate the story;
  5) maps engine statuses (`swf.JobStatus`) to `model.WorkflowStatus` for story status.

## API Surface Changes
- Add `GetJobRunStory` to the workflow service interface (`pkg/workflow.Service`) and implementation.
- Add HTTP/transport handler inside this project (parallel to existing workflow endpoints) that returns `JobRunStory` JSON.
- Update configs to stop requiring a Strata client; use only `swf.SWFEngine` for run reconstruction and artifact metadata. (Artifact bytes retrieval switches to `engine.GetArtifact` using IDs from `TaskIO.Artifacts`.)

## Story Construction Rules
- Job attempts: iterate `JobRunResponse.JobAttempts` in order; each becomes a `RecipeAttempt` with `Result` based on its `Outcome`. Use `Result` when present for final status.
- Ops: group `TaskRun` entries by `TaskType` that correspond to recipe ops; map each attempt’s ordinal/attempt number to timestamps. Populate inputs from `TaskAttempt.Input` when available or resolve via `InputRef` in the replay context.
- Sequences/state machines: derive structural context from the recipe; use executor entry/exit callbacks to emit events with timing derived from the first/last op in that scope (fallback to attempt timestamps when no ops ran).
- State transitions: whenever the executor advances a state, emit `StateTransition` with `from`, `to`, `condition`, `matched` as determined by recipe evaluation results recorded in the context (StoryBuildingContext carries the previously observed outcomes from the run to drive determinate branch selection).
- Runtime events: if a `TaskAttempt.Runtime` is present with lease/wait info, emit `RuntimeWait/Lease` events before the corresponding op attempt end.
- Validation: if the replayed attempts do not align with the recipe traversal (missing task type, mismatched attempt counts), return a typed error (`ErrJobRunReplayMismatch`) so the API can respond with 409/422 semantics.

## Migration Away From Strata
- Remove `Strata *client.Client` from the service config and implementation paths used for read APIs (summaries, details, artifacts, story) and replace with `engine.GetJobRun` + `engine.GetArtifact`.
- Rebuild existing `WorkflowDetail` chapter lists from `TaskRun` data to avoid direct chapter reads; preserve the current JSON shape for backward compatibility.
- Keep logging/telemetry parity (ordinal, attempt, worker id) using values already present in `GetJobRunResponse`.

## Testing
- Unit: StoryBuildingContext `DoTask` returns recorded outputs/errors and enforces ordering; executor emits expected event sequence for a fixture recipe/run (including retries and nested sequences).
- Unit: conversion from `GetJobRunResponse` to `JobRunStory` and legacy `ChapterDetail` matches golden fixtures (no Strata dependency).
- Integration: run a toy recipe via `swf.ToyEngine`, fetch `GetJobRun`, call `GetJobRunStory`, and assert ops/sequences/transition stories match the executed run.
- Error paths: missing task attempt, mismatched hash/task type, and runtime wait/lease fields correctly surface in the story and errors.

## Feasibility Note
All required data (tasks, attempts, inputs/outputs, artifacts) are provided by `swf.GetJobRun` and `engine.GetArtifact`, which are available via the existing `swf-go` dependency. No external repositories are needed beyond `server/workflow`; HTTP surface here already wraps the service, so the work stays within this project.
