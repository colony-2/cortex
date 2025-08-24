package compiler

import (
	"fmt"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.temporal.io/sdk/workflow"
)

// ExecuteStateMap runs the state machine with the new StateMap format
func executeStateMachine(ctx workflow.Context, activityRegistry *ops.ActivityRegistry, node *yamlpkg.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	stateMap := node.States
	// Initialize state context
	stateCtx := &yamlpkg.StateContext{
		CurrentState: stateMap.Initial,
		Inputs:       inputs,
		StateOutputs: make(map[string]map[string]interface{}),
		Attempts:     make(map[string]int),
		StateInfo:    make(map[string]*yamlpkg.StateInfo),
		StepOutputs:  make(map[string]interface{}),
	}

	// Extract recipe context from inputs if available
	if ctxVal, ok := inputs["context"]; ok {
		if recipeCtx, ok := ctxVal.(*yamlpkg.RecipeContext); ok {
			stateCtx.RecipeContext = recipeCtx
		}
	}

	// Execute state machine
	for !isTerminalState(stateCtx.CurrentState, stateMap.States) {
		// Get current state definition
		stateDef, exists := stateMap.States[stateCtx.CurrentState]
		if !exists {
			return nil, fmt.Errorf("state '%s' not found", stateCtx.CurrentState)
		}

		// Track state entry
		stateCtx.StateInfo[stateCtx.CurrentState] = &yamlpkg.StateInfo{
			Name:      stateCtx.CurrentState,
			Attempts:  stateCtx.Attempts[stateCtx.CurrentState],
			EnteredAt: workflow.Now(ctx),
		}

		// Execute state
		preparedInputs := prepareInputs(stateDef.Inputs, stateCtx)
		outputs, err := ExecuteNode(ctx, activityRegistry, &stateDef.Node, preparedInputs)
		if err != nil {
			// Handle retry if configured
			return nil, fmt.Errorf("state '%s' execution failed: %w", stateCtx.CurrentState, err)
		}

		// Store state outputs
		stateCtx.StateOutputs[stateCtx.CurrentState] = outputs

		// Evaluate transitions
		nextState, err := evaluateTransitions(stateDef.Transitions, outputs, stateCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate state transitions: %w", err)
		}
		stateCtx.Attempts[stateCtx.CurrentState]++
		if nextState == "" {
			// No transition matched, state machine completes
			break
		}

		stateCtx.CurrentState = nextState
	}

	// Return final outputs
	finalState := stateMap.States[stateCtx.CurrentState]
	if len(finalState.Transitions) == 0 && finalState.Outputs != nil {
		return prepareOutputs(finalState.Outputs, stateCtx), nil
	}

	// Return the last state's outputs
	if lastOutputs, ok := stateCtx.StateOutputs[stateCtx.CurrentState]; ok {
		return lastOutputs, nil
	}

	// Return all state outputs as a single map
	allOutputs := make(map[string]interface{})
	for stateName, stateOutputs := range stateCtx.StateOutputs {
		allOutputs[stateName] = stateOutputs
	}
	return allOutputs, nil
}

// isTerminalState checks if a state is terminal using the new State type
func isTerminalState(stateName string, states map[string]yamlpkg.State) bool {
	state, exists := states[stateName]
	if !exists {
		return true // Non-existent state is terminal
	}
	return len(state.Transitions) == 0
}

// evaluateTransitions evaluates transitions using the new Transition type
func evaluateTransitions(transitions []yamlpkg.Transition, outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) (string, error) {
	for _, transition := range transitions {
		shouldTransition, err := transition.When.AsBool(outputs)
		if err != nil {
			return "", fmt.Errorf("failed to evaluate transition condition: %w", err)
		}

		if shouldTransition {
			return transition.To, nil
		}

	}
	return "", nil
}

// prepareInputs prepares inputs by resolving templates
func prepareInputs(inputs map[string]interface{}, stateCtx *yamlpkg.StateContext) map[string]interface{} {
	if inputs == nil {
		return make(map[string]interface{})
	}

	resolver := NewTemplateResolver2()
	resolved, err := resolver.ResolveInputs(inputs, stateCtx)
	if err != nil {
		// Log error and return original inputs
		return inputs
	}
	return resolved
}

// prepareOutputs prepares outputs by resolving templates
func prepareOutputs(outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) map[string]interface{} {
	if outputs == nil {
		return make(map[string]interface{})
	}

	resolver := NewTemplateResolver2()
	resolved, err := resolver.ResolveOutputs(outputs, stateCtx)
	if err != nil {
		// Log error and return original outputs
		return outputs
	}
	return resolved
}
