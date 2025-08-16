package schema

// SleepOperation pauses execution for a specified duration
type SleepOperation struct {
	Duration Duration `json:"duration" required:"true" pattern:"^[0-9]+(s|m|h)$" description:"Duration to sleep"`
}

func (s *SleepOperation) SchemaDiscriminator() string { return "sleep" }

func (s *SleepOperation) SchemaInputs() interface{} {
	return struct {
		Duration Duration `json:"duration" required:"true" pattern:"^[0-9]+(s|m|h)$" description:"Duration to sleep"`
	}{s.Duration}
}

func (s *SleepOperation) SchemaOutputs() interface{} {
	return SleepOutput{}
}

// SleepOutput defines the output from sleep operation
type SleepOutput struct {
	StartTime       string `json:"start_time" format:"date-time" description:"When sleep started"`
	EndTime         string `json:"end_time" format:"date-time" description:"When sleep ended"`
	ActualDuration  string `json:"actual_duration" description:"Actual duration slept"`
	Completed       bool   `json:"completed" description:"Whether sleep completed normally"`
	Interrupted     bool   `json:"interrupted" description:"Whether sleep was interrupted"`
	ErrorMessage    string `json:"error_message" description:"Error message if failed"`
}

// CommandExecutionOperation executes shell commands
type CommandExecutionOperation struct {
	Run              string            `json:"run" required:"true" description:"Shell command to execute"`
	WorkingDirectory string            `json:"working_directory,omitempty" description:"Directory to execute the command in"`
	WorkingDir       string            `json:"working_dir,omitempty" description:"Working directory configuration"`
	Shell            string            `json:"shell,omitempty" description:"Shell to use (e.g., bash, sh)"`
	Env              map[string]string `json:"env,omitempty" description:"Environment variables"`
	ContinueOnError  bool              `json:"continue_on_error,omitempty" description:"Whether to continue on error"`
	Timeout          Duration          `json:"timeout,omitempty" description:"Command timeout duration"`
}

func (c *CommandExecutionOperation) SchemaDiscriminator() string { return "command_execution" }

func (c *CommandExecutionOperation) SchemaInputs() interface{} {
	return c
}

func (c *CommandExecutionOperation) SchemaOutputs() interface{} {
	return CommandExecutionOutput{}
}

// CommandExecutionOutput defines the output from command execution
type CommandExecutionOutput struct {
	Stdout       string `json:"stdout" description:"Standard output"`
	Stderr       string `json:"stderr" description:"Standard error"`
	ExitCode     int    `json:"exit_code" description:"Process exit code"`
	Success      bool   `json:"success" description:"Whether command succeeded"`
	TimedOut     bool   `json:"timed_out" description:"Whether command timed out"`
	ErrorMessage string `json:"error_message" description:"Error message if failed"`
}

// LLMInferenceOperation performs LLM inference
type LLMInferenceOperation struct {
	Prompt         string                 `json:"prompt,omitempty" description:"Input prompt"`
	SystemPrompt   string                 `json:"system_prompt,omitempty" description:"System prompt"`
	Temperature    float64                `json:"temperature,omitempty" min:"0" max:"2" default:"0.7" description:"Sampling temperature"`
	MaxTokens      int                    `json:"max_tokens,omitempty" min:"1" default:"4096" description:"Maximum tokens to generate"`
	TopP           float64                `json:"top_p,omitempty" min:"0" max:"1" default:"1" description:"Top-p sampling"`
	StopSequences  []string               `json:"stop_sequences,omitempty" description:"Stop sequences"`
	ResponseSchema map[string]interface{} `json:"response_schema,omitempty" description:"Expected response schema"`
	Provider       string                 `json:"provider,omitempty" enum:"OpenAI,Anthropic,Gemini" description:"LLM provider"`
	Model          string                 `json:"model,omitempty" description:"Model identifier"`
}

func (l *LLMInferenceOperation) SchemaDiscriminator() string { return "llm_inference" }

func (l *LLMInferenceOperation) SchemaInputs() interface{} {
	return l
}

func (l *LLMInferenceOperation) SchemaOutputs() interface{} {
	return LLMInferenceOutput{}
}

// LLMInferenceOutput defines the output from LLM inference
type LLMInferenceOutput struct {
	Response     interface{}            `json:"response" description:"Generated response"`
	Model        string                 `json:"model" description:"Model used"`
	FinishReason string                 `json:"finish_reason" description:"Reason for completion"`
	Usage        map[string]interface{} `json:"usage" description:"Token usage statistics"`
}

// GitShallowCloneOperation performs a shallow git clone
type GitShallowCloneOperation struct {
	SourceDir  string `json:"source_dir" required:"true" description:"Source git repository directory"`
	TargetDir  string `json:"target_dir" required:"true" description:"Target directory for the clone"`
	CommitHash string `json:"commit_hash,omitempty" description:"Specific commit to clone"`
}

func (g *GitShallowCloneOperation) SchemaDiscriminator() string { return "git_shallow_clone" }

func (g *GitShallowCloneOperation) SchemaInputs() interface{} {
	return g
}

func (g *GitShallowCloneOperation) SchemaOutputs() interface{} {
	return GitShallowCloneOutput{}
}

// GitShallowCloneOutput defines the output from git shallow clone
type GitShallowCloneOutput struct {
	ClonedPath string `json:"cloned_path" description:"Path to cloned repository"`
}

// RecipeOperation invokes another recipe
type RecipeOperation struct {
	Recipe      string                 `json:"recipe" required:"true" description:"Name of the recipe to invoke"`
	Version     string                 `json:"version,omitempty" description:"Version of the recipe"`
	Timeout     Duration               `json:"timeout,omitempty" description:"Timeout for the recipe execution"`
	RetryPolicy *RetryPolicy           `json:"retry_policy,omitempty" description:"Retry policy for the recipe"`
	Inputs      map[string]interface{} `json:"inputs,omitempty" description:"Inputs to pass to the recipe"`
}

func (r *RecipeOperation) SchemaDiscriminator() string { return "recipe" }

func (r *RecipeOperation) SchemaInputs() interface{} {
	return r
}

func (r *RecipeOperation) SchemaOutputs() interface{} {
	return RecipeOutput{}
}

// RecipeOutput defines the output from recipe invocation
type RecipeOutput struct {
	ExecutionID       string                 `json:"execution_id" description:"Unique execution identifier"`
	Result            interface{}            `json:"result" description:"Recipe execution result"`
	Status            string                 `json:"status" description:"Execution status"`
	RecipeOutputs     map[string]interface{} `json:"recipe_outputs" description:"Recipe output values"`
	ExecutionMetadata struct {
		StartTime    string `json:"start_time" format:"date-time" description:"Execution start time"`
		EndTime      string `json:"end_time" format:"date-time" description:"Execution end time"`
		DurationMs   int64  `json:"duration_ms" description:"Execution duration in milliseconds"`
		AttemptCount int    `json:"attempt_count" description:"Number of attempts"`
	} `json:"execution_metadata" description:"Execution metadata"`
}

// InputOperation collects user input
type InputOperation struct {
	BoxID            string                 `json:"box_id,omitempty" description:"Box identifier"`
	ActivityID       string                 `json:"activity_id,omitempty" description:"Activity identifier"`
	Context          map[string]interface{} `json:"context,omitempty" description:"Additional context data"`
	Title            string                 `json:"title,omitempty" description:"Form title"`
	Question         string                 `json:"question,omitempty" description:"Question to ask the user"`
	Type             string                 `json:"type,omitempty" enum:"short_answer,paragraph_text,multiple_choice,checkboxes,dropdown,linear_scale,date,time" description:"Input type"`
	Fields           []FormField            `json:"fields,omitempty" description:"Form fields for multi-field forms"`
	Timeout          int                    `json:"timeout,omitempty" min:"1" max:"3600" default:"300" description:"Timeout in seconds"`
	DefaultOnTimeout interface{}            `json:"default_on_timeout,omitempty" description:"Default value if input times out"`
}

// FormField defines a form field
type FormField struct {
	ID          string                 `json:"id" required:"true" description:"Field identifier"`
	Type        string                 `json:"type" required:"true" enum:"short_answer,paragraph_text,multiple_choice,checkboxes,dropdown,linear_scale,multiple_choice_grid,checkbox_grid,date,time,file_upload" description:"Field type"`
	Question    string                 `json:"question" required:"true" description:"Field question"`
	Placeholder string                 `json:"placeholder,omitempty" description:"Field placeholder text"`
	Required    bool                   `json:"required,omitempty" description:"Whether field is required"`
	Options     []FieldOption          `json:"options,omitempty" description:"Options for choice fields"`
	Scale       *ScaleConfig           `json:"scale,omitempty" description:"Scale configuration"`
	Validation  *FieldValidation       `json:"validation,omitempty" description:"Field validation rules"`
}

// FieldOption defines an option for choice fields
type FieldOption struct {
	Value string `json:"value" required:"true" description:"Option value"`
	Label string `json:"label" required:"true" description:"Option label"`
}

// ScaleConfig defines scale configuration
type ScaleConfig struct {
	Min      int    `json:"min" required:"true" description:"Minimum value"`
	Max      int    `json:"max" required:"true" description:"Maximum value"`
	MinLabel string `json:"min_label,omitempty" description:"Label for minimum value"`
	MaxLabel string `json:"max_label,omitempty" description:"Label for maximum value"`
}

// FieldValidation defines field validation rules
type FieldValidation struct {
	Min       int    `json:"min,omitempty" description:"Minimum value"`
	Max       int    `json:"max,omitempty" description:"Maximum value"`
	MinLength int    `json:"min_length,omitempty" description:"Minimum length"`
	MaxLength int    `json:"max_length,omitempty" description:"Maximum length"`
	Pattern   string `json:"pattern,omitempty" description:"Regex pattern"`
}

func (i *InputOperation) SchemaDiscriminator() string { return "input" }

func (i *InputOperation) SchemaInputs() interface{} {
	return i
}

func (i *InputOperation) SchemaOutputs() interface{} {
	return InputOutput{}
}

// InputOutput defines the output from input operation
type InputOutput struct {
	Response interface{}            `json:"response,omitempty" description:"User response for single question"`
	Fields   map[string]interface{} `json:"fields,omitempty" description:"User responses for multi-field form"`
	UserID   string                 `json:"user_id,omitempty" description:"ID of user who responded"`
	Metadata map[string]interface{} `json:"metadata,omitempty" description:"Additional metadata"`
}