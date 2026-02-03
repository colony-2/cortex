# Bug: Runtime state-machine outputs can't see `states` (states map is empty)

## Minimal Reproduction Recipe (no child recipe)
State machine with a single command state, exporting the `states` map:
```yaml
id: repro-states-dump
state:
  initial: a
  states:
    a:
      op: command_execution
      inputs:
        run: |
          echo hi
outputs:
  states_string: "{{ string(states) }}"
  has_a: "{{ 'a' in states }}"
```

### Observed runtime output
Run id `398uqyGm2FOb4qLTQaCLvFa1qq0`:
```json
{"states_string":"{}","has_a":false}
```
So `states` is `{}` at top-level output resolution time.

## Related reproduction (child recipe)
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

Example (run id `398ufvd1hYuJjdsYZTwvlqPI1jh`):
```
"error": "failed to resolve state machine outputs: failed to resolve input 'triage_cell_ok': failed to evaluate CEL expression: no such key: triage"
```

## Scope
- Not specific to `recipe.run_and_get_result`. Even in a one-state machine (`repro-states-dump`), the top-level outputs see `states` as an empty map `{}` at runtime.

## Expected
After a state-machine run, top-level outputs should have access to the executed states under `states.<state>.outputs` (and `states` should at least include the executed state keys).
