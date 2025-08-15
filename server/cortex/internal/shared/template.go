package shared

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// TemplateResolver handles template resolution for inputs and outputs
type TemplateResolver struct {
	funcMap template.FuncMap
	celEval *CELEvaluator
}

// NewTemplateResolver creates a new template resolver
func NewTemplateResolver() (*TemplateResolver, error) {
	celEval, err := NewCELEvaluator()
	if err != nil {
		return nil, err
	}
	
	return &TemplateResolver{
		funcMap: createFuncMap(),
		celEval: celEval,
	}, nil
}

// ResolveOutputTemplate resolves output templates with node execution results
func (tr *TemplateResolver) ResolveOutputTemplate(outputTemplate map[string]interface{}, nodeOutputs map[string]map[string]interface{}, inputs map[string]interface{}) (map[string]interface{}, error) {
	if outputTemplate == nil {
		return nil, nil
	}
	
	resolved := make(map[string]interface{})
	
	// Prepare template data
	data := map[string]interface{}{
		"nodes":  nodeOutputs,
		"inputs": inputs,
	}
	
	// Resolve each output field
	for key, value := range outputTemplate {
		resolvedValue, err := tr.resolveValue(value, data)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve output '%s': %w", key, err)
		}
		resolved[key] = resolvedValue
	}
	
	return resolved, nil
}

// ResolveInputTemplate resolves input templates for node execution
func (tr *TemplateResolver) ResolveInputTemplate(inputTemplate map[string]interface{}, context map[string]interface{}) (map[string]interface{}, error) {
	if inputTemplate == nil {
		return nil, nil
	}
	
	resolved := make(map[string]interface{})
	
	// Resolve each input field
	for key, value := range inputTemplate {
		resolvedValue, err := tr.resolveValue(value, context)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve input '%s': %w", key, err)
		}
		resolved[key] = resolvedValue
	}
	
	return resolved, nil
}

// resolveValue recursively resolves template values
func (tr *TemplateResolver) resolveValue(value interface{}, data map[string]interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// Check if it's a template string
		if isTemplate(v) {
			return tr.resolveTemplate(v, data)
		}
		return v, nil
		
	case map[string]interface{}:
		// Recursively resolve map values
		resolved := make(map[string]interface{})
		for key, val := range v {
			resolvedVal, err := tr.resolveValue(val, data)
			if err != nil {
				return nil, err
			}
			resolved[key] = resolvedVal
		}
		return resolved, nil
		
	case []interface{}:
		// Recursively resolve array values
		resolved := make([]interface{}, len(v))
		for i, val := range v {
			resolvedVal, err := tr.resolveValue(val, data)
			if err != nil {
				return nil, err
			}
			resolved[i] = resolvedVal
		}
		return resolved, nil
		
	default:
		// Return non-template values as-is
		return v, nil
	}
}

// resolveTemplate resolves a template string
func (tr *TemplateResolver) resolveTemplate(templateStr string, data map[string]interface{}) (interface{}, error) {
	// Handle different template formats
	
	// 1. Simple variable reference: {{ .nodes.X.outputs.Y }}
	if isSimpleReference(templateStr) {
		return tr.resolveReference(templateStr, data)
	}
	
	// 2. CEL expression: {{ cel: expression }}
	if isCELExpression(templateStr) {
		return tr.resolveCEL(templateStr, data)
	}
	
	// 3. Go template: complex template with logic
	return tr.resolveGoTemplate(templateStr, data)
}

// resolveReference resolves a simple reference like {{ .nodes.X.outputs.Y }}
func (tr *TemplateResolver) resolveReference(ref string, data map[string]interface{}) (interface{}, error) {
	// Extract the reference path
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "{{")
	ref = strings.TrimSuffix(ref, "}}")
	ref = strings.TrimSpace(ref)
	
	// Remove leading dot
	if strings.HasPrefix(ref, ".") {
		ref = ref[1:]
	}
	
	// Split path into parts
	parts := strings.Split(ref, ".")
	
	// Navigate through the data structure
	current := data
	for i, part := range parts {
		if part == "" {
			continue
		}
		
		// Check if current is a map
		if currentMap, ok := current[part]; ok {
			if i == len(parts)-1 {
				// Last part - return the value
				return currentMap, nil
			}
			// Navigate deeper
			if nextMap, ok := currentMap.(map[string]interface{}); ok {
				current = nextMap
			} else if nextMap, ok := currentMap.(map[string]map[string]interface{}); ok {
				// Handle nested maps
				if i+1 < len(parts) {
					if nested, ok := nextMap[parts[i+1]]; ok {
						current = nested
						i++ // Skip next part as we've already processed it
					}
				}
			} else {
				return nil, fmt.Errorf("cannot navigate through non-map value at '%s'", part)
			}
		} else {
			return nil, fmt.Errorf("field '%s' not found in path '%s'", part, ref)
		}
	}
	
	return current, nil
}

// resolveCEL resolves a CEL expression template
func (tr *TemplateResolver) resolveCEL(templateStr string, data map[string]interface{}) (interface{}, error) {
	// Extract CEL expression
	expr := extractCELExpression(templateStr)
	if expr == "" {
		return nil, fmt.Errorf("invalid CEL expression template: %s", templateStr)
	}
	
	// Evaluate CEL expression
	return tr.celEval.Evaluate(expr, data)
}

// resolveGoTemplate resolves a Go template
func (tr *TemplateResolver) resolveGoTemplate(templateStr string, data map[string]interface{}) (interface{}, error) {
	// Create template
	tmpl, err := template.New("output").Funcs(tr.funcMap).Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	
	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	
	result := buf.String()
	
	// Try to preserve type if possible
	if result == "true" {
		return true, nil
	}
	if result == "false" {
		return false, nil
	}
	
	// Check if it's a number
	var num float64
	if _, err := fmt.Sscanf(result, "%f", &num); err == nil {
		// Check if it's an integer
		if num == float64(int(num)) {
			return int(num), nil
		}
		return num, nil
	}
	
	return result, nil
}

// isTemplate checks if a string is a template
func isTemplate(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

// isSimpleReference checks if a template is a simple reference
func isSimpleReference(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{{") || !strings.HasSuffix(s, "}}") {
		return false
	}
	
	inner := strings.TrimSpace(s[2 : len(s)-2])
	// Simple reference starts with . and doesn't contain spaces (except in strings)
	return strings.HasPrefix(inner, ".") && !strings.Contains(inner, " ")
}

// isCELExpression checks if a template is a CEL expression
func isCELExpression(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{{") || !strings.HasSuffix(s, "}}") {
		return false
	}
	
	inner := strings.TrimSpace(s[2 : len(s)-2])
	return strings.HasPrefix(inner, "cel:") || strings.HasPrefix(inner, "CEL:")
}

// extractCELExpression extracts the CEL expression from a template
func extractCELExpression(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{{")
	s = strings.TrimSuffix(s, "}}")
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "cel:")
	s = strings.TrimPrefix(s, "CEL:")
	return strings.TrimSpace(s)
}

// createFuncMap creates template functions
func createFuncMap() template.FuncMap {
	return template.FuncMap{
		// String functions
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"trim":      strings.TrimSpace,
		"replace":   strings.ReplaceAll,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		
		// Type conversion
		"toString": func(v interface{}) string {
			return fmt.Sprintf("%v", v)
		},
		"toInt": func(v interface{}) int {
			switch val := v.(type) {
			case int:
				return val
			case float64:
				return int(val)
			case string:
				var i int
				fmt.Sscanf(val, "%d", &i)
				return i
			default:
				return 0
			}
		},
		
		// JSON functions
		"toJSON": func(v interface{}) string {
			// Simplified - real implementation would use json.Marshal
			return fmt.Sprintf("%v", v)
		},
		
		// List functions
		"first": func(list []interface{}) interface{} {
			if len(list) > 0 {
				return list[0]
			}
			return nil
		},
		"last": func(list []interface{}) interface{} {
			if len(list) > 0 {
				return list[len(list)-1]
			}
			return nil
		},
		"join": func(sep string, list []interface{}) string {
			strs := make([]string, len(list))
			for i, v := range list {
				strs[i] = fmt.Sprintf("%v", v)
			}
			return strings.Join(strs, sep)
		},
		
		// Map functions
		"get": func(m map[string]interface{}, key string) interface{} {
			return m[key]
		},
		"default": func(def interface{}, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
	}
}

// ValidateTemplate validates a template string
func (tr *TemplateResolver) ValidateTemplate(templateStr string) error {
	if !isTemplate(templateStr) {
		// Not a template, valid as plain string
		return nil
	}
	
	// Try to parse as different template types
	if isSimpleReference(templateStr) {
		// Validate reference format
		ref := strings.TrimSpace(templateStr[2 : len(templateStr)-2])
		if !strings.HasPrefix(ref, ".") {
			return fmt.Errorf("simple reference must start with '.': %s", templateStr)
		}
		return nil
	}
	
	if isCELExpression(templateStr) {
		// Validate CEL expression
		expr := extractCELExpression(templateStr)
		return tr.celEval.ValidateExpression(expr)
	}
	
	// Validate as Go template
	_, err := template.New("validate").Funcs(tr.funcMap).Parse(templateStr)
	return err
}

// ExtractTemplateVariables extracts variable references from a template
func ExtractTemplateVariables(templateStr string) []string {
	vars := []string{}
	seen := make(map[string]bool)
	
	// Pattern to match variable references
	pattern := regexp.MustCompile(`\{\{\s*\.([a-zA-Z_][a-zA-Z0-9_\.]*)\s*\}\}`)
	matches := pattern.FindAllStringSubmatch(templateStr, -1)
	
	for _, match := range matches {
		if len(match) > 1 {
			varPath := match[1]
			// Get the root variable name
			parts := strings.Split(varPath, ".")
			if len(parts) > 0 && !seen[parts[0]] {
				vars = append(vars, parts[0])
				seen[parts[0]] = true
			}
		}
	}
	
	return vars
}