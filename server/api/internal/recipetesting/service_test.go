package recipetesting

import (
	"context"
	"testing"

	"github.com/colony-2/colony2/server/project/pkg/project"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/require"
)

func TestExecute_AllowsMockedArtifactBindings(t *testing.T) {
	svc := NewService(nil, coreops.NewServiceDepsBuilder().Build())

	req := recipeTestCaseRequest{
		TargetRecipe: recipeTestTargetRecipe{
			Mode:   "inline_recipe",
			Format: "yaml",
			Content: `
id: mocked-artifact-binding
version: "1.0.0"
input_schema: {}
sequence:
  - id: write
    op: command_execution
    inputs:
      run: "echo write"
  - id: read
    op: command_execution
    artifacts:
      foo.txt: '${{ sequence.write.artifacts["foo.txt"] }}'
    inputs:
      run: "cat foo.txt"
outputs:
  result: "{{ sequence.read.outputs.stdout }}"
`,
		},
		Case: recipeTestCase{
			ID:   "artifact-binding-mock",
			Type: "recipe_case",
			Mocks: recipeTestMocks{
				Ops: []recipeTestOpMock{
					{
						Match: recipeTestOpMockMatch{Op: "command_execution"},
						Behavior: recipeTestMockBehavior{
							Mode:      "return",
							Outputs:   map[string]any{"stdout": "ok"},
							Artifacts: map[string]string{"foo.txt": "payload"},
						},
					},
				},
			},
		},
	}

	projectID := project.ID("recipe-test-project")
	prepared := svc.Prepare(context.Background(), projectID, req)
	require.True(t, prepared.Validation.Valid, "validation errors: %+v", prepared.Validation.Errors)

	resp := svc.Execute(context.Background(), projectID, req, prepared)
	require.Equal(t, "passed", resp.Status, "failure reason: %s", resp.FailureReason)
	require.Equal(t, "ok", resp.Outputs["result"])
}
