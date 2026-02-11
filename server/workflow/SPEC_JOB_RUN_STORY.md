# Spec: JobRunStory Replay API (Tree)

## Goal
Expose a new `JobRunStory` API that returns a **recipe-centric, hierarchical execution story** by replaying recorded outcomes from `swf.GetJobRun` (no Strata reads).

The response is a single consolidated tree (recipe -> sequences -> ops/steps -> state machines -> states -> transition evals), shaped for "what happened" readability.

## Non-Goals
- No UI work.
- No persistence/caching of the story (computed on demand).
- No changes to existing (legacy) workflow APIs; only the new story endpoint is replay-based.

## Inputs
- `swf.SWFEngine.GetJobRun` (IncludeInputs/Outputs/Artifacts/AttemptInputs=true)
- Recipe definition loaded from **chapter 0 job-start artifacts**: `GetJobRunResponse.Start.Input.Artifacts` (fetch bodies via `engine.GetArtifact`).

## Data Model (service side)
- `JobRunStory`:
  - `job_id`, `invocation_sequence`, `recipe {id,name,version,source}`, `status`, `started_at`, `finished_at`, `root`.
- `JobRunStoryNode`:
  - Always present fields: `id`, `kind`, `title`, `status`, `started_at?`, `finished_at?`, `path[]`, `invoke_seq`, `attempt`, `prior_attempts[]`, `input`, `output`, `artifact_keys[]`, `children[]`.
  - `path[]` is derived from the execution context’s `invocation.path` (split on `/`); some non-invocation nodes append a final segment (e.g. `step:<id>`, `transitionEval`, `contextPatch:<ordinal>`).
  - Optional task cursor fields: `task_ordinal?`, `restart_from_ordinal?` (present for task-backed nodes like op steps).
  - `prior_attempts` holds prior attempts of the same logical node (latest attempt is inline).
- `artifact_keys` uses the existing `swf.ArtifactKey` shape (jobId/taskOrdinal/name/sizeBytes). Artifact bodies are fetched separately.

## Components
- **StoryBuildingContext** (`swf.JobContext`):
  - Replays tasks from `GetJobRunResponse.Attempts[i].Tasks`.
  - `DoTask` consumes the next recorded `TaskRun` for the requested `taskType` and returns the recorded output (or an error). If the final attempt is non-terminal, returns `ErrReplayInProgress`.
- **StoryBuildingExecutor**:
  - Lightweight executor that walks the recipe structure and builds a story tree while calling `DoTask` to advance op execution.
  - Creates an `op` node per recipe op.
  - For multi-step ops, nests `opStep` nodes under the `op`.
  - For single-step ops, flattens the one `opStep` into the `op` node.
- **Multi-step detection** (by task type naming):
  - The recipe executor emits task types as `<opId>:<stepId>`.
  - Common single-step ops use `<opId>:<opId>`.
  - Multi-step ops have multiple distinct `stepId` values for the same `opId` and are represented as nested `opStep` nodes.
- **Transition evaluation**:
  - A single `transitionEval` node per decision point is nested under the `state`.
  - It contains ordered `evaluations[]` plus a `decision` (`state` or `fallthrough`).

## Error semantics
- `ErrReplayMismatch`: recorded run cannot be deterministically matched to the recipe traversal (e.g., missing task types/runs).
- `ErrReplayInProgress`: run is still active; the story is partial and ends at the current running node(s).

## Restart context patches

When a job has been restarted with context patching, the job story may contain synthetic chapters injected
by restart. These appear in the tree as:

- `kind=contextPatch`

The node's `output` is the patch object that was applied. Subsequent nodes reflect the updated template-visible
context used for input and output resolution.

## HTTP surface
- Add `GET /api/projects/{projectId}/jobs/{jobId}/story` in `/src/api/openapi/colony2-api.yaml`.
- Regenerate bindings in `/src/server/openapi`.
- Implement handler in `/src/server/api` that calls workflow service `GetJobRunStory`.

## Feasibility note
All required data (attempt-scoped tasks, attempts, IO, job-start recipe artifacts) is available via `swf.GetJobRun` + `engine.GetArtifact`. The replay + story building can be implemented entirely in `server/workflow` (HTTP wiring lives in `server/api` + OpenAPI spec/bindings as normal).
