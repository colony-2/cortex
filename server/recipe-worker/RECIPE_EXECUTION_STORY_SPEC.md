# Recipe Execution Story Spec

## Background
- `server/recipe-worker` runs compiled recipes as Temporal workflows where most ops execute as activities (see `server/recipe-worker/pkg/compiler/compiler.go:154`).
- History inspection today (`server/recipe-history/pkg/history/transformer.go`) only surfaces activity timelines and ignores inline ops, state transitions, and signal-driven input ops.
- We want a structured "story" artefact that can power a notebook-style replay showing each op, its inputs/outputs, decision points, and user interactions.

## Goals
- Produce a deterministic `RecipeExecutionStory` object for a single workflow run using only persisted Temporal history plus the recipe definition.
- Capture both chronological timeline and recipe-structure hierarchy (recipe → sequences/states → ops).
- Include resolved inputs, outputs, errors, attempts, durations, and any branching decisions.
- Represent inline ops (e.g., input await) and Temporal signals alongside activity-based ops.
- Enable notebook rendering by providing semantically rich event types (op run, decision, user input, log annotations) that can be serialized to JSON.

## Non-Goals
- Building the notebook UI itself.
- Changing recipe YAML or op authoring requirements beyond optional story hooks.
- Replacing existing job/activity summaries in `server/recipe-history` (they will consume the new builder).

## Target Story Output
```json
{
  "metadata": {
    "workflow_id": "ono-recipes.my-recipe.2024-06-01",
    "run_id": "a1b2",
    "recipe_id": "my-recipe",
    "status": "completed",
    "started_at": "2024-06-01T12:00:00Z",
    "completed_at": "2024-06-01T12:03:12Z",
    "attempt": 1
  },
  "timeline": [
    {"event_id": 12, "at": "2024-06-01T12:00:01Z", "ref": "node:build/app#op"},
    {"event_id": 27, "at": "2024-06-01T12:01:10Z", "ref": "decision:deploy.branch"},
    {"event_id": 33, "at": "2024-06-01T12:01:30Z", "ref": "input:approval.wait"},
    {"event_id": 41, "at": "2024-06-01T12:02:05Z", "ref": "input:approval.response"}
  ],
  "nodes": [{
    "path": "recipe-state/order-flow/seq[0]/approve-release",
    "type": "op",
    "op_type": "ops.input.collect",
    "display_name": "approve-release",
    "status": "completed",
    "attempts": 1,
    "started_at": "2024-06-01T12:01:30Z",
    "completed_at": "2024-06-01T12:02:05Z",
    "inputs": {"prompt": "Ship release?"},
    "outputs": {"response": "yes"},
    "events": [
      {"kind": "input-response", "user_id": "jdoe", "fields": {"decision": "yes"}}
    ],
    "children": []
  }]
}
```

## Story Model
- `StoryMetadata` — workflow/run identifiers, recipe metadata, status, durations, attempt.
- `StoryNode` — hierarchical node representing recipe elements:
  - `type`: `recipe|sequence|state|op`.
  - `path`: deterministic path from `invocationTracker` (`recipe-worker/pkg/compiler/invocation_tracker.go`) and the `ops.Invocation` hash.
  - `display_name`: recipe-friendly label (metadata ID or op name).
  - `status`: `scheduled|running|completed|failed|timed_out|canceled`.
  - `started_at`, `completed_at`, `duration`, `attempts`.
  - `inputs`, `outputs`, `error`.
  - `events`: ordered list of enriched sub-events (decisions, user input, logs).
  - `children`: nested nodes (sequence entries, state substates, downstream ops).
- `ops.Invocation` remains the canonical execution key; the story keeps both the hash (`ID`) and the structured fields so other systems can correlate without new identifiers.
- `StoryEvent` variants:
  - `op-start` / `op-complete` / `op-fail` (auto-generated from history).
  - `decision` — derived during story build by replaying the state machine over recorded node outputs and transition definitions.
  - `input-response` — emitted when a corresponding signal is processed.
  - `log` — optional annotation from inline ops.
- `StoryTimelineEntry` — flattened timeline referencing `StoryNode` IDs plus event IDs for notebook playback.

## Temporal History Mapping
| History event | Story effect |
| --- | --- |
| `WorkflowExecutionStarted` | Seed `StoryMetadata` (start time, input summary).
| `ActivityTaskScheduled` | Decode `workerops.ActivityInvocationRequest` to capture resolved inputs and invocation path. Create/locate `StoryNode` and mark `status=scheduled`.
| `ActivityTaskStarted` | Set `started_at`.
| `ActivityTaskCompleted` | Attach outputs (payload decoding via existing converters), mark `status=completed`, append `op-complete` event.
| `ActivityTaskFailed` / `TimedOut` | Attach failure payload, update status, append `op-fail` event.
| `WorkflowExecutionSignaled` (input responses) | Map to the relevant invocation using signal name `user-response:<id>`; append `input-response` event with payload.
| `MarkerRecorded` (new instrumentation) | Inline op boundaries (start/complete/fail), child-recipe lifecycle, optional logs.
| `WorkflowExecutionCompleted` / `Failed` etc. | Finalize `StoryMetadata.status` and `completed_at`.

## Required Instrumentation
### Marker Helpers
- Provide typed helper functions for each situation we need to record. Each helper accepts fully defined structs so payload schemas stay consistent and self-documenting.
  - `RecordInlineOpStart(ctx workflow.Context, meta InlineOpStartPayload)`
  - `RecordInlineOpComplete(ctx workflow.Context, meta InlineOpCompletePayload)`
  - `RecordInlineOpFail(ctx workflow.Context, meta InlineOpFailPayload)`
  - `RecordInlineOpTimeout(ctx workflow.Context, meta InlineOpTimeoutPayload)`
  - `RecordChildRecipeTrigger(ctx workflow.Context, meta ChildRecipeTriggerPayload)`
  - `RecordChildRecipeResult(ctx workflow.Context, meta ChildRecipeResultPayload)`
  - Optional `RecordInlineLog(ctx workflow.Context, meta InlineLogPayload)` for structured logging.
- Helpers wrap `workflow.RecordMarker` with a shared envelope (`kind`, `stage`, `invocation`, `payload`). Inline ops and recipe/recipeset ops invoke the appropriate helper at their natural boundaries.

### Inline Ops
- Extend `executeOp` (`server/recipe-worker/pkg/compiler/compiler.go:154`) to:
  1. Record an `inline-op` marker before invoking inline ops (stage `start`) with invocation metadata and resolved inputs.
  2. Record corresponding `complete` / `fail` markers with outputs or summarized errors.
- Markers focus on canonical data (op type, invocation ID/hash, resolved inputs/outputs, summary error). Any richer narration happens at build time.

### Input Ops
- In `ops.input` inline workflow (`server/ops/pkg/input/workflow.go`): rely on the inline-op start/complete markers plus `user-response:<id>` signal events to narrate the wait/respond cycle; do not emit special input markers.

### Recipe / RecipeSet Ops
- When a recipe or recipeset op schedules child recipes, emit markers around launch and completion that include child workflow IDs, invocation hash, and summary result (status, duration).
- For recipesets, include per-child identifiers so the builder can assemble rollups (counts, failure reasons).

### Marker Schema
- Envelope (common to all helpers):
  ```json
  {
    "kind": "inline-op|child-recipe|log",
    "stage": "start|complete|fail|timeout|trigger|result",
    "path": "recipe-state/...",
    "invocation": {
      "id": "...",
      "node_path": "...",
      "seq": 2,
      "recipe_id": "..."
    },
    "payload": { ... typed struct ... }
  }
  ```
- Typed payloads:
  ```go
  type InlineOpStartPayload struct {
      OpType      string                 `json:"op_type"`
      Inputs      map[string]interface{} `json:"inputs"`
      StartedAt   time.Time              `json:"started_at"`
  }

  type InlineOpCompletePayload struct {
      OpType     string                 `json:"op_type"`
      Outputs    map[string]interface{} `json:"outputs"`
      CompletedAt time.Time             `json:"completed_at"`
      Attempts    int                   `json:"attempts"`
  }

  type InlineOpFailPayload struct {
      OpType     string    `json:"op_type"`
      ErrorType  string    `json:"error_type"`
      Message    string    `json:"message"`
      FailedAt   time.Time `json:"failed_at"`
      Attempts   int       `json:"attempts"`
  }

  type InlineOpTimeoutPayload struct {
      OpType    string    `json:"op_type"`
      TimeoutAt time.Time `json:"timeout_at"`
      Duration  string    `json:"duration"`
  }

  type ChildRecipeTriggerPayload struct {
      ChildWorkflowID string                 `json:"child_workflow_id"`
      ChildRunID      string                 `json:"child_run_id,omitempty"`
      TriggeredAt     time.Time              `json:"triggered_at"`
      Inputs          map[string]interface{} `json:"inputs,omitempty"`
  }

  type ChildRecipeResultPayload struct {
      ChildWorkflowID string                 `json:"child_workflow_id"`
      ChildRunID      string                 `json:"child_run_id"`
      Status          string                 `json:"status"`
      CompletedAt     time.Time              `json:"completed_at"`
      Outputs         map[string]interface{} `json:"outputs,omitempty"`
      ErrorMessage    string                 `json:"error_message,omitempty"`
  }

  type InlineLogPayload struct {
      Level   string                 `json:"level"`
      Message string                 `json:"message"`
      Fields  map[string]interface{} `json:"fields,omitempty"`
      LoggedAt time.Time             `json:"logged_at"`
  }
  ```
- Payloads are serialized via Temporal’s data converter. Keep payloads under ~10 KB; truncate or summarize large data (store raw artifacts elsewhere if necessary).

## Story Builder Pipeline
1. **Fetch history**: `client.GetWorkflowHistory` with `history.IterationType=Raw` to obtain all events.
2. **Seed structures**: load recipe definition via existing `GetRecipeFunc` to build static tree skeleton (node IDs, ordering).
3. **Replay events**:
   - Build maps: `scheduledEventID → StoryNode`, `nodePath → StoryNode`, `invocation.ID → StoryNode` using hashes emitted in activity payloads/markers.
   - Apply activity events to nodes.
   - Apply marker events (inline ops, inputs, child recipes) by node path.
   - Merge signal events into matching input nodes using `ActivityID`/`BoxID` from payloads.
4. **State-machine replay**: walk the recipe states/sequences using recorded node outputs and inline markers to reconstruct transitions; emit `decision` story events for each evaluated guard.
5. **Finalize hierarchy**: compute durations, status rollups, and attach decision trails to parent states and sequences.
6. **Generate timeline**: sort collected `StoryEvent`s by Temporal event time, produce flat timeline array referencing node IDs + event IDs for UI playback.
6. **Serialize**: output `RecipeExecutionStory` (Go struct) with JSON tags for API responses. Provide helper to convert to notebook cells later.

## Story-Time Annotations
- Introduce an optional build-time extension in `recipe-core/pkg/ops`:
  ```go
  type StoryAnnotator interface {
      AnnotateStory(inv ops.Invocation, phase StoryPhase, snapshot StorySnapshot) *StoryAnnotation
  }
  ```
  - `StoryPhase` enumerates `Start|Complete|Failure|Await|Response|Decision`.
  - `StorySnapshot` contains the canonical facts collected from history (inputs, outputs, errors, marker payloads, signal data).
- During story building, look up the `RegisterableOp` by metadata type; if it implements `StoryAnnotator`, merge its annotation into the node/events (e.g., natural language summary, diff metadata, documentation links).
- Keeps workflow execution deterministic while allowing notebook presentation to evolve without redeploying workers.

## State-Machine Replay Details
- The story builder reuses the existing state-machine compiler utilities to re-run transitions offline using the captured node outputs and inline-op payloads.
- For each state, the replay gathers the same `ResolutionContext` data the workflow had (sequence outputs, state outputs, inline op results) from markers/history and evaluates transition CEL expressions in order.
- The resulting `decision` story events include:
  - state name and attempt index,
  - expression string from the recipe definition,
  - resolved value snapshot (basic scalar/JSON data),
  - boolean outcome and selected target state (if any).
- Loops and retries are handled by iterating until the recorded execution path matches the observed activities/markers; because inline ops emitted start/complete markers with invocation IDs, we can align each replay step with the real history without additional runtime instrumentation.

## Recipe and RecipeSet Ops
- Recipe ops (`ops.recipe.execute`) and recipe set ops execute child recipes as separate workflows.
- Use `ops.Invocation` (especially `ID`, `NodePath`, `InvokeSeq`, `RecipeID`) as the stable link between parent nodes, activity payloads, and any child histories—no duplicate key types needed.
- Emit markers when a child recipe is launched and when it completes, including `child_workflow_id`/`run_id`.
- Story builder responsibilities:
  - Detect child-launch markers and fetch nested stories via `BuildStory`, composing them into parent nodes (inline preview + deep link).
  - Aggregate recipe set results (counts by status, list of child links) while preserving individual child stories for notebook drill-down.

## API Surfaces
- `server/recipe-worker/pkg/compiler/storymarkers.go` (new file) exposing lightweight helpers for `RecordInlineOpMarker`, `RecordInputMarker`, `RecordChildRecipeMarker` that wrap `workflow.RecordMarker`.
- `server/recipe-history/pkg/storybuilder` (new package or sub-package of `history`) with:
  - `func BuildStory(ctx context.Context, temporal client.Client, recipeName, workflowID, runID string) (*story.Story, error)`.
  - Shared structs (`Story`, `StoryNode`, `StoryEvent`, etc.) for API/server reuse.
- HTTP/API exposure comes later via gateway changes once the builder is stable.

## Implementation Plan
1. **Instrumentation groundwork**
   - Add marker helper functions in `recipe-worker` and instrument inline ops, input workflows, and recipe/recipeset ops to emit start/complete markers with invocation metadata.
2. **History builder**
   - Implement story builder in `server/recipe-history` decoding activity + marker events into `StoryNode`s and replaying state machines.
   - Validate against fixture histories (extend `server/recipe-history/pkg/history/integration_test.go`).
3. **Notebook data contract**
   - Finalize JSON schema / Go structs and document in `api/openapi` for future endpoint.
   - Provide example story snapshot under `server/recipe-worker/test-fixtures/story/`.
4. **Incremental adoption**
   - Start with activity + inline input ops, then extend markers and enable build-time annotations as ops opt into `StoryAnnotator`.
   - Add metrics to flag missing markers so we can backfill instrumentation (e.g., inline ops without start/complete markers).

## Open Questions / Risks
- Marker volume: even without transition markers we should monitor inline-op logging to stay within Temporal limits (~50k events).
- Data privacy: user inputs may contain sensitive data; notebook UI must respect masking controls.
- Historical runs executed before instrumentation will lack markers; builder should degrade gracefully (activity ops still render, missing inline nodes flagged as `unknown-inline-op`).
- Should we persist story snapshots externally to avoid rebuilding on every notebook open? (Out of scope for first iteration but noted.)
