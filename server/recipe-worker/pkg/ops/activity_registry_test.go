package ops

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// testActivity is a mock RegisterableOp for testing
var testActivity = recipeops.NewActivityMappedOp(
	recipeops.OpMetadata{
		Type:           "test_registry_activity",
		Name:           "Test Registry Activity",
		Description:    "A test activity for unit testing",
		Version:        "1.0.0",
		DefaultTimeout: 30 * time.Second,
	},
	func(ctx context.Context, input TestInput) (TestOutput, error) {
		return testExecute(ctx, TestConfig{}, input)
	},
)

func testExecute(ctx context.Context, config TestConfig, input TestInput) (TestOutput, error) {
	return TestOutput{
		Result:  input.Data + " processed",
		Success: true,
	}, nil
}

func TestActivityRegistration(t *testing.T) {
	registry, err := NewActivityRegistry()
	require.NoError(t, err)

	t.Run("successful registration", func(t *testing.T) {
		err := Register(registry, testActivity)
		assert.NoError(t, err)

		// Verify activity was registered
		registration, exists := registry.Get("test_registry_activity")
		assert.True(t, exists)
		assert.NotNil(t, registration.Activity)
		assert.NotNil(t, registration.InputSchema)
		assert.NotNil(t, registration.OutputSchema)
		assert.Equal(t, "test_registry_activity", registration.Metadata.Type)
	})

	t.Run("duplicate registration fails", func(t *testing.T) {
		err := Register(registry, testActivity)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("list registered activities", func(t *testing.T) {
		types := registry.List()
		assert.Contains(t, types, "test_registry_activity")
	})
}

// BadActivity for testing validation failures
type BadActivity struct{}

func (a *BadActivity) GetMetadata() recipeops.OpMetadata {
	return recipeops.OpMetadata{
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
			Name   string    `json:"name"`
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
	registry, err := NewActivityRegistry()
	require.NoError(t, err)

	err = Register(registry, testActivity)
	require.NoError(t, err)

	registration, exists := registry.Get("test_registry_activity")
	require.True(t, exists)

	// Config schema test removed - no longer part of ActivityRegistration

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

// TestActivityProvider tests are commented out to avoid circular import
// These tests should be moved to a separate test package or integration tests
// func TestActivityProvider(t *testing.T) {
// 	registry := NewActivityRegistry()
//
// 	err := Register[TestInput, TestOutput](registry, testActivity)
// 	require.NoError(t, err)
//
// 	registration, _ := registry.Get("test_activity")
// 	provider := worker.NewActivityProvider(testActivity, registration)
//
// 	t.Run("provider metadata", func(t *testing.T) {
// 		assert.Equal(t, "test_activity", provider.GetType())
// 		assert.Equal(t, "A test activity for unit testing", provider.GetDescription())
// 	})
//
// 	t.Run("provider execution", func(t *testing.T) {
// 		ctx := context.Background()
//
// 		config := map[string]interface{}{
// 			"url":     "https://example.com",
// 			"timeout": 30,
// 		}
//
// 		input := map[string]interface{}{
// 			"data": "test data",
// 			"size": 100,
// 		}
//
// 		result, err := provider.Execute(ctx, config, input)
// 		assert.NoError(t, err)
//
// 		output, ok := result.(map[string]interface{})
// 		assert.True(t, ok)
// 		assert.Equal(t, "test data processed", output["result"])
// 		assert.Equal(t, true, output["success"])
// 	})
//
// 	t.Run("provider schemas", func(t *testing.T) {
// 		configSchema, inputSchema, outputSchema := provider.GetSchemas()
// 		assert.NotNil(t, configSchema)
// 		assert.NotNil(t, inputSchema)
// 		assert.NotNil(t, outputSchema)
// 	})
// }
