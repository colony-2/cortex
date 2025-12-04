package compiler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	ops2 "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CompilerTestSuite struct {
	suite.Suite
	eng  swf.SWFEngine
	deps ops2.OpDependencies
	//eng *impl.EmbeddedEngine
}

func (s *CompilerTestSuite) SetupTest() {
	s.eng = toy.NewToyEngine([]swf.WorkSet{})

	deps, err := ops2.NewOpDependenciesBuilder().Build()
	require.NoError(s.T(), err)
	s.deps = deps
	//eng, err := impl.StartEmbeddedEngine(context.Background(), nil)
}

func (s *CompilerTestSuite) AfterTest(suiteName, testName string) {
	//s.eng.Shutdown()
}

func TestCompilerTestSuite(t *testing.T) {
	suite.Run(t, new(CompilerTestSuite))
}

type simpleFunc func(context.Context, map[string]interface{}) (map[string]interface{}, error)

func registerActivity(registry *ops.ActivityRegistry, name string, fn simpleFunc) error {
	op := ops2.NewActivityMappedOpV2[map[string]interface{}, map[string]interface{}](
		ops2.OpMetadata{Type: name},
		func(inv ops2.OpDependencies, aCtx context.Context, in map[string]interface{}) (map[string]interface{}, error) {
			return fn(aCtx, in)
		},
	)
	return ops.Register(registry, op)
}

func (s *CompilerTestSuite) testRecipe(recipeYaml string, input map[string]interface{}, expectedOutputs map[string]interface{}) {

	registry, err := ops.NewActivityRegistry()
	require.NoError(s.T(), err)
	testRecipe, err := recipe.LoadRecipeFromString([]byte(recipeYaml))
	require.NoError(s.T(), err)
	workSet, err := NewRecipeWorker(ops2.NewServiceDepsBuilder().Build(), registry)
	require.NoError(s.T(), err)
	err = s.eng.RegisterWorkers(workSet)
	require.NoError(s.T(), err)

	jobCtx, gitCtx := generateTestContext()

	job := workflowctl.StartJob{
		RecipeName: "test-recipe",
		Inputs:     input,
		JobContext: jobCtx,
		GitContext: gitCtx,
	}

	jobId, err := StartRecipeJob(context.Background(), job, s.eng, *testRecipe)
	require.NoError(s.T(), err)
	require.NoError(s.T(), swf.WaitForJobToComplete(context.Background(), 30*time.Second, jobId, s.eng))
	r, err := s.eng.GetJobResult(context.Background(), jobId)
	require.NoError(s.T(), err)
	d, err := r.GetData()
	require.NoError(s.T(), err)
	out := make(map[string]interface{})
	require.NoError(s.T(), json.Unmarshal(d, &out))
	assert.Equal(s.T(), expectedOutputs, out)
}

func (s *CompilerTestSuite) TestCompileSimpleRecipe() {

	recipeYaml := `
---
id: test-recipe
input_schema:
  param1:
    type: string
inputs:
  iParam1: "{{ .inputs.param1 }}"
sequence:
  - id: echo
    op: echo_activity
    inputs:
      Message: "{{ .inputs.iParam1 }}"
outputs:
  result: "{{ .sequence.echo.outputs.output }}"
`

	input := map[string]interface{}{
		"param1": "Hello, World!",
	}

	expectedOutputs := map[string]interface{}{"result": "Hello, World!"}
	s.testRecipe(recipeYaml, input, expectedOutputs)
}

func (s *CompilerTestSuite) TestSequenceRecipeCompilation() {
	// Test sequence compilation instead of parallel
	node := &recipe.Node{
		NodeImpl: &recipe.NodeSequence{
			NodeMetadata: recipe.NodeMetadata{
				ID: "sequence-recipe",
			},
			SequenceData: recipe.SequenceData{
				Sequence: []recipe.Node{
					{
						NodeImpl: &recipe.NodeOp{
							NodeMetadata: recipe.NodeMetadata{
								ID: "task_a",
								Inputs: map[string]interface{}{
									"type": "function",
								},
							},
							OpData: recipe.OpData{
								Op: "activity_a",
							},
						},
					},
					{
						NodeImpl: &recipe.NodeOp{
							NodeMetadata: recipe.NodeMetadata{
								ID: "task_b",
								Inputs: map[string]interface{}{
									"type": "function",
								},
							},
							OpData: recipe.OpData{
								Op: "activity_b",
							},
						},
					},
				},
			},
		},
	}

	// Create activity registry
	registry, err := ops.NewActivityRegistry()
	require.NoError(s.T(), err)

	// Register test activity functions
	activityAFunc := func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"result": "output_a"}, nil
	}
	activityBFunc := func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"result": "output_b"}, nil
	}
	if _, exists := registry.Get("activity_a"); !exists {
		require.NoError(s.T(), registerActivity(registry, "activity_a", activityAFunc))
	}
	if _, exists := registry.Get("activity_b"); !exists {
		require.NoError(s.T(), registerActivity(registry, "activity_b", activityBFunc))
	}

	testRecipe := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeSequence{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: node.NodeImpl.(*recipe.NodeSequence).NodeMetadata,
			},
			SequenceData: node.NodeImpl.(*recipe.NodeSequence).SequenceData,
		},
	}

	workSet, err := NewRecipeWorker(ops2.NewServiceDepsBuilder().Build(), registry)
	err = s.eng.RegisterWorkers(workSet)
	stop := context.Background()
	s.eng.Run(stop) // we start after worker registration.

	require.NoError(s.T(), err)
	jobCtx, gitCtx := generateTestContext()
	in := map[string]interface{}{}

	job := workflowctl.StartJob{
		RecipeName: testRecipe.GetMetadata().ID,
		Inputs:     in,
		JobContext: jobCtx,
		GitContext: gitCtx,
	}

	jobId, err := StartRecipeJob(context.Background(), job, s.eng, *testRecipe)
	require.NoError(s.T(), err)
	require.NoError(s.T(), swf.WaitForJobToComplete(context.Background(), 30*time.Second, jobId, s.eng))
	r, err := s.eng.GetJobResult(context.Background(), jobId)
	require.NoError(s.T(), err)
	d, err := r.GetData()
	require.NoError(s.T(), err)
	out := make(map[string]interface{})
	require.NoError(s.T(), json.Unmarshal(d, &out))
}

func TestActivityRegistry(t *testing.T) {
	// Activity registry is now in ops package
	registry, _ := ops.NewActivityRegistry()

	// Test that registry exists and can be created
	assert.NotNil(t, registry)

	// List activities (should be empty or have defaults)
	activities := registry.List()
	assert.NotNil(t, activities)
}

func (s *CompilerTestSuite) TestRecipeWithSharedActivities() {
	recipeYaml := `
id: shared-recipe
version: 1.0
defs:
  my_llm:
    op: llm
    inputs:
      model: gpt-4
      type: ai_prompt

sequence:
  - shared: my_llm
`

	recipeDef, err := recipe.LoadRecipeFromString([]byte(recipeYaml))
	require.NoError(s.T(), err)

	// ensure the sequence node is of type llm.
	assert.NotNil(s.T(), recipeDef)
	recipeSeq := recipeDef.RecipeImpl.(*recipe.RecipeSequence)
	assert.Equal(s.T(), "1.0", recipeSeq.RecipeMetadata.Version)
	item := recipeSeq.Sequence[0]
	assert.NotNil(s.T(), recipeSeq.Sequence[0])
	op := item.NodeImpl.(*recipe.NodeOp)
	assert.Equal(s.T(), "llm", op.Op)
	assert.Len(s.T(), recipeSeq.Sequence, 1)
}
