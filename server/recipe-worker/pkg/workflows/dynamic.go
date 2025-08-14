package workflows

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// WorkflowExecutor interface for executing workflow logic
type WorkflowExecutor interface {
	ExecuteWorkflow(ctx workflow.Context, recipeDef *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error)
}

// ActivityInvoker interface for invoking activities from workflows
type ActivityInvoker interface {
	InvokeActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error)
}

// NodeExecutor handles execution of different node types
type NodeExecutor struct {
	ctx workflow.Context
}

// CreateDynamicWorkflow creates a Temporal workflow function from a unified recipe definition
func CreateDynamicWorkflow(recipeDef *yamlpkg.RecipeDefinition, executor WorkflowExecutor) interface{} {
	return func(ctx workflow.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		if executor != nil {
			return executor.ExecuteWorkflow(ctx, recipeDef, inputs)
		}
		
		// Set activity options
		ao := workflow.ActivityOptions{
			StartToCloseTimeout: 10 * time.Minute,
		}
		ctx = workflow.WithActivityOptions(ctx, ao)
		
		// Default implementation - execute based on root node type
		nodeExec := &NodeExecutor{ctx: ctx}
		
		// Execute based on root node type
		if recipeDef.Op != "" {
			// Root is an operation - resolve templates in recipe inputs
			resolvedInputs, err := nodeExec.resolveInputs(recipeDef.Inputs, inputs)
			if err != nil {
				return nil, err
			}
			result, err := nodeExec.executeOperation(recipeDef.Op, resolvedInputs)
			if err != nil {
				return nil, err
			}
			// Resolve outputs if defined
			if recipeDef.Outputs != nil {
				outputData := map[string]interface{}{
					"inputs": inputs,
				}
				// Add activity results to the template context
				for k, v := range result {
					outputData[k] = v
				}
				resolvedOutputs := make(map[string]interface{})
				for key, outputExpr := range recipeDef.Outputs {
					if strExpr, ok := outputExpr.(string); ok {
						resolved, err := nodeExec.resolveValue(strExpr, outputData)
						if err != nil {
							return nil, fmt.Errorf("failed to resolve output %s: %w", key, err)
						}
						resolvedOutputs[key] = resolved
					} else {
						resolvedOutputs[key] = outputExpr
					}
				}
				return resolvedOutputs, nil
			}
			return result, nil
		} else if len(recipeDef.Sequence) > 0 {
			// Root is a sequence
			result, err := nodeExec.executeSequence(recipeDef.Sequence, inputs)
			if err != nil {
				return nil, err
			}
			// If no outputs defined, return empty map (sequence side effects only)
			if recipeDef.Outputs == nil {
				return map[string]interface{}{}, nil
			}
			// TODO: Process outputs if defined
			return result, nil
		} else if len(recipeDef.Parallel) > 0 {
			// Root is parallel
			return nodeExec.executeParallel(recipeDef.Parallel, inputs)
		} else if recipeDef.States != nil {
			// Root is a state machine
			return nodeExec.executeStateMachine(recipeDef.States, inputs)
		}
		
		return nil, fmt.Errorf("recipe has no valid root node")
	}
}

// executeOperation executes a single operation node
func (e *NodeExecutor) executeOperation(op string, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Execute the operation as an activity
	var result map[string]interface{}
	err := workflow.ExecuteActivity(e.ctx, op, inputs).Get(e.ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to execute operation %s: %w", op, err)
	}
	return result, nil
}

// executeSequence executes nodes in sequence
func (e *NodeExecutor) executeSequence(nodes []yamlpkg.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	nodeOutputs := make(map[string]interface{})
	
	for _, node := range nodes {
		// Check conditional execution
		if node.When != "" && !e.evaluateCondition(node.When, nodeOutputs) {
			continue
		}
		
		// Merge inputs with node-specific inputs
		nodeInputs := e.mergeInputs(inputs, node.Inputs, nodeOutputs)
		
		// Execute the node
		result, err := e.executeNode(&node, nodeInputs)
		if err != nil {
			return nil, fmt.Errorf("failed to execute node %s: %w", node.ID, err)
		}
		
		// Store node outputs
		if node.ID != "" {
			nodeOutputs[node.ID] = result
		}
	}
	
	results["nodes"] = nodeOutputs
	return results, nil
}

// executeParallel executes nodes in parallel
func (e *NodeExecutor) executeParallel(nodes []yamlpkg.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	nodeOutputs := make(map[string]interface{})
	
	// Create futures for parallel execution
	var futures []workflow.Future
	var nodeIDs []string
	
	for _, node := range nodes {
		// Check conditional execution
		if node.When != "" && !e.evaluateCondition(node.When, inputs) {
			continue
		}
		
		node := node // Capture for closure
		nodeInputs := e.mergeInputs(inputs, node.Inputs, nil)
		
		future := workflow.ExecuteActivity(e.ctx, func(ctx context.Context) (map[string]interface{}, error) {
			return e.executeNode(&node, nodeInputs)
		})
		
		futures = append(futures, future)
		nodeIDs = append(nodeIDs, node.ID)
	}
	
	// Wait for all parallel executions
	for i, future := range futures {
		var result map[string]interface{}
		err := future.Get(e.ctx, &result)
		if err != nil {
			return nil, fmt.Errorf("failed to execute parallel node %s: %w", nodeIDs[i], err)
		}
		if nodeIDs[i] != "" {
			nodeOutputs[nodeIDs[i]] = result
		}
	}
	
	results["nodes"] = nodeOutputs
	return results, nil
}

// executeStateMachine executes a state machine
func (e *NodeExecutor) executeStateMachine(states *yamlpkg.StateMap, inputs map[string]interface{}) (map[string]interface{}, error) {
	currentState := states.Initial
	stateOutputs := make(map[string]interface{})
	
	for {
		state, exists := states.States[currentState]
		if !exists {
			return nil, fmt.Errorf("state %s not found", currentState)
		}
		
		// Execute the state as a node
		stateNode := yamlpkg.Node{
			Op:       state.Op,
			Sequence: state.Sequence,
			Parallel: state.Parallel,
			States:   state.States,
			Shared:   state.Shared,
			Inputs:   state.Inputs,
			Outputs:  state.Outputs,
			Timeout:  state.Timeout,
			Retry:    state.Retry,
		}
		
		result, err := e.executeNode(&stateNode, inputs)
		if err != nil {
			if state.Error != "" {
				return nil, fmt.Errorf("%s: %w", state.Error, err)
			}
			return nil, err
		}
		
		stateOutputs[currentState] = result
		
		// Check for terminal state
		if len(state.Transitions) == 0 {
			break
		}
		
		// Evaluate transitions
		nextState := ""
		for _, transition := range state.Transitions {
			if transition.When == "" || e.evaluateCondition(transition.When, result) {
				nextState = transition.To
				break
			}
		}
		
		if nextState == "" {
			return nil, fmt.Errorf("no valid transition from state %s", currentState)
		}
		
		currentState = nextState
	}
	
	return map[string]interface{}{
		"states":       stateOutputs,
		"current_state": currentState,
	}, nil
}

// executeNode executes a single node based on its type
func (e *NodeExecutor) executeNode(node *yamlpkg.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	if node.Op != "" {
		return e.executeOperation(node.Op, inputs)
	} else if len(node.Sequence) > 0 {
		return e.executeSequence(node.Sequence, inputs)
	} else if len(node.Parallel) > 0 {
		return e.executeParallel(node.Parallel, inputs)
	} else if node.States != nil {
		return e.executeStateMachine(node.States, inputs)
	} else if node.Shared != "" {
		// TODO: Implement shared node resolution
		return nil, fmt.Errorf("shared nodes not yet implemented")
	}
	
	return nil, fmt.Errorf("node has no valid execution type")
}

// resolveInputs resolves templates in input values
func (e *NodeExecutor) resolveInputs(nodeInputs map[string]interface{}, workflowInputs map[string]interface{}) (map[string]interface{}, error) {
	resolved := make(map[string]interface{})
	
	// Create template context
	templateData := map[string]interface{}{
		"inputs": workflowInputs,
	}
	
	// Resolve each input value
	for key, value := range nodeInputs {
		resolvedValue, err := e.resolveValue(value, templateData)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve input %s: %w", key, err)
		}
		resolved[key] = resolvedValue
	}
	
	return resolved, nil
}

// resolveValue resolves a single value that may contain templates
func (e *NodeExecutor) resolveValue(value interface{}, data map[string]interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// Check if it contains templates
		if strings.Contains(v, "{{") && strings.Contains(v, "}}") {
			return e.executeTemplate(v, data)
		}
		return v, nil
	case map[string]interface{}:
		// Recursively resolve map values
		resolved := make(map[string]interface{})
		for k, val := range v {
			resolvedVal, err := e.resolveValue(val, data)
			if err != nil {
				return nil, err
			}
			resolved[k] = resolvedVal
		}
		return resolved, nil
	case []interface{}:
		// Recursively resolve array values
		resolved := make([]interface{}, len(v))
		for i, val := range v {
			resolvedVal, err := e.resolveValue(val, data)
			if err != nil {
				return nil, err
			}
			resolved[i] = resolvedVal
		}
		return resolved, nil
	default:
		// Return other types as-is
		return v, nil
	}
}

// executeTemplate executes a Go template string
func (e *NodeExecutor) executeTemplate(expr string, data map[string]interface{}) (interface{}, error) {
	tmpl, err := template.New("expr").Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	
	return buf.String(), nil
}

// mergeInputs merges various input sources
func (e *NodeExecutor) mergeInputs(baseInputs, nodeInputs map[string]interface{}, nodeOutputs map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})
	
	// Copy base inputs
	for k, v := range baseInputs {
		merged[k] = v
	}
	
	// Override with node-specific inputs
	for k, v := range nodeInputs {
		merged[k] = v
	}
	
	// Add node outputs for reference
	if nodeOutputs != nil {
		merged["nodes"] = nodeOutputs
	}
	
	return merged
}

// evaluateCondition evaluates a CEL condition
func (e *NodeExecutor) evaluateCondition(condition string, context map[string]interface{}) bool {
	// TODO: Implement proper CEL evaluation
	// For now, return true to allow execution
	return true
}

// CreateDynamicActivity creates a Temporal activity function for operations
func CreateDynamicActivity(op string) interface{} {
	return func(ctx context.Context, inputs map[string]interface{}) (map[string]interface{}, error) {
		// Get activity info for logging
		info := activity.GetInfo(ctx)
		
		// Execute based on operation type
		switch op {
		case yamlpkg.OpCommandExecution:
			// Execute command
			if run, ok := inputs["run"].(string); ok {
				// TODO: Implement actual command execution
				return map[string]interface{}{
					"stdout":    fmt.Sprintf("Executed: %s", run),
					"stderr":    "",
					"exit_code": 0,
				}, nil
			}
			return nil, fmt.Errorf("missing 'run' input for command_execution")
			
		case yamlpkg.OpSleep:
			// Sleep operation
			if _, ok := inputs["duration"].(string); ok {
				// TODO: Parse and sleep for duration
				return map[string]interface{}{
					"status": "completed",
				}, nil
			}
			return nil, fmt.Errorf("missing 'duration' input for sleep")
			
		default:
			// Generic operation placeholder
			return map[string]interface{}{
				"status":      "completed",
				"operation":   op,
				"activityID":  info.ActivityID,
			}, nil
		}
	}
}