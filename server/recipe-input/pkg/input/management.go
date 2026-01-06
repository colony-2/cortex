package input

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	ops2 "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/go-chi/chi/v5"
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
	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	pending, err := s.collectPendingInputs(r.Context(), projectID)
	if err != nil {
		log.Printf("input_mgmt.list_pending: query_failed project_id=%s error=%v", projectID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(pending); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *inputManagementService) GetDetails(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	jobId := chi.URLParam(r, "jobId")
	if jobId == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}

	res := s.getDetails(r.Context(), projectID, jobId)
	if res.sendError(w) {
		return
	}
	form := res.value.Form
	d := details{
		jobKey:    res.value.JobKey,
		status:    res.value.Status,
		startTime: res.value.StartTime,
		form:      form,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(d); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

}

type detailsInput struct {
	JobKey    swf.JobKey `json:"jobKey"`
	Status    swf.JobStatus
	StartTime time.Time `json:"startTime"`
	Form      Config    `json:"form"`
	Hash      string    `json:"hash"`
}

// GetDetails returns details about a specific input request
func (s *inputManagementService) getDetails(ctx context.Context, projectID string, jobId string) result[*detailsInput] {
	if s.ctl == nil {
		return result[*detailsInput]{err: "workflow control unavailable"}
	}

	res := s.findJob(ctx, projectID, jobId)
	if res.hasError() {
		return result[*detailsInput]{err: res.err, status: res.status}
	}
	job := res.value

	req := ops2.ActivityInvocationRequest{}
	td := job.TaskData
	data, err := td.GetData()
	if err != nil {
		return result[*detailsInput]{err: err.Error()}
	}

	err = json.Unmarshal(data, &req)
	if err != nil {
		return result[*detailsInput]{err: err.Error()}
	}

	in := Input{}
	err = ops.DecodeWithJsonTags(req.Input, &in)
	if err != nil {
		return result[*detailsInput]{err: err.Error()}
	}

	return result[*detailsInput]{value: &detailsInput{
		JobKey:    job.JobKey,
		Status:    job.Status,
		StartTime: job.CreatedAt,
		Form:      in.Form,
		Hash:      req.GitTaskContext.PersistHash,
	}}
}

type details struct {
	jobKey    swf.JobKey
	status    swf.JobStatus
	startTime time.Time
	form      Config
	hash      string
}

func (s *inputManagementService) findJob(ctx context.Context, projectID string, jobId string) result[*workflowctl.JobItem] {
	// Query with TenantId filter and specific JobKey
	jobs, _, err := s.ctl.ListJobs(ctx, swf.ListJobsRequest{
		Stores:    []swf.JobStore{swf.JobStoreActive},
		TenantIds: []string{projectID},
		JobKeys:   []swf.JobKey{{TenantId: projectID, JobId: jobId}},
	})
	if err != nil {
		log.Printf("input_mgmt.find_job: query_failed project_id=%s job_id=%s error=%v", projectID, jobId, err)
		return result[*workflowctl.JobItem]{
			err: fmt.Errorf("failed to query workflow: %w", err).Error(),
		}
	}

	if len(jobs) == 0 {
		log.Printf("input_mgmt.find_job: not_found project_id=%s job_id=%s", projectID, jobId)
		return result[*workflowctl.JobItem]{
			err:    "not found",
			status: http.StatusNotFound,
		}
	}

	// Validate project ownership
	job := jobs[0]
	if job.JobKey.TenantId != projectID {
		log.Printf("input_mgmt.find_job: project_mismatch project_id=%s job_tenant=%s job_id=%s", projectID, job.JobKey.TenantId, jobId)
		return result[*workflowctl.JobItem]{
			err:    "workflow not found in project",
			status: http.StatusNotFound,
		}
	}

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
	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	jobId := chi.URLParam(r, "jobId")
	if jobId == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}

	output := FormResponse{}
	if err := json.NewDecoder(r.Body).Decode(&output); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if output.Fields == nil {
		http.Error(w, "fields are required", http.StatusBadRequest)
		return
	}

	if output.UserID == "" {
		output.UserID = r.Header.Get("X-User-ID")
	}

	res := s.submitResponse(r.Context(), projectID, jobId, output)
	if res.sendError(w) {
		return
	}
	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": res.value})
}

// SubmitResponse handles user form submission
func (s *inputManagementService) submitResponse(ctx context.Context, projectID string, jobId string, output FormResponse) result[bool] {
	if s.ctl == nil {
		return result[bool]{err: "workflow control unavailable"}
	}

	// Get user ID preference: body overrides header, fallback to default
	userID := output.UserID
	if userID == "" {
		userID = "anonymous"
	}

	res := s.findJob(ctx, projectID, jobId)
	if res.hasError() {
		return result[bool]{err: res.err, status: res.status}
	}

	outStep := res.value.TaskWaitOutput
	if outStep == nil {
		return result[bool]{err: "job is not waiting for user input"}
	}

	err := s.ctl.CompleteTask(ctx, res.value.JobKey, *outStep, output.Hash, output)

	if err != nil {
		return result[bool]{err: err.Error()}
	}

	// Notify via SSE if available
	if s.sse == nil {
		return result[bool]{err: "sse subsystem missing"}
	}
	return result[bool]{value: true}
}

// Cancel handles cancellation of a pending input request
func (s *inputManagementService) Cancel(w http.ResponseWriter, r *http.Request) {
	if s.ctl == nil {
		http.Error(w, "workflow control unavailable", http.StatusInternalServerError)
		return
	}

	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	jobId := chi.URLParam(r, "jobId")
	if jobId == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}

	// Parse cancellation request
	var cancelRequest struct {
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&cancelRequest); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	err := s.ctl.Cancel(r.Context(), swf.JobKey{TenantId: projectID, JobId: jobId})
	if err != nil {
		log.Printf("input_mgmt.cancel: cancel_failed project_id=%s job_id=%s error=%v", projectID, jobId, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		http.Error(w, "projectId is required", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a client ID
	clientID := r.Header.Get("X-Client-ID")
	if clientID == "" {
		clientID = generateClientID()
	}

	// Subscribe to events if SSE manager is available
	if s.sse == nil {
		// No SSE manager, just send a heartbeat and close
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"SSE not configured\"}\n\n")
		return
	}

	events := s.sse.Subscribe(clientID)
	defer s.sse.Unsubscribe(clientID)

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"client_id\": \"%s\"}\n\n", clientID)
	flush(w)

	// Emit snapshot of current pending inputs so subscribers have immediate context.
	if pending, err := s.collectPendingInputs(r.Context(), projectID); err != nil {
		log.Printf("input_mgmt.sse_stream: pending_snapshot_failed project_id=%s client_id=%s error=%v", projectID, clientID, err)
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"failed to load pending inputs\"}\n\n")
		flush(w)
	} else {
		for _, item := range pending {
			payload, err := json.Marshal(item)
			if err != nil {
				log.Printf("input_mgmt.sse_stream: marshal_pending_failed project_id=%s client_id=%s job_id=%s error=%v", projectID, clientID, item.JobID, err)
				continue
			}
			fmt.Fprintf(w, "event: input_pending\ndata: %s\n\n", payload)
		}
		flush(w)
	}

	// Create a ticker for heartbeat
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Stream events
	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return

		case event := <-events:
			// Send event to client
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
		JobTasks: []swf.JobTaskFilter{{
			JobType:  "recipe",
			TaskType: "input:input",
		}},
		PageSize: 500,
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
