# c2j APIs needed for restart selection and context inspection

## Status: implemented

c2j main commit `9ffc54b1ccb2c1c21df3f01de0f3f12fdd7703b8` provides the
authoritative mapping and context inspection requested here. Cortex now uses
`restart_chapter_ordinal` directly, including historical attempts, and calls
`GetRestartContext` for the full before-state. It no longer infers targets from
invocation prefixes. See [the integration guide](../GUIDE-Restart-Context.md).

The sections below preserve the original requirements and the gap observed before
that implementation. The delivered API identifies boundaries by source job and
chapter ordinal, so it needs no separate revision token. After-state preview is
not part of the delivered API.

## User request

When a user clicks **Restart job** while inspecting a story node, Cortex should
preselect the restart point corresponding to that location. When patching context,
the dialog should show the full context **before** the proposed patch, including
unchanged values. A JSON object containing only patch changes is not that context.

## Historical gap

Rechecked c2j main commit `94484c909de14a2fe2b2a62dfbd82d31595b5b75`
on 2026-10-21. The gaps originally reported against `v0.0.79` remain.

### Concrete invocation loss

In `pkg/story/live/recorder.go`, `OnTaskStart` assigns `InvocationID` to task
steps and attempts. The single-step collapse in `recordingExecutor.ExecuteOp`
copies the child step's task ordinal, attempts, input/output, timing, path, and
status into the operation and then removes the child. It does **not** copy
`InvocationID`. The resulting displayed operation has no invocation identity,
even though the discarded child had one. `copyAttemptIntoStep` also omits this
field (including for recipe-source resolution).

Please preserve these identities and expose the explicit node-to-restart-scope
association below. Copying invocation IDs alone does not define restart targets
for sequences, states, state machines, or historical attempts. Cortex should not
reverse engineer scope identities from display paths, ordinals, timestamps, or
execution-record layouts. This is blocking reliable restart preselection for
every displayed location; the existing manual selector remains the fallback.

- `pkg/story/internal/model/job_run_story.go`: `RestartScope` exposes only
  `scope_id` and `started_at`. `JobRunStoryNode` exposes `invocation_id` on some
  nodes, but no authoritative node-to-restart-target association or full context.
- `pkg/story/live/recorder.go`: task events populate invocation IDs. Some story
  nodes, including operations collapsed from a single task, do not retain that ID.
  Structural nodes cannot consistently be associated with a boundary through the
  existing public response.
- `pkg/story.Service`: exposes story inspection and restart submission, but no
  method to inspect the context at a chosen restart boundary.
- `pkg/worker/compiler/execution_scope.go`: maintains the resolver state on entry
  to a durable scope and applies restart patches to a cloned state. That state is
  the appropriate source for this feature.
- `pkg/template/context_patch.go`: applies job patches to applicable ancestor
  contexts and scoped patches to locally visible `sequence` or `states` outputs.
  This behavior cannot safely be reproduced by merging displayed node inputs and
  outputs in Cortex.

Cortex can now preselect a boundary for nodes that carry a matching recorded
invocation ID, and an unambiguous outer boundary for the recipe. It leaves other
nodes unselected instead of guessing from names or task ordinals. Historical job
attempts are not silently mapped to the current execution's scope list.

The current patch-only display is labeled **Patch payload**, with an explicit
notice that full context is unavailable. The full-context preview remains blocked
on the API additions below; this notice is not the requested finished preview.

## 1. Authoritative restart target for each story location

Extend the story model with a node-to-scope association. For example, a node could
expose `restart_scope_id`, referring to an entry in `restart_scopes`. The exact
field names are negotiable; these are requirements, not an API Cortex assumes
already exists.

The association must:

- Identify the boundary corresponding to the selected node, including operations,
  individual task steps, sequences, states, and state machines.
- Remain correct when wrappers or single-task operations are collapsed for display.
- Distinguish repeated state visits, nested inline recipes, scope retries, task
  retries, and job attempts even when paths and display labels repeat.
- State when a node has no valid restart target. If a containing boundary is used,
  identify it and explain that relationship; do not silently choose a sibling or
  a later invocation.
- Define whether a historical job attempt can be restarted. If not supported,
  expose that restriction rather than returning a current-attempt target with the
  same display path.

Return enough label/path information to present the target understandably while
submitting its exact opaque scope ID. Preserve invocation IDs when constructing
collapsed operation nodes as a separate correctness improvement.

## 2. Read-only context at a restart point

Expose a public service method, such as `GetRestartContext`, taking the tenant/job
identity and recorded scope ID, with any required attempt or execution revision.
It should return:

- The resolved target identity and the execution revision/attempt inspected.
- The complete context that will form the base for a restart at that boundary,
  before the user's proposed patch is applied.
- The job-level context and locally visible sequence/state output containers in
  shapes that can be related directly to `ContextPatch.Job` and `ContextPatch.Scopes`.
- Other template-visible values needed to understand the restart, clearly
  distinguished from editable fields. Unchanged values must remain visible.
- Explicit availability/redaction information. Missing data must not be represented
  as an empty context object.

The response must describe the chosen boundary's state, not the job's original
submission context, the job's final context, or just that node's resolved inputs.
Previously accepted restart patches and completed work retained by restart must
be reflected according to c2j's actual restart semantics.

Capture or reconstruct this through c2j's existing read-only replay and resolver
machinery. Inspection must not execute operations, submit jobs, consume input,
create restart records, or mutate stored execution state. Do not require Cortex
to reconstruct the recipe interpreter or duplicate replay logic.

For running jobs, define how context reads and restart submission refer to a
consistent execution revision. Return a clear stale/unavailable error when an
inspection cannot safely describe the submitted target. Keep existing redaction
rules, and do not expose runtime secrets or credentials through this API.

## Optional: authoritative patch validation and after-state

If an after-state preview is supported, accept the proposed `ContextPatch` and
apply it to a cloned resolver with the same implementation used by restart.
Return before-state, after-state, and validation errors separately. Respect null
deletion, array replacement, local container visibility, and restricted fields
such as workspace identity. Cortex should not implement a second patch engine.

The immediate user requirement is the complete **before-state**; after-state is
useful but must not delay exposing that base context.

## Cortex integration once available

1. Initialize the dialog from the selected node's authoritative restart target.
2. Fetch context for that target when the user enables context patching.
3. Display **Context before patching**, with unchanged values visible, separately
   from **Patch payload** and any future **Context after patching** view.
4. Refresh the context when the restart point changes; cancel or ignore stale
   requests so another target's context is never displayed as the current one.
5. Show loading and read errors explicitly, preserving the user's patch edits.
6. Submit only the proposed patch, never the displayed full context or redacted
   placeholders as replacement values.

## Acceptance tests

- Clicking two different operations preselects two corresponding scopes; closing
  and reopening the dialog does not retain the previous manual selection.
- Repeated visits to the same state and inspected prior attempts select the correct
  occurrence or explicitly report that historical restart is unsupported.
- A single-task operation retains its restart association after story compaction.
- A context with several existing keys remains fully visible when only one is
  patched. The displayed before-state does not change as the user edits the patch.
- Nested sequences/state machines expose only the correct locally visible outputs,
  and later task results do not appear in an earlier boundary's context.
- Context inspection incorporates existing restart history without applying the
  new proposed patch to stored state.
- Unknown scopes, wrong tenants, unavailable historical context, and stale execution
  revisions return explicit errors.
- Inspection produces no operation invocations or durable writes, and sensitive
  values follow the existing redaction policy.
