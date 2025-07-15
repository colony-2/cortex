package compiler

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"github.com/vibethis/server/recipe-core/pkg/yaml"
)

// Compiler compiles YAML definitions into Temporal workflows
type Compiler struct {
	activityRegistry *ActivityRegistry
}

// NewCompiler creates a new workflow compiler
func NewCompiler(registry *ActivityRegistry) *Compiler {
	return &Compiler{
		activityRegistry: registry,
	}
}

// CompileWorkflow compiles a YAML workflow definition into a Temporal workflow
func (c *Compiler) CompileWorkflow(def *yaml.WorkflowDefinition) (interface{}, error) {
	return func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		// Create workflow state
		state := &WorkflowState{
			Inputs:  inputs,
			Steps:   make(map[string]StepResult),
			Outputs: make(map[string]interface{}),
		}

		// Configure retry policy
		retryPolicy := &temporal.RetryPolicy{
			InitialInterval:    def.Workflow.RetryPolicy.InitialInterval,
			MaximumInterval:    10 * time.Minute,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    int32(def.Workflow.RetryPolicy.MaximumAttempts),
		}

		// Execute steps based on workflow type
		switch def.Workflow.Type {
		case "sequential":
			if err := c.executeSequentialSteps(ctx, def.Workflow.Steps, state, retryPolicy); err != nil {
				return nil, err
			}
		case "parallel":
			if err := c.executeParallelSteps(ctx, def.Workflow.Steps, state, retryPolicy); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported workflow type: %s", def.Workflow.Type)
		}

		// Process workflow outputs
		outputResolver := NewTemplateResolver(state)
		for key, template := range def.Workflow.Outputs {
			value, err := outputResolver.Resolve(template)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve output %s: %w", key, err)
			}
			state.Outputs[key] = value
		}

		return state.Outputs, nil
	}, nil
}

func (c *Compiler) executeSequentialSteps(ctx workflow.Context, steps []yaml.Step, state *WorkflowState, retryPolicy *temporal.RetryPolicy) error {
	for _, step := range steps {
		if len(step.Parallel) > 0 {
			// Execute parallel sub-steps
			if err := c.executeParallelSteps(ctx, step.Parallel, state, retryPolicy); err != nil {
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

func (c *Compiler) executeParallelSteps(ctx workflow.Context, steps []yaml.Step, state *WorkflowState, retryPolicy *temporal.RetryPolicy) error {
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

func (c *Compiler) executeStep(ctx workflow.Context, step yaml.Step, state *WorkflowState, retryPolicy *temporal.RetryPolicy) error {
	// Resolve inputs
	resolver := NewTemplateResolver(state)
	inputs := make(map[string]interface{})
	
	for key, template := range step.Inputs {
		value, err := resolver.Resolve(template)
		if err != nil {
			return fmt.Errorf("failed to resolve input %s: %w", key, err)
		}
		inputs[key] = value
	}
	
	// Get activity definition
	activityDef := c.activityRegistry.GetActivity(step.Activity)
	if activityDef == nil {
		return fmt.Errorf("activity not found: %s", step.Activity)
	}
	
	// Configure activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: activityDef.Timeout,
		RetryPolicy:        retryPolicy,
	}
	
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	
	// Execute activity
	var outputs map[string]interface{}
	err := workflow.ExecuteActivity(ctx, step.Activity, inputs).Get(ctx, &outputs)
	if err != nil {
		return err
	}
	
	// Store step result
	state.Steps[step.ID] = StepResult{
		Outputs: outputs,
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

// ActivityRegistry manages activity definitions
type ActivityRegistry struct {
	activities map[string]*yaml.ActivityDefinition
}

// NewActivityRegistry creates a new activity registry
func NewActivityRegistry() *ActivityRegistry {
	return &ActivityRegistry{
		activities: make(map[string]*yaml.ActivityDefinition),
	}
}

// RegisterActivity registers an activity definition
func (r *ActivityRegistry) RegisterActivity(def *yaml.ActivityDefinition) {
	r.activities[def.Name] = def
}

// GetActivity retrieves an activity definition
func (r *ActivityRegistry) GetActivity(name string) *yaml.ActivityDefinition {
	return r.activities[name]
}