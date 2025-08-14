package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/divisive-ai/vibethis/server/cortex/internal/shared"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var (
	schemaFormat         string
	schemaActivity       string
	schemaVersion        bool
	schemaOutput         string
	schemaIncludeExamples bool
)

// schemaCmd generates JSON schema for recipes and activities
var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Generate JSON schema for recipes and activities",
	Long:  `Automatically discovers and generates JSON schemas for all available activity types`,
	RunE:  runSchema,
}

func init() {
	schemaCmd.Flags().StringVarP(&schemaFormat, "format", "f", "json", "Output format (json, yaml, openapi)")
	schemaCmd.Flags().StringVarP(&schemaActivity, "activity", "a", "", "Filter to specific activity type")
	schemaCmd.Flags().BoolVarP(&schemaVersion, "version", "v", false, "Include version information in schema")
	schemaCmd.Flags().StringVarP(&schemaOutput, "output", "o", "", "Output file path (default: stdout)")
	schemaCmd.Flags().BoolVar(&schemaIncludeExamples, "include-examples", false, "Include example configurations in the schema")
}

func runSchema(cmd *cobra.Command, args []string) error {
	// Create logger
	logger := zap.NewNop()
	
	// Use shared registry manager
	rm, err := shared.NewRegistryManager(logger)
	if err != nil {
		return fmt.Errorf("failed to create registry manager: %w", err)
	}
	
	// Use shared schema manager
	sm := shared.NewSchemaManager(rm)
	schema, err := sm.GenerateCompleteSchema(schemaActivity, schemaVersion, schemaIncludeExamples)
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}
	
	// Convert to requested format
	var output []byte
	switch schemaFormat {
	case "json":
		output, err = json.MarshalIndent(schema, "", "  ")
	case "yaml":
		output, err = yaml.Marshal(schema)
	case "openapi":
		// Convert to OpenAPI schema format
		openAPISchema := convertToOpenAPI(schema)
		output, err = json.MarshalIndent(openAPISchema, "", "  ")
	default:
		return fmt.Errorf("unsupported format: %s", schemaFormat)
	}
	
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}
	
	// Output to file or stdout
	if schemaOutput != "" {
		return os.WriteFile(schemaOutput, output, 0644)
	}
	
	fmt.Print(string(output))
	return nil
}

// DEPRECATED: This function is kept for backward compatibility but is no longer used
// The schema generation is now handled by shared.SchemaManager
func generateCompleteSchemaLegacy(registry *worker.ActivityRegistry, filterActivity string, includeVersion bool, includeExamples bool) (map[string]interface{}, error) {
	// Base schema structure
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"title":   "Recipe Schema",
		"type":    "object",
		"properties": map[string]interface{}{
			"version": map[string]interface{}{
				"type":    "string",
				"default": "1.0",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Recipe name",
			},
			"description": map[string]interface{}{
				"type": "string",
			},
			"inputs": map[string]interface{}{
				"type":        "array",
				"description": "Recipe input definitions",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "string",
						},
						"type": map[string]interface{}{
							"type": "string",
							"enum": []string{"string", "number", "boolean", "object", "array"},
						},
						"required": map[string]interface{}{
							"type": "boolean",
						},
						"default": map[string]interface{}{},
					},
					"required": []string{"name", "type"},
				},
			},
			"steps": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "string",
						},
						"name": map[string]interface{}{
							"type": "string",
						},
						"type": map[string]interface{}{
							"type": "string",
							"enum": []string{"sequential", "parallel", "conditional"},
						},
						"activity": map[string]interface{}{
							"type": "object",
							"oneOf": []interface{}{},
						},
						"steps": map[string]interface{}{
							"type":        "array",
							"description": "Nested steps for compositions",
							"items":       map[string]interface{}{"$ref": "#/properties/steps/items"},
						},
						"branches": map[string]interface{}{
							"type":        "array",
							"description": "Conditional branches",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"condition": map[string]interface{}{
										"type": "string",
									},
									"steps": map[string]interface{}{
										"type":  "array",
										"items": map[string]interface{}{"$ref": "#/properties/steps/items"},
									},
								},
								"required": []string{"condition", "steps"},
							},
						},
					},
				},
			},
		},
		"definitions": map[string]interface{}{
			"activities": map[string]interface{}{},
			"configs":    map[string]interface{}{},
			"inputs":     map[string]interface{}{},
			"outputs":    map[string]interface{}{},
		},
	}
	
	if includeVersion {
		schema["version"] = "1.0.0"
	}
	
	// Get all activities or filter to specific one
	allActivities := registry.GetAll()
	
	// Sort activities for consistent output
	var activityTypes []string
	for actType := range allActivities {
		if filterActivity == "" || actType == filterActivity {
			activityTypes = append(activityTypes, actType)
		}
	}
	sort.Strings(activityTypes)
	
	// Generate schema for each activity
	definitions := schema["definitions"].(map[string]interface{})
	activityDefs := definitions["activities"].(map[string]interface{})
	configDefs := definitions["configs"].(map[string]interface{})
	inputDefs := definitions["inputs"].(map[string]interface{})
	outputDefs := definitions["outputs"].(map[string]interface{})
	
	oneOfList := []interface{}{}
	
	for _, actType := range activityTypes {
		registration := allActivities[actType]
		
		// Create activity schema reference
		activitySchema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"const": actType,
				},
			},
		}
		
		// Add config schema if available
		if registration.ConfigSchema != nil {
			configKey := sanitizeKey(actType)
			configDefs[configKey] = convertSchemaLegacy(registration.ConfigSchema)
			activitySchema["properties"].(map[string]interface{})["config"] = map[string]interface{}{
				"$ref": fmt.Sprintf("#/definitions/configs/%s", configKey),
			}
		}
		
		// Add input schema if available
		if registration.InputSchema != nil {
			inputKey := sanitizeKey(actType)
			inputDefs[inputKey] = convertSchemaLegacy(registration.InputSchema)
			activitySchema["properties"].(map[string]interface{})["inputs"] = map[string]interface{}{
				"$ref": fmt.Sprintf("#/definitions/inputs/%s", inputKey),
			}
		}
		
		// Add output schema if available
		if registration.OutputSchema != nil {
			outputKey := sanitizeKey(actType)
			outputDefs[outputKey] = convertSchemaLegacy(registration.OutputSchema)
			activitySchema["properties"].(map[string]interface{})["outputs"] = map[string]interface{}{
				"$ref": fmt.Sprintf("#/definitions/outputs/%s", outputKey),
			}
		}
		
		// Add metadata
		if registration.Metadata.Description != "" {
			activitySchema["description"] = registration.Metadata.Description
		}
		
		// Add example if requested
		if includeExamples {
			activitySchema["examples"] = generateExamples(actType)
		}
		
		activityDefs[sanitizeKey(actType)] = activitySchema
		
		// Add to oneOf list
		oneOfList = append(oneOfList, map[string]interface{}{
			"$ref": fmt.Sprintf("#/definitions/activities/%s", sanitizeKey(actType)),
		})
	}
	
	// Update the activity oneOf in steps
	steps := schema["properties"].(map[string]interface{})["steps"].(map[string]interface{})
	items := steps["items"].(map[string]interface{})
	props := items["properties"].(map[string]interface{})
	activityProp := props["activity"].(map[string]interface{})
	activityProp["oneOf"] = oneOfList
	
	// Add composition schemas
	addCompositionSchemas(definitions)
	
	return schema, nil
}

// DEPRECATED: Moved to shared package
func convertSchemaLegacy(schema interface{}) map[string]interface{} {
	return make(map[string]interface{})
}

func sanitizeKey(activityType string) string {
	// Convert activity type to valid JSON schema key
	// Replace special characters with underscores
	result := ""
	for _, ch := range activityType {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			result += string(ch)
		} else {
			result += "_"
		}
	}
	return result
}

func generateExamples(activityType string) []interface{} {
	// Generate example configurations for each activity type
	examples := []interface{}{}
	
	switch activityType {
	case "llm":
		examples = append(examples, map[string]interface{}{
			"type": "llm",
			"config": map[string]interface{}{
				"model":       "gpt-4",
				"max_tokens":  1000,
				"temperature": 0.7,
			},
			"inputs": map[string]interface{}{
				"prompt": "{{.Inputs.user_prompt}}",
			},
		})
	case "command_execution":
		examples = append(examples, map[string]interface{}{
			"type": "command_execution",
			"config": map[string]interface{}{
				"command": "echo",
				"args":    []string{"Hello", "World"},
				"timeout": "30s",
			},
			"inputs": map[string]interface{}{
				"env": map[string]string{
					"PATH": "/usr/bin:/usr/local/bin",
				},
			},
		})
	case "git_shallow":
		examples = append(examples, map[string]interface{}{
			"type": "git_shallow",
			"config": map[string]interface{}{
				"repo_url": "https://github.com/example/repo.git",
				"depth":    1,
			},
			"inputs": map[string]interface{}{
				"branch": "main",
			},
		})
	}
	
	return examples
}

func addCompositionSchemas(definitions map[string]interface{}) {
	compositions := map[string]interface{}{
		"sequential": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"inputs": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"$ref": "#/definitions/inputDefinition"},
					"description": "Explicit inputs from parent context (required for nested compositions)",
				},
				"outputs": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": map[string]interface{}{"type": "string"},
					"description":          "Explicit outputs accessible to parent context",
				},
				"steps": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"$ref": "#/properties/steps/items"},
				},
			},
			"required": []string{"steps"},
		},
		"parallel": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"inputs": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"$ref": "#/definitions/inputDefinition"},
				},
				"outputs": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"steps": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"$ref": "#/properties/steps/items"},
				},
			},
			"required": []string{"steps"},
		},
		"conditional": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"inputs": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"$ref": "#/definitions/inputDefinition"},
				},
				"outputs": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": map[string]interface{}{"type": "string"},
				},
				"branches": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"$ref": "#/definitions/conditionalBranch"},
				},
			},
			"required": []string{"branches"},
		},
	}
	
	definitions["compositions"] = compositions
	
	// Add input definition schema
	definitions["inputDefinition"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Internal name for the input",
			},
			"from": map[string]interface{}{
				"type":        "string",
				"description": "Template expression from parent context",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"string", "number", "boolean", "object", "array", "steps", "states"},
				"description": "Optional type hint for context passing",
			},
		},
		"required": []string{"name", "from"},
	}
	
	// Add conditional branch schema
	definitions["conditionalBranch"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"condition": map[string]interface{}{
				"type":        "string",
				"description": "CEL expression for branch condition",
			},
			"steps": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"$ref": "#/properties/steps/items"},
			},
		},
		"required": []string{"condition", "steps"},
	}
}

func convertToOpenAPI(schema map[string]interface{}) map[string]interface{} {
	// Convert JSON Schema to OpenAPI format
	openAPI := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":   "Recipe Schema API",
			"version": "1.0.0",
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"Recipe": schema,
			},
		},
	}
	
	return openAPI
}