package statemachine

// Internal legacy types for state machine compiler
// These are used internally for backward compatibility with existing state machine logic

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