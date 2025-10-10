# Ticket Rewind & Restart Spec

## Overview
Ticket rewinds restart a TicketRecipe at a prior user input checkpoint by supplying a **Resume Signal** that tells the workflow which execution path to traverse when it restarts. Each recipe invocation carries a compiler-assigned node id (e.g., `ticketRecipe.ExecutePlan.implStep1`), and input ops expose their deterministic invocation hashes via signal names. When the workflow receives a resume signal, it pauses at the first `recipe`/`recipe_set` node, inspects the payload, and decides whether to continue normally or to reset/reinvoke child workflows according to the execution path. This document focuses on the resume signal contract; construction of the payload (mapping node ids/hashes to event ids) is covered separately.

## Goals
- Define how TicketRecipe (and child recipes) block on a single `resume` signal before executing.
- Use the signal payload (ordered tuples with Temporal coordinates) to pop execution-path segments and deterministically reinvoke child recipes.
- Ensure the innermost workflow can auto-submit new input or re-prompt when it reaches the terminal tuple.
- Guarantee that only the intended restart run consumes the payload, even if prior runs emit older signals.

## Execution Path Format
Each recipe invocation is identified by a compiler-assigned **recipe node id** (for example `ticketRecipe.ExecutePlan.implStep1`). Input ops retain their deterministic invocation hashes (embedded in signal names such as `input::stateId::hash`). During the original execution, the workflow records the tuple `{recipe_node_id, workflow_id, run_id, event_id}` for every recipe invocation node, and—when applicable—the input signal/ hash for the terminal node. The rewind payload encodes this as:

```json
{
  "execution_path": [
    {
      "recipe_node_id": "ticketRecipe.ExecutePlan",
      "workflow_id": "ticket/core/tmp_abc",
      "run_id": "run-root",
      "event_id": 150,
      "input_signal": null
    },
    {
      "recipe_node_id": "implRecipe.DependencyFanOut",
      "workflow_id": "impl/core/tmp_def",
      "run_id": "run-impl",
      "event_id": 87,
      "input_signal": null
    },
    {
      "recipe_node_id": "ticketRecipe.DependencyTicket",
      "workflow_id": "ticket/dependency/tmp_xyz",
      "run_id": "run-dep",
      "event_id": 203,
      "input_signal": "input::DependencyApproval::hash-789"
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

- `execution_path` is ordered from outermost → innermost and each entry carries `{recipe_node_id, workflow_id, run_id, event_id}`. The terminal entry may also include the input signal (embedding the input’s invocation hash).
- Only the final entry may specify `input_signal`; intermediate entries identify recipe invocation nodes to traverse.
- Exactly one of `new_input` or `re_prompt` must be supplied when `input_signal` is present.

## Resume Signal Behaviour
1. **Prerequisites**
   - Recipe-worker compiler assigns a stable `recipe_node_id` to every recipe invocation node and exposes input invocation hashes via signal names.
   - During execution, workflows record tuples `{recipe_node_id, workflow_id, run_id, event_id}` for each invocation node (and the associated `input_signal` for terminal inputs) via notes/search attributes so the API/UI can retrieve them later.

2. **Signal schema**
   ```json
   {
     "target_run_id": "run-root-uuid",
     "execution_path": [
       {
         "recipe_node_id": "ticketRecipe.ExecutePlan",
         "workflow_id": "ticket/core/tmp_abc",
         "run_id": "run-root",
         "event_id": 150,
         "input_signal": null
       },
       {
         "recipe_node_id": "implRecipe.DependencyFanOut",
         "workflow_id": "impl/core/tmp_def",
         "run_id": "run-impl",
         "event_id": 87,
         "input_signal": null
       },
       {
         "recipe_node_id": "ticketRecipe.DependencyTicket",
         "workflow_id": "ticket/dep/tmp_xyz",
         "run_id": "run-dep",
         "event_id": 203,
         "input_signal": "input::DependencyApproval::hash-789"
       }
     ],
     "new_input": { ... },
     "re_prompt": false,
     "overrides": { ... }
   }
   ```
   - `target_run_id` specifies the workflow run that should consume this signal. If the currently running workflow has a different run id, it ignores the payload (preventing older signals from affecting newer runs).
   - `execution_path` is ordered outermost → innermost; each entry carries the recipe node id (plus terminal input signal) and the Temporal coordinates captured after the original execution.
   - Only the final entry may specify `input_signal`; intermediate entries represent invocation nodes.
   - Exactly one of `new_input` or `re_prompt` must be supplied when `input_signal` is present.

3. **Signal acceptance & waiting**
   - All recipe workflows register a single `resume` signal handler and block on a workflow channel until a valid payload arrives.
   - The handler only accepts payloads where `target_run_id == workflow.GetInfo().WorkflowExecution.RunID`. If the run id differs, the signal was meant for another run and is ignored.
   - Valid payloads are sent over the waiting channel. Duplicate signals for the same run simply replace the stored payload before execution begins. The queue/API always send a resume signal immediately after starting a run: initial runs receive an empty payload (`execution_path: []`), while rewinds provide the desired path.

4. **Processing the execution path**
   - Once the workflow receives a resume payload (blocking until it does), it stores the payload in workflow state and begins execution.
   - When the interpreter reaches a `recipe`/`recipe_set` node, it reads the head of `execution_path`:
     * If the node’s `recipe_node_id` matches the entry, it pops the entry. If additional entries remain, the workflow invokes the child recipe (already reset) and forwards the remaining path via the child’s start parameters; the same resume signal is delivered to the child run (its handler validates `target_run_id`). If no entries remain and `input_signal` is provided, the workflow continues to that input and either auto-submits `new_input` or re-prompts.
     * If the node id does not match, the payload is considered stale; the workflow clears it and continues without rewinding.
   - After the path is exhausted, the payload is cleared to prevent reuse.

5. **Interaction with resets**
   - Before delivering the resume signal, the API issues `ResetWorkflowExecution` for each workflow in the path (outermost → innermost), updating each tuple’s `run_id` to the newly created run.
   - The resume signal is then sent with `target_run_id` set to the outermost new run id. Because the handler validates run ids, any signal delivered to a different run (e.g., `resumeB` arriving at run C) is ignored. Duplicate signals for the same run are harmless because the workflow waits for the latest payload before continuing.

6. **Terminal input handling**
 - When the innermost workflow reaches the specified `input_signal`, it either auto-submits `new_input` (simulating the original signal) or re-prompts via the standard `input` op.

7. **Completion**
  - Workflows proceed normally once the execution path is exhausted. Resume context is cleared so subsequent runs ignore old signals.

> **Payload construction**: The mechanics for extracting tuples and building the resume payload (including reset ordering) are defined in a companion spec.

- **Unknown tuple**: if a tuple doesn’t match any recorded invocation, return `409 execution_path_not_found`.
- **Invalid input_signal**: if terminal node lacks a valid input signal, return `409 input_signal_not_found`.
- **Cancellation timeout**: queue retries or surfaces manual intervention.

## Observability
- UI exposes execution paths (hierarchical view) derived from compiler metadata and allows the user to select one.
- Events `ticket.rewind.requested` / `ticket.rewind.completed` include the execution path and whether new input was auto-supplied.
- Metrics track rewinds per path depth, re-prompts vs auto-submissions.

## Rollout Steps
1. Update recipe-worker compiler/runtime to record `{recipe_node_id, workflow_id, run_id, event_id}` tuples for every recipe invocation (and the input signal for terminal nodes) and surface them via ticket notes/search attributes.
2. Extend API to query available paths (`GET /tickets/{id}/rewind/paths`) and validate inbound execution paths.
3. Teach queues/workflows to register the resume handler, respect `target_run_id`, and consume execution-path entries when traversing child invocations.
4. Update UI to present tree of execution paths and optional input payload entry.
5. Enable feature flag after exercising rewinds across nested ticket/impl chains.

## Open Questions
- Should execution path entries include versioning metadata to handle updated recipe definitions? (Likely yes—include run id/hash.)
- How to handle long-lived child workflows still running when rewind requested? (Option: cancel them before restart as today.)
- Do we allow batching multiple rewinds in one request? (Out of scope for now.)
