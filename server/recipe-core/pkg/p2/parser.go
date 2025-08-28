package p2

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
	"gopkg.in/yaml.v3"
)

func Parse(data []byte) (Recipe, error) {
	var recipe Recipe
	err := yaml.Unmarshal(data, &recipe)
	return recipe, err
}

func opDataImpls(defs ...types.OpDef) func(*OpData, []byte) error {
	return func(impl *OpData, data []byte) error {
		if err := yaml.Unmarshal(data, &impl); err != nil {
			return err
		}

		// see if we can match the op name
		var def types.OpDef = nil
		for _, innerDef := range defs {
			if innerDef.GetName() == impl.Op {
				def = innerDef
			}
		}

		if def == nil {
			return fmt.Errorf("unknown op: %s", impl.Op)
		}

		data, err := yaml.Marshal(impl.Inputs)
		if err != nil {
			return err
		}

		concreteInputType := def.GetInputStruct()

		// Unmarshal into the concrete type and report error if exists.
		if err := yaml.Unmarshal(data, &concreteInputType); err != nil {
			return err
		}

		return nil
	}
}
