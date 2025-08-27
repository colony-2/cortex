package p2

import "github.com/goccy/go-yaml"

func recipe(recipe *Recipe, data []byte) error {
	return nil
}

func node(node *Node, data []byte) error {
	return nil
}

type NodeImplFunc func(*OpImpl, []byte) error

func nodeimpl(defs ...OpDef) NodeImplFunc {
	return func(impl *OpImpl, data []byte) error {
		return nil
	}
}

func Parse(data []byte) (InputMap, error) {
	var input InputMap
	err := yaml.UnmarshalWithOptions(data, &input,
		yaml.CustomUnmarshaler[Recipe](recipe),
		yaml.CustomUnmarshaler[Node](node),
		yaml.CustomUnmarshaler[Node](node),
	)
	return input, err
}
