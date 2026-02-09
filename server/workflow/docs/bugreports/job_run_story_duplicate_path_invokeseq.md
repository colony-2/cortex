# Bug: `JobRunStory` contains duplicate nodes for the same `(path, invoke_seq)`

## Summary
The `JobRunStory` API response contains multiple nodes with the **same** `path` and `invoke_seq` but different `task_ordinal` (and sometimes different status). Consumers expect `(path, invoke_seq)` to be **unique** across a workflow execution.

This appears related to how `invoke_seq` is derived/applied from the template/task execution context (likely in `server/recipe-template` / `server/recipe-core` context propagation).

## Expected
- `(path, invoke_seq)` is a stable, unique identifier for a logical node in a story.
- If a node is revisited due to a loop/new invocation, `invoke_seq` should change accordingly (or some other stable disambiguator should be included).
- If a node is retried, duplication should be represented via `attempt`/`prior_attempts`, not as a second node with the same `(path, invoke_seq)`.

## Observed
In `src/web/runexample.json`, multiple nodes share the same `invoke_seq=0` and identical `path`, yet represent different ordinals/iterations.

### Concrete duplicates (from `src/web/runexample.json`)
All examples below have `invoke_seq=0` and identical `path`, but appear more than once in the story:

- `invoke_seq=0` + `root/stateMachine:new-ticket/state:requirements_planning/op:recipe.run_and_get_result/step:start`
  - `opStep` `"step start"`, `task_ordinal=3` (attempt 1)
  - `opStep` `"step start"`, `task_ordinal=8` (attempt 1)

- `invoke_seq=0` + `root/stateMachine:new-ticket/state:requirements_planning/op:recipe.run_and_get_result/step:finish`
  - `opStep` `"step finish"`, `task_ordinal=4` (attempt 1)
  - `opStep` `"step finish"`, `task_ordinal=9` (attempt 1)

- `invoke_seq=0` + `root/stateMachine:new-ticket/state:requirements_planning/transitionEval`
  - `transitionEval` `"evaluate transitions"` duplicated twice (both `attempt 1`, `task_ordinal=null`)

- `invoke_seq=0` + `root/stateMachine:new-ticket/state:requirements_review/op:input/step:generate_form`
  - `opStep` `"step generate_form"`, `task_ordinal=5` (attempt 1)
  - `opStep` `"step generate_form"`, `task_ordinal=10` (attempt 1)

- `invoke_seq=0` + `root/stateMachine:new-ticket/state:requirements_review/op:input/step:collect_user_input`
  - `opStep` `"step collect_user_input"`, `task_ordinal=6` (attempt 1)
  - `opStep` `"step collect_user_input"`, `task_ordinal=11` (attempt 1)

### Additional context observed in the same file
- Some of the “second occurrences” show odd timestamps (e.g. `started_at = 0001-01-01T00:00:00Z`) and a `running` status, suggesting these might be runtime task runs appended by SWF for active jobs, or a second loop that did not increment `invoke_seq`.

## Impact
- UI/consumers that key nodes by `(path, invoke_seq)` will merge distinct logical nodes or overwrite earlier nodes.
- Ordering/selection logic becomes ambiguous (two nodes claim the same identity but different ordinals and statuses).
- Makes it difficult to correctly represent loops vs retries vs runtime pending tasks.

## Reproduction
1. Open `src/web/runexample.json`.
2. Traverse story nodes and group by `(path, invoke_seq)`.
3. Observe multiple groups with >1 node (examples listed above).

## Suspected root cause
`invoke_seq` appears not to change across what looks like a new logical invocation/loop of the same recipe node paths.

Likely causes include:
- Template/task execution context not incrementing `Invocation.InvokeSeq` across loops/reschedules, or
- Story builder deriving `invoke_seq` from the wrong source (e.g., defaulting to 0 when it cannot decode input), or
- Mixing “runtime” task runs (which may not carry proper invocation context) into the same namespace without disambiguation.

This likely lives in context propagation in `server/recipe-template` and/or `server/recipe-core` (the task execution context used to populate `gitstate.GlobalGitTaskContext.InvokeSeq`).

