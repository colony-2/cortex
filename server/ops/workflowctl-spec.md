Workflow Control Abstraction (Minimal)

- Purpose: Provide a minimal, SDK-agnostic interface for workflow control so services (e.g., input management) can work with embedded Temporal or tests without importing Temporal SDK types.

Package
- Intended path when moved to recipe-core: `server/recipe-core/pkg/workflowctl`

API Surface
```go
package workflowctl

import (
    "context"
    "errors"
    "time"
)

// Normalized status values (no SDK coupling).
type WorkflowStatus string

const (
    StatusUnspecified WorkflowStatus = "unspecified"
    StatusRunning     WorkflowStatus = "running"
    StatusCompleted   WorkflowStatus = "completed"
    StatusFailed      WorkflowStatus = "failed"
    StatusCanceled    WorkflowStatus = "canceled"
    StatusTerminated  WorkflowStatus = "terminated"
    StatusTimedOut    WorkflowStatus = "timed_out"
)

// ExecutionRef identifies a workflow execution.
type ExecutionRef struct {
    WorkflowID string
    RunID      string // optional; empty means latest
}

// WorkflowSummary is a normalized view of a workflow execution.
type WorkflowSummary struct {
    WorkflowID       string
    RunID            string
    Status           WorkflowStatus
    StartTime        *time.Time     // nil if unknown
    CloseTime        *time.Time     // nil if open/unknown
    SearchAttributes map[string]any // best-effort; may be empty
}

// WorkflowControl exposes the minimal control-plane interactions.
type WorkflowControl interface {
    // Describe returns a normalized summary for the referenced execution.
    Describe(ctx context.Context, ref ExecutionRef) (WorkflowSummary, error)

    // Signal delivers a named signal with an opaque payload to the execution.
    Signal(ctx context.Context, ref ExecutionRef, signalName string, payload any) error

    // Cancel requests cancellation of the execution.
    Cancel(ctx context.Context, ref ExecutionRef, reason string) error
}

// Canonical errors returned by implementations.
var (
    ErrNotFound    = errors.New("workflow not found")
    ErrUnavailable = errors.New("workflow service unavailable")
)
```

Semantics
- Describe: Returns WorkflowSummary with normalized Status, start/close times if known, and best‑effort search attributes. Returns ErrNotFound if the workflow is unknown.
- Signal: Fire‑and‑forget; delivers signalName with payload. Returns ErrNotFound if the workflow is unknown.
- Cancel: Requests cancellation of the target execution. Returns ErrNotFound if the workflow is unknown.

Status Normalization (adapter guidance)
- Running → StatusRunning
- Completed → StatusCompleted
- Failed → StatusFailed
- Canceled → StatusCanceled
- Terminated → StatusTerminated
- TimedOut → StatusTimedOut
- Unknown/other → StatusUnspecified

