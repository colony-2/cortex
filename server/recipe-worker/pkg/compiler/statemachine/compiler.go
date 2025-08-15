package statemachine

import (
	"fmt"
	"time"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/google/cel-go/cel"
	"go.temporal.io/sdk/workflow"
)

// StateMachineCompiler handles state machine compilation and execution
type StateMachineCompiler struct {
	celEnv           *cel.Env
	activityExecutor ActivityExecutor
}

// ActivityExecutor interface for executing activities
type ActivityExecutor interface {
	ExecuteActivity(ctx workflow.Context, activityName string, inputs map[string]interface{}) (map[string]interface{}, error)
}

// NewStateMachineCompiler creates a new state machine compiler
func NewStateMachineCompiler(executor ActivityExecutor) (*StateMachineCompiler, error) {
	// Create CEL environment with standard types
	env, err := cel.NewEnv(
		cel.Variable("Outputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("State", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("States", cel.MapType(cel.StringType, cel.MapType(cel.StringType, cel.DynType))),
		cel.Variable("Inputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("Context", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("Steps", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &StateMachineCompiler{
		celEnv:           env,
		activityExecutor: executor,
	}, nil
}

// ExecuteStateMap runs the state machine with the new StateMap format
func (s *StateMachineCompiler) ExecuteStateMap(ctx workflow.Context, stateMap *yamlpkg.StateMap, inputs map[string]interface{}) (map[string]interface{}, error) {
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
	for !s.isTerminalState(stateCtx.CurrentState, stateMap.States) {
		// Check if workflow context is cancelled
		if err := workflow.Sleep(ctx, 0); err != nil {
			return nil, fmt.Errorf("state machine execution cancelled: %w", err)
		}
		
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
		outputs, err := s.executeState(ctx, &stateDef, stateCtx)
		if err != nil {
			// Handle retry if configured
			if stateDef.Retry != nil && s.shouldRetry(stateDef.Retry, err, stateCtx) {
				stateCtx.Attempts[stateCtx.CurrentState]++
				continue
			}
			return nil, fmt.Errorf("state '%s' execution failed: %w", stateCtx.CurrentState, err)
		}

		// Store state outputs
		stateCtx.StateOutputs[stateCtx.CurrentState] = outputs

		// Evaluate transitions
		nextState := s.evaluateTransitions(stateDef.Transitions, outputs, stateCtx)
		if nextState == "" {
			// No transition matched, state machine completes
			break
		}

		stateCtx.CurrentState = nextState
	}

	// Return final outputs
	finalState := stateMap.States[stateCtx.CurrentState]
	if len(finalState.Transitions) == 0 && finalState.Outputs != nil {
		return s.prepareOutputs(finalState.Outputs, stateCtx), nil
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

// executeState executes a single state using the new State type
func (s *StateMachineCompiler) executeState(ctx workflow.Context, state *yamlpkg.State, stateCtx *yamlpkg.StateContext) (map[string]interface{}, error) {
	// Terminal states return configured outputs or error
	if len(state.Transitions) == 0 {
		if state.Error != "" {
			return nil, fmt.Errorf("%s", state.Error)
		}
		return s.prepareOutputs(state.Outputs, stateCtx), nil
	}

	// Prepare inputs for this state
	preparedInputs := s.prepareInputs(state.Inputs, stateCtx)

	// Execute based on state type
	if state.Op != "" {
		// Simple activity execution
		return s.activityExecutor.ExecuteActivity(ctx, state.Op, preparedInputs)
	} else if len(state.Sequence) > 0 {
		// Sequential execution
		return s.executeSequenceNodes(ctx, state.Sequence, stateCtx)
	} else if len(state.Parallel) > 0 {
		// Parallel execution
		return s.executeParallelNodes(ctx, state.Parallel, stateCtx)
	} else if state.States != nil {
		// Nested state machine
		return s.ExecuteStateMap(ctx, state.States, preparedInputs)
	} else if state.Shared != "" {
		// Shared node reference
		sharedActivityName := "shared/" + state.Shared
		return s.activityExecutor.ExecuteActivity(ctx, sharedActivityName, preparedInputs)
	}

	return nil, fmt.Errorf("state must specify one of: op, sequence, parallel, states, or shared")
}

// executeSequenceNodes executes nodes in sequence using the new Node type
func (s *StateMachineCompiler) executeSequenceNodes(ctx workflow.Context, nodes []yamlpkg.Node, stateCtx *yamlpkg.StateContext) (map[string]interface{}, error) {
	allOutputs := make(map[string]interface{})
	
	for i, node := range nodes {
		nodeOutputs, err := s.executeNode(ctx, &node, stateCtx.Inputs)
		if err != nil {
			return nil, fmt.Errorf("sequence node %d failed: %w", i, err)
		}
		
		// Store outputs with node ID if specified
		if node.ID != "" {
			allOutputs[node.ID] = nodeOutputs
			stateCtx.StepOutputs[node.ID] = nodeOutputs
		}
		
		// Pass outputs as inputs to next node
		for k, v := range nodeOutputs {
			stateCtx.Inputs[k] = v
		}
	}
	
	return allOutputs, nil
}

// executeParallelNodes executes nodes in parallel using the new Node type
func (s *StateMachineCompiler) executeParallelNodes(ctx workflow.Context, nodes []yamlpkg.Node, stateCtx *yamlpkg.StateContext) (map[string]interface{}, error) {
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
			outputs, err := s.executeNode(ctx, &node, stateCtx.Inputs)
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
			stateCtx.StepOutputs[nodes[i].ID] = result.outputs
		}
	}
	
	return allOutputs, nil
}

// executeNode executes a single node using the new Node type
func (s *StateMachineCompiler) executeNode(ctx workflow.Context, node *yamlpkg.Node, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Check conditional execution
	if node.When != "" {
		shouldExecute, err := s.evaluateCEL(node.When, inputs, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate node condition: %w", err)
		}
		if !shouldExecute {
			return make(map[string]interface{}), nil // Skip this node
		}
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
		return s.activityExecutor.ExecuteActivity(ctx, node.Op, mergedInputs)
	} else if len(node.Sequence) > 0 {
		// Create a temporary state context for nested execution
		nestedCtx := &yamlpkg.StateContext{
			Inputs:      mergedInputs,
			StepOutputs: make(map[string]interface{}),
		}
		return s.executeSequenceNodes(ctx, node.Sequence, nestedCtx)
	} else if len(node.Parallel) > 0 {
		// Create a temporary state context for nested execution
		nestedCtx := &yamlpkg.StateContext{
			Inputs:      mergedInputs,
			StepOutputs: make(map[string]interface{}),
		}
		return s.executeParallelNodes(ctx, node.Parallel, nestedCtx)
	} else if node.States != nil {
		return s.ExecuteStateMap(ctx, node.States, mergedInputs)
	} else if node.Shared != "" {
		// Execute shared node reference
		sharedActivityName := "shared/" + node.Shared
		return s.activityExecutor.ExecuteActivity(ctx, sharedActivityName, mergedInputs)
	}
	
	return nil, fmt.Errorf("node must define one of: op, sequence, parallel, states, or shared")
}

// Helper functions

// isTerminalState checks if a state is terminal using the new State type
func (s *StateMachineCompiler) isTerminalState(stateName string, states map[string]yamlpkg.State) bool {
	state, exists := states[stateName]
	if !exists {
		return true // Non-existent state is terminal
	}
	return len(state.Transitions) == 0
}

// shouldRetry handles retry logic for the new RetryPolicy type
func (s *StateMachineCompiler) shouldRetry(policy *yamlpkg.RetryPolicy, err error, stateCtx *yamlpkg.StateContext) bool {
	attempts := stateCtx.Attempts[stateCtx.CurrentState]
	if attempts >= policy.MaxAttempts {
		return false
	}
	
	// For now, just check max attempts. CEL evaluation can be added later
	return true
}

// evaluateTransitions evaluates transitions using the new Transition type
func (s *StateMachineCompiler) evaluateTransitions(transitions []yamlpkg.Transition, outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) string {
	for _, transition := range transitions {
		if transition.When == "" {
			// Unconditional transition
			return transition.To
		}

		shouldTransition, err := s.evaluateCEL(transition.When, outputs, stateCtx)
		if err == nil && shouldTransition {
			return transition.To
		}
	}
	return ""
}


// prepareInputs prepares inputs by resolving templates
func (s *StateMachineCompiler) prepareInputs(inputs map[string]interface{}, stateCtx *yamlpkg.StateContext) map[string]interface{} {
	if inputs == nil {
		return make(map[string]interface{})
	}

	resolver := NewTemplateResolver()
	resolved, err := resolver.ResolveInputs(inputs, stateCtx)
	if err != nil {
		// Log error and return original inputs
		return inputs
	}
	return resolved
}

// prepareOutputs prepares outputs by resolving templates
func (s *StateMachineCompiler) prepareOutputs(outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) map[string]interface{} {
	if outputs == nil {
		return make(map[string]interface{})
	}

	resolver := NewTemplateResolver()
	resolved, err := resolver.ResolveOutputs(outputs, stateCtx)
	if err != nil {
		// Log error and return original outputs
		return outputs
	}
	return resolved
}

// calculateBackoff calculates backoff duration for retry policy
func (s *StateMachineCompiler) calculateBackoff(policy *yamlpkg.RetryPolicy, attempt int) (time.Duration, error) {
	// Parse initial interval from string
	backoff, err := time.ParseDuration(policy.InitialInterval)
	if err != nil {
		return time.Second, fmt.Errorf("invalid initial interval: %w", err)
	}
	
	// Apply backoff coefficient for each attempt
	coefficient := policy.BackoffCoefficient
	if coefficient == 0 {
		coefficient = 2.0 // Default exponential backoff
	}
	
	for i := 1; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * coefficient)
	}
	
	return backoff, nil
}