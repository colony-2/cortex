# Const Nodes Thin-Pack Implementation Plan

This plan implements the `const` behavior described in `CONST_NODES.md` by first fixing the existing no-change git-state bug. After that fix, `const` can reuse the exact same git-state semantics as a normal unchanged op while still discarding local mutations and forwarding the prior thin-pack artifact.

## Current Execution Shape

Today the worker path is:

1. Parse node metadata in `recipe-core/pkg/recipe/node.go`.
2. Build per-node execution context in `recipe-template/pkg/template/template_resolver.go`.
3. Serialize git/task state into `git/pkg/gitstate/GlobalGitTaskContext` from `recipe-worker/pkg/compiler/compiler.go`.
4. Run the op in `recipe-worker/pkg/ops/op_executor.go`.
5. Always call `Controller.PersistWithDiffs(...)`, which creates a new thin pack when changes exist.
6. Let `recipe-worker/pkg/compiler/artifact_job_context.go` remember the last thin-pack artifact and prepend it to later tasks.

Two existing behaviors are useful here:

- Local worktrees are already disposable because each op runs in a temp directory that is deleted after the invocation.
- The thin-pack forwarder already preserves the previous thin pack when a task does not emit a new one.

The missing pieces are:

- node metadata does not yet expose `const`;
- `const` does not propagate through sequence/state containers;
- the current no-change path can regress from hash-mode back to ref-mode;
- the worker does not have a way to force "treat this as unchanged" for a const op, even if the op mutated files locally.

## Required Invariant

Git state must be monotonic across tasks: advance or stay the same, never go backwards.

For an unchanged task result:

- if the task started in hash-mode (`PersistHash` set), it must return the same hash-mode state unchanged;
- if the task started without a hash, it must remain hashless.

This preserves an important existing behavior:

- when a task is still ref-backed, a long delay between tasks is allowed to pick up a newer ref tip on the next restore;
- once a task has materialized a hash, later unchanged tasks must stay pinned to that hash.

## Target Behavior

For a normal no-change op:

- if it started with a hash, return the same `PersistHash`/`ParentHash`;
- if it started without a hash, return no hash;
- if an input thin-pack artifact exists, pass it through unchanged;
- do not create a new thin pack or diff artifacts.

For an effective-`const` op:

- restore the workspace exactly as today;
- run the op exactly as today;
- collect normal op outputs and outbox artifacts exactly as today;
- behave exactly like the fixed no-change case, regardless of whether the worktree was modified locally;
- if an input thin-pack artifact exists, append that same artifact to the output so downstream nodes continue from the prior mutable state.

For descendants of a `const` sequence/state:

- they behave exactly like directly-declared `const` ops;
- a child cannot opt back into persistence.

For child recipes:

- do not inherit `const` automatically across the recipe boundary.

## Implementation Plan

### 1. Add declared `const` to recipe metadata

Files:

- `recipe-core/pkg/recipe/node.go`
- `recipe-core/pkg/recipe/schema.go`
- generated schema artifacts that are checked in (`recipe-worker/schema.json`, `recipe-worker/oas.json`, or other generated outputs if they are still authoritative)

Changes:

- Add `Const bool \`yaml:"const,omitempty"\`` to `recipe.NodeMetadata`.
- Ensure the JSON schema for op/sequence/state nodes accepts `const: true`.
- Add parser/schema tests that cover:
  - `const` on an op;
  - `const` on a sequence;
  - `const` on a state.

Notes:

- This field is the declared value from YAML only.
- Parent-to-child inheritance should happen at runtime, not by mutating parsed child metadata.

### 2. Track effective const-ness in execution context

Files:

- `recipe-template/pkg/template/template_resolver.go`
- tests under `recipe-template/pkg/template`

Changes:

- Add an execution flag on `ResolutionContext` such as `EffectiveConst bool`.
- Root recipe contexts start with `false`.
- In `NewChildContext`, compute:
  - `child.EffectiveConst = rc.EffectiveConst || metadata.Const`
- Do not propagate this flag when starting a new recipe job; child recipes must start clean unless their own recipe declares `const`.

Why this layer:

- `ResolutionContext` already owns parent/child scope semantics.
- This keeps inheritance logic in one place for ops, sequences, and state-machine children.

### 3. Serialize the effective flag to workers

Files:

- `git/pkg/gitstate/git_task_context.go`
- `recipe-worker/pkg/compiler/compiler.go`
- `recipe-worker/pkg/ops/activity_registry.go`
- tests under `recipe-worker/pkg/compiler` and `recipe-worker/pkg/ops`

Changes:

- Extend the serialized invocation context so the worker can tell an op is effectively `const`.
  - Either add `Const bool` to `gitstate.GlobalGitTaskContext`, or add an explicit field to `ActivityInvocationRequest`.
- Reuse the existing `PersistHash` and `ParentHash` already present in `gitstate.GlobalGitTaskContext` so unchanged/hash-mode can be preserved without inventing a new state channel.

### 4. Fix normal no-change finalization first

Files:

- `recipe-worker/pkg/ops/op_executor.go`
- optionally `git/pkg/gitstate/workspace_controller.go` if a helper improves clarity

Changes:

- Refactor git-result finalization in the executor around the invariant above.
- Capture the incoming git state before restore/persist.
- After `PersistWithDiffs(...)`:
  - if changes were made, keep the existing advancing behavior;
  - if no changes were made and the input started in hash-mode, return the incoming `PersistHash`/`ParentHash` unchanged;
  - if no changes were made and the input started without a hash, return no hash.
- Keep the existing thin-pack pass-through behavior for unchanged ops.

Why this is the right fix:

- It preserves today's desirable ref-backed behavior for long gaps between tasks.
- It prevents the current regression where an unchanged task that started pinned to a hash falls back to ref-mode.
- Once this is correct, const becomes a small wrapper over the same unchanged finalization path.

### 5. Implement const by forcing unchanged finalization

Files:

- `recipe-worker/pkg/ops/op_executor.go`

Changes:

- After `controller.Restore(...)` and `reg.Step.Invoke(...)`, branch on the effective-const flag.
- For const ops:
  - do not call `controller.PersistWithDiffs(...)`;
  - do not generate a new thin pack;
  - do not generate `diff_from_parent.diff` or `diff_from_base.diff`;
  - route through the same unchanged finalization helper used by normal no-change behavior;
  - if `thinPackArtifact != nil`, append that same artifact to `outputArtifacts`.
- For mutable ops, keep the existing persistence path.

Why this is sufficient locally:

- The temp worktree is already deleted at the end of the invocation.
- Skipping persistence automatically gives the desired "discard local mutations" behavior.

Important detail:

- Do not model const behavior as a separate git-state contract.
- Const should be "force unchanged finalization", not "invent a new result shape".

### 6. Rely on existing thin-pack forwarding, but make pass-through explicit

Files:

- `recipe-worker/pkg/compiler/artifact_job_context.go`
- tests under `recipe-worker/pkg/compiler`

Changes:

- No major algorithm change should be required here.
- Keep using the existing forwarder that remembers the last thin pack and prepends it to future tasks.
- Still have const ops emit the same inbound thin-pack artifact when one exists.

Why keep explicit pass-through:

- It matches the existing no-change pass-through behavior.
- It keeps last-step/job-result artifact lists stable.
- It makes const behavior observable and testable without relying on hidden forwarder state.

### 7. Cover propagation through sequences and state machines

Files:

- `recipe-worker/pkg/compiler/compiler.go`
- `recipe-worker/pkg/compiler/statemachine_compiler.go`
- tests under `recipe-worker/pkg/compiler`

Changes:

- No executor branching should be needed in sequence/state-machine code once `ResolutionContext.EffectiveConst` is inherited correctly.
- Add integration coverage for:
  - mutable op -> const op -> mutable op;
  - mutable op -> const sequence -> mutable op;
  - state machine with a const validation state.

Expected result:

- the const subtree sees the prior mutable git state;
- mutations inside the const subtree are discarded;
- the next mutable node restores from the same prior thin pack and can continue the commit chain.

## Test Plan

### Unit tests

`recipe-core/pkg/recipe`

- YAML parse/schema accepts `const` on op/sequence/state nodes.

`recipe-template/pkg/template`

- `EffectiveConst` is inherited by children.
- A child with `const: false` under a const parent still resolves to effective const.
- A fresh recipe root does not inherit `const` from a caller.

`recipe-worker/pkg/ops`

- Normal no-change after hash-mode input:
  - input has `PersistHash = hashA`, `ParentHash = parentA`;
  - output returns the same `PersistHash = hashA`, `ParentHash = parentA`;
  - input thin-pack is passed through unchanged.

- Normal no-change after ref-mode input:
  - input has no `PersistHash`;
  - output stays hashless;
  - no new thin-pack artifacts are created.

- Effective-const op that makes worktree changes:
  - returns success and normal op outputs;
  - returns no new thin pack or diff artifacts;
  - returns the same git-state result that a normal no-change op would have returned for the same input mode;
  - returns the same inbound thin-pack artifact when one exists.

### Integration tests

`recipe-worker/pkg/compiler`

- Mutable -> no-change -> mutable chain:
  - first mutable step creates thin pack `A` and hash-mode state;
  - unchanged middle step returns the same hash-mode state and same thin pack `A`;
  - final mutable step restores from `A` rather than falling back to ref-mode.

- Mutable -> const -> mutable chain:
  - first mutable step creates thin pack `A`;
  - const step mutates files but is finalized exactly like no-change, returning thin pack `A` unchanged;
  - second mutable step restores from `A`, does not see the const step's discarded edits, and emits thin pack `B` if it makes changes.

- Const sequence/state propagation:
  - inner ops inherit const-ness from the parent container without each child declaring it.

- State-machine validation loop:
  - repeated const states reuse the same prior thin pack until a later mutable state advances git state.

## Docs Follow-Up

Update `CONST_NODES.md` after implementation to make the local-runtime rule explicit:

- const ops skip persistence entirely;
- no new thin pack or diff artifacts are generated;
- the prior thin-pack artifact is passed through unchanged when present.

If remote execution lands later (`gha.run` or similar), mirror the same contract there: const remote executions should not import a new git state and should leave downstream thin-pack flow unchanged.
