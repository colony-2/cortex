# Workflow Story UI (Job Run Story) — UI Spec

## Summary

We have a new API that returns a hierarchical, recipe-centric “story” of a job execution:

- `GET /api/projects/{projectId}/jobs/{jobId}/story`

This spec proposes a new UI that renders that story as an expandable outline/tree, supports drilling into nodes for details (input/output, timing, retries), and preserves the existing artifact view/download experience from the current Workflow Detail UI.

Key navigation changes:

- Workflow List links go to the new Story UI by default.
- The Story UI links back to the old Workflow Detail UI.
- The old Workflow Detail UI links to the new Story UI (symmetry).

Reference: `/src/server/workflow/docs/job_run_story_consumer_guide.md`.

---

## Goals

- Render the story tree in a way that matches how recipe authors think:
  - `recipe → sequences → ops (with retries) → op steps (only for multi-step ops)`
  - `state machines → states → transition evaluation decisions`
- Allow users to expand/collapse nodes and drill down into any node’s details:
  - status, timing, breadcrumbs (`path`), `invoke_seq`, input/output payloads
  - retry history (`attempt` + `prior_attempts`)
  - artifacts attached to that node (`artifact_keys`)
- Keep artifact interactions consistent with the current Workflow Detail UI:
  - list artifacts, show size, click to view (modal), allow download

## Non-goals (v1)

- Replacing the old chapter-based Workflow Detail UI.
- Real-time streaming (SSE/WebSockets). Polling is sufficient.
- Editing/rerunning workflows from this UI.

---

## Terminology / Data Model

### Top-level response fields (header/summary)

- `job_id` (== `workflow_id` in UI routing; confirmed invariant)
- `invocation_sequence`
- `recipe` (metadata from job-start artifact)
- `status` (job-level): `running|completed|failed|canceled|terminated|timed_out|unknown`
- `started_at`, `finished_at`
- `root` (story node; `kind=recipe` when present)

### Story node shape (tree)

Every node includes:

- `kind`: `recipe|sequence|op|opStep|stateMachine|state|transitionEval`
- `title`: display label
- `status` (node-level): `pending|running|succeeded|failed|canceled|skipped|unknown`
- `started_at`, `finished_at` (nullable)
- `path`: `string[]` breadcrumbs from root; stable-ish key for UI state
- `invoke_seq`: recipe invocation sequence for correlation within a run
- `input`, `output` (nullable payloads)
- `artifact_keys`: list of produced/attached artifacts (may be empty)
- `children`: nested nodes in chronological order

Retries:

- Inline node represents the **latest attempt**
- `attempt`: attempt number for latest attempt
- `prior_attempts`: array of previous attempts, same node shape

State machine transitions:

- `transitionEval` contains:
  - `evaluations`: ordered list `{expression, result, to_state_id}`
  - `decision`: `{kind:"state", to_state_id:"..."}` or `{kind:"fallthrough"}`

Artifacts:

- Nodes expose `artifact_keys[]` in this shape:
  - `{ jobId, taskOrdinal, name, sizeBytes }`
- Bytes fetched separately via existing endpoint:
  - `GET /api/projects/{projectId}/jobs/{jobId}/tasks/{taskOrdinal}/artifacts/{artifactName}`

---

## Routes / Navigation

### New: Story UI

Route:

- `/project/:projectId/workflows/:workflowId/story`

Data lookup:

- `jobId = workflowId` for calling the story endpoint.

### Existing: Old workflow detail UI

- `/project/:projectId/workflows/:workflowId`

### Required links

- Workflow List → Story UI (default click)
- Workflow List → Old UI (secondary action/link)
- Story UI → Old UI (prominent “Open old workflow detail” link/button)
- Old UI → Story UI (prominent “Open story view” link/button)
- Both detail pages should also have “Back to Workflows” to `/project/:projectId/workflows`

---

## Page Layout (Story UI)

### Header row

Left:

- “Back to Workflows” button/link
- Title: `Workflow Story`
- Compact ID display (e.g., last 12 chars) + copy full `job_id`

Right:

- “Open old workflow detail” button/link
- Refresh button
- If `status=running`:
  - Auto-refresh toggle + interval (default ~3–5s)
  - “Live” indicator when polling enabled

### Summary card (top)

Show:

- Status tag (job-level `status`)
- Recipe name/identifier (from `recipe` metadata)
- Started / Finished timestamps (and duration when both are present)
- `invocation_sequence`
- Full `job_id` (copyable)

### Main content: Split view

Two-pane layout (resizable if easy; fixed ratio otherwise):

1. Left pane — Story outline/tree
2. Right pane — Node details (for selected node)

---

## Story Outline / Tree (Left Pane)

### Rendering rules

- Render nodes in the order received; treat `children` as chronological.
- Display each node as a single row with indentation and a disclosure caret.
- Caret shown when:
  - `children.length > 0`, OR
  - `prior_attempts.length > 0` (retry disclosure)

### Row content

Each node row displays:

- Kind icon (distinct per `kind`)
- `title`
- Status tag (node-level `status`)
- Optional badges:
  - Attempt badge when `attempt > 1` or `prior_attempts.length > 0` (e.g., `Attempt 3`)
  - Artifact count badge when `artifact_keys.length > 0`
  - Duration (computed from `started_at`/`finished_at`) when available

### Status color mapping (recommended)

Node status (`pending|running|succeeded|failed|canceled|skipped|unknown`):

- `running` → blue
- `succeeded` → green
- `failed` → red
- `canceled` → default/gray
- `skipped` → gold/orange
- `pending` → default
- `unknown` → default

Job status (`running|completed|failed|canceled|terminated|timed_out|unknown`):

- `completed` → green
- `timed_out` → orange
- `terminated` → default/gray
- Others as above where applicable

### Retries UI

Latest attempt is displayed inline (the node itself).

If `prior_attempts` exists:

- Provide a “Prior attempts (N)” disclosure beneath/within the node.
- Prior attempts list rows show:
  - Attempt number, status, start/finish, duration
- Selecting a prior attempt should populate the Details pane for that attempt (without changing the main tree selection unless the user explicitly “switches”).

### Selection & drill-down

- Clicking a node selects it and updates the Details pane.
- Keep expansion state stable on refresh by keying state off `path.join('/')`.

### Deep linking (recommended)

Support selecting a node via URL state:

- Query param: `?path=...` (encode `path.join('/')` or JSON-encoded array)
- On load, auto-expand ancestors and select that node if present.
- “Copy link to node” action in Details pane sets this param.

---

## Node Details (Right Pane)

### Tabs (recommended)

1. **Overview**
   - Kind, title, status, attempt
   - Started/finished/duration
   - `invoke_seq`
   - Breadcrumbs using `path[]` + copy

2. **Input**
   - JSON viewer (collapsed by default; clipboard enabled)
   - Empty state when `input` is null

3. **Output**
   - JSON viewer as above
   - Empty state when `output` is null

4. **Artifacts**
   - List/table of `artifact_keys`
     - Columns: Name, Size, Task ordinal, Actions
   - Actions:
     - **View** (opens artifact modal)
     - **Download**

5. **Retries** (only when `prior_attempts.length > 0`)
   - Prior attempts list + quick compare metadata

### Specialized rendering: `transitionEval`

When `kind=transitionEval`, Overview adds a “Transition” section:

- Render `evaluations[]` as a table:
  - expression, result, to_state_id
- Highlight `decision`:
  - Taken transition (to_state_id), or fallthrough

---

## Artifacts UX (parity with old UI)

### Listing artifacts

- Show artifact name and size (`sizeBytes`).
- Include task ordinal (`taskOrdinal`) for traceability.

### Fetch/view/download behavior

Construct the artifact URL from the artifact key + existing endpoint:

- `/api/projects/{projectId}/jobs/{jobId}/tasks/{taskOrdinal}/artifacts/{artifactName}`

Modal viewer behavior mirrors current Workflow Detail:

- Title: artifact name
- View modes: Text / Hex
- Download button
- Error state: show a readable message if fetch fails

Dev/prod URL handling should remain consistent with existing patterns (dev uses API server base, prod uses relative `/api`).

---

## Running Jobs (Partial Stories + Polling)

If top-level `status=running`:

- Allow polling the story endpoint.
- Preserve:
  - Expanded/collapsed nodes (keyed by `path`)
  - Selected node (re-select by `path` after refresh)
  - Selected “prior attempt” view (if applicable)

UX notes:

- Show “Last updated” timestamp.
- If the selected node disappears (rare), keep the Details pane but show a warning and offer to reselect the nearest ancestor.

---

## Error / Empty States

- Story endpoint fails:
  - Show error alert with retry.
- `root` is null/absent:
  - Show “No story available yet” (common while starting) with refresh.
- Large payloads:
  - JSON tabs default collapsed; provide “Expand all” only if safe.

---

## Accessibility & Usability

- Keyboard navigation:
  - Up/down to move selection in the outline
  - Left/right to collapse/expand
- Announce status changes in live mode (polite aria live region).
- Ensure selection contrast and focus outlines are visible.

---

## Performance Considerations

- For large stories:
  - Prefer a virtualized outline list if available in the chosen component.
  - Avoid rendering full JSON payloads until the corresponding tab is opened.
- Memoize computed tree nodes and durations.

---

## Workflow List Page Updates

- Default link target:
  - Workflow ID click navigates to `/project/${projectId}/workflows/${workflowId}/story`
- Add a secondary “Old view” action/link per row:
  - `/project/${projectId}/workflows/${workflowId}`

---

## Old Workflow Detail Page Update (Symmetric Link)

Add a prominent link/button near the header actions:

- “Open story view” → `/project/${projectId}/workflows/${workflowId}/story`

---

## Acceptance Criteria

- Users can expand/collapse nodes and observe the correct hierarchical story order.
- Selecting any node shows its Overview + input/output (when present).
- Retried nodes show attempt metadata and prior attempts via disclosure.
- Artifacts are listed at the node level; view/download works and matches old UI affordances.
- Workflow List defaults to new Story UI and still provides access to old UI.
- Both detail UIs link to each other.
- Running workflows can be polled without losing UI context (expansions + selection).

---

## Open Questions

- Should the Story UI also include a compact “search/filter” (by title/kind/status) for very large stories?
- Do we want to expose “focus on subtree” mode in v1, or hold for v1.1?
- Should we persist auto-refresh preference per user (localStorage)?

