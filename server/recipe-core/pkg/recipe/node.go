package recipe

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/invopop/jsonschema"
	yamlv3 "gopkg.in/yaml.v3"
)

type Node struct {
	NodeImpl
}

func (n Node) GetMetadata() NodeMetadata {
	return n.GetMetadata()
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

	var nodeOp *NodeOp
	var impl NodeImpl
	switch {
	case raw["state"] != nil:
		impl = &NodeState{}
	case raw["sequence"] != nil:
		impl = &NodeSequence{}
	case raw["op"] != nil:
		nodeOp = &NodeOp{}
		impl = nodeOp
	case raw["shared"] != nil:
		impl = &NodeShared{}
	default:
		return fmt.Errorf("intermediate node must either be a op, sequence, state, or shared reference")
	}

	// Second pass: decode into concrete type
	if err := node.Decode(impl); err != nil {
		return err
	}

	if nodeOp != nil {
		if err := checkOpInputs(nodeOp.Op, nodeOp.Inputs); err != nil {
			return err
		}
	}
	n.NodeImpl = impl
	return nil
}

func checkOpInputs(opName string, inputs map[string]interface{}) error {
	op, exists := ops.Get(opName)
	if !exists {
		return fmt.Errorf("unknown op: %s", opName)
	}
	concreteInputType := op.GetInputStruct()

	data, err := yamlv3.Marshal(inputs)
	if err != nil {
		return err
	}
	if err := yamlv3.Unmarshal(data, &concreteInputType); err != nil {
		return err
	}
	return nil
}

type NodeImpl interface {
	isNode()
	GetMetadata() NodeMetadata
}

type NodeList []Node

type NodeShared struct {
	Shared string `yaml:"shared"`
}

func (n *NodeShared) GetMetadata() NodeMetadata {
	panic("not supported for shared. should have been replaced already")
}

func (n *NodeShared) isNode() {}

type NodeSequence struct {
	NodeMetadata `yaml:",inline"`
	SequenceData `yaml:",inline"`
}

func (n *NodeSequence) GetMetadata() NodeMetadata {
	return n.NodeMetadata
}

func (n *NodeSequence) isNode() {}

type NodeState struct {
	NodeMetadata `yaml:",inline"`
	StateData    `yaml:",inline"`
}

func (n *NodeState) GetMetadata() NodeMetadata {
	return n.NodeMetadata
}

func (n *NodeState) isNode() {}

type NodeOp struct {
	NodeMetadata `yaml:",inline"`
	OpData       `yaml:",inline"`
}

func (n *NodeOp) GetMetadata() NodeMetadata {
	return n.NodeMetadata
}

func (NodeOp) JSONSchema() *jsonschema.Schema {
	opList := ops.List()
	items := make([]interface{}, 0, len(opList))
	for _, op := range opList {
		items = append(items, op.GetInputStruct())
	}
	return oneOfSchema("op", items...)
}

func (n *NodeOp) isNode() {}

type NodeMetadata struct {
	ID      string       `yaml:"id,omitempty"`
	Desc    string       `yaml:"desc,omitempty"`
	Timeout Duration     `yaml:"timeout,omitempty"`
	Retry   *RetryPolicy `yaml:"retry,omitempty"`
	Inputs  InputMap     `yaml:"inputs,omitempty"`
	When    cel.CELExpr  `yaml:"when,omitempty"` // Conditional execution
}
