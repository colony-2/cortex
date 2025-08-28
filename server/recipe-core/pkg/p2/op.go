package p2

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/ops"
	yaml "gopkg.in/yaml.v3"
)

type OpData struct {
	Op     string                 `yaml:"op"`
	Inputs map[string]interface{} `yaml:"inputs"`
}

func (n *OpData) UnmarshalYAML(node *yaml.Node) error {
	// Create a type alias to avoid recursion
	type Alias OpData
	aux := (*Alias)(n)

	if err := node.Decode(aux); err != nil {
		return err
	}

	op, exists := ops.Get(n.Op)
	if !exists {
		return fmt.Errorf("unknown op: %s", n.Op)
	}

	data, err := yaml.Marshal(n.Inputs)
	if err != nil {
		return err
	}

	concreteInputType := op.GetInputStruct()

	if err := yaml.Unmarshal(data, &concreteInputType); err != nil {
		return err
	}
	return nil
}
