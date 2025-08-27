package p2

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/swaggest/jsonschema-go"
)

type baseNode struct{}

func (baseNode) JSONSchemaOneOf() []interface{} {
	// Helper builds an exposer from sample values.
	return []interface{}{NodeOp{}, NodeState{}, NodeShared{}, NodeSequence{}}
}

type Node interface {
	isNode()
}

type NodeShared struct {
	Shared string `yaml:"shared"`
}

func (n NodeShared) isNode() {}

type NodeSequence struct {
	NodeMetadata `yaml:",inline" refer:"true"`
	Sequence     []Node `yaml:"sequence,omitempty"` // Sequence node

	// Node properties that apply at root
	Inputs  InputMap  `yaml:"inputs,omitempty"`
	Outputs OutputMap `yaml:"outputs,omitempty"`
	_       struct{}  `unevaluatedProperties:"false"`
}

func (n *NodeSequence) isNode() {}

type NodeState struct {
	NodeMetadata `yaml:",inline" refer:"true"`
	States       *StateMap `yaml:"states,omitempty" refer:"true"` // State machine node
	Inputs       InputMap  `yaml:"inputs,omitempty"`
	Outputs      OutputMap `yaml:"outputs,omitempty"`
	_            struct{}  `unevaluatedProperties:"false"`
}

func (n *NodeState) isNode() {}

type NodeOp struct {
	NodeMetadata `yaml:",inline" refer:"true"`
	OpImpl       `yaml:",inline" refer:"true"`
}

func (c NodeOp) JSONSchema() (jsonschema.Schema, error) {
	var schema jsonschema.Schema
	schema.WithAllOf(
		(&jsonschema.Schema{}).WithRef(refSchema).ToSchemaOrBool(),
		(&jsonschema.Schema{}).WithRef("#/definitions/NodeMetadata").ToSchemaOrBool(),
	)
	return schema, nil
}

type OpImpl interface {
	GetName() string
	GetInputAsMap() InputMap
}

func (n NodeOp) isNode() {}

type NodeMetadata struct {
	ID      string            `yaml:"id,omitempty"`
	Desc    string            `yaml:"desc,omitempty"`
	Timeout Duration          `yaml:"timeout,omitempty"`
	Retry   *yaml.RetryPolicy `yaml:"retry,omitempty"`
	When    cel.CELExpr       `yaml:"when,omitempty"` // Conditional execution
}
