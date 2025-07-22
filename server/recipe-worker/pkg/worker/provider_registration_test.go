package worker_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	
	recipeworker "github.com/vibethis/server/recipe-worker"
	"github.com/vibethis/server/recipe-worker/pkg/worker"
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

// TestProviderRegistrationWithoutSchema tests that providers without schemas are only registered in provider registry
func TestProviderRegistrationWithoutSchema(t *testing.T) {
	// Create worker manager
	logger := zaptest.NewLogger(t)
	workerManager := worker.NewWorkerManager(logger, nil)
	
	// Create a basic provider (no schema)
	provider := &mockProvider{
		activityType: "basic_activity",
	}
	
	// Register provider
	err := workerManager.RegisterProvider(provider)
	require.NoError(t, err)
	
	// Verify it was NOT registered in the activity type registry
	activityTypeRegistry := workerManager.GetActivityTypeRegistry()
	_, exists := activityTypeRegistry.GetActivityType("basic_activity")
	assert.False(t, exists)
}

// TestBuiltInProviderSkipsTypeRegistration tests that built-in providers with nil schemas skip type registration
func TestBuiltInProviderSkipsTypeRegistration(t *testing.T) {
	// Create worker manager
	logger := zaptest.NewLogger(t)
	workerManager := worker.NewWorkerManager(logger, nil)
	
	// Create a provider that returns nil schemas (like built-in types)
	provider := &mockProviderWithSchema{
		activityType: "http", // Pretend to be HTTP provider
		description:  "HTTP requests",
		// Leave schemas nil
	}
	
	// Get current count of activity types
	activityTypeRegistry := workerManager.GetActivityTypeRegistry()
	initialTypes := activityTypeRegistry.ListActivityTypes()
	
	// Register provider
	err := workerManager.RegisterProvider(provider)
	require.NoError(t, err)
	
	// Verify no new types were added (since http is already built-in)
	finalTypes := activityTypeRegistry.ListActivityTypes()
	assert.Equal(t, len(initialTypes), len(finalTypes))
}

// mockProvider is a basic provider without schema support
type mockProvider struct {
	activityType string
}

func (m *mockProvider) GetType() string {
	return m.activityType
}

func (m *mockProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "ok"}, nil
}

// mockProviderWithSchema implements ActivityProviderWithSchema
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