# External Artifact Fetch/Auth Proposal

## Goal

Add fetch, auth, and credential handling for external artifacts without turning credentials into recipe state and without pushing host-specific logic into op execution code.

This proposal assumes the current model remains true:

1. External artifacts are a recipe-layer concept.
2. `artifacts.Ref` stores only pointer metadata such as `{url, expand}`.
3. Credentials are resolved only at materialization time inside the worker runtime.

## Current Gap

The current external artifact implementation materializes `http(s)` refs directly from `recipe-worker/pkg/ops/op_executor.go` using the default HTTP client.

That is sufficient for:

1. `file://` refs
2. unauthenticated `http(s)` refs

But it is not sufficient for:

1. GitHub artifact/log downloads
2. signed or bearer-token API fetches
3. per-project or per-cell credentials
4. centralized retry, timeout, redaction, and audit behavior

## Design Principles

### 1. Keep Refs Credential-Free

`artifacts.Ref` should continue to store only pointer metadata:

1. artifact name
2. URL
3. `expand`

It should not store:

1. bearer tokens
2. cookies
3. signed headers
4. long-lived secrets

This keeps recipe state replay-safe and avoids leaking secrets into persisted task output.

### 2. Resolve Credentials At Runtime

Auth should be derived from runtime context:

1. project / tenant
2. job identity
3. cell / workflow context
4. URL host and path

That lets the runtime choose the right credential source for the destination being fetched.

### 3. Move Fetch Logic Behind A Service Boundary

`op_executor` should not know how to:

1. build authenticated requests
2. choose tokens
3. sign requests
4. decide retry policy
5. interpret provider-specific rules

It should delegate to a dedicated materializer service.

## Proposed Architecture

### Runtime Service

Add a new runtime dependency, for example:

```go
type ExternalArtifactMaterializer interface {
	Materialize(ctx context.Context, req MaterializeRequest) error
}

type MaterializeRequest struct {
	JobKey      swf.JobKey
	JobContext  contextual.JobContext
	BindingName string
	Ref         artifacts.Ref
	InboxRoot   string
}
```

Responsibility:

1. validate the ref
2. fetch or copy the source
3. expand archives when requested
4. materialize content into the inbox safely

### Credential Resolution

Add a separate dependency for auth lookup:

```go
type CredentialResolver interface {
	ResolveHTTPRequestAuth(ctx context.Context, scope CredentialScope, u *url.URL) (RequestAuth, error)
}

type CredentialScope struct {
	JobKey     swf.JobKey
	JobContext contextual.JobContext
}

type RequestAuth interface {
	Apply(req *http.Request) error
}
```

Responsibility:

1. inspect the destination URL
2. inspect runtime context
3. choose the right credential
4. return an auth applicator that mutates the request

Examples of `RequestAuth` implementations:

1. no auth
2. bearer token header
3. basic auth
4. custom provider-specific header set

## Wiring Into Existing Code

### Extend `ServiceDependencies2`

The natural place for the new dependency is `recipe-core/pkg/ops/service_deps2.go`.

Suggested additions:

```go
type ServiceDependencies2 interface {
	WorkflowControl() workflowctl.WorkflowControl
	SSEManager() SSEManager
	Database() *gorm.DB
	ExternalArtifactMaterializer() ExternalArtifactMaterializer
	CredentialResolver() CredentialResolver
	serviceDependenciesMarker()
}
```

Builder methods would mirror the existing pattern:

1. `WithExternalArtifactMaterializer(...)`
2. `WithCredentialResolver(...)`

### Change `op_executor`

`recipe-worker/pkg/ops/op_executor.go` should stop issuing `http.DefaultClient` requests directly.

Instead:

1. stored refs stay on the existing SWF-backed path
2. external refs call the injected materializer service

Conceptually:

```go
if ref.External != nil {
	return deps.ExternalArtifactMaterializer().Materialize(ctx, req)
}
```

This keeps `op_executor` focused on orchestration, not transport/auth policy.

## Scheme Handling Pattern

The materializer should use a scheme registry internally.

Suggested first registry:

1. `file://`
2. `http://`
3. `https://`

Possible shape:

```go
type FetchHandler interface {
	Materialize(ctx context.Context, req MaterializeRequest) error
}
```

Where the materializer dispatches by URL scheme:

1. `file://` handler
2. `http(s)` handler
3. future handlers like `s3://` or `gs://`

This avoids hard-coding provider logic into a single function.

## HTTP Fetch Pattern

For `http(s)` refs, the common pattern is:

1. build a request with context
2. resolve auth for the target URL
3. apply auth to the request
4. send via injected `*http.Client`
5. enforce shared timeout/retry/user-agent policy
6. materialize or expand the response body

Suggested internal shape:

```go
type HTTPArtifactFetcher struct {
	Client             *http.Client
	CredentialResolver CredentialResolver
}
```

That is preferable to:

1. ad hoc `http.Get`
2. embedding tokens in refs
3. special-casing GitHub inside `op_executor`

## Credential Source Pattern

The credential resolver should be responsible for mapping a destination URL to a runtime credential source.

Typical resolution inputs:

1. tenant / project
2. job context
3. workflow cell
4. hostname
5. path prefix

Typical resolution outputs:

1. `Authorization: Bearer ...`
2. basic auth
3. custom headers
4. no auth

Common matching rules:

1. host-level match
2. host + path-prefix match
3. explicit provider type

Examples:

1. `api.github.com/repos/.../actions/artifacts/...` -> GitHub token
2. internal artifact proxy host -> internal service token
3. public file host -> no auth

## Where Credentials Should Live

This proposal does not require a specific backing store, but the runtime-facing abstraction should allow credentials to come from:

1. environment-backed runtime config
2. project-scoped secrets
3. cell-scoped integration credentials
4. future secret manager integrations

The key point is that fetch code depends only on `CredentialResolver`, not on any one storage mechanism.

## Safety And Observability

The fetch/materializer layer should centralize:

1. request timeout policy
2. retry policy for transient network failures
3. redaction of auth headers in logs
4. structured logs with host, scheme, status, and duration
5. audit events for external fetches if needed later

Error messages should identify:

1. artifact binding name
2. destination host
3. failure class such as auth, not found, timeout, or invalid archive

They should not include:

1. tokens
2. full signed URLs if they contain secret query params

## Testing Strategy

### Unit Tests

1. credential resolver returns no auth / bearer / basic auth as expected
2. HTTP fetcher applies auth to outgoing requests
3. logs and errors redact secrets
4. `file://` path remains credential-free

### Integration Tests

1. external artifact ref to authenticated test HTTP server
2. host-based credential selection
3. archive expansion after authenticated download
4. auth failure and 404 failure reporting

### Non-Regression Tests

1. stored SWF artifacts still materialize unchanged
2. unauthenticated public `http(s)` fetch still works
3. recipe outputs still pass through refs without materializing bytes

## Recommended Rollout

### Phase 1

Add the interfaces and inject a default materializer/fetcher.

Behavior:

1. `file://` works as today
2. `http(s)` uses injected client
3. auth resolver may initially return "no auth"

### Phase 2

Add a host-aware credential resolver.

Behavior:

1. GitHub artifact/log URLs can resolve bearer auth
2. provider-specific runtime config can be introduced without changing recipe state

### Phase 3

Add richer provider support if needed:

1. signed URL generation
2. extra schemes
3. caching
4. audit / metrics hooks

## Recommendation

The common pattern for this kind of problem is:

1. keep refs as pure pointers
2. inject a materializer service
3. inject a credential resolver
4. centralize HTTP client behavior
5. let `op_executor` delegate instead of implementing auth itself

That gives us a clean split between:

1. recipe semantics
2. transport/materialization
3. credential policy

and it leaves room for GitHub and other integrations without changing the recipe-layer artifact model.
