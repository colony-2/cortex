package recipe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActivityTypeRegistry(t *testing.T) {

	t.Run("RegisterCustomType", func(t *testing.T) {
		registry := NewActivityTypeRegistry()

		// Register a custom type
		customType := &ActivityTypeDefinition{
			Type:        "custom_api",
			Description: "Custom API integration",
			ConfigSchema: JSONSchema{
				"type":     "object",
				"required": []string{"endpoint"},
				"properties": map[string]interface{}{
					"endpoint": map[string]interface{}{
						"type": "string",
					},
				},
			},
			RequiredConfig: true,
		}

		err := registry.RegisterActivityType(customType)
		if err != nil {
			t.Fatalf("Failed to register custom type: %v", err)
		}

		// Verify it was registered
		retrieved, exists := registry.GetActivityType("custom_api")
		if !exists {
			t.Fatal("Custom type not found after registration")
		}

		if retrieved.Description != customType.Description {
			t.Errorf("Retrieved type description mismatch: got %s, want %s",
				retrieved.Description, customType.Description)
		}
	})

	t.Run("FlexibleSchema", func(t *testing.T) {
		registry := NewActivityTypeRegistry()

		// Register a flexible type (no schema)
		flexibleType := &ActivityTypeDefinition{
			Type:                  "flexible",
			Description:           "Accepts any config",
			ConfigSchema:          nil,
			RequiredConfig:        false,
			AllowAdditionalConfig: true,
		}

		err := registry.RegisterActivityType(flexibleType)
		if err != nil {
			t.Fatalf("Failed to register flexible type: %v", err)
		}

		// Any config should be valid
		configs := []map[string]interface{}{
			{},
			{"any": "value"},
			{"nested": map[string]interface{}{"data": true}},
		}

		for _, config := range configs {
			err := registry.ValidateActivityConfig("flexible", config)
			if err != nil {
				t.Errorf("Flexible type rejected valid config: %v", err)
			}
		}
	})

	t.Run("StrictSchema", func(t *testing.T) {
		registry := NewActivityTypeRegistry()

		// Register a strict type
		strictType := &ActivityTypeDefinition{
			Type:        "strict",
			Description: "Very strict validation",
			ConfigSchema: JSONSchema{
				"type":     "object",
				"required": []string{"id", "name"},
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
					},
					"name": map[string]interface{}{
						"type": "string",
					},
				},
				"additionalProperties": false,
			},
			RequiredConfig:        true,
			AllowAdditionalConfig: false,
		}

		err := registry.RegisterActivityType(strictType)
		if err != nil {
			t.Fatalf("Failed to register strict type: %v", err)
		}

		// Test valid config
		validConfig := map[string]interface{}{
			"id":   123,
			"name": "test",
		}
		err = registry.ValidateActivityConfig("strict", validConfig)
		if err != nil {
			t.Errorf("Strict type rejected valid config: %v", err)
		}

		// Test invalid configs
		invalidConfigs := []map[string]interface{}{
			{},                                     // missing required fields
			{"id": "not-a-number", "name": "test"}, // wrong type
			{"id": 123, "name": "test", "extra": "field"}, // additional property
		}

		for _, config := range invalidConfigs {
			err := registry.ValidateActivityConfig("strict", config)
			if err == nil {
				t.Errorf("Strict type accepted invalid config: %v", config)
			}
		}
	})

	t.Run("ListActivityTypes", func(t *testing.T) {
		registry := NewActivityTypeRegistry()

		// Add a custom type
		customType := &ActivityTypeDefinition{
			Type:        "my_custom",
			Description: "Custom type",
		}
		require.NoError(t, registry.RegisterActivityType(customType))

		types := registry.ListActivityTypes()

		// Should have built-in types + custom
		expectedTypes := map[string]bool{
			"my_custom": true,
		}

		if len(types) != len(expectedTypes) {
			t.Errorf("Expected %d types, got %d", len(expectedTypes), len(types))
		}

		for _, typeName := range types {
			if !expectedTypes[typeName] {
				t.Errorf("Unexpected type in list: %s", typeName)
			}
		}
	})
}
