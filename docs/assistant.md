# Contextual assistant

The Discuss launcher opens a conversation beside the current page. A new conversation includes no page evidence by default. On a story or loaded review, select the checkbox to include that story or the exact review request and its documents. Navigation keeps the original capture; use New conversation to select another page. Minimize preserves the unsent draft. Switching tenants discards the draft, transcript and evidence selection.

The panel displays recorded replies and individually inspectable source coverage. A denied, unavailable, unsupported, truncated or omitted document must not be treated as fully read. Downloads use opaque citation IDs and reauthorize access. Review decisions stay on the review page. For the goal lifecycle, Close cancels the observed execution and Reopen creates a fresh prompt-ready job in the same goal; it exposes no goal creation, approval, shell or editing action.

The serving adapter uses the accepted assistant module `0561937f92a8e15c9eedb0d3045ab432f8e2c17a` and matching immutable `discussion-goal.yaml`. The default server reports the assistant unavailable and continues to serve ordinary pages until trusted configuration and execution readiness are supplied. There is no endpoint or credential fallback.

## Trusted composition

Set `--assistant-config /path/to/assistant.json` (or `CORTEX_ASSISTANT_CONFIG`) when serving Cortex. This trusted JSON file is bounded to 64 KiB and rejects unknown fields. For example:

```json
{
  "root": {
    "cell_name": "root",
    "cell_repo": "github.com/example/root",
    "repository_source": "https://github.com/example/root.git",
    "default_ref": "main"
  },
  "bindings": {
    "BaseURL": "https://provider.example/v1",
    "APIKey": {"$c2j_secret": "discussion_model_key"},
    "Provider": "openai",
    "Model": "your-unprefixed-model-id"
  },
  "cursor_key": "REPLACE_WITH_BASE64_OF_AT_LEAST_32_RANDOM_BYTES",
  "execution_ready": true
}
```

Keep the cursor integrity key stable and private across serving instances. Provision the named model secret in the existing runtime secret authority; its value never belongs in this file. Set readiness only when ordinary workers can resolve and execute the pinned recipe, helper and inference package. Endpoint must use HTTPS; model IDs cannot contain provider prefixes. Invalid or incomplete bindings leave the assistant unavailable while ordinary pages remain available. Malformed configuration files fail startup with a safe configuration error.

`cortex.Config.AssistantRuntime` composes the actual facade with existing runtime/input/history/control/artifact services. The optional `cortex.Config.Assistant` hook remains available for deployment integrations; choose one composition. Without deployment identity callbacks, the existing tenant runtime uses a fixed server-owned discussion actor. Browser cookies and identity headers do not choose that actor. The built-in composition is restricted to the configured default tenant. New principal isolation and revocation remain deferred; existing runtime controls still apply.

Request-boundary capture reads selected authoritative story/review data with document/count/total limits, stable document order, exact review freshness checks and original producer references. Conversion validates the released frozen snapshot, hashes included source bytes and accounts for JSON escaping within the snapshot budget; omitted documents retain individual coverage. Trusted selection references are independently copied so modifying a submitted producer does not also modify its validation baseline. Browser projection preserves recorded job/task/attempt/invocation and receipt identities, while omitting executable proposals, routing and raw errors. cortex-assistant owns bounded snapshots, coverage, citations, durable history and recipe semantics; c2ops owns inference. Existing runtime controls and tenant/source integrity remain required. New identity, permission, revocation and two-principal behavior are deferred.

The browser boundary exposes `Start`, `List`, `Get`, `Replay`, `Respond`, `Close`, `Reopen` and `OpenArtifact`. Goal lifecycle methods are supplied through `AssistantLifecycleService`; frozen legacy contracts retain their original adapter path. Browser DTOs omit executable action fields, retained raw requests, provider exceptions and credential-bearing configuration. Entry receipts permit recovery after an uncertain HTTP response.

Sessions execute through ordinary recipes on existing workers. Cortex owns no runner, custom activity registration, inference adapter or second conversation store. The deterministic acceptance host resolves the unchanged c2ops inference package `nix:github:colony-2/c2ops/6396e1fe6462084e85ea44d86d372bdf3b133db0#pydantic`; the assistant recipe owns this selection. Cortex introduces no model operation override.

## Browser API

Routes are tenant-bound under `/api/projects/{tenant}/assistant`:

| Route | Method | Purpose |
| --- | --- | --- |
| `/capabilities` | GET | Trusted availability |
| `/sessions` | GET / POST | Bounded history / frozen start |
| `/sessions/{id}` | GET | Authoritative session and latest replay page |
| `/sessions/{id}/replay` | GET | Bounded continuation |
| `/sessions/{id}/respond` | POST | Receipt-based discussion send; legacy finish remains adapter-controlled |
| `/sessions/{id}/close` | POST | Cancel the observed execution |
| `/sessions/{id}/reopen` | POST | Reconcile a sequential reopen through the facade |
| `/sessions/{id}/artifacts/{citation}` | GET | Authorized recorded bytes |

Start fields are serving-process `generation`, `request_id`, frozen `submitted_at`, `prompt` and `source`. Global sources contain only `kind: global`; stories add `job_id`; reviews also require exact `request_id`. No source URL, model, credential, tenant claim or arbitrary attachment field is accepted. Respond fields are `request_id`, stable `submission_id`, frozen `submitted_at`, `command` and optional `prompt`. Goal Reopen also freezes `generation`. Goal Close/Reopen fields are frozen `request_id`, `submitted_at` and the observed `job_id`. The browser keeps the same envelope after uncertain transport. An empty reopen body is reserved for legacy adapters; a goal adapter must reject it and legacy `finish` submissions for goal sessions. Unknown fields and trailing JSON values are rejected.

Mutations require JSON and an exact same origin or explicitly configured permitted origin. `*` is not mutation authority. Browser requests are bounded to 40 KiB, prompts to 32 KiB UTF-8, identifiers to 512 bytes, cursors to 4096 bytes and history/replay pages to 50 entries. Encoded responses are capped at 4 MiB. HTTP reads/submissions have a 20-second context; the browser client also aborts hung HTTP requests. Aborting a read does not cancel inference. Downloads are capped at 8 MiB, buffered to verify their declared length, and served as attachments with `nosniff` and no-store. The released facade further limits human messages to 16 KiB of encoded text, captured snapshots to 64 KiB encoded, and each source document to 32 KiB. Requests that exceed the release limits fail safely.

Polling has one active refresh, a one-second normal interval, capped failure backoff, visibility pause and reconnect refresh. Late responses are discarded using tenant, session, execution and lifecycle-request generations. Failed sends retain frozen identity and payload for explicit retry. No reply or sent message is invented locally; authoritative replay supplies transcript entries. Markdown disables raw HTML and image loads, rejects executable protocols, and builds download routes only from recorded IDs.

Goal sessions explicitly declare `lifecycle: goal`, `goal_id` and the current `job_id`; their session ID remains stable across jobs. Close is available while queued, processing or waiting, and sending stays disabled during cancellation/reconciliation. A confirmed terminal close hides the panel and restores launcher focus. Reopen retains the unsent draft for deliberate sending, preserves earlier history and does not replay the opening prompt. An uncertain lifecycle request preserves its original observed execution and logical identity. Safe Start/Reopen retries require the original complete prepared submission bytes and destination; the browser envelope alone cannot reconstruct them.

Before freezing a new Start or goal Reopen, the panel reads current capabilities so a still-mounted browser can continue after serving-state replacement. It locks admission during that read and discards results from a detached tenant or execution. A failed capability read submits no operation. An uncertain retry keeps the original frozen generation and request instead of refreshing them.

The adapter retains at most 128 prepared operations and 32 MiB in aggregate per serving process, reserving 1 MiB for each in-flight preparation. It retains complete opaque frozen bytes and the original facade/runtime destination before publication, including successful and uncertain operations. New admission fails closed at either bound; existing frozen retries remain available. It serializes only duplicate logical operation keys, leaving concurrent sends to the facade's authoritative receipt checks. Records are never evicted or persisted.

The approved scope excludes new durable request storage. If a Start/Reopen response is lost and Cortex then restarts or loses its prepared-operation bytes, automatic reconciliation of that request is deferred. It may have succeeded. The panel shows an explicit unresolved outcome, disables retry and preserves the draft. Selecting recorded history does not establish the earlier receipt; starting a separate conversation requires a deliberate action. Recorded conversation history remains authoritative, but finding a conversation is not automatically proof of that request's receipt. This limitation does not waive ordinary successful sequential lifecycle, retries with retained bytes, concurrent sends, or known-session history recovery across reload and Cortex/worker replacement.

Minimize and Escape hide the panel without cancelling execution. Simultaneous multi-client reopen and duplicate open jobs from that race are excluded. Sequential close/reopen, delayed-close targeting, concurrent sends at one prompt and recovery remain required. No successor-election coordinator is introduced.

## Verification boundary

`server/internal/cortex/assistant_test.go` exercises the HTTP/security boundary with a service fixture. `web/shared/src/assistant.test.ts` verifies the browser envelope, retries and replay merge. `AssistantPanel.test.tsx` exercises asynchronous state, rendering and polling with explicit facade HTTP fixtures. `web/app/tests/assistant.spec.ts` checks real Chromium desktop/mobile behavior with intercepted assistant responses.

`web/app/tests/assistant-runtime.spec.ts` adds actual HTTP/browser acceptance using `server/cmd/assistanttestserver`: durable SQLite and blob storage, separate ordinary c2j worker processes, the exact clean merged assistant source, real resolved c2ops inference and a deterministic HTTPS provider. It checks actual model requests, original origins and download hashes; lifecycle races, frozen retries, replacement, explicit legacy descriptors and expired downloads use the same host. The host's controls and source fixtures exist only in this test command and are not production routes.

Run from the repository root:

```sh
pnpm test:e2e:assistant
```

The harness builds workers from the effective server module graph, checks out the exact assistant revision in scratch and uses the ordinary tenant Git profile to serve those unchanged bytes locally. Only provider responses are deterministic. Run this separately from the general browser suite, whose servers do not supply the assistant runtime fixture. Local execution requires installed Nix and network/cache access for the pinned source/packages. Evidence certifies this source integration; production deployment, Pulse dispatch and actual-provider compatibility require separate observations.
