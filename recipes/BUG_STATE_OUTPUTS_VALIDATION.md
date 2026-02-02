# Bug: Validation missing-key on state outputs for child recipe (resolved)

## Summary
Validation previously failed with `no such key: recommended_cell` when a state-machine recipe exported child recipe outputs from a `recipe.run_and_get_result` state. The issue is now fixed, but keeping this record for traceability.

## Minimal Reproduction (at the time)
Parent recipe (`repro-triage-missing-state.yaml`):
```yaml
id: repro-triage-missing-state
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

## Validation Error (fixed)
```
failed to resolve state machine outputs: failed to resolve input 'triage_rec_cell': failed to evaluate CEL expression: no such key: recommended_cell
```

## Status
- Validation now succeeds for this recipe (`valid: true`), so the missing-key validation bug is resolved.
- Runtime missing-key for `states.triage` is tracked separately in `BUG_STATE_OUTPUTS_RUNTIME.md`.
