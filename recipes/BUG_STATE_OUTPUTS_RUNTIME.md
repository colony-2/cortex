# Bug: Runtime `states.triage` missing after `recipe.run_and_get_result` (no such key: triage)

## Minimal Reproduction Recipe
Single-state machine calling a child recipe and exporting its outputs:
```yaml
id: repro-state-child-outputs
state:
  initial: triage
  states:
    triage:
      op: recipe.run_and_get_result
      inputs:
        name: new-ticket-triage
        inputs:
          title: "Repro title"
          description: "Repro description"
      transitions:
        - to: done
          when: "true"
    done:
      op: command_execution
      inputs:
        run: "echo done"
outputs:
  triage_cell_ok: "{{ states.triage.outputs.outputs.cell_is_appropriate }}"
  triage_rec_cell: "{{ states.triage.outputs.outputs.recommended_cell }}"
```
Child recipe: `new-ticket-triage` (published) returns `cell_is_appropriate`, `recommended_cell`, `rationale` via `llm_inference2` with `response_schema`.

## Behavior
- Validation: **passes** (no guards required).
- Runtime: Top-level outputs fail with `no such key: triage` when resolving `states.triage.outputs.outputs.*`; job ends `completed` with error.

Example (run id `398EIvBP0UqiovaN3fzvPz7CNT5`):
```
"error": "failed to resolve state machine outputs: failed to resolve input 'cell_is_appropriate': failed to evaluate CEL expression: no such key: triage"
```

## Scope
- Appears specific to `recipe.run_and_get_result` child states. A control recipe using a single `command_execution` state and exporting `states.cmd.outputs.*` resolves correctly at runtime.
- Both access patterns (`states.triage.outputs.*` and `states.triage.outputs.outputs.*`) hit the missing-key error at runtime, despite validating.

## Expected
After the `triage` state completes, `states.triage.outputs` should exist and include the child recipe outputs. Top-level outputs should resolve without guards.
