package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	recipe "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap/zaptest"
)

var (
	repoOnce sync.Once
	repoPath string
	repoHash string
)

func ensureTestRepo() (string, string) {
	repoOnce.Do(func() {
		dir, err := os.MkdirTemp("", "worker-test-repo-*")
		if err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "init"); err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "config", "user.email", "test@example.com"); err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "config", "user.name", "Test User"); err != nil {
			panic(err)
		}
		readme := filepath.Join(dir, "README.md")
		if err := os.WriteFile(readme, []byte("initial\n"), 0o644); err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "add", "."); err != nil {
			panic(err)
		}
		if err := runGit(dir, "git", "commit", "-m", "init"); err != nil {
			panic(err)
		}
		output, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
		if err != nil {
			panic(fmt.Errorf("rev-parse HEAD failed: %w (%s)", err, output))
		}
		repoPath = dir
		repoHash = strings.TrimSpace(string(output))
	})
	return repoPath, repoHash
}

func runGit(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v failed: %w (%s)", args, err, out)
	}
	return nil
}

func withRequiredGitInputs(inputs map[string]interface{}) map[string]interface{} {
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	repo, hash := ensureTestRepo()
	if _, ok := inputs["basegitrepo"]; !ok {
		inputs["basegitrepo"] = repo
	}
	if _, ok := inputs["basegithash"]; !ok {
		inputs["basegithash"] = hash
	}
	if _, ok := inputs["ticketid"]; !ok {
		inputs["ticketid"] = "TEST-TICKET"
	}
	if _, ok := inputs["cellname"]; !ok {
		inputs["cellname"] = "test-cell"
	}
	return inputs
}

type WorkerIntegrationTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *WorkerIntegrationTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	primeRecipeMetadataSignal(s.env)
}

func (s *WorkerIntegrationTestSuite) AfterTest(suiteName, testName string) {
	s.env.AssertExpectations(s.T())
}

func TestWorkerIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(WorkerIntegrationTestSuite))
}

func (s *WorkerIntegrationTestSuite) TestSimpleWorkflowExecution() {
	s.T().Skip("Workflow execution requires proper activity registration")
	// Create a unified recipe definition
	recipeDef := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeOp{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: recipe.NodeMetadata{
					ID:   "test-recipe",
					Desc: "Test recipe",
					Inputs: map[string]interface{}{
						"text": "Hello, World!",
					},
				},
				Version: "1.0",
			},
			OpData: recipe.OpData{
				Op: "echo-activity",
			},
		},
	}

	// Create activity registry
	registry, err := ops.NewActivityRegistry()
	s.Require().NoError(err)

	// Create mock activity function
	echoActivityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"echo_result": inputs["text"],
		}, nil
	}

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		echoActivityFunc,
		activity.RegisterOptions{
			Name: "echo-activity",
		},
	)

	// Create a workflow that uses ExecuteRecipe
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, *recipeDef, withRequiredGitInputs(inputs))
	}
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "test-recipe",
		},
	)

	// Mock the activity - must be after RegisterWorkflow
	s.env.OnActivity("echo-activity", mock.Anything, map[string]interface{}{"text": "Hello, World!"}).Return(
		map[string]interface{}{"echo_result": "Hello, World!"},
		nil,
	)

	// Execute the workflow with empty inputs since the text is hardcoded
	s.env.ExecuteWorkflow("test-recipe", withRequiredGitInputs(map[string]interface{}{}))

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestSequenceWorkflowExecution() {
	s.T().Skip("Sequence execution test")
	// Create a recipe with sequence steps
	recipeDef := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeSequence{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: recipe.NodeMetadata{
					ID: "sequence-recipe",
				},
				Version: "1.0",
			},
			SequenceData: recipe.SequenceData{
				Sequence: []recipe.Node{
					{
						NodeImpl: &recipe.NodeOp{
							NodeMetadata: recipe.NodeMetadata{
								ID: "task1",
								Inputs: map[string]interface{}{
									"data": "data1",
								},
							},
							OpData: recipe.OpData{
								Op: "process-activity",
							},
						},
					},
					{
						NodeImpl: &recipe.NodeOp{
							NodeMetadata: recipe.NodeMetadata{
								ID: "task2",
								Inputs: map[string]interface{}{
									"data": "data2",
								},
							},
							OpData: recipe.OpData{
								Op: "process-activity",
							},
						},
					},
				},
			},
		},
	}

	// Create activity registry
	registry, err := ops.NewActivityRegistry()
	s.Require().NoError(err)

	// Create mock activity function
	processActivityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		data := inputs["data"].(string)
		return map[string]interface{}{
			"result": "processed-" + data,
		}, nil
	}

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		processActivityFunc,
		activity.RegisterOptions{
			Name: "process-activity",
		},
	)

	// Create a workflow that uses ExecuteRecipe
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, *recipeDef, withRequiredGitInputs(inputs))
	}
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "sequence-recipe",
		},
	)

	// Mock the activity calls - must be after RegisterWorkflow
	s.env.OnActivity("process-activity", mock.Anything, map[string]interface{}{"data": "data1"}).Return(
		map[string]interface{}{"result": "processed-data1"},
		nil,
	)
	s.env.OnActivity("process-activity", mock.Anything, map[string]interface{}{"data": "data2"}).Return(
		map[string]interface{}{"result": "processed-data2"},
		nil,
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("sequence-recipe", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestSharedActivityWorkflow() {
	s.T().Skip("Shared activity resolution needs proper implementation")
	// Create a recipe with shared activities
	recipeDef := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeSequence{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: recipe.NodeMetadata{
					ID:   "shared-recipe",
					Desc: "Shared activity test recipe",
				},
				Version: "1.0",
				Defs: map[string]recipe.Node{
					"my_processor": {
						NodeImpl: &recipe.NodeOp{
							NodeMetadata: recipe.NodeMetadata{
								Inputs: map[string]interface{}{
									"type":    "function",
									"timeout": "30s",
								},
							},
							OpData: recipe.OpData{
								Op: "process-data",
							},
						},
					},
				},
			},
			SequenceData: recipe.SequenceData{
				Sequence: []recipe.Node{
					{
						NodeImpl: &recipe.NodeShared{
							Shared: "my_processor",
						},
					},
				},
			},
		},
	}

	// Create activity registry
	registry, err := ops.NewActivityRegistry()
	s.Require().NoError(err)

	// Create mock activity function
	processActivityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"result": "processed: " + inputs["input"].(string),
		}, nil
	}

	// Register shared activity with name
	s.env.RegisterActivityWithOptions(
		processActivityFunc,
		activity.RegisterOptions{
			Name: "shared/my_processor",
		},
	)

	// Create a workflow that uses ExecuteRecipe
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, *recipeDef, withRequiredGitInputs(inputs))
	}
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "shared-recipe",
		},
	)

	// Mock the activity call
	s.env.OnActivity("shared/my_processor", mock.Anything, mock.Anything).Return(
		map[string]interface{}{"result": "processed: test data"},
		nil,
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("shared-recipe", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestWorkflowWithRetry() {
	s.T().Skip("Retry handling needs proper implementation")
	// Create a workflow with retry functionality
	recipeDef := &recipe.Recipe{
		RecipeImpl: &recipe.RecipeOp{
			RecipeMetadata: recipe.RecipeMetadata{
				NodeMetadata: recipe.NodeMetadata{
					ID: "retry-recipe",
					Inputs: map[string]interface{}{
						"attempt": "1",
					},
				},
				Version: "1.0",
			},
			OpData: recipe.OpData{
				Op: "flaky-activity",
			},
		},
	}

	// Create activity registry
	registry, err := ops.NewActivityRegistry()
	s.Require().NoError(err)

	// Create mock activity function that fails first time
	attemptCount := 0
	flakyActivityFunc := func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		attemptCount++
		if attemptCount == 1 {
			return nil, temporal.NewApplicationError("temporary failure", "TEMPORARY")
		}
		return map[string]interface{}{"result": "success after retry"}, nil
	}

	// Register activity with name
	s.env.RegisterActivityWithOptions(
		flakyActivityFunc,
		activity.RegisterOptions{
			Name: "flaky-activity",
		},
	)

	// Create a workflow that uses ExecuteRecipe
	workflowFunc := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, registry, *recipeDef, withRequiredGitInputs(inputs))
	}
	s.env.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: "retry-recipe",
		},
	)

	// Mock the activity to fail first then succeed
	callCount := 0
	s.env.OnActivity("flaky-activity", mock.Anything, mock.Anything).Return(
		func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
			callCount++
			if callCount == 1 {
				return nil, temporal.NewApplicationError("temporary failure", "TEMPORARY")
			}
			return map[string]interface{}{"result": "success after retry"}, nil
		},
	)

	// Execute the workflow
	s.env.ExecuteWorkflow("retry-recipe", map[string]interface{}{})

	// Verify the result
	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result map[string]interface{}
	s.NoError(s.env.GetWorkflowResult(&result))
	s.NotNil(result)
}

func (s *WorkerIntegrationTestSuite) TestWorkerManagerWithMockClient() {
	logger := zaptest.NewLogger(s.T())

	// Create a test recipe using unified format
	testRecipe := &recipe.RecipeFile{
		ID:          "test-recipe",
		Version:     "1.0.0",
		Description: "Test recipe",
		Recipe: recipe.Recipe{
			RecipeImpl: &recipe.RecipeOp{
				RecipeMetadata: recipe.RecipeMetadata{
					NodeMetadata: recipe.NodeMetadata{
						ID: "test-recipe",
						Inputs: map[string]interface{}{
							"input": "test",
						},
					},
					Version: "1.0.0",
				},
				OpData: recipe.OpData{
					Op: "test-activity",
				},
			},
		},
	}

	// Create WorkerManager with nil client for this test
	// In a real test we would use a mock client
	manager := NewWorkerManager(logger, nil)

	// Verify task queue naming
	taskQueue := manager.GetTaskQueueForRecipe("test-recipe")
	s.Equal("ono-recipes-test-recipe", taskQueue)

	// Test worker status for non-existent worker
	status := manager.GetWorkerStatus("test-recipe")
	s.Equal(recipe.WorkerStatusStopped, status)

	// Test error case for stopping non-existent worker
	err := manager.StopWorker("non-existent")
	s.Error(err)
	s.Contains(err.Error(), "worker not found")

	// Verify the recipe was created correctly
	s.NotNil(testRecipe)
	s.NotNil(testRecipe.Recipe)
	s.Equal("test-recipe", testRecipe.ID)
	s.Equal("1.0.0", testRecipe.Version)
}
