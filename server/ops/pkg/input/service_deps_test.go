package input

import (
    "context"
    "fmt"
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
    "go.temporal.io/sdk/client"
)

// ServiceDependencies is a test helper to satisfy ops.ServiceDependencies
type ServiceDependencies struct {
    TemporalClient client.Client
    SSEManager     ops.SSEManager
    TemporalNamespace string
    // Optional typed workflow controller for tests; may be nil.
    WorkflowCtl workflowctl.WorkflowControl
}

func (d ServiceDependencies) Get(name string) (interface{}, error) {
    switch name {
    case "temporal_client":
        if d.TemporalClient == nil {
            return nil, fmt.Errorf("temporal client not provided")
        }
        return d.TemporalClient, nil
    case "sse":
        if d.SSEManager == nil {
            return nil, fmt.Errorf("sse manager not provided")
        }
        return d.SSEManager, nil
    case "temporal_namespace":
        if d.TemporalNamespace == "" {
            return nil, fmt.Errorf("temporal namespace not provided")
        }
        return d.TemporalNamespace, nil
    default:
        return nil, fmt.Errorf("dependency not found: %s", name)
    }
}

// WorkflowControl implements ops.ServiceDependencies2 for tests.
func (d ServiceDependencies) WorkflowControl() (workflowctl.WorkflowControl, bool) {
    if d.WorkflowCtl != nil {
        return d.WorkflowCtl, true
    }
    return nil, false
}

// mockWorkflowControl is a lightweight test double for workflowctl.WorkflowControl.
// Methods return configured errors and record the last call for assertions.
type mockWorkflowControl struct {
    // Configure per-method behavior
    DescribeErr     error
    DescribeSummary workflowctl.WorkflowSummary
    SignalErr       error
    CancelErr       error

    // Capture last-call details for assertions
    LastDescribeRef workflowctl.ExecutionRef
    LastSignalRef   workflowctl.ExecutionRef
    LastSignalName  string
    LastSignalArg   any
    LastCancelRef   workflowctl.ExecutionRef
    LastCancelReason string
}

func (m *mockWorkflowControl) Describe(_ context.Context, ref workflowctl.ExecutionRef) (workflowctl.WorkflowSummary, error) {
    m.LastDescribeRef = ref
    if m.DescribeErr != nil {
        return workflowctl.WorkflowSummary{}, m.DescribeErr
    }
    return m.DescribeSummary, nil
}

func (m *mockWorkflowControl) Signal(_ context.Context, ref workflowctl.ExecutionRef, signalName string, payload any) error {
    m.LastSignalRef = ref
    m.LastSignalName = signalName
    m.LastSignalArg = payload
    return m.SignalErr
}

func (m *mockWorkflowControl) Cancel(_ context.Context, ref workflowctl.ExecutionRef, reason string) error {
    m.LastCancelRef = ref
    m.LastCancelReason = reason
    return m.CancelErr
}
