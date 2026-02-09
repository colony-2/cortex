# Job Run Story API: Consumer Guide

This API returns a recipe-centric, hierarchical "story" of a job execution. It is designed for REST consumers who want to render **what happened**, in the same nested structure recipe authors think in:

- recipe
  - sequences
    - ops (with retries)
      - op steps (only when an op has multiple steps)
  - state machines
    - states
      - transition evaluation decisions

The server builds the story by replaying the recorded run, but the response does **not** require you to understand the underlying engine/task system.

## Endpoint

`GET /api/projects/{projectId}/jobs/{jobId}/story`

## Top-level fields

- `job_id`: the job you requested.
- `invocation_sequence`: the root recipe invocation sequence number (useful for correlation).
- `recipe`: metadata about the recipe that was executed, as loaded from the job-start artifact.
- `status`: normalized workflow status (`running|completed|failed|canceled|terminated|timed_out|unknown`).
- `started_at` / `finished_at`: job-level timestamps (finished may be null while running).
- `root`: the root story node (always `kind=recipe` when present).

## The story is a tree

The key idea: **`root` contains `children`, and each child contains more children**, forming a chronological tree. You can render it as an expandable outline, breadcrumbs, or a nested timeline.

Every node has:

- `kind`: what type of thing it is (`recipe|sequence|op|opStep|contextPatch|stateMachine|state|transitionEval`).
- `title`: display label (human-friendly).
- `status`: current state of that node (`pending|running|succeeded|failed|canceled|skipped|unknown`).
- `started_at` / `finished_at`: best-effort timestamps for that node (may be null).
- `path`: an array of strings representing the canonical recipe invocation path segments (derived from the execution context’s `invocation.path`, split on `/`). Some non-invocation nodes append a final segment (e.g. `step:<id>`, `transitionEval`, `contextPatch:<ordinal>`) for stable disambiguation.
- `invoke_seq`: the recipe invocation sequence for that node (stable for correlation within a run).
- `input` / `output`: the unmodified input/output payloads for the node (may be null if not available or not applicable).
- `artifact_keys`: list of artifact keys produced/attached at that node (may be empty).
- `task_ordinal`: SWF chapter ordinal for the node's latest attempt (only present for task-backed nodes, e.g. `opStep`).
- `restart_from_ordinal`: SWF chapter ordinal for the first attempt of the logical node (safe restart cursor; only present for task-backed nodes).
- `children`: nested story nodes in chronological order (may be empty).

### How to traverse it

- Treat `children` as "what happened inside this scope".
- Render nodes in the order they appear; that is the story order.
- `path` is ideal for breadcrumbs and for stable-ish UI keys when you need to associate client-side state with nodes.

## Retries: `attempt` + `prior_attempts`

Many things can be retried. The story represents this in a "latest-first" shape:

- The node object you see inline is the **latest attempt**.
- `attempt` is the attempt number for that latest attempt (usually `1` when no retry happened).
- `prior_attempts` is an array of previous attempts of the same logical node, using the exact same node shape.

Consumer recommendation:
- Render the latest attempt inline.
- Provide a disclosure to show `prior_attempts` when present.

## Context patches (restart injection)

When a job is restarted with a context patch, the story may include one or more nodes of:

- `kind=contextPatch`

These are synthetic chapters injected at restart time. The node `output` contains the patch object
that was applied, and subsequent nodes reflect the updated template context.

Consumer recommendation:
- Render these as timeline annotations ("Context patched before resuming").
- If you support "restart from here" UI, do not offer it on `contextPatch` nodes; use the next
  `opStep` node's `restart_from_ordinal` instead.

## Ops vs op steps (multi-step ops)

Most operations are single-step. Some operations involve multiple internal steps (for example: start a child recipe, then wait for its result).

The story models this as:

- `kind=op` is the logical operation.
- If the op is multi-step, the op will have `children` that are `kind=opStep`.
- If the op is single-step, the op will have no `opStep` children (the step is "flattened" into the op itself).

Consumer recommendation:
- If an `op` has `opStep` children, render them nested under the op.
- If it does not, render only the op.

## State machines, states, and transition decisions

When the recipe enters a state machine:

- `kind=stateMachine` is added.
- It contains `kind=state` nodes in the order they were entered.

Within each `state`, when transitions are evaluated, there is a single:

- `kind=transitionEval` node that includes:
  - `evaluations`: ordered list of `{expression, result, to_state_id}`
  - `decision`: either:
    - `{kind:"state", to_state_id:"..."}` when a transition was taken
    - `{kind:"fallthrough"}` when none matched

Consumer recommendation:
- Render the `transitionEval` node under the state where it happened.
- Show the list of evaluated conditions (false first, then the true one if any), and highlight the final `decision`.

## Artifacts: keys only (bodies fetched separately)

Nodes expose artifacts as `artifact_keys[]`, using this shape:

```json
{
  "jobId": "j_456",
  "taskOrdinal": 12,
  "name": "stdout.log",
  "sizeBytes": 12345
}
```

To fetch artifact bytes, use the existing artifact download API:

`GET /api/projects/{projectId}/jobs/{jobId}/tasks/{taskOrdinal}/artifacts/{artifactName}`

where:
- `jobId` comes from the story top-level `job_id` (and should match the key’s `jobId`)
- `taskOrdinal` comes from the artifact key
- `artifactName` comes from the artifact key’s `name`

Consumer recommendation:
- Do not inline artifact bodies in your UI by default.
- Display artifact name + size, and offer a download/view action that calls the artifact endpoint.

## Running jobs (partial stories)

For a job that is still running:

- `status` will be `running`.
- Some nodes near the "end" of the tree may have `status=running` and `finished_at=null`.
- Inputs/outputs may be null/partial for nodes whose underlying work has not completed.

Consumer recommendation:
- Render what is available; treat the story as append-only as the run progresses.
- Poll the endpoint (or later adopt server-sent events) to refresh.
