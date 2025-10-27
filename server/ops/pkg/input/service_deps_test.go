package input

import (
	"context"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
)

// mockWorkflowControl is a lightweight test double for workflowctl.WorkflowControl.
// Methods return configured errors and record the last call for assertions.
type mockWorkflowControl struct {
	// Configure per-method behavior
	DescribeErr     error
	DescribeSummary workflowctl.WorkflowSummary
	SignalErr       error
	CancelErr       error
	ListErr         error
	ListResp        workflowctl.ListWorkflowsResponse

	// Capture last-call details for assertions
	LastDescribeRef  workflowctl.ExecutionRef
	LastSignalRef    workflowctl.ExecutionRef
	LastSignalName   string
	LastSignalArg    any
	LastCancelRef    workflowctl.ExecutionRef
	LastCancelReason string
	LastListRequest  workflowctl.ListWorkflowsRequest
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

func (m *mockWorkflowControl) ListWorkflows(_ context.Context, req workflowctl.ListWorkflowsRequest) (workflowctl.ListWorkflowsResponse, error) {
	m.LastListRequest = req
	if m.ListErr != nil {
		return workflowctl.ListWorkflowsResponse{}, m.ListErr
	}
	return m.ListResp, nil
}

func (m *mockWorkflowControl) ResetWorkflow(context.Context, workflowctl.ResetRequest) (workflowctl.ResetResponse, error) {
	return workflowctl.ResetResponse{}, nil
}

func (m *mockWorkflowControl) StartWorkflow(context.Context, workflowctl.StartRequest) (workflowctl.StartResponse, error) {
	return workflowctl.StartResponse{}, nil
}

func (m *mockWorkflowControl) StartChildWorkflow(context.Context, workflowctl.StartChildRequest) (workflowctl.StartChildResponse, error) {
	return workflowctl.StartChildResponse{}, nil
}
