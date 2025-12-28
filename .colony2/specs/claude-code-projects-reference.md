# Claude Code Projects Directory Reference

## Overview

The `~/.claude/projects` directory serves as the persistent storage location for Claude Code conversation history. This directory maintains a complete record of all interactions between users and Claude Code across different project workspaces.

## Directory Structure

```
~/.claude/projects/
├── -path-to-project-directory-1/
│   ├── session-uuid-1.jsonl
│   ├── session-uuid-2.jsonl
│   └── session-uuid-3.jsonl
├── -path-to-project-directory-2/
│   ├── session-uuid-4.jsonl
│   └── session-uuid-5.jsonl
└── -path-to-project-directory-3/
    └── session-uuid-6.jsonl
```

### Naming Conventions

- **Project Directories**: System paths are encoded with dashes replacing forward slashes
  - Example: `/home/user/projects/myapp` becomes `-home-user-projects-myapp/`
- **Session Files**: UUID-based identifiers with `.jsonl` extension
  - Format: `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx.jsonl`

## File Format

Each session file uses **JSON Lines format** (JSONL), where each line contains a valid JSON object representing a single interaction or metadata entry.

### Entry Types

#### 1. Summary Entry
The first line typically contains session metadata:
```json
{
  "type": "summary",
  "summary": "Brief description of the session's main activity",
  "leafUuid": "unique-identifier-for-the-last-message"
}
```

#### 2. User Message Entry
```json
{
  "type": "user",
  "uuid": "unique-message-id",
  "parentUuid": "previous-message-id or null",
  "sessionId": "session-identifier",
  "timestamp": "ISO-8601-timestamp",
  "version": "claude-code-version",
  "cwd": "working-directory-path",
  "message": {
    "role": "user",
    "content": "user input text or content array"
  }
}
```

#### 3. Assistant Message Entry
```json
{
  "type": "assistant",
  "uuid": "unique-message-id",
  "parentUuid": "previous-message-id",
  "sessionId": "session-identifier",
  "timestamp": "ISO-8601-timestamp",
  "version": "claude-code-version",
  "message": {
    "id": "message-id",
    "type": "message",
    "role": "assistant",
    "model": "model-identifier",
    "content": ["response content"],
    "stop_reason": "completion-reason",
    "usage": {
      "input_tokens": "number",
      "output_tokens": "number",
      "cache_creation_input_tokens": "number",
      "cache_read_input_tokens": "number"
    }
  },
  "requestId": "api-request-id"
}
```

#### 4. Tool Use Results
```json
{
  "type": "user",
  "uuid": "unique-message-id",
  "parentUuid": "tool-use-message-id",
  "message": {
    "role": "user",
    "content": [{
      "tool_use_id": "tool-invocation-id",
      "type": "tool_result",
      "content": "result-content"
    }]
  },
  "toolUseResult": {
    "// tool-specific result data"
  }
}
```

## Common Fields

### Core Fields
- `uuid`: Unique identifier for each message
- `parentUuid`: Links messages in conversation threads (null for first message)
- `sessionId`: Groups all messages within a single session
- `timestamp`: ISO 8601 formatted timestamp
- `type`: Entry type (`user`, `assistant`, `summary`)
- `version`: Claude Code version used for the interaction

### Context Fields
- `cwd`: Current working directory when message was sent
- `userType`: User type (typically "external")
- `isSidechain`: Boolean indicating if message is part of main conversation flow

### Message Content
- `message.role`: Either "user" or "assistant"
- `message.content`: String or array of content blocks
- `message.model`: Model identifier for assistant messages
- `message.usage`: Token usage statistics for assistant messages

## Session Lifecycle

1. **Session Creation**: New JSONL file created when starting work in a project
2. **Message Recording**: Each interaction appended as a new line
3. **Summary Update**: First line updated with session summary
4. **Session Persistence**: Files retained for future reference and continuity

## Use Cases

### Session Continuity
- Resume previous conversations with full context
- Maintain state across Claude Code restarts
- Reference past decisions and implementations

### Development History
- Track project evolution over time
- Review implementation decisions
- Audit code changes and rationale

### Debugging & Analysis
- Investigate past interactions
- Analyze token usage patterns
- Debug tool invocation sequences

## Best Practices

### Storage Management
- Regularly review and archive old sessions
- Monitor disk usage in projects with many sessions
- Consider backing up important session histories

### Privacy Considerations
- Session files may contain sensitive code or data
- Ensure appropriate file permissions on the `.claude` directory
- Be aware that conversation history is persisted locally

## Technical Notes

- Files are append-only during active sessions
- Each line must be valid JSON (JSONL format)
- UTF-8 encoding is used throughout
- No built-in rotation or cleanup mechanism

## Integration Points

This directory structure enables:
- Session resume functionality (`--resume` flag)
- Historical context for better responses
- Project-specific conversation isolation
- Potential for third-party analysis tools

---

*This reference describes the Claude Code projects directory structure as of the current implementation. The format may evolve in future versions.*