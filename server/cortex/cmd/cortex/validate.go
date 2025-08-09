package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/divisive-ai/vibethis/server/ops/pkg/activity"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/worker"
	"github.com/olekukonko/tablewriter"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	validateSchemaPath string
	validateStrict     bool
	validateFormat     string
	validateOutput     string
)

// validateCmd validates recipe files against schema
var validateCmd = &cobra.Command{
	Use:   "validate <recipe-file> [recipe-files...]",
	Short: "Validate recipe files against schema",
	Long: `Validates one or more recipe YAML files against the auto-generated schema.
Returns exit code 0 if all files are valid, 1 if any validation errors found.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().StringVarP(&validateSchemaPath, "schema", "s", "", "Path to custom schema file (optional, uses built-in schema by default)")
	validateCmd.Flags().BoolVar(&validateStrict, "strict", false, "Enable strict validation mode (fail on warnings)")
	validateCmd.Flags().StringVarP(&validateFormat, "format", "f", "text", "Output format for validation results (text, markdown, json)")
	validateCmd.Flags().StringVarP(&validateOutput, "output", "o", "", "Output file path for validation results (default: stdout)")
}

// ValidationResult holds the result of validating a single file
type ValidationResult struct {
	File     string            `json:"file"`
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []ValidationError `json:"warnings,omitempty"`
}

// ValidationError represents a single validation error
type ValidationError struct {
	Field       string      `json:"field"`
	Description string      `json:"description"`
	Schema      string      `json:"schema,omitempty"`
	ErrorType   string      `json:"errorType"`
	Expected    interface{} `json:"expected,omitempty"`
	Actual      interface{} `json:"actual,omitempty"`
}

// ValidationSummary holds the overall validation results
type ValidationSummary struct {
	Results        []ValidationResult `json:"results"`
	FilesValidated int                `json:"filesValidated"`
	Valid          int                `json:"valid"`
	Invalid        int                `json:"invalid"`
	TotalErrors    int                `json:"totalErrors"`
	TotalWarnings  int                `json:"totalWarnings"`
}

func runValidate(cmd *cobra.Command, args []string) error {
	// Initialize schema
	var schema *jsonschema.Schema
	if validateSchemaPath != "" {
		// Load custom schema from file
		schemaData, err := os.ReadFile(validateSchemaPath)
		if err != nil {
			return fmt.Errorf("failed to read schema file: %w", err)
		}
		
		var schemaDoc interface{}
		if err := json.Unmarshal(schemaData, &schemaDoc); err != nil {
			return fmt.Errorf("failed to parse schema: %w", err)
		}
		
		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft7
		
		// Create a string reader for the schema
		schemaReader := bytes.NewReader(schemaData)
		if err := compiler.AddResource("schema.json", schemaReader); err != nil {
			return fmt.Errorf("failed to add schema resource: %w", err)
		}
		
		schema = compiler.MustCompile("schema.json")
	} else {
		// Generate schema from registered activities
		registry := worker.NewActivityRegistry()
		activities := activity.GetAll()
		for _, act := range activities {
			// Ignore duplicate registration errors
			if err := registry.RegisterGeneric(act); err != nil {
				// Skip duplicate registrations
				if !strings.Contains(err.Error(), "already registered") {
					return fmt.Errorf("failed to register activity: %w", err)
				}
			}
		}
		
		schemaMap, err := generateCompleteSchema(registry, "", false, false)
		if err != nil {
			return fmt.Errorf("failed to generate schema: %w", err)
		}
		
		// Convert schema map to JSON bytes
		schemaBytes, err := json.Marshal(schemaMap)
		if err != nil {
			return fmt.Errorf("failed to marshal schema: %w", err)
		}
		
		compiler := jsonschema.NewCompiler()
		compiler.Draft = jsonschema.Draft7
		
		schemaReader := bytes.NewReader(schemaBytes)
		if err := compiler.AddResource("schema.json", schemaReader); err != nil {
			return fmt.Errorf("failed to add schema resource: %w", err)
		}
		
		schema = compiler.MustCompile("schema.json")
	}
	
	// Expand file patterns
	var files []string
	for _, pattern := range args {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("invalid file pattern %s: %w", pattern, err)
		}
		if len(matches) == 0 {
			// If no matches, treat as literal file path
			files = append(files, pattern)
		} else {
			files = append(files, matches...)
		}
	}
	
	// Validate each file
	summary := ValidationSummary{
		Results:        []ValidationResult{},
		FilesValidated: len(files),
	}
	
	for _, file := range files {
		result := validateFile(file, schema, validateStrict)
		summary.Results = append(summary.Results, result)
		
		if result.Valid {
			summary.Valid++
		} else {
			summary.Invalid++
		}
		
		summary.TotalErrors += len(result.Errors)
		summary.TotalWarnings += len(result.Warnings)
	}
	
	// Output results
	if err := outputResults(summary, validateFormat, validateOutput); err != nil {
		return err
	}
	
	// Return error if any files invalid (for CI/CD integration)
	if summary.Invalid > 0 {
		os.Exit(1)
	}
	
	return nil
}

func validateFile(filepath string, schema *jsonschema.Schema, strict bool) ValidationResult {
	result := ValidationResult{
		File:  filepath,
		Valid: true,
	}
	
	// Read file
	data, err := os.ReadFile(filepath)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:       "",
			Description: fmt.Sprintf("Failed to read file: %v", err),
			ErrorType:   "file_error",
		})
		return result
	}
	
	// Parse YAML
	var doc interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:       "",
			Description: fmt.Sprintf("Failed to parse YAML: %v", err),
			ErrorType:   "parse_error",
		})
		return result
	}
	
	// Convert YAML types to JSON-compatible types
	doc = convertYAMLToJSON(doc)
	
	// Validate template variables
	templateErrors := validateTemplateVariables(string(data), doc)
	result.Errors = append(result.Errors, templateErrors...)
	
	// Validate against schema
	if err := schema.Validate(doc); err != nil {
		result.Valid = false
		
		// Parse validation errors
		if validationErr, ok := err.(*jsonschema.ValidationError); ok {
			result.Errors = append(result.Errors, parseValidationError(validationErr)...)
		} else {
			result.Errors = append(result.Errors, ValidationError{
				Field:       "",
				Description: err.Error(),
				ErrorType:   "validation_error",
			})
		}
	}
	
	// In strict mode, warnings become errors
	if strict && len(result.Warnings) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, result.Warnings...)
		result.Warnings = nil
	}
	
	if len(result.Errors) > 0 {
		result.Valid = false
	}
	
	return result
}

func convertYAMLToJSON(data interface{}) interface{} {
	// Convert YAML types to JSON-compatible types
	switch v := data.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			result[fmt.Sprintf("%v", key)] = convertYAMLToJSON(value)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = convertYAMLToJSON(item)
		}
		return result
	default:
		return v
	}
}

func parseValidationError(err *jsonschema.ValidationError) []ValidationError {
	var errors []ValidationError
	
	// Main error
	errors = append(errors, ValidationError{
		Field:       err.InstanceLocation,
		Description: err.Message,
		Schema:      "", // SchemaLocation not available in this version
		ErrorType:   getErrorType(err),
	})
	
	// Process sub-errors
	for _, cause := range err.Causes {
		errors = append(errors, parseValidationError(cause)...)
	}
	
	return errors
}

func getErrorType(err *jsonschema.ValidationError) string {
	// Extract error type from the validation error
	msg := strings.ToLower(err.Message)
	if strings.Contains(msg, "enum") {
		return "enum"
	} else if strings.Contains(msg, "type") {
		return "type"
	} else if strings.Contains(msg, "required") {
		return "required"
	} else if strings.Contains(msg, "minimum") {
		return "minimum"
	} else if strings.Contains(msg, "maximum") {
		return "maximum"
	} else if strings.Contains(msg, "pattern") {
		return "pattern"
	} else if strings.Contains(msg, "format") {
		return "format"
	} else if strings.Contains(msg, "minitems") {
		return "minItems"
	} else if strings.Contains(msg, "maxitems") {
		return "maxItems"
	} else if strings.Contains(msg, "unique") {
		return "uniqueItems"
	} else if strings.Contains(msg, "additional") {
		return "additionalProperties"
	}
	return "validation"
}

func validateTemplateVariables(content string, doc interface{}) []ValidationError {
	var errors []ValidationError
	
	// Extract available inputs and steps from the document
	availableInputs := extractInputs(doc)
	availableSteps := extractSteps(doc)
	
	// Find all template expressions
	templatePattern := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	matches := templatePattern.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		expr := strings.TrimSpace(match[1])
		
		// Check different types of references
		if strings.HasPrefix(expr, ".Inputs.") {
			inputName := strings.TrimPrefix(expr, ".Inputs.")
			inputName = strings.Split(inputName, ".")[0] // Get just the input name
			if !contains(availableInputs, inputName) {
				errors = append(errors, ValidationError{
					Field:       expr,
					Description: fmt.Sprintf("Reference to undefined input: %s", inputName),
					ErrorType:   "undefined_input",
				})
			}
		} else if strings.HasPrefix(expr, ".Steps.") {
			parts := strings.Split(expr, ".")
			if len(parts) >= 5 && parts[3] == "outputs" {
				stepID := parts[2]
				if !contains(availableSteps, stepID) {
					errors = append(errors, ValidationError{
						Field:       expr,
						Description: fmt.Sprintf("Reference to undefined step: %s", stepID),
						ErrorType:   "undefined_step",
					})
				}
			}
		} else if strings.HasPrefix(expr, ".Context.") {
			contextField := strings.TrimPrefix(expr, ".Context.")
			validContextFields := []string{"workflowID", "runID"}
			if !contains(validContextFields, contextField) {
				errors = append(errors, ValidationError{
					Field:       expr,
					Description: fmt.Sprintf("Invalid context field: %s", contextField),
					ErrorType:   "invalid_context_field",
				})
			}
		}
	}
	
	return errors
}

func extractInputs(doc interface{}) []string {
	var inputs []string
	
	if docMap, ok := doc.(map[string]interface{}); ok {
		if inputsList, ok := docMap["inputs"].([]interface{}); ok {
			for _, input := range inputsList {
				if inputMap, ok := input.(map[string]interface{}); ok {
					if name, ok := inputMap["name"].(string); ok {
						inputs = append(inputs, name)
					}
				}
			}
		}
	}
	
	return inputs
}

func extractSteps(doc interface{}) []string {
	var steps []string
	
	var extractStepsRecursive func(interface{})
	extractStepsRecursive = func(data interface{}) {
		if dataMap, ok := data.(map[string]interface{}); ok {
			// Check for step ID
			if id, ok := dataMap["id"].(string); ok {
				steps = append(steps, id)
			}
			
			// Check for nested steps
			if stepsList, ok := dataMap["steps"].([]interface{}); ok {
				for _, step := range stepsList {
					extractStepsRecursive(step)
				}
			}
			
			// Check for branches (conditional)
			if branches, ok := dataMap["branches"].([]interface{}); ok {
				for _, branch := range branches {
					extractStepsRecursive(branch)
				}
			}
		} else if dataList, ok := data.([]interface{}); ok {
			for _, item := range dataList {
				extractStepsRecursive(item)
			}
		}
	}
	
	if docMap, ok := doc.(map[string]interface{}); ok {
		if stepsList, ok := docMap["steps"].([]interface{}); ok {
			extractStepsRecursive(stepsList)
		}
	}
	
	return steps
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func outputResults(summary ValidationSummary, format string, outputPath string) error {
	var output string
	
	switch format {
	case "text":
		output = formatTextOutput(summary)
	case "markdown":
		output = formatMarkdownOutput(summary)
	case "json":
		bytes, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		output = string(bytes)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
	
	if outputPath != "" {
		return os.WriteFile(outputPath, []byte(output), 0644)
	}
	
	fmt.Print(output)
	return nil
}

func formatTextOutput(summary ValidationSummary) string {
	var buf bytes.Buffer
	
	// Create summary table
	summaryTable := tablewriter.NewWriter(&buf)
	summaryTable.SetHeader([]string{"File", "Status", "Errors", "Warnings"})
	summaryTable.SetBorder(false)
	summaryTable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	summaryTable.SetAlignment(tablewriter.ALIGN_LEFT)
	summaryTable.SetCenterSeparator("")
	summaryTable.SetColumnSeparator("")
	summaryTable.SetRowSeparator("")
	summaryTable.SetHeaderLine(false)
	summaryTable.SetTablePadding("\t")
	summaryTable.SetNoWhiteSpace(true)
	
	for _, result := range summary.Results {
		status := "✅ VALID"
		if !result.Valid {
			status = "❌ INVALID"
		}
		summaryTable.Append([]string{
			result.File,
			status,
			fmt.Sprintf("%d", len(result.Errors)),
			fmt.Sprintf("%d", len(result.Warnings)),
		})
	}
	
	buf.WriteString("VALIDATION RESULTS\n")
	buf.WriteString("==================\n\n")
	summaryTable.Render()
	
	// Add detailed errors for invalid files
	hasErrors := false
	for _, result := range summary.Results {
		if !result.Valid && len(result.Errors) > 0 {
			if !hasErrors {
				buf.WriteString("\n\nDETAILED ERRORS\n")
				buf.WriteString("===============\n")
				hasErrors = true
			}
			
			buf.WriteString(fmt.Sprintf("\n%s:\n", result.File))
			
			// Create error table
			errorTable := tablewriter.NewWriter(&buf)
			errorTable.SetHeader([]string{"Type", "Field", "Description"})
			errorTable.SetBorder(false)
			errorTable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
			errorTable.SetAlignment(tablewriter.ALIGN_LEFT)
			errorTable.SetCenterSeparator("")
			errorTable.SetColumnSeparator("")
			errorTable.SetRowSeparator("")
			errorTable.SetHeaderLine(false)
			errorTable.SetTablePadding("  ")
			errorTable.SetNoWhiteSpace(true)
			
			for _, err := range result.Errors {
				field := err.Field
				if field == "" {
					field = "-"
				}
				errorTable.Append([]string{
					err.ErrorType,
					field,
					err.Description,
				})
			}
			errorTable.Render()
		}
	}
	
	// Add summary statistics
	buf.WriteString("\n\nSUMMARY\n")
	buf.WriteString("=======\n")
	buf.WriteString(fmt.Sprintf("Files validated: %d\n", summary.FilesValidated))
	buf.WriteString(fmt.Sprintf("Valid:           %d\n", summary.Valid))
	buf.WriteString(fmt.Sprintf("Invalid:         %d\n", summary.Invalid))
	if summary.TotalErrors > 0 {
		buf.WriteString(fmt.Sprintf("Total errors:    %d\n", summary.TotalErrors))
	}
	if summary.TotalWarnings > 0 {
		buf.WriteString(fmt.Sprintf("Total warnings:  %d\n", summary.TotalWarnings))
	}
	
	return buf.String()
}

func formatMarkdownOutput(summary ValidationSummary) string {
	var buf bytes.Buffer
	
	buf.WriteString("# Recipe Validation Results\n\n")
	
	// Create summary table in markdown
	buf.WriteString("## Summary\n\n")
	
	summaryTable := tablewriter.NewWriter(&buf)
	summaryTable.SetHeader([]string{"File", "Status", "Errors", "Warnings"})
	summaryTable.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	summaryTable.SetCenterSeparator("|")
	summaryTable.SetAutoWrapText(false)
	
	for _, result := range summary.Results {
		status := "✅ VALID"
		if !result.Valid {
			status = "❌ INVALID"
		}
		summaryTable.Append([]string{
			fmt.Sprintf("`%s`", result.File),
			status,
			fmt.Sprintf("%d", len(result.Errors)),
			fmt.Sprintf("%d", len(result.Warnings)),
		})
	}
	summaryTable.Render()
	
	// Add detailed errors
	hasErrors := false
	for _, result := range summary.Results {
		if !result.Valid && len(result.Errors) > 0 {
			if !hasErrors {
				buf.WriteString("\n## Detailed Errors\n\n")
				hasErrors = true
			}
			
			buf.WriteString(fmt.Sprintf("### %s\n\n", result.File))
			
			// Create error table in markdown
			errorTable := tablewriter.NewWriter(&buf)
			errorTable.SetHeader([]string{"Type", "Field", "Description"})
			errorTable.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
			errorTable.SetCenterSeparator("|")
			errorTable.SetAutoWrapText(false)
			
			for _, err := range result.Errors {
				field := err.Field
				if field == "" {
					field = "-"
				} else {
					field = fmt.Sprintf("`%s`", field)
				}
				errorTable.Append([]string{
					err.ErrorType,
					field,
					err.Description,
				})
			}
			errorTable.Render()
			buf.WriteString("\n")
		}
	}
	
	// Add statistics
	buf.WriteString("\n## Statistics\n\n")
	buf.WriteString(fmt.Sprintf("- **Files validated:** %d\n", summary.FilesValidated))
	buf.WriteString(fmt.Sprintf("- **Valid:** %d\n", summary.Valid))
	buf.WriteString(fmt.Sprintf("- **Invalid:** %d\n", summary.Invalid))
	if summary.TotalErrors > 0 {
		buf.WriteString(fmt.Sprintf("- **Total errors:** %d\n", summary.TotalErrors))
	}
	if summary.TotalWarnings > 0 {
		buf.WriteString(fmt.Sprintf("- **Total warnings:** %d\n", summary.TotalWarnings))
	}
	
	return buf.String()
}