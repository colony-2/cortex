package statemachine

import (
	"time"
)

// StateMachineConfig defines the configuration for state machine activities
type StateMachineConfig struct {
	InitialState string                     `json:"initial_state"`
	States       map[string]StateDefinition `json:"states"`
	Timeout      string                     `json:"timeout,omitempty"`
}

// StateDefinition defines a single state in the state machine
type StateDefinition struct {
	Uses        string                 `json:"uses,omitempty"`        // The activity to use (single activity mode)
	Config      map[string]interface{} `json:"config,omitempty"`      // Activity-specific configuration
	Workflow    *WorkflowDefinition    `json:"workflow,omitempty"`    // Workflow composition mode
	Terminal    bool                   `json:"terminal,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Inputs      map[string]interface{} `json:"inputs,omitempty"`
	Outputs     map[string]interface{} `json:"outputs,omitempty"`
	Retry       *StateRetryPolicy      `json:"retry,omitempty"`
	Transitions []TransitionSpec       `json:"transitions,omitempty"`
}

// TransitionSpec defines state transitions with CEL conditions
type TransitionSpec struct {
	To   string `json:"to"`
	When string `json:"when"` // CEL expression
}

// StateRetryPolicy with CEL conditions
type StateRetryPolicy struct {
	When               string  `json:"when,omitempty"`
	MaxAttempts        int     `json:"max_attempts"`
	BackoffCoefficient float64 `json:"backoff_coefficient,omitempty"`
	InitialInterval    string  `json:"initial_interval,omitempty"`
}

// StateContext maintains the runtime context for state machine execution
type StateContext struct {
	CurrentState string                            `json:"current_state"`
	Inputs       map[string]interface{}            `json:"inputs"`
	StateOutputs map[string]map[string]interface{} `json:"state_outputs"`
	Attempts     map[string]int                    `json:"attempts"`
	StateInfo    map[string]*StateInfo             `json:"state_info"`
	RecipeContext *RecipeContext                   `json:"recipe_context,omitempty"`
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

// WorkflowDefinition defines a workflow within a state
type WorkflowDefinition struct {
	Type    WorkflowType   `json:"type"`              // sequential, parallel
	Steps   []WorkflowStep `json:"steps"`
	Timeout string         `json:"timeout,omitempty"`
}

// WorkflowType defines the execution type of a workflow
type WorkflowType string

const (
	WorkflowTypeSequential WorkflowType = "sequential"
	WorkflowTypeParallel   WorkflowType = "parallel"
)

// WorkflowStep defines a step within a workflow
type WorkflowStep struct {
	ID        string                 `json:"id"`
	Uses      string                 `json:"uses"`                   // Activity to use
	Config    map[string]interface{} `json:"config,omitempty"`       // Activity config
	Inputs    map[string]interface{} `json:"inputs,omitempty"`       // Template inputs
	DependsOn []string               `json:"depends_on,omitempty"`   // Step dependencies
	When      string                 `json:"when,omitempty"`         // CEL condition
	Retry     *StepRetryPolicy       `json:"retry,omitempty"`
}

// StepRetryPolicy defines retry behavior for workflow steps
type StepRetryPolicy struct {
	MaxAttempts        int     `json:"max_attempts"`
	InitialInterval    string  `json:"initial_interval,omitempty"`
	BackoffCoefficient float64 `json:"backoff_coefficient,omitempty"`
}

// CELVariables represents the variables available in CEL expressions
type CELVariables struct {
	Outputs map[string]interface{}            `json:"Outputs"`
	State   *StateInfo                        `json:"State"`
	States  map[string]map[string]interface{} `json:"States"`
	Inputs  map[string]interface{}            `json:"Inputs"`
	Context *RecipeContext                    `json:"Context"`
	Steps   map[string]interface{}            `json:"Steps"` // For workflow steps
}