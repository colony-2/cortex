package recipe

import (
	"fmt"

	yamlv3 "gopkg.in/yaml.v3"
)

type RecipeImpl interface {
	isRecipe()
}
type Recipe struct {
	RecipeImpl
}

func (r Recipe) GetMetdata() RecipeMetadata {
	switch t := r.RecipeImpl.(type) {
	case *RecipeState:
		return t.RecipeMetadata
	case *RecipeSequence:
		return t.RecipeMetadata
	case *RecipeOp:
		return t.RecipeMetadata
	default:
		panic("invalid recipe type")
	}
}

func (n *Recipe) MarshalYAML() (interface{}, error) {
	return n.RecipeImpl, nil
}

func (n *Recipe) UnmarshalYAML(node *yamlv3.Node) error {
	var raw map[string]interface{}
	if err := node.Decode(&raw); err != nil {
		return err
	}

	var impl RecipeImpl
	switch {
	case raw["state"] != nil:
		impl = &RecipeState{}
	case raw["sequence"] != nil:
		impl = &RecipeSequence{}
	case raw["op"] != nil:
		impl = &RecipeOp{}
	default:
		return fmt.Errorf("intermediate node must either be a op, sequence, state, or shared reference")
	}

	// Second pass: decode into concrete type
	if err := node.Decode(impl); err != nil {
		return err
	}

	n.RecipeImpl = impl
	return nil
}

type RecipeMetadata struct {
	Version      string `yaml:"version"`
	NodeMetadata `yaml:",inline"`
	Defs         map[string]Node        `yaml:"defs,omitempty"`         // Shared node definitions
	InputSchema  map[string]InputSchema `yaml:"input_schema,omitempty"` // Optional schema for inputs
}

type RecipeSequence struct {
	RecipeMetadata `yaml:",inline" refer:"true"`
	SequenceData   `yaml:",inline" refer:"true"`
}

func (r RecipeSequence) isRecipe() {}

type RecipeState struct {
	RecipeMetadata `yaml:",inline" refer:"true"`
	StateData      `yaml:",inline" refer:"true"`
}

func (r RecipeState) isRecipe() {}

type RecipeOp struct {
	RecipeMetadata `yaml:",inline" refer:"true"`
	OpData         `yaml:",inline" refer:"true"`
}

func (r RecipeOp) isRecipe() {}

// InputSchema defines the schema for an input parameter
type InputSchema struct {
	Type        string      `yaml:"type,omitempty"`                                                // Type of the input (string, number, boolean, etc.)
	Description string      `yaml:"description,omitempty"`                                         // Description of the input
	Required    bool        `yaml:"required,omitempty"`                                            // Whether the input is required
	Default     interface{} `yaml:"default_value,omitempty" jsonschema:"oneof_type=string;number"` // Default value if not provided
}
