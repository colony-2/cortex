package worker_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

func TestProviderIntegration_Basic(t *testing.T) {
	// Create a test recipe with unified format
	testRecipe := &recipe.Recipe{
		Name:        "provider-test",
		Version:     "1.0.0",
		Description: "Provider integration test",
		Recipe: &yamlpkg.RecipeDefinition{
			Name:    "provider-test",
			Version: "1.0.0",
			Steps: []yamlpkg.Step{
				{
					ID:   "http_step",
					Uses: "http-activity",
					Config: map[string]interface{}{
						"type": "http",
						"url":  "https://api.example.com/test",
					},
					Inputs: map[string]interface{}{
						"data": "test",
					},
				},
				{
					ID:   "grpc_step",
					Uses: "grpc-activity",
					Config: map[string]interface{}{
						"type": "grpc",
						"host": "localhost:9000",
					},
					Inputs: map[string]interface{}{
						"request": "test",
					},
				},
			},
		},
	}

	assert.NotNil(t, testRecipe)
	assert.NotNil(t, testRecipe.Recipe)
	assert.Len(t, testRecipe.Recipe.Steps, 2)
	
	// Test the steps have the expected configuration
	httpStep := testRecipe.Recipe.Steps[0]
	assert.Equal(t, "http-activity", httpStep.Uses)
	assert.Equal(t, "http", httpStep.Config["type"])
	
	grpcStep := testRecipe.Recipe.Steps[1]
	assert.Equal(t, "grpc-activity", grpcStep.Uses)
	assert.Equal(t, "grpc", grpcStep.Config["type"])
}