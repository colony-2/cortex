# JobRunStory API (Proposal)

Goal: return a recipe-centric, hierarchical execution story for a job run. The response should read like: "we started recipe X, entered sequence Y, executed op Z (with retries), entered state machine..., evaluated transitions..., spawned child recipes..., etc".

This proposal is a single consolidated tree where each node represents the latest attempt and optionally includes prior attempts.

---

## Endpoint

`GET /api/projects/{projectId}/jobs/{jobId}/story`

Status codes:
- `200` story returned
- `404` job not found
- `409` story cannot be built from the recorded run (recipe mismatch, incomplete run data, etc.)

Response invariants:
- `input` and `output` are always present (may be `null` if not available or not applicable).
- `artifactKeys` are always present (possibly empty). Artifact bodies are always fetched separately.
- Tree only (no timeline/flat list view).

---

## Top-level response: `JobRunStory`

```json
{
  "jobId": "j_456",
  "invocationSequence": 12,
  "recipe": {
    "id": "my-recipe",
    "name": "My Recipe",
    "version": "2026-02-05",
    "source": {
      "kind": "jobStartArtifact",
      "artifactName": "chapter0.recipe.yaml"
    }
  },
  "status": "running",
  "startedAt": "2026-02-05T13:55:00Z",
  "finishedAt": null,
  "root": { "kind": "recipe", "id": "root", "title": "recipe my-recipe", "children": [ /* ... */ ] }
}
```

Notes:
- URI already scopes to a project; `projectId` is not repeated in the response.
- `root` is always a `recipe` node.
- `children` are chronological (story order).

---

## Story tree model

### Core idea: "latest attempt + priorAttempts"

Some things can be reattempted (ops, sequences, state machines, entire recipes, etc.). For those nodes:
- The node returned inline is the latest attempt.
- `priorAttempts` contains the same node shape for prior attempts of that logical node.
- Clients typically render the latest attempt inline and fold `priorAttempts` behind a disclosure.

### `StoryNode` (discriminated union)

All nodes share:

```jsonc
{
  "id": "stable-within-this-story-response",
  "kind": "recipe|sequence|op|opStep|stateMachine|state|transitionEval",
  "title": "human-readable label",
  "status": "pending|running|succeeded|failed|canceled|skipped|unknown",
  "startedAt": "RFC3339 or null",
  "finishedAt": "RFC3339 or null",

  "path": ["root", "sequence:main", "op:llm2"],
  "invokeSeq": 42,

  "attempt": 2,
  "priorAttempts": [ /* StoryNode (same kind) */ ],

  "input": null,
  "output": null,
  "artifactKeys": [ /* ArtifactKey */ ],

  "children": [ /* StoryNode */ ]
}
```

Field notes:
- `path` is an array of strings (breadcrumbs). Every node must include it.
- `invokeSeq` is present on every node, derived from the recipe invocation sequence, so clients can correlate breadcrumbs later.
- `input`/`output` are unmodified JSON values for that node (optimization/compaction can be added later).

### `ArtifactKey` (use existing `swf.ArtifactKey` shape)

To keep things simple and consistent with existing artifact plumbing, `artifactKeys` uses the existing SWF ArtifactKey shape (no extra id/role/content-type fields):

```jsonc
{
  "jobId": "j_456",
  "taskOrdinal": 12,
  "name": "stdout.log",
  "sizeBytes": 12345
}
```

Notes:
- This is not "opaque": it includes name + size + location (task ordinal).
- Artifact bodies are fetched separately by this key.

---

## Node kinds

### 1) `recipe`

Additional fields:

```jsonc
{
  "kind": "recipe",
  "recipeId": "my-recipe",
  "invocation": {
    "args": {}
  }
}
```

### 2) `sequence`

Additional fields:

```jsonc
{
  "kind": "sequence",
  "sequenceId": "baz"
}
```

### 3) `op`

Additional fields:

```jsonc
{
  "kind": "op",
  "opId": "llm2",
  "opType": "llm|command|childRecipe|custom",
  "error": { "message": "rate limited", "code": "RATE_LIMIT" }
}
```

Multi-step ops:
- Some ops have more than one step in their task chain (example: an "input" step, then "run child recipe", then "wait for result").
- When an op is multi-step, steps are represented as child nodes of the op (`kind="opStep"`).
- When an op is single-step (common), no `opStep` children are needed.

### 4) `opStep`

Additional fields:

```jsonc
{
  "kind": "opStep",
  "stepId": "waitForChildResult",
  "stepType": "childRecipeWait|childRecipeRun|input|other",
  "error": null
}
```

Child recipes:
- Running one or more child recipes is modeled as an `op` (and sometimes as `opStep` children under that op).
- Child job IDs should appear in the step `output` when applicable.

### 5) `stateMachine`

Additional fields:

```jsonc
{
  "kind": "stateMachine",
  "stateMachineId": "foo"
}
```

### 6) `state`

Additional fields:

```jsonc
{
  "kind": "state",
  "stateId": "bar",
  "isInitial": false
}
```

### 7) `transitionEval` (single node per decision point)

Represents evaluating a set of transitions for a state and the resulting decision. This node is nested under the `state` where evaluation was applied.

Additional fields:

```jsonc
{
  "kind": "transitionEval",
  "fromStateId": "foo",
  "evaluations": [
    { "toStateId": "bar", "expression": "a = 5", "result": false, "reason": null },
    { "toStateId": "bar", "expression": "a = 10", "result": true, "reason": null }
  ],
  "decision": {
    "kind": "state",
    "toStateId": "bar"
  }
}
```

Decision rules:
- `evaluations` are ordered.
- If one evaluation is true, `decision.kind="state"` and `decision.toStateId` is the first true evaluation's `toStateId`.
- If all are false, `decision.kind="fallthrough"` (no `toStateId`).

---

## How we detect multi-step ops (proposed)

We need a deterministic way to group SWF task chains into recipe-visible steps without exposing SWF types.

Proposal: use the SWF task type naming convention emitted by the recipe executor:
- Single-step op: `taskType == "<opId>:<opId>"`
- Multi-step op: `taskType == "<opId>:<stepId>"` where multiple distinct `stepId` values occur for the same `opId`

Mapping:
- The story contains one `op` node per `<opId>`.
- If the only observed stepId for `<opId>` is `<opId>`, treat it as single-step and do not create `opStep` children.
- If more than one distinct stepId is observed for `<opId>`, create `opStep` nodes (chronological) using those stepIds.

Open question (for implementation): confirm op IDs cannot contain `:`. If they can, we need an escaping or a different delimiter.

---

## Example: narrative-shaped tree (abridged)

```json
{
  "jobId": "j1",
  "invocationSequence": 1,
  "recipe": { "id": "root-recipe", "name": "root-recipe", "version": "v1", "source": { "kind": "jobStartArtifact", "artifactName": "chapter0.recipe.yaml" } },
  "status": "running",
  "startedAt": "2026-02-05T13:50:00Z",
  "finishedAt": null,
  "root": {
    "id": "n_root",
    "kind": "recipe",
    "title": "recipe root-recipe",
    "status": "running",
    "startedAt": "2026-02-05T13:50:00Z",
    "finishedAt": null,
    "path": ["root"],
    "invokeSeq": 1,
    "attempt": 1,
    "priorAttempts": [],
    "input": { "args": {} },
    "output": null,
    "artifactKeys": [
      { "jobId": "j1", "taskOrdinal": 0, "name": "chapter0.recipe.yaml", "sizeBytes": 1234 }
    ],
    "children": [
      {
        "id": "n_seq_main",
        "kind": "sequence",
        "title": "sequence main",
        "status": "running",
        "startedAt": "2026-02-05T13:50:00Z",
        "finishedAt": null,
        "path": ["root", "sequence:main"],
        "invokeSeq": 2,
        "attempt": 1,
        "priorAttempts": [],
        "input": null,
        "output": null,
        "artifactKeys": [],
        "sequenceId": "main",
        "children": [
          {
            "id": "n_op_llm2",
            "kind": "op",
            "title": "op llm2",
            "status": "succeeded",
            "startedAt": "2026-02-05T13:50:20Z",
            "finishedAt": "2026-02-05T13:50:25Z",
            "path": ["root", "sequence:main", "op:llm2"],
            "invokeSeq": 4,
            "attempt": 2,
            "priorAttempts": [
              {
                "id": "n_op_llm2_a1",
                "kind": "op",
                "title": "op llm2 (attempt 1)",
                "status": "failed",
                "startedAt": "2026-02-05T13:50:10Z",
                "finishedAt": "2026-02-05T13:50:12Z",
                "path": ["root", "sequence:main", "op:llm2"],
                "invokeSeq": 4,
                "attempt": 1,
                "priorAttempts": [],
                "input": { "prompt": "..." },
                "output": null,
                "artifactKeys": [],
                "children": [],
                "opId": "llm2",
                "opType": "llm",
                "error": { "message": "timeout", "code": "TIMEOUT" }
              }
            ],
            "input": { "prompt": "..." },
            "output": { "text": "ok" },
            "artifactKeys": [
              { "jobId": "j1", "taskOrdinal": 7, "name": "request.json", "sizeBytes": 200 },
              { "jobId": "j1", "taskOrdinal": 7, "name": "response.json", "sizeBytes": 400 }
            ],
            "children": [],
            "opId": "llm2",
            "opType": "llm",
            "error": null
          },
          {
            "id": "n_sm",
            "kind": "stateMachine",
            "title": "state machine foo",
            "status": "running",
            "startedAt": "2026-02-05T13:51:01Z",
            "finishedAt": null,
            "path": ["root", "sequence:main", "stateMachine:foo"],
            "invokeSeq": 8,
            "attempt": 1,
            "priorAttempts": [],
            "input": null,
            "output": null,
            "artifactKeys": [],
            "children": [
              {
                "id": "n_state_foo",
                "kind": "state",
                "title": "state foo (initial)",
                "status": "succeeded",
                "startedAt": "2026-02-05T13:51:01Z",
                "finishedAt": "2026-02-05T13:51:10Z",
                "path": ["root", "sequence:main", "stateMachine:foo", "state:foo"],
                "invokeSeq": 9,
                "attempt": 1,
                "priorAttempts": [],
                "input": null,
                "output": null,
                "artifactKeys": [],
                "stateId": "foo",
                "isInitial": true,
                "children": [
                  {
                    "id": "n_te",
                    "kind": "transitionEval",
                    "title": "evaluate transitions",
                    "status": "succeeded",
                    "startedAt": "2026-02-05T13:51:05Z",
                    "finishedAt": "2026-02-05T13:51:06Z",
                    "path": ["root", "sequence:main", "stateMachine:foo", "state:foo", "transitionEval"],
                    "invokeSeq": 10,
                    "attempt": 1,
                    "priorAttempts": [],
                    "input": null,
                    "output": null,
                    "artifactKeys": [],
                    "children": [],
                    "fromStateId": "foo",
                    "evaluations": [
                      { "toStateId": "bar", "expression": "a = 5", "result": false, "reason": null },
                      { "toStateId": "bar", "expression": "a = 10", "result": true, "reason": null }
                    ],
                    "decision": { "kind": "state", "toStateId": "bar" }
                  }
                ]
              }
            ],
            "stateMachineId": "foo"
          }
        ]
      }
    ],
    "recipeId": "root-recipe",
    "invocation": { "args": {} }
  }
}
```
