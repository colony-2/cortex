package yaml

// InputDef defines the schema for an input parameter
type InputDef struct {
	Type        string      `yaml:"type,omitempty"`        // Type of the input (string, number, boolean, etc.)
	Description string      `yaml:"description,omitempty"` // Description of the input
	Required    bool        `yaml:"required,omitempty"`    // Whether the input is required
	Default     interface{} `yaml:"default,omitempty"`     // Default value if not provided
}

// RecipeDefinition represents the complete unified recipe YAML structure
// The recipe itself IS the root node with embedded node properties
type RecipeDefinition struct {
	// Metadata
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Version     string `yaml:"version"`
	
	// Shared node definitions
	Shared map[string]Node `yaml:"shared,omitempty"` // Shared node definitions
	
	// Root node - embedded directly (one of these four)
	Op       string      `yaml:"op,omitempty"`       // Operation node
	Sequence []Node      `yaml:"sequence,omitempty"` // Sequence node  
	Parallel []Node      `yaml:"parallel,omitempty"` // Parallel node
	States   *StateMap   `yaml:"states,omitempty"`   // State machine node
	
	// Node properties that apply at root
	ID          string                 `yaml:"id,omitempty"`
	Desc        string                 `yaml:"desc,omitempty"`
	Inputs      map[string]interface{} `yaml:"inputs,omitempty"`  // Root node inputs (new format)
	Outputs     map[string]interface{} `yaml:"outputs,omitempty"` // Root node outputs (new format) 
	InputSchema map[string]InputDef    `yaml:"input_schema,omitempty"` // Optional schema for inputs
	Timeout     string                 `yaml:"timeout,omitempty"`
	Retry       *RetryPolicy           `yaml:"retry,omitempty"`
	
	// Legacy field - will be removed once all code is migrated
	Steps []Step `yaml:"steps,omitempty"` // Legacy: use sequence or parallel instead
}

// Node represents the fundamental execution unit
// A node contains exactly one of five options
type Node struct {
	// Identity
	ID   string `yaml:"id,omitempty"`   // Optional at root, required in arrays
	Desc string `yaml:"desc,omitempty"` // Human readable description
	
	// Exactly one of these five:
	Op       string    `yaml:"op,omitempty"`       // Operation type
	Sequence []Node    `yaml:"sequence,omitempty"` // Sequential nodes
	Parallel []Node    `yaml:"parallel,omitempty"` // Parallel nodes
	States   *StateMap `yaml:"states,omitempty"`   // State machine
	Shared   string    `yaml:"shared,omitempty"`   // Reference to shared node
	
	// Common properties
	Inputs  map[string]interface{} `yaml:"inputs,omitempty"`
	Outputs map[string]interface{} `yaml:"outputs,omitempty"` // Only for composite types
	Timeout string                 `yaml:"timeout,omitempty"`
	Retry   *RetryPolicy           `yaml:"retry,omitempty"`
	When    string                 `yaml:"when,omitempty"` // Conditional execution
}

// StateMap represents a state machine configuration
type StateMap struct {
	Initial string           `yaml:"initial"` // Which state to start with
	States  map[string]State `yaml:",inline"` // Inline the states
}

// State represents a state in a state machine
// A state is a node plus transition information
type State struct {
	// All node fields apply
	Op       string    `yaml:"op,omitempty"`
	Sequence []Node    `yaml:"sequence,omitempty"`
	Parallel []Node    `yaml:"parallel,omitempty"`
	States   *StateMap `yaml:"states,omitempty"`
	Shared   string    `yaml:"shared,omitempty"`
	
	Desc    string                 `yaml:"desc,omitempty"`
	Inputs  map[string]interface{} `yaml:"inputs,omitempty"`
	Outputs map[string]interface{} `yaml:"outputs,omitempty"`
	Timeout string                 `yaml:"timeout,omitempty"`
	Retry   *RetryPolicy           `yaml:"retry,omitempty"`
	
	// State-specific fields
	Transitions []Transition `yaml:"transitions,omitempty"`
	Error       string       `yaml:"error,omitempty"` // For terminal error states
}

// Transition represents a state transition
type Transition struct {
	To   string `yaml:"to"`
	When string `yaml:"when,omitempty"` // CEL expression
}



// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxAttempts        int     `yaml:"max_attempts"`
	InitialInterval    string  `yaml:"initial_interval"`
	BackoffCoefficient float64 `yaml:"backoff_coefficient,omitempty"`
	MaxInterval        string  `yaml:"max_interval,omitempty"`
}

// Operation types
const (
	OpCommandExecution = "command_execution"
	OpLLMInference     = "llm_inference"
	OpGitShallowClone  = "git_shallow_clone"
	OpSleep            = "sleep"
	OpRecipe           = "recipe"
)

