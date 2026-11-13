# Reviews for recipe authors

A review is an `input` form with `kind: review`, questions, and optional documents.
Documents use existing stored artifact references. Answers use ordinary form
fields; reviewers can return documents through optional file-upload questions.

No extension, preparation recipe, hashes, or custom JSON Schema is needed.
c2j prepares the form and waits for the response using the input op's internal
substeps. Applications identify reviews through `form.kind` and find their
original documents in `form.documents`.

## Add a review

```yaml
- id: review
  op: input
  inputs:
    form:
      kind: review
      title: Review the design
      documents:
        design: '${{ sequence.draft.artifacts["design.md"] }}'
      fields:
        - id: decision
          type: multiple_choice
          question: How should we proceed?
          required: true
          options:
            - {value: approve, label: Approve}
            - {value: revise, label: Request changes}
        - id: feedback
          type: paragraph_text
          question: What should change?
        - id: annotated_design
          type: file_upload
          question: Optionally attach an annotated design
```

`design` is the review's document ID; `design.md` is the producer's artifact name.
Use `${{ ... }}` to pass artifact references intact. References from child jobs
work too. Do not pass filesystem paths, external URLs, or inline document bodies.
Submitted files can be referenced through `context.artifacts["design.md"]`.

See [the complete example](examples/review/review.yaml). With that file saved as
`review.yaml`, submit it using your configured runtime:

```bash
c2j submit "Review this design" \
  --advanced-recipe-file ./review.yaml \
  --artifact design.md=./design.md
```

Run workers as usual. An application using the c2j input library displays the
review and submits the response. The ordinary interactive CLI prompt does not
render document reviews; `--input-mode ops` exposes the pending form to a client.

## Use answers and returned documents

Read answers from `sequence.review.outputs.fields`:

```yaml
outputs:
  decision: "${{ sequence.review.outputs.fields.decision }}"
  feedback: "${{ sequence.review.outputs.fields.feedback }}"
  receipt: "${{ sequence.review.outputs.receipt }}"
  artifact_refs: "${{ sequence.review.outputs.artifact_refs }}"
```

An answered file-upload question contains a stored artifact reference. On a
branch where `annotated_design` was supplied, bind it into another op's inbox:

```yaml
artifacts:
  annotated-design.md: "${{ states.review.outputs.fields.annotated_design }}"
```

An optional file question can be absent. Before entering that branch, check:

```yaml
when: 'jq(states.review.outputs.fields, ".annotated_design // null") != null'
```

The response's `artifact_refs` map also exposes returned documents by attachment
name. It does not include the original review documents. Original references
remain recorded on the pending form and its persisted preparation outcome.

For a review state, route using ordinary transitions:

```yaml
transitions:
  - to: verify
    when: 'states.review.outputs.fields.decision == "approve"'
  - to: revise
    when: 'states.review.outputs.fields.decision == "revise"'
```

Define those destination states in your state machine. The decision names have
no built-in workflow meaning: approving does not merge, and requesting changes
does not automatically run another op. If an agent revises the documents, pass
its next output artifacts into a new review invocation.

Returned files can be any document format; c2j does not interpret or apply edits.
To continue an agent session, explicitly route its session checkpoint using that
extension's supported input alongside feedback and document attachments.

## Test without waiting for a reviewer

Use ordinary autofill on a test review:

```yaml
form:
  kind: review
  title: Review the proposal
  fields:
    - id: decision
      type: multiple_choice
      question: Proceed?
      required: true
      options:
        - {value: approve, label: Approve}
        - {value: revise, label: Revise}
  autofill:
    fields:
      decision: approve
```

Autofill uses the same review acceptance path and records an automation actor.
File answers must reference real stored artifacts. See the
[review fixture](pkg/input/test-fixtures/recipes/review-input.yaml) and
[integration test](pkg/input/test-fixtures/review_test.go).

Test your answer branches, missing required answers, returned documents, and
text-only responses. Keep production human reviews free of autofill.

## Library integration and limits

Use `GetForm` or `GetDetails` to discover the marker, questions, and documents.
Use `SubmitFormResponse` with the form's `request_id`, a submission ID, answers,
and an actor supplied by the application. The
[application guide](GUIDE-Structured-Input-And-Review.md) includes a Go example.

The request ID identifies this occurrence of the review. It survives replay;
a later review receives a different ID. Invalid answers, stale request IDs, and
unavailable response attachments leave the review pending. Receipts identify the
accepted request, submission, actor, and time.

The application provides presentation and authentication. Submission IDs are
correlation identifiers, not an idempotency guarantee. Historical receipt lookup
is not implemented, and the separate
[JobDB completion investigation](JOBDB_EXTERNAL_TASK_COMPLETION_DISCUSSION.md)
remains deferred.

## Describe attached documents

Presentation metadata is optional and sits alongside `documents`; keys and source
references are document IDs in that same map:

```yaml
kind: review
overview_document_id: design
documents:
  design: '${{ sequence.draft.artifacts["design.md"] }}'
  checks: '${{ sequence.draft.artifacts["checks.txt"] }}'
document_metadata:
  design:
    title: Proposed design
    description: Read the proposed behavior and tradeoffs first.
    role: primary
    order: 0
    source_document_ids: [checks]
  checks:
    title: Validation evidence
    description: Supporting check output.
    role: diagnostic
    order: 1
```

Cortex selects the overview initially, falling back to the first document ordered
by ascending `order`, with absent orders last and document IDs breaking ties.
The mobile reader starts with descriptive cards and opens when a file is chosen.
Titles, descriptions, and Primary/Supporting/Diagnostic roles explain each file;
filenames remain visible. Source links open attached files without recursively
expanding their sources. Metadata grants no additional artifact access and does
not alter file contents or download names. Omitted metadata retains filename-based
labels and document-ID ordering. Zero is a valid order, distinct from absence.

## Confirming a response in Cortex

For reviews with one required multiple-choice question, selecting a response
expands the fixed bottom bar. Selection sends nothing; OK validates and submits.
Back keeps the draft and restores focus to the selected response button.

A declared paragraph question with ID `feedback` appears once as the Why? editor
in that bar, with its original question. This applies to custom option values as
well as approve/revise. Required flags and validation patterns still apply;
the conventional Reject (raw value `revise`) also requires a nonblank reason.
Approve permits blank optional feedback. Reviews without that field confirm
without adding an answer. Other questions remain in the page.

This replaces immediate response-button submission and the rejection-only
Submit rejection control. Stored option values, field IDs, attachments, and
review API contracts are unchanged.

Paragraph editors show a live character count without a fixed character cap.
The former 1,000-character ceiling was hard-coded in Cortex, independent of
recipe properties. Cortex keeps its existing required and pattern checks.
The pinned c2j library transports ordinary-field `validation.min_length`,
`max_length`, and `pattern` settings but does not enforce them when accepting
review answers. Adding recipe text-length enforcement is separate work.

Feedback still shares backend size budgets with other data. Cortex limits the
whole review HTTP request, including attachments and multipart overhead, to
32 MiB. JobDB defaults to 256 KiB of application payload per persisted chapter,
including encoded answers and retained invocation data. That configurable byte
budget is not a per-field recipe limit or a universal character count. Exceeding
it can fail the job; text is not automatically truncated or stored as an artifact.
Later recipe steps that copy feedback also consume chapter payload space.
