package worker

import (
	"fmt"
	"log"
	"reflect"

	"github.com/vibethis/server/activity/pkg/activity"
	"github.com/vibethis/server/activity/pkg/types"
	worker "github.com/vibethis/server/recipe-worker"
)

// globalActivityRegistry is the singleton registry for all activities
var globalActivityRegistry *ActivityRegistry

// GetGlobalActivityRegistry returns the global activity registry
func GetGlobalActivityRegistry() *ActivityRegistry {
	if globalActivityRegistry == nil {
		globalActivityRegistry = NewActivityRegistry()
	}
	return globalActivityRegistry
}

// RegisterAllActivities registers all available activities from the activity module
func RegisterAllActivities() error {
	registry := GetGlobalActivityRegistry()

	// Get all activities from the centralized exports
	activities := activity.GetAll()

	// Register each activity using the generic registration method
	for _, act := range activities {
		if err := registry.RegisterGeneric(act); err != nil {
			return fmt.Errorf("failed to register activity: %w", err)
		}
	}

	log.Printf("Successfully registered %d activities", len(registry.List()))
	for _, activityType := range registry.List() {
		log.Printf("  - %s", activityType)
	}

	return nil
}


// CreateActivityProvider creates a provider for a registered activity
// This is used to bridge RegisterableActivity to the existing provider system
func CreateActivityProvider(activityType string) (worker.ActivityProvider, error) {
	registry := GetGlobalActivityRegistry()
	
	registration, exists := registry.Get(activityType)
	if !exists {
		return nil, fmt.Errorf("activity type %s not registered", activityType)
	}

	// Create a generic provider that handles type assertions internally
	// This allows us to work with any RegisterableActivity without knowing its specific types
	return NewGenericActivityProvider(registration.Activity, registration), nil
}

// RegisterActivityProviders registers all activities as providers in the provider registry
func RegisterActivityProviders(providerRegistry *worker.ProviderRegistry) error {
	activityRegistry := GetGlobalActivityRegistry()
	
	for activityType := range activityRegistry.GetAll() {
		provider, err := CreateActivityProvider(activityType)
		if err != nil {
			return fmt.Errorf("failed to create provider for %s: %w", activityType, err)
		}
		
		if err := providerRegistry.Register(provider); err != nil {
			return fmt.Errorf("failed to register provider for %s: %w", activityType, err)
		}
	}
	
	return nil
}

// GenericActivityProvider handles any RegisterableActivity type using reflection
type GenericActivityProvider struct {
	activity     interface{}
	registration ActivityRegistration
}

// GetType returns the activity type identifier
func (p *GenericActivityProvider) GetType() string {
	return p.registration.Metadata.Type
}

// Execute runs the activity with the given arguments using reflection
func (p *GenericActivityProvider) Execute(ctx context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("expected 2 arguments (config, inputs), got %d", len(args))
	}

	configMap, ok := args[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("first argument must be config map[string]interface{}")
	}

	inputsMap, ok := args[1].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("second argument must be inputs map[string]interface{}")
	}

	// Use reflection to call Execute method
	activityValue := reflect.ValueOf(p.activity)
	executeMethod := activityValue.MethodByName("Execute")
	if !executeMethod.IsValid() {
		return nil, fmt.Errorf("activity does not have Execute method")
	}

	// Get the Execute method type to create properly typed arguments
	executeType := executeMethod.Type()
	if executeType.NumIn() != 3 { // ctx, config, input
		return nil, fmt.Errorf("Execute method should have 3 parameters, has %d", executeType.NumIn())
	}

	// Create typed config value
	configType := executeType.In(1)
	configValue := reflect.New(configType).Interface()
	configBytes, _ := json.Marshal(configMap)
	if err := json.Unmarshal(configBytes, configValue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Create typed input value
	inputType := executeType.In(2)
	inputValue := reflect.New(inputType).Interface()
	inputBytes, _ := json.Marshal(inputsMap)
	if err := json.Unmarshal(inputBytes, inputValue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inputs: %w", err)
	}

	// Call Execute with reflection
	results := executeMethod.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(configValue).Elem(),
		reflect.ValueOf(inputValue).Elem(),
	})

	if len(results) != 2 {
		return nil, fmt.Errorf("Execute should return 2 values, returned %d", len(results))
	}

	// Check for error
	if !results[1].IsNil() {
		return nil, results[1].Interface().(error)
	}

	// Convert output to map
	output := results[0].Interface()
	outputBytes, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal output: %w", err)
	}

	var outputMap map[string]interface{}
	if err := json.Unmarshal(outputBytes, &outputMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal output: %w", err)
	}

	return outputMap, nil
}

// GetSchemas returns the JSON schemas for config, input, and output
func (p *GenericActivityProvider) GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{}) {
	// Convert jsonschema.Schema to map[string]interface{}
	var config, input, output map[string]interface{}
	
	if p.registration.ConfigSchema != nil {
		configBytes, _ := json.Marshal(p.registration.ConfigSchema)
		json.Unmarshal(configBytes, &config)
	}
	
	if p.registration.InputSchema != nil {
		inputBytes, _ := json.Marshal(p.registration.InputSchema)
		json.Unmarshal(inputBytes, &input)
	}
	
	if p.registration.OutputSchema != nil {
		outputBytes, _ := json.Marshal(p.registration.OutputSchema)
		json.Unmarshal(outputBytes, &output)
	}
	
	return config, input, output
}

// GetDescription returns the activity description
func (p *GenericActivityProvider) GetDescription() string {
	return p.registration.Metadata.Description
}

// GetSchemaOptions returns schema validation options
func (p *GenericActivityProvider) GetSchemaOptions() worker.SchemaOptions {
	return worker.SchemaOptions{
		RequiredConfig:         true,
		AllowAdditionalConfig:  false,
		AllowAdditionalInputs:  false,
		AllowAdditionalOutputs: true,
	}
}