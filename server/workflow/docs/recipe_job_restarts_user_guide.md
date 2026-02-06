# Recipe Job Restarts (with Context Patching): User Guide

This guide explains what "restart" means for recipe jobs, where restarts are allowed, how to
perform them via API, and how context patching behaves. It is intended for a frontend team
building restart UX.

## What a restart is

A restart creates a **new job** by cloning the prior job's recorded story up to a chosen SWF chapter
ordinal (the "step offset"), then resuming execution from that point.

Optionally, the restart can inject one (or more over time) **context patch** chapters that mutate
template-visible context right before execution continues.

## Where restarts can be done

Restarts are anchored to **SWF chapter ordinals** (a.k.a. task ordinals). In the UI, you should
offer restarts at nodes that correspond to actual task execution:

- `kind=opStep` nodes (and flattened `kind=op` nodes)

The Job Run Story API exposes two fields to support this:

- `task_ordinal`: the ordinal for the node's latest attempt (if any).
- `restart_from_ordinal`: the ordinal for attempt 1 of the logical node (the safe restart cursor).

Important constraints:

- **Do not restart "inside" a retry chain.** SWF restart rejects offsets that cut into retries.
  Use `restart_from_ordinal` (not `task_ordinal`) when `attempt > 1`.
- **Only restart from ordinals that exist in the source job's story.** The backend validates that
  the next chapter exists.

Recommended UI rule:

- If a node has `restart_from_ordinal != null`, show a "Restart from here" action that uses that
  value.

## How restarts are done (API)

Endpoint:

- `POST /api/projects/{projectId}/jobs/{jobId}/restart`

Request body:

```json
{
  "step_offset": 12,
  "context_patch": {
    "job": { "git": { "author": "james@example.com" } },
    "scopes": [
      { "container": "sequence", "id": "build", "outputs": { "image_tag": "v2" } }
    ]
  }
}
```

Fields:

- `step_offset` (required): SWF chapter ordinal (0-based) to resume from.
- `context_patch` (optional): patch to apply at the restart point. If omitted/null, the job is
  restarted without any context mutation.

Response:

```json
{ "job_id": "new_job_ksuid" }
```

Notes for UX:

- Restart returns a **new job id**. The UI should navigate to the new job, or offer a link.
- A restarted job may include `kind=contextPatch` nodes in its story (see below).

## How context patching works

Context patching is applied by injecting a synthetic "context patch" chapter into the restarted
job's story, immediately before execution continues.

This means:

- The patch is **visible in Job Run Story** as `kind=contextPatch`.
- After the patch chapter, subsequent step inputs/outputs are computed using the **updated**
  context.

### Patch object shape

The patch supports two independent patch targets:

1) `job` (global patch)
2) `scopes[]` (scoped patch; local visibility)

#### 1) Job patch (global)

`context_patch.job` is a JSON merge-patch-like object applied to the job context that recipes see
under `context`.

Semantics:

- Objects are merged recursively.
- Setting a key to `null` deletes it.
- The patch is applied globally (current scope + all ancestors), so outer scopes and later steps
  observe the change.

Example: change git author for all future resolution

```json
{ "job": { "git": { "author": "james@example.com" } } }
```

#### 2) Scoped patches (local containers)

`context_patch.scopes[]` mutates the locally-visible scope containers at the point the patch is
applied.

Each item:

- `container`: `"sequence"` or `"states"`
- `id`: the step/state id within that container (usually the recipe node `id`)
- `outputs`: a JSON merge-patch-like object applied to that step's `outputs`

Visibility rules (high level):

- Scoped patches affect only the container maps visible in the current resolution context.
- They are visible to subsequent steps that share that container (e.g. later ops in the same
  sequence), but should not leak to unrelated sibling/outer containers.

Example: override a prior step output in the current sequence container

```json
{
  "scopes": [
    {
      "container": "sequence",
      "id": "build",
      "outputs": { "image_tag": "v2" }
    }
  ]
}
```

### How patches show up in Job Run Story

The story may include:

- `kind=contextPatch`

For these nodes:

- `output` contains the patch object that was applied.
- `task_ordinal` is the injected chapter ordinal.

Frontend recommendation:

- Render these as timeline annotations.
- If showing restart actions, do not attach "restart from here" to `contextPatch` nodes; attach it
  to the next task-backed node using `restart_from_ordinal`.

