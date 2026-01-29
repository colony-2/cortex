package recipe

import "github.com/colony-2/colony2/server/recipe-core/pkg/cel"

// StateMap represents a state machine configuration
type StateMap struct {
	Initial string           `yaml:"initial" validate:"required"` // Which state to start with
	States  map[string]State `yaml:"states"`                      // Inline the states
}

// State represents a state in a state machine
// A state is a node plus transition information
type State struct {
	Node                `yaml:",inline"`
	SingleStateMetadata `yaml:",inline"`
}

type SingleStateMetadata struct {
	Transitions []Transition `json:"transitions,omitempty"`
}

// Transition represents a state transition
type Transition struct {
	To   string      `yaml:"to"`
	When cel.CELExpr `yaml:"when,omitempty"` // CEL expression
}

type StateMachineData struct {
	States  *StateMap              `yaml:"state,omitempty"`
	Outputs map[string]interface{} `yaml:"outputs,omitempty"`
}
