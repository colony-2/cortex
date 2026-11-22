# Cortex TODO

## Review and chapter reads

- [x] Read each selected review document by producer job, ordinal, and artifact name. JobDB handles the read and the preview displays errors. Review submission still validates the pending request. The remote transport test checks for exactly one upstream GET on success, denial, and a missing artifact.
- [x] Open a current review from its waiting task. Open a recorded review from the indexed question and acceptance chapters. Keep the existing review and document URLs working.
- [x] Audit the review, Goals, Conversation, and Trace paths for chapter reads, including reads inside GetJobRun. Keep full history reads on demand. Reuse the active Conversation or Trace scan for the job input inventory. Conversation also projects terminal returned-document metadata from its scan. Trace gets its Inbox and Outbox lists from the selected node in the story.
- [ ] Eliminate discovery on the first open of an unindexed archived review. JobDB currently exposes the active waiting task and chapter ordinals, but an archived review UUID carries no ordinal. Cortex indexes recorded review locations while reading Conversation or resolving a saved review link. A missing or stale index, or a legacy record that cannot be decoded at its ordinal, falls back to native history. An operator can prime a migrated job by requesting `GET /api/projects/{tenant}/jobs/{job}/conversation?page_size=1&include_sections=false` once. A write-time review-to-chapter index and backfill in the producer would remove first-open discovery.

## Input artifacts

- [x] Add an optional ordinal to the input-artifacts and input-document APIs and use it when the input chapter is known. A standalone job inventory reads the input prefix, including grouped contributions. A stored document can be opened by its producer key.
- [x] Limit Markdown image resolution in a goal prompt to chapter 0 of the job that supplied the prompt. On the Conversation tab, reuse the chapter-0 inventory already obtained with history.

## Trace details

- [x] Fix the refresh flicker. Keep the selected Inbox list, count, and preview mounted during a background story read. Clear the preview when its binding disappears or its source changes, or when the reader switches jobs, nodes, or attempts.
