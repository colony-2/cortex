package statemachine

import (
	"fmt"

	yamlpkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/yaml"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// evaluateCEL evaluates a CEL expression with the given context
func (s *StateMachineCompiler) evaluateCEL(expression string, outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) (bool, error) {
	// Parse the expression
	ast, issues := s.celEnv.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("failed to compile CEL expression '%s': %w", expression, issues.Err())
	}

	// Create the program
	prg, err := s.celEnv.Program(ast)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL program: %w", err)
	}

	// Prepare CEL variables
	celVars := s.prepareCELVariables(outputs, stateCtx)

	// Evaluate the expression
	out, _, err := prg.Eval(celVars)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate CEL expression: %w", err)
	}

	// Convert result to boolean
	switch v := out.(type) {
	case types.Bool:
		return bool(v), nil
	case ref.Val:
		if boolVal, ok := v.Value().(bool); ok {
			return boolVal, nil
		}
	}

	return false, fmt.Errorf("CEL expression did not evaluate to boolean: got %T", out)
}

// prepareCELVariables prepares variables for CEL evaluation
func (s *StateMachineCompiler) prepareCELVariables(outputs map[string]interface{}, stateCtx *yamlpkg.StateContext) map[string]interface{} {
	vars := make(map[string]interface{})

	// Current outputs
	if outputs != nil {
		vars["Outputs"] = outputs
	} else {
		vars["Outputs"] = make(map[string]interface{})
	}

	// State information
	if stateCtx.StateInfo != nil && stateCtx.CurrentState != "" {
		if info, exists := stateCtx.StateInfo[stateCtx.CurrentState]; exists {
			vars["State"] = map[string]interface{}{
				"name":       info.Name,
				"attempts":   info.Attempts,
				"entered_at": info.EnteredAt,
			}
		}
	}

	// Previous state outputs
	if stateCtx.StateOutputs != nil {
		vars["States"] = stateCtx.StateOutputs
	} else {
		vars["States"] = make(map[string]interface{})
	}

	// Current inputs
	if stateCtx.Inputs != nil {
		vars["Inputs"] = stateCtx.Inputs
	} else {
		vars["Inputs"] = make(map[string]interface{})
	}

	// Recipe context
	if stateCtx.RecipeContext != nil {
		vars["Context"] = map[string]interface{}{
			"recipe": map[string]interface{}{
				"name":         stateCtx.RecipeContext.Recipe.Name,
				"version":      stateCtx.RecipeContext.Recipe.Version,
				"execution_id": stateCtx.RecipeContext.Recipe.ExecutionID,
			},
			"environment": map[string]interface{}{
				"name":   stateCtx.RecipeContext.Environment.Name,
				"region": stateCtx.RecipeContext.Environment.Region,
			},
			"execution": map[string]interface{}{
				"started_at": stateCtx.RecipeContext.Execution.StartedAt,
				"timeout":    stateCtx.RecipeContext.Execution.Timeout,
			},
		}
	} else {
		vars["Context"] = make(map[string]interface{})
	}

	// Step outputs (within current state)
	if stateCtx.StepOutputs != nil {
		vars["Steps"] = stateCtx.StepOutputs
	} else {
		vars["Steps"] = make(map[string]interface{})
	}

	return vars
}