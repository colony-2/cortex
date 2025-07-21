package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// TemplateResolver resolves template expressions in workflow definitions
type TemplateResolver struct {
	state *WorkflowState
}

// NewTemplateResolver creates a new template resolver
func NewTemplateResolver(state *WorkflowState) *TemplateResolver {
	return &TemplateResolver{state: state}
}

// Resolve resolves a template expression
func (r *TemplateResolver) Resolve(expr string) (interface{}, error) {
	// If it doesn't look like a template, return as-is
	if !strings.Contains(expr, "{{") {
		return expr, nil
	}

	// Create template with custom functions
	tmpl, err := template.New("expr").Funcs(r.getFuncMap()).Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute template
	var buf bytes.Buffer
	data := r.getTemplateData()
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	result := buf.String()
	
	// Try to parse as JSON if it looks like JSON
	if strings.HasPrefix(strings.TrimSpace(result), "{") || strings.HasPrefix(strings.TrimSpace(result), "[") {
		var jsonResult interface{}
		if err := json.Unmarshal([]byte(result), &jsonResult); err == nil {
			return jsonResult, nil
		}
	}

	return result, nil
}

func (r *TemplateResolver) getFuncMap() template.FuncMap {
	return template.FuncMap{
		"json": func(v interface{}) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
		"join": func(sep string, items interface{}) string {
			switch v := items.(type) {
			case []string:
				return strings.Join(v, sep)
			case []interface{}:
				strs := make([]string, len(v))
				for i, item := range v {
					strs[i] = fmt.Sprintf("%v", item)
				}
				return strings.Join(strs, sep)
			default:
				return fmt.Sprintf("%v", items)
			}
		},
	}
}

func (r *TemplateResolver) getTemplateData() map[string]interface{} {
	data := make(map[string]interface{})
	
	// Add inputs
	data["Inputs"] = r.state.Inputs
	
	// Add steps with nested structure
	steps := make(map[string]interface{})
	for stepID, stepResult := range r.state.Steps {
		steps[stepID] = map[string]interface{}{
			"outputs": stepResult.Outputs,
		}
	}
	data["Steps"] = steps
	
	// Add environment variables (placeholder for now)
	data["Env"] = map[string]string{}
	
	// Add context (placeholder for now)
	data["Context"] = map[string]interface{}{
		"workflowID": "workflow-id",
		"runID":      "run-id",
	}
	
	return data
}

// ResolveStepOutput resolves a specific step output reference
func (r *TemplateResolver) ResolveStepOutput(stepID, outputName string) (interface{}, error) {
	step, ok := r.state.Steps[stepID]
	if !ok {
		return nil, fmt.Errorf("step %s not found", stepID)
	}
	
	output, ok := step.Outputs[outputName]
	if !ok {
		return nil, fmt.Errorf("output %s not found in step %s", outputName, stepID)
	}
	
	return output, nil
}