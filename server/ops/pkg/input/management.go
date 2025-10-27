package input

import (
	"context"
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
	lister       workflowctl.WorkflowLister
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
		if lister, ok := ctl.(workflowctl.WorkflowLister); ok {
			s.lister = lister
		} else {
			log.Printf("input_mgmt.initialize: workflow_control_missing_list_support")
		}
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
		{Method: "POST", Path: "/api/user-inputs/{workflowID}/respond", Handler: s.SubmitResponse},
		{Method: "POST", Path: "/api/user-inputs/{workflowID}/cancel", Handler: s.Cancel},
	}
}

// ListPending returns all pending input requests
func (s *inputManagementService) ListPending(w http.ResponseWriter, r *http.Request) {
	if s.ctl == nil {
		http.Error(w, "workflow control unavailable", http.StatusInternalServerError)
		return
	}
	if s.lister == nil {
		http.Error(w, "workflow control does not support workflow listing", http.StatusNotImplemented)
		return
	}

	pending, err := s.collectPendingInputs(r.Context())
	if err != nil {
		log.Printf("input_mgmt.list_pending: query_failed error=%v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(pending); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	flush(w)

	// Emit snapshot of current pending inputs so subscribers have immediate context.
	if pending, err := s.collectPendingInputs(r.Context()); err != nil {
		log.Printf("input_mgmt.sse_stream: pending_snapshot_failed client_id=%s error=%v", clientID, err)
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"failed to load pending inputs\"}\n\n")
		flush(w)
	} else {
		for _, item := range pending {
			payload, err := json.Marshal(item)
			if err != nil {
				log.Printf("input_mgmt.sse_stream: marshal_pending_failed client_id=%s workflow_id=%s error=%v", clientID, item.WorkflowID, err)
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

func (s *inputManagementService) collectPendingInputs(ctx context.Context) ([]PendingInput, error) {
	if s.lister == nil {
		return nil, fmt.Errorf("workflow control does not support workflow listing")
	}
	var (
		token []byte
		items []PendingInput
		seen  = make(map[string]struct{})
	)
	for {
		resp, err := s.lister.ListWorkflows(ctx, workflowctl.ListWorkflowsRequest{
			Query:         pendingStatusQuery,
			PageSize:      50,
			NextPageToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, summary := range resp.Executions {
			item, ok := buildPendingFromSummary(summary)
			if !ok {
				continue
			}
			key := item.WorkflowID + ":" + item.ID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, item)
		}
		if len(resp.NextPageToken) == 0 {
			break
		}
		token = resp.NextPageToken
	}
	sort.Slice(items, func(i, j int) bool {
		ti, okI := parseRFC3339(items[i].CreatedAt)
		tj, okJ := parseRFC3339(items[j].CreatedAt)
		switch {
		case okI && okJ && !ti.Equal(tj):
			return ti.Before(tj)
		case okI && !okJ:
			return true
		case !okI && okJ:
			return false
		default:
			if items[i].WorkflowID == items[j].WorkflowID {
				return items[i].ID < items[j].ID
			}
			return items[i].WorkflowID < items[j].WorkflowID
		}
	})
	return items, nil
}

func buildPendingFromSummary(summary workflowctl.WorkflowSummary) (PendingInput, bool) {
	if len(summary.SearchAttributes) == 0 {
		return PendingInput{}, false
	}
	if status := stringAttr(summary.SearchAttributes, "InputStatus"); status != "" && !strings.EqualFold(status, "pending") {
		return PendingInput{}, false
	}
	id := stringAttr(summary.SearchAttributes, "InputKey")
	if id == "" {
		id = summary.WorkflowID
	}
	return PendingInput{
		ID:         id,
		WorkflowID: summary.WorkflowID,
		BoxID:      stringAttr(summary.SearchAttributes, "InputBoxID"),
		FormTitle:  stringAttr(summary.SearchAttributes, "InputFormTitle"),
		CreatedAt:  timeAttr(summary.SearchAttributes, "InputCreatedAt"),
		ExpiresAt:  timeAttr(summary.SearchAttributes, "InputExpiresAt"),
	}, true
}

func stringAttr(attrs map[string]any, key string) string {
	val, ok := attrs[key]
	if !ok || val == nil {
		return ""
	}
	return stringValue(val)
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
