# Mutability Control: `const` Nodes

## Problem

Many recipe operations are logically read-only — they validate, inspect, or report without intending to change the codebase. But Colony2's auto-commit model persists all changes after every op. There's no way to enforce that an op shouldn't mutate git state, and no way for the recipe engine to distinguish a validation step from an implementation step.

This matters for:
- Validation ops (CI, linting, test suites) that should never produce code changes
- Code review ops (`codex.exec` with a review prompt) that should observe but not modify
- Diagnostic commands (`npm audit`, `go vet`) that are purely informational
- Any future op that runs in an external environment (e.g., `gha.run`) where mutation control is critical

## The `const` Property

Any node type (`op`, `sequence`, `state`) can be marked `const: true`. This is analogous to `const` in programming — the node and everything inside it is read-only with respect to git state.

```yaml
- id: validate
  op: command_execution
  const: true
  inputs:
    run: "npm test"
```

## Semantics

When a node is `const`:

1. **No git state advancement.** The post-op auto-commit is skipped. The git state pointer does not advance. The next op in the recipe sees the same commit hash as its input.

2. **Worktree is disposable.** The temporary worktree created for the op is discarded after execution, regardless of what the op wrote. For remote execution contexts, no thin pack is fetched back.

3. **Artifacts are still captured.** `const` prevents git mutation, not artifact production. Op artifacts, logs, and structured outputs all flow through normally. The op can produce information without producing code changes.

4. **Outputs are still available.** The op's outputs are accessible in transition conditions and later steps. `const` only constrains git, not data flow.

## Propagation Through Containers

`const` propagates downward through container nodes. A `const` sequence or `const` state machine makes all children `const`:

```yaml
# Every op in this sequence is const — no git mutation anywhere
- id: checks
  const: true
  sequence:
    - id: lint
      op: command_execution
      inputs:
        run: "npm run lint"
    - id: test
      op: command_execution
      inputs:
        run: "npm test"
    - id: audit
      op: command_execution
      inputs:
        run: "npm audit --audit-level=high"
```

A child cannot override a parent's `const: true`. If the parent is `const`, the child is `const` regardless of what it declares. (A non-`const` parent can have `const` children — constness is additive, never subtracted.)

```yaml
# Mixed: implementation mutates, validation doesn't
sequence:
  - id: implement
    op: codex.exec
    # not const — changes are committed
    inputs:
      prompt: "Implement the feature."

  - id: validate
    op: command_execution
    const: true
    # const — changes discarded, git state doesn't advance
    inputs:
      run: "npm test"
```

## Enforcement

| Context | Enforcement mechanism |
|---------|----------------------|
| **Local ops** | After the op completes, the engine compares the worktree state to the input commit. If there are changes and the node is `const`, changes are discarded instead of committed. |
| **Remote execution** | No thin pack is fetched. The remote ref is cleaned up without importing state. |
| **Child recipes** | `const` does not propagate across recipe boundaries. A child recipe invoked from a `const` node runs with its own `const` setting. |

## Use Cases

### Validation

```yaml
- id: ci
  op: command_execution
  const: true
  inputs:
    run: "npm test"
    continue_on_error: true
```

### Code Review (Agent-Based)

```yaml
- id: code_review
  op: codex.exec
  const: true
  inputs:
    prompt: "Review the implementation and provide feedback. Do not modify any files."
```

### Diagnostic Commands

```yaml
- id: check_deps
  op: command_execution
  const: true
  inputs:
    run: "npm audit --audit-level=high"
```

### Validation Phase in State Machine

```yaml
state:
  initial: implement
  states:
    implement:
      op: codex.exec
      transitions:
        - to: validate
          when: "true"
      inputs: { ... }

    validate:
      const: true
      sequence:
        - id: lint
          op: command_execution
          inputs:
            run: "npm run lint"
            continue_on_error: true
        - id: test
          op: command_execution
          inputs:
            run: "npm test"
            continue_on_error: true
      outputs:
        all_passed: '${{ sequence.lint.outputs.exit_code == 0 && sequence.test.outputs.exit_code == 0 }}'
      transitions:
        - to: implement
          when: "!outputs.all_passed"
        - to: merge
          when: "true"
```

## Design Decisions

1. **Violation behavior.** When a `const` op mutates the worktree, changes are silently discarded. The op succeeds, its outputs and artifacts are captured, but the git state pointer does not advance. No error, no warning — the engine treats the worktree as disposable.

2. **Default.** `const` defaults to `false`. All ops are mutable unless explicitly marked. This is a recipe-level declaration, not an op-level default.

3. **Child recipes.** `const` does not propagate to child recipes invoked via `recipe.run_and_get_result`. If a recipe author wants a child recipe to be `const`, they should define the property in the child recipe itself (or pass it as a recipe input). The parent's `const` only applies to the parent's own node tree.

4. **Scope.** `const` applies exclusively to git state — that is the only mutating state in recipes. Outputs, artifacts, and logs are always separate per-node (node1 and node2 have independent outputs, they don't override each other). There is nothing else to constrain.
