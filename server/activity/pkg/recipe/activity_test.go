package recipe

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/client"
)

// MockTemporalClient is a simplified mock implementation of client.Client
type MockTemporalClient struct {
	mock.Mock
	client.Client // Embed the interface to satisfy all methods
}

func (m *MockTemporalClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	argList := m.Called(ctx, options, workflow, args)
	if argList.Get(0) == nil {
		return nil, argList.Error(1)
	}
	return argList.Get(0).(client.WorkflowRun), argList.Error(1)
}

// MockWorkflowRun is a mock implementation of client.WorkflowRun
type MockWorkflowRun struct {
	mock.Mock
}

func (m *MockWorkflowRun) GetID() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockWorkflowRun) GetRunID() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockWorkflowRun) Get(ctx context.Context, valuePtr interface{}) error {
	args := m.Called(ctx, valuePtr)
	if args.Get(0) != nil {
		// Set the value if provided in the mock
		if result, ok := args.Get(0).(map[string]interface{}); ok {
			if ptr, ok := valuePtr.(*map[string]interface{}); ok {
				*ptr = result
			}
		}
	}
	return args.Error(1)
}

func (m *MockWorkflowRun) GetWithOptions(ctx context.Context, valuePtr interface{}, options client.WorkflowRunGetOptions) error {
	args := m.Called(ctx, valuePtr, options)
	return args.Error(0)
}

// MockClientProvider is a mock implementation of TemporalClientProvider
type MockClientProvider struct {
	mock.Mock
}

func (m *MockClientProvider) GetClient(ctx context.Context) (client.Client, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(client.Client), args.Error(1)
}

// TestExecuteRecipeActivity tests the ExecuteRecipeActivity function
func TestExecuteRecipeActivity(t *testing.T) {
	tests := []struct {
		name           string
		input          RecipeActivity
		setupMocks     func(*MockClientProvider, *MockTemporalClient, *MockWorkflowRun)
		expectedError  bool
		expectedStatus string
	}{
		{
			name: "successful recipe execution",
			input: RecipeActivity{
				Recipe:  "test-recipe",
				Timeout: 30 * time.Minute,
				Inputs: map[string]interface{}{
					"param1": "value1",
				},
				Context: &RecipeContext{
					Recipe: RecipeInfo{
						Name:        "parent-recipe",
						ExecutionID: "parent-123",
					},
					Execution: ExecutionInfo{
						TaskQueue: "test-queue",
						Namespace: "test-namespace",
					},
				},
			},
			setupMocks: func(provider *MockClientProvider, client *MockTemporalClient, run *MockWorkflowRun) {
				provider.On("GetClient", mock.Anything).Return(client, nil)
				
				run.On("GetID").Return("child-recipe-123")
				run.On("Get", mock.Anything, mock.Anything).Return(
					map[string]interface{}{
						"output1": "result1",
					},
					nil,
				)
				
				client.On("ExecuteWorkflow",
					mock.Anything,
					mock.Anything,  // StartWorkflowOptions
					"test-recipe",
					mock.Anything,
				).Return(run, nil)
			},
			expectedError:  false,
			expectedStatus: "completed",
		},
		{
			name: "recipe execution with retry policy",
			input: RecipeActivity{
				Recipe:  "retry-recipe",
				Timeout: 10 * time.Minute,
				RetryPolicy: &RetryPolicy{
					MaximumAttempts:    3,
					InitialInterval:    5 * time.Second,
					BackoffCoefficient: 2.0,
				},
				Inputs: map[string]interface{}{
					"data": "test",
				},
				Context: &RecipeContext{
					Recipe: RecipeInfo{
						Name:        "parent-recipe",
						ExecutionID: "parent-456",
					},
					Execution: ExecutionInfo{
						TaskQueue: "test-queue",
						Namespace: "test-namespace",
					},
				},
			},
			setupMocks: func(provider *MockClientProvider, client *MockTemporalClient, run *MockWorkflowRun) {
				provider.On("GetClient", mock.Anything).Return(client, nil)
				
				run.On("GetID").Return("retry-recipe-456")
				run.On("Get", mock.Anything, mock.Anything).Return(
					map[string]interface{}{
						"status": "processed",
					},
					nil,
				)
				
				client.On("ExecuteWorkflow",
					mock.Anything,
					mock.Anything,  // StartWorkflowOptions
					"retry-recipe",
					mock.Anything,
				).Return(run, nil)
			},
			expectedError:  false,
			expectedStatus: "completed",
		},
		{
			name: "client provider not initialized",
			input: RecipeActivity{
				Recipe: "test-recipe",
			},
			setupMocks:     nil, // No mocks for this test
			expectedError:  true,
			expectedStatus: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks based on test case
			if tt.setupMocks != nil {
				// Create mocks
				mockProvider := new(MockClientProvider)
				mockClient := new(MockTemporalClient)
				mockRun := new(MockWorkflowRun)
				
				tt.setupMocks(mockProvider, mockClient, mockRun)
				SetClientProvider(mockProvider)
				
				// Execute the activity
				ctx := context.Background()
				output, err := ExecuteRecipeActivity(ctx, tt.input)

				// Verify results
				if tt.expectedError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, output)
					assert.Equal(t, tt.expectedStatus, output.Status)
					assert.NotEmpty(t, output.ExecutionID)
				}

				// Verify mock expectations
				mockProvider.AssertExpectations(t)
				mockClient.AssertExpectations(t)
				mockRun.AssertExpectations(t)
			} else {
				// No mocks - provider is nil
				SetClientProvider(nil)
				
				// Execute the activity
				ctx := context.Background()
				_, err := ExecuteRecipeActivity(ctx, tt.input)

				// Verify results - expecting error when provider is nil
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not initialized")
			}
		})
	}
}

// TestParseRecipePath tests the parseRecipePath function
func TestParseRecipePath(t *testing.T) {
	tests := []struct {
		path            string
		expectedName    string
		expectedVersion string
	}{
		{
			path:            "simple-recipe",
			expectedName:    "simple-recipe",
			expectedVersion: "",
		},
		{
			path:            "data-processing/transform",
			expectedName:    "data-processing/transform",
			expectedVersion: "",
		},
		{
			path:            "recipe@v1.0.0",
			expectedName:    "recipe",
			expectedVersion: "v1.0.0",
		},
		{
			path:            "complex/path/recipe@v2.1.0",
			expectedName:    "complex/path/recipe",
			expectedVersion: "v2.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			name, version := parseRecipePath(tt.path)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}

// TestBuildRecipeContext tests the buildRecipeContext function
func TestBuildRecipeContext(t *testing.T) {
	// Override getEnv for testing
	originalGetEnv := getEnv
	defer func() { getEnv = originalGetEnv }()
	
	getEnv = func(key string) string {
		envMap := map[string]string{
			"ENVIRONMENT":         "production",
			"REGION":             "us-west-2",
			"CLUSTER":            "main-cluster",
			"HOSTNAME":           "worker-1",
			"TEMPORAL_NAMESPACE": "recipes",
			"TEMPORAL_TASK_QUEUE": "recipe-queue",
			"SERVICE_IDENTITY":   "recipe-service",
		}
		return envMap[key]
	}

	ctx := context.Background()
	recipeContext := buildRecipeContext(ctx, false)

	assert.NotNil(t, recipeContext)
	assert.Equal(t, "production", recipeContext.Environment.Name)
	assert.Equal(t, "us-west-2", recipeContext.Environment.Region)
	assert.Equal(t, "main-cluster", recipeContext.Environment.Cluster)
	assert.Equal(t, "worker-1", recipeContext.Execution.Host)
	assert.Equal(t, "recipes", recipeContext.Execution.Namespace)
	assert.Equal(t, "recipe-queue", recipeContext.Execution.TaskQueue)
	assert.Equal(t, "recipe-service", recipeContext.Auth.Identity)
}