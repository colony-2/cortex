package worker_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	worker "github.com/vibethis/server/recipe-worker"
)

// TestProviderRegistry tests the provider registry functionality
func TestProviderRegistry(t *testing.T) {
	t.Run("Register and retrieve provider", func(t *testing.T) {
		registry := worker.NewProviderRegistry()
		
		// Create a mock provider
		provider := &mockProvider{activityType: "test_activity"}
		
		// Register the provider
		err := registry.Register(provider)
		require.NoError(t, err)
		
		// Check if provider exists
		assert.True(t, registry.Has("test_activity"))
		
		// Retrieve the provider
		retrieved, err := registry.Get("test_activity")
		require.NoError(t, err)
		assert.Equal(t, provider, retrieved)
	})
	
	t.Run("Register duplicate provider", func(t *testing.T) {
		registry := worker.NewProviderRegistry()
		
		// Register first provider
		provider1 := &mockProvider{activityType: "test_activity"}
		err := registry.Register(provider1)
		require.NoError(t, err)
		
		// Try to register duplicate
		provider2 := &mockProvider{activityType: "test_activity"}
		err = registry.Register(provider2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
	
	t.Run("Get non-existent provider", func(t *testing.T) {
		registry := worker.NewProviderRegistry()
		
		// Try to get non-existent provider
		_, err := registry.Get("non_existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no provider registered")
	})
	
	t.Run("Register provider with empty type", func(t *testing.T) {
		registry := worker.NewProviderRegistry()
		
		// Create provider with empty type
		provider := &mockProvider{activityType: ""}
		
		// Should fail to register
		err := registry.Register(provider)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non-empty type")
	})
	
	t.Run("List registered types", func(t *testing.T) {
		registry := worker.NewProviderRegistry()
		
		// Register multiple providers
		providers := []string{"http", "database", "custom_api"}
		for _, actType := range providers {
			provider := &mockProvider{activityType: actType}
			err := registry.Register(provider)
			require.NoError(t, err)
		}
		
		// Get list of types
		types := registry.ListTypes()
		assert.Len(t, types, 3)
		
		// Check all types are present (order not guaranteed)
		typeMap := make(map[string]bool)
		for _, t := range types {
			typeMap[t] = true
		}
		for _, expected := range providers {
			assert.True(t, typeMap[expected], "Expected type %s not found", expected)
		}
	})
}

// TestTypedProvider tests the typed provider functionality
func TestTypedProvider(t *testing.T) {
	type TestConfig struct {
		URL    string
		Method string
	}
	
	type TestInput struct {
		Body string
	}
	
	type TestOutput struct {
		Status int
		Result string
	}
	
	t.Run("Typed provider execution", func(t *testing.T) {
		// Create typed provider
		provider := worker.NewTypedProvider("test_typed", 
			func(ctx context.Context, config TestConfig, input TestInput) (TestOutput, error) {
				return TestOutput{
					Status: 200,
					Result: config.URL + " - " + input.Body,
				}, nil
			})
		
		// Execute with correct types
		result, err := provider.Execute(context.Background(),
			TestConfig{URL: "http://example.com", Method: "GET"},
			TestInput{Body: "test body"})
		
		require.NoError(t, err)
		output, ok := result.(TestOutput)
		require.True(t, ok)
		assert.Equal(t, 200, output.Status)
		assert.Equal(t, "http://example.com - test body", output.Result)
	})
	
	t.Run("Typed provider with wrong argument count", func(t *testing.T) {
		provider := worker.NewTypedProvider("test_typed",
			func(ctx context.Context, config TestConfig, input TestInput) (TestOutput, error) {
				return TestOutput{}, nil
			})
		
		// Execute with wrong number of arguments
		_, err := provider.Execute(context.Background(), TestConfig{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected 2 arguments")
	})
	
	t.Run("Typed provider with wrong types", func(t *testing.T) {
		provider := worker.NewTypedProvider("test_typed",
			func(ctx context.Context, config TestConfig, input TestInput) (TestOutput, error) {
				return TestOutput{}, nil
			})
		
		// Execute with wrong types
		_, err := provider.Execute(context.Background(), 
			"wrong type", // Should be TestConfig
			TestInput{Body: "test"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config type")
	})
}

// TestRegisterTyped tests the RegisterTyped helper function
func TestRegisterTyped(t *testing.T) {
	registry := worker.NewProviderRegistry()
	
	type MyConfig struct {
		Setting string
	}
	
	type MyInput struct {
		Data string
	}
	
	type MyOutput struct {
		Result string
	}
	
	// Register typed provider
	err := worker.RegisterTyped(registry, "my_activity",
		func(ctx context.Context, config MyConfig, input MyInput) (MyOutput, error) {
			return MyOutput{Result: config.Setting + ":" + input.Data}, nil
		})
	
	require.NoError(t, err)
	
	// Verify it was registered
	assert.True(t, registry.Has("my_activity"))
	
	// Get and execute
	provider, err := registry.Get("my_activity")
	require.NoError(t, err)
	
	result, err := provider.Execute(context.Background(),
		MyConfig{Setting: "test"},
		MyInput{Data: "data"})
	
	require.NoError(t, err)
	output, ok := result.(MyOutput)
	require.True(t, ok)
	assert.Equal(t, "test:data", output.Result)
}

// mockProvider is a simple mock implementation of ActivityProvider
type mockProvider struct {
	activityType string
	executeFunc  func(ctx context.Context, args ...interface{}) (interface{}, error)
}

func (m *mockProvider) GetType() string {
	return m.activityType
}

func (m *mockProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, args...)
	}
	return map[string]interface{}{"status": "mock"}, nil
}

func (m *mockProvider) GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{}) {
	// Return basic schemas for testing
	return map[string]interface{}{"type": "object"},
		map[string]interface{}{"type": "object"},
		map[string]interface{}{"type": "object"}
}

func (m *mockProvider) GetDescription() string {
	return "Mock provider for testing"
}

func (m *mockProvider) GetSchemaOptions() worker.SchemaOptions {
	return worker.SchemaOptions{}
}