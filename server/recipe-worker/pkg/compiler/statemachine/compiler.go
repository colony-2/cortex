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

// Execute runs the state machine with provided configuration and inputs
func (s *StateMachineCompiler) Execute(ctx workflow.Context, config yamlpkg.StateMachineConfig, inputs map[string]interface{}) (map[string]interface{}, error) {
	// Initialize state context
	stateCtx := &yamlpkg.StateContext{
		CurrentState: config.InitialState,
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
		var cancel workflow.CancelFunc
		ctx, cancel = workflow.WithCancel(ctx)
		defer cancel()
		workflow.Go(ctx, func(ctx workflow.Context) {
			_ = workflow.Sleep(ctx, timeout)
			cancel()
		})
	}

	// Execute state machine
	for !s.isTerminal(stateCtx.CurrentState, config.States) {
		// Check if workflow context is cancelled
		if err := workflow.Sleep(ctx, 0); err != nil {
			return nil, fmt.Errorf("state machine execution cancelled: %w", err)
		}
		
		// Get current state definition
		stateDef, exists := config.States[stateCtx.CurrentState]
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
		outputs, err := s.executeState(ctx, stateDef, stateCtx)
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
	finalState := config.States[stateCtx.CurrentState]
	if finalState.Terminal && finalState.Outputs != nil {
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

// executeState executes a single state with composition support
func (s *StateMachineCompiler) executeState(ctx workflow.Context, state yamlpkg.StateDefinition, stateCtx *yamlpkg.StateContext) (map[string]interface{}, error) {
	// Terminal states return configured outputs
	if state.Terminal {
		if state.Error != "" {
			return nil, fmt.Errorf("%s", state.Error)
		}
		return s.prepareOutputs(state.Outputs, stateCtx), nil
	}

	// Create a root scoped context for this state
	rootScope := NewScopedContext(nil, stateCtx, "state:"+stateCtx.CurrentState)

	// Prepare inputs for this state
	preparedInputs := s.prepareInputsWithScope(state.Inputs, rootScope)

	// Determine execution type and delegate
	if state.Uses != "" {
		// Simple activity execution
		return s.activityExecutor.ExecuteActivity(ctx, state.Uses, preparedInputs)
	} else if state.Sequential != nil {
		// Sequential composition
		return s.executeSequentialScoped(ctx, state.Sequential, rootScope)
	} else if state.Parallel != nil {
		// Parallel composition
		return s.executeParallelScoped(ctx, state.Parallel, rootScope)
	} else if state.Conditional != nil {
		// Conditional composition
		result, err := s.executeConditionalScoped(ctx, state.Conditional, rootScope)
		if err != nil {
			return nil, err
		}
		// Convert interface{} to map[string]interface{}
		if resultMap, ok := result.(map[string]interface{}); ok {
			return resultMap, nil
		}
		// Wrap non-map result
		return map[string]interface{}{"result": result}, nil
	}

	return nil, fmt.Errorf("state must specify 'uses', 'sequential', 'parallel', or 'conditional'")
}

// executeStepScoped executes a single step within a composition with proper scoping
func (s *StateMachineCompiler) executeStepScoped(ctx workflow.Context, step yamlpkg.CompositionStep, scope *ScopedContext) (interface{}, error) {
	// Check conditional execution
	if step.When != "" {
		shouldExecute, err := s.evaluateCELScoped(step.When, nil, scope)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate step condition: %w", err)
		}
		if !shouldExecute {
			return nil, nil // Skip this step
		}
	}

	// Prepare step inputs with current scope
	preparedInputs := s.prepareInputsWithScope(step.Inputs, scope)

	// Execute based on step type
	var result interface{}
	var err error

	if step.Uses != "" {
		// Simple activity
		activityResult, actErr := s.activityExecutor.ExecuteActivity(ctx, step.Uses, preparedInputs)
		result = activityResult
		err = actErr
	} else if step.Sequential != nil {
		// Create nested scope for sequential composition
		nestedScope := NewScopedContext(scope, scope.StateContext, "seq:"+step.ID)
		result, err = s.executeSequentialScoped(ctx, step.Sequential, nestedScope)
	} else if step.Parallel != nil {
		// Create nested scope for parallel composition
		nestedScope := NewScopedContext(scope, scope.StateContext, "par:"+step.ID)
		result, err = s.executeParallelScoped(ctx, step.Parallel, nestedScope)
	} else if step.Conditional != nil {
		// Create nested scope for conditional composition
		nestedScope := NewScopedContext(scope, scope.StateContext, "cond:"+step.ID)
		result, err = s.executeConditionalScoped(ctx, step.Conditional, nestedScope)
	} else {
		return nil, fmt.Errorf("step '%s' must specify execution type", step.ID)
	}

	// Handle retry if needed
	if err != nil && step.Retry != nil {
		attempts := 1
		for attempts < step.Retry.MaxAttempts {
			// Wait with backoff
			backoffDuration := s.calculateBackoff(step.Retry, attempts)
			_ = workflow.Sleep(ctx, backoffDuration)

			// Retry execution
			if step.Uses != "" {
				activityResult, retryErr := s.activityExecutor.ExecuteActivity(ctx, step.Uses, preparedInputs)
				result = activityResult
				err = retryErr
			} else {
				// Re-execute the composition
				result, err = s.executeStepScoped(ctx, step, scope)
			}

			if err == nil {
				break
			}
			attempts++
		}
	}

	return result, err
}

// executeStep executes a single step within a composition
func (s *StateMachineCompiler) executeStep(ctx workflow.Context, step yamlpkg.CompositionStep, outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) (interface{}, error) {
	// Check conditional execution
	if step.When != "" {
		shouldExecute, err := s.evaluateCEL(step.When, outputs, stateCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate step condition: %w", err)
		}
		if !shouldExecute {
			return nil, nil // Skip this step
		}
	}

	// Prepare step inputs
	preparedInputs := s.prepareInputs(step.Inputs, stateCtx)

	// Execute based on step type
	var result interface{}
	var err error

	if step.Uses != "" {
		// Simple activity
		activityResult, actErr := s.activityExecutor.ExecuteActivity(ctx, step.Uses, preparedInputs)
		result = activityResult
		err = actErr
	} else if step.Sequential != nil {
		// Nested sequential
		result, err = s.executeSequential(ctx, step.Sequential, stateCtx)
	} else if step.Parallel != nil {
		// Nested parallel
		result, err = s.executeParallel(ctx, step.Parallel, stateCtx)
	} else if step.Conditional != nil {
		// Nested conditional
		result, err = s.executeConditional(ctx, step.Conditional, stateCtx)
	} else {
		return nil, fmt.Errorf("step '%s' must specify execution type", step.ID)
	}

	// Handle retry if needed
	if err != nil && step.Retry != nil {
		attempts := 1
		for attempts < step.Retry.MaxAttempts {
			// Wait with backoff
			backoffDuration := s.calculateBackoff(step.Retry, attempts)
			_ = workflow.Sleep(ctx, backoffDuration)

			// Retry execution
			if step.Uses != "" {
				activityResult, retryErr := s.activityExecutor.ExecuteActivity(ctx, step.Uses, preparedInputs)
				result = activityResult
				err = retryErr
			} else {
				// Re-execute the composition
				result, err = s.executeStep(ctx, step, outputs, stateCtx)
			}

			if err == nil {
				break
			}
			attempts++
		}
	}

	return result, err
}

// executeSequentialScoped executes steps in sequence with proper scoping
func (s *StateMachineCompiler) executeSequentialScoped(ctx workflow.Context, steps []yamlpkg.CompositionStep, scope *ScopedContext) (map[string]interface{}, error) {
	for _, step := range steps {
		result, err := s.executeStepScoped(ctx, step, scope)
		if err != nil {
			return nil, fmt.Errorf("step '%s' failed: %w", step.ID, err)
		}

		if step.ID != "" && result != nil {
			// Store in current scope only
			scope.SetStepOutput(step.ID, result)
		}
	}

	// Return merged outputs from this scope
	return scope.MergeOutputs(), nil
}

// executeSequential executes steps in sequence (legacy method for backward compatibility)
func (s *StateMachineCompiler) executeSequential(ctx workflow.Context, steps []yamlpkg.CompositionStep, stateCtx *yamlpkg.StateContext) (map[string]interface{}, error) {
	outputs := make(map[string]interface{})

	for _, step := range steps {
		result, err := s.executeStep(ctx, step, outputs, stateCtx)
		if err != nil {
			return nil, fmt.Errorf("step '%s' failed: %w", step.ID, err)
		}

		if step.ID != "" && result != nil {
			outputs[step.ID] = result
			// Also update state context step outputs
			stateCtx.StepOutputs[step.ID] = result
		}
	}

	return outputs, nil
}

// executeParallelScoped executes steps in parallel with proper scoping
func (s *StateMachineCompiler) executeParallelScoped(ctx workflow.Context, steps []yamlpkg.CompositionStep, scope *ScopedContext) (map[string]interface{}, error) {
	// Group steps by dependencies
	groups := s.groupByDependencies(steps)

	for _, group := range groups {
		// Use channels for parallel execution results
		type stepResult struct {
			id     string
			result interface{}
			err    error
		}
		
		resultsChan := workflow.NewChannel(ctx)
		pendingCount := len(group)
		
		for _, step := range group {
			step := step // capture loop variable

			// Check dependencies are met in current scope
			if !s.dependenciesMetScoped(step.DependsOn, scope) {
				return nil, fmt.Errorf("dependencies not met for step '%s'", step.ID)
			}

			workflow.Go(ctx, func(ctx workflow.Context) {
				result, err := s.executeStepScoped(ctx, step, scope)
				resultsChan.Send(ctx, stepResult{
					id:     step.ID,
					result: result,
					err:    err,
				})
			})
		}

		// Collect results from all parallel executions
		for i := 0; i < pendingCount; i++ {
			var res stepResult
			resultsChan.Receive(ctx, &res)
			
			if res.err != nil {
				return nil, fmt.Errorf("parallel step '%s' failed: %w", res.id, res.err)
			}
			if res.id != "" && res.result != nil {
				scope.SetStepOutput(res.id, res.result)
			}
		}
	}

	// Return merged outputs from this scope
	return scope.MergeOutputs(), nil
}

// executeParallel executes steps in parallel with dependency support
func (s *StateMachineCompiler) executeParallel(ctx workflow.Context, steps []yamlpkg.CompositionStep, stateCtx *yamlpkg.StateContext) (map[string]interface{}, error) {
	// Group steps by dependencies
	groups := s.groupByDependencies(steps)
	outputs := make(map[string]interface{})

	for _, group := range groups {
		// Use channels for parallel execution results
		type stepResult struct {
			id     string
			result interface{}
			err    error
		}
		
		resultsChan := workflow.NewChannel(ctx)
		pendingCount := len(group)

		for _, step := range group {
			step := step // capture loop variable

			// Check dependencies are met
			if !s.dependenciesMet(step.DependsOn, outputs) {
				return nil, fmt.Errorf("dependencies not met for step '%s'", step.ID)
			}

			workflow.Go(ctx, func(ctx workflow.Context) {
				result, err := s.executeStep(ctx, step, outputs, stateCtx)
				resultsChan.Send(ctx, stepResult{
					id:     step.ID,
					result: result,
					err:    err,
				})
			})
		}

		// Collect results from all parallel executions
		for i := 0; i < pendingCount; i++ {
			var res stepResult
			resultsChan.Receive(ctx, &res)
			
			if res.err != nil {
				return nil, fmt.Errorf("parallel step '%s' failed: %w", res.id, res.err)
			}
			if res.id != "" && res.result != nil {
				outputs[res.id] = res.result
				stateCtx.StepOutputs[res.id] = res.result
			}
		}
	}

	return outputs, nil
}

// executeConditionalScoped evaluates conditions and executes the matching branch with proper scoping
func (s *StateMachineCompiler) executeConditionalScoped(ctx workflow.Context, branches []yamlpkg.ConditionalBranch, scope *ScopedContext) (interface{}, error) {
	for _, branch := range branches {
		// Check condition (default branch has no condition)
		shouldExecute := branch.Default
		
		if !branch.Default && branch.When != "" {
			// Evaluate the condition with current scope
			evaluated, err := s.evaluateCELScoped(branch.When, nil, scope)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate condition: %w", err)
			}
			shouldExecute = evaluated
		}
		
		if !shouldExecute {
			continue
		}

		// Execute the selected branch with current scope
		preparedInputs := s.prepareInputsWithScope(branch.Inputs, scope)

		if branch.Uses != "" {
			return s.activityExecutor.ExecuteActivity(ctx, branch.Uses, preparedInputs)
		} else if branch.Sequential != nil {
			// Create nested scope for sequential
			nestedScope := NewScopedContext(scope, scope.StateContext, "cond-seq")
			return s.executeSequentialScoped(ctx, branch.Sequential, nestedScope)
		} else if branch.Parallel != nil {
			// Create nested scope for parallel
			nestedScope := NewScopedContext(scope, scope.StateContext, "cond-par")
			return s.executeParallelScoped(ctx, branch.Parallel, nestedScope)
		} else if branch.Conditional != nil {
			// Create nested scope for conditional
			nestedScope := NewScopedContext(scope, scope.StateContext, "cond-cond")
			return s.executeConditionalScoped(ctx, branch.Conditional, nestedScope)
		}

		return nil, fmt.Errorf("conditional branch must specify execution type")
	}

	return nil, fmt.Errorf("no conditional branch matched")
}

// executeConditional evaluates conditions and executes the matching branch
func (s *StateMachineCompiler) executeConditional(ctx workflow.Context, branches []yamlpkg.ConditionalBranch, stateCtx *yamlpkg.StateContext) (interface{}, error) {
	for _, branch := range branches {
		// Check condition (default branch has no condition)
		shouldExecute := branch.Default
		
		if !branch.Default && branch.When != "" {
			// Evaluate the condition
			evaluated, err := s.evaluateCEL(branch.When, nil, stateCtx)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate condition: %w", err)
			}
			shouldExecute = evaluated
		}
		
		if !shouldExecute {
			continue
		}

		// Execute the selected branch
		preparedInputs := s.prepareInputs(branch.Inputs, stateCtx)

		if branch.Uses != "" {
			return s.activityExecutor.ExecuteActivity(ctx, branch.Uses, preparedInputs)
		} else if branch.Sequential != nil {
			return s.executeSequential(ctx, branch.Sequential, stateCtx)
		} else if branch.Parallel != nil {
			return s.executeParallel(ctx, branch.Parallel, stateCtx)
		} else if branch.Conditional != nil {
			return s.executeConditional(ctx, branch.Conditional, stateCtx)
		}

		return nil, fmt.Errorf("conditional branch must specify execution type")
	}

	return nil, fmt.Errorf("no conditional branch matched")
}

// Helper functions

func (s *StateMachineCompiler) isTerminal(stateName string, states map[string]yamlpkg.StateDefinition) bool {
	state, exists := states[stateName]
	if !exists {
		return true // Non-existent state is terminal
	}
	return state.Terminal
}

func (s *StateMachineCompiler) shouldRetry(policy *yamlpkg.StateRetryPolicy, err error, stateCtx *yamlpkg.StateContext) bool {
	attempts := stateCtx.Attempts[stateCtx.CurrentState]
	if attempts >= policy.MaxAttempts {
		return false
	}

	if policy.When != "" {
		shouldRetry, evalErr := s.evaluateCEL(policy.When, nil, stateCtx)
		if evalErr != nil || !shouldRetry {
			return false
		}
	}

	return true
}

func (s *StateMachineCompiler) calculateBackoff(policy *yamlpkg.StepRetryPolicy, attempt int) time.Duration {
	// Parse initial interval from string
	backoff, err := time.ParseDuration(policy.InitialInterval)
	if err != nil {
		// Default to 1 second if parsing fails
		backoff = time.Second
	}
	
	for i := 1; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * policy.BackoffCoefficient)
	}
	return backoff
}

func (s *StateMachineCompiler) groupByDependencies(steps []yamlpkg.CompositionStep) [][]yamlpkg.CompositionStep {
	// Simple grouping - steps with no dependencies go first
	// This can be enhanced with proper topological sorting
	var noDeps []yamlpkg.CompositionStep
	var withDeps []yamlpkg.CompositionStep

	for _, step := range steps {
		if len(step.DependsOn) == 0 {
			noDeps = append(noDeps, step)
		} else {
			withDeps = append(withDeps, step)
		}
	}

	groups := [][]yamlpkg.CompositionStep{}
	if len(noDeps) > 0 {
		groups = append(groups, noDeps)
	}
	if len(withDeps) > 0 {
		groups = append(groups, withDeps)
	}

	return groups
}

func (s *StateMachineCompiler) dependenciesMet(deps []string, outputs map[string]interface{}) bool {
	for _, dep := range deps {
		if _, exists := outputs[dep]; !exists {
			return false
		}
	}
	return true
}

// dependenciesMetScoped checks if dependencies are met within the current scope
func (s *StateMachineCompiler) dependenciesMetScoped(deps []string, scope *ScopedContext) bool {
	for _, dep := range deps {
		if _, exists := scope.GetStepOutput(dep); !exists {
			return false
		}
	}
	return true
}

func (s *StateMachineCompiler) evaluateTransitions(transitions []yamlpkg.TransitionSpec, outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) string {
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

// prepareInputsWithScope prepares inputs with scoped template resolution
func (s *StateMachineCompiler) prepareInputsWithScope(inputs map[string]interface{}, scope *ScopedContext) map[string]interface{} {
	if inputs == nil {
		return make(map[string]interface{})
	}

	resolver := NewScopedTemplateResolver()
	resolved, err := resolver.ResolveInputsScoped(inputs, scope)
	if err != nil {
		// Log error and return original inputs
		return inputs
	}
	return resolved
}