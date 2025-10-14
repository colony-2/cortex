# Recipe Rewind & Reset Spec

## Overview
Recipe rewinds restart a recipe to just before an op by supplying a **recipe_run_metadata signal** that tells the workflow which execution path to traverse when it restarts and, when applicable, which parent triggered the run. Each op invocation carries a compiler-assigned invocation hash. When the workflow receives the metadata signal, it examines each `recipe`/`recipe_set` op, inspects the payload, and decides whether to continue normally or to reset the child recipe according to the execution path.

## Goals
- Define how recipes (and child recipes) block on a single `recipe_run_metadata` signal before executing.
- Use the signal payload (ordered invocation hashes plus event ids) to pop execution-path segments and deterministically reinvoke child recipes.
- Guarantee that only the intended restart run consumes the payload, even if prior runs emit older signals.

## Execution Path Format
Each op invocation has a deterministic **invocation hash** (for example `ticketRecipe.ExecutePlan.implStep1#abc123`). A rewind payload lists these invocations from the root recipe down to the leaf recipe that contains the point we want to reset to. Every entry carries the invocation hash, the Temporal run_id of the previous recipe execution and the Temporal `event_id` immediately before that op so the workflow can reset right before the op. For `recipe_set` ops the entry also includes `recipe_set_index` identifying which child invocation to reset. Example:

```json
{
  "execution_path": [
    { "invocation_hash": "ticketRecipe.ExecutePlan.implStep1#abc123", "event_id": 150, "run_id": "run-root-uuid" },
    { "invocation_hash": "implRecipe.DependencyFanOut#def456", "event_id": 87, "run_id": "abc"  },
    { "invocation_hash": "ticketRecipe.DependencyTicket#ghi789", "recipe_set_index": 1, "event_id": 203, "run_id": "def"  }
  ],
  "reason": "Need to adjust dependency scope"
}
```

- `execution_path` is ordered from outermost → innermost; each entry is a recipe invocation hash (and optional `recipe_set_index`) paired with the run_id we want to reset and the event id immediately before that op invocation.
- Within the metadata signal the same structure is nested under `resume.execution_path`.

## Story Builder output is used to Build Execution Paths
`server/recipe-history`'s story builder already tracks invocation dependencies. We will build a new component that consumes the story builder output to generate a complete execution path from a specific op invocation hash.

## Key Clarifications
- **Reset event source**: The rewind payload must carry the `WorkflowTaskCompletedEventId` that completed immediately *before* the op began. For activity ops this is the value embedded in `ActivityTaskScheduledEventAttributes.WorkflowTaskCompletedEventId`, the workflow task completion that directly precedes the activity start. For inline ops it is the `WorkflowTaskCompletedEventId` directly before the inline "start" marker recorded by `story.RecordInlineOpStart`. The execution-path builder should treat both as the canonical `event_id` and ignore marker ids or any later workflow tasks.
- **Invocation hash vs. ID**: Story builder currently exposes a `NodeRun.InvocationID`. We assume this matches the compiler hash used in the rewind payload, but the spec should confirm whether any ops override `Invocation.ID` with non-hash values and how to handle legacy runs where the value is blank (story builder falls back to `nodePath#seq`). The rewind implementation will treat this value strictly as `InvocationHash`; story builder should surface a dedicated `InvocationHash` field (with `InvocationID` kept as a compatibility alias).
- **recipe_set indexing**: The rewind payload expects a `recipe_set_index`, yet the run metadata only exposes the index inside marker metadata. The execution-path builder will walk *up* from the leaf run toward the root by following parent run links, so each run must expose its parent workflow id/run id and the index that parent used (`recipe_set_index` when the parent op is a recipeset). Story builder (and underlying Temporal markers) must capture this linkage so the rewind service can deterministically populate the field while reversing the path.
- **Multiple attempts per invocation**: A node can have multiple `NodeRun` entries (retries or partial progress). We will follow the final attempt recorded in the story (e.g., the third successful try), since rewinding to the most recent execution preserves the observable state and keeps the behavior consistent.
- **Async child behavior**: The resume logic references "standard wait logic (wait for sync, don't wait for async)", but the current `recipe_set` implementation always waits on the child future. Rewinds should mirror the original behavior: if the child was launched async the first time, the reset run also launches it async; if the parent waited originally, the reset run waits again.
- **Cross-run discovery**: Building a path that spans multiple child recipes requires finding parent invocations by run id. This can be done by reviewing the metadata provided as part of the metadata signal at recipe start. Story builder then surfaces these parent identifiers, allowing the execution-path builder to walk upward without querying Temporal search attributes.

## Execution Path Construction via Story Builder

The rewind builder will use `storybuilder.Story` instances to derive the ordered `execution_path` payload. Each story represents one Temporal workflow run and one recipe run. 

### Inputs
- Leaf recipe identifier (`recipe_name`) and Temporal workflow ids (workflow id + run id) for the run that emitted the target invocation hash.
- Target invocation descriptor `{invocation_hash}` describing the op we need to rewind to.
- Access to `history.Client.BuildStory` (from `server/recipe-history/pkg/history`).

### Traversal Steps
1. Start from the run that contains the invocation_hash: build its story (`leaf := BuildStory(leafRecipeName, leafWorkflowID)`) and verify the target `InvocationHash` exists in that run (falling back to `InvocationID` only for legacy runs). Capture the run's `ResumeEventID`, `RecipeSetIndex`, and parent linkage metadata.
2. Append the leaf invocation segment to an in-memory stack.
3. While the current run exposes `ParentWorkflowID`/`ParentRunID`, fetch the parent's story (from cache when possible), locate the parent `NodeRun` by matching `ParentInvocationHash`, and push a new segment containing the parent's hash, run ids, resume event id, and optional `RecipeSetIndex`.
4. Repeat step 3 until there is no parent linkage (root reached) or a necessary story is missing. If any lookup fails (missing story, missing parent hash), abort with `ErrInvocationNotFound`.
5. Once the root is reached, reverse the stack to produce the ordered outermost → innermost execution path. Append contextual metadata (reason, timestamp) outside the list.

### Failure Handling
- If `BuildStory` fails (missing history, Temporal outage) bubble the error so the rewind request can be retried later.
- If a child story references a run id but no recipe name, reject the request.
- If multiple runs share the same invocation hash (due to retries), select the run whose temporal execution window contains the supplied child run id or the most recent completed attempt; callers can override this logic by supplying the child run id explicitly.

## Story Builder Metadata Requirements
- Surface a dedicated `InvocationHash` field on `NodeRun` (aliasing the existing `InvocationID`) and guarantee it is populated for inline ops, activities, and child recipes.
- Capture the `WorkflowTaskCompletedEventId` that immediately preceded the op as `NodeRun.ResumeEventID`. For activities this comes from `ActivityTaskScheduledEventAttributes.WorkflowTaskCompletedEventId`; for inline ops this comes from the `MarkerRecordedEventAttributes.WorkflowTaskCompletedEventId` associated with the inline "start" marker; for child recipes we should record the trigger marker's value. This id is what the rewind service will pass to Temporal's reset API.
- Retain the `ScheduledEventId` / `MarkerEventId` that ties the run back to history for debugging and optional validation.
- Promote the `recipe_set` child index into a first-class `NodeRun.RecipeSetIndex` integer, derived from the marker metadata written by `recipe_set`.
- Record parent linkage for child runs (`NodeRun.ParentWorkflowID`, `NodeRun.ParentRunID`, and `NodeRun.ParentInvocationHash`) so the execution-path builder can reverse-walk from a leaf run to the root without additional Temporal queries.
  These values are populated from the `parent` block of the `recipe_run_metadata` signal and stored alongside the run metadata.
- Record whether the parent waited for the child (`WaitForChild bool`) so the rewind service knows if replaying the child requires pausing the parent or can resume asynchronously.
- Provide lightweight lookup helpers (e.g., maps from `InvocationHash → NodeRun` and `RunID → NodeRun`) so the execution-path builder does not need to perform O(n) scans for every lookup.

## recipe_run_metadata Signal Behaviour
1. **Signal schema**
   ```json
   {
     "target_run_id": "run-root-uuid",
     "parent": {
       "workflow_id": "run-parent-workflow",
       "run_id": "run-parent-uuid",
       "invocation_hash": "ticketRecipe.ExecutePlan.implStep1#abc123",
       "recipe_set_index": 1
     },
     "resume": {
       "execution_path": [
         { "invocation_hash": "ticketRecipe.ExecutePlan.implStep1#abc123", "event_id": 150, "run_id": "run-root-uuid" },
         { "invocation_hash": "implRecipe.DependencyFanOut#def456", "event_id": 87, "run_id": "abc" },
         { "invocation_hash": "ticketRecipe.DependencyTicket#ghi789", "recipe_set_index": 1, "event_id": 203, "run_id": "def" }
       ]
     }
   }
   ```
   - `target_run_id` specifies the workflow run that should consume this signal. If the currently running workflow has a different run id, it ignores the payload (preventing older signals from affecting newer runs).
   - `parent` is optional. It is omitted for root recipes, populated for every child invocation (initial runs and resets). It carries the parent's workflow/run ids, the parent invocation hash, and an optional `recipe_set_index` when the parent op is a recipeset.
   - `resume.execution_path` is ordered outermost → innermost; all entries are op invocation hashes (with optional `recipe_set_index` when the parent is a recipeset), each paired with the event id immediately before that invocation. `resume` is omitted or `{ "execution_path": [] }` for first-run executions; populated for resets.
   

2. **Signal acceptance & waiting**
   - All recipe workflows register a single `recipe_run_metadata` signal handler and block on a workflow channel until a valid payload arrives.
   - The handler only accepts payloads where `target_run_id == workflow.GetInfo().WorkflowExecution.RunID`. If the run id differs, the signal was meant for another run and is ignored (but the recipe continues anyway).
   - Valid payloads are sent over the waiting channel. Duplicate signals for the same run simply replace the stored payload before execution begins. The queue/API always send a metadata signal immediately after starting a run: root executions receive `{target_run_id}` only, child first-runs receive a payload with `parent` populated and an empty `resume`, while rewinds provide both `parent` and the desired `resume`. Because the workflow waits for this signal before executing new work, it never advances past a checkpoint before the payload arrives.

3. **Processing the execution path**
   - Once the workflow receives a metadata payload (blocking until it does), it stores the payload in workflow state, extracting the parent linkage (if present) and the resume details, then begins execution.
   - If `resume.execution_path` is empty or `resume` is omitted, the recipe runs normally.
   - The resume payload is exposed to ops through `ServiceDependencies2.ResumeMetadata()`. Before each op executes, the worker provides the same dependency container populated from the signal so every op can read the execution path.
   - If the op is a `recipe` or `recipe_set`, the head invocation_hash from `resume.execution_path` is compared to the op invocation_hash. If they do not match, the op is executed normally (non-resume behavior). The recipe will continue to other ops, ultimately landing on the given invocation_hash.
   - When the invocation_hash matches:
     - If op is a `recipe`, the child recipe is restarted via the Temporal workflow reset API using the provided `run_id` and `event_id`, and the workflow follows the standard wait logic (wait for sync, don't wait for async). The parent includes its identifiers in the metadata signal so the child can record linkage for story builder. The op also constructs a new `runmetadata.Resume` excluding the consumed head segment and passes that trimmed resume (along with the existing parent metadata) when cloning dependencies for the child.
     - If op is a `recipe_set`, the op moves through each child recipe in its set. Non-matching children run through the standard path. When the child index matches the resume segment, the op uses the Temporal reset API instead of the start-child call, follows the standard wait logic, and forwards a cloned dependency container whose resume metadata omits the head segment for the selected child.
   - After a reset branch is executed, the parent workflow continues with the original dependency view; additional resume segments (if any) are consumed by downstream ops that observe the trimmed resume forwarded to them.

4. **Payload construction**
   - Invoke the execution-path builder described above to produce the ordered list of segments. Each segment becomes one entry in the `resume.execution_path` array with fields:
     - `invocation_hash`: taken from `NodeRun.InvocationHash`.
     - `run_id`: the Temporal `RunID` whose history we will reset (for parent ops this is the parent run, for the leaf it is the run containing the target invocation).
     - `event_id`: the `NodeRun.ResumeEventID` recorded from the workflow task completion that immediately preceded the op.
     - `recipe_set_index` (optional): included only when present on the `NodeRun` and the parent op is `recipe_set`.
   - The builder must validate that every entry has a populated `run_id` and `event_id`; missing values indicate a gap in history capture and should abort rewind with a descriptive error.

## Implementation Steps
1. Implement the recipe_run_metadata signal handler and recipe wait with unit/integration tests (leveraging temporal's workflowtestsuite)
2. Implement resume-aware recipe/recipeset branching:
   - Ensure the workflow passes the `runmetadata.Resume` from the signal straight through `ServiceDependencies2` so ops can inspect it.
   - Update the new `recipe` and `recipe_set` ops to read `deps.ResumeMetadata()` to determine whether they are the target invocation and, when they are, invoke the Temporal reset API with the segment’s `run_id` and `event_id` instead of starting a fresh child workflow execution.
   - When performing a reset, clone the dependency container (and associated run metadata) with a new resume object whose `ExecutionPath` excludes the consumed head segment before handing it to the child recipe.
   - Non-matching ops ignore the resume metadata and continue normal execution. Tests should cover both matching and non-matching cases across recipe and recipeset flows.
3. Add additional needed properties to story builder with unit/integration tests (including the embeddedtemporal ones)
4. Implement the execution-path builder with unit/integration tests
5. Add new embeddedtemporal tests similar to story builder ones entire cycle: run recipe with sub recipe and sub-sub-recipe. Pick a mid op of inner-most recipe and, build rewind execution path and then execute from rewound state. validate that the correct portion of state was replayed at each level.
