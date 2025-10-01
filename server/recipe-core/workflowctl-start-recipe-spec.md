# Workflow Control `StartRecipe` Specification

## Overview
Extend the `workflowctl.WorkflowControl` abstraction with a `StartRecipe` entry point so recipe-aware callers can launch new workflow executions without reaching into Temporal SDKs. The new method captures the initiating actor (shared with ticketing) and the logical recipe name, returning a portable execution reference. As part of the change, the reusable `Actor` model will move from `server/ticket` into `recipe-core` so both workflow orchestration and ticketing share a single definition.

## Goals
- Provide a runtime-agnostic API to start recipe executions from core/ops packages.
- Include the initiating actor and recipe name on every start request.
- Centralize the `Actor` model inside `recipe-core` to avoid ad-hoc copies.
- Preserve existing ticket storage behaviour while re-exporting the shared actor types.

## Non-Goals
- Designing new REST/CLI surfaces that call `StartRecipe` (will follow once the API exists).
- Changing worker-side recipe execution semantics or metadata schema beyond what is required to record the actor.
- Replacing the existing Temporal client integrations in one pass (incremental adoption is acceptable).

## Current State
- `workflowctl.WorkflowControl` exposes `Describe`, `Signal`, and `Cancel`. Callers that need to start workflows use Temporal SDK clients directly, coupling recipe-core to SDK types.
- Ticketing defines `Actor`/`ActorUser`/`ActorAgent` in `server/ticket/internal/model` and re-exports them from `pkg/ticket`. The representation is duplicated when other systems need to identify humans or automations.
- Actor metadata is not surfaced to workflow starts, limiting audit trails and recipe ownership when triggered outside the ticket system.

## Proposed Changes

### 1. Shared Actor Package in recipe-core
- Add `server/recipe-core/pkg/identity` (name chosen to allow future shared identity concepts) containing:
  ```go
  package identity

  type ActorType string

  const (
      ActorTypeUser  ActorType = "user"
      ActorTypeAgent ActorType = "agent"
  )

  type ActorUser struct {
      Email string `json:"email"`
  }

  type ActorAgent struct {
      CellName       string `json:"cell"`
      WorkflowName   string `json:"workflow_name"`
      ExecutionID    string `json:"execution_id"`
      InvocationHash string `json:"invocation_hash"`
  }

  type Actor struct {
      Type  ActorType
      User  *ActorUser  `gorm:"embedded;embeddedPrefix:actor_user_"`
      Agent *ActorAgent `gorm:"embedded;embeddedPrefix:actor_agent_"`
  }

  func NewUserActor(email string) Actor { ... }
  func NewAgentActor(cell, workflow, executionID, invocationHash string) Actor { ... }
  ```
- Keep the existing struct tags so `server/ticket` continues to embed `Actor` into GORM models without extra glue.
- Update `server/ticket/internal/model` to import `identity` and alias (`type Actor = identity.Actor`, etc.) so the public ticket API is unchanged. Update helper constructors to delegate to the new package.
- Adjust go.mod files: add a `require github.com/divisive-ai/vibethis/server/recipe-core v0.0.0` entry in `server/ticket`, with a local `replace` mirroring existing patterns.

### 2. New StartRecipe API Surface
- Extend the interface in `pkg/workflowctl/workflowctl.go`:
  ```go
  type RecipeStartRequest struct {
      RecipeName       string
      Actor            identity.Actor
      InvocationID     string            // optional idempotency token
      Input            any               // optional recipe input payload
      SearchAttributes map[string]any    // optional additional index data
      Tags             map[string]string // optional string metadata
  }

  type RecipeStartResult struct {
      Execution ExecutionRef
      StartedAt time.Time
  }

  type WorkflowControl interface {
      Describe(ctx context.Context, ref ExecutionRef) (WorkflowSummary, error)
      Signal(ctx context.Context, ref ExecutionRef, signalName string, payload any) error
      Cancel(ctx context.Context, ref ExecutionRef, reason string) error
      StartRecipe(ctx context.Context, req RecipeStartRequest) (RecipeStartResult, error)
  }
  ```
- The `Actor` field is required. `RecipeName` is required. Optional fields support future adoption without needing another interface break.
- Define new canonical errors as needed, e.g. `ErrAlreadyExists` (idempotency collision) and `ErrInvalidRequest` (missing recipe/actor). Implementations map Temporal-specific failures to these errors.
- Document how `RecipeName` maps to runtime concepts (Temporal workflow type or recipe registry ID) inside the package comments.

### 3. Runtime / Temporal Expectations
- Temporal-backed implementations (`recipe-worker` control plane) capture actor metadata by:
  - Storing the actor JSON in workflow memo for audit.
  - Optionally indexing `ActorType`/`ActorUser.Email` as search attributes if the namespace allows.
  - Using `InvocationID` for Temporal idempotency keys (`WorkflowID` hashing) when provided.
- Ensure the implementation returns the assigned `WorkflowID`/`RunID` as `ExecutionRef` and the authoritative start time.

### 4. Ops & Dependency Integration
- Update `pkg/ops/deps_workflowctl.go` helpers to expose `StartRecipe` so operations that already access `WorkflowControl` can start recipes without Temporal imports.
- Update relevant documentation (`AGENTS.md`, `RUNTIME_CONFIGURABLE_SPEC.md`) with the new method and actor package location.

## Backwards Compatibility
- Interface addition is a breaking change for existing `WorkflowControl` implementations (currently in `recipe-worker`). We will update those implementations in the same change set.
- Ticket public API retains type aliases, so downstream consumers continue using `ticket.Actor` without code changes.
- Actor JSON shape is unchanged; moving the definition does not impact persisted rows or API contracts.

## Implementation Plan
1. Create `pkg/identity` with actor types and constructors; add unit tests.
2. Update `workflowctl` package to import `identity` and define `RecipeStartRequest`/`RecipeStartResult`; extend the interface and error set; refresh docs.
3. Refactor `server/ticket` model/helpers to consume `identity` and adjust go.mod/replace entries.
4. Update Temporal-backed `WorkflowControl` implementation in `recipe-worker` to satisfy the new method, handling actor serialization, idempotency, and error mapping.
5. Update ops helpers and documentation references; ensure `ServiceDependencies2` guidance mentions the new capability.
6. Add tests (or mocks) covering successful starts, validation failures, and runtime error translation.

## Open Questions
- Should `InvocationID` be required for safety, or remain optional with runtime-specific defaults?
- Do we want to enforce actor presence at compile time (e.g. separate constructors) or keep runtime validation?
- Which search attributes should be indexed by default for user actors (email, cell) to balance observability and PII safeguards?

