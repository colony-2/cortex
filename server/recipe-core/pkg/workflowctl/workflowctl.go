// Package workflowctl provides a minimal, SDK-agnostic interface
// for controlling and inspecting workflow executions. It is designed
// to be implemented by runtimes like Temporal without leaking SDK types
// into recipe-core or ops packages.
package workflowctl

import (
    "context"
    "errors"
    "fmt"
    "time"
)

// DependencyName is the well-known key for retrieving a WorkflowControl
// implementation from a ServiceDependencies-style container.
const DependencyName = "workflowctl"

// Getter is the minimal dependency accessor used to retrieve a
// WorkflowControl instance without importing ops.ServiceDependencies
// (avoids a package cycle).
type Getter interface {
    Get(name string) (interface{}, error)
}

// From returns the WorkflowControl instance registered under DependencyName
// using a minimal Getter. Callers can pass ops.ServiceDependencies or any
// compatible dependency container.
func From(deps Getter) (WorkflowControl, error) {
    v, err := deps.Get(DependencyName)
    if err != nil {
        return nil, err
    }
    ctl, ok := v.(WorkflowControl)
    if !ok {
        return nil, fmt.Errorf("dependency %q has wrong type: %T", DependencyName, v)
    }
    return ctl, nil
}

// WorkflowStatus represents a normalized workflow execution status.
// The values are SDK-agnostic to avoid coupling to specific runtimes.
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
// RunID may be empty to reference the latest run for a WorkflowID.
type ExecutionRef struct {
    WorkflowID string
    RunID      string
}

// WorkflowSummary is a normalized description of a workflow execution.
// StartTime/CloseTime may be nil if unknown or if the workflow is still open.
// SearchAttributes is best-effort and may be empty.
type WorkflowSummary struct {
    WorkflowID       string
    RunID            string
    Status           WorkflowStatus
    StartTime        *time.Time
    CloseTime        *time.Time
    SearchAttributes map[string]any
}

// WorkflowControl exposes the minimal control-plane interactions
// for an execution. Implementations should map runtime-specific
// behavior and errors into this portable shape.
type WorkflowControl interface {
    // Describe returns a normalized summary for the referenced execution.
    // ErrNotFound should be returned when the execution cannot be located.
    Describe(ctx context.Context, ref ExecutionRef) (WorkflowSummary, error)

    // Signal delivers a named signal with an opaque payload to the execution.
    // The call is fire-and-forget with respect to the workflow; delivery or
    // handling status is not reported here. ErrNotFound if the execution is unknown.
    Signal(ctx context.Context, ref ExecutionRef, signalName string, payload any) error

    // Cancel requests cancellation of the execution. Implementations should
    // translate runtime-specific outcomes to ErrNotFound when applicable.
    Cancel(ctx context.Context, ref ExecutionRef, reason string) error
}

// Canonical errors returned by implementations.
var (
    ErrNotFound    = errors.New("workflow not found")
    ErrUnavailable = errors.New("workflow service unavailable")
)

