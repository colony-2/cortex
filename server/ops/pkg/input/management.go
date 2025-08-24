package input

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
	"github.com/go-chi/chi/v5"
	"go.temporal.io/sdk/client"
)

// inputManagementService implements ManagementService for input activities
type inputManagementService struct {
	workflowType string
	sse          types.SSEManager
	client       client.Client
}

// newInputManagementService creates a new input management service
func newInputManagementService() *inputManagementService {
	return &inputManagementService{
		workflowType: "InputCollectionWorkflow",
	}
}

// Initialize sets up the service with dependencies
func (s *inputManagementService) Initialize(deps types.ServiceDependencies) error {
	sse, err := deps.Get("sse")
	if err != nil {
		return err
	}
	s.sse = sse.(types.SSEManager)
	c, err := deps.Get("temporal_client")
	if err != nil {
		return err
	}

	s.client = c.(client.Client)
	return nil
}

func (s *inputManagementService) Close() {
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

// GetRoutes returns the HTTP routes provided by this service
func (s *inputManagementService) GetRoutes() []types.Route {
	return []types.Route{
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

	// Describe the workflow execution to get current state
	desc, err := s.client.DescribeWorkflowExecution(context.Background(), workflowID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Extract workflow info and search attributes
	result := map[string]interface{}{
		"workflow_id": workflowID,
		"status":      desc.WorkflowExecutionInfo.Status.String(),
		"start_time":  desc.WorkflowExecutionInfo.StartTime,
	}

	// Add search attributes if available
	if desc.WorkflowExecutionInfo.SearchAttributes != nil {
		result["attributes"] = desc.WorkflowExecutionInfo.SearchAttributes.IndexedFields
	}

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

	// Send signal to workflow
	err := s.client.SignalWorkflow(
		context.Background(),
		workflowID,
		"",
		"user-response",
		UserResponseSignal{
			Fields:      submission.Fields,
			UserID:      userID,
			RespondedAt: time.Now(),
			Metadata:    submission.Metadata,
		},
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Notify via SSE if available
	if s.sse != nil {
		s.sse.Broadcast(types.SSEEvent{
			Type: "input_completed",
			Data: map[string]interface{}{
				"workflow_id": workflowID,
				"user_id":     userID,
			},
		})
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

	// Cancel the workflow
	err := s.client.CancelWorkflow(context.Background(), workflowID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Notify via SSE if available
	if s.sse != nil {
		s.sse.Broadcast(types.SSEEvent{
			Type: "input_cancelled",
			Data: map[string]interface{}{
				"workflow_id": workflowID,
				"reason":      cancelRequest.Reason,
			},
		})
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
