//go:build temporal
// +build temporal

package compiler

import (
	"context"
	"testing"

	recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	gitrecipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"gopkg.in/yaml.v3"
)

type contextProbeInput struct {
	Value string `json:"value"`
}

type contextProbeOutput struct {
	Captured string `json:"captured"`
}

func registerContextProbeOp(t *testing.T) func() {
	existing := recipeops.List()
	recipeops.Clear()
	recipeops.Register(existing...)

	probeOp := recipeops.NewActivityMappedOpV2[contextProbeInput, contextProbeOutput](
		recipeops.OpMetadata{
			Type:        "context_probe",
			Description: "captures value passed through template",
			Version:     "1.0.0",
		},
		func(inv recipeops.Invocation, ctx context.Context, input contextProbeInput) (contextProbeOutput, error) {
			return contextProbeOutput{Captured: input.Value}, nil
		},
	)

	recipeops.Register(probeOp)

	return func() {
		recipeops.Clear()
		recipeops.Register(existing...)
	}
}

func TestContextShortcutExposesGitMetadata(t *testing.T) {
	cleanup := registerContextProbeOp(t)
	defer cleanup()

	const recipeYAML = `
        id: context-shortcut
        version: '1.0'
        sequence:
          - id: capture
            op: context_probe
            inputs:
              value: '{{ context.git.base_hash }}'
        outputs:
          captured: '{{ sequence.capture.outputs.captured }}'
    `

	var compiled gitrecipe.Recipe
	require.NoError(t, yaml.Unmarshal([]byte(recipeYAML), &compiled))

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)

	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)

	registry.EnableActivitiesInWorker(env)
	primeDefaultMetadataSignal(env)

	_, expectedHash := ensureTestRepo()

	env.ExecuteWorkflow(func(ctx workflow.Context) (map[string]interface{}, error) {
		inputs, execCtx := withRequiredGitInputs(nil)
		return ExecuteRecipe(ctx, registry, compiled, inputs, execCtx)
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var outputs map[string]interface{}
	require.NoError(t, env.GetWorkflowResult(&outputs))

	captured, ok := outputs["captured"].(string)
	require.True(t, ok)
	require.Equal(t, expectedHash, captured)
}
