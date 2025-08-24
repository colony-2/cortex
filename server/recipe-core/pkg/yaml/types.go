package yaml

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
)

// InputDef defines the schema for an input parameter
type InputDef struct {
	Type        string      `yaml:"type,omitempty"`        // Type of the input (string, number, boolean, etc.)
	Description string      `yaml:"description,omitempty"` // Description of the input
	Required    bool        `yaml:"required,omitempty"`    // Whether the input is required
	Default     interface{} `yaml:"default,omitempty"`     // Default value if not provided
}

type Node struct {
	ID   string `yaml:"id,omitempty"`
	Desc string `yaml:"desc,omitempty"`

	// Root node - embedded directly (one of these four)
	Op       string    `yaml:"op,omitempty"`       // Operation node
	Sequence []Node    `yaml:"sequence,omitempty"` // Sequence node
	Parallel []Node    `yaml:"parallel,omitempty"` // Parallel node
	States   *StateMap `yaml:"states,omitempty"`   // State machine node
	Shared   string    `yaml:"shared,omitempty"`   // Reference to shared node

	// Node properties that apply at root
	Inputs  map[string]interface{} `yaml:"inputs,omitempty"`
	Outputs map[string]interface{} `yaml:"outputs,omitempty"` // Only for composite types
	Timeout types.Duration         `yaml:"timeout,omitempty"`
	Retry   *types.RetryPolicy     `yaml:"retry,omitempty"`
	When    cel.CELExpr            `yaml:"when,omitempty"` // Conditional execution
}

// RecipeDefinition represents the complete unified recipe YAML structure
// The recipe itself IS the root node with embedded node properties
type RecipeDefinition struct {
	Node    `yaml:",inline"`
	Version string `yaml:"version"`

	// Shared node definitions
	Shared      map[string]Node     `yaml:"shared,omitempty"`       // Shared node definitions
	InputSchema map[string]InputDef `yaml:"input_schema,omitempty"` // Optional schema for inputs
}

// StateMap represents a state machine configuration
type StateMap struct {
	Initial string           `yaml:"initial"` // Which state to start with
	States  map[string]State `yaml:",inline"` // Inline the states
}

// State represents a state in a state machine
// A state is a node plus transition information
type State struct {
	Node `yaml:",inline"`
	// State-specific fields
	Transitions []Transition `yaml:"transitions,omitempty"`
	Error       string       `yaml:"error,omitempty"` // For terminal error states
}

// Transition represents a state transition
type Transition struct {
	To   string      `yaml:"to"`
	When cel.CELExpr `yaml:"when,omitempty"` // CEL expression
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxAttempts        int     `yaml:"max_attempts"`
	InitialInterval    string  `yaml:"initial_interval"`
	BackoffCoefficient float64 `yaml:"backoff_coefficient,omitempty"`
	MaxInterval        string  `yaml:"max_interval,omitempty"`
}
