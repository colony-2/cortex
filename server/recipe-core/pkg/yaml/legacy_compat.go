package yaml

// Legacy type aliases for state machine compiler compatibility
// These map old types to new types since the structure is mostly the same

// Step represents a legacy workflow step (for activities that haven't been migrated yet)
type Step struct {
	ID       string                 `yaml:"id"`
	Name     string                 `yaml:"name"`
	Uses     string                 `yaml:"uses"`
	Config   map[string]interface{} `yaml:"config"`
	Inputs   map[string]interface{} `yaml:"inputs"`
	Outputs  map[string]string      `yaml:"outputs"`
	Parallel *ParallelSpec          `yaml:"parallel"`
}

// ParallelSpec defines legacy parallel execution
type ParallelSpec struct {
	ForEach string `yaml:"for_each"`
	As      string `yaml:"as"`
	Steps   []Step `yaml:"steps"`
}

// SharedActivity represents a reusable activity configuration (legacy)
type SharedActivity struct {
	Uses   string                 `yaml:"uses"`
	Config map[string]interface{} `yaml:"config"`
}

// StateMachineConfig represents the old state machine configuration
type StateMachineConfig struct {
	InitialState string                     `yaml:"initial_state" json:"initial_state"`
	States       map[string]StateDefinition `yaml:"states" json:"states"`
	Timeout      string                     `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// StateDefinition represents the old state definition
type StateDefinition struct {
	// Simple operation (maps to Op in new model)
	Uses   string                 `yaml:"uses,omitempty" json:"uses,omitempty"`
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	
	// Composition types (still supported)
	Sequential  []CompositionStep   `yaml:"sequential,omitempty" json:"sequential,omitempty"`
	Parallel    []CompositionStep   `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	Conditional []ConditionalBranch `yaml:"conditional,omitempty" json:"conditional,omitempty"`
	
	// State-specific fields
	Terminal    bool                   `yaml:"terminal,omitempty" json:"terminal,omitempty"`
	Error       string                 `yaml:"error,omitempty" json:"error,omitempty"`
	Inputs      map[string]interface{} `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Outputs     map[string]interface{} `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	Retry       *StateRetryPolicy      `yaml:"retry,omitempty" json:"retry,omitempty"`
	Transitions []TransitionSpec       `yaml:"transitions,omitempty" json:"transitions,omitempty"`
}

// CompositionStep represents a step in a composition
type CompositionStep struct {
	ID string `yaml:"id" json:"id"`
	
	// Simple activity
	Uses string `yaml:"uses,omitempty" json:"uses,omitempty"`
	
	// Nested compositions
	Sequential  []CompositionStep   `yaml:"sequential,omitempty" json:"sequential,omitempty"`
	Parallel    []CompositionStep   `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	Conditional []ConditionalBranch `yaml:"conditional,omitempty" json:"conditional,omitempty"`
	
	// Step configuration
	Inputs    map[string]interface{} `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	DependsOn []string               `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	When      string                 `yaml:"when,omitempty" json:"when,omitempty"`
	Retry     *StepRetryPolicy       `yaml:"retry,omitempty" json:"retry,omitempty"`
}

// ConditionalBranch represents a conditional branch
type ConditionalBranch struct {
	When    string `yaml:"when,omitempty" json:"when,omitempty"`
	Default bool   `yaml:"default,omitempty" json:"default,omitempty"`
	
	// Branch content
	Uses        string              `yaml:"uses,omitempty" json:"uses,omitempty"`
	Sequential  []CompositionStep   `yaml:"sequential,omitempty" json:"sequential,omitempty"`
	Parallel    []CompositionStep   `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	Conditional []ConditionalBranch `yaml:"conditional,omitempty" json:"conditional,omitempty"`
	
	Inputs map[string]interface{} `yaml:"inputs,omitempty" json:"inputs,omitempty"`
}

// TransitionSpec represents a state transition
type TransitionSpec struct {
	To   string `yaml:"to" json:"to"`
	When string `yaml:"when" json:"when"`
}

// StateRetryPolicy represents retry policy for states
type StateRetryPolicy struct {
	When               string  `yaml:"when,omitempty" json:"when,omitempty"`
	MaxAttempts        int     `yaml:"max_attempts" json:"max_attempts"`
	BackoffCoefficient float64 `yaml:"backoff_coefficient,omitempty" json:"backoff_coefficient,omitempty"`
	InitialInterval    string  `yaml:"initial_interval,omitempty" json:"initial_interval,omitempty"`
}

// StepRetryPolicy represents retry policy for steps
type StepRetryPolicy struct {
	When               string  `yaml:"when,omitempty" json:"when,omitempty"`
	MaxAttempts        int     `yaml:"max_attempts" json:"max_attempts"`
	BackoffCoefficient float64 `yaml:"backoff_coefficient,omitempty" json:"backoff_coefficient,omitempty"`
	InitialInterval    string  `yaml:"initial_interval,omitempty" json:"initial_interval,omitempty"`
}

// Convert new StateMap to old StateMachineConfig for compatibility
func StateMapToStateMachineConfig(sm *StateMap) StateMachineConfig {
	config := StateMachineConfig{
		InitialState: sm.Initial,
		States:       make(map[string]StateDefinition),
	}
	
	// Convert each State to StateDefinition
	for name, state := range sm.States {
		def := StateDefinition{
			Uses:    state.Op,
			Inputs:  state.Inputs,
			Outputs: state.Outputs,
			Error:   state.Error,
		}
		
		// Convert transitions
		if len(state.Transitions) > 0 {
			def.Transitions = make([]TransitionSpec, len(state.Transitions))
			for i, t := range state.Transitions {
				def.Transitions[i] = TransitionSpec{
					To:   t.To,
					When: t.When,
				}
			}
		}
		
		// Check if terminal (no transitions)
		def.Terminal = len(state.Transitions) == 0
		
		config.States[name] = def
	}
	
	return config
}