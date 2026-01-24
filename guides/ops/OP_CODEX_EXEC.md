# codex.exec Op

Runs Codex CLI non-interactively and returns normalized status/summary; stdout/stderr are surfaced as output artifacts (`stdout.jsonl`, `stderr.txt`).

## Input Structure

```json
{
  "prompt": "Summarize the changes in this repo.",
  "sessionId": "optional-session-id",
  "model": "gpt-5-codex",
  "env": {
    "CODEX_API_KEY": "optional"
  },
  "worktree_path": "/path/to/repo",
  "cell_relative_path": "cells/api"
}
```

## Output Structure

```json
{
  "status": "completed",
  "sessionId": "session-id",
  "assistantSummary": "Short summary of work performed.",
  "incompleteReason": "",
  "incompleteCategory": "",
  "pendingDependencies": [
    {
      "component": "deps/service-a",
      "requestedChanges": "Update contract to v2."
    }
  ]
}
```
