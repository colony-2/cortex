# Structured Output Support

The LLM adapters now support structured output through the `ResponseSchema` field in the `Config` struct.

## Overview

Structured output allows you to define a JSON Schema that the LLM will follow when generating responses. This ensures consistent, parseable output that matches your expected data structure.

## Configuration

To use structured output, set the following fields in your `Config`:

```go
config := adapters.Config{
    Model:          "gemini-1.5-flash",
    ResponseFormat: "json",              // Must be "json" for structured output
    ResponseSchema: json.RawMessage(`{   // JSON Schema definition
        "type": "object",
        "properties": {
            "name": {"type": "string"},
            "age": {"type": "integer"}
        },
        "required": ["name", "age"]
    }`),
}
```

## Adapter Support

### Gemini Adapter ✅
The Gemini adapter has **native support** for structured output using the `ResponseJsonSchema` field. When you provide a `ResponseSchema`, it will:
1. Set `ResponseMIMEType` to "application/json"
2. Pass your schema to `ResponseJsonSchema` for native validation

### OpenAI Adapter ⚠️
The OpenAI adapter currently only supports `ResponseFormat: "json"` without schema validation. You'll need to use prompt engineering to ensure the correct structure.

### Anthropic Adapter ⚠️
Similar to OpenAI, Anthropic only supports JSON mode without native schema validation.

### Bedrock Adapter ⚠️
Bedrock's support varies by model. Most models only support JSON mode without schema validation.

## Example Usage

```go
// Define your expected structure
type PersonInfo struct {
    Name       string   `json:"name"`
    Age        int      `json:"age"`
    Occupation string   `json:"occupation"`
    Hobbies    []string `json:"hobbies"`
}

// Create the JSON Schema
schema := json.RawMessage(`{
    "type": "object",
    "properties": {
        "name": {"type": "string"},
        "age": {"type": "integer"},
        "occupation": {"type": "string"},
        "hobbies": {
            "type": "array",
            "items": {"type": "string"}
        }
    },
    "required": ["name", "age", "occupation", "hobbies"]
}`)

// Configure the adapter
config := adapters.Config{
    Model:          "gemini-1.5-flash",
    Temperature:    0.3,
    ResponseFormat: "json",
    ResponseSchema: schema,
}

// Generate structured output
response, err := adapter.Generate(ctx, "Generate a person profile", config)

// Parse the response
var person PersonInfo
err = json.Unmarshal([]byte(response.Content), &person)
```

## Best Practices

1. **Use lower temperatures** (0.3-0.5) for more consistent structured output
2. **Include descriptions** in your schema properties to help the model understand what to generate
3. **Test your schemas** thoroughly as different models may interpret them differently
4. **Fallback handling**: Always handle parsing errors gracefully as models may occasionally deviate from the schema

## Future Improvements

- Add native schema support to OpenAI adapter when their SDK is updated
- Add schema validation for adapters that don't have native support
- Support for more complex JSON Schema features (oneOf, anyOf, etc.)