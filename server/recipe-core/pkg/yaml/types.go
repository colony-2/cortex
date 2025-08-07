package yaml

import (
	"time"
)

// RecipeDefinition represents the complete unified recipe YAML structure
// Everything is an activity with a unified 'uses' pattern
type RecipeDefinition struct {
	Name        string                    `yaml:"name"`
	Description string                    `yaml:"description"`
	Version     string                    `yaml:"version"`
	Inputs      []InputDefinition         `yaml:"inputs"`
	Outputs     []OutputDefinition        `yaml:"outputs"`
	Shared      map[string]SharedActivity `yaml:"shared"`   // Reusable activity configurations
	Steps       []Step                    `yaml:"steps"`    // Workflow steps
	Parallel    *ParallelSpec             `yaml:"parallel"` // For parallel workflows
}

// SharedActivity represents a reusable activity configuration in the shared section
type SharedActivity struct {
	Uses   string                 `yaml:"uses"`   // The activity to use
	Config map[string]interface{} `yaml:"config"` // Activity-specific configuration
}

// InputDefinition defines workflow input parameters
type InputDefinition struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Required    bool        `yaml:"required"`
	Default     interface{} `yaml:"default"`
	Description string      `yaml:"description"`
}

// OutputDefinition defines workflow output parameters
type OutputDefinition struct {
	Name        string `yaml:"name"`
	Value       string `yaml:"value"` // Expression to compute output value
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

// Step represents a unified workflow step where everything uses an activity
type Step struct {
	ID       string                 `yaml:"id"`
	Name     string                 `yaml:"name"`     // Human-readable name
	Uses     string                 `yaml:"uses"`     // The activity to use (e.g., "validation_activity", "llm", "state_machine", "data-processor")
	Config   map[string]interface{} `yaml:"config"`   // Activity-specific configuration
	Inputs   map[string]interface{} `yaml:"inputs"`   // Runtime inputs
	Outputs  map[string]string      `yaml:"outputs"`  // Output mapping
	Parallel *ParallelSpec          `yaml:"parallel"` // For parallel steps
}

// ParallelSpec defines parallel execution
type ParallelSpec struct {
	ForEach string `yaml:"for_each"` // Expression to iterate over
	As      string `yaml:"as"`       // Variable name for iteration
	Steps   []Step `yaml:"steps"`    // Steps to execute in parallel
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	InitialInterval    time.Duration `yaml:"initial_interval"`
	MaximumAttempts    int           `yaml:"maximum_attempts"`
	BackoffCoefficient float64       `yaml:"backoff_coefficient"`
	MaximumInterval    time.Duration `yaml:"maximum_interval"`
}

