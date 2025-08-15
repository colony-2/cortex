package shared

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
)

// CELEvaluator handles CEL expression evaluation
type CELEvaluator struct {
	env     *cel.Env
	cache   map[string]cel.Program
}

// NewCELEvaluator creates a new CEL evaluator with standard environment
func NewCELEvaluator() (*CELEvaluator, error) {
	// Create CEL environment with common types
	env, err := cel.NewEnv(
		cel.Declarations(
			// Common variables
			decls.NewVar("inputs", decls.NewMapType(decls.String, decls.Dyn)),
			decls.NewVar("outputs", decls.NewMapType(decls.String, decls.Dyn)),
			decls.NewVar("state", decls.NewMapType(decls.String, decls.Dyn)),
			decls.NewVar("nodes", decls.NewMapType(decls.String, decls.Dyn)),
			
			// Common scalar types
			decls.NewVar("score", decls.Int),
			decls.NewVar("threshold", decls.Int),
			decls.NewVar("status", decls.String),
			decls.NewVar("success", decls.Bool),
			decls.NewVar("error", decls.String),
			decls.NewVar("attempts", decls.Int),
			decls.NewVar("max_attempts", decls.Int),
			
			// Complex types
			decls.NewVar("data", decls.Dyn),
			decls.NewVar("result", decls.Dyn),
			decls.NewVar("items", decls.NewListType(decls.Dyn)),
			decls.NewVar("config", decls.NewMapType(decls.String, decls.Dyn)),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	
	return &CELEvaluator{
		env:   env,
		cache: make(map[string]cel.Program),
	}, nil
}

// Evaluate evaluates a CEL expression with the given data
func (ce *CELEvaluator) Evaluate(expression string, data map[string]interface{}) (interface{}, error) {
	// Check cache for compiled program
	prg, exists := ce.cache[expression]
	if !exists {
		// Parse expression
		ast, issues := ce.env.Parse(expression)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("failed to parse expression '%s': %w", expression, issues.Err())
		}
		
		// Check expression
		checked, issues := ce.env.Check(ast)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("failed to check expression '%s': %w", expression, issues.Err())
		}
		
		// Create program
		var err error
		prg, err = ce.env.Program(checked)
		if err != nil {
			return nil, fmt.Errorf("failed to create program for expression '%s': %w", expression, err)
		}
		
		// Cache the compiled program
		ce.cache[expression] = prg
	}
	
	// Evaluate with data
	out, _, err := prg.Eval(data)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate expression '%s': %w", expression, err)
	}
	
	// Return the native value
	return out.Value(), nil
}

// EvaluateBool evaluates a CEL expression and returns a boolean result
func (ce *CELEvaluator) EvaluateBool(expression string, data map[string]interface{}) (bool, error) {
	result, err := ce.Evaluate(expression, data)
	if err != nil {
		return false, err
	}
	
	// Convert to bool
	switch v := result.(type) {
	case bool:
		return v, nil
	default:
		// Try to convert to bool
		if b, ok := result.(bool); ok {
			return b, nil
		}
	}
	
	return false, fmt.Errorf("expression did not evaluate to boolean: got %T", result)
}

// EvaluateString evaluates a CEL expression and returns a string result
func (ce *CELEvaluator) EvaluateString(expression string, data map[string]interface{}) (string, error) {
	result, err := ce.Evaluate(expression, data)
	if err != nil {
		return "", err
	}
	
	// Convert to string
	switch v := result.(type) {
	case string:
		return v, nil
	default:
		// Try to convert other types to string
		return fmt.Sprintf("%v", result), nil
	}
}

// EvaluateTemplate evaluates a template string with CEL expressions
func (ce *CELEvaluator) EvaluateTemplate(template string, data map[string]interface{}) (string, error) {
	// Simple implementation - in production would use proper template parser
	// Look for {{ expression }} patterns
	
	result := template
	// This is simplified - real implementation would properly parse templates
	if len(template) > 4 && template[:2] == "{{" && template[len(template)-2:] == "}}" {
		expression := template[2 : len(template)-2]
		// Trim spaces
		for len(expression) > 0 && expression[0] == ' ' {
			expression = expression[1:]
		}
		for len(expression) > 0 && expression[len(expression)-1] == ' ' {
			expression = expression[len(expression)-1:]
		}
		
		evaluated, err := ce.EvaluateString(expression, data)
		if err != nil {
			return template, err
		}
		return evaluated, nil
	}
	
	return result, nil
}

// ValidateExpression validates a CEL expression without evaluating it
func (ce *CELEvaluator) ValidateExpression(expression string) error {
	// Parse expression
	ast, issues := ce.env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("invalid expression '%s': %w", expression, issues.Err())
	}
	
	// Check expression
	_, issues = ce.env.Check(ast)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("expression check failed for '%s': %w", expression, issues.Err())
	}
	
	return nil
}

// CreateCustomEnvironment creates a CEL environment with custom variable declarations
func CreateCustomEnvironment(variables map[string]interface{}) (*cel.Env, error) {
	envOpts := []cel.EnvOption{}
	
	for name := range variables {
		envOpts = append(envOpts, cel.Declarations(
			decls.NewVar(name, decls.Dyn),
		))
	}
	
	return cel.NewEnv(envOpts...)
}

// EvaluateWithCustomEnv evaluates an expression with a custom environment
func EvaluateWithCustomEnv(env *cel.Env, expression string, data map[string]interface{}) (interface{}, error) {
	// Parse expression
	ast, issues := env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	
	// Check expression
	checked, issues := env.Check(ast)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	
	// Create program
	prg, err := env.Program(checked)
	if err != nil {
		return nil, err
	}
	
	// Evaluate
	out, _, err := prg.Eval(data)
	if err != nil {
		return nil, err
	}
	
	return out.Value(), nil
}

// ConvertToActivation converts a map to CEL activation (variable bindings)
func ConvertToActivation(data map[string]interface{}) map[string]interface{} {
	activation := make(map[string]interface{})
	
	for key, value := range data {
		activation[key] = convertValue(value)
	}
	
	return activation
}

// convertValue converts Go values to CEL-compatible types
func convertValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Map:
		m := make(map[string]interface{})
		for _, key := range v.MapKeys() {
			m[key.String()] = convertValue(v.MapIndex(key).Interface())
		}
		return m
	case reflect.Slice, reflect.Array:
		var list []interface{}
		for i := 0; i < v.Len(); i++ {
			list = append(list, convertValue(v.Index(i).Interface()))
		}
		return list
	default:
		return value
	}
}

// StandardFunctions returns a set of standard CEL functions for recipes
func StandardFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("len",
			cel.Overload("len_string", []*cel.Type{cel.StringType}, cel.IntType),
			cel.Overload("len_list", []*cel.Type{cel.ListType(cel.DynType)}, cel.IntType),
			cel.Overload("len_map", []*cel.Type{cel.MapType(cel.StringType, cel.DynType)}, cel.IntType),
		),
		cel.Function("contains",
			cel.Overload("contains_string", []*cel.Type{cel.StringType, cel.StringType}, cel.BoolType),
			cel.Overload("contains_list", []*cel.Type{cel.ListType(cel.DynType), cel.DynType}, cel.BoolType),
		),
		cel.Function("matches",
			cel.Overload("matches_string", []*cel.Type{cel.StringType, cel.StringType}, cel.BoolType),
		),
	}
}

// EvaluateTransitionCondition evaluates a state transition condition
func (ce *CELEvaluator) EvaluateTransitionCondition(condition string, stateOutputs map[string]interface{}, globalState map[string]interface{}) (bool, error) {
	// Prepare data for evaluation
	data := map[string]interface{}{
		"outputs": stateOutputs,
		"state":   globalState,
	}
	
	// Add individual output fields for easier access
	for key, value := range stateOutputs {
		data[key] = value
	}
	
	return ce.EvaluateBool(condition, data)
}

// EvaluateRetryCondition evaluates a retry condition
func (ce *CELEvaluator) EvaluateRetryCondition(condition string, error error, attempts int, maxAttempts int) (bool, error) {
	data := map[string]interface{}{
		"error":        error != nil,
		"attempts":     attempts,
		"max_attempts": maxAttempts,
	}
	
	if error != nil {
		data["error_message"] = error.Error()
	}
	
	return ce.EvaluateBool(condition, data)
}