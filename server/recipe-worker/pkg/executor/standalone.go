package executor

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

// StandaloneExecutor executes recipes without a Temporal server
type StandaloneExecutor struct {
	registry *ops.ActivityRegistry
	logger   *zap.Logger
}

// NewStandaloneExecutor creates a new standalone recipe executor
func NewStandaloneExecutor(registry *ops.ActivityRegistry, logger *zap.Logger) (*StandaloneExecutor, error) {

	return &StandaloneExecutor{
		registry: registry,
		logger:   logger,
	}, nil
}

// ExecutionOptions configures the execution behavior
type ExecutionOptions struct {
	// SuppressLogs suppresses test environment debug logs
	SuppressLogs bool
	// ContextPropagators for workflow context
	ContextPropagators []workflow.ContextPropagator
}

// DefaultExecutionOptions returns default execution options
func DefaultExecutionOptions() ExecutionOptions {
	return ExecutionOptions{
		SuppressLogs:       true,
		ContextPropagators: []workflow.ContextPropagator{},
	}
}

// Execute runs a recipe with the given inputs
func (e *StandaloneExecutor) Execute(
	ctx context.Context,
	r recipe.Recipe,
	inputs map[string]interface{},
	opts ...ExecutionOptions,
) (map[string]interface{}, error) {
	// Use default options if none provided
	options := DefaultExecutionOptions()
	if len(opts) > 0 {
		options = opts[0]
	}

	// Suppress test environment debug logs if requested
	var originalLogger io.Writer
	var originalStdout *os.File
	if options.SuppressLogs {
		// Set environment variable to disable temporal test debug logs
		os.Setenv("TEMPORAL_DEBUG", "false")
		defer os.Unsetenv("TEMPORAL_DEBUG")

		// Redirect standard log output to discard
		originalLogger = log.Writer()
		log.SetOutput(io.Discard)
		defer log.SetOutput(originalLogger)

		// Also suppress standard output temporarily
		originalStdout = os.Stdout
		os.Stdout, _ = os.Open(os.DevNull)
		defer func() { os.Stdout = originalStdout }()
	}

	// Create test environment for execution
	testSuite := &testsuite.WorkflowTestSuite{}
	testEnv := testSuite.NewTestWorkflowEnvironment()

	// Disable test environment debug logging
	if options.SuppressLogs {
		testEnv.SetOnActivityCompletedListener(nil)
	}

	e.registry.EnableActivitiesInWorker(testEnv)
	fn := func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return compiler.ExecuteRecipe(ctx, e.registry, r, inputs)
	}

	// Register workflow
	testEnv.RegisterWorkflowWithOptions(
		fn,
		workflow.RegisterOptions{
			Name: r.GetMetdata().ID,
		},
	)

	// Set context propagators
	testEnv.SetContextPropagators(options.ContextPropagators)

	// Execute workflow
	testEnv.ExecuteWorkflow(r.GetMetdata().ID, inputs)

	// Check for errors
	if err := testEnv.GetWorkflowError(); err != nil {
		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("execution timeout exceeded")
		}
		return nil, err
	}

	// Get result
	var outputs map[string]interface{}
	if err := testEnv.GetWorkflowResult(&outputs); err != nil {
		return nil, err
	}

	return outputs, nil
}

// GetActivityRegistry returns the activity registry
func (e *StandaloneExecutor) GetActivityRegistry() *ops.ActivityRegistry {
	return e.registry
}
