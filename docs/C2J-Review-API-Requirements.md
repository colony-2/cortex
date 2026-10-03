# c2j APIs for Cortex reviews

Cortex now pins c2j `v0.0.56-0.20260930003717-614bfac82f15` (the upstream HEAD
inspected on 2026-09-30). Its `input.Runtime` already provides `GetForm`,
`GetDetails`, and `SubmitFormResponse`. These cover immutable document reviews
without duplicating request identity, answer validation, attachment binding,
receipt creation, or workflow completion. No blocking c2j changes are needed for
this first UI.

The following library additions would remove the remaining adapters and improve
scalability. They are requirements for follow-up c2j work, not APIs Cortex assumes
already exist.

## Pending input summaries

Extend discovery with a paginated summary method returning:

- Job ID, form kind, title (or single question), request ID, and pending task ordinal.
- Document count, and the time this input occurrence became pending.
- An optional kind filter and a continuation token; preserve ordinary input callers.

Today `ListPendingInputs` returns only job IDs and caps its query at 1,000 jobs.
Cortex must call `GetForm` for each job to classify reviews and populate the two
navigation lists. Move this into c2j and avoid loading every full document/form
merely to produce summaries. Discovery must handle an input completing between
listing and reading without failing the whole list. Successive review occurrences
in the same job must carry different identities. Do not collapse them by job ID.

## Document access scoped to a review occurrence

Provide a library method taking tenant ID, job ID, request ID, and document ID,
returning a stored artifact handle with name and size. Resolve the reference from
the frozen form, preserve the original producer job/task ordinal, and enforce the
current tenant. Return typed errors for stale requests, missing documents, and
unavailable bytes. Do not accept filesystem paths or arbitrary URLs.

Cortex currently implements this small adapter using `GetForm` and
`Engine.GetArtifact`. A library method would let other clients reuse the same
occurrence and document-membership checks. Streaming is preferable to embedding
bodies in form JSON. Originals remain immutable.

## Typed submission errors

`SubmitFormResponse` should distinguish validation errors (including field ID),
stale request IDs, no-longer-pending inputs, unavailable attachments, and storage
failures. Cortex can then consistently return 422, 409, 404, or 5xx and provide
useful recovery actions. Currently many review rejections are plain formatted
errors; the adapter treats them as bad requests.

Continue accepting `jobdb.Artifact` values in `FormSubmission`: Cortex's HTTP
multipart adapter passes real bytes to that API, so c2j remains responsible for
persisting and validating returned documents. The actor must continue to be a
separate application-supplied parameter, never trusted from the submission body.
Cortex currently supplies its existing email-login identity; that login is a
self-asserted identity, not a new verified authentication mechanism.

## Scope and acceptance

This UI lists pending reviews only. Historical review/receipt lookup is deferred
in c2j and is not emulated by reconstructing task history in Cortex. If completed
reviews become a product requirement, expose an occurrence-based history API
with the frozen form, original documents, accepted answers, and receipt.

Validate additions with mixed ordinary/review forms, empty and multiple-document
reviews, child-producer references, tenant isolation, sequential reviews in one
job, concurrent completion, pagination, stale submissions, and unavailable
artifacts. Keep legacy input submission unable to bypass review validation.
