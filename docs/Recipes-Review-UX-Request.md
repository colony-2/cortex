# Request: make build reviews readable and actionable

Audience: `colony-2/recipes` maintainers, with Cortex and c2j coordination where noted.

## Problem

A user reviewing a build in Cortex received a long decision prompt containing
internal phase results as JSON, several empty `{}` sections, and a single radio
option labeled “Revise the plan.” The actual request was to resolve an ownership
question before work could continue. The review did not make that distinction
clear, and the agent's explanation appeared inside the user's response area.

Please make the recipe produce a human-facing explanation, a specific question,
and actions appropriate to the current checkpoint. Cortex should render the
supplied form without understanding build phases or parsing JSON embedded in
question text.

## Source investigated

Inspected `colony-2/recipes` main at commit
`318f8073a3d9756a9cc41108c9f8ab6c785b0fdd`:

- [build.yaml](https://github.com/colony-2/recipes/blob/318f8073a3d9756a9cc41108c9f8ab6c785b0fdd/build.yaml)
  delegates to the shared develop workflow.
- [develop.yaml, plan_feedback](https://github.com/colony-2/recipes/blob/318f8073a3d9756a9cc41108c9f8ab6c785b0fdd/recipes/develop/develop.yaml#L198)
  defines the form matching the reported content.
- [agent.yaml, design phase](https://github.com/colony-2/recipes/blob/318f8073a3d9756a9cc41108c9f8ab6c785b0fdd/recipes/develop/agent.yaml#L335)
  instructs the agent to ask for clarification when the mandate is missing or
  ambiguous, and defines the result schema.

The running job's exact recipe commit was not supplied. The inspected template
matches the reported structure and wording; this is not a claim that the running
job used that exact commit.

## Example received

The user supplied rendered HTML. The following reconstructs the relevant form
fields from that HTML, preserving the displayed question, labels, and option
value. It is not a captured API response. Request IDs, document references, and
other undisclosed metadata are intentionally omitted.

```yaml
fields:
  - id: decision
    type: multiple_choice
    required: true
    question: |-
      Build implements the requested software behavior in the selected cell directory.

      Resolve questions and review findings before implementation.

      Design:
      {"next":"ask_user","summary":"The request provisionally fits c2j ownership, but .c2j/mandate.md is missing. Wrote an outbox-only design with a proposed initial mandate: c2j owns recipe compilation/execution and extension integration; JobDB owns generic durable timing, dispatchers/provisioners own allocation, extension owners own domain ops, and colony-2/recipes owns shared workflow policy. Should this proposal become the initial mandate, including the recipe-timeout versus JobDB timing boundary, or is there an existing ownership document to use? No external work agreed, implementation performed, or tests run."}

      Design review:
      {}

      Test plan:
      {}

      Test review:
      {}

      Implementation:
      {}

      What should change or what is the answer to the open questions?

      How should we proceed? Uploaded edits require another revision and review.
    options:
      - value: revise
        label: Revise the plan
  - id: feedback
    type: paragraph_text
    question: Feedback or answers to open questions. Select a revision decision to request changes.
  - id: annotated_design
    type: file_upload
    question: Optional annotated design (Markdown may include CriticMarkup).
  - id: annotated_test_statements
    type: file_upload
    question: Optional annotated test statements.
```

The reported screen placed the entire decision question below **Your response**,
as the label for a radio group containing only **Revise the plan**. Feedback and
upload fields followed, then **Cancel** and **Submit**.

Separately, clicking Submit failed before making a request:

```text
Failed to submit response: TypeError: crypto.randomUUID is not a function
    at rs.submitReview (inputActivityService.ts:433:29)
```

That exception is a Cortex browser compatibility bug, not a recipe defect.

## What is fixed structure versus generated content?

| Layer | Responsibility |
| --- | --- |
| Recipe | Form fields, option values and labels, section headings, serialization of phase results, empty-result defaults, and transitions after submission. |
| LLM | Situation-specific summary and a `next` value constrained by the recipe's result schema. The summary can vary substantially between runs. |
| c2j input/review contract | Form representation, declared field validation, review identity, document references, and response acceptance. It does not assign workflow semantics to the name `decision` or the value `revise`. |
| Cortex | Placement of content, choice controls, document previews, submission, and display of errors. |

The single option and JSON dumps are authored by the recipe. They are not an LLM
inventing the form layout. Conversely, the particular missing-mandate explanation
must not become a special case in Cortex or the shared recipe renderer.

## Requested recipe changes

1. **Explain why the review is needed.** Distinguish a request for clarification
   from approval of a completed plan or a request to revise completed work. Make
   the current blocker and the next useful user action clear.
2. **Present readable content instead of serialized results.** Select the relevant
   explanation and questions from phase results. Omit absent phases. Keep routing
   values and diagnostic objects in execution details, rather than the primary
   review prompt. Preserve relevant limitations, such as work not yet performed,
   in plain language when they affect the reviewer's decision.
3. **Use checkpoint-appropriate labels.** For this clarification checkpoint,
   “Send feedback” or “Send answer” is clearer than “Revise the plan.” A label can
   change while retaining the existing internal `revise` value and transitions.
   Do not introduce an approval action where the workflow does not support one.
4. **Separate explanation from answer instructions.** The prompt should state the
   actual question. The feedback label should describe what to enter, without
   telling users to manipulate a radio control that another client might render
   differently.
5. **Avoid a silent repeat when nothing useful is submitted.** The inspected
   `plan_feedback` transitions return to design only when `revise` includes
   non-whitespace feedback or an attachment; otherwise they return to
   `plan_feedback`. Explain the missing answer to the user. If text OR an attachment
   is valid, do not make text universally required just to approximate that rule.
6. **Keep attachment guidance relevant.** Where feasible, offer annotations for
   documents actually available at this checkpoint. Explain any applicable
   revision consequence near the upload controls in user-facing terms.

## Example of the intended presentation

This is an illustration for the reported situation, not mandatory wording or a
new API schema:

> **Clarification needed: ownership mandate**
>
> I drafted a proposed ownership mandate because `.c2j/mandate.md` is missing.
> It places recipe execution and extension integration in c2j, generic durable
> timing in JobDB, allocation in dispatchers/provisioners, domain operations with
> extension owners, and shared workflow policy in colony-2/recipes.
>
> Should this proposal become the initial mandate, including the boundary between
> recipe timeouts and JobDB timing, or should I use an existing ownership document?
>
> Implementation has not started and tests have not run.

Then show any available review documents, followed by **Your response**, a
**Feedback or answer** field, relevant optional attachments, and a **Send feedback**
action. Explain that submitting an answer or annotated document returns the work
for revision. Keep the full explanation and any important caveats available;
readability should not come from silently truncating the agent's content.

## Coordination with Cortex and c2j

The current form contract overloads a field's `question` with both review context
and the decision question. Recipe wording can improve immediately within that
contract. A cleaner future contract could expose a separate review introduction
and explicit submit-action metadata. Agree that contract with c2j and Cortex
before emitting new properties; those properties are a proposal, not an existing
API capability.

Cortex changes have been implemented locally to:

- Generate submission IDs when `crypto.randomUUID` is unavailable.
- Present options as submit actions when a review has exactly one required
  multiple-choice field, with no other multiple-choice fields. Optional or
  multiple choice questions retain ordinary form controls.
- Move that choice field's prompt above **Your response** and show submission
  errors with the response form while preserving entered feedback.

The action rendering rule is a Cortex presentation convention, not a formal c2j
decision type. It uses field structure and preserves supplied IDs, labels, and
values. It does not inspect `next`, recognize build phase names, parse JSON in a
question, remove `{}` sections, or infer that feedback must be supplied.

These changes are not evidence of a deployed release. No recipes repository
changes were made as part of this investigation.

## Acceptance criteria

- A clarification review states the blocker and asks the specific question without
  exposing raw phase-result JSON, routing values, or empty phase sections.
- Approval and clarification checkpoints have distinct, accurate action labels.
- Different LLM summaries and questions use the same presentation structure;
  rendering does not depend on the mandate example or particular English phrases.
- Text-only feedback and attachment-only feedback follow the intended transitions.
  An empty or whitespace-only response without attachments produces an actionable
  explanation instead of an unexplained repeat of the review.
- Uploaded edits preserve the existing revision/review requirement, and the UI
  explains that consequence without changing workflow policy.
- Original review documents remain immutable.
- Recipe tests exercise the actual build transitions, including clarification,
  approval, uploads, and insufficient feedback. Cortex browser tests exercise
  rendering and submission through the generic form contract.

Cortex's current browser coverage uses deterministic review fixtures against the
real Cortex server and c2j worker. It covers the reported single-option shape,
multi-option reviews, missing `randomUUID`, validation, uploads, submission failure
and retry, immutable originals, and worker completion. All 15 browser tests passed.
This does not replace tests of the full build recipe or live LLM-generated output.
