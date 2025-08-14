package worker

import (
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
	"github.com/divisive-ai/vibethis/server/ops/pkg/types"
)

// ActivityRegistration holds the activity and its generated schemas
type ActivityRegistration struct {
	Activity     interface{}        // The generic activity interface
	ConfigSchema *jsonschema.Schema
	InputSchema  *jsonschema.Schema
	OutputSchema *jsonschema.Schema
	Metadata     types.ActivityMetadata
}

// ActivityRegistry manages all registered activities
type ActivityRegistry struct {
	activities map[string]ActivityRegistration
	generator  SchemaGenerator
}

// SchemaGenerator validates struct tags and generates JSON schemas
// This is internal to recipe-worker
type SchemaGenerator interface {
	GenerateSchema(typ reflect.Type) (*jsonschema.Schema, error)
	ValidateStructTags(typ reflect.Type) error
}

// NewActivityRegistry creates a new activity registry
func NewActivityRegistry() *ActivityRegistry {
	return &ActivityRegistry{
		activities: make(map[string]ActivityRegistration),
		generator:  NewDefaultSchemaGenerator(),
	}
}

// RegisterGeneric registers any activity without knowing its specific generic types
// This allows dynamic registration of activities from external packages
func (r *ActivityRegistry) RegisterGeneric(activity interface{}) error {
	// Use reflection to get metadata
	activityValue := reflect.ValueOf(activity)
	metadataMethod := activityValue.MethodByName("GetMetadata")
	if !metadataMethod.IsValid() {
		return fmt.Errorf("activity does not have GetMetadata method")
	}
	
	// Call GetMetadata
	results := metadataMethod.Call(nil)
	if len(results) == 0 {
		return fmt.Errorf("GetMetadata returned no results")
	}
	
	metadata, ok := results[0].Interface().(types.ActivityMetadata)
	if !ok {
		return fmt.Errorf("GetMetadata did not return ActivityMetadata")
	}
	
	// Check if already registered
	if _, exists := r.activities[metadata.Type]; exists {
		return fmt.Errorf("activity type %s already registered", metadata.Type)
	}
	
	registration := ActivityRegistration{
		Activity: activity,
		Metadata: metadata,
	}
	
	// Generate schemas immediately using reflection
	r.generateSchemasForRegistration(&registration)
	
	r.activities[metadata.Type] = registration
	
	return nil
}

// Register accepts any generic RegisterableActivity from the activity module
func Register[TConfig any, TInput any, TOutput any](
	r *ActivityRegistry,
	activity types.RegisterableActivity[TConfig, TInput, TOutput],
) error {
	metadata := activity.GetMetadata()

	if _, exists := r.activities[metadata.Type]; exists {
		return fmt.Errorf("activity type %s already registered", metadata.Type)
	}

	// Generate schemas from types using reflection on the generic type parameters
	var config TConfig
	var input TInput
	var output TOutput

	// Validate all struct fields have json tags before generating schemas
	if err := r.generator.ValidateStructTags(reflect.TypeOf(config)); err != nil {
		return fmt.Errorf("config type validation failed: %w", err)
	}
	if err := r.generator.ValidateStructTags(reflect.TypeOf(input)); err != nil {
		return fmt.Errorf("input type validation failed: %w", err)
	}
	if err := r.generator.ValidateStructTags(reflect.TypeOf(output)); err != nil {
		return fmt.Errorf("output type validation failed: %w", err)
	}

	configSchema, err := r.generator.GenerateSchema(reflect.TypeOf(config))
	if err != nil {
		return fmt.Errorf("config schema generation failed: %w", err)
	}

	inputSchema, err := r.generator.GenerateSchema(reflect.TypeOf(input))
	if err != nil {
		return fmt.Errorf("input schema generation failed: %w", err)
	}

	outputSchema, err := r.generator.GenerateSchema(reflect.TypeOf(output))
	if err != nil {
		return fmt.Errorf("output schema generation failed: %w", err)
	}

	r.activities[metadata.Type] = ActivityRegistration{
		Activity:     activity,
		ConfigSchema: configSchema,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Metadata:     metadata,
	}

	return nil
}

// Get retrieves an activity registration by type
func (r *ActivityRegistry) Get(activityType string) (ActivityRegistration, bool) {
	registration, exists := r.activities[activityType]
	return registration, exists
}

// List returns all registered activity types
func (r *ActivityRegistry) List() []string {
	types := make([]string, 0, len(r.activities))
	for activityType := range r.activities {
		types = append(types, activityType)
	}
	return types
}

// GetAll returns all activity registrations
func (r *ActivityRegistry) GetAll() map[string]ActivityRegistration {
	// Return a copy to prevent external modification
	result := make(map[string]ActivityRegistration)
	for k, v := range r.activities {
		result[k] = v
	}
	return result
}

// UpdateRegistration updates an existing activity registration
// This is used by the schema manager to update schemas after generation
func (r *ActivityRegistry) UpdateRegistration(activityType string, registration ActivityRegistration) {
	r.activities[activityType] = registration
}

// generateSchemasForRegistration generates schemas for an activity using reflection
func (r *ActivityRegistry) generateSchemasForRegistration(registration *ActivityRegistration) {
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
	
	// Config is the second parameter (after context)
	configType := methodType.In(1)
	if configType.Kind() != reflect.Interface {
		registration.ConfigSchema, _ = r.generator.GenerateSchema(configType)
	}
	
	// Input is the third parameter
	inputType := methodType.In(2)
	if inputType.Kind() != reflect.Interface {
		registration.InputSchema, _ = r.generator.GenerateSchema(inputType)
	}
	
	// Output is the first return value
	outputType := methodType.Out(0)
	if outputType.Kind() != reflect.Interface {
		registration.OutputSchema, _ = r.generator.GenerateSchema(outputType)
	}
}