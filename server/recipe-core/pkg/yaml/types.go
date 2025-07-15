package yaml

import (
	"time"
)

// WorkflowDefinition represents the complete workflow YAML structure
type WorkflowDefinition struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Version     string             `yaml:"version"`
	Inputs      []InputDefinition  `yaml:"inputs"`
	Outputs     []OutputDefinition `yaml:"outputs"`
	Workflow    WorkflowSpec       `yaml:"workflow"`
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
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

// WorkflowSpec defines the workflow execution structure
type WorkflowSpec struct {
	Type        string      `yaml:"type"` // sequential, parallel, state_machine
	RetryPolicy RetryPolicy `yaml:"retry_policy"`
	Steps       []Step      `yaml:"steps"`
	Outputs     map[string]string `yaml:"outputs"`
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	InitialInterval  time.Duration `yaml:"initial_interval"`
	MaximumAttempts  int          `yaml:"maximum_attempts"`
	BackoffCoefficient float64    `yaml:"backoff_coefficient"`
}

// Step represents a workflow step
type Step struct {
	ID       string            `yaml:"id"`
	Activity string            `yaml:"activity"`
	Inputs   map[string]string `yaml:"inputs"`
	Outputs  map[string]string `yaml:"outputs"`
	Parallel []Step            `yaml:"parallel"`
}

// ActivityDefinition represents an activity YAML structure
type ActivityDefinition struct {
	Name           string                 `yaml:"name"`
	Description    string                 `yaml:"description"`
	Timeout        time.Duration          `yaml:"timeout"`
	Retry          ActivityRetryPolicy    `yaml:"retry"`
	Inputs         []InputDefinition      `yaml:"inputs"`
	Outputs        []OutputDefinition     `yaml:"outputs"`
	Implementation ActivityImplementation `yaml:"implementation"`
}

// ActivityRetryPolicy defines activity-specific retry behavior
type ActivityRetryPolicy struct {
	MaximumAttempts    int      `yaml:"maximum_attempts"`
	NonRetryableErrors []string `yaml:"non_retryable_errors"`
}

// ActivityImplementation defines how the activity is executed
type ActivityImplementation struct {
	Type   string                 `yaml:"type"` // http, grpc, script, function, ai_prompt
	Config map[string]interface{} `yaml:"config"`
}

// ProjectManifest represents the project.yaml structure
type ProjectManifest struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Files       struct {
		Workflow   string `yaml:"workflow"`
		Activities string `yaml:"activities"`
		Agents     string `yaml:"agents"`
	} `yaml:"files"`
	File string `yaml:"file"` // For single-file projects
}

// ActivitiesFile represents the activities.yaml structure
type ActivitiesFile struct {
	Activities []ActivityDefinition `yaml:"activities"`
}

// AgentDefinition represents an agent/role configuration
type AgentDefinition struct {
	Role         string   `yaml:"role"`
	Capabilities []string `yaml:"capabilities"`
	Goals        []string `yaml:"goals"`
	Constraints  []string `yaml:"constraints"`
}

// AgentsFile represents the agents.yaml structure
type AgentsFile struct {
	Agents map[string]AgentDefinition `yaml:"agents"`
}