package template

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// ScopeType defines resolver scope kinds.
type ScopeType string

const (
	ScopeRecipe       ScopeType = "recipe"
	ScopeSequence     ScopeType = "sequence"
	ScopeStateMachine ScopeType = "state_machine"
	ScopeState        ScopeType = "state"
	ScopeOp           ScopeType = "op"
)

// templateData is the root context for both Go templates and CEL
type templateData struct {
	ContainerInputs map[string]interface{}          `json:"container_inputs"` // ContainerInputs map if this is a sequence or state machine.
	Sequence        map[string]StepOutput           `json:"sequence"`         // Sibling nodes in sequence
	States          map[string]StepOutput           `json:"states"`           // Completed states in state machine
	Scope           ScopeMetadata                   `json:"scope"`            // Execution metadata
	Context         contextual.TaskExecutionContext `json:"context"`          // Execution context (typed)
	// Note: No unqualified "outputs" field - outputs always qualified by context
}

type StepOutput struct {
	Outputs map[string]interface{} `json:"outputs"`
	Runs    []RunOutput            `json:"runs"` // Previous runs (state loops)
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
	commitContext *contextual.GitCommitContext

	// Scope type: "root", "sequence", "state_machine", "state"
	ScopeType ScopeType

	scopeId string

	// Parent context (nil for root)
	Parent *ResolutionContext

	// Template data for current scope
	TemplateData templateData

	// CEL environment for when expressions
	CELEnv *cel.Env

	CurrentRunID string // Track current run for this scope

	tracker *invocationTracker

	lastExecution map[string]interface{}
}

func (rc *ResolutionContext) UpdateGitState(parentHash string, persistHash string) {
	rc.commitContext.ParentHash = parentHash
	rc.commitContext.PersistHash = persistHash
}

func (rc *ResolutionContext) GetGitCommitContext() contextual.GitCommitContext {
	return *rc.commitContext
}

// NewRecipeResolutionContext creates a new resolution context for a recipe
func NewRecipeResolutionContext(commitContext *contextual.GitCommitContext, recipeInputs map[string]interface{}, execCtx contextual.JobContext) (*ResolutionContext, error) {
	tracker := newInvocationTracker()

	return newResolutionContext(commitContext, tracker, ScopeRecipe, "", recipeInputs, execCtx)
}

func newResolutionContext(commitContext *contextual.GitCommitContext, tracker *invocationTracker, scopeType ScopeType, scopeId string, containerInputs map[string]interface{}, execCtx contextual.JobContext) (*ResolutionContext, error) {
	rc := &ResolutionContext{
		commitContext: commitContext,
		ScopeType:     scopeType,
		tracker:       tracker,
		scopeId:       scopeId,
		TemplateData: templateData{

			ContainerInputs: containerInputs,
			Sequence:        make(map[string]StepOutput),
			States:          make(map[string]StepOutput),
			Scope: ScopeMetadata{
				ExecutionID: generateExecutionID(),
				Timestamp:   time.Now(),
			},
			Context: contextual.TaskExecutionContext{
				JobContext: execCtx,
				TaskContext: contextual.TaskContext{
					Invocation: tracker.nextInvocation(),
					GitCommit:  commitContext,
				},
			},
		},
	}

	// Initialize CEL environment
	env, err := cel.NewEnv(
		cel.Variable("inputs", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("sequence", cel.MapType(cel.StringType, cel.MapType(cel.StringType, cel.DynType))),
		cel.Variable("states", cel.MapType(cel.StringType, cel.MapType(cel.StringType, cel.DynType))),
		cel.Variable("scope", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("context", cel.MapType(cel.StringType, cel.DynType)),
		ext.NativeTypes(
			reflect.TypeOf(StepOutput{}),
			reflect.TypeOf(RunOutput{}),
			ext.ParseStructTag("json"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	rc.CELEnv = env
	rc.ensureContextBackfill()

	return rc, nil
}

func (rc *ResolutionContext) TaskExecutionContext() contextual.TaskExecutionContext {
	return rc.TemplateData.Context
}

func scopeId(meta recipe.NodeMetadata, fallback string, scopeType ScopeType) string {
	if meta.ID != "" {
		return meta.ID
	} else if fallback != "" {
		return fallback
	} else {
		switch scopeType {
		case ScopeOp:
			return "op"
		case ScopeState:
			return "state"
		case ScopeRecipe:
			return "recipe"
		case ScopeStateMachine:
			return "state_machine"
		case ScopeSequence:
			return "sequence"
		default:
			panic(fmt.Sprintf("invalid scope type: %s", scopeType))
		}
	}
}

// NewChildContext creates a child resolution context
func (rc *ResolutionContext) NewChildContext(scopeType ScopeType, metadata recipe.NodeMetadata, fallback string, inputs map[string]interface{}) (*ResolutionContext, error) {
	switch scopeType {
	case ScopeRecipe, ScopeSequence, ScopeStateMachine:
		if inputs == nil {
			return nil, fmt.Errorf("inputs cannot be nil for root or sequence scope")
		}
	default:
		if inputs != nil {
			return nil, fmt.Errorf("inputs cannot be set for state or op scope")
		}
	}

	scopeId := scopeId(metadata, fallback, scopeType)
	child, err := newResolutionContext(rc.commitContext, rc.tracker.child(scopeId), scopeType, scopeId, inputs, rc.TaskExecutionContext().JobContext)
	if err != nil {
		return nil, err
	}

	// copy items from parents based on scope.
	switch scopeType {
	case ScopeSequence, ScopeRecipe, ScopeStateMachine:
		// new scope, copy nothing down.

	case ScopeState:
		// child state can see the whole state machine's state and the state machine inputs.
		// containers are not cloned because we want to be able to add to them and have parent context see where appropriate.
		child.TemplateData.ContainerInputs = rc.TemplateData.ContainerInputs
		child.TemplateData.States = rc.TemplateData.States
	case ScopeOp:
		// op can see their container's inputs as well as states and sequences of their container.
		// containers are not cloned because we want to be able to add to them and have parent context see where appropriate.
		child.TemplateData.ContainerInputs = rc.TemplateData.ContainerInputs
		child.TemplateData.States = rc.TemplateData.States
		child.TemplateData.Sequence = rc.TemplateData.Sequence
	}

	child.Parent = rc
	rc.ensureContextBackfill()
	return child, nil
}

// ensureContextBackfill keeps the template data context initialized even when callers omit it.
func (rc *ResolutionContext) ensureContextBackfill() {
	if rc.TemplateData.Sequence == nil {
		rc.TemplateData.Sequence = make(map[string]StepOutput)
	}
	if rc.TemplateData.States == nil {
		rc.TemplateData.States = make(map[string]StepOutput)
	}
	if rc.TemplateData.ContainerInputs == nil {
		rc.TemplateData.ContainerInputs = make(map[string]interface{})
	}
}

func (rc *ResolutionContext) ResolveMap(input map[string]interface{}) (map[string]interface{}, error) {
	resolved := make(map[string]interface{})
	for key, value := range input {
		resolvedValue, err := rc.resolveValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve input '%s': %w", key, err)
		}
		resolved[key] = resolvedValue
	}
	return resolved, nil
}

// resolveTemplate handles expression evaluation using CEL with interpolation support
func (rc *ResolutionContext) resolveTemplate(expr string) (interface{}, error) {
	// Use the new interpolation mode by default for backward compatibility
	return rc.interpolateString(expr, ModeInterpolation)
}

// EvaluateCEL handles pure CEL evaluation for when conditions
func (rc *ResolutionContext) EvaluateCEL(expr string) (bool, error) {
	if expr == "" || strings.ToLower(expr) == "true" {
		return true, nil
	}

	out, err := rc.evaluateCELExpression(expr)
	if err != nil {
		return false, err
	}
	boolOut, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("CEL expression did not return bool")
	}
	return boolOut, nil
}

// evaluateCELExpression evaluates a CEL expression and returns any type
func (rc *ResolutionContext) evaluateCELExpression(expr string) (interface{}, error) {
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

	// Pass templateData fields as CEL variables without coercing structs to maps
	result, _, err := program.Eval(map[string]interface{}{
		"inputs":   rc.TemplateData.ContainerInputs,
		"sequence": rc.TemplateData.Sequence,
		"states":   rc.TemplateData.States,
		"scope":    rc.TemplateData.Scope,
		"context":  rc.TemplateData.Context,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate CEL expression: %w", err)
	}

	return result.Value(), nil
}

// resolveValue recursively resolves templates in a value (uses interpolation mode by default)
func (rc *ResolutionContext) resolveValue(value interface{}) (interface{}, error) {
	return rc.ResolveValueWithMode(value, ModeInterpolation)
}

func (rc *ResolutionContext) AddExecution(output map[string]interface{}) {
	rc.lastExecution = output

	var container map[string]StepOutput

	switch rc.ScopeType {
	case ScopeSequence:
		container = rc.TemplateData.Sequence
	case ScopeStateMachine, ScopeState:
		container = rc.TemplateData.States
	case ScopeRecipe:
		// no context storage other than last execution needed.
		return
	case ScopeOp:
		switch rc.Parent.ScopeType {
		case ScopeSequence:
			container = rc.TemplateData.Sequence
		case ScopeStateMachine, ScopeState:
			container = rc.TemplateData.States
		case ScopeRecipe:
			rc.Parent.lastExecution = output
			return
		default:
			panic(fmt.Sprintf("invalid parent scope type: %s", rc.Parent.ScopeType))
		}
	default:
		panic(fmt.Sprintf("invalid parent scope type: %s", rc.Parent.ScopeType))
	}

	if existing, ok := container[rc.scopeId]; ok {
		existing.Runs = append(existing.Runs, RunOutput{
			Outputs:   existing.Outputs,
			RunID:     generateRunID(),
			Timestamp: time.Now(),
		})
		existing.Outputs = output
		container[rc.scopeId] = existing
	} else {
		container[rc.scopeId] = StepOutput{
			Outputs: output,
			Runs:    []RunOutput{},
		}
	}
}

func (rc *ResolutionContext) GetLastExecution() map[string]interface{} {
	return rc.lastExecution
}

// validateTemplateReferences validates all template references before execution
func (rc *ResolutionContext) validateTemplateReferences(expr string) error {
	// Check if it's a template expression
	trimmed := strings.TrimSpace(expr)
	if !strings.HasPrefix(trimmed, "{{") || !strings.HasSuffix(trimmed, "}}") {
		return nil // Not a template
	}

	// Extract and validate the CEL expression
	innerExpr := strings.TrimSpace(trimmed[2 : len(trimmed)-2])
	return rc.validateCELExpression(innerExpr)
}

// validateCELExpression validates a CEL expression
func (rc *ResolutionContext) validateCELExpression(expr string) error {
	if expr == "" || strings.ToLower(expr) == "true" {
		return nil
	}

	_, issues := rc.CELEnv.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("invalid CEL expression: %w", issues.Err())
	}

	return nil
}

// Helper functions for ID generation
func generateExecutionID() string {
	return fmt.Sprintf("exec-%d", time.Now().UnixNano())
}

func generateRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}
