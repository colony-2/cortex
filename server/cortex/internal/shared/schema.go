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
	
	// Build the complete recipe schema - OpenAPI 3.1 / JSON Schema 2020-12 compatible
	schema := map[string]interface{}{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         "https://vibethis.com/schemas/recipe.schema.json",
		"title":       "Recipe Schema",
		"type":        "object",
		"description": "Schema for Vibethis recipe definitions",
	}
	
	// Add version if requested
	if includeVersion {
		schema["version"] = "1.0.0"
	}
	
	// Build the Recipe schema - strict typing with no unknown fields
	recipeProperties := map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Name of the recipe",
		},
		"version": map[string]interface{}{
			"type":        "string",
			"description": "Version of the recipe",
			"default":     "1.0",
		},
		"description": map[string]interface{}{
			"type":        "string",
			"description": "Human-readable description of what the recipe does",
		},
		// inputs and outputs are maps for node input values
		"inputs": map[string]interface{}{
			"type":                 "object",
			"description":          "Node input values",
			"additionalProperties": true,
		},
		"outputs": map[string]interface{}{
			"type":                 "object",
			"description":          "Node output mappings",
			"additionalProperties": true,
		},
		"input_schema": map[string]interface{}{
			"type":                 "object",
			"description":          "Schema definitions for recipe inputs",
			"additionalProperties": map[string]interface{}{"$ref": "#/definitions/InputDef"},
		},
		"shared": map[string]interface{}{
			"type":                 "object",
			"description":          "Shared node definitions that can be referenced",
			"additionalProperties": map[string]interface{}{"$ref": "#/definitions/Node"},
		},
		// Node properties - exactly one of these
		"op":       map[string]interface{}{"type": "string", "description": "Operation type"},
		"sequence": map[string]interface{}{"$ref": "#/definitions/SequenceValue"},
		"parallel": map[string]interface{}{"$ref": "#/definitions/ParallelValue"},
		"states":   map[string]interface{}{"$ref": "#/definitions/StateMap"},
		// Common node properties
		"id": map[string]interface{}{
			"type":        "string",
			"description": "Unique identifier for the root node",
		},
		"desc": map[string]interface{}{
			"type":        "string",
			"description": "Human-readable description of the root node",
		},
		"timeout": map[string]interface{}{
			"type":        "string",
			"description": "Timeout duration (e.g., '30s', '5m')",
		},
		"retry": map[string]interface{}{
			"$ref": "#/definitions/RetryPolicy",
		},
		"when": map[string]interface{}{
			"type":        "string",
			"description": "CEL expression for conditional execution",
		},
	}
	
	schema["type"] = "object"
	schema["properties"] = recipeProperties
	schema["required"] = []string{"name", "version"}
	schema["additionalProperties"] = false  // Disallow unknown fields
	
	// Add oneOf constraint for node type (op, sequence, parallel, or states)
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
	
	// Build operation schemas with const discrimination
	operationRefs := []map[string]interface{}{}
	
	// Sleep Operation
	definitions["SleepOperation"] = sm.buildSleepOperation()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/SleepOperation"})
	
	// Command Execution Operation
	definitions["CommandExecutionOperation"] = sm.buildCommandExecutionOperation()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/CommandExecutionOperation"})
	
	// LLM Inference Operation
	definitions["LLMInferenceOperation"] = sm.buildLLMInferenceOperation()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/LLMInferenceOperation"})
	
	// Git Shallow Clone Operation
	definitions["GitShallowCloneOperation"] = sm.buildGitShallowCloneOperation()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/GitShallowCloneOperation"})
	
	// Recipe Operation
	definitions["RecipeOperation"] = sm.buildRecipeOperation()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/RecipeOperation"})
	
	// Input Operation
	definitions["InputOperation"] = sm.buildInputOperation()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/InputOperation"})
	
	// Composite node types
	definitions["SequenceNode"] = sm.buildSequenceNode()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/SequenceNode"})
	
	definitions["ParallelNode"] = sm.buildParallelNode()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/ParallelNode"})
	
	definitions["StateNode"] = sm.buildStateNode()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/StateNode"})
	
	definitions["SharedRefNode"] = sm.buildSharedRefNode()
	operationRefs = append(operationRefs, map[string]interface{}{"$ref": "#/definitions/SharedRefNode"})
	
	// Node schema - uses oneOf with all operation and composite types
	definitions["Node"] = map[string]interface{}{
		"oneOf": operationRefs,
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
		"required": []string{"initial"},
		"additionalProperties": map[string]interface{}{"$ref": "#/definitions/State"},
		"properties": map[string]interface{}{
			"initial": map[string]interface{}{
				"type":        "string",
				"description": "Initial state to start execution",
			},
		},
	}
	
	// State - A state can be any node type plus transitions
	// We need to define it differently because states allow additional properties (transitions, error)
	definitions["State"] = map[string]interface{}{
		"type": "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			// Node properties
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"op": map[string]interface{}{"type": "string"},
			"sequence": map[string]interface{}{"$ref": "#/definitions/SequenceValue"},
			"parallel": map[string]interface{}{"$ref": "#/definitions/ParallelValue"},
			"states": map[string]interface{}{"$ref": "#/definitions/StateMap"},
			"shared": map[string]interface{}{"type": "string"},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			"timeout": map[string]interface{}{"type": "string"},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"when": map[string]interface{}{"type": "string"},
			// State-specific properties
			"transitions": map[string]interface{}{
				"type":        "array",
				"description": "State transitions",
				"items": map[string]interface{}{
					"type": "object",
					"required": []string{"to"},
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
				},
			},
			"error": map[string]interface{}{
				"type":        "string",
				"description": "Error message for terminal error states",
			},
		},
		// States must have one of these node types
		"oneOf": []map[string]interface{}{
			{"required": []string{"op"}},
			{"required": []string{"sequence"}},
			{"required": []string{"parallel"}},
			{"required": []string{"states"}},
			{"required": []string{"shared"}},
		},
	}
	
	// RetryPolicy
	definitions["RetryPolicy"] = map[string]interface{}{
		"type": "object",
		"required": []string{"max_attempts", "initial_interval"},
		"properties": map[string]interface{}{
			"max_attempts": map[string]interface{}{
				"type":        "integer",
				"minimum":     1,
				"description": "Maximum number of retry attempts",
			},
			"initial_interval": map[string]interface{}{
				"type":        "string",
				"description": "Initial retry interval (e.g., '1s')",
			},
			"backoff_coefficient": map[string]interface{}{
				"type":        "number",
				"minimum":     1,
				"description": "Exponential backoff coefficient",
			},
			"max_interval": map[string]interface{}{
				"type":        "string",
				"description": "Maximum retry interval",
			},
		},
	}
	
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

// buildSleepOperation builds the sleep operation schema with const discrimination
func (sm *SchemaManager) buildSleepOperation() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"op"},
		"properties": map[string]interface{}{
			"op": map[string]interface{}{
				"const": "sleep",
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Unique identifier for the node",
			},
			"desc": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable description",
			},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,  // Allow additional properties for templating
				"required":             []string{"duration"},
				"properties": map[string]interface{}{
					"duration": map[string]interface{}{
						"type":        "string",
						"description": "Duration to sleep (e.g., '5s', '2m')",
					},
				},
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"start_time", "end_time", "actual_duration", "completed", "interrupted", "error_message"},
				"properties": map[string]interface{}{
					"start_time": map[string]interface{}{
						"type":   "string",
						"format": "date-time",
					},
					"end_time": map[string]interface{}{
						"type":   "string",
						"format": "date-time",
					},
					"actual_duration": map[string]interface{}{
						"type": "string",
					},
					"completed": map[string]interface{}{
						"type": "boolean",
					},
					"interrupted": map[string]interface{}{
						"type": "boolean",
					},
					"error_message": map[string]interface{}{
						"type": "string",
					},
				},
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{
				"type":        "string",
				"description": "Timeout duration",
			},
			"when": map[string]interface{}{
				"type":        "string",
				"description": "CEL expression for conditional execution",
			},
		},
	}
}

// buildCommandExecutionOperation builds the command execution operation schema
func (sm *SchemaManager) buildCommandExecutionOperation() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"op"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"op": map[string]interface{}{
				"const": "command_execution",
			},
			"id": map[string]interface{}{
				"type": "string",
			},
			"desc": map[string]interface{}{
				"type": "string",
			},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,  // Allow additional properties for templating
				"required":             []string{"run"},  // Only run is required
				"properties": map[string]interface{}{
					"run": map[string]interface{}{
						"type":        "string",
						"description": "Shell command to execute",
					},
					"working_directory": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Directory to execute the command in",
					},
					"working_dir": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Working directory configuration",
					},
					"shell": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Shell to use (e.g., bash, sh)",
					},
					"env": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": map[string]interface{}{"type": "string"},
						"description":          "Optional: Environment variables",
					},
					"continue_on_error": map[string]interface{}{
						"type":        "boolean",
						"description": "Optional: Whether to continue on error",
					},
					"timeout": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Command timeout duration",
					},
				},
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"stdout", "stderr", "exit_code", "success", "timed_out", "error_message"},
				"properties": map[string]interface{}{
					"stdout": map[string]interface{}{"type": "string"},
					"stderr": map[string]interface{}{"type": "string"},
					"exit_code": map[string]interface{}{"type": "integer"},
					"success": map[string]interface{}{"type": "boolean"},
					"timed_out": map[string]interface{}{"type": "boolean"},
					"error_message": map[string]interface{}{"type": "string"},
				},
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{
				"type": "string",
			},
			"when": map[string]interface{}{
				"type": "string",
			},
		},
	}
}

// buildLLMInferenceOperation builds the LLM inference operation schema
func (sm *SchemaManager) buildLLMInferenceOperation() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"op"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"op": map[string]interface{}{
				"const": "llm_inference",
			},
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,  // Allow additional properties for templating
				"required":             []string{},  // All fields are optional with defaults
				"properties": map[string]interface{}{
					"prompt": map[string]interface{}{"type": "string"},
					"system_prompt": map[string]interface{}{"type": "string"},
					"temperature": map[string]interface{}{"type": "number"},
					"max_tokens": map[string]interface{}{"type": "integer"},
					"top_p": map[string]interface{}{"type": "number"},
					"stop_sequences": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{"type": "string"},
					},
					"response_schema": map[string]interface{}{"type": "object"},
					"provider": map[string]interface{}{
						"type":        "string",
						"description": "LLM provider (OpenAI, Anthropic, Gemini)",
					},
					"model": map[string]interface{}{
						"type":        "string",
						"description": "Model identifier",
					},
				},
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"response", "model", "finish_reason", "usage"},
				"properties": map[string]interface{}{
					"response": map[string]interface{}{"type": "object"},
					"model": map[string]interface{}{"type": "string"},
					"finish_reason": map[string]interface{}{"type": "string"},
					"usage": map[string]interface{}{"type": "object"},
				},
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{"type": "string"},
			"when": map[string]interface{}{"type": "string"},
		},
	}
}

// buildGitShallowCloneOperation builds the git shallow clone operation schema
func (sm *SchemaManager) buildGitShallowCloneOperation() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"op"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"op": map[string]interface{}{
				"const": "git_shallow_clone",
			},
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
				"required":             []string{},  // Check actual requirements
				"properties": map[string]interface{}{
					"source_dir": map[string]interface{}{
						"type":        "string",
						"description": "Source git repository directory",
					},
					"target_dir": map[string]interface{}{
						"type":        "string",
						"description": "Target directory for the clone",
					},
					"commit_hash": map[string]interface{}{
						"type":        "string",
						"description": "Specific commit to clone",
					},
				},
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"cloned_path"},
				"properties": map[string]interface{}{
					"cloned_path": map[string]interface{}{"type": "string"},
				},
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{"type": "string"},
			"when": map[string]interface{}{"type": "string"},
		},
	}
}

// buildRecipeOperation builds the recipe operation schema
func (sm *SchemaManager) buildRecipeOperation() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"op"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"op": map[string]interface{}{
				"const": "recipe",
			},
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
				"required":             []string{"recipe"},  // Only recipe name is required
				"properties": map[string]interface{}{
					"recipe": map[string]interface{}{
						"type":        "string",
						"description": "Name of the recipe to invoke",
					},
					"version": map[string]interface{}{
						"type":        "string",
						"description": "Version of the recipe",
					},
					"timeout": map[string]interface{}{
						"type":        "string",
						"description": "Timeout for the recipe execution",
					},
					"retry_policy": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
				},
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"execution_id", "result", "status", "recipe_outputs", "execution_metadata"},
				"properties": map[string]interface{}{
					"execution_id": map[string]interface{}{"type": "string"},
					"result": map[string]interface{}{"type": "object"},
					"status": map[string]interface{}{"type": "string"},
					"recipe_outputs": map[string]interface{}{"type": "object"},
					"execution_metadata": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"start_time": map[string]interface{}{"type": "string"},
							"end_time": map[string]interface{}{"type": "string"},
							"duration_ms": map[string]interface{}{"type": "integer"},
							"attempt_count": map[string]interface{}{"type": "integer"},
						},
					},
				},
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{"type": "string"},
			"when": map[string]interface{}{"type": "string"},
		},
	}
}

// buildInputOperation builds the input operation schema
func (sm *SchemaManager) buildInputOperation() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"op"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"op": map[string]interface{}{
				"const": "input",
			},
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
				"required":             []string{},  // Check actual requirements
				"properties": map[string]interface{}{
					"box_id": map[string]interface{}{
						"type":        "string",
						"description": "Box identifier",
					},
					"activity_id": map[string]interface{}{
						"type":        "string",
						"description": "Activity identifier",
					},
					"context": map[string]interface{}{
						"type":        "object",
						"description": "Additional context data",
					},
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Form title",
					},
					"question": map[string]interface{}{
						"type":        "string",
						"description": "Question to ask the user",
					},
					"type": map[string]interface{}{
						"type": "string",
						"enum": []string{"short_answer", "paragraph_text", "multiple_choice", "checkboxes", "dropdown", "linear_scale", "date", "time"},
					},
					"fields": map[string]interface{}{
						"type":        "array",
						"description": "Form fields for multi-field forms",
						"items": map[string]interface{}{
							"type": "object",
							"required": []string{"id", "type", "question"},
							"properties": map[string]interface{}{
								"id": map[string]interface{}{"type": "string"},
								"type": map[string]interface{}{
									"type": "string",
									"enum": []string{"short_answer", "paragraph_text", "multiple_choice", "checkboxes", "dropdown", "linear_scale", "multiple_choice_grid", "checkbox_grid", "date", "time", "file_upload"},
								},
								"question": map[string]interface{}{"type": "string"},
								"placeholder": map[string]interface{}{"type": "string"},
								"required": map[string]interface{}{"type": "boolean"},
								"options": map[string]interface{}{
									"type": "array",
									"items": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"value": map[string]interface{}{"type": "string"},
											"label": map[string]interface{}{"type": "string"},
										},
									},
								},
								"scale": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"min": map[string]interface{}{"type": "integer"},
										"max": map[string]interface{}{"type": "integer"},
										"min_label": map[string]interface{}{"type": "string"},
										"max_label": map[string]interface{}{"type": "string"},
									},
								},
								"validation": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"min": map[string]interface{}{"type": "integer"},
										"max": map[string]interface{}{"type": "integer"},
										"min_length": map[string]interface{}{"type": "integer"},
										"max_length": map[string]interface{}{"type": "integer"},
										"pattern": map[string]interface{}{"type": "string"},
									},
								},
							},
						},
					},
					"timeout": map[string]interface{}{
						"type":    "integer",
						"default": 300,
						"minimum": 1,
						"maximum": 3600,
					},
					"default_on_timeout": map[string]interface{}{
						"description": "Default value if input times out",
					},
				},
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"response": map[string]interface{}{
						"description": "User response for single question",
					},
					"fields": map[string]interface{}{
						"type":        "object",
						"description": "User responses for multi-field form",
					},
					"user_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of user who responded",
					},
					"metadata": map[string]interface{}{
						"type":        "object",
						"description": "Additional metadata",
					},
				},
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{"type": "string"},
			"when": map[string]interface{}{"type": "string"},
		},
	}
}

// buildSequenceNode builds the sequence node schema
func (sm *SchemaManager) buildSequenceNode() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"sequence"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"sequence": map[string]interface{}{
				"type":     "array",
				"minItems": 1,
				"items":    map[string]interface{}{"$ref": "#/definitions/Node"},
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{"type": "string"},
			"when": map[string]interface{}{"type": "string"},
		},
	}
}

// buildParallelNode builds the parallel node schema
func (sm *SchemaManager) buildParallelNode() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"parallel"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"parallel": map[string]interface{}{
				"type":     "array",
				"minItems": 1,
				"items":    map[string]interface{}{"$ref": "#/definitions/Node"},
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{"type": "string"},
			"when": map[string]interface{}{"type": "string"},
		},
	}
}

// buildStateNode builds the state node schema
func (sm *SchemaManager) buildStateNode() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"states"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"states": map[string]interface{}{"$ref": "#/definitions/StateMap"},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			"retry": map[string]interface{}{"$ref": "#/definitions/RetryPolicy"},
			"timeout": map[string]interface{}{"type": "string"},
			"when": map[string]interface{}{"type": "string"},
		},
	}
}

// buildSharedRefNode builds the shared reference node schema
func (sm *SchemaManager) buildSharedRefNode() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"required": []string{"shared"},
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"shared": map[string]interface{}{
				"type":        "string",
				"description": "Reference to a shared node definition",
			},
			"id": map[string]interface{}{"type": "string"},
			"desc": map[string]interface{}{"type": "string"},
			"inputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			"outputs": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
		},
	}
}