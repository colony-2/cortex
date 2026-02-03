package compiler

import (
	"fmt"

	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-template/pkg/template"
)

// ExecuteStateMachine runs the state machine with the new StateMap format
func (d DefaultRecipeExecutor) ExecuteStateMachine(ctx workflow.Context, parentContext *template.ResolutionContext, metadata recipe.NodeMetadata, outputTemplate map[string]interface{}, stateMap *recipe.StateMap) error {
	// Create resolution context for the state machine
	resolvedInputs, err := parentContext.ResolveMap(metadata.Inputs)
	if err != nil {
		return fmt.Errorf("failed to resolve state machine inputs: %w", err)
	}

	resCtx, err := parentContext.NewChildContext(template.ScopeStateMachine, metadata, "", resolvedInputs)
	if err != nil {
		return fmt.Errorf("failed to create resolution context: %w", err)
	}
	if err := seedStateMachinePlaceholders(resCtx, stateMap); err != nil {
		return err
	}

	// Initialize state tracking
	currentState := stateMap.Initial
	if currentState == "" {
		return fmt.Errorf("state machine initial state is required")
	}
	if _, ok := stateMap.States[currentState]; !ok {
		return fmt.Errorf("state '%s' not found", currentState)
	}
	stateInvocationCount := make(map[string]int)

	if resCtx.Options.Mode == template.ModeValidate && resCtx.Options.ValidationMode == string(ValidateAll) {
		stateNames := sortedStateNames(stateMap.States)
		lastStateName := ""
		lastStateDef := recipe.State{}
		for _, stateName := range stateNames {
			stateDef := stateMap.States[stateName]
			if err := d.runState(ctx, resCtx, stateName, stateDef); err != nil {
				return fmt.Errorf("state '%s' execution failed: %w", stateName, err)
			}
			lastStateName = stateName
			lastStateDef = stateDef
			if _, err := evaluateTransitionsWithContext(stateDef.Transitions, resCtx); err != nil {
				return fmt.Errorf("failed to evaluate state transitions: %w", err)
			}
		}

		resolvedOutputs, err := resCtx.ResolveMap(outputTemplate)
		if err != nil {
			return fmt.Errorf("failed to resolve state machine outputs: %w", err)
		}

		parentContext.AddExecutionWithArtifacts(resolvedOutputs, stateArtifacts(resCtx, lastStateName, lastStateDef))
		return nil
	}

	// Execute state machine. Always run the current state at least once, even if it is terminal.
	for {
		// Get current state definition
		stateDef, exists := stateMap.States[currentState]
		if !exists {
			return fmt.Errorf("state '%s' not found", currentState)
		}

		if err := d.runState(ctx, resCtx, currentState, stateDef); err != nil {
			// Handle retry if configured
			return fmt.Errorf("state '%s' execution failed: %w", currentState, err)
		}

		// Terminal states end the machine after they run.
		if isTerminalState(currentState, stateMap.States) {
			break
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
	finalState, ok := stateMap.States[currentState]
	if currentState != "" && !ok {
		return fmt.Errorf("state '%s' not found", currentState)
	}

	resolvedOutputs, err := resCtx.ResolveMap(outputTemplate)
	if err != nil {
		return fmt.Errorf("failed to resolve state machine outputs: %w", err)
	}

	parentContext.AddExecutionWithArtifacts(resolvedOutputs, stateArtifacts(resCtx, currentState, finalState))

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

func (d DefaultRecipeExecutor) runState(ctx workflow.Context, resCtx *template.ResolutionContext, stateName string, node recipe.State) error {
	stateResCtx, err := resCtx.NewChildContext(template.ScopeState, node.GetMetadata(), stateName, nil)
	if err != nil {
		return fmt.Errorf("failed to create state context: %w", err)
	}

	return d.ExecuteNode(ctx, stateResCtx, &node.Node)
}
