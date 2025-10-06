package input

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/go-chi/chi/v5"
)

// inputManagementService implements ManagementService for input activities
type inputManagementService struct {
	workflowType string
	sse          ops.SSEManager
	namespace    string
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
	if ctl, ok := deps.WorkflowControl(); ok && ctl != nil {
		s.ctl = ctl
	} else {
		log.Printf("input_mgmt.initialize: missing_workflow_control error=workflow control dependency not provided")
		return fmt.Errorf("workflow control dependency not provided")
	}
	if sse, ok := deps.SSEManager(); ok {
		s.sse = sse
	} else {
		log.Printf("input_mgmt.initialize: missing_sse_manager")
		return fmt.Errorf("sse manager dependency not provided")
	}
	// Best-effort namespace detection via deps or env
	if ns, ok := deps.TemporalNamespace(); ok && ns != "" {
		s.namespace = ns
	}
	if s.namespace == "" {
		s.namespace = os.Getenv("TEMPORAL_NAMESPACE")
	}
	if s.namespace == "" {
		s.namespace = "unknown"
	}
	log.Printf("input_mgmt.initialize: initialized sse_present=%t namespace=%s", s.sse != nil, s.namespace)
	return nil
}

func (s *inputManagementService) Close() {
	// No-op: service depends on external WorkflowControl lifetime
}

// GetRoutes returns the HTTP routes provided by this service
func (s *inputManagementService) GetRoutes() []ops.Route {
	return []ops.Route{
		{Method: "GET", Path: "/api/user-inputs/pending", Handler: s.ListPending},
		{Method: "GET", Path: "/api/user-inputs/stream", Handler: s.SSEStream},
		{Method: "GET", Path: "/api/user-inputs/{workflowID}", Handler: s.GetDetails},
		{Method: "POST", Path: "/api/user-inputs/{workflowID}/pending", Handler: s.MarkPending},
		{Method: "POST", Path: "/api/user-inputs/{workflowID}/respond", Handler: s.SubmitResponse},
		{Method: "POST", Path: "/api/user-inputs/{workflowID}/cancel", Handler: s.Cancel},
	}
}

// ListPending returns all pending input requests
func (s *inputManagementService) ListPending(w http.ResponseWriter, r *http.Request) {
	// For now, return an empty list since we need to integrate with actual Temporal client
	// The exact API depends on the Temporal SDK version and configuration
	// This will be properly implemented when the Temporal client is fully configured

	pending := []PendingInput{}

	// TODO: Implement actual query when Temporal client is properly configured
	// The query would look something like:
	// query := `InputWorkflowType = "user-input" AND InputStatus = "pending"`
	// And use the client's ListWorkflow method or WorkflowService().ListWorkflowExecutions

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(pending); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// MarkPending records a newly created input request and emits an SSE event.
// Body: {"id": string, "title"?: string, "box_id"?: string, "expires_at"?: string(ISO8601), "metadata"?: object}
func (s *inputManagementService) MarkPending(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "user-inputs" && i+1 < len(parts) {
				workflowID = parts[i+1]
				break
			}
		}
	}
	var body struct {
		ID        string                 `json:"id"`
		Title     string                 `json:"title"`
		BoxID     string                 `json:"box_id"`
		ExpiresAt string                 `json:"expires_at"`
		Metadata  map[string]interface{} `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if s.sse != nil {
		s.sse.Broadcast(ops.SSEEvent{
			Type: "input_pending",
			Data: map[string]interface{}{
				"id":          body.ID,
				"workflow_id": workflowID,
				"title":       body.Title,
				"box_id":      body.BoxID,
				"expires_at":  body.ExpiresAt,
				"metadata":    body.Metadata,
			},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// GetDetails returns details about a specific input request
func (s *inputManagementService) GetDetails(w http.ResponseWriter, r *http.Request) {
	if s.ctl == nil {
		http.Error(w, "workflow control unavailable", http.StatusInternalServerError)
		return
	}
	// Extract from chi if available, otherwise parse from URL path segments.
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "user-inputs" {
				if i+1 < len(parts) {
					workflowID = parts[i+1]
				}
				break
			}
		}
	}
	ns := s.namespace
	if hdr := r.Header.Get("X-Temporal-Namespace"); hdr != "" {
		ns = hdr
	}
	log.Printf("input_mgmt.get_details: entry workflow_id=%s namespace=%s", workflowID, ns)

	// Describe the workflow execution to get current state
	var status interface{}
	var start interface{}
	sum, err := s.ctl.Describe(r.Context(), workflowctl.ExecutionRef{WorkflowID: workflowID})
	if err != nil {
		log.Printf("input_mgmt.get_details: describe_failed workflow_id=%s error=%v", workflowID, err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	status = sum.Status
	if sum.StartTime != nil {
		start = sum.StartTime
	}

	// Extract workflow info and search attributes
	result := map[string]interface{}{
		"workflow_id": workflowID,
		"status":      nil,
		"start_time":  nil,
	}

	result["status"] = status
	result["start_time"] = start
	log.Printf("input_mgmt.get_details: describe_ok workflow_id=%s status=%v namespace=%s", workflowID, result["status"], ns)

	// TODO: Fetch the actual form definition from workflow state
	// This would require querying the workflow or storing form data separately

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SubmitResponse handles user form submission
func (s *inputManagementService) SubmitResponse(w http.ResponseWriter, r *http.Request) {
	if s.ctl == nil {
		http.Error(w, "workflow control unavailable", http.StatusInternalServerError)
		return
	}
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "user-inputs" {
				if i+1 < len(parts) {
					workflowID = parts[i+1]
				}
				break
			}
		}
	}

	// Parse the submission
	var submission struct {
		ID       string                 `json:"id"`
		UserID   string                 `json:"user_id"`
		Fields   map[string]interface{} `json:"fields"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(submission.ID) == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if submission.Fields == nil {
		http.Error(w, "fields are required", http.StatusBadRequest)
		return
	}

	// Get user ID preference: body overrides header, fallback to default
	userID := submission.UserID
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}
	if userID == "" {
		userID = "anonymous"
	}

	// Handler entry log
	ns := s.namespace
	if hdr := r.Header.Get("X-Temporal-Namespace"); hdr != "" {
		ns = hdr
	}
	log.Printf(
		"input_mgmt.submit_response: entry workflow_id=%s id=%s user_id=%s field_keys=%s namespace=%s",
		workflowID, submission.ID, userID, strings.Join(sortedMapKeys(submission.Fields), ","), ns,
	)

	// Send signal to workflow
	payload := UserResponseSignal{
		Fields:      submission.Fields,
		UserID:      userID,
		RespondedAt: time.Now(),
		Metadata:    submission.Metadata,
	}

	signalName := userResponseSignalName(submission.ID)

	// Log before signaling
	log.Printf(
		"input_mgmt.submit_response: pre_signal signal=%s workflow_id=%s run_id=%s namespace=%s payload_type=%s payload_checksum=%s",
		signalName, workflowID, "", ns, reflect.TypeOf(payload).String(), checksumForFieldsAndMeta(submission.Fields, submission.Metadata),
	)

	if err := s.ctl.Signal(r.Context(), workflowctl.ExecutionRef{WorkflowID: workflowID}, signalName, payload); err != nil {
		log.Printf("input_mgmt.submit_response: signal_failed workflow_id=%s run_id=%s namespace=%s id=%s error=%v", workflowID, "", ns, submission.ID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("input_mgmt.submit_response: signal_ok workflow_id=%s run_id=%s namespace=%s id=%s", workflowID, "", ns, submission.ID)

	// Optional: short post-signal verification
	if sum, derr := s.ctl.Describe(r.Context(), workflowctl.ExecutionRef{WorkflowID: workflowID}); derr != nil {
		log.Printf("input_mgmt.submit_response: post_describe_failed workflow_id=%s namespace=%s error=%v", workflowID, ns, derr)
	} else {
		log.Printf("input_mgmt.submit_response: post_describe_ok workflow_id=%s namespace=%s status=%s", workflowID, ns, sum.Status)
	}

	// Notify via SSE if available
	if s.sse != nil {
		log.Printf("input_mgmt.submit_response: sse_broadcast_pre event=input_completed workflow_id=%s id=%s user_id=%s namespace=%s", workflowID, submission.ID, userID, ns)
		s.sse.Broadcast(ops.SSEEvent{
			Type: "input_completed",
			Data: map[string]interface{}{
				"workflow_id": workflowID,
				"id":          submission.ID,
				"user_id":     userID,
				"fields":      submission.Fields,
			},
		})
		log.Printf("input_mgmt.submit_response: sse_broadcast_ok event=input_completed workflow_id=%s id=%s user_id=%s namespace=%s", workflowID, submission.ID, userID, ns)
	}
	if s.sse == nil {
		log.Printf("input_mgmt.submit_response: sse_broadcast_skipped reason=nil_manager workflow_id=%s user_id=%s namespace=%s", workflowID, userID, ns)
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// Cancel handles cancellation of a pending input request
func (s *inputManagementService) Cancel(w http.ResponseWriter, r *http.Request) {
	if s.ctl == nil {
		http.Error(w, "workflow control unavailable", http.StatusInternalServerError)
		return
	}
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "user-inputs" {
				if i+1 < len(parts) {
					workflowID = parts[i+1]
				}
				break
			}
		}
	}

	// Parse cancellation request
	var cancelRequest struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&cancelRequest); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(cancelRequest.ID) == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if cancelRequest.Reason == "" {
		cancelRequest.Reason = "User cancelled"
	}

	log.Printf("input_mgmt.cancel: entry workflow_id=%s id=%s reason=%q", workflowID, cancelRequest.ID, cancelRequest.Reason)

	// Cancel the workflow
	err := s.ctl.Cancel(r.Context(), workflowctl.ExecutionRef{WorkflowID: workflowID}, cancelRequest.Reason)
	if err != nil {
		log.Printf("input_mgmt.cancel: cancel_failed workflow_id=%s id=%s error=%v", workflowID, cancelRequest.ID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("input_mgmt.cancel: cancel_ok workflow_id=%s id=%s", workflowID, cancelRequest.ID)

	// Notify via SSE if available
	if s.sse != nil {
		log.Printf("input_mgmt.cancel: sse_broadcast_pre event=input_cancelled workflow_id=%s id=%s", workflowID, cancelRequest.ID)
		s.sse.Broadcast(ops.SSEEvent{
			Type: "input_cancelled",
			Data: map[string]interface{}{
				"workflow_id": workflowID,
				"id":          cancelRequest.ID,
				"reason":      cancelRequest.Reason,
			},
		})
		log.Printf("input_mgmt.cancel: sse_broadcast_ok event=input_cancelled workflow_id=%s id=%s", workflowID, cancelRequest.ID)
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// SSEStream handles Server-Sent Events streaming for real-time updates
func (s *inputManagementService) SSEStream(w http.ResponseWriter, r *http.Request) {
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
	w.(http.Flusher).Flush()

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
			w.(http.Flusher).Flush()

		case <-ticker.C:
			// Send heartbeat
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"timestamp\": \"%s\"}\n\n", time.Now().Format(time.RFC3339))
			w.(http.Flusher).Flush()
		}
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

// checksumForFieldsAndMeta returns a deterministic checksum of field keys/types and metadata keys/types
func checksumForFieldsAndMeta(fields, meta map[string]interface{}) string {
	b := strings.Builder{}
	if fields != nil {
		fk := sortedMapKeys(fields)
		for _, k := range fk {
			var t string
			if v, ok := fields[k]; ok && v != nil {
				t = reflect.TypeOf(v).String()
			}
			b.WriteString("f:")
			b.WriteString(k)
			b.WriteString(":")
			b.WriteString(t)
			b.WriteString(";")
		}
	}
	if meta != nil {
		mk := sortedMapKeys(meta)
		for _, k := range mk {
			var t string
			if v, ok := meta[k]; ok && v != nil {
				t = reflect.TypeOf(v).String()
			}
			b.WriteString("m:")
			b.WriteString(k)
			b.WriteString(":")
			b.WriteString(t)
			b.WriteString(";")
		}
	}
	// simple FNV-like hash without introducing crypto dependencies
	var h uint64 = 1469598103934665603
	const prime64 = 1099511628211
	s := b.String()
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return fmt.Sprintf("fnv64-%x", h)
}
