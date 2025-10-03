package compiler

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
)

// TemplateData is the root context for both Go templates and CEL
type TemplateData struct {
	Inputs   map[string]interface{} `json:"inputs"`   // Current scope inputs
	Sequence map[string]NodeOutput  `json:"sequence"` // Sibling nodes in sequence
	States   map[string]StateOutput `json:"states"`   // Completed states in state machine
	Scope    ScopeMetadata          `json:"scope"`    // Execution metadata
	Context  map[string]interface{} `json:"context"`  // Execution context (git, worktree, etc.)
	// Note: No unqualified "outputs" field - outputs always qualified by context
}

// NodeOutput represents a node's execution output
type NodeOutput struct {
	Outputs map[string]interface{} `json:"outputs"`
	Runs    []RunOutput            `json:"runs"` // Previous runs (retries or loops)
}

// StateOutput represents a state's execution output
type StateOutput struct {
	Outputs map[string]interface{} `json:"outputs"`
	Runs    []RunOutput            `json:"runs"` // Previous runs (state loops or retries)
}

// RunOutput represents a single execution run
type RunOutput struct {
	Outputs   map[string]interface{} `json:"outputs"`
	RunID     string                 `json:"run_id"`
	Timestamp time.Time              `json:"timestamp"`
}

// ScopeMetadata contains execution context metadata
type ScopeMetadata struct {
	ExecutionID string    `json:"execution_id"`
	Timestamp   time.Time `json:"timestamp"`
	// Note: No attempts/runs counter - ambiguous which level
}

// ResolutionContext represents template resolution context
type ResolutionContext struct {
	// Scope type: "root", "sequence", "state_machine", "state"
	ScopeType string

	// Parent context (nil for root)
	Parent *ResolutionContext

	// Template data for current scope
	TemplateData TemplateData

	// CEL environment for when expressions
	CELEnv *cel.Env

	// Metadata
	ScopeID      string
	CurrentRunID string // Track current run for this scope
}

// NewResolutionContext creates a new resolution context
func NewResolutionContext(scopeType string, scopeID string) (*ResolutionContext, error) {
	rc := &ResolutionContext{
		ScopeType: scopeType,
		ScopeID:   scopeID,
		TemplateData: TemplateData{
			Inputs:   make(map[string]interface{}),
			Sequence: make(map[string]NodeOutput),
			States:   make(map[string]StateOutput),
			Context:  make(map[string]interface{}),
			Scope: ScopeMetadata{
				ExecutionID: generateExecutionID(),
				Timestamp:   time.Now(),
			},
		},
	}

	// Initialize CEL environment
	env, err := cel.NewEnv(
		cel.Variable("inputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("sequence", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("states", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("scope", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("context", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	rc.CELEnv = env

	return rc, nil
}

// NewChildContext creates a child resolution context
func (rc *ResolutionContext) NewChildContext(scopeType string, scopeID string, inputs map[string]interface{}) (*ResolutionContext, error) {
	child, err := NewResolutionContext(scopeType, scopeID)
	if err != nil {
		return nil, err
	}

	child.Parent = rc
	child.SetInputs(inputs)

	// Child can see parent's states if in state machine
	if rc.ScopeType == "state_machine" || rc.ScopeType == "state" {
		child.TemplateData.States = rc.TemplateData.States
	}

	return child, nil
}

// SetInputs updates the resolution context inputs and synchronizes execution context shortcuts.
func (rc *ResolutionContext) SetInputs(inputs map[string]interface{}) {
	rc.TemplateData.Inputs = inputs
	if ctxMap, ok := inputs["context"].(map[string]interface{}); ok {
		rc.TemplateData.Context = ctxMap
	} else {
		rc.TemplateData.Context = map[string]interface{}{}
	}
}

// ResolveTemplate handles expression evaluation using CEL with interpolation support
func (rc *ResolutionContext) ResolveTemplate(expr string) (interface{}, error) {
	// Use the new interpolation mode by default for backward compatibility
	return rc.interpolateString(expr, ModeInterpolation)
}

// EvaluateCEL handles pure CEL evaluation for when conditions
func (rc *ResolutionContext) EvaluateCEL(expr string) (bool, error) {
	if expr == "" || strings.ToLower(expr) == "true" {
		return true, nil
	}

	ast, issues := rc.CELEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("failed to compile CEL expression: %w", issues.Err())
	}

	program, err := rc.CELEnv.Program(ast)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL program: %w", err)
	}

	// Pass TemplateData fields as CEL variables
	result, _, err := program.Eval(map[string]interface{}{
		"inputs":   rc.TemplateData.Inputs,
		"sequence": convertNodeOutputsForCEL(rc.TemplateData.Sequence),
		"states":   convertStateOutputsForCEL(rc.TemplateData.States),
		"scope":    convertScopeForCEL(rc.TemplateData.Scope),
		"context":  rc.TemplateData.Context,
	})
	if err != nil {
		return false, fmt.Errorf("failed to evaluate CEL expression: %w", err)
	}

	// Convert result to bool
	boolVal, ok := result.Value().(bool)
	if !ok {
		return false, fmt.Errorf("CEL expression did not evaluate to boolean: got %T", result.Value())
	}

	return boolVal, nil
}

// EvaluateCELExpression evaluates a CEL expression and returns any type
func (rc *ResolutionContext) EvaluateCELExpression(expr string) (interface{}, error) {
	// Handle empty expressions
	if expr == "" {
		return "", nil
	}

	ast, issues := rc.CELEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("failed to compile CEL expression: %w", issues.Err())
	}

	program, err := rc.CELEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL program: %w", err)
	}

	// Pass TemplateData fields as CEL variables
	result, _, err := program.Eval(map[string]interface{}{
		"inputs":   rc.TemplateData.Inputs,
		"sequence": convertNodeOutputsForCEL(rc.TemplateData.Sequence),
		"states":   convertStateOutputsForCEL(rc.TemplateData.States),
		"scope":    convertScopeForCEL(rc.TemplateData.Scope),
		"context":  rc.TemplateData.Context,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate CEL expression: %w", err)
	}

	return result.Value(), nil
}

// ResolveValue recursively resolves templates in a value (uses interpolation mode by default)
func (rc *ResolutionContext) ResolveValue(value interface{}) (interface{}, error) {
	return rc.ResolveValueWithMode(value, ModeInterpolation)
}

// AddSequenceNode adds a node output to the sequence context
func (rc *ResolutionContext) AddSequenceNode(nodeID string, outputs map[string]interface{}) {
	if rc.TemplateData.Sequence == nil {
		rc.TemplateData.Sequence = make(map[string]NodeOutput)
	}

	// If node already exists, add to runs history
	if existing, ok := rc.TemplateData.Sequence[nodeID]; ok {
		existing.Runs = append(existing.Runs, RunOutput{
			Outputs:   existing.Outputs,
			RunID:     generateRunID(),
			Timestamp: time.Now(),
		})
		existing.Outputs = outputs
		rc.TemplateData.Sequence[nodeID] = existing
	} else {
		rc.TemplateData.Sequence[nodeID] = NodeOutput{
			Outputs: outputs,
			Runs:    []RunOutput{},
		}
	}
}

// AddStateOutput adds a state output to the state machine context
func (rc *ResolutionContext) AddStateOutput(stateID string, outputs map[string]interface{}) {
	if rc.TemplateData.States == nil {
		rc.TemplateData.States = make(map[string]StateOutput)
	}

	// If state already exists, add to runs history
	if existing, ok := rc.TemplateData.States[stateID]; ok {
		existing.Runs = append(existing.Runs, RunOutput{
			Outputs:   existing.Outputs,
			RunID:     generateRunID(),
			Timestamp: time.Now(),
		})
		existing.Outputs = outputs
		rc.TemplateData.States[stateID] = existing
	} else {
		rc.TemplateData.States[stateID] = StateOutput{
			Outputs: outputs,
			Runs:    []RunOutput{},
		}
	}
}

// ValidateTemplateReferences validates all template references before execution
func (rc *ResolutionContext) ValidateTemplateReferences(expr string) error {
	// Check if it's a template expression
	trimmed := strings.TrimSpace(expr)
	if !strings.HasPrefix(trimmed, "{{") || !strings.HasSuffix(trimmed, "}}") {
		return nil // Not a template
	}

	// Extract and validate the CEL expression
	innerExpr := strings.TrimSpace(trimmed[2 : len(trimmed)-2])
	return rc.ValidateCELExpression(innerExpr)
}

// ValidateCELExpression validates a CEL expression
func (rc *ResolutionContext) ValidateCELExpression(expr string) error {
	if expr == "" || strings.ToLower(expr) == "true" {
		return nil
	}

	_, issues := rc.CELEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("invalid CEL expression: %w", issues.Err())
	}

	return nil
}

// Helper functions for CEL type conversion
func convertNodeOutputsForCEL(nodes map[string]NodeOutput) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range nodes {
		nodeData := map[string]interface{}{
			"outputs": v.Outputs,
		}
		if len(v.Runs) > 0 {
			runs := make([]interface{}, len(v.Runs))
			for i, run := range v.Runs {
				runs[i] = map[string]interface{}{
					"outputs":   run.Outputs,
					"run_id":    run.RunID,
					"timestamp": run.Timestamp.Format(time.RFC3339),
				}
			}
			nodeData["runs"] = runs
		}
		result[k] = nodeData
	}
	return result
}

func convertStateOutputsForCEL(states map[string]StateOutput) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range states {
		stateData := map[string]interface{}{
			"outputs": v.Outputs,
		}
		if len(v.Runs) > 0 {
			runs := make([]interface{}, len(v.Runs))
			for i, run := range v.Runs {
				runs[i] = map[string]interface{}{
					"outputs":   run.Outputs,
					"run_id":    run.RunID,
					"timestamp": run.Timestamp.Format(time.RFC3339),
				}
			}
			stateData["runs"] = runs
		}
		result[k] = stateData
	}
	return result
}

func convertScopeForCEL(scope ScopeMetadata) map[string]interface{} {
	return map[string]interface{}{
		"execution_id": scope.ExecutionID,
		"timestamp":    scope.Timestamp.Format(time.RFC3339),
	}
}

// Helper functions for ID generation
func generateExecutionID() string {
	return fmt.Sprintf("exec-%d", time.Now().UnixNano())
}

func generateRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}
