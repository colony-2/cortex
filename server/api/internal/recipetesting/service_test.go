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

func TestSelectOpMock_ConsumesDuplicatesInDeclarationOrder(t *testing.T) {
	mocks := []recipeTestOpMock{
		{
			Match: recipeTestOpMockMatch{NodePath: "root.loop", Op: "input"},
			Behavior: recipeTestMockBehavior{
				Mode: "return",
			},
		},
		{
			Match: recipeTestOpMockMatch{NodePath: "root.loop", Op: "input"},
			Behavior: recipeTestMockBehavior{
				Mode: "return",
			},
		},
	}

	consumed := map[int]struct{}{}

	idx, ok := selectOpMock(mocks, "root.loop", "input", consumed)
	require.True(t, ok)
	require.Equal(t, 0, idx)
	consumed[idx] = struct{}{}

	idx, ok = selectOpMock(mocks, "root.loop", "input", consumed)
	require.True(t, ok)
	require.Equal(t, 1, idx)
	consumed[idx] = struct{}{}

	_, ok = selectOpMock(mocks, "root.loop", "input", consumed)
	require.False(t, ok)
}

func TestRecipeTestJobContext_ReusesMockWithinInvocationButNotAcrossInvocations(t *testing.T) {
	j := newRecipeTestJobContext(
		project.ID("recipe-test-project"),
		recipeTestCase{
			ID:   "reuse-by-invocation",
			Type: "recipe_case",
			Mocks: recipeTestMocks{
				Ops: []recipeTestOpMock{
					{
						Match: recipeTestOpMockMatch{NodePath: "root.loop", Op: "input"},
						Behavior: recipeTestMockBehavior{
							Mode: "return",
						},
					},
				},
			},
		},
		recipeTestPolicy{},
		coreops.NewServiceDepsBuilder().Build(),
	)

	_, ok := j.selectOpMockForInvocation("root.loop::input::1", "root.loop", "input")
	require.True(t, ok)
	require.Len(t, j.consumedMockIdxs, 1)

	_, ok = j.selectOpMockForInvocation("root.loop::input::1", "root.loop", "input")
	require.True(t, ok)
	require.Len(t, j.consumedMockIdxs, 1)

	_, ok = j.selectOpMockForInvocation("root.loop::input::2", "root.loop", "input")
	require.False(t, ok)
	require.True(t, hasOpMockCandidate(j.caseDef.Mocks.Ops, "root.loop", "input"))
}
