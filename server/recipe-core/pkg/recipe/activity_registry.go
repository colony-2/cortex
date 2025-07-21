package recipe

import (
	"encoding/json"
	"fmt"
	"sync"
)

// JSONSchema represents a JSON Schema definition for validation
type JSONSchema map[string]interface{}

// ActivityTypeDefinition defines the structure and validation rules for a custom activity type
type ActivityTypeDefinition struct {
	// Type is the unique identifier for this activity type (e.g., "http", "database", "custom_api")
	Type string

	// Description provides human-readable information about what this activity type does
	Description string

	// InputSchema defines the JSON schema for validating activity inputs
	// Can be nil for activities that accept any input
	InputSchema JSONSchema

	// OutputSchema defines the JSON schema for validating activity outputs
	// Can be nil for activities that produce any output
	OutputSchema JSONSchema

	// ConfigSchema defines the JSON schema for validating the activity implementation config
	// This validates the config section in the YAML activity definition
	ConfigSchema JSONSchema

	// RequiredConfig specifies if the config section is mandatory
	RequiredConfig bool

	// AllowAdditionalConfig allows extra fields in config beyond those defined in ConfigSchema
	AllowAdditionalConfig bool

	// AllowAdditionalInputs allows extra fields in inputs beyond those defined in InputSchema
	AllowAdditionalInputs bool

	// AllowAdditionalOutputs allows extra fields in outputs beyond those defined in OutputSchema
	AllowAdditionalOutputs bool
}

// ActivityTypeRegistry manages registered activity types
type ActivityTypeRegistry struct {
	mu    sync.RWMutex
	types map[string]*ActivityTypeDefinition
}

// NewActivityTypeRegistry creates a new activity type registry with built-in types
func NewActivityTypeRegistry() *ActivityTypeRegistry {
	registry := &ActivityTypeRegistry{
		types: make(map[string]*ActivityTypeDefinition),
	}

	// Register built-in types
	registry.registerBuiltInTypes()

	return registry
}

// registerBuiltInTypes registers the default activity types
func (r *ActivityTypeRegistry) registerBuiltInTypes() {
	// HTTP activity type
	r.types["http"] = &ActivityTypeDefinition{
		Type:        "http",
		Description: "Makes HTTP requests to external APIs",
		ConfigSchema: JSONSchema{
			"type":     "object",
			"required": []string{"method", "url"},
			"properties": map[string]interface{}{
				"method": map[string]interface{}{
					"type": "string",
					"enum": []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"},
				},
				"url": map[string]interface{}{
					"type": "string",
					"format": "uri",
				},
				"headers": map[string]interface{}{
					"type": "object",
					"additionalProperties": map[string]interface{}{
						"type": "string",
					},
				},
				"body": map[string]interface{}{
					"type": "object",
				},
			},
		},
		RequiredConfig:        true,
		AllowAdditionalConfig: true,
		AllowAdditionalInputs: true,
		AllowAdditionalOutputs: true,
	}

	// AI Prompt activity type
	r.types["ai_prompt"] = &ActivityTypeDefinition{
		Type:        "ai_prompt",
		Description: "Executes AI/LLM prompts",
		ConfigSchema: JSONSchema{
			"type":     "object",
			"required": []string{"prompt"},
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type": "string",
				},
				"model": map[string]interface{}{
					"type": "string",
				},
				"provider": map[string]interface{}{
					"type": "string",
					"enum": []string{"openai", "anthropic", "gemini", "bedrock"},
				},
				"temperature": map[string]interface{}{
					"type":    "number",
					"minimum": 0,
					"maximum": 2,
				},
				"response_format": map[string]interface{}{
					"type": "string",
					"enum": []string{"text", "json"},
				},
			},
		},
		RequiredConfig:        true,
		AllowAdditionalConfig: true,
		AllowAdditionalInputs: true,
		AllowAdditionalOutputs: true,
	}

	// Function activity type
	r.types["function"] = &ActivityTypeDefinition{
		Type:        "function",
		Description: "Calls a registered function handler",
		ConfigSchema: JSONSchema{
			"type":     "object",
			"required": []string{"handler"},
			"properties": map[string]interface{}{
				"handler": map[string]interface{}{
					"type": "string",
				},
				"runtime": map[string]interface{}{
					"type": "string",
				},
			},
		},
		RequiredConfig:        true,
		AllowAdditionalConfig: false,
		AllowAdditionalInputs: true,
		AllowAdditionalOutputs: true,
	}

	// Script activity type (placeholder)
	r.types["script"] = &ActivityTypeDefinition{
		Type:           "script",
		Description:    "Executes external scripts or commands",
		RequiredConfig: true,
		ConfigSchema: JSONSchema{
			"type":     "object",
			"required": []string{"command"},
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type": "string",
				},
				"args": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
		AllowAdditionalConfig: true,
		AllowAdditionalInputs: true,
		AllowAdditionalOutputs: true,
	}

	// gRPC activity type (placeholder)
	r.types["grpc"] = &ActivityTypeDefinition{
		Type:           "grpc",
		Description:    "Makes gRPC calls to services",
		RequiredConfig: true,
		ConfigSchema: JSONSchema{
			"type":     "object",
			"required": []string{"service", "method"},
			"properties": map[string]interface{}{
				"service": map[string]interface{}{
					"type": "string",
				},
				"method": map[string]interface{}{
					"type": "string",
				},
				"address": map[string]interface{}{
					"type": "string",
				},
			},
		},
		AllowAdditionalConfig: true,
		AllowAdditionalInputs: true,
		AllowAdditionalOutputs: true,
	}
}

// RegisterActivityType registers a new activity type
func (r *ActivityTypeRegistry) RegisterActivityType(def *ActivityTypeDefinition) error {
	if def.Type == "" {
		return fmt.Errorf("activity type cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.types[def.Type]; exists {
		return fmt.Errorf("activity type %q already registered", def.Type)
	}

	// Validate schemas if provided
	if def.InputSchema != nil {
		if err := validateJSONSchema(def.InputSchema); err != nil {
			return fmt.Errorf("invalid input schema: %w", err)
		}
	}

	if def.OutputSchema != nil {
		if err := validateJSONSchema(def.OutputSchema); err != nil {
			return fmt.Errorf("invalid output schema: %w", err)
		}
	}

	if def.ConfigSchema != nil {
		if err := validateJSONSchema(def.ConfigSchema); err != nil {
			return fmt.Errorf("invalid config schema: %w", err)
		}
	}

	r.types[def.Type] = def
	return nil
}

// GetActivityType retrieves an activity type definition
func (r *ActivityTypeRegistry) GetActivityType(activityType string) (*ActivityTypeDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, exists := r.types[activityType]
	return def, exists
}

// ListActivityTypes returns all registered activity types
func (r *ActivityTypeRegistry) ListActivityTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.types))
	for t := range r.types {
		types = append(types, t)
	}
	return types
}

// ValidateActivityConfig validates an activity's configuration against its type definition
func (r *ActivityTypeRegistry) ValidateActivityConfig(activityType string, config map[string]interface{}) error {
	def, exists := r.GetActivityType(activityType)
	if !exists {
		return fmt.Errorf("unknown activity type: %s", activityType)
	}

	if def.RequiredConfig && len(config) == 0 {
		return fmt.Errorf("config is required for activity type %s", activityType)
	}

	if def.ConfigSchema != nil {
		return validateAgainstSchema(config, def.ConfigSchema, def.AllowAdditionalConfig)
	}

	return nil
}

// validateJSONSchema performs basic validation of a JSON schema
func validateJSONSchema(schema JSONSchema) error {
	// Basic validation - ensure it's a valid JSON schema structure
	schemaType, hasType := schema["type"]
	if !hasType {
		return fmt.Errorf("schema must have a 'type' field")
	}

	validTypes := []string{"object", "array", "string", "number", "integer", "boolean", "null"}
	typeStr, ok := schemaType.(string)
	if !ok {
		return fmt.Errorf("schema 'type' must be a string")
	}

	validType := false
	for _, vt := range validTypes {
		if typeStr == vt {
			validType = true
			break
		}
	}

	if !validType {
		return fmt.Errorf("invalid schema type: %s", typeStr)
	}

	return nil
}

// validateAgainstSchema validates data against a JSON schema
func validateAgainstSchema(data interface{}, schema JSONSchema, allowAdditional bool) error {
	// This is a simplified validation - in production, you'd use a proper JSON schema validator
	// For now, we'll do basic type checking
	
	schemaType, _ := schema["type"].(string)
	
	switch schemaType {
	case "object":
		dataMap, ok := data.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected object, got %T", data)
		}
		
		// Check required fields
		if required, ok := schema["required"].([]interface{}); ok {
			for _, field := range required {
				if fieldStr, ok := field.(string); ok {
					if _, exists := dataMap[fieldStr]; !exists {
						return fmt.Errorf("missing required field: %s", fieldStr)
					}
				}
			}
		} else if required, ok := schema["required"].([]string); ok {
			for _, field := range required {
				if _, exists := dataMap[field]; !exists {
					return fmt.Errorf("missing required field: %s", field)
				}
			}
		}
		
		// Check properties
		if properties, ok := schema["properties"].(map[string]interface{}); ok {
			for key, value := range dataMap {
				if propSchema, exists := properties[key]; exists {
					if propSchemaMap, ok := propSchema.(map[string]interface{}); ok {
						// Check enum values
						if enum, hasEnum := propSchemaMap["enum"]; hasEnum {
							if !validateEnum(value, enum) {
								return fmt.Errorf("field %s: value %v not in allowed enum values", key, value)
							}
						} else {
							if err := validateAgainstSchema(value, propSchemaMap, true); err != nil {
								return fmt.Errorf("field %s: %w", key, err)
							}
						}
					}
				} else if !allowAdditional {
					return fmt.Errorf("additional property not allowed: %s", key)
				}
			}
		}
		
	case "string":
		if _, ok := data.(string); !ok {
			return fmt.Errorf("expected string, got %T", data)
		}
		
	case "integer":
		switch data.(type) {
		case int, int64, int32, float64:
			// For float64, we should check if it's a whole number
			if f, ok := data.(float64); ok {
				if f != float64(int64(f)) {
					return fmt.Errorf("expected integer, got float: %v", f)
				}
			}
		default:
			return fmt.Errorf("expected integer, got %T", data)
		}
		
	case "number":
		switch data.(type) {
		case float64, float32, int, int64, int32:
			// Valid number types
		default:
			return fmt.Errorf("expected number, got %T", data)
		}
		
	case "array":
		if _, ok := data.([]interface{}); !ok {
			return fmt.Errorf("expected array, got %T", data)
		}
	}
	
	return nil
}

// validateEnum checks if a value is in the allowed enum values
func validateEnum(value interface{}, enum interface{}) bool {
	enumSlice, ok := enum.([]interface{})
	if !ok {
		if enumStrSlice, ok := enum.([]string); ok {
			valStr, isStr := value.(string)
			if !isStr {
				return false
			}
			for _, allowed := range enumStrSlice {
				if valStr == allowed {
					return true
				}
			}
		}
		return false
	}
	
	for _, allowed := range enumSlice {
		if value == allowed {
			return true
		}
	}
	return false
}

// MarshalJSON marshals the registry for debugging/inspection
func (r *ActivityTypeRegistry) MarshalJSON() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	return json.Marshal(r.types)
}