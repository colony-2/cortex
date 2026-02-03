# Bug: `recipe.run_and_get_result` panics when child recipe fails

## Minimal Reproduction

### Child recipe (fails)
```yaml
id: repro-child-fail
sequence:
  - id: boom
    op: command_execution
    inputs:
      run: |
        echo "child failing"
        exit 1
```

### Parent recipe (calls child)
```yaml
id: repro-parent-child-fail
sequence:
  - id: child
    op: recipe.run_and_get_result
    inputs:
      name: repro-child-fail
      inputs: {}
  - id: after
    op: command_execution
    inputs:
      run: |
        set -euo pipefail
        mkdir -p "{{ context.environment.outbox }}"
        echo "after ran" > "{{ context.environment.outbox }}/after.txt"
```

## Observed behavior
Running the parent recipe results in a server panic (nil pointer dereference), rather than a structured child-failure propagated to the parent:

Run id `398v0hq6zNrtbvPaFcLO4b47yt9`:
```
status: failed
error: "panic: runtime error: invalid memory address or nil pointer dereference"
```

No artifacts are produced (`c2 workflow artifact list <run-id>` returns `[]`), and `c2 workflow output get <run-id>` reports `no chapter has output`.

## Expected behavior
- Parent should fail cleanly with a structured error indicating the child recipe failure (e.g., “child failed: exit status 1”), and not panic.
- Parent should not execute subsequent steps (`after`) when the child fails.
