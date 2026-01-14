package input

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	ops2 "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/fatih/structs"
	"github.com/gorilla/mux"
)

// inputManagementService implements ManagementService for input activities
type inputManagementService struct {
	workflowType string
	sse          ops.SSEManager
	ctl          workflowctl.WorkflowControl
}

// newInputManagementService creates a new input management service
func newInputManagementService() *inputManagementService {
	return &inputManagementService{
		workflowType: "InputCollectionWorkflow",
	}
}

// Initialize sets up the service with dependencies
func (s *inputManagementService) Initialize(deps ops.ServiceDependencies2) error {
	// Require a typed WorkflowControl; fail if not provided
	s.ctl = deps.WorkflowControl()
	s.sse = deps.SSEManager()

	if s.ctl == nil {
		return fmt.Errorf("workflow control must be provided")
	}
	if s.sse == nil {
		return fmt.Errorf("sse manager must be provided")
	}
	return nil
}

func (s *inputManagementService) Close() {
	// No-op: service depends on external WorkflowControl lifetime
}

// GetRoutes returns the HTTP routes provided by this service
func (s *inputManagementService) GetRoutes() []ops.Route {
	return []ops.Route{
		{Method: "GET", Path: "/api/projects/{projectId}/user-inputs/pending", Handler: s.ListPending},
		{Method: "GET", Path: "/api/projects/{projectId}/user-inputs/stream", Handler: s.SSEStream},
		{Method: "GET", Path: "/api/projects/{projectId}/user-inputs/{jobId}", Handler: s.GetDetails},
		{Method: "POST", Path: "/api/projects/{projectId}/user-inputs/{jobId}/respond", Handler: s.SubmitResponse},
		{Method: "POST", Path: "/api/projects/{projectId}/user-inputs/{jobId}/cancel", Handler: s.Cancel},
	}
}

// ListPending returns all pending input requests
func (s *inputManagementService) ListPending(w http.ResponseWriter, r *http.Request) {
	slog.Info("list_pending: request received", "method", r.Method, "url", r.URL.Path)

	vars := mux.Vars(r)
	projectID := vars["projectId"]
	if projectID == "" {
		slog.Warn("list_pending: missing projectId parameter", "url", r.URL.Path, "vars", vars)
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	slog.Info("list_pending: collecting pending inputs", "project_id", projectID)
	pending, err := s.collectPendingInputs(r.Context(), projectID)
	if err != nil {
		slog.Error("list_pending: failed to collect pending inputs",
			"project_id", projectID,
			"error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("list_pending: success", "project_id", projectID, "count", len(pending))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(pending); err != nil {
		slog.Error("list_pending: failed to encode response", "project_id", projectID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *inputManagementService) GetDetails(w http.ResponseWriter, r *http.Request) {
	slog.Info("get_details: request received", "method", r.Method, "url", r.URL.Path)

	vars := mux.Vars(r)
	projectID := vars["projectId"]
	if projectID == "" {
		slog.Warn("get_details: missing projectId parameter", "url", r.URL.Path, "vars", vars)
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	jobId := vars["jobId"]
	if jobId == "" {
		slog.Warn("get_details: missing jobId parameter", "url", r.URL.Path, "project_id", projectID, "vars", vars)
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}

	slog.Info("get_details: fetching job details", "project_id", projectID, "job_id", jobId)
	res := s.getDetails(r.Context(), projectID, jobId)
	if res.sendError(w) {
		slog.Error("get_details: failed to get details", "project_id", projectID, "job_id", jobId, "error", res.err)
		return
	}
	d := res.value
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(d); err != nil {
		slog.Error("get_details: failed to encode response", "project_id", projectID, "job_id", jobId, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

}

type detailsInput struct {
	JobKey    swf.JobKey `json:"jobKey"`
	Status    swf.JobStatus
	StartTime time.Time                   `json:"startTime"`
	Form      InputForm                   `json:"form"`
	Git       contextual.GitCommitContext `json:"git"`
}

func (s *inputManagementService) getOutput(ctx context.Context, projectID string, jobId string) (workflowctl.TaskHandle, ops2.ActivityInvocationOutput, []swf.Artifact, error) {
	if s.ctl == nil {
		return nil, ops2.ActivityInvocationOutput{}, nil, errors.New("workflow control unavailable")
	}

	task, err := s.ctl.GetWaitingTask(ctx, swf.JobKey{
		TenantId: projectID,
		JobId:    jobId,
	})

	if err != nil {
		return nil, ops2.ActivityInvocationOutput{}, nil, fmt.Errorf("failed to find job: %w", err)
	}

	td, err := task.Data()
	if err != nil {
		return nil, ops2.ActivityInvocationOutput{}, nil, fmt.Errorf("failed to get task data: %w", err)
	}

	data, err := td.GetData()
	if err != nil {
		return nil, ops2.ActivityInvocationOutput{}, nil, fmt.Errorf("failed to get task data: %w", err)
	}

	artifacts, err := td.GetArtifacts()
	if err != nil {
		return nil, ops2.ActivityInvocationOutput{}, nil, fmt.Errorf("failed to get task artifacts: %w", err)
	}

	req := ops2.ActivityInvocationOutput{}
	err = json.Unmarshal(data, &req)
	if err != nil {
		return nil, ops2.ActivityInvocationOutput{}, nil, fmt.Errorf("failed to unmarshal task data: %w", err)
	}

	return task, req, artifacts, nil
}

// GetDetails returns details about a specific input request
func (s *inputManagementService) getDetails(ctx context.Context, projectID string, jobId string) result[*detailsInput] {
	task, req, _, err := s.getOutput(ctx, projectID, jobId)
	form := InputForm{}
	err = ops.DecodeWithJsonTags(req.OpOutput, &form)
	if err != nil {
		return result[*detailsInput]{err: err.Error()}
	}

	return result[*detailsInput]{value: &detailsInput{
		JobKey:    task.JobKey(),
		Status:    swf.JobStatusReady,
		StartTime: time.Time{},
		Form:      form,
		Git:       req.GitResult,
	}}
}

func (s *inputManagementService) findJob(ctx context.Context, projectID string, jobId string) result[*workflowctl.JobItem] {
	slog.Info("find_job: searching for job", "project_id", projectID, "job_id", jobId)

	// Query with TenantId filter and specific JobKey, only for jobs waiting for input
	jobs, _, err := s.ctl.ListJobs(ctx, swf.ListJobsRequest{
		Stores:    []swf.JobStore{swf.JobStoreActive},
		TenantIds: []string{projectID},
		JobKeys:   []swf.JobKey{{TenantId: projectID, JobId: jobId}},
		Statuses:  []swf.JobStatus{swf.JobStatusReady},
		JobTasks: []swf.JobTaskFilter{{
			JobType:  "recipe",
			TaskType: "input:collect_user_input",
		}},
	})
	if err != nil {
		slog.Error("find_job: query failed",
			"project_id", projectID,
			"job_id", jobId,
			"error", err)
		return result[*workflowctl.JobItem]{
			err: fmt.Errorf("failed to query workflow: %w", err).Error(),
		}
	}

	if len(jobs) == 0 {
		slog.Warn("find_job: job not found",
			"project_id", projectID,
			"job_id", jobId)
		return result[*workflowctl.JobItem]{
			err:    "not found",
			status: http.StatusNotFound,
		}
	}

	// Validate project ownership
	job := jobs[0]
	if job.JobKey.TenantId != projectID {
		slog.Warn("find_job: project mismatch",
			"project_id", projectID,
			"job_tenant_id", job.JobKey.TenantId,
			"job_id", jobId)
		return result[*workflowctl.JobItem]{
			err:    "workflow not found in project",
			status: http.StatusNotFound,
		}
	}

	slog.Info("find_job: found job", "project_id", projectID, "job_id", jobId)
	return result[*workflowctl.JobItem]{
		value: &job,
	}
}

type result[T any] struct {
	value  T
	err    string
	status int // HTTP Status Code to be used by the handler
}

func (res result[T]) hasError() bool {
	return res.err != ""
}

func (res result[T]) sendError(w http.ResponseWriter) bool {
	if res.err != "" {
		if res.status == 0 {
			res.status = http.StatusInternalServerError
		}
		http.Error(w, res.err, res.status)
		return true
	}
	return false
}

func (s *inputManagementService) SubmitResponse(w http.ResponseWriter, r *http.Request) {
	slog.Info("submit_response: request received", "method", r.Method, "url", r.URL.Path)

	vars := mux.Vars(r)
	projectID := vars["projectId"]
	if projectID == "" {
		slog.Warn("submit_response: missing projectId parameter", "url", r.URL.Path, "vars", vars)
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	jobId := vars["jobId"]
	if jobId == "" {
		slog.Warn("submit_response: missing jobId parameter", "url", r.URL.Path, "project_id", projectID, "vars", vars)
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}

	slog.Info("submit_response: decoding request body", "project_id", projectID, "job_id", jobId)
	output := FormResponse{}
	if err := json.NewDecoder(r.Body).Decode(&output); err != nil {
		slog.Warn("submit_response: invalid request body", "project_id", projectID, "job_id", jobId, "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if output.Fields == nil {
		slog.Warn("submit_response: missing fields in request", "project_id", projectID, "job_id", jobId)
		http.Error(w, "fields are required", http.StatusBadRequest)
		return
	}

	if output.UserID == "" {
		output.UserID = r.Header.Get("X-User-ID")
		slog.Info("submit_response: using user ID from header", "project_id", projectID, "job_id", jobId, "user_id", output.UserID)
	} else {
		slog.Info("submit_response: using user ID from body", "project_id", projectID, "job_id", jobId, "user_id", output.UserID)
	}

	slog.Info("submit_response: submitting response", "project_id", projectID, "job_id", jobId, "field_count", len(output.Fields))
	res := s.submitResponse(r.Context(), projectID, jobId, output)
	if res.sendError(w) {
		slog.Error("submit_response: failed to submit response", "project_id", projectID, "job_id", jobId, "error", res.err)
		return
	}
	// Return success response
	slog.Info("submit_response: success", "project_id", projectID, "job_id", jobId)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": res.value})
}

// SubmitResponse handles user form submission
func (s *inputManagementService) submitResponse(ctx context.Context, projectID string, jobId string, output FormResponse) result[bool] {
	slog.Debug("submitResponse: starting", "project_id", projectID, "job_id", jobId)

	task, req, artifacts, err := s.getOutput(ctx, projectID, jobId)
	if err != nil {
		return result[bool]{err: err.Error()}
	}
	// Get user ID preference: body overrides header, fallback to default
	userID := output.UserID
	if userID == "" {
		userID = "anonymous"
		slog.Info("submitResponse: using anonymous user ID", "project_id", projectID, "job_id", jobId)
	}

	opOut := Output{
		Response: output.Response,
		Fields:   output.Fields,
		UserID:   userID,
	}

	sConv := structs.New(opOut)
	sConv.TagName = "json"
	opOutMap := sConv.Map()

	out := ops2.ActivityInvocationOutput{
		GitResult: req.GitResult,
		OpOutput:  opOutMap,
	}
	outData, error := swf.NewTaskData(out, artifacts...)
	if error != nil {
		return result[bool]{err: error.Error()}
	}

	err = task.Finish(ctx, outData)
	if err != nil {
		return result[bool]{err: err.Error()}
	}

	// Notify via SSE if available
	if s.sse == nil {
		slog.Warn("submitResponse: SSE subsystem missing", "project_id", projectID, "job_id", jobId)
		return result[bool]{err: "sse subsystem missing"}
	}
	slog.Info("submitResponse: successfully completed", "project_id", projectID, "job_id", jobId)
	return result[bool]{value: true}
}

// Cancel handles cancellation of a pending input request
func (s *inputManagementService) Cancel(w http.ResponseWriter, r *http.Request) {
	slog.Info("cancel: request received", "method", r.Method, "url", r.URL.Path)

	if s.ctl == nil {
		slog.Error("cancel: workflow control unavailable")
		http.Error(w, "workflow control unavailable", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	projectID := vars["projectId"]
	if projectID == "" {
		slog.Warn("cancel: missing projectId parameter", "url", r.URL.Path, "vars", vars)
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	jobId := vars["jobId"]
	if jobId == "" {
		slog.Warn("cancel: missing jobId parameter", "url", r.URL.Path, "project_id", projectID, "vars", vars)
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}

	// Parse cancellation request
	var cancelRequest struct {
		Reason string `json:"reason"`
	}

	slog.Info("cancel: decoding cancellation request", "project_id", projectID, "job_id", jobId)
	if err := json.NewDecoder(r.Body).Decode(&cancelRequest); err != nil {
		slog.Warn("cancel: invalid request body", "project_id", projectID, "job_id", jobId, "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	slog.Info("cancel: attempting to cancel job", "project_id", projectID, "job_id", jobId, "reason", cancelRequest.Reason)
	err := s.ctl.Cancel(r.Context(), swf.JobKey{TenantId: projectID, JobId: jobId})
	if err != nil {
		slog.Error("cancel: cancel failed",
			"project_id", projectID,
			"job_id", jobId,
			"error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("cancel: successfully cancelled job", "project_id", projectID, "job_id", jobId)

	// Notify via SSE if available
	if s.sse != nil {
		s.sse.Broadcast(ops.SSEEvent{
			Type: "input_cancelled",
			Data: map[string]interface{}{
				"jobId":  jobId,
				"reason": cancelRequest.Reason,
			},
		})
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// SSEStream handles Server-Sent Events streaming for real-time updates
func (s *inputManagementService) SSEStream(w http.ResponseWriter, r *http.Request) {
	slog.Info("sse_stream: request received",
		"method", r.Method,
		"url", r.URL.Path,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	vars := mux.Vars(r)
	projectID := vars["projectId"]
	if projectID == "" {
		slog.Warn("sse_stream: missing projectId parameter",
			"url", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"vars", vars)
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	slog.Info("sse_stream: validated projectId", "project_id", projectID)

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	slog.Info("sse_stream: set SSE headers", "project_id", projectID)

	// Create a client ID
	clientID := r.Header.Get("X-Client-ID")
	if clientID == "" {
		clientID = generateClientID()
		slog.Info("sse_stream: generated client ID", "project_id", projectID, "client_id", clientID)
	} else {
		slog.Info("sse_stream: using provided client ID", "project_id", projectID, "client_id", clientID)
	}

	// Subscribe to events if SSE manager is available
	if s.sse == nil {
		slog.Error("sse_stream: SSE manager not configured", "project_id", projectID, "client_id", clientID)
		// No SSE manager, just send a heartbeat and close
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"SSE not configured\"}\n\n")
		return
	}

	slog.Info("sse_stream: subscribing to events", "project_id", projectID, "client_id", clientID)
	events := s.sse.Subscribe(clientID)
	defer func() {
		slog.Info("sse_stream: unsubscribing client", "project_id", projectID, "client_id", clientID)
		s.sse.Unsubscribe(clientID)
	}()

	// Send initial connection event
	slog.Info("sse_stream: sending connection event", "project_id", projectID, "client_id", clientID)
	fmt.Fprintf(w, "event: connected\ndata: {\"client_id\": \"%s\"}\n\n", clientID)
	flush(w)

	// Emit snapshot of current pending inputs so subscribers have immediate context.
	slog.Info("sse_stream: collecting pending inputs snapshot", "project_id", projectID, "client_id", clientID)
	if pending, err := s.collectPendingInputs(r.Context(), projectID); err != nil {
		slog.Error("sse_stream: failed to collect pending inputs snapshot",
			"project_id", projectID,
			"client_id", clientID,
			"error", err)
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"failed to load pending inputs\"}\n\n")
		flush(w)
	} else {
		slog.Info("sse_stream: sending pending inputs snapshot",
			"project_id", projectID,
			"client_id", clientID,
			"count", len(pending))
		for _, item := range pending {
			payload, err := json.Marshal(item)
			if err != nil {
				slog.Error("sse_stream: failed to marshal pending input",
					"project_id", projectID,
					"client_id", clientID,
					"job_id", item.JobID,
					"error", err)
				continue
			}
			fmt.Fprintf(w, "event: input_pending\ndata: %s\n\n", payload)
		}
		flush(w)
		slog.Info("sse_stream: finished sending pending inputs", "project_id", projectID, "client_id", clientID)
	}

	// Create a ticker for heartbeat
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	slog.Info("sse_stream: entering event loop", "project_id", projectID, "client_id", clientID)

	// Stream events
	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			slog.Info("sse_stream: client disconnected",
				"project_id", projectID,
				"client_id", clientID,
				"context_error", r.Context().Err())
			return

		case event := <-events:
			// Send event to client
			slog.Info("sse_stream: sending event to client",
				"project_id", projectID,
				"client_id", clientID,
				"event_type", event.Type)
			data, _ := json.Marshal(event.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
			flush(w)

		case <-ticker.C:
			// Send heartbeat
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"timestamp\": \"%s\"}\n\n", time.Now().Format(time.RFC3339))
			flush(w)
		}
	}
}

const pendingStatusQuery = "InputStatus = \"pending\""

func (s *inputManagementService) collectPendingInputs(ctx context.Context, projectID string) ([]PendingInput, error) {
	// TODO: explore supporting pagination.
	jobs, _, err := s.ctl.ListJobs(ctx, swf.ListJobsRequest{
		Stores:    []swf.JobStore{swf.JobStoreActive},
		TenantIds: []string{projectID},
		Statuses:  []swf.JobStatus{swf.JobStatusReady},
		JobTasks: []swf.JobTaskFilter{{
			JobType:  "recipe",
			TaskType: "input:collect_user_input",
		}},
		PageSize: 1000,
	})

	if err != nil {
		return nil, err
	}
	out := make([]PendingInput, len(jobs))
	for i, job := range jobs {
		out[i] = PendingInput{
			JobID: job.JobKey.JobId,
		}
	}

	return out, nil
}

func stringValue(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case []byte:
		return string(v)
	case float32, float64, int, int32, int64, uint, uint32, uint64, bool:
		return fmt.Sprintf("%v", v)
	case time.Time:
		return v.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("%v", val)
}

func timeAttr(attrs map[string]any, key string) string {
	val, ok := attrs[key]
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case time.Time:
		return v.UTC().Format(time.RFC3339)
	case string:
		if t, ok := parseRFC3339(v); ok {
			return t.UTC().Format(time.RFC3339)
		}
		return v
	default:
		return stringValue(val)
	}
}

func parseRFC3339(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("client-%d", time.Now().UnixNano())
}

// sortedMapKeys returns sorted keys of the map for stable logging
func sortedMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
