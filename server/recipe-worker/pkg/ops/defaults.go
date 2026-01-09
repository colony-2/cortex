package ops

import (
	"reflect"
	"strings"
)

// InjectDefaults adds default values to the input map for fields that are missing.
// Defaults are injected as strings and will be resolved by the template system.
// This function recursively handles nested structs at arbitrary depth.
func InjectDefaults(inputType reflect.Type, inputMap map[string]interface{}) error {
	if inputType.Kind() == reflect.Ptr {
		inputType = inputType.Elem()
	}

	if inputType.Kind() != reflect.Struct {
		return nil
	}

	return injectDefaultsRecursive(inputType, inputMap)
}

func injectDefaultsRecursive(structType reflect.Type, inputMap map[string]interface{}) error {
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON tag to determine map key
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Extract field name (before comma, which may have omitempty etc)
		fieldName := strings.Split(jsonTag, ",")[0]
		if fieldName == "" {
			continue
		}

		// Dereference pointer types to check underlying type
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// Handle nested structs
		if fieldType.Kind() == reflect.Struct {
			// Get or create nested map
			var nestedMap map[string]interface{}
			if existing, exists := inputMap[fieldName]; exists {
				var ok bool
				nestedMap, ok = existing.(map[string]interface{})
				if !ok {
					// User provided a non-map value, skip recursion
					continue
				}
			} else {
				// Create nested map for defaults
				nestedMap = make(map[string]interface{})
				inputMap[fieldName] = nestedMap
			}

			// Recurse into nested struct
			if err := injectDefaultsRecursive(fieldType, nestedMap); err != nil {
				return err
			}

			// If nested map is empty after recursion, remove it
			if len(nestedMap) == 0 {
				delete(inputMap, fieldName)
			}

			continue
		}

		// For non-struct fields, check if value already provided
		if _, exists := inputMap[fieldName]; exists {
			continue
		}

		// Check for default tag
		defaultValue := field.Tag.Get("default")
		if defaultValue == "" {
			continue
		}

		// Inject default as string (template resolver will handle type conversion)
		inputMap[fieldName] = defaultValue
	}

	return nil
}
