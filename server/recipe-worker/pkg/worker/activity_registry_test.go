package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/divisive-ai/vibethis/server/ops/pkg/types"
)

// Test types with proper JSON tags
type TestConfig struct {
	URL     string `json:"url"`
	Timeout int    `json:"timeout"`
}

type TestInput struct {
	Data string `json:"data"`
	Size int    `json:"size"`
}

type TestOutput struct {
	Result  string `json:"result"`
	Success bool   `json:"success"`
}

// Test types missing JSON tags (should fail registration)
type BadConfig struct {
	URL     string // Missing json tag
	Timeout int    `json:"timeout"`
}

type BadInput struct {
	Data string `json:"data"`
	Size int    // Missing json tag
}

// TestActivity implements RegisterableActivity for testing
type TestActivity struct{}

func (a *TestActivity) GetMetadata() types.ActivityMetadata {
	return types.ActivityMetadata{
		Type:           "test_activity",
		Name:           "Test Activity",
		Description:    "A test activity for unit testing",
		Version:        "1.0.0",
		DefaultTimeout: 30 * time.Second,
	}
}

func (a *TestActivity) Execute(ctx context.Context, config TestConfig, input TestInput) (TestOutput, error) {
	return TestOutput{
		Result:  input.Data + " processed",
		Success: true,
	}, nil
}

func TestActivityRegistration(t *testing.T) {
	registry := NewActivityRegistry()

	t.Run("successful registration", func(t *testing.T) {
		activity := &TestActivity{}
		err := Register[TestConfig, TestInput, TestOutput](registry, activity)
		assert.NoError(t, err)

		// Verify activity was registered
		registration, exists := registry.Get("test_activity")
		assert.True(t, exists)
		assert.NotNil(t, registration.Activity)
		assert.NotNil(t, registration.ConfigSchema)
		assert.NotNil(t, registration.InputSchema)
		assert.NotNil(t, registration.OutputSchema)
		assert.Equal(t, "test_activity", registration.Metadata.Type)
	})

	t.Run("duplicate registration fails", func(t *testing.T) {
		activity := &TestActivity{}
		err := Register[TestConfig, TestInput, TestOutput](registry, activity)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("list registered activities", func(t *testing.T) {
		types := registry.List()
		assert.Contains(t, types, "test_activity")
	})
}

// BadActivity for testing validation failures
type BadActivity struct{}

func (a *BadActivity) GetMetadata() types.ActivityMetadata {
	return types.ActivityMetadata{
		Type:           "bad_activity",
		Name:           "Bad Activity",
		Description:    "Test activity with bad types",
		Version:        "1.0.0",
		DefaultTimeout: 30 * time.Second,
	}
}

func (a *BadActivity) Execute(ctx context.Context, config BadConfig, input BadInput) (TestOutput, error) {
	return TestOutput{}, nil
}

func TestJSONTagValidation(t *testing.T) {
	// Test the schema generator directly for validation
	generator := NewDefaultSchemaGenerator()

	t.Run("missing config json tag fails", func(t *testing.T) {
		err := generator.ValidateStructTags(reflect.TypeOf(BadConfig{}))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required json tag")
		assert.Contains(t, err.Error(), "URL")
	})

	t.Run("missing input json tag fails", func(t *testing.T) {
		err := generator.ValidateStructTags(reflect.TypeOf(BadInput{}))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required json tag")
		assert.Contains(t, err.Error(), "Size")
	})

	t.Run("nested struct validation", func(t *testing.T) {
		type NestedBad struct {
			Field string // Missing json tag
		}
		
		type ConfigWithNested struct {
			Name   string     `json:"name"`
			Nested NestedBad `json:"nested"`
		}

		err := generator.ValidateStructTags(reflect.TypeOf(ConfigWithNested{}))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required json tag")
		assert.Contains(t, err.Error(), "Field")
	})

	t.Run("ignored fields are allowed", func(t *testing.T) {
		type ConfigWithIgnored struct {
			Public  string `json:"public"`
			Private string `json:"-"` // Explicitly ignored
		}

		err := generator.ValidateStructTags(reflect.TypeOf(ConfigWithIgnored{}))
		assert.NoError(t, err)
	})

	t.Run("all fields with tags pass", func(t *testing.T) {
		err := generator.ValidateStructTags(reflect.TypeOf(TestConfig{}))
		assert.NoError(t, err)
		
		err = generator.ValidateStructTags(reflect.TypeOf(TestInput{}))
		assert.NoError(t, err)
		
		err = generator.ValidateStructTags(reflect.TypeOf(TestOutput{}))
		assert.NoError(t, err)
	})
}

func TestSchemaGeneration(t *testing.T) {
	registry := NewActivityRegistry()
	activity := &TestActivity{}
	
	err := Register[TestConfig, TestInput, TestOutput](registry, activity)
	require.NoError(t, err)

	registration, exists := registry.Get("test_activity")
	require.True(t, exists)

	t.Run("config schema", func(t *testing.T) {
		schema := registration.ConfigSchema
		assert.NotNil(t, schema)
		
		// Convert to JSON and verify structure
		schemaJSON, err := json.Marshal(schema)
		require.NoError(t, err)
		
		var schemaMap map[string]interface{}
		err = json.Unmarshal(schemaJSON, &schemaMap)
		require.NoError(t, err)
		
		assert.Equal(t, "object", schemaMap["type"])
		properties, ok := schemaMap["properties"].(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, properties, "url")
		assert.Contains(t, properties, "timeout")
	})

	t.Run("input schema", func(t *testing.T) {
		schema := registration.InputSchema
		assert.NotNil(t, schema)
		
		schemaJSON, err := json.Marshal(schema)
		require.NoError(t, err)
		
		var schemaMap map[string]interface{}
		err = json.Unmarshal(schemaJSON, &schemaMap)
		require.NoError(t, err)
		
		properties, ok := schemaMap["properties"].(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, properties, "data")
		assert.Contains(t, properties, "size")
	})
}

func TestActivityProvider(t *testing.T) {
	registry := NewActivityRegistry()
	activity := &TestActivity{}
	
	err := Register[TestConfig, TestInput, TestOutput](registry, activity)
	require.NoError(t, err)

	registration, _ := registry.Get("test_activity")
	provider := NewActivityProvider[TestConfig, TestInput, TestOutput](activity, registration)

	t.Run("provider metadata", func(t *testing.T) {
		assert.Equal(t, "test_activity", provider.GetType())
		assert.Equal(t, "A test activity for unit testing", provider.GetDescription())
	})

	t.Run("provider execution", func(t *testing.T) {
		ctx := context.Background()
		
		config := map[string]interface{}{
			"url":     "https://example.com",
			"timeout": 30,
		}
		
		input := map[string]interface{}{
			"data": "test data",
			"size": 100,
		}
		
		result, err := provider.Execute(ctx, config, input)
		assert.NoError(t, err)
		
		output, ok := result.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "test data processed", output["result"])
		assert.Equal(t, true, output["success"])
	})

	t.Run("provider schemas", func(t *testing.T) {
		configSchema, inputSchema, outputSchema := provider.GetSchemas()
		assert.NotNil(t, configSchema)
		assert.NotNil(t, inputSchema)
		assert.NotNil(t, outputSchema)
	})
}