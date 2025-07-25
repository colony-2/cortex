package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vibethis/server/activity/pkg/types"
	worker "github.com/vibethis/server/recipe-worker"
)

// RegisterableActivityProvider wraps any RegisterableActivity for use in recipe-worker
type RegisterableActivityProvider[TConfig any, TInput any, TOutput any] struct {
	activity     types.RegisterableActivity[TConfig, TInput, TOutput]
	registration ActivityRegistration
}

// NewActivityProvider creates a new provider that bridges RegisterableActivity to the provider system
func NewActivityProvider[TConfig any, TInput any, TOutput any](
	activity types.RegisterableActivity[TConfig, TInput, TOutput],
	registration ActivityRegistration,
) *RegisterableActivityProvider[TConfig, TInput, TOutput] {
	return &RegisterableActivityProvider[TConfig, TInput, TOutput]{
		activity:     activity,
		registration: registration,
	}
}

// GetType returns the activity type identifier
func (p *RegisterableActivityProvider[TConfig, TInput, TOutput]) GetType() string {
	return p.activity.GetMetadata().Type
}

// Execute runs the activity with the given arguments
// args[0] = config (map[string]interface{})
// args[1] = inputs (map[string]interface{})
func (p *RegisterableActivityProvider[TConfig, TInput, TOutput]) Execute(
	ctx context.Context,
	args ...interface{},
) (interface{}, error) {
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
	// Unmarshal config from map to typed struct
	configBytes, err := json.Marshal(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var config TConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Unmarshal inputs from map to typed struct
	inputBytes, err := json.Marshal(inputsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inputs: %w", err)
	}

	var inputs TInput
	if err := json.Unmarshal(inputBytes, &inputs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inputs: %w", err)
	}

	// Execute with typed parameters
	output, err := p.activity.Execute(ctx, config, inputs)
	if err != nil {
		return nil, err
	}

	// Convert output back to map
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
func (p *RegisterableActivityProvider[TConfig, TInput, TOutput]) GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{}) {
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
func (p *RegisterableActivityProvider[TConfig, TInput, TOutput]) GetDescription() string {
	return p.activity.GetMetadata().Description
}

// GetSchemaOptions returns schema validation options
func (p *RegisterableActivityProvider[TConfig, TInput, TOutput]) GetSchemaOptions() worker.SchemaOptions {
	return worker.SchemaOptions{
		RequiredConfig:         true,
		AllowAdditionalConfig:  false,
		AllowAdditionalInputs:  false,
		AllowAdditionalOutputs: true,
	}
}