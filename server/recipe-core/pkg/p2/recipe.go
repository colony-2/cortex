package p2

import "github.com/swaggest/jsonschema-go"

type baseRecipe struct{}

func (baseRecipe) JSONSchemaOneOf() []interface{} {
	// Helper builds an exposer from sample values.
	return []interface{}{RecipeState{}, RecipeSequence{}, RecipeOp{}}
}
func (baseRecipe) InlineJSONSchema() {}

type Recipe interface {
	isRecipe()
}

type RecipeMetadata struct {
	Version      string `yaml:"version"`
	NodeMetadata `yaml:",inline" refer:"true"`
	Defs         map[string]Node        `yaml:"defs,omitempty"`         // Shared node definitions
	InputSchema  map[string]InputSchema `yaml:"input_schema,omitempty"` // Optional schema for inputs
}

type RecipeSequence struct {
	RecipeMetadata `yaml:",inline" refer:"true"`
	Sequence       []Node   `yaml:"sequence,omitempty"`
	_              struct{} `additionalProperties:"false"`
}

func (r RecipeSequence) isRecipe() {}

type RecipeState struct {
	RecipeMetadata `yaml:",inline" refer:"true"`
	States         *StateMap `yaml:"states,omitempty" refer:"true"`
	_              struct{}  `additionalProperties:"false"`
}

func (r RecipeState) isRecipe() {}

type RecipeOp struct {
	RecipeMetadata `yaml:",inline" refer:"true"`
	OpImpl         `yaml:",inline" refer:"true"`
	_              struct{} `additionalProperties:"false"`
}

func (c RecipeOp) JSONSchema() (jsonschema.Schema, error) {
	var schema jsonschema.Schema
	schema.WithAllOf(
		(&jsonschema.Schema{}).WithRef(refSchema).ToSchemaOrBool(),
		(&jsonschema.Schema{}).WithRef("#/definitions/RecipeMetadata").ToSchemaOrBool(),
	)
	return schema, nil
}

func (r RecipeOp) isRecipe() {}

// InputSchema defines the schema for an input parameter
type InputSchema struct {
	Type        string      `yaml:"type,omitempty"`        // Type of the input (string, number, boolean, etc.)
	Description string      `yaml:"description,omitempty"` // Description of the input
	Required    bool        `yaml:"required,omitempty"`    // Whether the input is required
	Default     interface{} `yaml:"default,omitempty"`     // Default value if not provided
}
