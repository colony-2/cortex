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
    "github.com/go-chi/chi/v5"
    "go.temporal.io/sdk/client"
    "google.golang.org/grpc/status"
)

// inputManagementService implements ManagementService for input activities
type inputManagementService struct {
    workflowType string
    sse          ops.SSEManager
    client       client.Client
    namespace    string
}

// newInputManagementService creates a new input management service
func newInputManagementService() *inputManagementService {
	return &inputManagementService{
		workflowType: "InputCollectionWorkflow",
	}
}

// Initialize sets up the service with dependencies
func (s *inputManagementService) Initialize(deps ops.ServiceDependencies2) error {
    // Optional typed accessor for workflow control if available
    if ctl, ok := deps.WorkflowControl(); ok && ctl != nil {
        // Currently unused, but presence enables typed interactions in the future.
        _ = ctl
    }
    sse, err := deps.Get("sse")
    if err != nil {
        log.Printf("input_mgmt.initialize: missing_sse_manager error=%v", err)
        return err
    }
    s.sse = sse.(ops.SSEManager)
    c, err := deps.Get("temporal_client")
    if err != nil {
        log.Printf("input_mgmt.initialize: temporal_client_not_provided note=HTTP endpoints depending on Temporal may fail")
        return err
    }

    s.client = c.(client.Client)
    // Best-effort namespace detection via deps or env
    if ns, err := deps.Get("temporal_namespace"); err == nil {
        if v, ok := ns.(string); ok && v != "" {
            s.namespace = v
        }
    }
    if s.namespace == "" {
        s.namespace = os.Getenv("TEMPORAL_NAMESPACE")
    }
    if s.namespace == "" {
        s.namespace = "unknown"
    }
    log.Printf("input_mgmt.initialize: initialized temporal_client_present=%t sse_present=%t namespace=%s", s.client != nil, s.sse != nil, s.namespace)
    return nil
}

func (s *inputManagementService) Close() {
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
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

// GetDetails returns details about a specific input request
func (s *inputManagementService) GetDetails(w http.ResponseWriter, r *http.Request) {
    workflowID := chi.URLParam(r, "workflowID")
    ns := s.namespace
    if hdr := r.Header.Get("X-Temporal-Namespace"); hdr != "" {
        ns = hdr
    }
    log.Printf("input_mgmt.get_details: entry workflow_id=%s client_nil=%t namespace=%s", workflowID, s.client == nil, ns)

    // Describe the workflow execution to get current state
    desc, err := s.client.DescribeWorkflowExecution(context.Background(), workflowID, "")
    if err != nil {
        log.Printf("input_mgmt.get_details: describe_failed workflow_id=%s error=%v", workflowID, err)
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }

    // Extract workflow info and search attributes
    result := map[string]interface{}{
        "workflow_id": workflowID,
        "status":      nil,
        "start_time":  nil,
    }

    if desc.WorkflowExecutionInfo != nil {
        result["status"] = desc.WorkflowExecutionInfo.Status.String()
        result["start_time"] = desc.WorkflowExecutionInfo.StartTime
        // Add search attributes if available
        if desc.WorkflowExecutionInfo.SearchAttributes != nil {
            result["attributes"] = desc.WorkflowExecutionInfo.SearchAttributes.IndexedFields
        }
    }
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
    workflowID := chi.URLParam(r, "workflowID")

	// Parse the submission
	var submission struct {
		Fields   map[string]interface{} `json:"fields"`
		Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get user ID from header (would be set by auth middleware)
	userID := r.Header.Get("X-User-ID")
    if userID == "" {
        userID = "anonymous"
    }

    // Handler entry log
    ns := s.namespace
    if hdr := r.Header.Get("X-Temporal-Namespace"); hdr != "" {
        ns = hdr
    }
    log.Printf(
        "input_mgmt.submit_response: entry workflow_id=%s user_id=%s field_keys=%s client_nil=%t namespace=%s",
        workflowID, userID, strings.Join(sortedMapKeys(submission.Fields), ","), s.client == nil, ns,
    )

    // Send signal to workflow
    payload := UserResponseSignal{
        Fields:      submission.Fields,
        UserID:      userID,
        RespondedAt: time.Now(),
        Metadata:    submission.Metadata,
    }

    // Log before signaling
    log.Printf(
        "input_mgmt.submit_response: pre_signal signal=user-response workflow_id=%s run_id=%s namespace=%s payload_type=%s payload_checksum=%s",
        workflowID, "", ns, reflect.TypeOf(payload).String(), checksumForFieldsAndMeta(submission.Fields, submission.Metadata),
    )

    err := s.client.SignalWorkflow(
        context.Background(),
        workflowID,
        "",
        "user-response",
        payload,
    )

    if err != nil {
        if st, ok := status.FromError(err); ok {
            log.Printf("input_mgmt.submit_response: signal_failed workflow_id=%s run_id=%s namespace=%s grpc_code=%s error=%v", workflowID, "", ns, st.Code().String(), err)
        } else {
            log.Printf("input_mgmt.submit_response: signal_failed workflow_id=%s run_id=%s namespace=%s error=%v", workflowID, "", ns, err)
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    log.Printf("input_mgmt.submit_response: signal_ok workflow_id=%s run_id=%s namespace=%s", workflowID, "", ns)

    // Optional: short post-signal verification
    if s.client != nil {
        desc, derr := s.client.DescribeWorkflowExecution(context.Background(), workflowID, "")
        if derr != nil {
            log.Printf("input_mgmt.submit_response: post_describe_failed workflow_id=%s namespace=%s error=%v", workflowID, ns, derr)
        } else if desc != nil && desc.WorkflowExecutionInfo != nil {
            log.Printf(
                "input_mgmt.submit_response: post_describe_ok workflow_id=%s namespace=%s status=%s start_time=%v",
                workflowID, ns,
                desc.WorkflowExecutionInfo.Status.String(),
                desc.WorkflowExecutionInfo.StartTime,
            )
        }
    }

    // Notify via SSE if available
    if s.sse != nil {
        log.Printf("input_mgmt.submit_response: sse_broadcast_pre event=input_completed workflow_id=%s user_id=%s namespace=%s", workflowID, userID, ns)
        s.sse.Broadcast(ops.SSEEvent{
            Type: "input_completed",
            Data: map[string]interface{}{
                "workflow_id": workflowID,
                "user_id":     userID,
            },
        })
        log.Printf("input_mgmt.submit_response: sse_broadcast_ok event=input_completed workflow_id=%s user_id=%s namespace=%s", workflowID, userID, ns)
    }
    if s.sse == nil {
        log.Printf("input_mgmt.submit_response: sse_broadcast_skipped reason=nil_manager workflow_id=%s user_id=%s namespace=%s", workflowID, userID, ns)
    }

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Response submitted successfully",
	})
}

// Cancel handles cancellation of a pending input request
func (s *inputManagementService) Cancel(w http.ResponseWriter, r *http.Request) {
    workflowID := chi.URLParam(r, "workflowID")

	// Parse cancellation reason
	var cancelRequest struct {
		Reason string `json:"reason"`
	}

    if err := json.NewDecoder(r.Body).Decode(&cancelRequest); err != nil {
        cancelRequest.Reason = "User cancelled"
    }

    log.Printf("input_mgmt.cancel: entry workflow_id=%s reason=%q", workflowID, cancelRequest.Reason)

	// Cancel the workflow
    err := s.client.CancelWorkflow(context.Background(), workflowID, "")
    if err != nil {
        if st, ok := status.FromError(err); ok {
            log.Printf("input_mgmt.cancel: cancel_failed workflow_id=%s grpc_code=%s error=%v", workflowID, st.Code().String(), err)
        } else {
            log.Printf("input_mgmt.cancel: cancel_failed workflow_id=%s error=%v", workflowID, err)
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    log.Printf("input_mgmt.cancel: cancel_ok workflow_id=%s", workflowID)

	// Notify via SSE if available
    if s.sse != nil {
        log.Printf("input_mgmt.cancel: sse_broadcast_pre event=input_cancelled workflow_id=%s", workflowID)
        s.sse.Broadcast(ops.SSEEvent{
            Type: "input_cancelled",
            Data: map[string]interface{}{
                "workflow_id": workflowID,
                "reason":      cancelRequest.Reason,
            },
        })
        log.Printf("input_mgmt.cancel: sse_broadcast_ok event=input_cancelled workflow_id=%s", workflowID)
    }

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Input request cancelled",
	})
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
