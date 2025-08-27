package p2

import "github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"

// StateMap represents a state machine configuration
type StateMap struct {
	Initial string           `yaml:"initial"` // Which state to start with
	States  map[string]State `yaml:",inline"` // Inline the states
}

// State represents a state in a state machine
// A state is a node plus transition information
type State struct {
	Node Node `yaml:",inline" refer:"true"`
	// State-specific fields
	Transitions []Transition `yaml:"transitions,omitempty"`
	Error       string       `yaml:"error,omitempty"` // For terminal error states
}

// Transition represents a state transition
type Transition struct {
	To   string      `yaml:"to"`
	When cel.CELExpr `yaml:"when,omitempty"` // CEL expression
}
