# Ops Config vs Inputs Cleanup Specification

## Problem Statement

The current implementation in `server/ops` and `server/git` activities mix configuration and runtime inputs together, which creates confusion and violates separation of concerns. Activities should have clear distinction between:

1. **Configuration** - Static settings that define HOW an activity behaves (e.g., timeout settings, retry policies, API endpoints)
2. **Inputs** - Dynamic runtime data that the activity processes (e.g., repository path, commit hash, prompt text)

## Current State Analysis

### server/ops Activities

Example from `llm/activity.go`:
- `LLMActivity` struct combines both configuration (Temperature, MaxTokens, TopP) and inputs (Prompt, SystemPrompt) in the same structure
- Configuration is embedded directly in the activity input struct

### server/git Activities  

Example from `gitcommit/wrapper.go`:
- Uses separate `PersistCommitConfig` (empty) and `PersistCommitInput` structures
- Better separation but config is currently unused
- Input contains both runtime data (RepoPath, CommitMessage) and settings (Timeout)

### recipe-worker/recipe-core

- `recipe-worker` has `ActivityProvider` interface that separates config and inputs in Execute method: `Execute(ctx, args...)` where args[0]=config, args[1]=inputs
- `recipe-core` defines `ActivityTypeDefinition` with separate schemas for ConfigSchema, InputSchema, OutputSchema
- The infrastructure already supports proper separation

## Proposed Solution

### 1. Standardize Activity Structure

All activities should follow this pattern:

```go
// Configuration - HOW the activity behaves
type XxxConfig struct {
    // Static settings
    Timeout        time.Duration `json:"timeout,omitempty"`
    MaxRetries     int          `json:"max_retries,omitempty"`
    // Provider-specific settings
    Temperature    float64      `json:"temperature,omitempty"`  // LLM specific
    MaxTokens      int         `json:"max_tokens,omitempty"`   // LLM specific
}

// Input - WHAT the activity processes
type XxxInput struct {
    // Runtime data only
    Prompt         string      `json:"prompt"`
    RepoPath       string      `json:"repo_path"`
    CommitHash     string      `json:"commit_hash"`
}

// Output - Result of processing
type XxxOutput struct {
    // Result fields
}
```

### 2. Migration Plan

#### Phase 1: Update server/ops Activities

**LLM Activity Refactoring:**
```go
// Before
type LLMActivity struct {
    Prompt       string     // Input
    SystemPrompt string     // Input  
    ModelName    string     // Config
    AdapterName  string     // Config
    Temperature  float64    // Config
    MaxTokens    int        // Config
    // ...
}

// After
type LLMConfig struct {
    ModelName     string    `json:"model_name"`
    AdapterName   string    `json:"adapter_name"`
    Temperature   float64   `json:"temperature,omitempty"`
    MaxTokens     int       `json:"max_tokens,omitempty"`
    TopP          float64   `json:"top_p,omitempty"`
    StopSequences []string  `json:"stop_sequences,omitempty"`
}

type LLMInput struct {
    Prompt         string                 `json:"prompt"`
    SystemPrompt   string                 `json:"system_prompt,omitempty"`
    ResponseSchema json.RawMessage        `json:"response_schema,omitempty"`
    Metadata       map[string]interface{} `json:"metadata,omitempty"`
}
```

**Recipe Activity Refactoring:**
```go
// Before  
type RecipeActivity struct {
    Recipe       string                  // Input
    Timeout      time.Duration          // Config
    RetryPolicy  *RetryPolicy           // Config
    Version      string                 // Config
    Inputs       map[string]interface{} // Input
    Context      *RecipeContext         // Input
}

// After
type RecipeConfig struct {
    Timeout      time.Duration  `json:"timeout,omitempty"`
    RetryPolicy  *RetryPolicy   `json:"retry_policy,omitempty"`
    Version      string         `json:"version,omitempty"`
}

type RecipeInput struct {
    Recipe   string                  `json:"recipe"`
    Inputs   map[string]interface{} `json:"inputs,omitempty"`
    Context  *RecipeContext         `json:"context,omitempty"`
}
```

#### Phase 2: Update server/git Activities

**Git Activities Already Partially Separated:**
```go
// Move timeout from input to config
type PersistCommitConfig struct {
    Timeout         time.Duration `json:"timeout,omitempty"`
    MaxPackSize     int64        `json:"max_pack_size,omitempty"`
}

type PersistCommitInput struct {
    RepoPath        string `json:"repo_path"`
    StorageLocation string `json:"storage_location"`
    RootHash        string `json:"root_hash"`
    CommitMessage   string `json:"commit_message,omitempty"`
    Author          string `json:"author,omitempty"`
    // Remove Timeout from here
}
```

#### Phase 3: Update Activity Execution

**Wrapper Pattern Update:**
```go
func (a *ActivityWrapper) Execute(ctx context.Context, config Config, input Input) (Output, error) {
    // Configuration is passed separately from input
    // This matches the recipe-worker ActivityProvider pattern
}
```

### 3. Integration with recipe-worker

The `recipe-worker` already expects this separation:

```go
// ActivityProvider.Execute expects:
// args[0] = config (map[string]interface{})
// args[1] = inputs (map[string]interface{})
func (p *Provider) Execute(ctx context.Context, args ...interface{}) (interface{}, error)
```

Update the `RegisterableActivityProvider` to properly marshal config and inputs separately:
- Config from activity registration/template
- Inputs from runtime workflow execution

### 4. Schema Registration

Each activity should define schemas for validation:

```go
func (a *LLMActivityWrapper) GetSchemas() (config, input, output *jsonschema.Schema) {
    // Return proper schemas for each part
    return configSchema, inputSchema, outputSchema
}
```

## Benefits

1. **Clear Separation of Concerns**: Configuration is static, inputs are dynamic
2. **Better Reusability**: Same config can be reused across multiple executions
3. **Improved Testing**: Can test with different configs without changing inputs
4. **Schema Validation**: Separate validation for config vs inputs
5. **Alignment with recipe-worker**: Matches expected provider interface

## Implementation Priority

1. **High Priority**: 
   - LLM activities (heavy config)
   - Recipe activities (complex config)

2. **Medium Priority**:
   - Git activities (minimal config)
   - Input activities

3. **Low Priority**:
   - Simple activities with no config

## Testing Strategy

1. Add tests that verify separation of config and inputs
2. Ensure backward compatibility during migration
3. Validate schema generation for both config and inputs
4. Test provider registration with new structure

## Migration Checklist

- [ ] Update LLM activity structures
- [ ] Update Recipe activity structures  
- [ ] Update Git commit activities
- [ ] Update Git shallow activities
- [ ] Update activity wrappers to handle separate config/input
- [ ] Update schema generation
- [ ] Update provider registration
- [ ] Add migration tests
- [ ] Update documentation