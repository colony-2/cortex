package ops

import (
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
)

// DefaultSchemaGenerator uses invopop/jsonschema for schema generation
type DefaultSchemaGenerator struct {
	reflector *jsonschema.Reflector
}

// NewDefaultSchemaGenerator creates a new schema generator
func NewDefaultSchemaGenerator() *DefaultSchemaGenerator {
	reflector := &jsonschema.Reflector{
		// Don't allow additional properties by default
		AllowAdditionalProperties: false,
		// Anonymous fields should be expanded
		ExpandedStruct: true,
	}

	return &DefaultSchemaGenerator{
		reflector: reflector,
	}
}

// GenerateSchema creates a JSON schema from a Go type
func (g *DefaultSchemaGenerator) GenerateSchema(typ reflect.Type) (*jsonschema.Schema, error) {
	// Handle nil types
	if typ == nil {
		return &jsonschema.Schema{
			Type: "object",
			AdditionalProperties: &jsonschema.Schema{},
		}, nil
	}
	
	// Handle interface types
	if typ.Kind() == reflect.Interface {
		// For interface{} types, return a schema that accepts any type
		return &jsonschema.Schema{
			Type: "object",
			AdditionalProperties: &jsonschema.Schema{},
		}, nil
	}
	
	// Handle map types
	if typ.Kind() == reflect.Map {
		// For map[string]interface{} types, return a flexible object schema
		return &jsonschema.Schema{
			Type: "object",
			AdditionalProperties: &jsonschema.Schema{},
		}, nil
	}
	
	// Create a value from the type
	val := reflect.New(typ).Interface()
	
	// Use invopop/jsonschema to generate the schema
	schema := g.reflector.Reflect(val)
	return schema, nil
}

// ValidateStructTags ensures all fields have explicit json tags
func (g *DefaultSchemaGenerator) ValidateStructTags(typ reflect.Type) error {
	// Handle nil types
	if typ == nil {
		return nil
	}
	
	// Handle interface and map types - they don't need validation
	if typ.Kind() == reflect.Interface || typ.Kind() == reflect.Map {
		return nil
	}
	
	// Ensure type is a struct
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil // Non-structs don't need json tags
	}

	// Check each field has an explicit json tag
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Check for json tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			return fmt.Errorf("field %s.%s is missing required json tag", typ.Name(), field.Name)
		}

		// If field is "-", it's explicitly ignored, which is fine
		if jsonTag == "-" {
			continue
		}

		// Recursively check nested structs
		fieldType := field.Type
		if fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Array {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Map {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// Only validate struct types
		if fieldType.Kind() == reflect.Struct {
			// Skip time.Time and other standard library types
			if fieldType.PkgPath() == "" || fieldType.PkgPath() == "time" || 
			   fieldType.PkgPath() == "encoding/json" {
				continue
			}
			
			if err := g.ValidateStructTags(fieldType); err != nil {
				return err
			}
		}
	}

	return nil
}