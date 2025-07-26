package worker_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	
	recipeworker "github.com/divisive-ai/vibethis/server/recipe-worker"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
)

// TestProviderRegistrationWithSchema tests that providers with schemas are registered in both registries
func TestProviderRegistrationWithSchema(t *testing.T) {
	// Create worker manager
	logger := zaptest.NewLogger(t)
	workerManager := worker.NewWorkerManager(logger, nil)
	
	// Create a provider with schema
	provider := &mockProviderWithSchema{
		activityType: "custom_api",
		description:  "Custom API integration",
		configSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"endpoint": map[string]interface{}{"type": "string"},
			},
			"required": []string{"endpoint"},
		},
	}
	
	// Register provider
	err := workerManager.RegisterProvider(provider)
	require.NoError(t, err)
	
	// Verify it was registered in the activity type registry
	activityTypeRegistry := workerManager.GetActivityTypeRegistry()
	activityType, exists := activityTypeRegistry.GetActivityType("custom_api")
	assert.True(t, exists)
	assert.Equal(t, "custom_api", activityType.Type)
	assert.Equal(t, "Custom API integration", activityType.Description)
	assert.NotNil(t, activityType.ConfigSchema)
}

// TestProviderRegistrationWithoutSchema tests that providers without schemas fail registration
func TestProviderRegistrationWithoutSchema(t *testing.T) {
	// Create worker manager
	logger := zaptest.NewLogger(t)
	workerManager := worker.NewWorkerManager(logger, nil)
	
	// Create a provider with no schemas
	provider := &mockProvider{
		activityType: "basic_activity",
	}
	
	// Register provider should fail
	err := workerManager.RegisterProvider(provider)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must provide at least one schema")
	
	// Verify it was NOT registered in the activity type registry
	activityTypeRegistry := workerManager.GetActivityTypeRegistry()
	_, exists := activityTypeRegistry.GetActivityType("basic_activity")
	assert.False(t, exists)
}

// TestProviderWithOnlyOutputSchema tests that providers can register with just output schema
func TestProviderWithOnlyOutputSchema(t *testing.T) {
	// Create worker manager
	logger := zaptest.NewLogger(t)
	workerManager := worker.NewWorkerManager(logger, nil)
	
	// Create a provider with only output schema
	provider := &mockProviderWithSchema{
		activityType: "output_only",
		description:  "Provider with output schema only",
		// Only output schema provided
		outputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"result": map[string]interface{}{"type": "string"},
			},
		},
	}
	
	// Register provider should succeed
	err := workerManager.RegisterProvider(provider)
	require.NoError(t, err)
	
	// Verify it was registered in the activity type registry
	activityTypeRegistry := workerManager.GetActivityTypeRegistry()
	activityType, exists := activityTypeRegistry.GetActivityType("output_only")
	assert.True(t, exists)
	assert.NotNil(t, activityType.OutputSchema)
	assert.Nil(t, activityType.ConfigSchema)
	assert.Nil(t, activityType.InputSchema)
}

// mockProvider is a basic provider that returns no schemas
type mockProvider struct {
	activityType string
}

func (m *mockProvider) GetType() string {
	return m.activityType
}

func (m *mockProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "ok"}, nil
}

func (m *mockProvider) GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{}) {
	return nil, nil, nil
}

func (m *mockProvider) GetDescription() string {
	return "Mock provider"
}

func (m *mockProvider) GetSchemaOptions() recipeworker.SchemaOptions {
	return recipeworker.SchemaOptions{}
}

// mockProviderWithSchema implements ActivityProvider with schema support
type mockProviderWithSchema struct {
	activityType  string
	description   string
	configSchema  map[string]interface{}
	inputSchema   map[string]interface{}
	outputSchema  map[string]interface{}
	schemaOptions recipeworker.SchemaOptions
}

func (m *mockProviderWithSchema) GetType() string {
	return m.activityType
}

func (m *mockProviderWithSchema) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "ok"}, nil
}

func (m *mockProviderWithSchema) GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{}) {
	return m.configSchema, m.inputSchema, m.outputSchema
}

func (m *mockProviderWithSchema) GetDescription() string {
	return m.description
}

func (m *mockProviderWithSchema) GetSchemaOptions() recipeworker.SchemaOptions {
	return m.schemaOptions
}