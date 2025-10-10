# Ticket Rewind & Restart Spec

## Overview
Ticket rewinds restart a TicketRecipe at a prior user input checkpoint by supplying an **execution path** built from the deterministic invocation hashes emitted at each recipe/input node, augmented with the Temporal coordinates needed to reset history. Because Temporal assigns event IDs only when events are written, the execution path entries include the tuple `{invocation_hash, workflow_id, run_id, event_id}` captured after the original execution. The rewind request lists these tuples from the root TicketRecipe down to the targeted input, plus optional new signal payload. Temporal replay brings each workflow back to its invocation point; the runtime then replays up to the reset boundary and reinvokes children via Temporal resets until the final tuple reaches the desired input signal.

## Goals
- Accept a rewind request containing `execution_path` (array of tuples `{invocation_hash, workflow_id, run_id, event_id}`) and optionally `input_signal`/`new_input` for the innermost tuple.
- Cancel and restart the outermost workflow, replaying history until it encounters the recorded invocation path; at each level the workflow reinvokes the child with the remainder of the path.
- Allow the innermost workflow to auto-submit new input or re-prompt when it reaches the terminal hash.
- Keep Temporal as the system of record, avoiding custom history mutation.

## Execution Path Format
Each recipe invocation already emits a deterministic **invocation hash** (used in signal names such as `input::stateId::hash`). Child recipes inherit the parent path by appending their own hash, so nested invocations produce a unique chain even across loops. During the original execution, the workflow records the tuple `{invocation_hash, workflow_id, run_id, event_id}` for each invocation/input (event id is the workflow task event immediately after the invocation). The rewind payload encodes this as:

```json
{
  "execution_path": [
    {
      "invocation_hash": "root-123",
      "workflow_id": "ticket/core/tmp_abc",
      "run_id": "run-root",
      "event_id": 150,
      "input_signal": null
    },
    {
      "invocation_hash": "impl-456",
      "workflow_id": "impl/core/tmp_def",
      "run_id": "run-impl",
      "event_id": 87,
      "input_signal": null
    },
    {
      "invocation_hash": "depTicket-789",
      "workflow_id": "ticket/dependency/tmp_xyz",
      "run_id": "run-dep",
      "event_id": 203,
      "input_signal": "input::DependencyApproval::depTicket-789"
    }
  ],
  "actor": { "type": "user", "email": "pm@example.com" },
  "reason": "Need to adjust dependency scope",
  "override_requirements": [],
  "new_input": {
    "fields": { "decision": "replan", "notes": "Drop optional feature" }
  },
  "re_prompt": false
}
```

Rules:
- `execution_path` is ordered from outermost → innermost and each entry carries `{invocation_hash, workflow_id, run_id, event_id}`.
- Only the final entry may specify `input_signal`; intermediate entries identify invocation nodes to traverse.
- Exactly one of `new_input` or `re_prompt` must be supplied when `input_signal` is present.

## Implementation Sketch
1. **Runtime metadata**:
   - recipe-worker emits invocation hashes for every recipe node (including inputs).
   - After each invocation/input completes, the workflow records `{invocation_hash, workflow_id, run_id, event_id}` via `ticket.manage` notes and/or search attributes so the API/UI can fetch the tuple later.
   - When invoking child recipes, the runtime appends the child’s tuple to the accumulated path and stores it alongside the child’s async handle.
2. **Rewind request flow**:
   - API validates permissions and resolves the target execution path directly from the recorded tuples.
  - For each workflow in the path (outermost → innermost), the API calls Temporal `ResetWorkflowExecution`, trimming history after the recorded `event_id` and creating a new run that will replay only up to that point.
  - After issuing resets, the API sends an `ApplyRewindContext` signal to the new outermost run carrying `{ execution_path, overrides, new_input?, re_prompt?, input_signal? }`.
3. **Queue handling**:
   - The per-cell queue observes the reset-created run and re-applies `ExecutionGranted` so execution resumes. No additional cancellation is necessary because history beyond the reset point has been removed.
4. **Workflow replay**:
   - Each reset-generated run deterministically replays events up to its retained `event_id` and stops. Because subsequent events were pruned, there are no stale signals or activities for Temporal to deliver.
   - As the workflow processes the replay, it consumes the first tuple from `execution_path`. If more tuples remain, it invokes the child recipe (already reset) and forwards the remaining path; otherwise it continues locally toward the terminal input checkpoint.
5. **Terminal input**:
   - When the innermost workflow reaches the specified `input_signal`, it either auto-submits `new_input` or re-prompts, following the rewind payload.
6. **Completion**:
   - Workflows proceed normally from their reset points, emitting standard outputs. Since history after the reset was removed, Temporal guarantees no post-checkpoint events replay.

- **Unknown tuple**: if a tuple doesn’t match any recorded invocation, return `409 execution_path_not_found`.
- **Invalid input_signal**: if terminal hash lacks a valid input signal, return `409 input_signal_not_found`.
- **Cancellation timeout**: queue retries or surfaces manual intervention.

## Observability
- UI exposes execution paths (hierarchical view) derived from compiler metadata and allows the user to select one.
- Events `ticket.rewind.requested` / `ticket.rewind.completed` include the execution path and whether new input was auto-supplied.
- Metrics track rewinds per path depth, re-prompts vs auto-submissions.

## Rollout Steps
1. Update recipe-worker compiler/runtime to record `{invocation_hash, workflow_id, run_id, event_id}` tuples for every recipe invocation/input and surface them via ticket notes/search attributes.
2. Extend API to query available paths (`GET /tickets/{id}/rewind/paths`) and validate inbound execution paths.
3. Teach queues/workflows to carry `rewind_context` and recursively continue-as-new along the provided execution path.
4. Update UI to present tree of execution paths and optional input payload entry.
5. Enable feature flag after exercising rewinds across nested ticket/impl chains.

## Open Questions
- Should execution path hashes include versioning to handle updated recipe definitions? (Likely yes—include run id/hash.)
- How to handle long-lived child workflows still running when rewind requested? (Option: cancel them before restart as today.)
- Do we allow batching multiple rewinds in one request? (Out of scope for now.)
