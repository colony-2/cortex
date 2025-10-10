# Agent-Driven Development Program Rollout

## Purpose
Summarize the sequencing of feature and recipe work required to deliver the Ticket/Plan/Impl/Deprecation program. This overview references the detailed specs in `server/cortex/docs/specs/` and identifies critical dependencies between them.

## Phase 1 — Infrastructure Foundations
1. **Temporal Ticket Queue & API Orchestration** (`ticket-queue-temporal-spec.md`)
   - Implements per-cell Temporal queue workflow, REST ↔ Temporal signaling, and dependency validation plumbing.
   - Establishes FIFO execution, keeps Temporal as system of record, and unblocks all downstream recipes.
2. **Ticket Rewind & Restart** (`ticket-rewind-spec.md`)
   - Snapshot capture, API endpoint, workflow termination/resume logic.
   - Needed before exposing TicketRecipe widely to ensure recovery path.

## Phase 2 — Core Ticket Lifecycle
3. **PlanRecipe** (`plan-recipe-spec.md`)
   - Introduces `.colony2/specs` authoring and RecipePlan schema.
   - Requires dependency helper (Phase 1) for plan validation.
4. **ImplRecipe** (`impl-recipe-spec.md`)
   - Builds on plan outputs; depends on codex + testing harness maturity.
5. **TicketRecipe** (`ticket-recipe-spec.md`)
   - Wires Plan/Impl recipes together, handles approvals, generates deprecation plan.
   - Must integrate with concurrency + rewind features.

## Phase 3 — Deprecation Follow-Through
6. **DeprecationRecipe** (`deprecation-recipe-spec.md`)
   - Consumes DeprecationPlan, discovers consumers via detection tooling, runs child TicketRecipes in parallel, and executes repeated cleanup/fast-forward loops.
7. **Git Fast-Forward Merge Enhancement** (`git-fast-forward-spec.md`)
   - Adds `skip_rebase` mode and structured `NotFastForward` errors to `squashrebasemerge` for deprecation workflows.

## Phase 4 — Polish & Operations
8. UI/UX updates for ticket creation, plan visualization, rewind history, and deprecation detection/merge telemetry.
9. Observability/metrics dashboards leveraging telemetry outlined in each spec.
10. End-to-end integration tests and load validation across representative cells.

## Dependency Graph
- TicketRecipe depends on PlanRecipe + ImplRecipe + Rewind + Temporal queue/API orchestration.
- PlanRecipe depends on the queue/API layer (for dependency validation hooks) and Template updates.
- ImplRecipe depends on PlanRecipe outputs and the queue/API layer (for dependency gating).
- DeprecationRecipe depends on git fast-forward merge enhancement and TicketRecipe outputs.

## Testing Strategy
- Unit: new services (locks/queue), plan/implementation helpers, git op enhancements (`skip_rebase`).
- Integration: Temporal workflow simulations using test fixtures mirroring `.colony2` structure.
- End-to-end: API -> TicketRecipe -> Plan/Impl -> Deprecation using ephemeral worktrees.

## Rollout Recommendations
- Start with beta cells to validate lock + rewind behaviour before enabling global.
- Instrument phased feature flags aligning with phases above.
- Provide playbook for manual lock release and fast-forward merge remediation during initial adoption.
