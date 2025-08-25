package schema

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
)

// Generator handles schema generation from Go types
type Generator struct {
	definitions map[string]interface{}
	processed   map[reflect.Type]bool
}

// NewGenerator creates a new schema generator
func NewGenerator() *Generator {
	return &Generator{
		definitions: make(map[string]interface{}),
		processed:   make(map[reflect.Type]bool),
	}
}

// GenerateRecipeSchema generates a complete recipe schema
func (g *Generator) GenerateRecipeSchema() (map[string]interface{}, error) {
	// Generate the main recipe schema
	recipe := Recipe{}
	recipeSchema := g.generateFromType(reflect.TypeOf(recipe), recipe)
	
	// Build the complete schema document
	schema := map[string]interface{}{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         "https://vibethis.com/schemas/recipe.schema.json",
		"title":       "Recipe Schema",
		"type":        "object",
		"description": "Schema for Vibethis recipe definitions",
	}
	
	// Merge recipe properties
	if props, ok := recipeSchema["properties"].(map[string]interface{}); ok {
		schema["properties"] = props
	}
	if req, ok := recipeSchema["required"].([]string); ok {
		schema["required"] = req
	}
	
	// Add oneOf constraint for node type
	schema["oneOf"] = []map[string]interface{}{
		{"required": []string{"op"}},
		{"required": []string{"sequence"}},
		{"required": []string{"parallel"}},
		{"required": []string{"states"}},
	}
	
	// Generate all definitions
	g.generateAllDefinitions()
	schema["definitions"] = g.definitions
	
	return schema, nil
}

// GenerateActivitySchema generates schema for a specific activity
func (g *Generator) GenerateActivitySchema(activityType string) (map[string]interface{}, error) {
	var operation SchemaType
	
	switch activityType {
	case "sleep":
		operation = &SleepOperation{}
	case "command_execution":
		operation = &CommandExecutionOperation{}
	case "llm_inference":
		operation = &LLMInferenceOperation{}
	case "git_shallow_clone":
		operation = &GitShallowCloneOperation{}
	case "recipe":
		operation = &RecipeOperation{}
	case "input":
		operation = &InputOperation{}
	default:
		return nil, fmt.Errorf("unknown activity type: %s", activityType)
	}
	
	schema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"title":       "Activity Schema: " + activityType,
		"type":        "object",
		"description": fmt.Sprintf("Schema for %s activity", activityType),
	}
	
	props := make(map[string]interface{})
	
	// Add operation schema with const discrimination
	opSchema := g.generateOperationSchema(operation)
	props["operation"] = opSchema
	
	// Add input schema
	if inputs := operation.SchemaInputs(); inputs != nil {
		inputSchema := g.generateFromType(reflect.TypeOf(inputs), inputs)
		props["inputs"] = inputSchema
	}
	
	// Add output schema
	if outputs := operation.SchemaOutputs(); outputs != nil {
		outputSchema := g.generateFromType(reflect.TypeOf(outputs), outputs)
		props["outputs"] = outputSchema
	}
	
	schema["properties"] = props
	
	return schema, nil
}

// generateFromType generates schema from a reflect.Type
func (g *Generator) generateFromType(t reflect.Type, v interface{}) map[string]interface{} {
	// Handle special interfaces
	if provider, ok := v.(SchemaProvider); ok {
		return provider.ProvideSchema()
	}
	
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		if reflect.ValueOf(v).IsNil() {
			v = reflect.New(t).Elem().Interface()
		} else {
			v = reflect.ValueOf(v).Elem().Interface()
		}
	}
	
	schema := make(map[string]interface{})
	
	switch t.Kind() {
	case reflect.Struct:
		schema = g.generateStructSchema(t, v)
	case reflect.String:
		schema["type"] = "string"
		g.addStringValidation(schema, t, v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		schema["type"] = "integer"
		g.addNumberValidation(schema, t)
	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"
		g.addNumberValidation(schema, t)
	case reflect.Bool:
		schema["type"] = "boolean"
	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		if t.Elem().Kind() != reflect.Uint8 { // Not a byte array
			elemSchema := g.generateFromType(t.Elem(), reflect.Zero(t.Elem()).Interface())
			schema["items"] = elemSchema
		}
	case reflect.Map:
		schema["type"] = "object"
		if t.Key().Kind() == reflect.String {
			valueSchema := g.generateFromType(t.Elem(), reflect.Zero(t.Elem()).Interface())
			schema["additionalProperties"] = valueSchema
		}
	case reflect.Interface:
		// Interface types are usually "any"
		schema["type"] = "object"
		schema["additionalProperties"] = true
	}
	
	// Apply schema enhancer if available
	if enhancer, ok := v.(SchemaEnhancer); ok {
		enhancer.EnhanceSchema(schema)
	}
	
	return schema
}

// generateStructSchema generates schema for a struct type
func (g *Generator) generateStructSchema(t reflect.Type, v interface{}) map[string]interface{} {
	schema := map[string]interface{}{
		"type": "object",
	}
	
	properties := make(map[string]interface{})
	required := []string{}
	hasOneOf := false
	oneOfFields := []map[string]interface{}{}
	
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		
		// Handle embedded fields
		if field.Anonymous {
			if field.Tag.Get("json") == ",inline" {
				// Inline the embedded struct's fields
				embeddedSchema := g.generateFromType(field.Type, reflect.Zero(field.Type).Interface())
				if embProps, ok := embeddedSchema["properties"].(map[string]interface{}); ok {
					for k, v := range embProps {
						properties[k] = v
					}
				}
				if embReq, ok := embeddedSchema["required"].([]string); ok {
					required = append(required, embReq...)
				}
			}
			continue
		}
		
		// Parse json tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			// Check for oneOf tag
			if field.Tag.Get("oneOf") == "true" {
				hasOneOf = true
				// Add to oneOf constraint
				if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.String {
					// SharedRef case
					oneOfFields = append(oneOfFields, map[string]interface{}{
						"required": []string{getJSONFieldName(field)},
					})
				} else {
					// Other node types
					oneOfFields = append(oneOfFields, map[string]interface{}{
						"required": []string{getNodeTypeField(field.Name)},
					})
				}
			}
			// Check for op tag (discriminated union)
			if opTag := field.Tag.Get("op"); opTag != "" {
				// This is an operation type in a discriminated union
				continue // Will be handled by NodeOperation
			}
			continue
		}
		
		fieldName := getJSONFieldName(field)
		if fieldName == "" {
			continue
		}
		
		// Get field value for schema generation
		fieldValue := reflect.Zero(field.Type).Interface()
		if reflect.ValueOf(v).Kind() == reflect.Struct && reflect.ValueOf(v).NumField() > i {
			fieldValue = reflect.ValueOf(v).Field(i).Interface()
		}
		
		// Generate field schema
		fieldSchema := g.generateFromType(field.Type, fieldValue)
		
		// Add field metadata from tags
		g.addFieldMetadata(fieldSchema, field)
		
		// Check if required
		if field.Tag.Get("required") == "true" {
			required = append(required, fieldName)
		}
		
		properties[fieldName] = fieldSchema
	}
	
	// Handle WorkflowNode special case
	if t.Name() == "WorkflowNode" {
		// Add the node type discriminators
		properties["op"] = map[string]interface{}{
			"type":        "string",
			"description": "Operation type",
		}
		properties["sequence"] = map[string]interface{}{
			"$ref": "#/definitions/SequenceValue",
		}
		properties["parallel"] = map[string]interface{}{
			"$ref": "#/definitions/ParallelValue",
		}
		properties["states"] = map[string]interface{}{
			"$ref": "#/definitions/StateMap",
		}
	}
	
	// Handle NodeOperation special case
	if t.Name() == "NodeOperation" {
		// Build oneOf array for all operation types
		schema = g.generateNodeOperationSchema()
		return schema
	}
	
	if len(properties) > 0 {
		schema["properties"] = properties
	}
	
	if len(required) > 0 {
		schema["required"] = required
	}
	
	// Add additionalProperties: false by default for strict validation
	if _, hasAdditional := schema["additionalProperties"]; !hasAdditional {
		schema["additionalProperties"] = false
	}
	
	// Add oneOf constraints if needed
	if hasOneOf && len(oneOfFields) > 0 {
		schema["oneOf"] = oneOfFields
	}
	
	return schema
}

// generateNodeOperationSchema generates the discriminated union schema for operations
func (g *Generator) generateNodeOperationSchema() map[string]interface{} {
	operations := []struct {
		name string
		op   SchemaType
	}{
		{"SleepOperation", &SleepOperation{}},
		{"CommandExecutionOperation", &CommandExecutionOperation{}},
		{"LLMInferenceOperation", &LLMInferenceOperation{}},
		{"GitShallowCloneOperation", &GitShallowCloneOperation{}},
		{"RecipeOperation", &RecipeOperation{}},
		{"InputOperation", &InputOperation{}},
	}
	
	oneOf := []map[string]interface{}{}
	
	for _, op := range operations {
		// Generate and add operation schema to definitions automatically
		opSchema := g.generateOperationSchema(op.op)
		g.definitions[op.name] = opSchema
		
		// Add to oneOf with reference to definitions
		oneOf = append(oneOf, map[string]interface{}{
			"$ref": fmt.Sprintf("#/definitions/%s", op.name),
		})
	}
	
	return map[string]interface{}{
		"oneOf": oneOf,
	}
}

// generateOperationSchema generates schema for a specific operation
func (g *Generator) generateOperationSchema(op SchemaType) map[string]interface{} {
	schema := map[string]interface{}{
		"type":     "object",
		"required": []string{"op"},
		"additionalProperties": false,
	}
	
	properties := map[string]interface{}{
		"op": map[string]interface{}{
			"const": op.SchemaDiscriminator(),
		},
		"id": map[string]interface{}{
			"type":        "string",
			"description": "Unique identifier for the node",
		},
		"desc": map[string]interface{}{
			"type":        "string",
			"description": "Human-readable description",
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
	}
	
	// Add inputs
	if inputs := op.SchemaInputs(); inputs != nil {
		inputSchema := g.generateFromType(reflect.TypeOf(inputs), inputs)
		// Merge input properties into a single inputs object
		properties["inputs"] = map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true, // Allow templating
			"properties":           inputSchema["properties"],
			"required":             inputSchema["required"],
		}
	}
	
	// Add outputs
	if outputs := op.SchemaOutputs(); outputs != nil {
		outputSchema := g.generateFromType(reflect.TypeOf(outputs), outputs)
		properties["outputs"] = outputSchema
	}
	
	schema["properties"] = properties
	
	return schema
}

// generateAllDefinitions generates all type definitions
func (g *Generator) generateAllDefinitions() {
	// Core types
	g.definitions["RetryPolicy"] = g.generateFromType(reflect.TypeOf(types.RetryPolicy{}), RetryPolicy{})
	g.definitions["InputDef"] = g.generateFromType(reflect.TypeOf(InputDef{}), InputDef{})
	g.definitions["Transition"] = g.generateFromType(reflect.TypeOf(types.Transition{}), Transition{})

	// Node types
	g.definitions["Node"] = g.generateFromType(reflect.TypeOf(WorkflowNode{}), WorkflowNode{})
	g.definitions["SequenceNode"] = g.generateFromType(reflect.TypeOf(SequenceNode{}), SequenceNode{})
	g.definitions["ParallelNode"] = g.generateFromType(reflect.TypeOf(ParallelNode{}), ParallelNode{})
	g.definitions["StateNode"] = g.generateFromType(reflect.TypeOf(StateNode{}), StateNode{})
	g.definitions["State"] = g.generateFromType(reflect.TypeOf(State{}), State{})
	
	// Trigger automatic generation of operation types by processing NodeOperation
	// This will automatically add all operation schemas to definitions
	nodeOp := NodeOperation{}
	g.generateFromType(reflect.TypeOf(nodeOp), nodeOp)
	
	// Value types for arrays
	g.definitions["SequenceValue"] = map[string]interface{}{
		"type":        "array",
		"description": "Sequential execution of nodes",
		"items":       map[string]interface{}{"$ref": "#/definitions/Node"},
		"minItems":    1,
	}
	
	g.definitions["ParallelValue"] = map[string]interface{}{
		"type":        "array",
		"description": "Parallel execution of nodes",
		"items":       map[string]interface{}{"$ref": "#/definitions/Node"},
		"minItems":    1,
	}
	
	g.definitions["StateMap"] = map[string]interface{}{
		"type":     "object",
		"required": []string{"initial"},
		"additionalProperties": map[string]interface{}{"$ref": "#/definitions/State"},
		"properties": map[string]interface{}{
			"initial": map[string]interface{}{
				"type":        "string",
				"description": "Initial state to start execution",
			},
		},
	}
	
	// Shared reference node
	g.definitions["SharedRefNode"] = map[string]interface{}{
		"type":     "object",
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
	
	// Form field types
	g.definitions["FormField"] = g.generateFromType(reflect.TypeOf(FormField{}), FormField{})
	g.definitions["FieldOption"] = g.generateFromType(reflect.TypeOf(FieldOption{}), FieldOption{})
	g.definitions["ScaleConfig"] = g.generateFromType(reflect.TypeOf(ScaleConfig{}), ScaleConfig{})
	g.definitions["FieldValidation"] = g.generateFromType(reflect.TypeOf(FieldValidation{}), FieldValidation{})
}

// addFieldMetadata adds metadata from struct tags to the schema
func (g *Generator) addFieldMetadata(schema map[string]interface{}, field reflect.StructField) {
	// Description
	if desc := field.Tag.Get("description"); desc != "" {
		schema["description"] = desc
	}
	
	// Default value
	if def := field.Tag.Get("default"); def != "" {
		schema["default"] = parseValue(def, field.Type)
	}
	
	// Pattern (for strings)
	if pattern := field.Tag.Get("pattern"); pattern != "" {
		schema["pattern"] = pattern
	}
	
	// Format
	if format := field.Tag.Get("format"); format != "" {
		schema["format"] = format
	}
	
	// Enum values
	if enum := field.Tag.Get("enum"); enum != "" {
		schema["enum"] = strings.Split(enum, ",")
	}
	
	// Min/Max for numbers
	if min := field.Tag.Get("min"); min != "" {
		if field.Type.Kind() == reflect.String {
			schema["minLength"] = parseIntValue(min)
		} else {
			schema["minimum"] = parseNumericValue(min, field.Type)
		}
	}
	
	if max := field.Tag.Get("max"); max != "" {
		if field.Type.Kind() == reflect.String {
			schema["maxLength"] = parseIntValue(max)
		} else {
			schema["maximum"] = parseNumericValue(max, field.Type)
		}
	}
	
	// MinItems for arrays
	if minItems := field.Tag.Get("minItems"); minItems != "" {
		schema["minItems"] = parseIntValue(minItems)
	}
}

// addStringValidation adds validation for string types
func (g *Generator) addStringValidation(schema map[string]interface{}, t reflect.Type, v interface{}) {
	// Check for Duration type
	if t.Name() == "Duration" {
		if d, ok := v.(Duration); ok {
			schema["pattern"] = d.SchemaPattern()
			schema["description"] = d.SchemaDescription()
		}
	}
}

// addNumberValidation adds validation for number types
func (g *Generator) addNumberValidation(schema map[string]interface{}, t reflect.Type) {
	// Could add type-specific validation here
}

// Helper functions

func getJSONFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	
	parts := strings.Split(tag, ",")
	name := parts[0]
	
	// Handle omitempty and other options
	if name == "" && len(parts) > 1 {
		for _, part := range parts[1:] {
			if part != "omitempty" && part != "inline" {
				return field.Name // Use field name if no explicit json name
			}
		}
	}
	
	if name == "" {
		return field.Name
	}
	
	return name
}

func getNodeTypeField(fieldName string) string {
	switch fieldName {
	case "Operation":
		return "op"
	case "Sequence":
		return "sequence"
	case "Parallel":
		return "parallel"
	case "States":
		return "states"
	case "SharedRef":
		return "shared"
	default:
		return strings.ToLower(fieldName)
	}
}

func parseValue(s string, t reflect.Type) interface{} {
	switch t.Kind() {
	case reflect.Bool:
		return s == "true"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return parseIntValue(s)
	case reflect.Float32, reflect.Float64:
		return parseFloatValue(s)
	default:
		return s
	}
}

func parseIntValue(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func parseFloatValue(s string) float64 {
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

func parseNumericValue(s string, t reflect.Type) interface{} {
	if t.Kind() >= reflect.Int && t.Kind() <= reflect.Int64 {
		return parseIntValue(s)
	}
	return parseFloatValue(s)
}