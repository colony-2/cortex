# Agent Orchestration Spec (Plans, Evaluation, Ticket Splitting)

## Overview

Define a multi-agent workflow that turns a user ticket into a concrete plan, evaluates the plan, iterates on feedback, splits work into per-cell subplans, and manages tickets. Agents operate as separate Codex sessions and exchange artifacts via the job context artifact inbox/outbox paths. The workflow uses a state machine with explicit loop limits and CEL checks for artifact existence.

## Goals

- Generate a plan markdown file in the artifact outbox for the ticket's cell.
- Confirm plan creation via command execution; if missing, diagnose and retry.
- Evaluate the plan with a separate agent and iterate up to five loops.
- Split the plan into per-cell subplans, one cell per spec, ordered for implementation.
- Create new tickets for each subplan except the last; update the current ticket to the final subplan.
- Keep prompts concise and focused on the core goal; avoid extras.

## Non-Goals

- Implementing new ops in this spec.
- Changing the runtime or execution engine behavior.
- Replacing existing ticket or workflow systems.

## Context Review (contextual.JobContext)

Use the JobContext values to scope file paths and prompts.

```go
type JobContext struct {
	Actor       ActorContext       `json:"actor,omitempty"`       // ticket_id, actor_name, actor_email
	Ticket      TicketContext      `json:"ticket,omitempty"`      // id, title, description, creator, created_at, updated_at
	Environment EnvironmentContext `json:"environment,omitempty"` // worktree_path, workdir, inbox, outbox
	Workflow    WorkflowContext    `json:"workflow,omitempty"`    // cell, cell_path, job_id, project_id
	GitBase     GitBaseContext     `json:"git,omitempty"`         // repo, ref, resolved_hash, author
}
```

### Required fields for orchestration

- `context.ticket.id`, `context.ticket.title`, `context.ticket.description`
- `context.workflow.cell_name`, `context.workflow.cell_path`, `context.workflow.project_id`
- `context.environment.inbox`, `context.environment.outbox`

## Artifact Paths and File Naming

Use the artifact outbox for all generated documents. Use deterministic, versioned names to prevent agent confusion.

### Base Paths

- Artifact inbox: `context.environment.inbox`
- Artifact outbox: `context.environment.outbox`

### File Names (outbox)

- Plan (draft/revisions): `planwriter_plan_v{n}.md`
- Plan evaluation: `planevaluator_feedback_v{n}.md`
- Missing-plan analysis: `planwriter_missing_analysis_v{n}.md`
- Planwriter question summary to user: `planwriter_questions_v{n}.md`
- Subplans (per cell): `subplan_{order}_{cell}.md`
  - `{order}` is zero-padded (e.g., `01`, `02`)
  - `{cell}` is the cell name from `context.workflow.cell_name` or the target cell name for that spec

If the outbox is already scoped per cell, write directly into it; otherwise, use a `cell_{cell_name}` subfolder.

## Required Ops

- `codex` for planwriter, plan evaluator, and diagnostic summaries.
- `commandexecution` to confirm file creation (existence + non-empty).
- `ticket.manage` to create new tickets and update the current ticket.

## High-Level Workflow

1. **Planwriter session** drafts a plan and writes `planwriter_plan_v1.md`.
2. **Command execution** confirms the plan file exists and is non-empty.
3. **If missing**, use a "flash" LLM to diagnose why; decide whether to push back to planwriter or ask the user.
4. **Planevaluator session** reviews plan and writes `planevaluator_feedback_v1.md`.
5. **Planwriter session** revises the plan to address feedback; repeat up to five loops.
6. **Planwriter session** splits final plan into per-cell subplans (`subplan_XX_cell.md`).
7. **Ticket management**: create new tickets for all subplans except the last; update the current ticket to the last subplan.

## State Machine

Use explicit states and CEL checks for file existence. Max plan-eval loops: 5.

### States

1. `PlanDraft`
2. `PlanVerify`
3. `PlanDiagnose`
4. `PlanEvaluate`
5. `PlanRevise`
6. `SubplanSplit`
7. `TicketCreate`
8. `TicketUpdate`
9. `Done`

### State Transitions (pseudocode)

```
state PlanDraft:
  run codex(planwriter_prompt, output=planwriter_plan_v{n}.md)
  goto PlanVerify

state PlanVerify:
  run commandexecution("test -s <outbox>/planwriter_plan_v{n}.md")
  if success -> PlanEvaluate
  else -> PlanDiagnose

state PlanDiagnose:
  run codex_flash(diagnose_missing_plan, output=planwriter_missing_analysis_v{n}.md)
  if diagnosis.contains_questions -> ask user via input op
  else -> retry PlanDraft (n = n + 1)

state PlanEvaluate:
  run codex(planevaluator_prompt, input=planwriter_plan_v{n}.md, output=planevaluator_feedback_v{n}.md)
  goto PlanRevise

state PlanRevise:
  run codex(planwriter_revision_prompt, input=planevaluator_feedback_v{n}.md, output=planwriter_plan_v{n+1}.md)
  if loop_count >= 5 -> SubplanSplit
  else -> PlanVerify

state SubplanSplit:
  run codex(planwriter_split_prompt, input=planwriter_plan_v{latest}.md, outputs=subplan_XX_cell.md)
  goto TicketCreate

state TicketCreate:
  for each subplan except last:
    run ticket.manage(create_ticket, title/description from subplan)
  goto TicketUpdate

state TicketUpdate:
  run ticket.manage(update_ticket, ticket_id=context.ticket.id, title/description from last subplan)
  goto Done
```

### CEL Checks

Use CEL expressions to confirm required artifacts exist before transitions, e.g.:

```
exists(artifacts.outbox["planwriter_plan_v" + string(loop_index) + ".md"])
```

## Planwriter Prompt (Draft)

Use a prompt that is concise and scope-bound.

```
You are Planwriter. Write a focused, minimal plan for the ticket.
Constraints:
- Only plan the core goal from the ticket title/description.
- No extra features or bells-and-whistles.
- Keep it actionable and ordered.
- Output markdown to <outbox>/planwriter_plan_v{n}.md.
Context:
- Ticket: {ticket.title} - {ticket.description}
- Cell: {workflow.cell_name} ({workflow.cell_path})
- Project: {workflow.project_id}
```

## Plan Evaluator Prompt

```
You are Planevaluator. Review the plan for feasibility, missing steps, risks, and clarity.
- Provide concrete feedback and required changes.
- Keep it short and precise.
- Output markdown to <outbox>/planevaluator_feedback_v{n}.md.
Input plan: <outbox>/planwriter_plan_v{n}.md
```

## Planwriter Prompt (Revision)

```
Revise the plan to address all Planevaluator feedback.
- Be explicit about changes.
- Keep scope minimal.
- Output markdown to <outbox>/planwriter_plan_v{n+1}.md.
Inputs: <outbox>/planwriter_plan_v{n}.md, <outbox>/planevaluator_feedback_v{n}.md
```

## Handling Missing Plan Output

If the plan file is missing:

1. Use a "flash" LLM to categorize:
   - **Questions**: asks for clarification or missing inputs.
   - **Complaints**: refusal or vague inability.
2. If **questions**, write `planwriter_questions_v{n}.md` and send an input op to the user with the summarized questions.
3. If **complaints**, re-prompt planwriter with more positive language and a simplified instruction set.

## Subplan Splitting

Planwriter must split the final plan into per-cell subplans:

- Each subplan is for exactly one cell.
- Use `moon query projects` if needed to identify cells.
- Number subplans in implementation order.
- Output: `subplan_01_<cell>.md`, `subplan_02_<cell>.md`, ...

Subplan template:

```
# Subplan {order}: {cell}

## Scope
...

## Steps
1. ...
2. ...

## Files/Areas
- ...
```

## Ticket Management

Use `ticket.manage` actions:

- Create tickets from each subplan except the last.
- Update the current ticket to match the last subplan's title/description.

### Title/Description Rules

- Title: use the subplan header or derived summary.
- Description: include scope + steps (keep concise).

## Open Questions / Potential Enhancements

- Consider adding a dedicated op to list cells/projects without invoking shell commands (reduces reliance on `commandexecution`).
- Consider a `ticket.manage` action for bulk creation to reduce sequential ops.
- Consider standardized "artifact exists" op to remove shell checks.

## Acceptance Criteria

- Plan is written to outbox with the expected filename.
- Plan evaluation doc exists and is addressed within five loops.
- Subplans are per-cell and numbered.
- New tickets created for all subplans except last.
- Current ticket updated to final subplan scope.
