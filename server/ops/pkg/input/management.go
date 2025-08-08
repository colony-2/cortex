package input

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	
	"github.com/go-chi/chi/v5"
	"go.temporal.io/sdk/client"
)

// ManagementService provides HTTP endpoints for managing input requests
type ManagementService interface {
	// GetRoutes returns HTTP routes this service provides
	GetRoutes() []Route
	
	// Initialize with injected dependencies
	Initialize(deps ServiceDependencies)
}

// ServiceDependencies contains dependencies injected by the framework
type ServiceDependencies struct {
	// Temporal client injected by framework
	TemporalClient client.Client
	// SSE manager for real-time updates
	SSEManager SSEManager
}

// SSEManager interface for Server-Sent Events
type SSEManager interface {
	Broadcast(event SSEEvent)
	Subscribe(clientID string) <-chan SSEEvent
	Unsubscribe(clientID string)
}

// SSEEvent represents a server-sent event
type SSEEvent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// Route represents an HTTP route
type Route struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// InputManagementService implements ManagementService for input activities
type InputManagementService struct {
	workflowType string
	deps         ServiceDependencies
}

// NewInputManagementService creates a new input management service
func NewInputManagementService() *InputManagementService {
	return &InputManagementService{
		workflowType: "InputCollectionWorkflow",
	}
}

// Initialize sets up the service with dependencies
func (s *InputManagementService) Initialize(deps ServiceDependencies) {
	s.deps = deps
}

// GetRoutes returns the HTTP routes provided by this service
func (s *InputManagementService) GetRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/api/user-inputs/pending", Handler: s.ListPending},
		{Method: "GET", Path: "/api/user-inputs/stream", Handler: s.SSEStream},
		{Method: "GET", Path: "/api/user-inputs/{workflowID}", Handler: s.GetDetails},
		{Method: "POST", Path: "/api/user-inputs/{workflowID}/respond", Handler: s.SubmitResponse},
		{Method: "POST", Path: "/api/user-inputs/{workflowID}/cancel", Handler: s.Cancel},
	}
}

// ListPending returns all pending input requests
func (s *InputManagementService) ListPending(w http.ResponseWriter, r *http.Request) {
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
func (s *InputManagementService) GetDetails(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	
	// Describe the workflow execution to get current state
	desc, err := s.deps.TemporalClient.DescribeWorkflowExecution(context.Background(), workflowID, "")
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
func (s *InputManagementService) SubmitResponse(w http.ResponseWriter, r *http.Request) {
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
	err := s.deps.TemporalClient.SignalWorkflow(
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
	if s.deps.SSEManager != nil {
		s.deps.SSEManager.Broadcast(SSEEvent{
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
func (s *InputManagementService) Cancel(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	
	// Parse cancellation reason
	var cancelRequest struct {
		Reason string `json:"reason"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&cancelRequest); err != nil {
		cancelRequest.Reason = "User cancelled"
	}
	
	// Cancel the workflow
	err := s.deps.TemporalClient.CancelWorkflow(context.Background(), workflowID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Notify via SSE if available
	if s.deps.SSEManager != nil {
		s.deps.SSEManager.Broadcast(SSEEvent{
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
func (s *InputManagementService) SSEStream(w http.ResponseWriter, r *http.Request) {
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
	if s.deps.SSEManager == nil {
		// No SSE manager, just send a heartbeat and close
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"SSE not configured\"}\n\n")
		return
	}
	
	events := s.deps.SSEManager.Subscribe(clientID)
	defer s.deps.SSEManager.Unsubscribe(clientID)
	
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