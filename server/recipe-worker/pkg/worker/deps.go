package worker

import (
	"context"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

func newWorkerDependencies(cli client.Client, namespace string) ops.ServiceDependencies2 {
	builder := ops.NewServiceDepsBuilder().
		WithSSEManager(noopSSEManager{}).
		WithTemporalNamespace(namespace)
	if cli != nil {
		builder = builder.WithWorkflowControl(&temporalWorkflowControl{client: cli, namespace: namespace})
	}
	return builder.Build()
}

type noopSSEManager struct{}

func (noopSSEManager) Broadcast(event ops.SSEEvent) {}

func (noopSSEManager) Subscribe(clientID string) <-chan ops.SSEEvent {
	ch := make(chan ops.SSEEvent)
	close(ch)
	return ch
}

func (noopSSEManager) Unsubscribe(clientID string) {}

type temporalWorkflowControl struct {
	client    client.Client
	namespace string
}

func (c *temporalWorkflowControl) Describe(ctx context.Context, ref workflowctl.ExecutionRef) (workflowctl.WorkflowSummary, error) {
	if c == nil || c.client == nil {
		return workflowctl.WorkflowSummary{}, workflowctl.ErrUnavailable
	}
	req := &workflowservice.DescribeWorkflowExecutionRequest{
		Namespace: c.namespace,
		Execution: &commonpb.WorkflowExecution{
			WorkflowId: ref.WorkflowID,
			RunId:      ref.RunID,
		},
	}
	resp, err := c.client.WorkflowService().DescribeWorkflowExecution(ctx, req)
	if err != nil {
		return workflowctl.WorkflowSummary{}, mapTemporalError(err)
	}
	info := resp.GetWorkflowExecutionInfo()
	if info == nil {
		return workflowctl.WorkflowSummary{}, workflowctl.ErrUnavailable
	}
	summary := workflowctl.WorkflowSummary{
		WorkflowID: info.GetExecution().GetWorkflowId(),
		RunID:      info.GetExecution().GetRunId(),
		Status:     mapTemporalStatus(info.GetStatus()),
	}
	if ts := info.GetStartTime(); ts != nil {
		t := ts.AsTime()
		summary.StartTime = &t
	}
	if ts := info.GetCloseTime(); ts != nil {
		t := ts.AsTime()
		summary.CloseTime = &t
	}
	return summary, nil
}

func (c *temporalWorkflowControl) Signal(ctx context.Context, ref workflowctl.ExecutionRef, signalName string, payload any) error {
	if c == nil || c.client == nil {
		return workflowctl.ErrUnavailable
	}
	return mapTemporalError(c.client.SignalWorkflow(ctx, ref.WorkflowID, ref.RunID, signalName, payload))
}

func (c *temporalWorkflowControl) Cancel(ctx context.Context, ref workflowctl.ExecutionRef, reason string) error {
	if c == nil || c.client == nil {
		return workflowctl.ErrUnavailable
	}
	return mapTemporalError(c.client.CancelWorkflow(ctx, ref.WorkflowID, ref.RunID))
}

func mapTemporalStatus(status enumspb.WorkflowExecutionStatus) workflowctl.WorkflowStatus {
	switch status {
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return workflowctl.StatusCompleted
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
		return workflowctl.StatusFailed
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return workflowctl.StatusCanceled
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return workflowctl.StatusTerminated
	case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return workflowctl.StatusTimedOut
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return workflowctl.StatusRunning
	default:
		return workflowctl.StatusUnspecified
	}
}

func mapTemporalError(err error) error {
	switch err.(type) {
	case *serviceerror.NotFound:
		return workflowctl.ErrNotFound
	case *serviceerror.Unavailable:
		return workflowctl.ErrUnavailable
	default:
		return err
	}
}
