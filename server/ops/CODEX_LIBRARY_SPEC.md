Codex Non-Interactive Library Specification

Context
- Recipes and future ops need a reusable way to drive `codex exec` in non-interactive mode with predictable outputs.
- Today logic for command construction, Shai invocation, stdout capture, JSON parsing, and blobstore persistence is not centralized.

Goals
- Provide a Go library in `server/ops/pkg/codex` that encapsulates interaction with Codex CLI for non-interactive runs.
- Enforce a canonical structured-output contract, surface resume/session metadata, and persist raw stdout to blobstore.
- Allow consumers (ops, services, tests) to exercise Codex behaviour without duplicating orchestration code.

Public API (initial draft)
- Package path: `server/ops/pkg/codex`.
- Primary entry point:
  - `func Execute(ctx context.Context, opts Options) (Result, error)`
- Auxiliary helpers:
  - `func BuildSchema() ([]byte, error)` or exported `var StructuredOutputSchema = []byte(...)`
  - `type Runner interface { Run(ctx context.Context, opts Options) (Result, error) }` for easier testing/mocking.

Options
```go
type Options struct {
    Prompt           string            // required
    SessionID        string            // optional resume identifier
    Model            string            // optional custom model flag
    ExtraEnv         map[string]string // merged with process env

    WorktreeRoot     string            // absolute path used as Shai working dir
    CellRelativePath string            // path relative to worktree granted RW access
    BlobstoreURI     string            // base URI (same provider as gitstate)
    WorkflowID       string            // used for blob namespace scoping

    Clock            Clock             // injectable clock for deterministic blob paths (tests)
    Logger           log.Logger        // optional structured logger interface
}
```
- Library validates required fields (`Prompt`, `WorktreeRoot`, `CellRelativePath`, `BlobstoreURI`, `WorkflowID`).
- `ExtraEnv` allows consumers to inject env vars (e.g., API keys) without modifying library defaults.
- `Clock` defaults to `time.Now` when nil.

Result
```go
type Status string

const (
    StatusCompleted Status = "completed"
    StatusIncomplete Status = "incomplete"
    StatusError      Status = "error"
)

type Result struct {
    Status              Status
    SessionID           string   // resolved from Codex thread.started event
    AssistantSummary    string
    IncompleteReason    string
    IncompleteCategory  string   // dependency_blockers | agent_abandoned | ""
    PendingDependencies []Dependency
    ErrorMessage        string   // non-empty when Status == StatusError
    Stderr              string   // aggregated stderr text
    StdoutBlobURI       string   // persisted stdout location
}

type Dependency struct {
    Component        string
    RequestedChanges string
}
```
- `Status` is a closed enum represented by the `Status` type with constants `StatusCompleted`, `StatusIncomplete`, and `StatusError`; JSON serialization still yields the lowercase strings expected by downstream consumers and the schema.

Canonical Codex Structured Output Schema
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "status",
    "assistantSummary",
    "incompleteReason",
    "incompleteCategory",
    "pendingDependencies",
    "errorMessage"
  ],
  "additionalProperties": false,
  "properties": {
    "status": {
      "type": "string",
      "enum": ["completed", "incomplete", "error"]
    },
    "assistantSummary": { "type": "string" },
    "incompleteReason": { "type": "string" },
    "incompleteCategory": {
      "type": "string",
      "enum": ["dependency_blockers", "agent_abandoned", ""]
    },
    "pendingDependencies": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["component", "requestedChanges"],
        "additionalProperties": false,
        "properties": {
          "component": { "type": "string" },
          "requestedChanges": { "type": "string" }
        }
      }
    },
    "errorMessage": { "type": "string" }
  }
}
```
- Library emits this schema to a temp file and passes it to Codex via `--output-schema`.
- Codex requires every property listed in `properties` to appear in `required`; optional values should be surfaced as empty strings when unused.
- Schema exported as `[]byte` (or string) so consumers/tests can validate outputs independently.

Execution Flow
1. Validate `opts` and resolve dependencies (default clock, logger, env).
2. Create temporary working directory under the writable cell path for schema file and stdout capture.
3. Materialize structured-output schema into `<tmp>/codex_schema.json`.
4. Prepare stdout sink file `<tmp>/stdout.jsonl` and buffers for stderr.
5. Build Codex argv:
   - `codex exec --experimental-json --dangerously-bypass-approvals-and-sandbox --skip-git-repo-check --output-schema <schemaPath>`.
   - Append model flag when provided.
   - Insert `resume <SessionID>` tokens when `opts.SessionID` is non-empty.
   - Final positional argument is the prompt string.
6. Invoke Codex through Shai ephemeral runner:
   - Configure `WorkingDir = opts.WorktreeRoot` and read-write path = `opts.CellRelativePath`.
   - Set `PostSetupExec` command to the argv above with `UseTTY=false`.
   - Stream stdout to the prepared file and stderr into buffer.
7. Parse JSONL stdout events to reconstruct conversation:
   - Capture `thread.started` (session id), `assistant_message` (structured payload), and `turn.failed` events.
   - Validate assistant message JSON against canonical schema. On failure, surface an error result.
8. Map structured payload into `Result` fields.
9. Determine status precedence: CLI exit code, validation failure, structured status, heuristics for missing category.
10. Derive blobstore key: `codex/<workflow-id>/<timestamp>/<uuid>.jsonl`.
11. Upload stdout file using blobstore client (same abstraction as gitstate).
12. Populate `Result.StdoutBlobURI` with uploaded object URI and return result/stderr.

Error Handling
- CLI exit != 0 → `Status = "error"`, capture exit code and stderr in `ErrorMessage`.
- Missing/invalid structured payload → `Status = "error"`, `ErrorMessage` describes validation issue.
- Blobstore upload failure → return error (no partial result) so caller retries or aborts.
- Shai start/attach errors → wrap and return.

Testing Strategy
1. Command builder unit tests: verify argv includes schema path, resume tokens, model flag.
2. JSONL parser tests: feed canned streams for completed, dependency blockers, agent abandonment, and turn failure scenarios.
3. Structured schema validation tests: ensure malformed payloads trigger errors.
4. Blobstore tests: use in-memory or temp filesystem provider to assert correct pathing and URI formatting.
5. Resume cycle test: first run returns session ID; second run with `SessionID` reuses/resets appropriately.
6. Error propagation tests: non-zero exit, Shai failure, upload failure.

Implementation Notes
- Place implementation under `server/ops/pkg/codex` with subfiles: `execute.go`, `command.go`, `parser.go`, `blobstore.go`, `schema.go`, `execute_test.go`.
- Reuse existing blobstore abstractions exposed via gitstate adapter context (consider extracting shared interface if needed).
- Provide small helper interfaces (`ShaiRunner`, `BlobStore`) to allow dependency injection in tests.
- Ensure library is agnostic of Temporal; callers provide workflow/site context.

Open Items
- Confirm blobstore client interface from gitstate is accessible or needs to be refactored for reuse.
- Decide whether `Result.Stderr` should be truncated or fully persisted based on size observations (initial implementation: full text).
