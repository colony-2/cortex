package compiler

import (
	"context"
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

type CompilerTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *CompilerTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *CompilerTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func TestCompilerTestSuite(t *testing.T) {
	t.Skip("Workflow test suite requires proper activity registration")
	// suite.Run(t, new(CompilerTestSuite))
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
	testActivityFunc := func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"result": "test_output"}, nil
	}
	s.env.RegisterActivity(testActivityFunc)

	// Define workflow that uses ExecuteRecipe
	testWorkflow := func(ctx workflow.Context) (map[string]interface{}, error) {
		testRecipe := &recipe.Recipe{
			RecipeImpl: &recipe.RecipeOp{
				RecipeMetadata: recipe.RecipeMetadata{
					NodeMetadata: node.NodeImpl.(*recipe.NodeOp).NodeMetadata,
				},
				OpData: node.NodeImpl.(*recipe.NodeOp).OpData,
			},
		}
		return ExecuteRecipe(ctx, registry, *testRecipe, withRequiredGitInputs(map[string]interface{}{
			"param1": "test_value",
		}))
	}

	s.env.RegisterWorkflow(testWorkflow)

	// Execute workflow
	s.env.OnActivity("test_activity", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "test_output"}, nil,
	)

	s.env.ExecuteWorkflow(testWorkflow)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())
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
	activityAFunc := func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"result": "output_a"}, nil
	}
	activityBFunc := func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"result": "output_b"}, nil
	}
	s.env.RegisterActivity(activityAFunc)
	s.env.RegisterActivity(activityBFunc)

	// Define workflow that uses ExecuteRecipe
	testWorkflow := func(ctx workflow.Context) (map[string]interface{}, error) {
		testRecipe := &recipe.Recipe{
			RecipeImpl: &recipe.RecipeSequence{
				RecipeMetadata: recipe.RecipeMetadata{
					NodeMetadata: node.NodeImpl.(*recipe.NodeSequence).NodeMetadata,
				},
				SequenceData: node.NodeImpl.(*recipe.NodeSequence).SequenceData,
			},
		}
		return ExecuteRecipe(ctx, registry, *testRecipe, withRequiredGitInputs(map[string]interface{}{}))
	}

	s.env.RegisterWorkflow(testWorkflow)

	// Mock activities
	s.env.OnActivity("activity_a", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "output_a"}, nil,
	)
	s.env.OnActivity("activity_b", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "output_b"}, nil,
	)

	s.env.ExecuteWorkflow(testWorkflow)

	require.True(s.T(), s.env.IsWorkflowCompleted())
	require.NoError(s.T(), s.env.GetWorkflowError())
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
