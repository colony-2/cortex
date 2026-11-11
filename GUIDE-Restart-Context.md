# Restart selection and context inspection

The `pkg/story` service exposes a recorded restart boundary on story nodes and
can reconstruct the complete context at that boundary before a new patch is
applied. The boundary is identified by **tenant/project, source job ID, and
chapter ordinal**. No revision token is required: later chapters do not change
the selected history prefix.

## Select a restart boundary

Fetch the story as usual:

```go
st, err := svc.GetJobRunStory(ctx, story.GetJobRunStoryRequest{
    ProjectID: tenantID,
    JobID:     sourceJobID,
})
```

Each `JobRunStoryNode` may have:

| Go field / JSON field | Meaning |
| --- | --- |
| `RestartChapterOrdinal` / `restart_chapter_ordinal` | Pointer to the recorded scope-entry chapter. Use this for inspection and restart. |
| `RestartRelationship` / `restart_relationship` | `exact` for the node's scope, `containing` for an enclosing scope, or `body` for a recipe's executable root body. |
| `RestartUnavailableReason` / `restart_unavailable_reason` | Explanation when no recorded recipe boundary is available. |

`st.RestartScopes` lists the available boundaries with `chapter_ordinal`,
`scope_id`, `started_at`, and, when associated with a displayed node, `title` and
`path`. Use the ordinal to join the selected node to this list.

Do not substitute `TaskOrdinal`. That field identifies stored task data, not a
restart boundary. Do not calculate a boundary from a node's path or subtract one
from the returned ordinal; the service handles the retained prefix internally.

Collapsed single-task operations retain both their invocation identity and their
restart association. Repeated state visits and nested scope invocations have
their own recorded boundaries. Individual task retries use their containing
recipe scope: restarting there reruns that scope, not only the chosen task retry.
Prior job attempts can be inspected and restarted through their recorded
ordinals; they are never mapped to the current attempt by display path.

Source-resolution nodes and locations without a recorded recipe scope may have
no target. Show the unavailable reason or let the user choose a boundary from
the explicit list.

## Inspect context before patching

```go
if node.RestartChapterOrdinal == nil {
    // Present node.RestartUnavailableReason.
    return
}
ordinal := *node.RestartChapterOrdinal
before, err := svc.GetRestartContext(ctx, story.GetRestartContextRequest{
    ProjectID:      tenantID,
    JobID:          sourceJobID,
    ChapterOrdinal: ordinal,
})
```

The `RestartContext` response contains:

| Field | Contents |
| --- | --- |
| `ProjectID`, `JobID`, `ChapterOrdinal`, `ScopeID` | The exact source and boundary inspected. |
| `Job` | The base job context, projected under `/context` when generating a patch. |
| `Visible` | Template-visible `inputs`, `sequence`, `states`, `vars`, `outputs`, `transition`, `failure`, `scope`, `context`, `item`, and `index`. |
| `RedactedPaths` | JSON pointers to redacted values in the response, for example `/visible/sequence/write/outputs/api_key`. |

The snapshot includes unchanged values. Scoped patches address
`Visible["sequence"][id]["outputs"]` or
`Visible["states"][id]["outputs"]`. These containers contain only outputs
visible at the chosen boundary; a later task's result will not appear there.

The context is captured at scope entry, before that scope resolves its own
inputs or runs its body. Thus `Visible.inputs` belongs to the resolver being
patched and need not equal the displayed operation's resolved inputs.

`Visible` also contains read-only information. Only the existing patch contract
is editable: job-context patches and local sequence/state **outputs**. Workspace
and cell-resolution identities cannot be patched. Artifacts and objects remain
references; inspection does not fetch their content. Runtime paths and other
execution-specific values may remain symbolic, and transient resolver metadata
such as `scope.timestamp` is reconstructed during replay.

Sensitive values use the existing redaction policy. Treat `RedactedPaths` as
metadata and never submit redaction placeholders back as values. Missing or
unreconstructable context is an error, not an empty successful snapshot.

## Submit the restart

Submit the selected ordinal and only the changes the user requested:

```go
patch, err := jobrestart.ParsePatch([]byte(`[
  {"op":"replace","path":"/sequence/write/outputs/document_path","value":"design.md"}
]`))
if err != nil { return err }
result, err := svc.RestartRecipeJob(ctx, story.RestartRecipeJobRequest{
    ProjectID:      tenantID,
    JobID:          sourceJobID,
    ChapterOrdinal: &ordinal,
    Patch:         patch,
})
// result.JobID identifies the new continuation job.
```

The examples use `github.com/colony-2/c2j/pkg/story` and
`github.com/colony-2/c2j/pkg/jobrestart`. Construct the service with the same engine and
schema registry configuration used for existing story/restart calls.

The existing `ScopeID` request field remains supported. Supply either
`ChapterOrdinal` or `ScopeID`, never both. `GetRestartContext` uses the ordinal.
An absent patch means an unpatched restart; a supplied empty array is retained.
The HTTP API accepts RFC 6902 arrays only. Legacy merge objects, supplied null,
malformed operations and unauthorized paths return 400 before job submission.
Use `add`, `remove`, `replace`, `move`, `copy`, or `test`, in execution order.
Escape pointer tokens with `~0` for tilde and `~1` for slash. Dots are literal
characters in API pointers. `add` or `replace` with a null value assigns null;
`remove` deletes a member. Object replacement discards its prior siblings.
Parents and scoped steps must already exist. A structurally valid request can
return 201 and subsequently fail in the worker, for example on a missing target,
source, parent, invisible step or failed test. No synchronous context preflight
is performed. Context application is atomic, but the accepted job remains.

Restart creates a new job and retains the recorded prefix before the selected
scope. It does not rewind or cancel the source job. Previously patched work in
that retained prefix is reflected in inspection. A new restart **at the same
boundary** replaces that boundary's previous patch; the before-state is the
base for the new patch, not the old patched invocation. Restarting without a
patch at that boundary therefore does not retain its previous patch.

## Read-only behavior and errors

Inspection reuses c2j's interpreter and JobDB's read-only replay, stopping before
the selected scope executes. It does not execute tasks, consume external input,
submit jobs, create restart records, or modify stored history. Expression
extension functions may be reevaluated; they are assumed to have no side effects.
Inspection can fail if the history or recipe artifacts needed to reach the
boundary are unavailable or incompatible with replay.

Use `errors.Is` with these public errors:

| Error | Meaning |
| --- | --- |
| `story.ErrInvalidRestartBoundary` | The ordinal is not a recorded patchable scope entry, or both target selectors were supplied. |
| `story.ErrRestartContextUnavailable` | The selected boundary exists but its context could not be reconstructed. The underlying replay error is preserved. |
| `story.ErrNotFound` | The source job is not found in the requested tenant/project. |
| `story.ErrInvalidProject` | The tenant/project was omitted. |

Caller cancellation and deadlines are returned normally. Use the source job ID
for the ordinal; after creating a continuation, fetch its own story before
choosing a boundary in that new job.

## Dialog behavior

1. Initialize selection from the clicked node's restart ordinal on every open.
2. When patching is enabled, fetch and display **Context before patching**.
3. Keep that snapshot separate from **Patch payload**; editing the patch does not
   change the before-state.
4. Refetch when the boundary changes and ignore responses for older selections.
5. Show loading/errors explicitly and preserve the user's patch edits.
6. Submit only the patch, never the complete snapshot.

After-state preview is not part of this API.

## Editing in the browser

The dialog retains global/scoped rows, dot paths and JSON-or-text values. Rows
express recursive merge intent: object members preserve untouched siblings,
null removes a member, and arrays replace as a unit. These browser semantics
are separate from the array-only public API. A row containing null generates
`remove`, not literal null assignment. No operation controls are needed.

Cortex constructs the desired result from the inspected baseline, applies rows
in order, then traverses keys in stable order to generate add/remove/replace
operations. Unchanged values are omitted. The payload preview and request use
the same lossless serialization, including numbers outside JavaScript's safe
integer range. Drafts belong to one tenant, job and selected boundary. Switching
boundaries restores that boundary's draft rather than carrying edits across.

Inspection errors or required redacted/unavailable context block edited
submission and keep drafts available for retry. Unrelated redaction does not
block a known edit. Disable patching to restart without edits. HTTP errors keep
the dialog and its draft; a 201 opens the returned job story. Later worker
failures appear there without automatic resubmission or reopening the dialog.

Frontend and backend must release together. External object-patch callers must
migrate to arrays; historical persisted merge records remain replayable through
c2j. Retry storage uses a new JSON Patch version, preserving identical-payload
request IDs and leaving unrelated session storage unchanged.
