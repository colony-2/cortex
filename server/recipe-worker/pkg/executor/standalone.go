package executor

import (
	"context"
	"fmt"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	recipeworkflows "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/workflows"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
	"io"
	"log"
	"os"
)

// StandaloneExecutor executes recipes without a Temporal server
type StandaloneExecutor struct {
	registry         *worker.ActivityRegistry
	compiler         *compiler.Compiler
	logger           *zap.Logger
	activityExecutor *ActivityExecutor
}

// NewStandaloneExecutor creates a new standalone recipe executor
func NewStandaloneExecutor(registry *worker.ActivityRegistry, logger *zap.Logger) (*StandaloneExecutor, error) {

	// Create compiler activity registry
	compilerRegistry := compiler.NewActivityRegistry()
	// Register all activities in the compiler registry
	for activityType := range registry.GetAll() {
		compilerRegistry.RegisterActivity(activityType)
	}

	// Create compiler
	comp := compiler.NewCompiler(compilerRegistry)

	// Create activity executor
	activityExecutor := NewActivityExecutor(registry, logger)

	return &StandaloneExecutor{
		registry:         registry,
		compiler:         comp,
		logger:           logger,
		activityExecutor: activityExecutor,
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
	recipe *yamlpkg.RecipeDefinition,
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

	// Create workflow function
	// Use nil executor to use the default implementation that supports unified format
	workflowFunc := recipeworkflows.CreateDynamicWorkflow(recipe, nil)

	// Register workflow
	testEnv.RegisterWorkflowWithOptions(
		workflowFunc,
		workflow.RegisterOptions{
			Name: recipe.Name,
		},
	)

	// Register activities with the test environment
	for activityType := range e.registry.GetAll() {
		// Create a generic activity executor that wraps the real activity
		activityFunc := e.activityExecutor.CreateTemporalActivity(activityType)
		testEnv.RegisterActivityWithOptions(
			activityFunc,
			activity.RegisterOptions{
				Name: activityType,
			},
		)
	}

	// Set context propagators
	testEnv.SetContextPropagators(options.ContextPropagators)

	// Execute workflow
	testEnv.ExecuteWorkflow(recipe.Name, inputs)

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
func (e *StandaloneExecutor) GetActivityRegistry() *worker.ActivityRegistry {
	return e.registry
}

// GetCompiler returns the compiler
func (e *StandaloneExecutor) GetCompiler() *compiler.Compiler {
	return e.compiler
}
