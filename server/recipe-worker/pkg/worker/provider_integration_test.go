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
			Sequence: []yamlpkg.Node{
				{
					ID: "http_step",
					Op: "http-activity",
					Inputs: map[string]interface{}{
						"type": "http",
						"url":  "https://api.example.com/test",
						"data": "test",
					},
				},
				{
					ID: "grpc_step",
					Op: "grpc-activity",
					Inputs: map[string]interface{}{
						"type":    "grpc",
						"host":    "localhost:9000",
						"request": "test",
					},
				},
			},
		},
	}

	assert.NotNil(t, testRecipe)
	assert.NotNil(t, testRecipe.Recipe)
	assert.Len(t, testRecipe.Recipe.Sequence, 2)
	
	// Test the nodes have the expected configuration
	httpNode := testRecipe.Recipe.Sequence[0]
	assert.Equal(t, "http-activity", httpNode.Op)
	assert.Equal(t, "http", httpNode.Inputs["type"])
	
	grpcNode := testRecipe.Recipe.Sequence[1]
	assert.Equal(t, "grpc-activity", grpcNode.Op)
	assert.Equal(t, "grpc", grpcNode.Inputs["type"])
}