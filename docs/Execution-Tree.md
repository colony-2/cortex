# Job execution tree

Open the **DAG** tab in a job story or follow a DAG link from Goals. Generic jobs use `/project/<tenant>/jobs/<entry-job>/tree`. The opened job remains the discovery anchor when you select another card. **Find opened job** returns to it.

The diagram retains the opened job's recorded runtime family, including available ancestors and siblings. Starting at the opened job, Cortex also automatically follows runtime children and outgoing exact-job waits, then follows both kinds of relationship from every reached job. It does not climb a wait target's parents or discover reverse waits. Family context alone does not seed recursive wait discovery. Selecting a context-only job reads its direct waits for inspection without starting another recursive crawl.

Every readable job uses the same card and details: cell, title/prompt fallback, status, submission time, age or frozen time to finish, full identity and prompt, type, completion, timestamps, parent and recipe attribution, coverage and applicable actions. Original family cards use confirmed root-relative submission offsets; external cards use absolute UTC. Invalid timestamps remain unavailable.

Rounded cards and solid lines describe runtime parentage. Chamfered cards and **Wait target** badges identify jobs also reached through waits. Dashed arrows retain their conditions and use the same local side ports and column gaps as ordinary child edges. Shared targets have one card and all observed relationships. Self and return links use nearby detours; cycle boundaries do not establish scheduler deadlock. Goal waits remain literal diamond references with **Open goal**, without choosing an attempt.

The selected job's **Waits for** pane belongs to that job. It retains wait IDs, conditions, deadlines and blocked reasons, and lists incoming waiting jobs separately. `complete` means completion regardless of outcome; `success` requires successful completion. Unknown conditions remain literal with a warning. Legacy exact-job waits have an unavailable condition. **Back to waiting job** selects that exact consumer. **Open job DAG** creates a new scope anchored at the target; recipe execution links retain the original DAG return destination. Generic jobs retain their story limitation.

## Discovery limits and coverage

Discovery reads authorized retained records, including hidden and archived summaries. Recipe attribution, shared goals and layout do not create runtime parentage. Current pending waits cannot reconstruct resolved dependency history. Separate child and wait coverage identifies loading, exhaustion, failed pages, malformed responses and limits. Missing summaries retain their exact identities and incoming relationships with **Retry summary**. Other branches continue. A still-waiting job with no explanatory waits remains explicitly unexplained.

The view shares these bounds across family context, downward discovery and explicit inspection:

- 1,000 job/goal/reference identities and 4,000 grouped typed edges.
- 64 combined downward child/wait hops; family context retains 64 ancestor hops and 64 descendant levels.
- 200 attempted child/wait listing pages, including failures and retries.
- 10,000 inspected dependency entries in total; at most 10 wait pages and 1,000 entries per consumer, including invalid, resolved and legacy entries. Retained rows also consume storage capacity.
- 300 requests and 30 seconds per initial/resume/retry run, at most four simultaneous reads and one read per stream. Each request has a deadline of at most ten seconds and the remaining run time.

Page cursors advance automatically, including empty continuation pages. Failed pages retain their cursor for explicit retry. **Resume discovery** resets request/time allowances while preserving observations, cursors and hard counters. Hard limits stay labelled; opening a smaller scope does not bypass per-consumer bounds. The former 50-target overlay limit no longer applies. The pane renders 100 loaded rows initially; **Show more loaded targets** reveals more without fetching.

**Refresh waits** rechecks the selected consumer. Previous rows and relationships remain labelled until a complete valid replacement verifies removals. Failed or incomplete refreshes cannot establish that an old dependency disappeared. Verified removal prunes unsupported branches, including orphaned cycles, while preserving shared incoming paths and family context. **Refresh tree** starts a fresh observation and rechecks the anchored scope, retaining previous presentation while it is verified. Storage bounds still apply to retained data.

Search matches all loaded job content and reveals a known path, including waits. Collapse affects presentation only; a shared job can remain visible through another path. Zoom, Fit and scrolling include the loaded graph. Keyboard selection and stacked phone details provide the same actions. Hidden tabs pause reads; return resumes interrupted cursors once. There is no tree or dependency polling.

## Cancel a selected job

The detail pane offers **Cancel job**, **Goal overview** and **Open execution** when applicable. Icon actions have accessible names and tooltips, with larger touch targets on phones. Every unfinished recipe or generic job can be cancelled, including readable wait targets. Confirmation identifies the exact tenant and job and accepts an optional message. Submitted children remain independent; running work may take time to stop and performed actions remain.

After submission, Cortex rechecks the exact job and updates its card and details without resetting diagram controls. **Cancellation requested** remains pending until a fresh read confirms termination. **Refresh status** checks pending or unavailable outcomes. Newer cancellation/completion observations take precedence over older discovery summaries. Tree readability does not grant mutation access.

## APIs

The controller uses the existing caller-scoped `tree-node`, `tree-children?pageToken=…` and `tree-waits?pageToken=…` endpoints. Wait reads combine exact hidden-inclusive consumer summaries with pending `ListDependencyWaits` records and legacy `WaitFor` IDs. Supplementary failures and foreign/malformed records remain explicit. No server, JobDB or c2j API migration is required.
