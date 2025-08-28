package validate

import (
	"fmt"
	"strings"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/p2"
	"github.com/goccy/go-yaml"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func Validate(recipe string) error {
	// Compile the schema
	schemaStr, err := p2.GenerateSchemaString()
	if err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.AssertVocabs()
	compiler.AssertContent()
	s, err := jsonschema.UnmarshalJSON(strings.NewReader(schemaStr))
	if err != nil {
		return fmt.Errorf("failure parsing schema's JSON: %w", err)
	}

	err = compiler.AddResource("schema5.json", s)
	if err != nil {
		return fmt.Errorf("schema add error: %w", err)
	}

	schema, err := compiler.Compile("schema5.json")
	if err != nil {
		return fmt.Errorf("schema compilation error: %w", err)
	}

	var data interface{}
	if err := yaml.Unmarshal([]byte(recipe), &data); err != nil {
		return err
	}

	return schema.Validate(data)
}
