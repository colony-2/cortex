package statemachine

import (
	"context"
	"fmt"
	"time"

	"github.com/divisive-ai/vibethis/server/activity/pkg/types"
	"github.com/google/cel-go/cel"
)

// StateMachineActivity implements RegisterableActivity interface for state machines
type StateMachineActivity struct {
	executor ExecutorImplementation
	celEnv   *cel.Env
}

// ExecutorImplementation provides methods to execute activities and recipes
type ExecutorImplementation interface {
	ExecuteActivity(ctx context.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error)
	ExecuteRecipe(ctx context.Context, recipeName string, inputs map[string]interface{}) (map[string]interface{}, error)
}

// NewStateMachineActivity creates a new state machine activity
func NewStateMachineActivity(executor ExecutorImplementation) (*StateMachineActivity, error) {
	// Create CEL environment with standard types
	env, err := cel.NewEnv(
		cel.Variable("Outputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("State", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("States", cel.MapType(cel.StringType, cel.MapType(cel.StringType, cel.DynType))),
		cel.Variable("Inputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("Context", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &StateMachineActivity{
		executor: executor,
		celEnv:   env,
	}, nil
}

// GetMetadata returns activity metadata for registration
func (s *StateMachineActivity) GetMetadata() types.ActivityMetadata {
	return types.ActivityMetadata{
		Type:           "state_machine",
		Name:           "State Machine Activity",
		Description:    "Executes workflows with conditional state transitions using CEL expressions",
		Version:        "1.0.0",
		DefaultTimeout: 5 * time.Minute,
		RetryPolicy: &types.RetryPolicy{
			MaximumAttempts:    3,
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
		},
	}
}

// Execute runs the state machine with provided configuration and inputs
func (s *StateMachineActivity) Execute(ctx context.Context, config StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Initialize state context
	stateCtx := &StateContext{
		CurrentState: config.InitialState,
		Inputs:       inputs,
		StateOutputs: make(map[string]map[string]interface{}),
		Attempts:     make(map[string]int),
		StateInfo:    make(map[string]*StateInfo),
	}

	// Extract recipe context from inputs if available
	if ctxVal, ok := inputs["context"]; ok {
		if recipeCtx, ok := ctxVal.(*RecipeContext); ok {
			stateCtx.RecipeContext = recipeCtx
		}
	}

	// Parse timeout if specified
	var timeout time.Duration
	if config.Timeout != "" {
		parsed, err := time.ParseDuration(config.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		timeout = parsed
	}

	// Apply timeout to context if specified
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Execute state machine
	for !isTerminal(stateCtx.CurrentState, config.States) {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("state machine execution timed out or cancelled")
		default:
		}

		state, exists := config.States[stateCtx.CurrentState]
		if !exists {
			return nil, fmt.Errorf("state '%s' not found in configuration", stateCtx.CurrentState)
		}

		// Track state entry
		stateCtx.StateInfo[stateCtx.CurrentState] = &StateInfo{
			Name:      stateCtx.CurrentState,
			Attempts:  stateCtx.Attempts[stateCtx.CurrentState] + 1,
			EnteredAt: time.Now(),
		}
		stateCtx.Attempts[stateCtx.CurrentState]++

		// Execute state activity or recipe
		outputs, err := s.executeState(ctx, state, stateCtx)
		if err != nil {
			// Check retry policy
			if shouldRetry(state.Retry, stateCtx, err) {
				continue // Retry the same state
			}
			return nil, fmt.Errorf("state '%s' execution failed: %w", stateCtx.CurrentState, err)
		}

		// Store outputs
		stateCtx.StateOutputs[stateCtx.CurrentState] = outputs

		// Evaluate transitions
		nextState, err := s.evaluateTransitions(state.Transitions, outputs, stateCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate transitions for state '%s': %w", stateCtx.CurrentState, err)
		}

		// No valid transition found
		if nextState == "" {
			return nil, fmt.Errorf("no valid transition from state '%s'", stateCtx.CurrentState)
		}

		stateCtx.CurrentState = nextState
	}

	// Handle terminal state
	terminalState := config.States[stateCtx.CurrentState]
	if terminalState.Error != "" {
		return nil, fmt.Errorf("%s", terminalState.Error)
	}

	// Return terminal state outputs or configured outputs
	if terminalState.Outputs != nil {
		return terminalState.Outputs, nil
	}
	if outputs, ok := stateCtx.StateOutputs[stateCtx.CurrentState]; ok {
		return outputs, nil
	}

	// Return all state outputs if no specific output configured
	result := make(map[string]interface{})
	for stateName, outputs := range stateCtx.StateOutputs {
		result[stateName] = outputs
	}
	return result, nil
}

// executeState executes a single state's activity or recipe
func (s *StateMachineActivity) executeState(ctx context.Context, state StateDefinition, stateCtx *StateContext) (map[string]interface{}, error) {
	// Terminal states don't execute anything
	if state.Terminal {
		return state.Outputs, nil
	}

	// Prepare inputs for the activity/recipe
	inputs := state.Inputs
	if inputs == nil {
		inputs = stateCtx.Inputs
	}

	// Execute activity or recipe
	if state.Activity != "" {
		return s.executor.ExecuteActivity(ctx, state.Activity, inputs)
	} else if state.Recipe != "" {
		return s.executor.ExecuteRecipe(ctx, state.Recipe, inputs)
	}

	return nil, fmt.Errorf("state must specify either 'activity' or 'recipe'")
}

// evaluateTransitions evaluates CEL expressions to determine next state
func (s *StateMachineActivity) evaluateTransitions(transitions []TransitionSpec, outputs map[string]interface{}, stateCtx *StateContext) (string, error) {
	if len(transitions) == 0 {
		return "", nil
	}

	// Convert StateInfo to map for CEL
	stateInfo := stateCtx.StateInfo[stateCtx.CurrentState]
	stateMap := map[string]interface{}{
		"name":      stateInfo.Name,
		"attempts":  stateInfo.Attempts,
		"Attempts":  stateInfo.Attempts, // Support both cases
		"enteredAt": stateInfo.EnteredAt,
	}

	// Convert RecipeContext to map if available
	var contextMap map[string]interface{}
	if stateCtx.RecipeContext != nil {
		contextMap = map[string]interface{}{
			"Recipe": map[string]interface{}{
				"Name":        stateCtx.RecipeContext.Recipe.Name,
				"Version":     stateCtx.RecipeContext.Recipe.Version,
				"ExecutionID": stateCtx.RecipeContext.Recipe.ExecutionID,
			},
			"Environment": map[string]interface{}{
				"Name":   stateCtx.RecipeContext.Environment.Name,
				"Region": stateCtx.RecipeContext.Environment.Region,
			},
			"Execution": map[string]interface{}{
				"StartedAt": stateCtx.RecipeContext.Execution.StartedAt,
				"Timeout":   stateCtx.RecipeContext.Execution.Timeout,
			},
		}
	}

	// Convert to map for CEL evaluation
	varMap := map[string]interface{}{
		"Outputs": outputs,
		"State":   stateMap,
		"States":  stateCtx.StateOutputs,
		"Inputs":  stateCtx.Inputs,
		"Context": contextMap,
	}

	// Evaluate each transition condition
	for _, transition := range transitions {
		if transition.When == "" {
			// Default transition with no condition
			return transition.To, nil
		}

		// Parse CEL expression
		ast, issues := s.celEnv.Parse(transition.When)
		if issues != nil && issues.Err() != nil {
			return "", fmt.Errorf("failed to parse CEL expression '%s': %w", transition.When, issues.Err())
		}

		// Check expression
		checked, issues := s.celEnv.Check(ast)
		if issues != nil && issues.Err() != nil {
			return "", fmt.Errorf("failed to check CEL expression '%s': %w", transition.When, issues.Err())
		}

		// Compile program
		prg, err := s.celEnv.Program(checked)
		if err != nil {
			return "", fmt.Errorf("failed to compile CEL expression '%s': %w", transition.When, err)
		}

		// Evaluate expression
		out, _, err := prg.Eval(varMap)
		if err != nil {
			// Log evaluation error but continue to next transition
			continue
		}

		// Check if condition is true
		if result, ok := out.Value().(bool); ok && result {
			return transition.To, nil
		}
	}

	return "", nil
}

// isTerminal checks if a state is terminal
func isTerminal(stateName string, states map[string]StateDefinition) bool {
	state, exists := states[stateName]
	return exists && state.Terminal
}

// shouldRetry determines if a state should be retried based on retry policy
func shouldRetry(policy *StateRetryPolicy, stateCtx *StateContext, err error) bool {
	if policy == nil {
		return false
	}

	currentState := stateCtx.CurrentState
	attempts := stateCtx.Attempts[currentState]

	if attempts >= policy.MaxAttempts {
		return false
	}

	// TODO: Evaluate CEL condition if specified in policy.When
	// For now, always retry if under max attempts
	return true
}