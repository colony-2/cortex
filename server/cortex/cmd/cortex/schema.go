package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/divisive-ai/vibethis/server/cortex/internal/shared"
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