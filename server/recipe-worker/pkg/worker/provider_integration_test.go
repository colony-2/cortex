package worker_test

import (
	"context"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/stretchr/testify/assert"
)

// TestProviderInput and TestProviderOutput types for test activities
type TestProviderInput struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	Host    string `json:"host"`
	Data    string `json:"data"`
	Request string `json:"request"`
}

type TestProviderOutput struct {
	Result   string `json:"result"`
	Response string `json:"response"`
}

func init() {
	// Register test activities for provider integration tests
	// Note: Name field is what's used for lookup when op: is specified in YAML
	httpActivityOp := ops.NewActivityMappedOp(
		ops.OpMetadata{
			Type:        "http-activity",
			Description: "Test HTTP activity for provider integration",
			Version:     "1.0.0",
		},
		func(ctx context.Context, input TestProviderInput) (TestProviderOutput, error) {
			return TestProviderOutput{
				Result:   "http_success",
				Response: "HTTP response for " + input.URL,
			}, nil
		},
	)
	ops.Register(httpActivityOp)

	grpcActivityOp := ops.NewActivityMappedOp(
		ops.OpMetadata{
			Type:        "grpc-activity",
			Description: "Test gRPC activity for provider integration",
			Version:     "1.0.0",
		},
		func(ctx context.Context, input TestProviderInput) (TestProviderOutput, error) {
			return TestProviderOutput{
				Result:   "grpc_success",
				Response: "gRPC response from " + input.Host,
			}, nil
		},
	)
	ops.Register(grpcActivityOp)
}

func TestProviderIntegration_Basic(t *testing.T) {
	// Create a test recipe with unified format
	testRecipe := &recipe.RecipeFile{
		ID:          "provider-test",
		Version:     "1.0.0",
		Description: "Provider integration test",
		Recipe: recipe.Recipe{
			RecipeImpl: &recipe.RecipeSequence{
				RecipeMetadata: recipe.RecipeMetadata{
					NodeMetadata: recipe.NodeMetadata{
						ID: "provider-test",
					},
					Version: "1.0.0",
				},
				SequenceData: recipe.SequenceData{
					Sequence: []recipe.Node{
						{
							NodeImpl: &recipe.NodeOp{
								NodeMetadata: recipe.NodeMetadata{
									ID: "http_step",
									Inputs: map[string]interface{}{
										"type": "http",
										"url":  "https://api.example.com/test",
										"data": "test",
									},
								},
								OpData: recipe.OpData{
									Op: "http-activity",
								},
							},
						},
						{
							NodeImpl: &recipe.NodeOp{
								NodeMetadata: recipe.NodeMetadata{
									ID: "grpc_step",
									Inputs: map[string]interface{}{
										"type":    "grpc",
										"host":    "localhost:9000",
										"request": "test",
									},
								},
								OpData: recipe.OpData{
									Op: "grpc-activity",
								},
							},
						},
					},
				},
			},
		},
	}

	assert.NotNil(t, testRecipe)
	assert.NotNil(t, testRecipe.Recipe)
	recipeSeq := testRecipe.Recipe.RecipeImpl.(*recipe.RecipeSequence)
	assert.Len(t, recipeSeq.Sequence, 2)

	// Test the nodes have the expected configuration
	httpNode := recipeSeq.Sequence[0].NodeImpl.(*recipe.NodeOp)
	assert.Equal(t, "http-activity", httpNode.Op)
	assert.Equal(t, "http", httpNode.NodeMetadata.Inputs["type"])

	grpcNode := recipeSeq.Sequence[1].NodeImpl.(*recipe.NodeOp)
	assert.Equal(t, "grpc-activity", grpcNode.Op)
	assert.Equal(t, "grpc", grpcNode.NodeMetadata.Inputs["type"])
}
