package recipe

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestRecipeActivityWrapper_GetMetadata tests the GetMetadata method
func TestRecipeActivityWrapper_GetMetadata(t *testing.T) {
	wrapper := NewRecipeActivity()
	metadata := wrapper.GetMetadata()

	assert.Equal(t, "recipe", metadata.Type)
	assert.Equal(t, "Recipe Invocation", metadata.Name)
	assert.Contains(t, metadata.Description, "child workflow")
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, 35*time.Minute, metadata.DefaultTimeout)
	assert.NotNil(t, metadata.RetryPolicy)
	assert.Equal(t, int32(3), metadata.RetryPolicy.MaximumAttempts)
	assert.Equal(t, 5*time.Second, metadata.RetryPolicy.InitialInterval)
	assert.Equal(t, 2.0, metadata.RetryPolicy.BackoffCoefficient)
}

// TestRecipeActivityWrapper_Execute tests the Execute method
func TestRecipeActivityWrapper_Execute(t *testing.T) {
	// Override getEnv for testing
	originalGetEnv := getEnv
	defer func() { getEnv = originalGetEnv }()
	
	getEnv = func(key string) string {
		return os.Getenv(key)
	}

	tests := []struct {
		name          string
		config        RecipeConfig
		input         RecipeInput
		setupMocks    func()
		expectedError bool
		errorContains string
	}{
		{
			name: "successful execution",
			config: RecipeConfig{
				Recipe:  "test-recipe",
				Timeout: "30m",
			},
			input: RecipeInput{
				"param1": "value1",
				"param2": 42,
			},
			setupMocks: func() {
				mockProvider := new(MockClientProvider)
				mockClient := new(MockTemporalClient)
				mockRun := new(MockWorkflowRun)

				mockProvider.On("GetClient", mock.Anything).Return(mockClient, nil)
				mockRun.On("GetID").Return("test-execution-123")
				mockRun.On("Get", mock.Anything, mock.Anything).Return(
					map[string]interface{}{
						"result": "success",
					},
					nil,
				)
				
				mockClient.On("ExecuteWorkflow",
					mock.Anything,
					mock.Anything,  // StartWorkflowOptions
					"test-recipe",
					mock.Anything,
				).Return(mockRun, nil)

				SetClientProvider(mockProvider)
			},
			expectedError: false,
		},
		{
			name: "recipe with version",
			config: RecipeConfig{
				Recipe:  "versioned-recipe@v2.0.0",
				Timeout: "15m",
			},
			input: RecipeInput{
				"data": "test-data",
			},
			setupMocks: func() {
				mockProvider := new(MockClientProvider)
				mockClient := new(MockTemporalClient)
				mockRun := new(MockWorkflowRun)

				mockProvider.On("GetClient", mock.Anything).Return(mockClient, nil)
				mockRun.On("GetID").Return("versioned-execution-456")
				mockRun.On("Get", mock.Anything, mock.Anything).Return(
					map[string]interface{}{
						"version": "v2.0.0",
						"status":  "processed",
					},
					nil,
				)
				
				mockClient.On("ExecuteWorkflow",
					mock.Anything,
					mock.Anything,
					"versioned-recipe",
					mock.Anything,
				).Return(mockRun, nil)

				SetClientProvider(mockProvider)
			},
			expectedError: false,
		},
		{
			name: "recipe with retry policy",
			config: RecipeConfig{
				Recipe:  "retry-recipe",
				Timeout: "10m",
				RetryPolicy: &RetryPolicyConfig{
					MaximumAttempts:    5,
					InitialInterval:    "10s",
					BackoffCoefficient: 1.5,
					MaximumInterval:    "2m",
				},
			},
			input: RecipeInput{
				"operation": "process",
			},
			setupMocks: func() {
				mockProvider := new(MockClientProvider)
				mockClient := new(MockTemporalClient)
				mockRun := new(MockWorkflowRun)

				mockProvider.On("GetClient", mock.Anything).Return(mockClient, nil)
				mockRun.On("GetID").Return("retry-execution-789")
				mockRun.On("Get", mock.Anything, mock.Anything).Return(
					map[string]interface{}{
						"attempts": 2,
						"result":   "completed",
					},
					nil,
				)
				
				mockClient.On("ExecuteWorkflow",
					mock.Anything,
					mock.Anything,
					"retry-recipe",
					mock.Anything,
				).Return(mockRun, nil)

				SetClientProvider(mockProvider)
			},
			expectedError: false,
		},
		{
			name: "invalid timeout format",
			config: RecipeConfig{
				Recipe:  "test-recipe",
				Timeout: "invalid",
			},
			input: RecipeInput{},
			setupMocks: func() {
				// No mocks needed - should fail on timeout parsing
			},
			expectedError: true,
			errorContains: "invalid timeout format",
		},
		{
			name: "invalid retry interval format",
			config: RecipeConfig{
				Recipe: "test-recipe",
				RetryPolicy: &RetryPolicyConfig{
					InitialInterval: "not-a-duration",
				},
			},
			input: RecipeInput{},
			setupMocks: func() {
				// No mocks needed - should fail on duration parsing
			},
			expectedError: true,
			errorContains: "invalid initial_interval format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			// Create wrapper and execute
			wrapper := NewRecipeActivity()
			ctx := context.Background()
			output, err := wrapper.Execute(ctx, tt.config, tt.input)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, output)
				assert.NotEmpty(t, output.ExecutionID)
				assert.NotNil(t, output.Metadata)
			}
		})
	}
}

// TestGetConfigSchema tests the GetConfigSchema function
func TestGetConfigSchema(t *testing.T) {
	schema := GetConfigSchema()
	
	assert.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])
	
	properties, ok := schema["properties"].(map[string]interface{})
	assert.True(t, ok)
	
	// Check recipe property
	recipeProp, ok := properties["recipe"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "string", recipeProp["type"])
	
	// Check retry_policy property
	retryProp, ok := properties["retry_policy"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "object", retryProp["type"])
	
	// Check required fields
	required, ok := schema["required"].([]string)
	assert.True(t, ok)
	assert.Contains(t, required, "recipe")
}

// TestGetInputSchema tests the GetInputSchema function
func TestGetInputSchema(t *testing.T) {
	schema := GetInputSchema()
	
	assert.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, true, schema["additionalProperties"])
}

// TestGetOutputSchema tests the GetOutputSchema function
func TestGetOutputSchema(t *testing.T) {
	schema := GetOutputSchema()
	
	assert.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])
	
	properties, ok := schema["properties"].(map[string]interface{})
	assert.True(t, ok)
	
	// Check key output properties
	executionIDProp, ok := properties["execution_id"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "string", executionIDProp["type"])
	
	statusProp, ok := properties["status"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "string", statusProp["type"])
	assert.Contains(t, statusProp["enum"], "completed")
	assert.Contains(t, statusProp["enum"], "failed")
	
	metadataProp, ok := properties["execution_metadata"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "object", metadataProp["type"])
}

// TestRecipeContext_JSON tests JSON marshaling/unmarshaling of RecipeContext
func TestRecipeContext_JSON(t *testing.T) {
	original := &RecipeContext{
		Recipe: RecipeInfo{
			Name:              "test-recipe",
			Version:           "1.0.0",
			ExecutionID:       "exec-123",
			ParentExecutionID: "parent-456",
		},
		Environment: EnvironmentInfo{
			Name:    "production",
			Region:  "us-west-2",
			Cluster: "main",
		},
		Execution: ExecutionInfo{
			Host:      "worker-1",
			Namespace: "default",
			TaskQueue: "recipe-queue",
			StartedAt: time.Now(),
			Timeout:   30 * time.Minute,
		},
		Auth: AuthInfo{
			Identity: "service-account",
			Token:    "secret-token",
		},
	}

	// Marshal to JSON
	data, err := original.MarshalJSON()
	assert.NoError(t, err)
	assert.NotNil(t, data)

	// Unmarshal back
	var restored RecipeContext
	err = restored.UnmarshalJSON(data)
	assert.NoError(t, err)

	// Verify fields
	assert.Equal(t, original.Recipe.Name, restored.Recipe.Name)
	assert.Equal(t, original.Recipe.Version, restored.Recipe.Version)
	assert.Equal(t, original.Recipe.ExecutionID, restored.Recipe.ExecutionID)
	assert.Equal(t, original.Environment.Name, restored.Environment.Name)
	assert.Equal(t, original.Environment.Region, restored.Environment.Region)
	assert.Equal(t, original.Execution.Namespace, restored.Execution.Namespace)
	assert.Equal(t, original.Auth.Identity, restored.Auth.Identity)
}

// TestGenerateExecutionID tests the generateExecutionID function
func TestGenerateExecutionID(t *testing.T) {
	id1 := generateExecutionID()
	id2 := generateExecutionID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2) // Should generate unique IDs
	assert.Contains(t, id1, "recipe-")
	assert.Contains(t, id2, "recipe-")
}

// TestNewRecipeWorkflowActivity tests the NewRecipeWorkflowActivity function
func TestNewRecipeWorkflowActivity(t *testing.T) {
	wrapper := NewRecipeWorkflowActivity()
	
	assert.NotNil(t, wrapper)
	
	// Verify it's configured for workflow context
	wrapperImpl, ok := wrapper.(*RecipeActivityWrapper)
	assert.True(t, ok)
	assert.True(t, wrapperImpl.isWorkflowContext)
}

// BenchmarkRecipeExecution benchmarks the recipe execution
func BenchmarkRecipeExecution(b *testing.B) {
	// Setup mock provider
	mockProvider := new(MockClientProvider)
	mockClient := new(MockTemporalClient)
	mockRun := new(MockWorkflowRun)

	mockProvider.On("GetClient", mock.Anything).Return(mockClient, nil)
	mockRun.On("GetID").Return("bench-execution")
	mockRun.On("Get", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "success"},
		nil,
	)
	
	mockClient.On("ExecuteWorkflow",
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(mockRun, nil)

	SetClientProvider(mockProvider)

	wrapper := NewRecipeActivity()
	config := RecipeConfig{
		Recipe:  "benchmark-recipe",
		Timeout: "10m",
	}
	input := RecipeInput{
		"data": "test",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = wrapper.Execute(ctx, config, input)
	}
}