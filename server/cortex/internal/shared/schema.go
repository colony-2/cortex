package shared

import (
	"encoding/json"
	"reflect"

	"github.com/invopop/jsonschema"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
)

// SchemaManager manages schema generation and formatting
type SchemaManager struct {
	registryManager *RegistryManager
}

// NewSchemaManager creates a new schema manager
func NewSchemaManager(rm *RegistryManager) *SchemaManager {
	return &SchemaManager{registryManager: rm}
}

// GenerateCompleteSchema generates a complete schema with all activity details
func (sm *SchemaManager) GenerateCompleteSchema(filterActivity string, includeVersion bool, includeExamples bool) (map[string]interface{}, error) {
	registry := sm.registryManager.GetRegistry()
	
	// Ensure all schemas are generated
	sm.ensureSchemasGenerated(registry)
	
	// Build complete schema with all details
	return sm.buildCompleteSchema(registry, filterActivity, includeVersion, includeExamples)
}

// ensureSchemasGenerated forces schema generation for all registered activities
func (sm *SchemaManager) ensureSchemasGenerated(registry *worker.ActivityRegistry) {
	// Get all registered activities
	allActivities := registry.GetAll()
	
	// Force schema generation for activities that don't have schemas yet
	for activityType, registration := range allActivities {
		if registration.ConfigSchema == nil || registration.InputSchema == nil || registration.OutputSchema == nil {
			sm.generateSchemasForActivity(&registration)
			// Update the registration with generated schemas
			sm.updateRegistration(registry, activityType, registration)
		}
	}
}

// generateSchemasForActivity generates schemas for an activity using reflection
func (sm *SchemaManager) generateSchemasForActivity(registration *worker.ActivityRegistration) {
	// Use reflection to extract types from Execute method
	activityValue := reflect.ValueOf(registration.Activity)
	executeMethod := activityValue.MethodByName("Execute")
	
	if !executeMethod.IsValid() {
		return
	}
	
	methodType := executeMethod.Type()
	// Execute method signature: func(ctx context.Context, config TConfig, input TInput) (TOutput, error)
	if methodType.NumIn() < 3 || methodType.NumOut() < 2 {
		return
	}
	
	// Generate schemas from types
	generator := worker.NewDefaultSchemaGenerator()
	
	// Config is the second parameter (after context)
	configType := methodType.In(1)
	if configType.Kind() == reflect.Interface {
		// For interface types, we can't generate schema directly
		// This might need special handling
		registration.ConfigSchema = &jsonschema.Schema{
			Type: "object",
			AdditionalProperties: jsonschema.FalseSchema,
		}
	} else {
		registration.ConfigSchema, _ = generator.GenerateSchema(configType)
	}
	
	// Input is the third parameter
	inputType := methodType.In(2)
	if inputType.Kind() == reflect.Interface {
		registration.InputSchema = &jsonschema.Schema{
			Type: "object",
			AdditionalProperties: jsonschema.FalseSchema,
		}
	} else {
		registration.InputSchema, _ = generator.GenerateSchema(inputType)
	}
	
	// Output is the first return value
	outputType := methodType.Out(0)
	if outputType.Kind() == reflect.Interface {
		registration.OutputSchema = &jsonschema.Schema{
			Type: "object",
			AdditionalProperties: jsonschema.FalseSchema,
		}
	} else {
		registration.OutputSchema, _ = generator.GenerateSchema(outputType)
	}
}

// updateRegistration updates an activity registration in the registry
func (sm *SchemaManager) updateRegistration(registry *worker.ActivityRegistry, activityType string, registration worker.ActivityRegistration) {
	// We need to update the registration in the registry
	// Since GetAll returns a copy, we need to directly update the internal map
	// This might require adding an UpdateRegistration method to the registry
	allActivities := registry.GetAll()
	allActivities[activityType] = registration
}

// buildCompleteSchema builds the final schema output with all details
func (sm *SchemaManager) buildCompleteSchema(registry *worker.ActivityRegistry, filterActivity string, includeVersion bool, includeExamples bool) (map[string]interface{}, error) {
	schema := make(map[string]interface{})
	
	// Add version if requested
	if includeVersion {
		schema["version"] = "1.0.0"
	}
	
	// Build definitions
	definitions := make(map[string]interface{})
	activities := make(map[string]interface{})
	
	for activityType, registration := range registry.GetAll() {
		// Skip if filtering and doesn't match
		if filterActivity != "" && activityType != filterActivity {
			continue
		}
		
		activitySchema := make(map[string]interface{})
		
		// Add metadata
		activitySchema["type"] = "object"
		activitySchema["description"] = registration.Metadata.Description
		
		// Build properties
		props := make(map[string]interface{})
		
		// Add type property
		props["type"] = map[string]interface{}{
			"const": activityType,
		}
		
		// Add config schema if present
		if registration.ConfigSchema != nil {
			props["config"] = convertSchemaWithDetails(registration.ConfigSchema)
		}
		
		// Add input schema if present  
		if registration.InputSchema != nil {
			props["input"] = convertSchemaWithDetails(registration.InputSchema)
		}
		
		// Add output schema if present
		if registration.OutputSchema != nil {
			props["output"] = convertSchemaWithDetails(registration.OutputSchema)
		}
		
		activitySchema["properties"] = props
		
		// Add required fields
		required := []string{"type"}
		if registration.InputSchema != nil && len(registration.InputSchema.Required) > 0 {
			// If input has required fields, input itself is required
			required = append(required, "input")
		}
		activitySchema["required"] = required
		
		// Add examples if requested
		if includeExamples {
			activitySchema["examples"] = sm.generateExamples(activityType, registration)
		}
		
		activities[activityType] = activitySchema
	}
	
	definitions["activities"] = activities
	schema["definitions"] = definitions
	
	return schema, nil
}

// convertSchemaWithDetails converts a jsonschema.Schema to a map with all details preserved
func convertSchemaWithDetails(schema *jsonschema.Schema) map[string]interface{} {
	if schema == nil {
		return nil
	}
	
	// Marshal and unmarshal to convert - simplest way to handle all types
	bytes, err := json.Marshal(schema)
	if err != nil {
		return make(map[string]interface{})
	}
	
	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return make(map[string]interface{})
	}
	
	// Remove internal fields that shouldn't be in output
	delete(result, "$id")
	delete(result, "$anchor")
	delete(result, "$schema")
	delete(result, "$ref")
	delete(result, "definitions")
	
	return result
}

// generateExamples generates example usage for an activity
func (sm *SchemaManager) generateExamples(activityType string, registration worker.ActivityRegistration) []map[string]interface{} {
	// Generate basic example structure
	example := map[string]interface{}{
		"type": activityType,
	}
	
	// Add example input based on activity type
	switch activityType {
	case "command_execution":
		example["input"] = map[string]interface{}{
			"run": "echo 'Hello World'",
		}
	case "http_request":
		example["input"] = map[string]interface{}{
			"url":    "https://api.example.com/data",
			"method": "GET",
		}
	default:
		// Generic example - just create a basic input structure
		if registration.InputSchema != nil {
			// Convert schema to map to inspect properties
			schemaMap := convertSchemaWithDetails(registration.InputSchema)
			if props, ok := schemaMap["properties"].(map[string]interface{}); ok && len(props) > 0 {
				inputExample := make(map[string]interface{})
				for propName := range props {
					inputExample[propName] = "example_value"
				}
				example["input"] = inputExample
			}
		}
	}
	
	return []map[string]interface{}{example}
}