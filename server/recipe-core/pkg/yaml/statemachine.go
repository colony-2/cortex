package yaml

import (
	"time"
)

// StateMachineConfig defines the configuration for state machine execution
type StateMachineConfig struct {
	InitialState string                     `yaml:"initial_state" json:"initial_state"`
	States       map[string]StateDefinition `yaml:"states" json:"states"`
	Timeout      string                     `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// StateDefinition extends existing structure to support compositions
type StateDefinition struct {
	// Single activity (backward compatible)
	Uses   string                 `yaml:"uses,omitempty" json:"uses,omitempty"`
	Config map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`

	// NEW: Composition types (mutually exclusive with Uses)
	Sequential  []CompositionStep      `yaml:"sequential,omitempty" json:"sequential,omitempty"`
	Parallel    []CompositionStep      `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	Conditional []ConditionalBranch    `yaml:"conditional,omitempty" json:"conditional,omitempty"`

	// Existing fields (unchanged)
	Terminal    bool                   `yaml:"terminal,omitempty" json:"terminal,omitempty"`
	Error       string                 `yaml:"error,omitempty" json:"error,omitempty"`
	Inputs      map[string]interface{} `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Outputs     map[string]interface{} `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	Retry       *StateRetryPolicy      `yaml:"retry,omitempty" json:"retry,omitempty"`
	Transitions []TransitionSpec       `yaml:"transitions,omitempty" json:"transitions,omitempty"`
}

// TransitionSpec remains unchanged
type TransitionSpec struct {
	To   string `yaml:"to" json:"to"`
	When string `yaml:"when" json:"when"` // CEL expression
}

// StateRetryPolicy remains unchanged
type StateRetryPolicy struct {
	When               string  `yaml:"when,omitempty" json:"when,omitempty"`
	MaxAttempts        int     `yaml:"max_attempts" json:"max_attempts"`
	BackoffCoefficient float64 `yaml:"backoff_coefficient,omitempty" json:"backoff_coefficient,omitempty"`
	InitialInterval    string  `yaml:"initial_interval,omitempty" json:"initial_interval,omitempty"`
}

// CompositionStep can itself be a composition or a simple activity
type CompositionStep struct {
	ID string `yaml:"id" json:"id"`

	// Simple activity
	Uses string `yaml:"uses,omitempty" json:"uses,omitempty"`

	// OR nested compositions (mutually exclusive)
	Sequential  []CompositionStep   `yaml:"sequential,omitempty" json:"sequential,omitempty"`
	Parallel    []CompositionStep   `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	Conditional []ConditionalBranch `yaml:"conditional,omitempty" json:"conditional,omitempty"`

	// Step configuration
	Inputs    map[string]interface{} `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	DependsOn []string               `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	When      string                 `yaml:"when,omitempty" json:"when,omitempty"` // CEL condition for step execution
	Retry     *StepRetryPolicy       `yaml:"retry,omitempty" json:"retry,omitempty"`
}

// ConditionalBranch represents a branch in conditional logic
type ConditionalBranch struct {
	When    string `yaml:"when,omitempty" json:"when,omitempty"`       // CEL condition (omit for default)
	Default bool   `yaml:"default,omitempty" json:"default,omitempty"` // Mark as default branch

	// Branch can be activity or composition
	Uses        string                 `yaml:"uses,omitempty" json:"uses,omitempty"`
	Sequential  []CompositionStep      `yaml:"sequential,omitempty" json:"sequential,omitempty"`
	Parallel    []CompositionStep      `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	Conditional []ConditionalBranch    `yaml:"conditional,omitempty" json:"conditional,omitempty"`

	Inputs map[string]interface{} `yaml:"inputs,omitempty" json:"inputs,omitempty"`
}

// StepRetryPolicy defines retry behavior for steps
type StepRetryPolicy struct {
	When               string        `yaml:"when,omitempty" json:"when,omitempty"`
	MaxAttempts        int           `yaml:"max_attempts" json:"max_attempts"`
	BackoffCoefficient float64       `yaml:"backoff_coefficient,omitempty" json:"backoff_coefficient,omitempty"`
	InitialInterval    time.Duration `yaml:"initial_interval,omitempty" json:"initial_interval,omitempty"`
}

// StateContext maintains the runtime context for state machine execution
type StateContext struct {
	CurrentState  string                            `json:"current_state"`
	Inputs        map[string]interface{}            `json:"inputs"`
	StateOutputs  map[string]map[string]interface{} `json:"state_outputs"`
	Attempts      map[string]int                    `json:"attempts"`
	StateInfo     map[string]*StateInfo             `json:"state_info"`
	RecipeContext *RecipeContext                    `json:"recipe_context,omitempty"`
	StepOutputs   map[string]interface{}            `json:"step_outputs"` // NEW: Track step outputs within current state
}

// StateInfo tracks metadata for each state
type StateInfo struct {
	Name      string    `json:"name"`
	Attempts  int       `json:"attempts"`
	EnteredAt time.Time `json:"entered_at"`
}

// RecipeContext contains system-provided context variables
type RecipeContext struct {
	Recipe      RecipeInfo      `json:"recipe"`
	Environment EnvironmentInfo `json:"environment"`
	Execution   ExecutionInfo   `json:"execution"`
}

// RecipeInfo contains information about the current recipe
type RecipeInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	ExecutionID string `json:"execution_id"`
}

// EnvironmentInfo contains environment context
type EnvironmentInfo struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

// ExecutionInfo contains execution context
type ExecutionInfo struct {
	StartedAt time.Time     `json:"started_at"`
	Timeout   time.Duration `json:"timeout"`
}

// CELVariables represents the variables available in CEL expressions
type CELVariables struct {
	Outputs map[string]interface{}            `json:"Outputs"`
	State   *StateInfo                        `json:"State"`
	States  map[string]map[string]interface{} `json:"States"`
	Inputs  map[string]interface{}            `json:"Inputs"`
	Context *RecipeContext                    `json:"Context"`
	Steps   map[string]interface{}            `json:"Steps"` // NEW: Access to step outputs within current state
}