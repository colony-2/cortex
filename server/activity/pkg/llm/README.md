# LLM Temporal Activity

This package provides a Temporal activity for integrating with various LLM providers (OpenAI, Anthropic, AWS Bedrock) through a unified interface.

## Features

- **Multiple LLM Provider Support**: OpenAI, Anthropic, AWS Bedrock
- **Structured Response Support**: Define Go structs or JSON schemas for typed responses
- **Cost Tracking**: Automatic calculation of API costs based on token usage
- **Flexible Configuration**: Temperature, max tokens, system prompts, etc.
- **Registry Pattern**: Dynamic adapter registration based on available credentials

## Usage

### 1. Initialize the Registry

Before using the activity, initialize the LLM adapter registry in your worker:

```go
import "github.com/divisive-ai/vibethis/server/activity/pkg/llm"

// During worker initialization
if err := llm.InitializeRegistry(); err != nil {
    log.Fatal("Failed to initialize LLM registry:", err)
}
```

The registry will automatically register adapters based on environment variables:
- `OPENAI_API_KEY` - For OpenAI adapter
- `ANTHROPIC_API_KEY` - For Anthropic adapter  
- `AWS_REGION` - For AWS Bedrock adapter

### 2. Register the Activity

Register the activity with your Temporal worker:

```go
worker.RegisterActivity(llm.ExecuteLLMTask)
```

### 3. Use in Workflows

#### Text Response

```go
input := llm.LLMActivity{
    Prompt:       "Explain quantum computing in simple terms",
    SystemPrompt: "You are a helpful science educator",
    ModelName:    "gpt-3.5-turbo",
    AdapterName:  "openai",
    Temperature:  0.7,
    MaxTokens:    200,
}

var output llm.LLMActivityOutput
err := workflow.ExecuteActivity(ctx, llm.ExecuteLLMTask, input).Get(ctx, &output)
if err != nil {
    return err
}

// Extract text response
var textResponse string
json.Unmarshal(output.Response, &textResponse)
```

#### Structured Response with JSON Schema

```go
// Define your expected response schema
schema := json.RawMessage(`{
    "type": "object",
    "properties": {
        "summary": {"type": "string"},
        "keyPoints": {"type": "array", "items": {"type": "string"}},
        "difficulty": {"type": "string", "enum": ["easy", "medium", "hard"]}
    },
    "required": ["summary", "keyPoints", "difficulty"]
}`)

input := llm.LLMActivity{
    Prompt:         "Analyze this technical document and provide a structured summary",
    ModelName:      "claude-3-haiku",
    AdapterName:    "anthropic",
    ResponseSchema: schema,
}

var output llm.LLMActivityOutput
err := workflow.ExecuteActivity(ctx, llm.ExecuteLLMTask, input).Get(ctx, &output)

// Parse the structured response
var result struct {
    Summary    string   `json:"summary"`
    KeyPoints  []string `json:"keyPoints"`
    Difficulty string   `json:"difficulty"`
}
json.Unmarshal(output.Response, &result)
```

### 4. Cost Tracking

The activity automatically tracks token usage and estimates costs:

```go
fmt.Printf("Tokens used: %d (prompt: %d, completion: %d)\n", 
    output.Telemetry.TotalTokens,
    output.Telemetry.PromptTokens,
    output.Telemetry.CompletionTokens)

fmt.Printf("Estimated cost: $%.4f USD\n", output.Telemetry.EstimatedCostUSD)
```

## Direct Function Usage

You can also use the LLM task function directly without Temporal:

```go
// Define a struct for structured responses
type ProductInfo struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    Features    []string `json:"features"`
    Price       float64  `json:"price"`
}

// Create input with response structure
var product ProductInfo
input := llm.LLMTaskInput{
    Prompt:            "Generate a product description for a smart water bottle",
    ModelName:         "gpt-4",
    AdapterName:       "openai",
    ResponseStructure: &product,
    Temperature:       0.8,
}

// Execute task
registry := llm.GetRegistry() // Or use your own registry
output, err := llm.LLMTask(ctx, input, registry)
if err != nil {
    return err
}

// The product struct is now populated with the response
fmt.Printf("Product: %+v\n", product)
```

## Supported Models

### OpenAI
- gpt-4
- gpt-4-turbo
- gpt-3.5-turbo

### Anthropic
- claude-3-opus
- claude-3-sonnet
- claude-3-haiku

### AWS Bedrock
- anthropic.claude-v2
- amazon.titan-text-express-v1
- (and other Bedrock-supported models)

## Configuration Options

- `Temperature`: Controls randomness (0-1)
- `MaxTokens`: Maximum tokens to generate
- `TopP`: Nucleus sampling parameter
- `StopSequences`: Array of sequences that stop generation
- `SystemPrompt`: System message to guide the model
- `Metadata`: Additional metadata to pass through

## Testing

For testing, you can use a mock registry:

```go
mockAdapter := &mockAdapter{
    generateFunc: func(ctx context.Context, prompt string, config llmadapters.Config) (llmadapters.Response, error) {
        return llmadapters.Response{
            Content: "Mock response",
            Usage: llmadapters.Usage{TotalTokens: 10},
        }, nil
    },
}

registry := llmadapters.NewRegistry()
registry.Register("mock", mockAdapter)
llm.SetRegistry(registry)
```