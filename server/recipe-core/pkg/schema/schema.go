package main

import (
	"encoding/json"
	"fmt"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/invopop/jsonschema"
)

func main() {
	fmt.Println(GenerateSchema())
}

func GenerateSchema() string {
	//processedTypes := make(map[reflect.Type]bool)
	// Configure reflector to use yaml tags
	r := jsonschema.Reflector{
		// Use yaml tag instead of json tag for field names
		FieldNameTag: "yaml",
		// Optional: allow additional properties
		AllowAdditionalProperties: false,
		// Optional: use jsonschema tags for requirements
		RequiredFromJSONSchemaTags: true,
		ExpandedStruct:             true,
	}

	schema := r.Reflect(yaml.RecipeDefinition{})
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic(err)
	}

	return string(schemaJSON)
}
