# Feature Request: Workflow Outcome API

## Problem
Clients (CLI, UI, automations) need a single API call to retrieve the final outcome of a workflow run, including:
- Overall workflow status.
- A clear error message when failed/terminated/timed out.
- The latest (or selected) chapter output.
- The complete list of produced artifacts.

Today, clients must:
1. Call `/api/projects/{projectId}/workflows/{workflowId}` to get status, chapters, and artifacts.
2. Inspect chapter errors/outputs manually.
3. Call `/api/projects/{projectId}/workflows/{workflowId}/chapters/{chapterNumber}/artifacts/{artifactName}` to fetch artifacts one by one.

This forces client-side logic to infer failure vs. success and to stitch together outputs and artifacts. A single response should provide a consistent, canonical view.

## Proposal: `GET /api/projects/{projectId}/workflows/{workflowId}/outcome`
Returns a consolidated outcome document.

### Response shape (example, JSON)
```json
{
  "workflow_id": "w1",
  "run_id": "w1",
  "status": "failed",          // or completed, running, terminated, timed_out, canceled, unknown
  "chapter": 3,                // chapter chosen by the server (see selection rules)
  "output": { "foo": "bar" },  // present when available
  "error": "op xyz: timeout",  // present on failure/termination/timeouts/cancel
  "artifacts": [
    {
      "artifact_id": "a1",
      "artifact_type": "text",
      "name": "log.txt",
      "created_at": "2025-01-01T00:00:00Z",
      "size_bytes": 1024,
      "url": "https://..."
    }
  ]
}
```

### Behavior
- **Chapter selection**: server picks the last chapter that has either `output` or `error`. Accept optional query `chapter=N` to override.
- **Status semantics**: mirrors workflow status; `error` is populated when status ∈ {failed, terminated, timed_out, canceled, unknown}.
- **Artifacts**: aggregated across all chapters; include metadata and direct `url` when available.
- **Backward compatibility**: non-breaking; new endpoint only. Existing endpoints remain untouched.

### Benefits
- Single round-trip for outcome + artifacts metadata.
- Consistent failure reporting across clients.
- Simplifies CLI/UI logic; reduces chances of divergence in failure detection.

### Optional extensions
- `select=latest_output|latest_error|chapter:N` to control chapter resolution.
- `include=artifacts=false` to skip artifact aggregation when not needed.
- Streaming mode for large outputs is out of scope for now.

## Acceptance criteria
- Endpoint implemented and documented in OpenAPI (server/openapi).
- Integration tests cover success, failure, timeout, and canceled workflows.
- Artifacts list matches what is currently returned in workflow detail chapters.
- Returned `error` is human-readable and derived from chapter or workflow-level failure information.
