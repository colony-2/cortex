package compiler

import (
	"context"
	"testing"
	"time"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/impl"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	task2 "github.com/divisive-ai/vibethis/server/recipe-core/pkg/task"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CompilerTestSuite struct {
	suite.Suite
	eng *impl.EmbeddedEngine
}

func (s *CompilerTestSuite) SetupTest() {
	eng, err := impl.StartEmbeddedEngine(context.Background(), nil)
	if err != nil {
		s.T().Fatalf("fail starting engine %v", err)
	}
	s.eng = eng
}

func (s *CompilerTestSuite) AfterTest(suiteName, testName string) {
	s.eng.Shutdown()
}

func TestCompilerTestSuite(t *testing.T) {
	suite.Run(t, new(CompilerTestSuite))
}

func (s *CompilerTestSuite) TestCompileSimpleRecipe() {
	// Create a simple recipe definition using unified format
	node := &recipe.Node{
		NodeImpl: &recipe.NodeOp{
			NodeMetadata: recipe.NodeMetadata{
				ID:   "test-recipe",
				Desc: "Test recipe",
				Inputs: map[string]interface{}{
					"type":   "function",
					"param1": "test_value",
				},
			},
			OpData: recipe.OpData{
				Op: "test_activity",
			},
		},
	}

	// Create activity registry
	registry, err := ops.NewActivityRegistry()
	require.NoError(s.T(), err)

	// Register a test activity function
	testActivityFunc := func(ctx task2.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"result": "test_output"}, nil
	}
	task := task2.AsTask("test_activity", testActivityFunc)

	testRecipe := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeOp{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: node.NodeImpl.(*recipe.NodeOp).NodeMetadata,
			},
			OpData: node.NodeImpl.(*recipe.NodeOp).OpData,
		},
	}

	worker := NewRecipeWorker(registry)
	err = s.eng.RegisterWorkers(worker, task)
	require.NoError(s.T(), err)

	input := withRequiredGitInputs(map[string]interface{}{
		"param1": "test_value",
	})

	jobId, err := StartRecipeJob(context.Background(), input, s.eng, *testRecipe)
	require.NoError(s.T(), err)
	require.NoError(s.T(), swf.WaitForJobToComplete(context.Background(), 5*time.Second, jobId, s.eng))
	r, err := s.eng.GetJobResult(context.Background(), jobId)
	require.NoError(s.T(), err)
	d, err := r.GetData()
	require.NoError(s.T(), err)
	asMap, err := d.ToMap()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), map[string]interface{}{"result": "test_output"}, asMap)
}

func TestTemplateResolver(t *testing.T) {
	state := &WorkflowState{
		Inputs: map[string]interface{}{
			"topic":       "AI Agents",
			"max_sources": 5,
		},
		Steps: map[string]StepResult{
			"research": {
				Outputs: map[string]interface{}{
					"sources": []string{"source1", "source2"},
					"summary": "Research summary",
				},
			},
		},
	}

	resolver := NewTemplateResolver(state)

	tests := []struct {
		name     string
		template string
		expected interface{}
	}{
		{
			name:     "simple input reference",
			template: "{{ .Inputs.topic }}",
			expected: "AI Agents",
		},
		{
			name:     "step output reference",
			template: "{{ .Steps.research.outputs.summary }}",
			expected: "Research summary",
		},
		{
			name:     "no template",
			template: "plain text",
			expected: "plain text",
		},
		{
			name:     "complex template",
			template: "Research on {{ .Inputs.topic }} with {{ .Inputs.max_sources }} sources",
			expected: "Research on AI Agents with 5 sources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.Resolve(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
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
	activityAFunc := func(ctx task2.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"result": "output_a"}, nil
	}
	activityBFunc := func(ctx task2.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"result": "output_b"}, nil
	}

	t1 := task2.AsTask("activity_a", activityAFunc)
	t2 := task2.AsTask("activity_b", activityBFunc)

	testRecipe := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeSequence{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: node.NodeImpl.(*recipe.NodeSequence).NodeMetadata,
			},
			SequenceData: node.NodeImpl.(*recipe.NodeSequence).SequenceData,
		},
	}

	worker := NewRecipeWorker(registry)
	err = s.eng.RegisterWorkers(worker, t1, t2)
	require.NoError(s.T(), err)
	in := withRequiredGitInputs(map[string]interface{}{})

	jobId, err := StartRecipeJob(context.Background(), in, s.eng, *testRecipe)
	require.NoError(s.T(), err)
	require.NoError(s.T(), swf.WaitForJobToComplete(context.Background(), 5*time.Second, jobId, s.eng))
	r, err := s.eng.GetJobResult(context.Background(), jobId)
	require.NoError(s.T(), err)
	d, err := r.GetData()
	require.NoError(s.T(), err)
	_, err = d.ToMap()
	require.NoError(s.T(), err)
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
	recipeDef := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeSequence{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: recipe.NodeMetadata{
					ID: "shared-recipe",
				},
				Version: "1.0",
				Defs: map[string]recipe.Node{
					"my_llm": {
						NodeImpl: &recipe.NodeOp{
							NodeMetadata: recipe.NodeMetadata{
								Inputs: map[string]interface{}{
									"type":  "ai_prompt",
									"model": "gpt-4",
								},
							},
							OpData: recipe.OpData{
								Op: "llm",
							},
						},
					},
				},
			},
			SequenceData: recipe.SequenceData{
				Sequence: []recipe.Node{
					{
						NodeImpl: &recipe.NodeShared{
							Shared: "my_llm",
						},
					},
				},
			},
		},
	}

	// Create activity registry
	registry, err := ops.NewActivityRegistry()
	require.NoError(s.T(), err)

	// For now just verify structure is correct
	assert.NotNil(s.T(), recipeDef)
	assert.NotNil(s.T(), registry)
	recipeSeq := recipeDef.RecipeImpl.(*recipe.RecipeSequence)
	assert.Equal(s.T(), "1.0", recipeSeq.RecipeMetadata.Version)
	assert.NotNil(s.T(), recipeSeq.RecipeMetadata.Defs)
	assert.Len(s.T(), recipeSeq.Sequence, 1)
}
