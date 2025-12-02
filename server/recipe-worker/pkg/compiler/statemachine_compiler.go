package compiler

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflow"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/template"
)

// ExecuteStateMap runs the state machine with the new StateMap format
func executeStateMachine(ctx workflow.Context, parentContext *template.ResolutionContext, activityRegistry *workerops.ActivityRegistry, metadata recipe.NodeMetadata, outputTemplate recipe.OutputMap, stateMap *recipe.StateMap) error {
	// Create resolution context for the state machine
	resolvedInputs, err := parentContext.ResolveMap(metadata.Inputs)
	if err != nil {
		return fmt.Errorf("failed to resolve state machine inputs: %w", err)
	}

	resCtx, err := parentContext.NewChildContext(template.ScopeStateMachine, metadata, "", resolvedInputs)
	if err != nil {
		return fmt.Errorf("failed to create resolution context: %w", err)
	}

	// Initialize state tracking
	currentState := stateMap.Initial
	stateInvocationCount := make(map[string]int)

	// Execute state machine
	for !isTerminalState(currentState, stateMap.States) {
		// Get current state definition
		stateDef, exists := stateMap.States[currentState]
		if !exists {
			return fmt.Errorf("state '%s' not found", currentState)
		}

		err := runState(ctx, activityRegistry, resCtx, currentState, stateDef)

		if err != nil {
			// Handle retry if configured
			return fmt.Errorf("state '%s' execution failed: %w", currentState, err)
		}

		// Evaluate transitions using resolution context
		nextState, err := evaluateTransitionsWithContext(stateDef.Transitions, resCtx)
		if err != nil {
			return fmt.Errorf("failed to evaluate state transitions: %w", err)
		}
		stateInvocationCount[currentState]++
		if nextState == "" {
			// No transition matched, state machine completes
			break
		}

		currentState = nextState
	}

	// Return final outputs
	finalState := stateMap.States[currentState]
	_ = finalState

	resolvedOutputs, err := resCtx.ResolveMap(outputTemplate)
	if err != nil {
		return fmt.Errorf("failed to resolve state machine outputs: %w", err)
	}

	parentContext.AddExecution(resolvedOutputs)

	return nil
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
func evaluateTransitionsWithContext(transitions []recipe.Transition, resCtx *template.ResolutionContext) (string, error) {
	// Create a temporary context for transition evaluation
	evalCtx := &template.ResolutionContext{
		ScopeType:    resCtx.ScopeType,
		TemplateData: resCtx.TemplateData,
		CELEnv:       resCtx.CELEnv,
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

func runState(ctx workflow.Context, activityRegistry *workerops.ActivityRegistry, resCtx *template.ResolutionContext, stateName string, node recipe.State) error {
	stateResCtx, err := resCtx.NewChildContext(template.ScopeState, node.GetMetadata(), stateName, nil)
	if err != nil {
		return fmt.Errorf("failed to create state context: %w", err)
	}

	return executeNode(ctx, stateResCtx, activityRegistry, &node.Node, nil)
}
