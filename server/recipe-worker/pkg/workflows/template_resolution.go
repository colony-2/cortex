package workflows

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// resolveInputs resolves templates in input values
func resolveInputs(nodeInputs map[string]interface{}, workflowInputs map[string]interface{}) (map[string]interface{}, error) {
	resolved := make(map[string]interface{})

	// Create template context
	templateData := map[string]interface{}{
		"inputs": workflowInputs,
	}

	// Resolve each input value
	for key, value := range nodeInputs {
		resolvedValue, err := resolveValue(value, templateData)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve input %s: %w", key, err)
		}
		resolved[key] = resolvedValue
	}

	return resolved, nil
}

// resolveValue resolves a single value that may contain templates
func resolveValue(value interface{}, data map[string]interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// Check if it contains templates
		if strings.Contains(v, "{{") && strings.Contains(v, "}}") {
			return executeTemplate(v, data)
		}
		return v, nil
	case map[string]interface{}:
		// Recursively resolve map values
		resolved := make(map[string]interface{})
		for k, val := range v {
			resolvedVal, err := resolveValue(val, data)
			if err != nil {
				return nil, err
			}
			resolved[k] = resolvedVal
		}
		return resolved, nil
	case []interface{}:
		// Recursively resolve array values
		resolved := make([]interface{}, len(v))
		for i, val := range v {
			resolvedVal, err := resolveValue(val, data)
			if err != nil {
				return nil, err
			}
			resolved[i] = resolvedVal
		}
		return resolved, nil
	default:
		// Return other types as-is
		return v, nil
	}
}

// executeTemplate executes a Go template string
func executeTemplate(expr string, data map[string]interface{}) (interface{}, error) {
	tmpl, err := template.New("expr").Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// mergeInputs merges various input sources
func mergeInputs(baseInputs, nodeInputs map[string]interface{}, nodeOutputs map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Copy base inputs
	for k, v := range baseInputs {
		merged[k] = v
	}

	// Override with node-specific inputs
	for k, v := range nodeInputs {
		merged[k] = v
	}

	// Add node outputs for reference
	if nodeOutputs != nil {
		merged["nodes"] = nodeOutputs
	}

	return merged
}
