package p2

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"

	//	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
	"github.com/swaggest/jsonschema-go"
)

//
//func getInterceptSchema(oneOfSchemas []jsonschema.SchemaOrBool) jsonschema.InterceptSchemaFunc {
//
//	return func(params jsonschema.InterceptSchemaParams) (stop bool, err error) {
//		fmt.Printf("Schema: %s (%s) \n", params.Value.Type().Name(), params.Value.Type())
//		if !params.Processed || params.Value.Type() != reflect.TypeFor[*OpImpl]() {
//			return false, nil
//		}
//		fmt.Printf("Intercepting schema for %s\n", params.Value.Type())
//		params.Schema.WithOneOf(oneOfSchemas...).WithRequired()
//		params.Schema.WithExtraPropertiesItem("discriminator", map[string]interface{}{
//			"propertyName": "op",
//		})
//		return false, nil
//	}
//}
//
//func getInterceptProp(oneOfSchemas []jsonschema.SchemaOrBool) jsonschema.InterceptPropFunc {
//	return func(params jsonschema.InterceptPropParams) error {
//		if !params.Processed || params.Field.Type != reflect.TypeFor[*OpImpl]() {
//			return nil
//		}
//
//		params.PropertySchema.Type = nil
//		params.PropertySchema.OneOf = oneOfSchemas
//		return nil
//	}
//}

const refName = "OpImpl"
const refSchema = "#/definitions/" + refName

func getDef(ops []types.OpDef) (jsonschema.SchemaOrBool, error) {
	oneOfSchemas := make([]jsonschema.SchemaOrBool, 0, len(ops))

	reflector := &jsonschema.Reflector{}
	for _, op := range ops {
		// Create schema for the input type
		input := op.GetInputStruct()
		inputSchema, err := reflector.Reflect(input)
		if err != nil {
			return jsonschema.SchemaOrBool{}, fmt.Errorf("failed to reflect input type for %s: %w", op.GetName(), err)
		}

		// Create const schema for "op" field
		opFieldSchema := jsonschema.Schema{}
		opFieldSchema.AddType(jsonschema.String)
		opFieldSchema.WithConst(op.GetName())

		// Create properties map
		properties := map[string]jsonschema.SchemaOrBool{
			"op":     *(&jsonschema.SchemaOrBool{}).WithTypeObject(opFieldSchema),
			"inputs": *(&jsonschema.SchemaOrBool{}).WithTypeObject(inputSchema),
		}

		// Create the schema for this operation
		opSchema := jsonschema.Schema{}
		opSchema.AddType(jsonschema.Object)
		opSchema.WithProperties(properties)
		opSchema.WithAdditionalProperties(addpropfalse)
		opSchema.WithTitle(op.GetName())
		opSchema.WithRequired("op", "inputs")
		oneOfSchemas = append(oneOfSchemas, *(&jsonschema.SchemaOrBool{}).WithTypeObject(opSchema))
	}

	oneof := jsonschema.Schema{}
	oneof.WithOneOf(oneOfSchemas...)
	oneof.WithRequired()
	oneof.WithExtraPropertiesItem("discriminator", map[string]interface{}{
		"propertyName": "op",
	})
	return oneof.ToSchemaOrBool(), nil
}

var falseval = false

var addpropfalse = jsonschema.SchemaOrBool{
	TypeBoolean: &falseval,
}

func GenerateSchema(ops ...types.OpDef) (jsonschema.Schema, error) {
	r := jsonschema.Reflector{}

	r.AddTypeMapping((*Recipe)(nil), baseRecipe{})
	r.AddTypeMapping((*Node)(nil), baseNode{})
	r.AddTypeMapping((*[]Node)(nil), []baseNode{})

	schema, err := r.Reflect(
		(*Recipe)(nil),
		jsonschema.PropertyNameTag("yaml"),
		jsonschema.InterceptSchema(func(params jsonschema.InterceptSchemaParams) (stop bool, err error) {
			// Only apply to processed schemas that are objects
			if params.Processed && params.Schema.Type != nil {
				for _, t := range params.Schema.Type.SliceOfSimpleTypeValues {
					if t == jsonschema.Object && params.Schema.AdditionalProperties == nil {
						params.Schema.WithAdditionalProperties(addpropfalse)
						break
					}
				}
			}
			return false, nil
		}),
		jsonschema.InterceptDefName(func(t reflect.Type, defaultDefName string) string {
			if t == reflect.TypeOf(baseRecipe{}) {
				return "Recipe"
			} else if t == reflect.TypeOf(baseNode{}) {
				return "OneOfNode"
			} else {
				return t.Name()
			}

		}),
	)

	if err != nil {
		return jsonschema.Schema{}, err
	}
	oneof, err := getDef(ops)
	if err != nil {
		return jsonschema.Schema{}, err
	}
	schema.Definitions[refName] = oneof
	meta := "https://json-schema.org/draft/2020-12/schema"
	schema.Schema = &meta
	schema.WithTitle("Recipe")
	schema.WithDescription("Recipe schema")
	return schema, err

}

func GenerateSchemaString(ops ...types.OpDef) (string, error) {
	s, err := GenerateSchema(ops...)
	if err != nil {
		return "", err
	}
	j, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return "", nil
	}

	str := string(j)
	str = strings.ReplaceAll(str, "\"additionalProperties\": false", "\"unevaluatedProperties\": false")
	str = strings.ReplaceAll(str, "\"additionalProperties\": {}}", "\"unevaluatedProperties\": {}")

	return str, nil

}
func PrintSchema(ops ...types.OpDef) {
	s, err := GenerateSchemaString(ops...)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(s)
}
