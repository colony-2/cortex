# Recipe-defined reviews with continuous discussion

Cortex renders `form.presentation.type: review/v2` using the existing structured input operator. Each direction explicitly declares its feedback rule and whether the reviewer should stay for another turn. Recipes supply every setting ID, control, label, value, applicability rule and effect. Cortex has no model catalog or stage-routing policy.

Ordinary inputs and `kind: review` remain supported. A new producer requires a supporting Cortex client and retained input-history/artifact access. Deploy and verify the client before activating producers. Unknown presentations and incompatible declarations disable submission with an explanation.

## Request

Leave native `kind` unset. Supply `title`, `presentation: {type: review/v2}`, `request`, `request_schema` and `response_schema`. Bundle schemas locally using draft 2020-12; native validation remains authoritative. Structured requests cannot mix ordinary fields or native review document metadata at the form's top level.

The request contains:

| Property | Meaning |
| --- | --- |
| `review_id` | Recipe-owned logical identity, persisted once at review entry. Questions keep it; revised work gets a new identity. |
| `title`, `question` | Review title and instructions. |
| `documents` | Original stored artifact references keyed by document ID. Preserve their producer coordinates. |
| `document_metadata`, `overview_document_id` | Optional document titles, roles, ordering, descriptions, sources and initial selection. |
| `directions` | Ordered `{id, label, feedback_mode, allow_attachments, continues_review}` declarations. Both flags are required booleans; the mode is `forbidden`, `optional` or `required`. |
| `feedback` | `{label}` for new declarations. Each direction owns its upload permission. |
| `settings` | Optional ordered declaration array. No settings means no gear. |
| `conversation` | Ordered recipe-owned human and agent turns. Initially empty. |

IDs must be nonempty and unique within their collection. Overview, metadata and sources must name attached documents. Metadata keeps the existing 120-character title, 300-character description, integer order through 1000000, 128-source and 65536-byte presentation bounds. ASCII controls and self/duplicate sources are invalid.

Meaningful feedback contains a character outside Unicode White_Space plus U+FEFF: U+0009–000D, U+0020, U+0085, U+00A0, U+1680, U+2000–200A, U+2028–2029, U+202F, U+205F, U+3000 and U+FEFF. Use the same explicit set in generated response validation and recipe routing. Preserve submitted text exactly. Settings never satisfy feedback requirements.

Forbidden feedback must be exactly empty: even whitespace is rejected. A forbidden direction must disallow attachments. Optional feedback preserves any text, including blanks; required feedback needs meaningful text or a permitted new upload. Previous conversation and reviewed originals are independent of the new contribution.

For example, a recipe can declare clean approval as forbidden/false, Request changes and Review doc comments as required/true, and Ask a question as required/true with `continues_review: true`. Cortex follows declarations regardless of the IDs and labels. The recipe decides whether document comments are applicable, omits that direction when no originals exist, and revises review documents with fresh identity and approval after accepting comments. It can explain the omission in `question`: “There are no attached review documents to revise. Use Request changes to request changes.” Cortex neither infers applicability nor performs revisions.

Frozen earlier `review/v2` forms remain supported: every direction has boolean `feedback_required` and `continues_review`, and `feedback.allow_attachments` is boolean. True means required, false means optional, and the global permission applies to every direction. New forms omit both old properties entirely. Reject mixed collections, partial rules, both encodings together, null flags, unknown modes and forbidden-with-uploads. Property presence matters, even when null or false. The parser and server interpret frozen rules without rewriting retained forms or schemas; historical approvals with notes remain readable.

Selecting a direction forbidding pending text or files preserves those drafts, explains the conflict and blocks confirmation. The reviewer must remove them explicitly or return to a permitted direction; Cortex never silently filters an approval. Direction changes invalidate confirmation and revalidate applicable settings. Unsent drafts remain local to the mounted logical review.

## Settings

Every setting declares `id`, `control`, `label`, `value`, `applies_to`, `schema` and readable `effect`. `description` is optional. Reset requires both an explicit `default` and `reset_label`; the default is an ordinary declared value, including null only when allowed.

Supported controls are `text`, `choice`, `boolean` and `number`. A choice supplies ordered `{value, label}` pairs and preserves scalar JSON types. Choice values must be distinct; numeric 1 and 1.0 are equal, while false and zero differ. Current/default values must be offered choices, except a declared null reset default with a reset label.

The bounded scalar schema vocabulary is `type`, `enum`, `minLength`, `maxLength`, `pattern`, `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`. Declare one non-null scalar type, optionally null. `integer` is supported by the number control. Values and defaults must satisfy the schema; contradictory bounds and unsupported keywords are rejected. String length counts Unicode code points.

Patterns use Unicode search semantics and a portable grammar checked before compilation. It permits literals, scalar classes/ranges, concatenation, nonempty alternatives/groups, one leading `^`, and greedy repeats on literals/classes with canonical decimal bounds through 1000 (no leading zeros). It rejects dot/end anchors, shorthand/property/POSIX classes, regex Unicode/hex escapes, lookaround, backreferences, special groups, lazy/possessive repeats and quantified groups. JSON decoding precedes grammar checking. Use the producer/consumer fixtures when authoring patterns.

Opening, editing or closing the gear sends nothing. A direction submission includes only changed applicable setting IDs. Inapplicable drafts stay local; invalid applicable drafts block confirmation. A successor revalidates remaining drafts and flags removed/invalidated declarations. Drafts do not cross logical review identities or persist across leaving/reloading the page.

Recipes apply the submitted values and implement their documented effects. Cortex does not execute effect text or interpret names such as model, reasoning effort, or return stage.

## Contribution and discussion

The browser posts multipart to `/api/projects/:tenant/reviews/:job/respond` with one `submission` JSON part and repeated `attachments` file parts in contribution order:

```json
{
  "request_id": "the-current-native-request",
  "submission_id": "correlation-id",
  "response": {
    "direction": "question",
    "feedback": "What survives a restart?",
    "attachments": [],
    "settings": {"response_style": "brief"}
  }
}
```

The server supplies actor identity, creates distinct upload bindings, and calls native `SubmitStructuredResponse`. It returns the accepted output and unmodified receipt. Client artifact keys, receipts, actors, replacement conversations, duplicate submission parts and other file-part names are rejected. The 32 MiB limit covers the complete multipart envelope. Original filenames remain understandable, including when two uploads share a filename.

Generate one closed response-schema branch per direction, requiring all four contribution fields. Forbidden feedback has `maxLength: 0`; required feedback needs the shared nonblank pattern or an allowed resolved attachment. Settings are sparse, typed and limited to that direction's declared IDs. Disallowed attachments have `maxItems: 0`. Omission means unchanged; no schema defaults fill missing responses. The authenticated adapter checks actual multipart file parts, including empty files, before creating upload bindings. Native schema validation independently checks resolved contributions before accepting them.

After accepting a question, the recipe appends a human turn with `role`, `direction`, exact `text`, resolved `attachments`, optional accepted `settings`, and native `receipt`. It then appends an agent turn with `role: agent`, actual answer `text` and `attachments`. The next input carries the authoritative full conversation and frozen original documents, with the same logical review ID and a fresh native request ID. A question does not itself authorize corrections or acceptance.

Cortex keeps the logical review in `review_id` and the selected original attachment in `document_id`. Document changes add browser history; Back and Forward preserve mounted feedback, files, settings, and composer state. Plain links and removed-document fallbacks replace the current entry. Only a matching current native occurrence permits submission.

The page head shows the current cell and recorded runtime ancestor branch, using titles or prompt snippets. The overview or primary document opens inline. Supporting documents appear as attachments below the preview. Open a linked document or attachment to read it in a scrollable dialog. The primary document stays open beneath the dialog. The gear menu contains document details, display options and Download.

The selected request’s question appears last in Discussion as an ordinary Agent message. Markdown headings, lists, emphasis, links, tables, fenced code and authored prose line breaks retain their formatting. Earlier recorded turns keep their original presentation. Raw HTML is skipped; unbound images and relative or unsupported links show readable fallback text. Public HTTP(S) and mail links open in a new tab.

On wide screens, Discussion sits beside the response. Drag the divider to resize Discussion, or focus the separator and use Left/Right (16px), Home (minimum) or End (maximum). Hide or show Discussion with the toolbar button. Narrow screens stack the response and Discussion. Both sections scroll with the page. A review without documents shows Discussion and any available reply controls.

The **Changes** attachment opens the latest saved main-workspace diff in a dialog. Each read shows the cumulative comparison from the recorded active base to its head. Unified, Side by side, Source and file navigation share that saved file. Download saved diff opens its exact original artifact. Refresh changes replaces the preview and download together. Missing saved comparisons and empty comparisons have distinct messages, and shortened previews retain the full original download. Binary notices explain omitted contents.

Document and discussion controls preserve their state while you open Changes. The screen shows the latest comparison. API and CLI callers can select recorded task or scope occurrences, attempts, workspaces and before/after boundaries. Changes describes saved code independently of earlier checks and approvals. See [saved changes API and migration](README.md#reviewing-saved-changes).

Discussion starts visible. Its visibility and preferred width last while this logical review is mounted, including successor requests. A smaller window temporarily limits the width. Widening the window restores the preference. Leaving the review or reloading restores the default layout.

Response choices sit below the review. Every choice opens an inline composer with a response-type menu and an explicit Send button, including approvals without feedback. Back retains drafts. Incompatible drafts stay visible and block submission until you remove them or choose a compatible action. Settings edits remain visible with their applicability. Frozen legacy optional actions still open a composer.

An accepted continuing contribution shows “Response pending” after the unanswered human turn in the discussion. An actual successor answer replaces it. Sending, unavailable state, accepted exits, cancellation, and terminal status remain distinct. Another logical review remains separate. Read failures preserve saved material and disable uncertain actions.

A successful response is only acceptance of that contribution. It does not prove publication or job success. An ambiguous submission triggers reconciliation, never an automatic repost. Submission IDs provide correlation, not an exactly-once guarantee.

Originals and earlier notes resolve through selected retained manifest membership, including cross-job producers; arbitrary artifact keys or URLs cannot become review documents.

## Local verification

`web/shared/src/reviewV2.test.ts` exercises declaration and pattern validation. App reducer/hook/component tests cover drafts, history, snapshot expiry, presentation dispatch, long threads and frozen labels. Go API tests cross native structured submission and SQLite artifact binding, including near-limit uploads. The `review/v2` cases in `web/app/tests/recipe-input.spec.ts` use a real local HTTP JobDB, production Cortex, the pinned worker, and actual uploads.

The current browser fixture is a self-contained contract fixture. It includes clean approval, required change requests and a recipe-declared document-comments direction, plus generic settings and multiple questions. Native tests cover frozen compatibility, strict declaration rejection, forbidden multipart contributions and direct native schema submission without browser checks. Producer-generated fixture parity and adaptive3 policy/effect checks belong to the Recipes delivery and must be verified before full feature acceptance.

Ship and verify compatible Cortex support first. Only then resume the Recipes migration and enable its default producers, after checking the deployed retained-history/artifact backend. Keep existing producers unchanged until that point. Local contract-fixture checks establish consumer readiness, not deployed compatibility or document revision behavior in adaptive3.
