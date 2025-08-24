package compiler

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
)

// TemplateResolver2 handles template resolution for inputs and outputs
type TemplateResolver2 struct {
	templates map[string]*template.Template
}

// NewTemplateResolver creates a new template resolver
func NewTemplateResolver2() *TemplateResolver2 {
	return &TemplateResolver2{
		templates: make(map[string]*template.Template),
	}
}

// ResolveInputs resolves template expressions in input values
func (t *TemplateResolver2) ResolveInputs(inputs map[string]interface{}, stateCtx *yamlpkg.StateContext) (map[string]interface{}, error) {
	if inputs == nil {
		return make(map[string]interface{}), nil
	}

	resolved := make(map[string]interface{})
	for key, value := range inputs {
		resolvedValue, err := t.resolveValue(value, stateCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve input '%s': %w", key, err)
		}
		resolved[key] = resolvedValue
	}

	return resolved, nil
}

// ResolveOutputs resolves template expressions in output values
func (t *TemplateResolver2) ResolveOutputs(outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) (map[string]interface{}, error) {
	if outputs == nil {
		return make(map[string]interface{}), nil
	}

	resolved := make(map[string]interface{})
	for key, value := range outputs {
		resolvedValue, err := t.resolveValue(value, stateCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve output '%s': %w", key, err)
		}
		resolved[key] = resolvedValue
	}

	return resolved, nil
}

// resolveValue resolves a single value which may contain template expressions
func (t *TemplateResolver2) resolveValue(value interface{}, stateCtx *yamlpkg.StateContext) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// Check if it's a template expression
		if isTemplateExpression(v) {
			return t.executeTemplate(v, stateCtx)
		}
		return v, nil

	case map[string]interface{}:
		// Recursively resolve map values
		resolved := make(map[string]interface{})
		for k, val := range v {
			resolvedVal, err := t.resolveValue(val, stateCtx)
			if err != nil {
				return nil, err
			}
			resolved[k] = resolvedVal
		}
		return resolved, nil

	case []interface{}:
		// Recursively resolve slice values
		resolved := make([]interface{}, len(v))
		for i, val := range v {
			resolvedVal, err := t.resolveValue(val, stateCtx)
			if err != nil {
				return nil, err
			}
			resolved[i] = resolvedVal
		}
		return resolved, nil

	default:
		// Return non-string values as-is
		return value, nil
	}
}

// executeTemplate executes a template expression
func (t *TemplateResolver2) executeTemplate(expr string, stateCtx *yamlpkg.StateContext) (interface{}, error) {
	// Get or create template with strict error handling
	tmpl, exists := t.templates[expr]
	if !exists {
		var err error
		// Use Option("missingkey=error") to fail on undefined variables
		tmpl, err = template.New(expr).Option("missingkey=error").Parse(expr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template: %w", err)
		}
		t.templates[expr] = tmpl
	}

	// Prepare template data
	data := t.prepareTemplateData(stateCtx)

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// Check if this is a missing key error
		if strings.Contains(err.Error(), "map has no entry for key") {
			// Return a placeholder to indicate the reference couldn't be resolved
			return "<invalid-ref>", nil
		}
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// prepareTemplateData prepares data for template execution
func (t *TemplateResolver2) prepareTemplateData(stateCtx *yamlpkg.StateContext) map[string]interface{} {
	data := make(map[string]interface{})

	// Add inputs
	data["Inputs"] = stateCtx.Inputs

	// Add state outputs
	data["States"] = stateCtx.StateOutputs

	// Add step outputs
	data["Steps"] = stateCtx.StepOutputs

	// Add context
	if stateCtx.RecipeContext != nil {
		data["Context"] = map[string]interface{}{
			"user_id":      getContextValue(stateCtx.Inputs, "user_id"),
			"execution_id": stateCtx.RecipeContext.Recipe.ExecutionID,
			"recipe_name":  stateCtx.RecipeContext.Recipe.Name,
		}
	}

	// Add current state outputs if available
	if current, exists := stateCtx.StateOutputs[stateCtx.CurrentState]; exists {
		data["Outputs"] = current
	}

	return data
}

// isTemplateExpression checks if a string contains a template expression
func isTemplateExpression(s string) bool {
	return len(s) >= 4 && s[:2] == "{{" && s[len(s)-2:] == "}}"
}

// getContextValue safely gets a value from a map
func getContextValue(m map[string]interface{}, key string) interface{} {
	if m == nil {
		return nil
	}
	return m[key]
}

// ScopedTemplateResolver handles template resolution with proper scoping
type ScopedTemplateResolver struct {
	templates map[string]*template.Template
}

// NewScopedTemplateResolver creates a new scoped template resolver
func NewScopedTemplateResolver() *ScopedTemplateResolver {
	return &ScopedTemplateResolver{
		templates: make(map[string]*template.Template),
	}
}

// ResolveInputsScoped resolves template expressions in input values with scoped context
func (t *ScopedTemplateResolver) ResolveInputsScoped(inputs map[string]interface{}, scope *ScopedContext) (map[string]interface{}, error) {
	if inputs == nil {
		return make(map[string]interface{}), nil
	}

	resolved := make(map[string]interface{})
	for key, value := range inputs {
		resolvedValue, err := t.resolveValueScoped(value, scope)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve input '%s': %w", key, err)
		}
		resolved[key] = resolvedValue
	}

	return resolved, nil
}

// resolveValueScoped resolves a single value which may contain template expressions
func (t *ScopedTemplateResolver) resolveValueScoped(value interface{}, scope *ScopedContext) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// Check if it's a template expression
		if isTemplateExpression(v) {
			return t.executeTemplateScoped(v, scope)
		}
		return v, nil

	case map[string]interface{}:
		// Recursively resolve map values
		resolved := make(map[string]interface{})
		for k, val := range v {
			resolvedVal, err := t.resolveValueScoped(val, scope)
			if err != nil {
				return nil, err
			}
			resolved[k] = resolvedVal
		}
		return resolved, nil

	case []interface{}:
		// Recursively resolve slice values
		resolved := make([]interface{}, len(v))
		for i, val := range v {
			resolvedVal, err := t.resolveValueScoped(val, scope)
			if err != nil {
				return nil, err
			}
			resolved[i] = resolvedVal
		}
		return resolved, nil

	default:
		// Return non-string values as-is
		return value, nil
	}
}

// executeTemplateScoped executes a template expression with scoped context
func (t *ScopedTemplateResolver) executeTemplateScoped(expr string, scope *ScopedContext) (interface{}, error) {
	// Get or create template with strict error handling
	tmpl, exists := t.templates[expr]
	if !exists {
		var err error
		// Use Option("missingkey=error") to fail on undefined variables
		tmpl, err = template.New(expr).Option("missingkey=error").Parse(expr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template: %w", err)
		}
		t.templates[expr] = tmpl
	}

	// Prepare template data from scoped context
	data := scope.GetTemplateData()

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// Check if this is a missing key error
		if strings.Contains(err.Error(), "map has no entry for key") {
			// Return a placeholder to indicate the reference couldn't be resolved
			return "<invalid-ref>", nil
		}
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
