package schema

// SchemaType provides discriminated union support for operations
type SchemaType interface {
	SchemaDiscriminator() string // Returns the const value for "op" field
	SchemaInputs() interface{}   // Returns the input struct
	SchemaOutputs() interface{}  // Returns the output struct
}

// SchemaProvider allows types to customize their schema generation
type SchemaProvider interface {
	ProvideSchema() map[string]interface{}
}

// SchemaEnhancer allows adding schema metadata to generated schemas
type SchemaEnhancer interface {
	EnhanceSchema(schema map[string]interface{})
}

// Duration with built-in validation patterns
type Duration string

func (d Duration) SchemaPattern() string {
	return "^[0-9]+(s|m|h)$"
}

func (d Duration) SchemaDescription() string {
	return "Duration string (e.g., '30s', '5m', '1h')"
}

func (d Duration) SchemaType() string {
	return "string"
}

// RetryPolicy defines retry behavior for operations
type RetryPolicy struct {
	MaxAttempts        int      `json:"max_attempts" min:"1" required:"true" description:"Maximum number of retry attempts"`
	InitialInterval    Duration `json:"initial_interval" required:"true" description:"Initial retry interval"`
	BackoffCoefficient float64  `json:"backoff_coefficient,omitempty" min:"1" default:"2.0" description:"Exponential backoff coefficient"`
	MaxInterval        Duration `json:"max_interval,omitempty" description:"Maximum retry interval"`
}

// InputDef defines an input parameter schema
type InputDef struct {
	Type        string      `json:"type" required:"true" description:"Type of the input (string, number, boolean, etc.)"`
	Description string      `json:"description,omitempty" description:"Description of the input"`
	Required    bool        `json:"required,omitempty" description:"Whether the input is required"`
	Default     interface{} `json:"default,omitempty" description:"Default value if not provided"`
}

// Transition defines a state machine transition
type Transition struct {
	To   string `json:"to" required:"true" description:"Target state name"`
	When string `json:"when,omitempty" description:"CEL expression condition for transition"`
}

// WorkflowNode represents any node in a workflow (operation, sequence, parallel, etc.)
type WorkflowNode struct {
	// Common fields
	ID      string       `json:"id,omitempty" description:"Unique identifier for the node"`
	Desc    string       `json:"desc,omitempty" description:"Human-readable description"`
	When    string       `json:"when,omitempty" description:"CEL expression for conditional execution"`
	Timeout Duration     `json:"timeout,omitempty" description:"Timeout duration"`
	Retry   *RetryPolicy `json:"retry,omitempty" description:"Retry policy"`

	// Node inputs and outputs
	Inputs  map[string]interface{} `json:"inputs,omitempty" description:"Node input values"`
	Outputs map[string]interface{} `json:"outputs,omitempty" description:"Node output mappings"`

	// Exactly one of these should be set (enforced by validation)
	Operation *NodeOperation `json:"-" oneOf:"true"`
	Sequence  *SequenceNode  `json:"-" oneOf:"true"`
	Parallel  *ParallelNode  `json:"-" oneOf:"true"`
	States    *StateNode     `json:"-" oneOf:"true"`
	SharedRef *string        `json:"shared,omitempty" oneOf:"true" description:"Reference to a shared node definition"`
}

// SequenceNode executes nodes sequentially
type SequenceNode struct {
	Nodes []WorkflowNode `json:"sequence" minItems:"1" description:"Sequential execution of nodes"`
}

// ParallelNode executes nodes in parallel
type ParallelNode struct {
	Nodes []WorkflowNode `json:"parallel" minItems:"1" description:"Parallel execution of nodes"`
}

// StateNode implements a state machine
type StateNode struct {
	Initial string            `json:"initial" required:"true" description:"Initial state to start execution"`
	States  map[string]*State `json:"states" required:"true" description:"State definitions"`
}

// State represents a single state in a state machine
type State struct {
	WorkflowNode              // Embed node fields
	Transitions  []Transition `json:"transitions,omitempty" description:"State transitions"`
	Error        string       `json:"error,omitempty" description:"Error message for terminal error states"`
}

// Recipe defines a complete workflow recipe
type Recipe struct {
	Name        string                  `json:"name" required:"true" description:"Name of the recipe"`
	Version     string                  `json:"version" default:"1.0" description:"Version of the recipe"`
	Description string                  `json:"description,omitempty" description:"Human-readable description of what the recipe does"`
	InputSchema map[string]InputDef     `json:"input_schema,omitempty" description:"Schema definitions for recipe inputs"`
	Shared      map[string]WorkflowNode `json:"defs,omitempty" description:"Shared node definitions that can be referenced"`
	
	// Root node - embed WorkflowNode fields
	WorkflowNode `json:",inline"`
}

// NodeOperation represents a discriminated union of all operation types
type NodeOperation struct {
	// Exactly one of these should be set
	Sleep           *SleepOperation            `json:"-" op:"sleep"`
	Command         *CommandExecutionOperation `json:"-" op:"command_execution"`
	LLM             *LLMInferenceOperation     `json:"-" op:"llm_inference"`
	GitShallowClone *GitShallowCloneOperation  `json:"-" op:"git_shallow_clone"`
	Recipe          *RecipeOperation           `json:"-" op:"recipe"`
	Input           *InputOperation            `json:"-" op:"input"`
}

// GetOperationType returns the operation type discriminator
func (n *NodeOperation) GetOperationType() string {
	if n.Sleep != nil {
		return "sleep"
	}
	if n.Command != nil {
		return "command_execution"
	}
	if n.LLM != nil {
		return "llm_inference"
	}
	if n.GitShallowClone != nil {
		return "git_shallow_clone"
	}
	if n.Recipe != nil {
		return "recipe"
	}
	if n.Input != nil {
		return "input"
	}
	return ""
}

// GetOperation returns the active operation
func (n *NodeOperation) GetOperation() SchemaType {
	if n.Sleep != nil {
		return n.Sleep
	}
	if n.Command != nil {
		return n.Command
	}
	if n.LLM != nil {
		return n.LLM
	}
	if n.GitShallowClone != nil {
		return n.GitShallowClone
	}
	if n.Recipe != nil {
		return n.Recipe
	}
	if n.Input != nil {
		return n.Input
	}
	return nil
}
