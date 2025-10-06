package input

import (
	"context"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
)

// ServiceDependencies is a test helper to satisfy ops.ServiceDependencies2.
type ServiceDependencies struct {
	WorkflowCtl workflowctl.WorkflowControl
	SSEMgr      ops.SSEManager
	Namespace   string
}

func (d ServiceDependencies) WorkflowControl() (workflowctl.WorkflowControl, bool) {
	if d.WorkflowCtl != nil {
		return d.WorkflowCtl, true
	}
	return nil, false
}

func (d ServiceDependencies) SSEManager() (ops.SSEManager, bool) {
	if d.SSEMgr != nil {
		return d.SSEMgr, true
	}
	return nil, false
}

func (d ServiceDependencies) TemporalNamespace() (string, bool) {
	if d.Namespace != "" {
		return d.Namespace, true
	}
	return "", false
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
	LastDescribeRef  workflowctl.ExecutionRef
	LastSignalRef    workflowctl.ExecutionRef
	LastSignalName   string
	LastSignalArg    any
	LastCancelRef    workflowctl.ExecutionRef
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
