# VIBETHIS - Operations Activities Module

## Overview

The ops module provides a comprehensive set of RegisterableActivity implementations for Temporal workflow orchestration. Activities include LLM inference, command execution, user input collection, recipe invocation, and utility functions like sleep/delay operations.

## Architecture

### Core Components

- **RegisterableActivity Interface**: Generic activity pattern with metadata, configuration, and execution methods
- **Activity Registry**: Central registration system via `pkg/activity/exports.go`
- **Wrapper Pattern**: Each activity implements a standardized wrapper conforming to `types.RegisterableActivity[TConfig, TInput, TOutput]`

### Key Modules

- **LLM Module** (`pkg/llm/`): Multi-provider LLM inference with OpenAI, Anthropic, and Gemini support
- **Command Module** (`pkg/command/`): Cross-platform shell command execution
- **Input Module** (`pkg/input/`): Interactive user input collection via forms
- **Recipe Module** (`pkg/recipe/`): Child recipe workflow invocation
- **Sleep Module** (`pkg/sleep/`): Execution delay utilities

## Key Interfaces

### RegisterableActivity Interface

```go
type RegisterableActivity[TConfig any, TInput any, TOutput any] interface {
    GetMetadata() ActivityMetadata
    Execute(ctx context.Context, config TConfig, inputs TInput) (TOutput, error)
}
```

### Activity Discovery

```go
func GetAll() []interface{}  // Returns all available activities
```

### LLM Activity

```go
// Configuration
type LLMConfig struct {
    Provider string `json:"provider"` // openai, anthropic, gemini
    Model    string `json:"model"`    // model name
}

// Input
type LLMInput struct {
    Prompt         string          `json:"prompt"`
    SystemPrompt   string          `json:"system_prompt"`
    Temperature    float64         `json:"temperature"`
    MaxTokens      int             `json:"max_tokens"`
    ResponseSchema json.RawMessage `json:"response_schema,omitempty"`
}

// Output
type LLMOutput struct {
    Response     json.RawMessage        `json:"response"`
    Model        string                 `json:"model"`
    FinishReason string                 `json:"finish_reason"`
    Usage        map[string]interface{} `json:"usage"`
}
```

### Command Execution Activity

```go
// Configuration
type CommandExecutionConfig struct {
    WorkingDir string            `json:"working_dir"`
    Shell      string            `json:"shell"`
    Env        map[string]string `json:"env"`
}

// Input
type CommandExecutionInput struct {
    Run              string            `json:"run"`
    WorkingDirectory string            `json:"working_directory"`
    Shell            string            `json:"shell"`
    Env              map[string]string `json:"env"`
    ContinueOnError  bool              `json:"continue_on_error"`
    Timeout          string            `json:"timeout"`
}

// Output
type CommandExecutionOutput struct {
    Stdout       string `json:"stdout"`
    Stderr       string `json:"stderr"`
    ExitCode     int    `json:"exit_code"`
    Success      bool   `json:"success"`
    TimedOut     bool   `json:"timed_out"`
    ErrorMessage string `json:"error_message"`
}
```

### Input Collection Activity

```go
// Configuration
type Config struct {
    Question string      `json:"question,omitempty"`
    Type     FieldType   `json:"type,omitempty"`
    Options  []Option    `json:"options,omitempty"`
    Title    string      `json:"title,omitempty"`
    Fields   []FormField `json:"fields,omitempty"`
    Timeout  int         `json:"timeout,omitempty"`
}

// Output
type Output struct {
    Response interface{}            `json:"response,omitempty"`
    Fields   map[string]interface{} `json:"fields,omitempty"`
    UserID   string                 `json:"user_id,omitempty"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}
```

## Usage Examples

### LLM Inference

```go
// Initialize registry (once per worker)
if err := llm.InitializeRegistry(); err != nil {
    log.Fatal(err)
}

// Execute LLM activity
activity := llm.NewLLMActivity()
config := llm.LLMConfig{
    Provider: "openai",
    Model:    "gpt-4",
}
input := llm.LLMInput{
    Prompt:      "Explain quantum computing",
    Temperature: 0.7,
    MaxTokens:   200,
}

output, err := activity.Execute(ctx, config, input)
```

### Command Execution

```go
activity := command.NewCommandExecutionActivity()
config := command.CommandExecutionConfig{
    WorkingDir: "/workspace",
    Shell:      "bash",
}
input := command.CommandExecutionInput{
    Run:             "git status",
    Timeout:         "30s",
    ContinueOnError: false,
}

output, err := activity.Execute(ctx, config, input)
fmt.Printf("Exit code: %d\nOutput: %s\n", output.ExitCode, output.Stdout)
```

### User Input Collection

```go
activity := input.NewInputActivity()
config := input.Config{
    Question: "What is your preferred deployment environment?",
    Type:     input.FieldTypeMultipleChoice,
    Options: []input.Option{
        {Label: "Production", Value: "prod"},
        {Label: "Staging", Value: "staging"},
        {Label: "Development", Value: "dev"},
    },
    Timeout: 300,
}
inputData := input.Input{
    BoxID:      "deployment-config",
    ActivityID: "env-selection",
}

output, err := activity.Execute(ctx, config, inputData)
```

### Recipe Invocation

```go
activity := recipe.NewRecipeActivity()
config := recipe.RecipeConfig{
    Recipe:  "data-processing/transform",
    Version: "v2.1.0",
    Timeout: "30m",
}
input := recipe.RecipeInput{
    "source_path": "/data/raw",
    "target_path": "/data/processed",
}

output, err := activity.Execute(ctx, config, input)
fmt.Printf("Recipe execution ID: %s\n", output.ExecutionID)
```

### Sleep/Delay

```go
activity := sleep.NewSleepActivity()
config := sleep.SleepConfig{
    DefaultDuration: "5s",
}
input := sleep.SleepInput{
    Duration: "30s",
}

output, err := activity.Execute(ctx, config, input)
fmt.Printf("Slept for: %s\n", output.ActualDuration)
```

## Configuration

### Environment Variables

**LLM Configuration:**
- `OPENAI_API_KEY`: OpenAI API access
- `ANTHROPIC_API_KEY`: Anthropic Claude API access  
- `GEMINI_API_KEY`: Google Gemini API access

**Recipe Context:**
- `ENVIRONMENT`: Execution environment (production, staging, development)
- `REGION`: Deployment region
- `TEMPORAL_NAMESPACE`: Temporal namespace
- `TEMPORAL_TASK_QUEUE`: Default task queue

### Registry Initialization

```go
// Initialize all activities for recipe-worker consumption
activities := activity.GetAll()
for _, act := range activities {
    worker.RegisterActivity(act)
}
```

### Activity Metadata

Each activity provides standardized metadata including type identifiers, timeout configurations, and retry policies for robust workflow orchestration.