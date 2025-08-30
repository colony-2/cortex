package recipe

import "github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"

// StateMap represents a state machine configuration
type StateMap struct {
	Initial string           `yaml:"initial"` // Which state to start with
	States  map[string]State `yaml:",inline"` // Inline the states
}

// State represents a state in a state machine
// A state is a node plus transition information
type State struct {
	Node                `yaml:",inline"`
	SingleStateMetadata `yaml:",inline"`
}

type SingleStateMetadata struct {
	Error       *string      `json:"error,omitempty"`
	Transitions []Transition `json:"transitions,omitempty"`
}

// Transition represents a state transition
type Transition struct {
	To   string      `yaml:"to"`
	When cel.CELExpr `yaml:"when,omitempty"` // CEL expression
}

type StateData struct {
	States  *StateMap `yaml:"states,omitempty"`
	Inputs  InputMap  `yaml:"inputs"`
	Outputs OutputMap `yaml:"outputs,omitempty"`
}
