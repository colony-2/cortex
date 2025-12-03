package worker

import (
	"context"
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/google/uuid"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	temporalconverter "go.temporal.io/sdk/converter"
	"gorm.io/gorm"
)

func newWorkerDependencies(db *gorm.DB) ops.ServiceDependencies2 {
	builder := ops.NewServiceDepsBuilder().
		WithSSEManager(noopSSEManager{}).
		WithTemporalNamespace(namespace).
		WithDatabase(db)
	if cli != nil {
		ctl := &temporalWorkflowControl{client: cli, namespace: namespace}
		builder = builder.WithWorkflowControl(ctl)
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
	return buildWorkflowSummary(info), nil
}

func (c *temporalWorkflowControl) ListWorkflows(ctx context.Context, req workflowctl.ListWorkflowsRequest) (workflowctl.ListWorkflowsResponse, error) {
	if c == nil || c.client == nil {
		return workflowctl.ListWorkflowsResponse{}, workflowctl.ErrUnavailable
	}
	listReq := &workflowservice.ListWorkflowExecutionsRequest{
		Namespace:     c.namespace,
		PageSize:      req.PageSize,
		Query:         req.Query,
		NextPageToken: req.NextPageToken,
	}
	resp, err := c.client.WorkflowService().ListWorkflowExecutions(ctx, listReq)
	if err != nil {
		return workflowctl.ListWorkflowsResponse{}, mapTemporalError(err)
	}
	executions := make([]workflowctl.WorkflowSummary, 0, len(resp.GetExecutions()))
	for _, info := range resp.GetExecutions() {
		if info == nil {
			continue
		}
		executions = append(executions, buildWorkflowSummary(info))
	}
	return workflowctl.ListWorkflowsResponse{
		Executions:    executions,
		NextPageToken: resp.GetNextPageToken(),
	}, nil
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

func (c *temporalWorkflowControl) ResetWorkflow(ctx context.Context, req workflowctl.ResetRequest) (workflowctl.ResetResponse, error) {
	if c == nil || c.client == nil {
		return workflowctl.ResetResponse{}, workflowctl.ErrUnavailable
	}
	if req.Execution.WorkflowID == "" {
		return workflowctl.ResetResponse{}, fmt.Errorf("workflow id is required for reset")
	}
	if req.WorkflowTaskFinishEventID <= 0 {
		return workflowctl.ResetResponse{}, fmt.Errorf("workflow task finish event id must be positive")
	}
	resetReq := &workflowservice.ResetWorkflowExecutionRequest{
		Namespace: c.namespace,
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: req.Execution.WorkflowID,
			RunId:      req.Execution.RunID,
		},
		Reason:                    req.Reason,
		WorkflowTaskFinishEventId: req.WorkflowTaskFinishEventID,
		RequestId:                 uuid.NewString(),
		ResetReapplyType:          enumspb.RESET_REAPPLY_TYPE_SIGNAL,
	}
	resp, err := c.client.WorkflowService().ResetWorkflowExecution(ctx, resetReq)
	if err != nil {
		return workflowctl.ResetResponse{}, mapTemporalError(err)
	}
	newRunID := resp.GetRunId()
	if newRunID == "" {
		newRunID = req.Execution.RunID
	}
	result := workflowctl.ResetResponse{
		Execution: workflowctl.ExecutionRef{WorkflowID: req.Execution.WorkflowID, RunID: newRunID},
	}
	if req.WaitForResult {
		workflowRun := c.client.GetWorkflow(ctx, req.Execution.WorkflowID, newRunID)
		if workflowRun == nil {
			return workflowctl.ResetResponse{}, workflowctl.ErrUnavailable
		}
		var payload map[string]interface{}
		if err := workflowRun.Get(ctx, &payload); err != nil {
			return workflowctl.ResetResponse{}, mapTemporalError(err)
		}
		if payload == nil {
			payload = make(map[string]interface{})
		}
		result.Completed = true
		result.Result = payload
	}
	return result, nil
}

func (c *temporalWorkflowControl) StartWorkflow(ctx context.Context, req workflowctl.StartRequest) (workflowctl.StartResponse, error) {
	return workflowctl.StartResponse{}, workflowctl.ErrUnavailable
}

func (c *temporalWorkflowControl) StartChildWorkflow(ctx context.Context, req workflowctl.StartChildRequest) (workflowctl.StartChildResponse, error) {
	return workflowctl.StartChildResponse{}, workflowctl.ErrUnavailable
}

func buildWorkflowSummary(info *workflowpb.WorkflowExecutionInfo) workflowctl.WorkflowSummary {
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
	if attrs := info.GetSearchAttributes(); attrs != nil && len(attrs.IndexedFields) > 0 {
		summary.SearchAttributes = decodeSearchAttributes(attrs.IndexedFields)
	}
	return summary
}

func decodeSearchAttributes(indexed map[string]*commonpb.Payload) map[string]any {
	if len(indexed) == 0 {
		return nil
	}
	dc := temporalconverter.GetDefaultDataConverter()
	result := make(map[string]any, len(indexed))
	for key, payload := range indexed {
		if payload == nil {
			continue
		}
		var value any
		if err := dc.FromPayload(payload, &value); err != nil {
			result[key] = fmt.Sprintf("<decode error: %v>", err)
			continue
		}
		result[key] = value
	}
	return result
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
