package input

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// InputActivityExecute is a test wrapper matching the activity signature
func InputActivityExecute(ctx context.Context, input Input) (Output, error) {
	return Output{
		Response: "approve",
		UserID:   "test-user-123",
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}, nil
}

// MockTemporalClient implements a minimal Temporal client for testing
type MockTemporalClient struct {
	client.Client
	workflows      map[string]*WorkflowExecution
	mu             sync.RWMutex
	signalHandlers map[string]chan interface{}
	describeCalls  int
}

type WorkflowExecution struct {
	ID               string
	Status           string
	SearchAttributes map[string]interface{}
	SignalChannel    chan interface{}
}

func NewMockTemporalClient() *MockTemporalClient {
	return &MockTemporalClient{
		workflows:      make(map[string]*WorkflowExecution),
		signalHandlers: make(map[string]chan interface{}),
	}
}

func (m *MockTemporalClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if wf, exists := m.workflows[workflowID]; exists {
		if wf.SignalChannel != nil {
			wf.SignalChannel <- arg
		}
		return nil
	}
	return fmt.Errorf("workflow %s not found", workflowID)
}

func (m *MockTemporalClient) DescribeWorkflowExecution(ctx context.Context, workflowID, runID string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.describeCalls++
	if _, exists := m.workflows[workflowID]; exists {
		// Return a mock description
		// In a real implementation, this would return actual workflow details
		return &workflowservice.DescribeWorkflowExecutionResponse{}, nil
	}
	return nil, fmt.Errorf("workflow %s not found", workflowID)
}

func (m *MockTemporalClient) CancelWorkflow(ctx context.Context, workflowID, runID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if wf, exists := m.workflows[workflowID]; exists {
		wf.Status = "cancelled"
		return nil
	}
	return fmt.Errorf("workflow %s not found", workflowID)
}

func (m *MockTemporalClient) RegisterWorkflow(workflowID string, execution *WorkflowExecution) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workflows[workflowID] = execution
}

// RecipeWorkflow simulates a recipe workflow that uses the input activity
func RecipeWorkflow(ctx workflow.Context, recipeID string) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting recipe workflow", "recipeID", recipeID)

	// Step 1: Execute some initial activity
	result := map[string]interface{}{
		"recipe_id": recipeID,
		"step":      "initial",
	}

	// Step 2: Execute input activity to get user approval
	inputActivityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, inputActivityOptions)

	inputConfig := Config{
		Question: "Do you approve this deployment?",
		Type:     FieldTypeMultipleChoice,
		Options: []Option{
			{Value: "approve", Label: "Approve"},
			{Value: "reject", Label: "Reject"},
		},
		Timeout: 60,
	}

	inputArgs := Input{
		BoxID:      "test-cell",
		ActivityID: "approval-activity",
		Context: map[string]interface{}{
			"recipe_id": recipeID,
		},
	}

	var inputOutput Output
	inputArgs.Config = inputConfig
	err := workflow.ExecuteActivity(ctx, InputActivityExecute, inputArgs).Get(ctx, &inputOutput)
	if err != nil {
		return nil, err
	}

	// Step 3: Process based on user response
	if inputOutput.Response == "approve" {
		result["status"] = "approved"
		result["approved_by"] = inputOutput.UserID
	} else {
		result["status"] = "rejected"
		result["rejected_by"] = inputOutput.UserID
	}

	return result, nil
}

// TestFullInputActivityLifecycle tests the complete flow from recipe to REST API
// Standalone executor integration test is defined in standalone_integration_test.go

// TestInputActivityWithChildWorkflow tests the input collection workflow directly
// Child workflow testing with signals in test framework is complex, so we test
// the InputCollectionWorkflow directly which is what the activity would spawn
func TestInputActivityWithChildWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Register the input collection workflow
	env.RegisterWorkflow(InputCollectionWorkflow)

	// Setup signal to respond to the input request
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("user-response", UserResponseSignal{
			Fields: map[string]interface{}{
				"response": "yes",
			},
			UserID:      "test-user-456",
			RespondedAt: time.Now(),
			Metadata: map[string]interface{}{
				"source": "integration-test",
			},
		})
	}, 500*time.Millisecond)

	// Execute the InputCollectionWorkflow directly
	params := InputWorkflowParams{
		Form: InputForm{
			Question: "Do you approve?",
			Type:     FieldTypeMultipleChoice,
			Options: []Option{
				{Value: "yes", Label: "Yes"},
				{Value: "no", Label: "No"},
			},
			Timeout: 5 * time.Minute,
		},
		Timeout:    5 * time.Minute,
		BoxID:      "test-cell",
		ActivityID: "test-activity",
	}

	env.ExecuteWorkflow(InputCollectionWorkflow, params)

	// Verify completion
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify the result
	var result InputWorkflowResult
	require.NoError(t, env.GetWorkflowResult(&result))
	assert.Equal(t, "yes", result.FormResponse["response"])
	assert.Equal(t, "test-user-456", result.UserID)
	assert.Equal(t, "integration-test", result.Metadata["source"])
}

// TestInputManagementServiceAPI tests the REST API endpoints
func TestInputManagementServiceAPI(t *testing.T) {
	// Create mock temporal client
	mockClient := NewMockTemporalClient()

	// Create SSE manager
	sseManager := NewSimpleSSEManager()

	// Create management service
	service := newInputManagementService()
	service.Initialize(ServiceDependencies{
		TemporalClient: mockClient,
		SSEManager:     sseManager,
	})

	// Create test router
	router := chi.NewRouter()
	routes := service.GetRoutes()
	for _, route := range routes {
		router.Method(route.Method, route.Path, route.Handler)
	}

	// Test 1: List pending inputs (should return empty list)
	t.Run("ListPending", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/user-inputs/pending", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var pending []PendingInput
		err := json.Unmarshal(rec.Body.Bytes(), &pending)
		require.NoError(t, err)
		assert.Empty(t, pending)
	})

	// Test 2: Submit response to workflow
	t.Run("SubmitResponse", func(t *testing.T) {
		// Register a mock workflow
		workflowID := "test-workflow-123"
		signalChan := make(chan interface{}, 1)
		mockClient.RegisterWorkflow(workflowID, &WorkflowExecution{
			ID:            workflowID,
			Status:        "running",
			SignalChannel: signalChan,
		})

		// Prepare response submission
		submission := map[string]interface{}{
			"fields": map[string]interface{}{
				"approval": "yes",
				"comments": "Looks good to me",
			},
			"metadata": map[string]interface{}{
				"submitted_via": "api-test",
			},
		}

		body, _ := json.Marshal(submission)
		req := httptest.NewRequest("POST", "/api/user-inputs/"+workflowID+"/respond", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", "test-user-789")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify response
		var response map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response["success"].(bool))

		// Verify signal was sent
		select {
		case signal := <-signalChan:
			userResponse := signal.(UserResponseSignal)
			assert.Equal(t, "test-user-789", userResponse.UserID)
			assert.Equal(t, "yes", userResponse.Fields["approval"])
			assert.Equal(t, "Looks good to me", userResponse.Fields["comments"])
		case <-time.After(1 * time.Second):
			t.Fatal("Expected signal not received")
		}

		// Verify a post-signal Describe was attempted
		mockClient.mu.RLock()
		calls := mockClient.describeCalls
		mockClient.mu.RUnlock()
		assert.GreaterOrEqual(t, calls, 1, "expected at least one describe call after signaling")
	})

	// Test 3: Cancel workflow
	t.Run("CancelWorkflow", func(t *testing.T) {
		workflowID := "test-workflow-456"
		mockClient.RegisterWorkflow(workflowID, &WorkflowExecution{
			ID:     workflowID,
			Status: "running",
		})

		cancelRequest := map[string]string{
			"reason": "User cancelled the operation",
		}

		body, _ := json.Marshal(cancelRequest)
		req := httptest.NewRequest("POST", "/api/user-inputs/"+workflowID+"/cancel", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify workflow was cancelled
		mockClient.mu.RLock()
		wf := mockClient.workflows[workflowID]
		mockClient.mu.RUnlock()
		assert.Equal(t, "cancelled", wf.Status)
	})
}

// Additional tests for management service behaviors
func TestInputManagementServiceAPI_SSEAndGetDetails(t *testing.T) {
	// Create mock temporal client and SSE manager
	mockClient := NewMockTemporalClient()
	sseManager := NewSimpleSSEManager()

	// Create management service
	service := newInputManagementService()
	service.Initialize(ServiceDependencies{
		TemporalClient: mockClient,
		SSEManager:     sseManager,
	})

	// Create test router
	router := chi.NewRouter()
	routes := service.GetRoutes()
	for _, route := range routes {
		router.Method(route.Method, route.Path, route.Handler)
	}

	// Register a mock workflow for SSE test
	workflowID := "sse-workflow-1"
	signalChan := make(chan interface{}, 1)
	mockClient.RegisterWorkflow(workflowID, &WorkflowExecution{
		ID:            workflowID,
		Status:        "running",
		SignalChannel: signalChan,
	})

	// Subscribe to SSE before submitting response
	events := sseManager.Subscribe("test-client-sse")
	defer sseManager.Unsubscribe("test-client-sse")

	// Submit a response
	submission := map[string]interface{}{
		"fields":   map[string]interface{}{"approval": "yes"},
		"metadata": map[string]interface{}{"submitted_via": "api-test"},
	}
	body, _ := json.Marshal(submission)
	req := httptest.NewRequest("POST", "/api/user-inputs/"+workflowID+"/respond", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-sse-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Expect an SSE event broadcast
	select {
	case evt := <-events:
		assert.Equal(t, "input_completed", evt.Type)
		assert.Equal(t, workflowID, evt.Data["workflow_id"])
		assert.Equal(t, "user-sse-1", evt.Data["user_id"])
	case <-time.After(1 * time.Second):
		t.Fatal("expected SSE event not received")
	}

	// GetDetails for unknown workflow should be 404
	req404 := httptest.NewRequest("GET", "/api/user-inputs/unknown-workflow", nil)
	rec404 := httptest.NewRecorder()
	router.ServeHTTP(rec404, req404)
	assert.Equal(t, http.StatusNotFound, rec404.Code)
}

// TestSSEEventBroadcasting tests the SSE event system
func TestSSEEventBroadcasting(t *testing.T) {
	sseManager := NewSimpleSSEManager()

	// Subscribe a client
	clientID := "test-client-1"
	events := sseManager.Subscribe(clientID)

	// Broadcast an event
	testEvent := SSEEvent{
		Type: "input_completed",
		Data: map[string]interface{}{
			"workflow_id": "test-123",
			"user_id":     "user-456",
		},
	}

	sseManager.Broadcast(testEvent)

	// Verify event was received
	select {
	case received := <-events:
		assert.Equal(t, testEvent.Type, received.Type)
		assert.Equal(t, testEvent.Data["workflow_id"], received.Data["workflow_id"])
	case <-time.After(1 * time.Second):
		t.Fatal("Expected event not received")
	}

	// Unsubscribe
	sseManager.Unsubscribe(clientID)
	assert.Equal(t, 0, sseManager.GetClients())
}

// TestEndToEndScenario tests a complete scenario with timeouts
func TestEndToEndScenario(t *testing.T) {
	t.Run("SuccessfulApproval", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		// Track workflow execution stages
		stages := []string{}

		// Define the deployment workflow
		deploymentWorkflow := func(ctx workflow.Context) (string, error) {
			stages = append(stages, "workflow_started")

			// Execute input activity
			activityOptions := workflow.ActivityOptions{
				StartToCloseTimeout: 2 * time.Minute,
			}
			ctx = workflow.WithActivityOptions(ctx, activityOptions)

			config := Config{
				Question: "Proceed with deployment?",
				Type:     FieldTypeMultipleChoice,
				Options: []Option{
					{Value: "yes", Label: "Yes, deploy"},
					{Value: "no", Label: "No, cancel"},
				},
				Timeout: 30,
			}

			input := Input{
				BoxID:      "deployment-cell",
				ActivityID: "deploy-approval",
			}

			stages = append(stages, "input_activity_called")

			var output Output
			input.Config = config
			err := workflow.ExecuteActivity(ctx, InputActivityExecute, input).Get(ctx, &output)
			if err != nil {
				return "", err
			}

			stages = append(stages, "input_received")

			if output.Response == "yes" {
				return "deployment_approved", nil
			}
			return "deployment_cancelled", nil
		}

		// Register the workflow
		env.RegisterWorkflow(deploymentWorkflow)

		// Mock the input activity to return approval
		env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything).Return(
			Output{
				Response: "yes",
				UserID:   "approver-123",
			}, nil,
		)

		// Execute workflow
		env.ExecuteWorkflow(deploymentWorkflow)

		// Verify successful completion
		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError())

		var result string
		require.NoError(t, env.GetWorkflowResult(&result))
		assert.Equal(t, "deployment_approved", result)

		// Verify all stages were executed
		assert.Contains(t, stages, "workflow_started")
		assert.Contains(t, stages, "input_activity_called")
		assert.Contains(t, stages, "input_received")
	})

	t.Run("TimeoutScenario", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestWorkflowEnvironment()

		// Register the input collection workflow
		env.RegisterWorkflow(InputCollectionWorkflow)

		// Execute workflow with very short timeout
		params := InputWorkflowParams{
			Form: InputForm{
				Question: "Quick decision needed",
				Type:     FieldTypeMultipleChoice,
				Options: []Option{
					{Value: "yes"},
					{Value: "no"},
				},
				Timeout: 100 * time.Millisecond, // Very short timeout
			},
			Timeout:    100 * time.Millisecond,
			BoxID:      "timeout-test-cell",
			ActivityID: "timeout-activity",
		}

		// Don't send any signal - let it timeout
		env.ExecuteWorkflow(InputCollectionWorkflow, params)

		// Verify timeout occurred
		require.True(t, env.IsWorkflowCompleted())
		err := env.GetWorkflowError()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deadline exceeded")
	})
}

// TestActivityWithTemporalContext tests the activity with injected Temporal context
func TestActivityWithTemporalContext(t *testing.T) {
	activity := newInputActivity()

	// Test without temporal context (standalone mode)
	config := Config{
		Question: "Test question?",
		Type:     FieldTypeShortAnswer,
		Timeout:  60,
	}

	input := Input{
		BoxID:      "test-cell",
		ActivityID: "test-activity",
	}

	in := input
	in.Config = config
	output, err := activity.Execute(context.Background(), in)
	require.NoError(t, err)

	// In standalone mode, it returns mock response
	assert.Equal(t, "test response", output.Response)
	assert.Equal(t, "test-user", output.UserID)

	// Verify management service is available
	mgmtService := activity.GetManagementService()
	assert.NotNil(t, mgmtService)
}

// TestMultiFieldFormIntegration tests complex multi-field forms
func TestMultiFieldFormIntegration(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Define workflow with multi-field form
	configWorkflow := func(ctx workflow.Context) (map[string]interface{}, error) {
		activityOptions := workflow.ActivityOptions{
			StartToCloseTimeout: 5 * time.Minute,
		}
		ctx = workflow.WithActivityOptions(ctx, activityOptions)

		config := Config{
			Title: "Deployment Configuration",
			Fields: []FormField{
				{
					ID:       "environment",
					Type:     FieldTypeDropdown,
					Question: "Target environment",
					Required: true,
					Options: []Option{
						{Value: "dev", Label: "Development"},
						{Value: "staging", Label: "Staging"},
						{Value: "prod", Label: "Production"},
					},
				},
				{
					ID:       "strategy",
					Type:     FieldTypeMultipleChoice,
					Question: "Deployment strategy",
					Required: true,
					Options: []Option{
						{Value: "rolling", Label: "Rolling Update"},
						{Value: "blue_green", Label: "Blue-Green"},
						{Value: "canary", Label: "Canary"},
					},
				},
				{
					ID:       "urgency",
					Type:     FieldTypeLinearScale,
					Question: "Urgency level",
					Scale: &LinearScale{
						Min:      1,
						Max:      5,
						MinLabel: "Low",
						MaxLabel: "Critical",
					},
				},
				{
					ID:          "notes",
					Type:        FieldTypeParagraphText,
					Question:    "Additional notes",
					Placeholder: "Any special considerations?",
				},
			},
			Timeout: 300,
		}

		input := Input{
			BoxID:      "config-cell",
			ActivityID: "config-activity",
		}

		var output Output
		input.Config = config
		err := workflow.ExecuteActivity(ctx, InputActivityExecute, input).Get(ctx, &output)
		if err != nil {
			return nil, err
		}

		// Process the multi-field response
		result := map[string]interface{}{
			"environment": output.Fields["environment"],
			"strategy":    output.Fields["strategy"],
			"urgency":     output.Fields["urgency"],
			"notes":       output.Fields["notes"],
			"approved_by": output.UserID,
		}

		return result, nil
	}

	// Register the workflow
	env.RegisterWorkflow(configWorkflow)

	// Mock the input activity to return multi-field response
	env.OnActivity(InputActivityExecute, mock.Anything, mock.Anything).Return(
		Output{
			Fields: map[string]interface{}{
				"environment": "staging",
				"strategy":    "blue_green",
				"urgency":     3,
				"notes":       "Please monitor closely after deployment",
			},
			UserID: "config-user-999",
		}, nil,
	)

	// Execute workflow
	env.ExecuteWorkflow(configWorkflow)

	// Verify completion
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify result
	var result map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&result))

	assert.Equal(t, "staging", result["environment"])
	assert.Equal(t, "blue_green", result["strategy"])
	assert.Equal(t, float64(3), result["urgency"])
	assert.Equal(t, "Please monitor closely after deployment", result["notes"])
	assert.Equal(t, "config-user-999", result["approved_by"])
}

// BenchmarkInputActivityExecution benchmarks the input activity performance
func BenchmarkInputActivityExecution(b *testing.B) {
	activity := newInputActivity()

	config := Config{
		Question: "Benchmark question?",
		Type:     FieldTypeMultipleChoice,
		Options: []Option{
			{Value: "option1"},
			{Value: "option2"},
		},
		Timeout: 60,
	}

	input := Input{
		BoxID:      "bench-cell",
		ActivityID: "bench-activity",
	}

	ctx := context.Background()
	in := input
	in.Config = config

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := activity.Execute(ctx, in)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestWorkerIntegration tests the activity registration with a Temporal worker
func TestWorkerIntegration(t *testing.T) {
	// This test verifies that the activity can be properly registered with a worker
	// In a real scenario, this would connect to an actual Temporal server

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// Create a mock worker
	env.RegisterWorkflow(RecipeWorkflow)
	env.RegisterWorkflow(InputCollectionWorkflow)

	// Register the actual input activity
	inputActivity := newInputActivity()
	env.RegisterActivity(inputActivity.Execute)

	// Set up activity options
	env.SetWorkerOptions(worker.Options{
		MaxConcurrentActivityExecutionSize: 10,
	})

	// Execute a workflow that uses the activity
	env.ExecuteWorkflow(RecipeWorkflow, "integration-recipe-001")

	// The workflow should complete (even though the activity returns mock data in test mode)
	require.True(t, env.IsWorkflowCompleted())
}
