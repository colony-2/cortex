package worker

import (
	"context"
	"encoding/json"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
	worker "github.com/divisive-ai/vibethis/server/recipe-worker"
)

// RegisterableActivityProvider wraps any RegisterableOp for use in recipe-worker
type RegisterableActivityProvider struct {
	activity     types.RegisterableOp
	registration ActivityRegistration
}

// NewActivityProvider creates a new provider that bridges RegisterableOp to the provider system
func NewActivityProvider(
	activity types.RegisterableOp,
	registration ActivityRegistration,
) *RegisterableActivityProvider {
	return &RegisterableActivityProvider{
		activity:     activity,
		registration: registration,
	}
}

// GetType returns the activity type identifier
func (p *RegisterableActivityProvider) GetType() string {
	return p.activity.GetMetadata().Type
}

// Execute runs the activity with the given arguments
// args[0] = config (map[string]interface{})
// args[1] = inputs (map[string]interface{})
func (p *RegisterableActivityProvider) Execute(
	ctx context.Context,
	config map[string]interface{}, input map[string]interface{},
) (interface{}, error) {
	return p.activity.Execute(ctx, config, input)
}

// GetSchemas returns the JSON schemas for config, input, and output
func (p *RegisterableActivityProvider) GetSchemas() (configSchema, inputSchema, outputSchema map[string]interface{}) {
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
func (p *RegisterableActivityProvider) GetDescription() string {
	return p.activity.GetMetadata().Description
}

// GetSchemaOptions returns schema validation options
func (p *RegisterableActivityProvider) GetSchemaOptions() worker.SchemaOptions {
	return worker.SchemaOptions{
		RequiredConfig:         true,
		AllowAdditionalConfig:  false,
		AllowAdditionalInputs:  false,
		AllowAdditionalOutputs: true,
	}
}
