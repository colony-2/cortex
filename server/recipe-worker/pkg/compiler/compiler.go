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
		// Handle new unified format - check which node type is defined
		if def.Op != "" {
			// Single operation node
			return c.executeOperation(ctx, def.Op, def.Inputs, inputs)
		} else if len(def.Sequence) > 0 {
			// Sequential execution
			return c.executeSequenceNodes(ctx, def.Sequence, inputs)
		} else if len(def.Parallel) > 0 {
			// Parallel execution
			return c.executeParallelNodes(ctx, def.Parallel, inputs)
		} else if def.States != nil {
			// State machine execution
			if c.stateMachineCompiler != nil {
				return c.stateMachineCompiler.ExecuteStateMap(ctx, def.States, inputs)
			}
			return nil, fmt.Errorf("state machine compiler not initialized")
		}

		return nil, fmt.Errorf("recipe must define one of: op, sequence, parallel, or states")
}

// CompileWorkflow compiles a unified recipe definition into a Temporal workflow
func (c *Compiler) CompileWorkflow(def *yamlpkg.RecipeDefinition) (interface{}, error) {
	return func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		return c.ExecuteWorkflow(ctx, def, inputs)
	}, nil
}

// Legacy step methods have been removed - use node-based execution instead

// processNodeOutputs processes outputs from node execution
func (c *Compiler) processNodeOutputs(outputs map[string]interface{}, outputTemplates map[string]interface{}) (map[string]interface{}, error) {
	if outputTemplates == nil {
		return outputs, nil
	}
	
	// TODO: Implement output template resolution for new node format
	// For now, return outputs as-is
	return outputs, nil
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

// executeOperation executes a single operation node
func (c *Compiler) executeOperation(ctx workflow.Context, op string, nodeInputs map[string]interface{}, workflowInputs map[string]interface{}) (map[string]interface{}, error) {
	// Merge workflow inputs with node inputs
	inputs := make(map[string]interface{})
	for k, v := range workflowInputs {
		inputs[k] = v
	}
	for k, v := range nodeInputs {
		inputs[k] = v
	}
	
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
	
	// Execute the operation
	var outputs map[string]interface{}
	err := workflow.ExecuteActivity(ctx, op, inputs).Get(ctx, &outputs)
	if err != nil {
		return nil, err
	}
	
	return outputs, nil
}

// executeSequenceNodes executes nodes in sequence
func (c *Compiler) executeSequenceNodes(ctx workflow.Context, nodes []yamlpkg.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	outputs := make(map[string]interface{})
	
	for i, node := range nodes {
		nodeOutputs, err := c.executeNode(ctx, &node, inputs)
		if err != nil {
			return nil, fmt.Errorf("sequence node %d failed: %w", i, err)
		}
		
		// Store outputs with node ID if specified
		if node.ID != "" {
			outputs[node.ID] = nodeOutputs
		}
		
		// Pass outputs as inputs to next node
		for k, v := range nodeOutputs {
			inputs[k] = v
		}
	}
	
	return outputs, nil
}

// executeParallelNodes executes nodes in parallel
func (c *Compiler) executeParallelNodes(ctx workflow.Context, nodes []yamlpkg.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	type nodeResult struct {
		id      string
		outputs map[string]interface{}
		err     error
	}
	
	resultChan := workflow.NewChannel(ctx)
	
	// Start all parallel nodes
	for i, node := range nodes {
		node := node // capture loop variable
		nodeIndex := i
		workflow.Go(ctx, func(ctx workflow.Context) {
			outputs, err := c.executeNode(ctx, &node, inputs)
			resultChan.Send(ctx, nodeResult{
				id:      fmt.Sprintf("%s_%d", node.ID, nodeIndex),
				outputs: outputs,
				err:     err,
			})
		})
	}
	
	// Collect results
	allOutputs := make(map[string]interface{})
	for i := 0; i < len(nodes); i++ {
		var result nodeResult
		resultChan.Receive(ctx, &result)
		
		if result.err != nil {
			return nil, fmt.Errorf("parallel node '%s' failed: %w", result.id, result.err)
		}
		
		// Store outputs with node ID
		if nodes[i].ID != "" {
			allOutputs[nodes[i].ID] = result.outputs
		}
	}
	
	return allOutputs, nil
}

// executeNode executes a single node
func (c *Compiler) executeNode(ctx workflow.Context, node *yamlpkg.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Check conditional execution
	if node.When != "" {
		// TODO: Implement CEL evaluation for conditional execution
		// For now, always execute
	}
	
	// Merge inputs
	mergedInputs := make(map[string]interface{})
	for k, v := range inputs {
		mergedInputs[k] = v
	}
	for k, v := range node.Inputs {
		mergedInputs[k] = v
	}
	
	// Execute based on node type
	if node.Op != "" {
		return c.executeOperation(ctx, node.Op, node.Inputs, inputs)
	} else if len(node.Sequence) > 0 {
		return c.executeSequenceNodes(ctx, node.Sequence, mergedInputs)
	} else if len(node.Parallel) > 0 {
		return c.executeParallelNodes(ctx, node.Parallel, mergedInputs)
	} else if node.States != nil {
		if c.stateMachineCompiler != nil {
			return c.stateMachineCompiler.ExecuteStateMap(ctx, node.States, mergedInputs)
		}
		return nil, fmt.Errorf("state machine compiler not initialized")
	} else if node.Shared != "" {
		// Execute shared node reference - call the shared activity
		sharedActivityName := "shared/" + node.Shared
		return c.executeOperation(ctx, sharedActivityName, node.Inputs, inputs)
	}
	
	return nil, fmt.Errorf("node must define one of: op, sequence, parallel, states, or shared")
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