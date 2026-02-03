package recipe

import (
	"github.com/colony-2/colony2/server/recipe-core/pkg/cel"
	yamlv3 "gopkg.in/yaml.v3"
)

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
	Transitions []Transition `yaml:"transitions,omitempty" json:"transitions,omitempty"`
}

// MarshalYAML customizes encoding so state-only fields (like transitions) are preserved
// alongside the inline node fields.
func (s State) MarshalYAML() (interface{}, error) {
	// Use the node's MarshalYAML behavior (delegates to NodeImpl) and then merge in
	// state-only keys.
	var out map[string]interface{}
	b, err := yamlv3.Marshal(s.Node)
	if err != nil {
		return nil, err
	}
	if err := yamlv3.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if len(s.Transitions) > 0 {
		out["transitions"] = s.Transitions
	}
	return out, nil
}

// UnmarshalYAML customizes decoding so state-only fields (like transitions) don't get
// swallowed by the inline Node's UnmarshalYAML implementation.
func (s *State) UnmarshalYAML(node *yamlv3.Node) error {
	// Decode transitions (ignore unknown keys here; strictness is enforced at the recipe level).
	type metaOnly struct {
		Transitions []Transition `yaml:"transitions,omitempty"`
	}
	var meta metaOnly
	_ = node.Decode(&meta)

	// Decode the node definition using a filtered mapping node that omits state-only keys.
	filtered := &yamlv3.Node{
		Kind:    yamlv3.MappingNode,
		Tag:     "!!map",
		Content: make([]*yamlv3.Node, 0, len(node.Content)),
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind == yamlv3.ScalarNode && k.Value == "transitions" {
			continue
		}
		filtered.Content = append(filtered.Content, k, v)
	}

	var n Node
	if err := filtered.Decode(&n); err != nil {
		return err
	}

	s.Node = n
	s.SingleStateMetadata = SingleStateMetadata{Transitions: meta.Transitions}
	return nil
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
