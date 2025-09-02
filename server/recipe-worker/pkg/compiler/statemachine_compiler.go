package compiler

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.temporal.io/sdk/workflow"
)

// ExecuteStateMap runs the state machine with the new StateMap format
func executeStateMachine(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, stateMap *recipe.StateMap, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Create resolution context for the state machine
	resCtx, err := NewResolutionContext("state_machine", "sm-root")
	if err != nil {
		return nil, fmt.Errorf("failed to create resolution context: %w", err)
	}
	resCtx.TemplateData.Inputs = inputs
	
	// Initialize state tracking
	currentState := stateMap.Initial
	stateAttempts := make(map[string]int)

	// Execute state machine
	for !isTerminalState(currentState, stateMap.States) {
		// Get current state definition
		stateDef, exists := stateMap.States[currentState]
		if !exists {
			return nil, fmt.Errorf("state '%s' not found", currentState)
		}

		// Create a child context for the state if it's a sequence
		var stateOutputs map[string]interface{}
		var stateResCtx *ResolutionContext
		
		// Check if state is a sequence that needs its own scope
		switch stateDef.NodeImpl.(type) {
		case *recipe.NodeSequence:
			// Create child context for sequence scope
			stateResCtx, err = resCtx.NewChildContext("state", currentState, resCtx.TemplateData.Inputs)
			if err != nil {
				return nil, fmt.Errorf("failed to create state context: %w", err)
			}
			// Execute state with child context that can see parent states
			stateOutputs, err = executeStateNode(ctx, activityRegistry, &stateDef.Node, stateResCtx)
		default:
			// For non-sequence states, use parent context directly
			stateOutputs, err = executeStateNode(ctx, activityRegistry, &stateDef.Node, resCtx)
		}
		
		if err != nil {
			// Handle retry if configured
			return nil, fmt.Errorf("state '%s' execution failed: %w", currentState, err)
		}

		// Store state outputs in parent context
		resCtx.AddStateOutput(currentState, stateOutputs)

		// Evaluate transitions using resolution context
		nextState, err := evaluateTransitionsWithContext(stateDef.Transitions, stateOutputs, resCtx, currentState)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate state transitions: %w", err)
		}
		stateAttempts[currentState]++
		if nextState == "" {
			// No transition matched, state machine completes
			break
		}

		currentState = nextState
	}

	// Return final outputs
	finalState := stateMap.States[currentState]
	var outputTemplates map[string]interface{}
	switch t := finalState.NodeImpl.(type) {
	case *recipe.NodeState:
		outputTemplates = t.Outputs
	case *recipe.NodeSequence:
		outputTemplates = t.Outputs
	}
	
	// If there are output templates, resolve them
	if len(finalState.Transitions) == 0 && outputTemplates != nil {
		resolvedOutputs := make(map[string]interface{})
		for key, tmpl := range outputTemplates {
			resolved, err := resCtx.ResolveValue(tmpl)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve output template %s: %w", key, err)
			}
			resolvedOutputs[key] = resolved
		}
		return resolvedOutputs, nil
	}

	// Return the last state's outputs
	if lastState, ok := resCtx.TemplateData.States[currentState]; ok {
		return lastState.Outputs, nil
	}

	// Return all state outputs as a single map
	allOutputs := make(map[string]interface{})
	for stateName, stateOutput := range resCtx.TemplateData.States {
		allOutputs[stateName] = stateOutput.Outputs
	}
	return allOutputs, nil
}

// isTerminalState checks if a state is terminal using the new State type
func isTerminalState(stateName string, states map[string]recipe.State) bool {
	state, exists := states[stateName]
	if !exists {
		return true // Non-existent state is terminal
	}
	return len(state.Transitions) == 0
}

// evaluateTransitionsWithContext evaluates transitions using resolution context
func evaluateTransitionsWithContext(transitions []recipe.Transition, currentOutputs map[string]interface{}, resCtx *ResolutionContext, currentState string) (string, error) {
	// Create a temporary context for transition evaluation
	// This context should have access to the current state's sequence nodes if it's a sequence
	evalCtx := &ResolutionContext{
		ScopeType:    resCtx.ScopeType,
		TemplateData: resCtx.TemplateData,
		CELEnv:       resCtx.CELEnv,
	}
	
	// If current state was a sequence, add its nodes to the sequence context for transition evaluation
	if currentOutputs != nil {
		// Check if outputs contain node references (from a sequence)
		if sequence, ok := currentOutputs["__sequence_nodes__"]; ok {
			if seqMap, ok := sequence.(map[string]NodeOutput); ok {
				evalCtx.TemplateData.Sequence = seqMap
			}
		}
	}
	
	for _, transition := range transitions {
		shouldTransition, err := evalCtx.EvaluateCEL(transition.When.String())
		if err != nil {
			return "", fmt.Errorf("failed to evaluate transition condition: %w", err)
		}

		if shouldTransition {
			return transition.To, nil
		}
	}
	return "", nil
}

// executeStateNode executes a node within a state with proper context
func executeStateNode(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, node *recipe.Node, resCtx *ResolutionContext) (map[string]interface{}, error) {
	metadata := node.GetMetadata()
	
	// Resolve the node's inputs if any
	resolvedInputs := make(map[string]interface{})
	for k, v := range resCtx.TemplateData.Inputs {
		resolvedInputs[k] = v
	}
	
	if metadata.Inputs != nil {
		for k, v := range metadata.Inputs {
			resolved, err := resCtx.ResolveValue(v)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve input %s: %w", k, err)
			}
			resolvedInputs[k] = resolved
		}
	}
	
	// Execute based on node type
	switch t := node.NodeImpl.(type) {
	case *recipe.NodeSequence:
		// Execute sequence and collect node outputs
		nodeOutputs := make(map[string]interface{})
		for i, seqNode := range t.SequenceData.Sequence {
			seqMeta := seqNode.GetMetadata()
			
			// Prepare inputs for this node
			nodeInputs := make(map[string]interface{})
			for k, v := range resolvedInputs {
				nodeInputs[k] = v
			}
			
			// Resolve node's input templates if any
			if seqMeta.Inputs != nil {
				for k, v := range seqMeta.Inputs {
					resolved, err := resCtx.ResolveValue(v)
					if err != nil {
						return nil, fmt.Errorf("failed to resolve input %s for node %s: %w", k, seqMeta.ID, err)
					}
					nodeInputs[k] = resolved
				}
			}
			
			// Execute the node
			outputs, err := executeNode(ctx, activityRegistry, &seqNode, nodeInputs)
			if err != nil {
				return nil, fmt.Errorf("sequence node %d failed: %w", i, err)
			}
			
			// Store outputs with node's ID
			if seqMeta.ID != "" {
				resCtx.AddSequenceNode(seqMeta.ID, outputs)
				nodeOutputs[seqMeta.ID] = outputs
			}
		}
		
		// Process sequence outputs if defined
		if t.SequenceData.Outputs != nil {
			resolvedOutputs := make(map[string]interface{})
			for key, tmpl := range t.SequenceData.Outputs {
				resolved, err := resCtx.ResolveValue(tmpl)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve output template %s: %w", key, err)
				}
				resolvedOutputs[key] = resolved
			}
			// Add sequence nodes for transition evaluation
			resolvedOutputs["__sequence_nodes__"] = resCtx.TemplateData.Sequence
			return resolvedOutputs, nil
		}
		
		// Return node outputs with sequence nodes for transition evaluation
		nodeOutputs["__sequence_nodes__"] = resCtx.TemplateData.Sequence
		return nodeOutputs, nil
		
	default:
		// For other node types, execute normally
		return executeNode(ctx, activityRegistry, node, resolvedInputs)
	}
}
