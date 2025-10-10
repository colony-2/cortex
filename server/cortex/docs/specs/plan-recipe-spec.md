# PlanRecipe Specification

## Overview
PlanRecipe transforms ticket requirements into executable specifications and a structured `RecipePlan` that drives downstream implementation. It runs entirely within the owning cell's workspace and does not modify code.

## Inputs
- `ticket_id` (string)
- `cell` (string)
- `description` (markdown)
- `existing_specs` (list of blob references)
- `constraints` (e.g., dependency budget)

## Outputs
- `spec_documents` (array of `{path, blob_uri, summary}`)
- `recipe_plan` (see schema below)
- `validation_scores` (map of validator -> last score)
- `needs_human_review` (bool)

## Directory Conventions
- Specifications saved under `.colony2/specs/{ticket_id}-{slug}.md`.
- RecipePlan written to `.colony2/plan/{ticket_id}.yaml`.
- Metadata returned to TicketRecipe for attachment via `ticket.manage`.

## RecipePlan Schema
YAML file with top-level keys:
```yaml
version: 1
owning_cell: core
stages:
  - name: parallel-deps
    kind: parallel
    steps:
      - recipe: ticket
        cell: depcell1
        spec_doc: .colony2/specs/dep1.md
      - recipe: ticket
        cell: depcell2
        spec_doc: .colony2/specs/dep2.md
  - name: impl-phase
    kind: sequence
    steps:
      - recipe: impl
        cell: core
        spec_doc: .colony2/specs/impl1.md
        git_state: shared
      - recipe: impl
        cell: core
        spec_doc: .colony2/specs/impl2.md
```

Validation:
- `kind` is `parallel` or `sequence`.
- `recipe` enumerations: `ticket`, `impl`, `noop` (for coordination markers).
- Optional `inputs` map forwarded to child recipe invocation.
- For non-owning cells, `git_state` defaults to `discrete`.

## State Machine
```
Bootstrap -> DraftSpecs -> ReviewCycle -> PlanSynthesis -> ValidationLoop -> Finalize
```

### Bootstrap
- Gather context: load ticket description, existing specs, dependency graph snapshot.
- Seed prompt template for Codex sessions.

### DraftSpecs
- Run `codex.exec` to generate spec proposals (session `plan-draft`).
- On each turn, persist drafts to `.colony2/specs` via `codex.exec` file write instructions.
- Call `ticket.manage` `link_markdown_doc` for newly created specs.

### ReviewCycle
- For each spec, run independent validator sessions:
  - `backwards-compatibility` via `codex.exec` (distinct session).
  - `coverage` validator.
  - `scope-drift` validator (checks for extraneous features).
- Validators append issues to aggregated feedback list.

### PlanSynthesis
- Convert accepted specs into `RecipePlan` structure using `codex.exec` with template referencing spec list and dependency graph.
- Write plan file to `.colony2/plan` and return path.

### ValidationLoop
- Evaluate quality scores from validators (1..5). If any below threshold (default 4), gather human feedback via `input` op and return to `DraftSpecs` with adjustments.
- Ensure plan respects dependency rules by verifying each step's cell using new dependency helper; flag blocking errors.

### Finalize
- Summarize plan decisions and validator scores; emit aggregated note via `ticket.manage`.
- Output plan URI and spec metadata.

## Error Handling
- If `codex.exec` returns `incomplete` with `pendingDependencies`, propagate to TicketRecipe for dependency resolution.
- On validator disagreement, mark `needs_human_review` and halt awaiting manual confirmation.

## Observability
- Log number of spec iterations, validator scores, and plan stages.
- Provide recipe outputs for plan metrics to TicketRecipe.

## Implementation Notes
- Use shared gitstate to retain `.colony2` artifacts for downstream phases.
- Manage codex session IDs deterministically (`plan-draft`, `validator-bc`, etc.) so rewinds can resume.
- Template prompts should live under `server/recipe-worker/templates/plan/` with entries in cheatsheet.

## Open Questions
- Should plan include estimated effort for scheduling? (Optional extension.)
- How to handle spec deletions on re-run? (Proposed: track existing docs and use `ticket.manage` `override_markdown_doc`.)
