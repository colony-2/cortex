package temporalharness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	embeddedtemporal "github.com/divisive-ai/vibethis/server/embeddedtemporal/pkg/temporal"
	inputops "github.com/divisive-ai/vibethis/server/ops/pkg/input"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	runmetadata "github.com/divisive-ai/vibethis/server/recipe-core/pkg/runmetadata"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	recipeworker "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	operatorservicepb "go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	temporalconverter "go.temporal.io/sdk/converter"
	"go.uber.org/zap/zaptest"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Harness spins up an embedded Temporal server and the real recipe worker for integration tests.
type Harness struct {
	t          *testing.T
	server     *embeddedtemporal.Server
	client     client.Client
	worker     *recipeworker.Worker
	namespace  string
	recipesDir string
	recipeID   string
	hostPort   string
}

const (
	workflowStartTimeout = 10 * time.Second
	describePollInterval = 50 * time.Millisecond
)

var registerOpsOnce sync.Once

// Start launches an embedded Temporal server, starts the recipe worker, and registers a test recipe.
func Start(t *testing.T) *Harness {
	t.Helper()

	registerOpsOnce.Do(func() {
		coreops.Register(inputops.GetOp())
	})

	namespace := fmt.Sprintf("harness-%d", time.Now().UnixNano())
	recipesDir := t.TempDir()
	recipeID := fmt.Sprintf("input-harness-%d", time.Now().UnixNano())
	recipePath := filepath.Join(recipesDir, fmt.Sprintf("%s.yaml", recipeID))

	writeTestRecipe(t, recipePath, recipeID)

	frontendPort := embeddedtemporal.FindFreePort()
	hostPort := fmt.Sprintf("127.0.0.1:%d", frontendPort)
	dbDir := t.TempDir()
	dbFile := filepath.Join(dbDir, "temporal.db")

	serverOpts := embeddedtemporal.Options{
		FrontendIP:               "127.0.0.1",
		FrontendPort:             frontendPort,
		DatabaseFile:             dbFile,
		Namespaces:               []string{namespace},
		LogLevel:                 "error",
		DisableScanners:          true,
		DisableParentClosePolicy: true,
		DisableNexus:             true,
	}

	srv, err := embeddedtemporal.NewServer(serverOpts)
	require.NoError(t, err, "create embedded temporal server")
	require.NoError(t, srv.Start(), "start embedded temporal")
	time.Sleep(500 * time.Millisecond)

	h := &Harness{
		t:          t,
		server:     srv,
		namespace:  namespace,
		recipesDir: recipesDir,
		recipeID:   recipeID,
		hostPort:   hostPort,
	}

	t.Cleanup(func() {
		_ = h.Stop()
	})

	ensureNamespaceExists(t, hostPort, namespace)

	cli, err := embeddedtemporal.NewClient(embeddedtemporal.ClientOptions{HostPort: hostPort, Namespace: namespace})
	require.NoError(t, err, "create temporal client")
	h.client = cli
	waitForNamespaceVisibility(t, cli, namespace)
	time.Sleep(500 * time.Millisecond)
	ensureInputSearchAttributesRegistered(t, cli, namespace)

	logger := zaptest.NewLogger(t)

	workerInstance, err := recipeworker.NewWorker(logger, recipesDir, cli, namespace)
	require.NoError(t, err, "create recipe worker")
	h.worker = workerInstance

	require.NoError(t, workerInstance.Start(), "start recipe worker")

	h.waitForWorkerReady()
	time.Sleep(200 * time.Millisecond)

	return h
}

// Stop terminates the worker, client, and embedded server.
func (h *Harness) Stop() error {
	if h.worker != nil {
		_ = h.worker.Stop()
		h.worker = nil
	}
	if h.client != nil {
		h.client.Close()
		h.client = nil
	}
	if h.server != nil {
		_ = h.server.Stop()
		h.server = nil
	}
	return nil
}

func (h *Harness) waitForWorkerReady() {
	deadline := time.Now().Add(10 * time.Second)
	registry := h.worker.GetRegistry()
	for {
		recipeFile, err := registry.GetRecipe(h.recipeID)
		if err == nil && recipeFile.WorkerStatus == recipe.WorkerStatusRunning {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("recipe worker for %s did not become ready: %v", h.recipeID, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// WorkflowControl returns a Temporal client-backed workflow controller.
func (h *Harness) WorkflowControl() workflowctl.WorkflowControl {
	return &harnessWorkflowControl{client: h.client, namespace: h.namespace}
}

// ServiceDependencies builds ServiceDependencies2 with the provided SSE manager.
func (h *Harness) ServiceDependencies(sse coreops.SSEManager) coreops.ServiceDependencies2 {
	builder := coreops.NewServiceDepsBuilder().WithTemporalNamespace(h.namespace)
	if sse != nil {
		builder = builder.WithSSEManager(sse)
	}
	builder = builder.WithWorkflowControl(h.WorkflowControl())
	return builder.Build()
}

// StartWorkflow launches the registered recipe workflow with provided inputs and returns the workflow ID.
func (h *Harness) StartWorkflow(t *testing.T, inputs map[string]interface{}) string {
	t.Helper()
	if inputs == nil {
		inputs = make(map[string]interface{})
	}
	if _, ok := inputs["basegitrepo"]; !ok {
		inputs["basegitrepo"] = "git@local/test.git"
	}
	if _, ok := inputs["basegithash"]; !ok {
		inputs["basegithash"] = "0000000000000000000000000000000000000000"
	}
	if _, ok := inputs["ticketid"]; !ok {
		inputs["ticketid"] = "TEST-1"
	}
	if _, ok := inputs["cellname"]; !ok {
		if v, ok := inputs["box_id"].(string); ok && v != "" {
			inputs["cellname"] = v
		} else {
			inputs["cellname"] = "cell-harness"
		}
	}
	workflowID := fmt.Sprintf("%s-run-%d", h.recipeID, time.Now().UnixNano())
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: h.worker.GetWorkerManager().GetTaskQueueForRecipe(h.recipeID),
	}

	ctx, cancel := context.WithTimeout(context.Background(), workflowStartTimeout)
	defer cancel()

	run, err := h.client.ExecuteWorkflow(ctx, options, h.recipeID+"-workflow", inputs)
	require.NoError(t, err, "execute workflow")
	runID := run.GetRunID()
	require.NotEmpty(t, runID, "workflow run id")

	signalCtx, cancelSignal := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSignal()
	metadata := runmetadata.Signal{TargetRunID: runID}
	require.NoError(t, h.client.SignalWorkflow(signalCtx, workflowID, runID, "recipe_run_metadata", metadata), "signal metadata")

	return workflowID
}

// WaitForSearchAttribute waits until Describe reports the given search attribute value.
func (h *Harness) WaitForSearchAttribute(t *testing.T, workflowID, key, expected string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	ref := workflowctl.ExecutionRef{WorkflowID: workflowID}
	for {
		summary, err := h.WorkflowControl().Describe(context.Background(), ref)
		if err == nil {
			if val, ok := summary.SearchAttributes[key]; ok {
				if s, ok := val.(string); ok && strings.EqualFold(s, expected) {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("search attribute %s not %s for workflow %s", key, expected, workflowID)
		}
		time.Sleep(describePollInterval)
	}
}

// WaitForStatus waits until the workflow reaches the desired status.
func (h *Harness) WaitForStatus(t *testing.T, workflowID string, status workflowctl.WorkflowStatus) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	ref := workflowctl.ExecutionRef{WorkflowID: workflowID}
	for {
		summary, err := h.WorkflowControl().Describe(context.Background(), ref)
		if err == nil && summary.Status == status {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("workflow %s did not reach status %s", workflowID, status)
		}
		time.Sleep(describePollInterval)
	}
}

// DumpHistory logs the event types for the workflow's execution history (best-effort).
func (h *Harness) DumpHistory(t *testing.T, workflowID string) {
	t.Helper()
	if h.client == nil {
		t.Logf("harness has no temporal client; cannot dump history for %s", workflowID)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var token []byte
	for {
		resp, err := h.client.WorkflowService().GetWorkflowExecutionHistory(ctx, &workflowservice.GetWorkflowExecutionHistoryRequest{
			Namespace:     h.namespace,
			Execution:     &commonpb.WorkflowExecution{WorkflowId: workflowID},
			NextPageToken: token,
		})
		if err != nil {
			t.Logf("history fetch failed for %s: %v", workflowID, err)
			return
		}
		if resp.GetHistory() != nil {
			events := resp.GetHistory().GetEvents()
			t.Logf("history page events=%d next=%t", len(events), len(resp.GetNextPageToken()) > 0)
			for _, evt := range events {
				t.Logf("history event %d: %s", evt.GetEventId(), evt.GetEventType())
			}
		}
		if len(resp.GetNextPageToken()) == 0 {
			break
		}
		token = resp.GetNextPageToken()
	}
}

// HostPort returns the Temporal frontend address.
func (h *Harness) HostPort() string {
	return h.hostPort
}

// Namespace returns the Temporal namespace used by the harness.
func (h *Harness) Namespace() string {
	return h.namespace
}

func writeTestRecipe(t *testing.T, path, id string) {
	t.Helper()
	recipe := fmt.Sprintf(`id: %s
version: '1.0'
desc: Harness input recipe
input_schema:
  box_id:
    type: string
    required: true
sequence:
  - id: gather
    op: input
    inputs:
      box_id: '{{ inputs.box_id }}'
      activity_id: harness-approval
      config:
        question: 'Approve?'
        type: multiple_choice
        options:
          - label: Approve
            value: yes
          - label: Reject
            value: no
        timeout: 60
outputs:
  fields: '{{ sequence.gather.outputs.fields }}'
`, id)
	require.NoError(t, os.WriteFile(path, []byte(recipe), 0644))
}

func ensureNamespaceExists(t *testing.T, hostPort, namespace string) {
	if namespace == "" {
		return
	}
	nc, err := client.NewNamespaceClient(client.Options{HostPort: hostPort})
	require.NoError(t, err, "create namespace client")
	defer nc.Close()

	deadline := time.Now().Add(10 * time.Second)
	registered := false
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := nc.Describe(ctx, namespace)
		cancel()
		if err == nil {
			return
		}
		if _, ok := err.(*serviceerror.NamespaceNotFound); ok {
			if !registered {
				ctxReg, cancelReg := context.WithTimeout(context.Background(), 3*time.Second)
				err := nc.Register(ctxReg, &workflowservice.RegisterNamespaceRequest{
					Namespace:                        namespace,
					WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour),
				})
				cancelReg()
				if err != nil {
					if _, exists := err.(*serviceerror.NamespaceAlreadyExists); !exists {
						require.NoError(t, err, "register namespace")
					}
				}
				registered = true
			}
		} else {
			require.NoError(t, err, "describe namespace")
		}
		if time.Now().After(deadline) {
			t.Fatalf("namespace %s not ready", namespace)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func waitForNamespaceVisibility(t *testing.T, cli client.Client, namespace string) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := cli.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{Namespace: namespace})
		cancel()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("namespace %s not visible to client", namespace)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func ensureInputSearchAttributesRegistered(t *testing.T, cli client.Client, namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	attributes := map[string]enumspb.IndexedValueType{
		"InputKey":         enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputStatus":      enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputFormTitle":   enumspb.INDEXED_VALUE_TYPE_TEXT,
		"InputBoxID":       enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputActivityID":  enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputCreatedAt":   enumspb.INDEXED_VALUE_TYPE_DATETIME,
		"InputExpiresAt":   enumspb.INDEXED_VALUE_TYPE_DATETIME,
		"InputRespondedBy": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"InputRespondedAt": enumspb.INDEXED_VALUE_TYPE_DATETIME,
	}

	_, err := cli.OperatorService().AddSearchAttributes(ctx, &operatorservicepb.AddSearchAttributesRequest{
		Namespace:        namespace,
		SearchAttributes: attributes,
	})
	if err != nil {
		var invalid *serviceerror.InvalidArgument
		var unimpl *serviceerror.Unimplemented
		switch {
		case errors.As(err, &invalid):
			t.Logf("register search attributes skipped: %v", err)
		case errors.As(err, &unimpl):
			t.Logf("register search attributes unsupported: %v", err)
		default:
			require.NoError(t, err, "register search attributes")
		}
	}

	listResp, err := cli.OperatorService().ListSearchAttributes(ctx, &operatorservicepb.ListSearchAttributesRequest{Namespace: namespace})
	if err != nil {
		t.Logf("list search attributes: %v", err)
	} else {
		t.Logf("custom search attributes: %v", listResp.CustomAttributes)
		for key := range attributes {
			if _, ok := listResp.CustomAttributes[key]; !ok {
				t.Logf("search attribute %s not present after registration", key)
			}
		}
	}

	expectedLabels := map[string]struct{}{
		"InputKey":         {},
		"InputStatus":      {},
		"InputFormTitle":   {},
		"InputBoxID":       {},
		"InputActivityID":  {},
		"InputCreatedAt":   {},
		"InputExpiresAt":   {},
		"InputRespondedBy": {},
		"InputRespondedAt": {},
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctxDesc, cancelDesc := context.WithTimeout(context.Background(), time.Second)
		desc, derr := cli.WorkflowService().DescribeNamespace(ctxDesc, &workflowservice.DescribeNamespaceRequest{Namespace: namespace})
		cancelDesc()
		if derr == nil && desc != nil && desc.Config != nil {
			aliases := desc.Config.CustomSearchAttributeAliases
			remaining := make(map[string]struct{}, len(expectedLabels))
			for label := range expectedLabels {
				remaining[label] = struct{}{}
			}
			for _, alias := range aliases {
				delete(remaining, alias)
			}
			if len(remaining) == 0 {
				t.Logf("namespace alias mapping ready: %v", aliases)
				time.Sleep(200 * time.Millisecond)
				break
			}
			t.Logf("waiting for search attribute aliases: current=%v", aliases)
		}

		if time.Now().After(deadline) {
			t.Fatalf("namespace %s did not register input search attribute aliases", namespace)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// harnessWorkflowControl mirrors the recipe worker Temporal workflow control implementation.
type harnessWorkflowControl struct {
	client    client.Client
	namespace string
}

func (c *harnessWorkflowControl) Describe(ctx context.Context, ref workflowctl.ExecutionRef) (workflowctl.WorkflowSummary, error) {
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

func (c *harnessWorkflowControl) ListWorkflows(ctx context.Context, req workflowctl.ListWorkflowsRequest) (workflowctl.ListWorkflowsResponse, error) {
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

func (c *harnessWorkflowControl) Signal(ctx context.Context, ref workflowctl.ExecutionRef, signalName string, payload any) error {
	if c == nil || c.client == nil {
		return workflowctl.ErrUnavailable
	}
	return mapTemporalError(c.client.SignalWorkflow(ctx, ref.WorkflowID, ref.RunID, signalName, payload))
}

func (c *harnessWorkflowControl) Cancel(ctx context.Context, ref workflowctl.ExecutionRef, reason string) error {
	if c == nil || c.client == nil {
		return workflowctl.ErrUnavailable
	}
	return mapTemporalError(c.client.CancelWorkflow(ctx, ref.WorkflowID, ref.RunID))
}

func (c *harnessWorkflowControl) ResetWorkflow(ctx context.Context, req workflowctl.ResetRequest) (workflowctl.ResetResponse, error) {
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
		RequestId:                 fmt.Sprintf("reset-%d", time.Now().UnixNano()),
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

func (c *harnessWorkflowControl) StartWorkflow(ctx context.Context, req workflowctl.StartRequest) (workflowctl.StartResponse, error) {
	return workflowctl.StartResponse{}, workflowctl.ErrUnavailable
}

func (c *harnessWorkflowControl) StartChildWorkflow(ctx context.Context, req workflowctl.StartChildRequest) (workflowctl.StartChildResponse, error) {
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
