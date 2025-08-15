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
	// If filtering for a specific activity, return just that activity schema
	if filterActivity != "" {
		return sm.buildActivitySchema(registry, filterActivity, includeVersion, includeExamples)
	}
	
	// Build the complete recipe schema
	schema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"title":       "Recipe Schema",
		"type":        "object",
		"description": "Schema for Vibethis recipe definitions",
	}
	
	// Add version if requested
	if includeVersion {
		schema["version"] = "1.0.0"
	}
	
	// Build recipe properties
	properties := map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Name of the recipe",
		},
		"description": map[string]interface{}{
			"type":        "string",
			"description": "Human-readable description of what the recipe does",
		},
		"version": map[string]interface{}{
			"type":        "string",
			"description": "Version of the recipe",
			"default":     "1.0",
		},
		"shared": map[string]interface{}{
			"type":                 "object",
			"description":          "Shared node definitions that can be referenced",
			"additionalProperties": map[string]interface{}{"$ref": "#/definitions/Node"},
		},
		"input_schema": map[string]interface{}{
			"type":                 "object",
			"description":          "Schema definitions for recipe inputs",
			"additionalProperties": map[string]interface{}{"$ref": "#/definitions/InputDef"},
		},
		
		// Root node properties (exactly one of these)
		"op":       map[string]interface{}{"$ref": "#/definitions/OpValue"},
		"sequence": map[string]interface{}{"$ref": "#/definitions/SequenceValue"},
		"parallel": map[string]interface{}{"$ref": "#/definitions/ParallelValue"},
		"states":   map[string]interface{}{"$ref": "#/definitions/StateMap"},
		
		// Common node properties at root level
		"id": map[string]interface{}{
			"type":        "string",
			"description": "Unique identifier for the root node",
		},
		"desc": map[string]interface{}{
			"type":        "string",
			"description": "Human-readable description of the root node",
		},
		"inputs": map[string]interface{}{
			"type":                 "object",
			"description":          "Input values or templates for the root node",
			"additionalProperties": true,
		},
		"outputs": map[string]interface{}{
			"type":                 "object",
			"description":          "Output mappings for the root node",
			"additionalProperties": true,
		},
		"timeout": map[string]interface{}{
			"type":        "string",
			"description": "Timeout duration (e.g., '30s', '5m')",
		},
		"retry": map[string]interface{}{
			"$ref": "#/definitions/RetryPolicy",
		},
	}
	
	schema["properties"] = properties
	schema["required"] = []string{"name", "version"}
	
	// Add oneOf constraint for root node type
	schema["oneOf"] = []map[string]interface{}{
		{"required": []string{"op"}},
		{"required": []string{"sequence"}},
		{"required": []string{"parallel"}},
		{"required": []string{"states"}},
	}
	
	// Build definitions
	definitions := sm.buildDefinitions(registry)
	schema["definitions"] = definitions
	
	// Add examples if requested
	if includeExamples {
		schema["examples"] = sm.generateRecipeExamples()
	}
	
	return schema, nil
}

// buildActivitySchema builds schema for a specific activity
func (sm *SchemaManager) buildActivitySchema(registry *worker.ActivityRegistry, activityType string, includeVersion bool, includeExamples bool) (map[string]interface{}, error) {
	registration, exists := registry.Get(activityType)
	if !exists {
		return nil, nil
	}
	
	activitySchema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"title":       "Activity Schema: " + activityType,
		"type":        "object",
		"description": registration.Metadata.Description,
	}
	
	if includeVersion {
		activitySchema["version"] = "1.0.0"
	}
	
	// Build properties
	props := make(map[string]interface{})
	
	// Add config schema if present
	if registration.ConfigSchema != nil {
		props["config"] = convertSchemaWithDetails(registration.ConfigSchema)
	}
	
	// Add input schema if present  
	if registration.InputSchema != nil {
		props["inputs"] = convertSchemaWithDetails(registration.InputSchema)
	}
	
	// Add output schema if present
	if registration.OutputSchema != nil {
		props["outputs"] = convertSchemaWithDetails(registration.OutputSchema)
	}
	
	activitySchema["properties"] = props
	
	// Add examples if requested
	if includeExamples {
		activitySchema["examples"] = sm.generateExamples(activityType, registration)
	}
	
	return activitySchema, nil
}

// buildDefinitions builds all the type definitions for the schema
func (sm *SchemaManager) buildDefinitions(registry *worker.ActivityRegistry) map[string]interface{} {
	definitions := make(map[string]interface{})
	
	// InputDef schema
	definitions["InputDef"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"type": map[string]interface{}{
				"type":        "string",
				"description": "Type of the input (string, number, boolean, etc.)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Description of the input",
			},
			"required": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether the input is required",
			},
			"default": map[string]interface{}{
				"description": "Default value if not provided",
			},
		},
	}
	
	// Node schema
	definitions["Node"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Unique identifier for the node",
			},
			"desc": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable description",
			},
			"op":       map[string]interface{}{"$ref": "#/definitions/OpValue"},
			"sequence": map[string]interface{}{"$ref": "#/definitions/SequenceValue"},
			"parallel": map[string]interface{}{"$ref": "#/definitions/ParallelValue"},
			"states":   map[string]interface{}{"$ref": "#/definitions/StateMap"},
			"shared": map[string]interface{}{
				"type":        "string",
				"description": "Reference to a shared node definition",
			},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"description":          "Input values or templates",
				"additionalProperties": true,
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"description":          "Output mappings (composite nodes only)",
				"additionalProperties": true,
			},
			"timeout": map[string]interface{}{
				"type":        "string",
				"description": "Timeout duration",
			},
			"retry": map[string]interface{}{
				"$ref": "#/definitions/RetryPolicy",
			},
			"when": map[string]interface{}{
				"type":        "string",
				"description": "CEL expression for conditional execution",
			},
		},
		"oneOf": []map[string]interface{}{
			{"required": []string{"op"}},
			{"required": []string{"sequence"}},
			{"required": []string{"parallel"}},
			{"required": []string{"states"}},
			{"required": []string{"shared"}},
		},
	}
	
	// OpValue - operation types
	opValuesMap := make(map[string]bool)
	for activityType := range registry.GetAll() {
		opValuesMap[activityType] = true
	}
	// Add built-in operations
	opValuesMap["recipe"] = true
	opValuesMap["sleep"] = true
	
	opValues := []string{}
	for op := range opValuesMap {
		opValues = append(opValues, op)
	}
	
	definitions["OpValue"] = map[string]interface{}{
		"type":        "string",
		"description": "Operation type",
		"enum":        opValues,
	}
	
	// SequenceValue
	definitions["SequenceValue"] = map[string]interface{}{
		"type":        "array",
		"description": "Sequential execution of nodes",
		"items":       map[string]interface{}{"$ref": "#/definitions/Node"},
		"minItems":    1,
	}
	
	// ParallelValue
	definitions["ParallelValue"] = map[string]interface{}{
		"type":        "array",
		"description": "Parallel execution of nodes",
		"items":       map[string]interface{}{"$ref": "#/definitions/Node"},
		"minItems":    1,
	}
	
	// StateMap
	definitions["StateMap"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"initial": map[string]interface{}{
				"type":        "string",
				"description": "Initial state to start execution",
			},
		},
		"required":             []string{"initial"},
		"additionalProperties": map[string]interface{}{"$ref": "#/definitions/State"},
	}
	
	// State
	definitions["State"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"op":       map[string]interface{}{"$ref": "#/definitions/OpValue"},
			"sequence": map[string]interface{}{"$ref": "#/definitions/SequenceValue"},
			"parallel": map[string]interface{}{"$ref": "#/definitions/ParallelValue"},
			"states":   map[string]interface{}{"$ref": "#/definitions/StateMap"},
			"shared": map[string]interface{}{
				"type":        "string",
				"description": "Reference to a shared node definition",
			},
			"desc": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable description",
			},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"description":          "Input values or templates",
				"additionalProperties": true,
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"description":          "Output mappings",
				"additionalProperties": true,
			},
			"timeout": map[string]interface{}{
				"type":        "string",
				"description": "Timeout duration",
			},
			"retry": map[string]interface{}{
				"$ref": "#/definitions/RetryPolicy",
			},
			"transitions": map[string]interface{}{
				"type":        "array",
				"description": "State transitions",
				"items":       map[string]interface{}{"$ref": "#/definitions/Transition"},
			},
			"error": map[string]interface{}{
				"type":        "string",
				"description": "Error message for terminal error states",
			},
		},
		"oneOf": []map[string]interface{}{
			{"required": []string{"op"}},
			{"required": []string{"sequence"}},
			{"required": []string{"parallel"}},
			{"required": []string{"states"}},
			{"required": []string{"shared"}},
		},
	}
	
	// Transition
	definitions["Transition"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Target state name",
			},
			"when": map[string]interface{}{
				"type":        "string",
				"description": "CEL expression condition for transition",
			},
		},
		"required": []string{"to"},
	}
	
	// RetryPolicy
	definitions["RetryPolicy"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"max_attempts": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of retry attempts",
				"minimum":     1,
			},
			"initial_interval": map[string]interface{}{
				"type":        "string",
				"description": "Initial retry interval (e.g., '1s')",
			},
			"backoff_coefficient": map[string]interface{}{
				"type":        "number",
				"description": "Exponential backoff coefficient",
				"minimum":     1.0,
			},
			"max_interval": map[string]interface{}{
				"type":        "string",
				"description": "Maximum retry interval",
			},
		},
		"required": []string{"max_attempts", "initial_interval"},
	}
	
	// Add activity schemas
	activities := make(map[string]interface{})
	for activityType, registration := range registry.GetAll() {
		activitySchema := make(map[string]interface{})
		activitySchema["type"] = "object"
		activitySchema["description"] = registration.Metadata.Description
		
		// Build properties for the activity
		props := make(map[string]interface{})
		
		// Add config schema if present
		if registration.ConfigSchema != nil {
			props["config"] = convertSchemaWithDetails(registration.ConfigSchema)
		}
		
		// Add input schema if present  
		if registration.InputSchema != nil {
			props["inputs"] = convertSchemaWithDetails(registration.InputSchema)
		}
		
		// Add output schema if present
		if registration.OutputSchema != nil {
			props["outputs"] = convertSchemaWithDetails(registration.OutputSchema)
		}
		
		activitySchema["properties"] = props
		activities[activityType] = activitySchema
	}
	
	definitions["Activities"] = activities
	
	return definitions
}

// generateRecipeExamples generates example recipes
func (sm *SchemaManager) generateRecipeExamples() []map[string]interface{} {
	examples := []map[string]interface{}{
		// Simple operation example
		{
			"name":        "simple-command",
			"version":     "1.0",
			"description": "Execute a simple command",
			"op":          "command_execution",
			"inputs": map[string]interface{}{
				"run": "echo 'Hello World'",
			},
		},
		// Sequence example
		{
			"name":        "sequential-workflow",
			"version":     "1.0",
			"description": "Execute commands in sequence",
			"sequence": []map[string]interface{}{
				{
					"id": "step1",
					"op": "command_execution",
					"inputs": map[string]interface{}{
						"run": "echo 'Step 1'",
					},
				},
				{
					"id": "step2",
					"op": "command_execution",
					"inputs": map[string]interface{}{
						"run": "echo 'Step 2'",
					},
				},
			},
		},
		// Parallel example
		{
			"name":        "parallel-workflow",
			"version":     "1.0",
			"description": "Execute commands in parallel",
			"parallel": []map[string]interface{}{
				{
					"id": "task1",
					"op": "command_execution",
					"inputs": map[string]interface{}{
						"run": "echo 'Task 1'",
					},
				},
				{
					"id": "task2",
					"op": "command_execution",
					"inputs": map[string]interface{}{
						"run": "echo 'Task 2'",
					},
				},
			},
		},
	}
	
	return examples
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