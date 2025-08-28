package p2

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/divisive-ai/vibethis/server/recipe-core/ops"
	"github.com/invopop/jsonschema"
)

var sharedReflector = &jsonschema.Reflector{
	// Use yaml tag instead of json tag for field names
	FieldNameTag:               "yaml",
	AllowAdditionalProperties:  true,
	RequiredFromJSONSchemaTags: true,
	ExpandedStruct:             true,
	DoNotReference:             true,
}

func GenerateSchemaString() (string, error) {
	r := sharedReflector
	schema := r.Reflect(Recipe{})
	//schema := &jsonschema.Schema{}
	if schema.Definitions == nil {
		schema.Definitions = make(map[string]*jsonschema.Schema)
	}

	r.Anonymous = true
	nodeSchema, err := getNodeSchema(r)
	if err != nil {
		return "", err
	}
	schema.Definitions["Node"] = nodeSchema
	s, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", err
	}

	return string(s), nil

}

func stripSchema(s *jsonschema.Schema) *jsonschema.Schema {
	s.Version = ""
	return s
}

func getNodeSchema(r *jsonschema.Reflector) (*jsonschema.Schema, error) {
	nmd := r.Reflect(NodeMetadata{})
	opList := ops.List()
	items := make([]*jsonschema.Schema, 0, len(opList))
	for _, op := range opList {
		local, err := cloneSchema(nmd)
		if err != nil {
			return nil, err
		}
		opType := &jsonschema.Schema{}
		opType.Const = op.GetName()
		opType.Type = "string"
		local.Properties.Set("op", opType)
		local.Type = "object"
		local.Properties.Set("inputs", r.Reflect(op.GetInputStruct()))
		local.Required = append(local.Required, "op", "inputs")
		local.Title = op.GetName()
		items = append(items, local)
	}
	seq := stripSchema(r.Reflect(NodeSequence{}))
	seq.Title = "Sequence"
	items = append(items, seq)
	state := stripSchema(r.Reflect(NodeState{}))
	state.Title = "State"
	items = append(items, state)

	nodeopSchema := &jsonschema.Schema{
		Title: "Node",
		OneOf: items,
	}
	return nodeopSchema, nil
}

func cloneSchema(src *jsonschema.Schema) (*jsonschema.Schema, error) {
	if src == nil {
		return nil, nil
	}
	b, err := json.Marshal(src) // calls (*Schema).MarshalJSON
	if err != nil {
		return nil, err
	}
	var dst jsonschema.Schema
	if err := json.Unmarshal(b, &dst); err != nil { // calls (*Schema).UnmarshalJSON
		return nil, err
	}
	return &dst, nil
}

func PrintSchema() {
	s, err := GenerateSchemaString()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(s)
}

func oneOfSchema(name string, t ...any) *jsonschema.Schema {
	schemas := make([]*jsonschema.Schema, 0, len(t))
	reflector := sharedReflector
	reflector.Anonymous = true
	for _, typ := range t {
		schemas = append(schemas, stripSchema(reflector.Reflect(typ)))
	}
	reflector.Anonymous = false
	return &jsonschema.Schema{
		OneOf: schemas,
		Title: name,
	}
}

func (Recipe) JSONSchema() *jsonschema.Schema {
	return oneOfSchema("recipe", RecipeState{}, RecipeSequence{}, RecipeOp{})
}

//func (Node) JSONSchema() *jsonschema.Schema {
//	return oneOfSchema("node", NodeOp{}, NodeState{}, NodeShared{}, NodeSequence{})
//}
