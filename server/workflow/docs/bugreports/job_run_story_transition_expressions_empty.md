# Bug: `JobRunStory` transition expressions are empty

## Symptom
`GET /api/projects/{projectId}/jobs/{jobId}/story` returns `transitionEval.evaluations[].expression` as `""` (empty string) even when the recipe YAML has a non-empty `when:` expression.

Example (from REST):

```json
{
  "kind": "transitionEval",
  "evaluations": [
    {
      "expression": "",
      "result": true,
      "to_state_id": "requirements_planning"
    }
  ]
}
```

## Repro (integration test)
There is an integration test that currently fails and reproduces the issue:

```bash
cd /src/server/workflow
go test -tags=integration ./internal/story -run TestIntegration_JobRunStoryTransitionExpressions_ArePreserved
```

Expected: first `evaluations[0].expression` equals the recipe text:

```
has(outputs.exit_code) && outputs.exit_code == 0
```

Actual: `evaluations[0].expression == ""`.

## Why this happens (suspected root cause)
The story recorder takes the expression from the recipe-worker state machine compiler callback:

- `compiler.evaluateTransitionsWithContext(...)` calls `obs.TransitionEvalauted(transition.When.String(), shouldTransition, transition.To)`

So if `transition.When.String()` is empty, the story will show `""`.

One likely reason `transition.When.String()` becomes empty is that recipe parsing is currently swallowing transition decode errors:

- `recipe.State.UnmarshalYAML` decodes transitions via `_ = node.Decode(&meta)` (error ignored).

If the `when:` expression fails to decode/compile (for example, expressions using unqualified `outputs.*` which the template CEL validation currently disallows), the decode error is dropped and the transition's `When` field stays at its zero value (empty expression).

Downstream effects:
- `transition.When.String()` is `""`.
- Runtime transition evaluation treats empty expression as `true` (because `EvaluateCEL("")` is `true`), so the first transition always matches.
- Story shows `expression: ""` even though the YAML had a string.

## Proposed fixes
Pick one (or both) depending on intended language support for transition `when:`:

1) **Do not swallow transition decode/compile errors**
   - Return an error from `State.UnmarshalYAML` if decoding `transitions` fails.
   - This prevents silently turning non-empty `when:` into `""` and avoids “first transition always wins”.

2) **If unqualified `outputs.*` is intended to be valid in state transitions**
   - Update the transition evaluation context to support an `outputs` CEL variable (bound to the current state's outputs), and ensure the compile/check environment used during YAML decode also supports it.
   - Update/adjust template validation tests accordingly (today they assert unqualified `outputs.*` is not allowed).

Either way, once transitions reliably retain their original `when:` text, `JobRunStory` should surface it consistently.

