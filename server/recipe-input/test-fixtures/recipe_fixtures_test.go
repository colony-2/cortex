package test_fixtures_test

import (
	"context"
	"testing"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-input/pkg/input"
	testfixtures "github.com/colony-2/colony2/server/recipe-worker/test-fixtures"
)

type echoInput struct {
	Message string `json:"message"`
}

type echoOutput struct {
	Output string `json:"output"`
}

func TestRecipeFixtures(t *testing.T) {
	coreops.Register(input.GetOp())
	coreops.Register(input.GetAutoFillOp())
	coreops.Register(coreops.NewActivityMappedOpV2[echoInput, echoOutput](
		coreops.OpMetadata{Type: "echo"},
		func(_ coreops.OpDependencies, _ context.Context, in echoInput) (echoOutput, error) {
			return echoOutput{Output: in.Message}, nil
		},
	))

	testfixtures.RunTestOnAllRecipes("recipes/*.test.yaml", t)
}
