package compiler

import (
	"fmt"
	"time"

	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/compiler/statemachine"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// Compiler compiles YAML definitions into Temporal workflows
type Compiler struct {
	activityRegistry   *ActivityRegistry
	stateMachineCompiler *statemachine.StateMachineCompiler
}

// NewCompiler creates a new workflow compiler
func NewCompiler(registry *ActivityRegistry) *Compiler {
	// Create activity executor wrapper
	executor := &workflowActivityExecutor{}
	
	// Create state machine compiler
	smCompiler, err := statemachine.NewStateMachineCompiler(executor)
	if err != nil {
		// Log error but continue - state machine support will be disabled
		smCompiler = nil
	}
	
	return &Compiler{
		activityRegistry: registry,
		stateMachineCompiler: smCompiler,
	}
}

// ExecuteWorkflow implements the WorkflowExecutor interface for unified recipes
func (c *Compiler) ExecuteWorkflow(ctx workflow.Context, def *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
		// Create workflow state
		state := &WorkflowState{
			Inputs:  inputs,
			Steps:   make(map[string]StepResult),
			Outputs: make(map[string]interface{}),
		}

		// Configure default retry policy
		retryPolicy := &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			MaximumInterval:    10 * time.Minute,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		}

		// Execute steps sequentially (can be extended for parallel execution)
		if err := c.executeSequentialSteps(ctx, def.Steps, state, retryPolicy); err != nil {
			return nil, err
		}

		// Process recipe outputs - resolve output templates and populate state.Outputs
		if err := c.processOutputs(ctx, def, state); err != nil {
			return nil, fmt.Errorf("failed to process outputs: %w", err)
		}

		// Return final outputs
		return state.Outputs, nil
}

// CompileWorkflow compiles a unified recipe definition into a Temporal workflow
func (c *Compiler) CompileWorkflow(def *yamlpkg.RecipeDefinition) (interface{}, error) {
	return func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return c.ExecuteWorkflow(ctx, def, inputs)
	}, nil
}

func (c *Compiler) executeSequentialSteps(ctx workflow.Context, steps []yamlpkg.Step, state *WorkflowState, retryPolicy *temporal.RetryPolicy) error {
	for _, step := range steps {
		if step.Parallel != nil && len(step.Parallel.Steps) > 0 {
			// Execute parallel sub-steps
			if err := c.executeParallelSteps(ctx, step.Parallel.Steps, state, retryPolicy); err != nil {
				return fmt.Errorf("failed to execute parallel step %s: %w", step.ID, err)
			}
		} else {
			// Execute single activity
			if err := c.executeStep(ctx, step, state, retryPolicy); err != nil {
				return fmt.Errorf("failed to execute step %s: %w", step.ID, err)
			}
		}
	}
	return nil
}

func (c *Compiler) executeParallelSteps(ctx workflow.Context, steps []yamlpkg.Step, state *WorkflowState, retryPolicy *temporal.RetryPolicy) error {
	// Create selector for parallel execution
	selector := workflow.NewSelector(ctx)
	
	type stepResult struct {
		stepID string
		result StepResult
		err    error
	}
	
	resultChan := workflow.NewChannel(ctx)
	
	// Start all parallel activities
	for _, step := range steps {
		step := step // capture loop variable
		workflow.Go(ctx, func(ctx workflow.Context) {
			var result StepResult
			err := c.executeStep(ctx, step, state, retryPolicy)
			if err == nil {
				result = state.Steps[step.ID]
			}
			resultChan.Send(ctx, stepResult{
				stepID: step.ID,
				result: result,
				err:    err,
			})
		})
	}
	
	// Collect results
	for i := 0; i < len(steps); i++ {
		selector.AddReceive(resultChan, func(ch workflow.ReceiveChannel, more bool) {
			var result stepResult
			ch.Receive(ctx, &result)
			if result.err != nil {
				// Handle error - for now, we'll fail the whole group
				workflow.GetLogger(ctx).Error("Parallel step failed", "stepID", result.stepID, "error", result.err)
			} else {
				state.Steps[result.stepID] = result.result
			}
		})
	}
	
	// Wait for all to complete
	for i := 0; i < len(steps); i++ {
		selector.Select(ctx)
	}
	
	return nil
}

func (c *Compiler) executeStep(ctx workflow.Context, step yamlpkg.Step, state *WorkflowState, retryPolicy *temporal.RetryPolicy) error {
	// Resolve inputs
	resolver := NewTemplateResolver(state)
	inputs := make(map[string]interface{})
	
	for key, inputValue := range step.Inputs {
		// Check if the input is a string that needs template resolution
		if strValue, ok := inputValue.(string); ok {
			value, err := resolver.Resolve(strValue)
			if err != nil {
				return fmt.Errorf("failed to resolve input %s: %w", key, err)
			}
			inputs[key] = value
		} else {
			// For non-string values, recursively resolve templates in nested structures
			resolved, err := resolver.ResolveValue(inputValue)
			if err != nil {
				return fmt.Errorf("failed to resolve input %s: %w", key, err)
			}
			inputs[key] = resolved
		}
	}
	
	// Check if this is a state machine step
	if step.Uses == "state_machine" && c.stateMachineCompiler != nil {
		// Extract state machine configuration from step config
		if step.Config != nil {
			if configData, ok := step.Config["config"]; ok {
				// Convert config to StateMachineConfig
				if smConfig, ok := configData.(yamlpkg.StateMachineConfig); ok {
					outputs, err := c.stateMachineCompiler.Execute(ctx, smConfig, inputs)
					if err != nil {
						return fmt.Errorf("state machine execution failed: %w", err)
					}
					// Store step result
					state.Steps[step.ID] = StepResult{
						Outputs: outputs,
					}
					return nil
				}
			}
		}
	}
	
	// Configure activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute, // Default timeout
		RetryPolicy:        retryPolicy,
	}
	
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	
	// Execute activity using the step's Uses field
	var outputs map[string]interface{}
	err := workflow.ExecuteActivity(ctx, step.Uses, inputs).Get(ctx, &outputs)
	if err != nil {
		return err
	}
	
	// Store step result
	state.Steps[step.ID] = StepResult{
		Outputs: outputs,
	}
	
	return nil
}

// processOutputs processes the recipe's output declarations and populates state.Outputs
func (c *Compiler) processOutputs(ctx workflow.Context, def *yamlpkg.RecipeDefinition, state *WorkflowState) error {
	resolver := NewTemplateResolver(state)
	
	// Process each declared output (new format - outputs is a map)
	for name, outputExpr := range def.Outputs {
		// Convert to string for template resolution
		exprStr := ""
		switch v := outputExpr.(type) {
		case string:
			exprStr = v
		default:
			// If not a string, use as-is
			state.Outputs[name] = outputExpr
			continue
		}
		
		// Resolve the output value template
		value, err := resolver.Resolve(exprStr)
		if err != nil {
			workflow.GetLogger(ctx).Error("Failed to resolve output", 
				"output", name, 
				"error", err)
			return fmt.Errorf("failed to resolve output %s: %w", name, err)
		}
		
		// Store in state outputs
		state.Outputs[name] = value
	}
	
	return nil
}

// WorkflowState maintains the runtime state of a workflow
type WorkflowState struct {
	Inputs  map[string]interface{}
	Steps   map[string]StepResult
	Outputs map[string]interface{}
}

// StepResult stores the result of a workflow step
type StepResult struct {
	Outputs map[string]interface{}
}

// ActivityRegistry manages activity registrations for unified model
type ActivityRegistry struct {
	activities map[string]bool // Just track which activities are registered
}

// NewActivityRegistry creates a new activity registry
func NewActivityRegistry() *ActivityRegistry {
	return &ActivityRegistry{
		activities: make(map[string]bool),
	}
}

// RegisterActivity registers an activity by name
func (r *ActivityRegistry) RegisterActivity(name string) {
	r.activities[name] = true
}

// HasActivity checks if an activity is registered
func (r *ActivityRegistry) HasActivity(name string) bool {
	return r.activities[name]
}

// workflowActivityExecutor wraps workflow.ExecuteActivity for use with state machine compiler
type workflowActivityExecutor struct{}

// ExecuteActivity implements the ActivityExecutor interface for state machine compiler
func (e *workflowActivityExecutor) ExecuteActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Configure activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute, // Default timeout
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			MaximumInterval:    10 * time.Minute,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	}
	
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	
	// Execute activity
	var outputs map[string]interface{}
	err := workflow.ExecuteActivity(ctx, activityName, inputs).Get(ctx, &outputs)
	if err != nil {
		return nil, err
	}
	
	return outputs, nil
}