package validate

import (
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/p2"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
	"github.com/goccy/go-yaml"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func Validate(recipe string, ops ...types.OpDef) error {
	// Compile the schema
	schemaStr, err := p2.GenerateSchemaString(ops...)
	if err != nil {
		return err
	}

	schema, err := jsonschema.CompileString("schema.json", schemaStr)
	if err != nil {
		return fmt.Errorf("Schema compilation error: %w", err)
	}

	var data interface{}
	if err := yaml.Unmarshal([]byte(recipe), &data); err != nil {
		return err
	}

	return schema.Validate(data)
}
