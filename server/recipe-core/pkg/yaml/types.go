package yaml

import (
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/cel"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/types"
)

// InputSchema defines the schema for an input parameter
type InputSchema struct {
	Type        string      `yaml:"type,omitempty"`        // Type of the input (string, number, boolean, etc.)
	Description string      `yaml:"description,omitempty"` // Description of the input
	Required    bool        `yaml:"required,omitempty"`    // Whether the input is required
	Default     interface{} `yaml:"default,omitempty"`     // Default value if not provided
}

type Node struct {
	ID   string `yaml:"id,omitempty" jsonschema:"oneof_required=sequence,op,parallel,state"`
	Desc string `yaml:"desc,omitempty" jsonschema:"oneof_required=sequence,op,parallel,state"`

	// Root node - embedded directly (one of these four)
	Op       string    `yaml:"op,omitempty" jsonschema:"oneof_required=op"`             // Operation node
	Sequence []Node    `yaml:"sequence,omitempty" jsonschema:"oneof_required=sequence"` // Sequence node
	Parallel []Node    `yaml:"parallel,omitempty" jsonschema:"oneof_required=parallel"` // Parallel node
	States   *StateMap `yaml:"states,omitempty" jsonschema:"oneof_required=state"`      // State machine node
	Shared   string    `yaml:"shared,omitempty"  jsonschema:"oneof_required=shared"`    // Reference to shared node

	// Node properties that apply at root
	Inputs  map[string]interface{} `yaml:"inputs,omitempty" jsonschema:"oneof_required=sequence,op,parallel,state"`
	Outputs map[string]interface{} `yaml:"outputs,omitempty" jsonschema:"oneof_required=sequence,parallel,state"`
	Timeout types.Duration         `yaml:"timeout,omitempty" jsonschema:"oneof_required=sequence,op,parallel,state"`
	Retry   *RetryPolicy           `yaml:"retry,omitempty" jsonschema:"oneof_required=sequence,op,parallel,state"`
	When    cel.CELExpr            `yaml:"when,omitempty" jsonschema:"oneof_required=sequence,op,parallel,state"`
}

// RecipeDefinition represents the complete unified recipe YAML structure
// The recipe itself IS the root node with embedded node properties
type RecipeDefinition struct {
	Node    `yaml:",inline"`
	Version string `yaml:"version"`

	// Shared node definitions
	Defs        map[string]Node        `yaml:"defs,omitempty"`         // Shared node definitions
	InputSchema map[string]InputSchema `yaml:"input_schema,omitempty"` // Optional schema for inputs
}

// StateMap represents a state machine configuration
type StateMap struct {
	Initial string           `yaml:"initial"` // Which state to start with
	States  map[string]State `yaml:",inline"` // Inline the states
}

// State represents a state in a state machine
// A state is a node plus transition information
type State struct {
	Node `yaml:",inline"`
	// State-specific fields
	Transitions []Transition `yaml:"transitions,omitempty"`
	Error       string       `yaml:"error,omitempty"` // For terminal error states
}

// Transition represents a state transition
type Transition struct {
	To   string      `yaml:"to"`
	When cel.CELExpr `yaml:"when,omitempty"` // CEL expression
}

// RetryPolicy represents a retry configuration that can be serialized to/from YAML
// This struct mirrors Temporal's RetryPolicy but without any Temporal dependencies
type RetryPolicy struct {
	InitialInterval        types.Duration `yaml:"initial_interval,omitempty"`
	BackoffCoefficient     float64        `yaml:"backoff_coefficient,omitempty"`
	MaximumInterval        types.Duration `yaml:"maximum_interval,omitempty"`
	MaximumAttempts        int32          `yaml:"maximum_attempts,omitempty"`
	NonRetryableErrorTypes []string       `yaml:"non_retryable_error_types,omitempty"`
}
