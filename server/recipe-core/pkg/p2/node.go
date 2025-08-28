package p2

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/invopop/jsonschema"
	yamlv3 "gopkg.in/yaml.v3"
)

type Node struct {
	NodeImpl
}

func (Node) JSONSchema() *jsonschema.Schema {
	s := &jsonschema.Schema{}
	s.Ref = "#/$defs/Node"
	return s
}

func (n *Node) UnmarshalYAML(node *yamlv3.Node) error {
	var raw map[string]interface{}
	if err := node.Decode(&raw); err != nil {
		return err
	}

	var impl NodeImpl
	switch {
	case raw["state"] != nil:
		impl = &NodeState{}
	case raw["sequence"] != nil:
		impl = &NodeSequence{}
	case raw["op"] != nil:
		impl = &NodeOp{}
	case raw["shared"] != nil:
		impl = &NodeShared{}
	default:
		return fmt.Errorf("intermediate node must either be a op, sequence, state, or shared reference")
	}

	// Second pass: decode into concrete type
	if err := node.Decode(impl); err != nil {
		return err
	}

	n.NodeImpl = impl
	return nil
}

type NodeImpl interface {
	isNode()
}

type NodeList []Node

type NodeShared struct {
	Shared string `yaml:"shared"`
}

func (n NodeShared) isNode() {}

type NodeSequence struct {
	NodeMetadata `yaml:",inline"`
	SequenceData `yaml:",inline"`
}

func (n NodeSequence) isNode() {}

type NodeState struct {
	NodeMetadata `yaml:",inline"`
	StateData    `yaml:",inline"`
	//_            struct{} `additionalProperties:"false"`
}

func (n NodeState) isNode() {}

type NodeOp struct {
	NodeMetadata `yaml:",inline"`
	OpData       `yaml:",inline"`
}

func (NodeOp) JSONSchema() *jsonschema.Schema {
	ops := ops.List()
	items := make([]interface{}, 0, len(ops))
	for _, op := range ops {
		items = append(items, op.GetInputStruct())
	}
	return oneOfSchema("op", items...)
}

func (n NodeOp) isNode() {}

type NodeMetadata struct {
	ID      string            `yaml:"id,omitempty"`
	Desc    string            `yaml:"desc,omitempty"`
	Timeout Duration          `yaml:"timeout,omitempty"`
	Retry   *yaml.RetryPolicy `yaml:"retry,omitempty"`
	When    cel.CELExpr       `yaml:"when,omitempty"` // Conditional execution
}
