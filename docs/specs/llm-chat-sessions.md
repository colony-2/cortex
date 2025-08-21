# LLM Chat Session Support Specification

## Overview

This specification defines the architecture for introducing persistent chat session support to the existing LLM operations and adapters infrastructure. The key insight is that since sessions are constrained to a single recipe (Temporal workflow), we can leverage Temporal's built-in workflow history and state management instead of requiring external storage.

## Goals

1. **Persistent Sessions**: Support chat sessions that can span days or weeks using Temporal's durable state
2. **Context Recovery**: Automatically reconstruct context from workflow history
3. **Recipe Scoped**: Sessions are naturally constrained to a single recipe/temporal workflow
4. **Provider Agnostic**: Work across all LLM providers (OpenAI, Anthropic, Gemini, etc.)
5. **No External Dependencies**: Use Temporal's existing infrastructure for persistence
6. **Backward Compatible**: Maintain compatibility with existing single-turn operations

## Architecture

### Core Design Principle

Instead of external storage, we use:
- **Temporal Workflow State**: For maintaining the current session context
- **Temporal Activity Results**: For storing individual messages and responses
- **Temporal Query Handlers**: For retrieving session history
- **Temporal Signals**: For session management operations

### Core Components

#### 1. Session State in Workflow

```go
// server/ops/pkg/llm/session_workflow.go

type LLMSessionState struct {
    SessionID         string            `json:"session_id"`
    Messages          []Message         `json:"messages"`
    ProviderSessions  map[string]string `json:"provider_sessions"` // adapter -> provider session ID
    TokenUsage        TokenUsage        `json:"token_usage"`
    CreatedAt         time.Time         `json:"created_at"`
    LastInteraction   time.Time         `json:"last_interaction"`
    ContextStrategy   string            `json:"context_strategy"`
    MaxContextMessages int              `json:"max_context_messages"`
}

type Message struct {
    ID        string                 `json:"id"`
    Role      MessageRole            `json:"role"` // "system", "user", "assistant", "tool"
    Content   string                 `json:"content"`
    ToolCalls []ToolCall            `json:"tool_calls,omitempty"`
    ToolResults []ToolResult        `json:"tool_results,omitempty"`
    Timestamp time.Time             `json:"timestamp"`
    TokenCount int                   `json:"token_count"`
    ActivityID string                `json:"activity_id"` // Links to Temporal activity execution
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Workflow maintains session state
type RecipeWorkflow struct {
    // Existing recipe fields...
    
    // Chat session state
    Sessions map[string]*LLMSessionState `json:"sessions"` // sessionID -> state
}
```

#### 2. Enhanced LLM Activity with Session Support

```go
// server/ops/pkg/llm/llm_session.go

type LLMSessionTaskInput struct {
    // Session management
    SessionID     string `json:"sessionId,omitempty"`      // Session identifier within workflow
    CreateSession bool   `json:"createSession,omitempty"`   // Create new session if true
    
    // Standard LLM input
    Prompt       string `json:"prompt"`
    ModelName    string `json:"modelName"`
    AdapterName  string `json:"adapterName"`
    
    // Optional configuration
    SystemPrompt  string                 `json:"systemPrompt,omitempty"`
    Temperature   float64                `json:"temperature,omitempty"`
    MaxTokens     int                    `json:"maxTokens,omitempty"`
    
    // Context management
    MaxContextMessages int    `json:"maxContextMessages,omitempty"` // Limit context size
    ContextStrategy    string `json:"contextStrategy,omitempty"`    // "full", "sliding_window", "summarized"
    
    // Session context (populated by workflow before activity execution)
    PreviousMessages []Message `json:"previousMessages,omitempty"`
    ProviderSessionID string   `json:"providerSessionId,omitempty"`
}

type LLMSessionTaskOutput struct {
    Response  interface{}  `json:"response"`
    Message   Message      `json:"message"`      // Complete message to store
    SessionID string       `json:"sessionId"`
    Telemetry LLMTelemetry `json:"telemetry"`
    
    // Provider session info
    ProviderSessionID string `json:"providerSessionId,omitempty"`
    ContextRecreated  bool   `json:"contextRecreated"` // True if context was rebuilt
}
```

#### 3. Workflow Implementation

```go
// server/ops/pkg/recipe/workflow_with_session.go

func (w *RecipeWorkflow) ExecuteLLMWithSession(ctx workflow.Context, input LLMSessionTaskInput) (*LLMSessionTaskOutput, error) {
    // 1. Get or create session state
    var session *LLMSessionState
    if input.SessionID != "" {
        session = w.Sessions[input.SessionID]
        if session == nil && input.CreateSession {
            session = w.createNewSession(input.SessionID)
        }
    } else if input.CreateSession {
        input.SessionID = generateSessionID()
        session = w.createNewSession(input.SessionID)
    }
    
    if session == nil {
        // Fall back to single-turn mode
        return w.executeSingleTurnLLM(ctx, input)
    }
    
    // 2. Prepare context for activity
    input.PreviousMessages = w.prepareContextMessages(session, input.ContextStrategy)
    input.ProviderSessionID = session.ProviderSessions[input.AdapterName]
    
    // 3. Execute activity with retry logic
    activityOptions := workflow.ActivityOptions{
        StartToCloseTimeout: 5 * time.Minute,
        RetryPolicy: &temporal.RetryPolicy{
            MaximumAttempts: 3,
        },
    }
    ctx = workflow.WithActivityOptions(ctx, activityOptions)
    
    var output LLMSessionTaskOutput
    err := workflow.ExecuteActivity(ctx, LLMSessionActivity, input).Get(ctx, &output)
    if err != nil {
        return nil, err
    }
    
    // 4. Update session state in workflow
    session.Messages = append(session.Messages, output.Message)
    session.LastInteraction = workflow.Now(ctx)
    session.TokenUsage.TotalTokens += output.Telemetry.TotalTokens
    
    // Update provider session ID if changed
    if output.ProviderSessionID != "" {
        session.ProviderSessions[input.AdapterName] = output.ProviderSessionID
    }
    
    // 5. Store updated state
    w.Sessions[input.SessionID] = session
    
    return &output, nil
}

func (w *RecipeWorkflow) prepareContextMessages(session *LLMSessionState, strategy string) []Message {
    messages := session.Messages
    
    switch strategy {
    case "sliding_window":
        if session.MaxContextMessages > 0 && len(messages) > session.MaxContextMessages {
            // Keep system message if present, then take last N messages
            startIdx := len(messages) - session.MaxContextMessages
            if len(messages) > 0 && messages[0].Role == RoleSystem {
                return append([]Message{messages[0]}, messages[startIdx:]...)
            }
            return messages[startIdx:]
        }
    case "summarized":
        // This would trigger a summarization activity for older messages
        return w.summarizeOldMessages(messages)
    default: // "full"
        return messages
    }
    
    return messages
}

func (w *RecipeWorkflow) createNewSession(sessionID string) *LLMSessionState {
    if w.Sessions == nil {
        w.Sessions = make(map[string]*LLMSessionState)
    }
    
    session := &LLMSessionState{
        SessionID:        sessionID,
        Messages:         []Message{},
        ProviderSessions: make(map[string]string),
        TokenUsage:       TokenUsage{},
        CreatedAt:        workflow.Now(ctx),
        LastInteraction:  workflow.Now(ctx),
    }
    
    w.Sessions[sessionID] = session
    return session
}
```

#### 4. Enhanced Activity Implementation

```go
// server/ops/pkg/llm/activity_session.go

func LLMSessionActivity(ctx context.Context, input LLMSessionTaskInput) (*LLMSessionTaskOutput, error) {
    // Get adapter
    adapter := getAdapter(input.AdapterName)
    
    // Check if we need to use session-aware adapter
    sessionAdapter, isSessionAware := adapter.(SessionAwareAdapter)
    
    var response Response
    var providerSessionID string
    contextRecreated := false
    
    if isSessionAware && len(input.PreviousMessages) > 0 {
        // Try to use existing provider session
        if input.ProviderSessionID != "" {
            hasSession, _ := sessionAdapter.HasProviderSession(ctx, input.ProviderSessionID)
            if !hasSession {
                // Need to recreate context
                newSessionID, err := sessionAdapter.ReconstructSession(ctx, input.PreviousMessages, config)
                if err == nil {
                    providerSessionID = newSessionID
                    contextRecreated = true
                }
            } else {
                providerSessionID = input.ProviderSessionID
            }
        }
        
        // Generate with session context
        response, err = sessionAdapter.GenerateWithSession(ctx, providerSessionID, input.Prompt, config)
    } else {
        // Fall back to standard generation with manual context
        fullPrompt := constructPromptWithHistory(input.PreviousMessages, input.Prompt)
        response, err = adapter.Generate(ctx, fullPrompt, config)
    }
    
    if err != nil {
        return nil, fmt.Errorf("generation failed: %w", err)
    }
    
    // Build complete message for storage
    message := Message{
        ID:         generateMessageID(),
        Role:       RoleAssistant,
        Content:    response.Content,
        ToolCalls:  response.ToolCalls,
        Timestamp:  time.Now(),
        TokenCount: response.Usage.CompletionTokens,
        ActivityID: activity.GetInfo(ctx).ActivityID,
    }
    
    return &LLMSessionTaskOutput{
        Response:          response.Content,
        Message:           message,
        SessionID:         input.SessionID,
        Telemetry:         buildTelemetry(response.Usage),
        ProviderSessionID: providerSessionID,
        ContextRecreated:  contextRecreated,
    }, nil
}

func constructPromptWithHistory(messages []Message, currentPrompt string) string {
    // For adapters without native session support,
    // construct a prompt that includes conversation history
    var prompt strings.Builder
    
    for _, msg := range messages {
        switch msg.Role {
        case RoleSystem:
            prompt.WriteString(fmt.Sprintf("System: %s\n\n", msg.Content))
        case RoleUser:
            prompt.WriteString(fmt.Sprintf("User: %s\n\n", msg.Content))
        case RoleAssistant:
            prompt.WriteString(fmt.Sprintf("Assistant: %s\n\n", msg.Content))
        }
    }
    
    prompt.WriteString(fmt.Sprintf("User: %s\n\nAssistant:", currentPrompt))
    return prompt.String()
}
```

#### 5. Query Handlers for Session History

```go
// server/ops/pkg/recipe/workflow_queries.go

type GetSessionHistoryQuery struct {
    SessionID string `json:"sessionId"`
}

type SessionHistoryResponse struct {
    Messages   []Message  `json:"messages"`
    TokenUsage TokenUsage `json:"tokenUsage"`
    CreatedAt  time.Time  `json:"createdAt"`
}

func (w *RecipeWorkflow) GetSessionHistory(query GetSessionHistoryQuery) (*SessionHistoryResponse, error) {
    session, exists := w.Sessions[query.SessionID]
    if !exists {
        return nil, fmt.Errorf("session not found: %s", query.SessionID)
    }
    
    return &SessionHistoryResponse{
        Messages:   session.Messages,
        TokenUsage: session.TokenUsage,
        CreatedAt:  session.CreatedAt,
    }, nil
}

// Register query handler in workflow
func RecipeWorkflowWithQueries(ctx workflow.Context, input RecipeInput) error {
    w := &RecipeWorkflow{
        Sessions: make(map[string]*LLMSessionState),
    }
    
    // Register query handlers
    err := workflow.SetQueryHandler(ctx, "get-session-history", w.GetSessionHistory)
    if err != nil {
        return err
    }
    
    // Continue with normal workflow execution...
}
```

#### 6. Signals for Session Management

```go
// server/ops/pkg/recipe/workflow_signals.go

type ClearSessionSignal struct {
    SessionID string `json:"sessionId"`
}

type UpdateSessionConfigSignal struct {
    SessionID          string `json:"sessionId"`
    ContextStrategy    string `json:"contextStrategy,omitempty"`
    MaxContextMessages int    `json:"maxContextMessages,omitempty"`
}

func (w *RecipeWorkflow) HandleClearSession(signal ClearSessionSignal) error {
    if session, exists := w.Sessions[signal.SessionID]; exists {
        // Clear messages but keep session structure
        session.Messages = []Message{}
        session.TokenUsage = TokenUsage{}
        session.ProviderSessions = make(map[string]string)
    }
    return nil
}

func (w *RecipeWorkflow) HandleUpdateSessionConfig(signal UpdateSessionConfigSignal) error {
    if session, exists := w.Sessions[signal.SessionID]; exists {
        if signal.ContextStrategy != "" {
            session.ContextStrategy = signal.ContextStrategy
        }
        if signal.MaxContextMessages > 0 {
            session.MaxContextMessages = signal.MaxContextMessages
        }
    }
    return nil
}
```

### Provider-Specific Adapters

```go
// server/llm/adapters/session_aware.go

type SessionAwareAdapter interface {
    Adapter // Extends existing Adapter interface
    
    // Generate with session context
    GenerateWithSession(ctx context.Context, sessionID string, prompt string, config Config) (Response, error)
    
    // Check if provider still has session in memory
    HasProviderSession(ctx context.Context, sessionID string) (bool, error)
    
    // Reconstruct session context at provider
    ReconstructSession(ctx context.Context, messages []Message, config Config) (string, error)
}

// OpenAI implementation using threads API
type OpenAISessionAdapter struct {
    *OpenAIAdapter
    threads map[string]*openai.Thread // Cache of active threads
}

func (a *OpenAISessionAdapter) GenerateWithSession(ctx context.Context, threadID string, prompt string, config Config) (Response, error) {
    // Use OpenAI's threads API for persistent conversation
    if threadID == "" {
        // Create new thread
        thread, err := a.client.CreateThread(ctx)
        if err != nil {
            return Response{}, err
        }
        threadID = thread.ID
    }
    
    // Add message to thread
    _, err := a.client.CreateMessage(ctx, threadID, openai.MessageRequest{
        Role:    "user",
        Content: prompt,
    })
    
    // Run assistant on thread
    run, err := a.client.CreateRun(ctx, threadID, openai.RunRequest{
        AssistantID: a.assistantID,
        Model:       config.Model,
    })
    
    // Wait for completion and return response
    // ...
}

// Anthropic implementation using manual context management
type AnthropicSessionAdapter struct {
    *AnthropicAdapter
}

func (a *AnthropicSessionAdapter) GenerateWithSession(ctx context.Context, sessionID string, prompt string, config Config) (Response, error) {
    // Anthropic doesn't have native sessions, so we handle it in the activity
    // by constructing the full conversation in the prompt
    return a.Generate(ctx, prompt, config)
}

func (a *AnthropicSessionAdapter) HasProviderSession(ctx context.Context, sessionID string) (bool, error) {
    // Anthropic doesn't maintain server-side sessions
    return false, nil
}

func (a *AnthropicSessionAdapter) ReconstructSession(ctx context.Context, messages []Message, config Config) (string, error) {
    // No provider session to reconstruct for Anthropic
    return "", nil
}
```

### Advantages of Temporal-Based Approach

1. **No External Dependencies**: Uses Temporal's existing persistence layer
2. **Automatic Durability**: Sessions persist across worker restarts
3. **Natural Scoping**: Sessions are automatically scoped to workflows
4. **Version Safety**: Temporal handles versioning of workflow code
5. **Query Support**: Easy to retrieve session history via queries
6. **Signal Support**: Can modify sessions during execution
7. **Audit Trail**: Complete history in Temporal's event log
8. **Replay Safety**: Deterministic replay ensures consistency

### Usage Example

```go
// In a recipe workflow
func MyRecipeWorkflow(ctx workflow.Context, input RecipeInput) error {
    // Start a chat session for iterative refinement
    sessionInput := LLMSessionTaskInput{
        CreateSession: true,
        SessionID:     "refinement-session",
        AdapterName:   "openai",
        ModelName:     "gpt-4",
        SystemPrompt:  "You are helping refine a document through iterative feedback",
        Prompt:        "Let's start working on improving this document: " + input.Document,
        ContextStrategy: "sliding_window",
        MaxContextMessages: 20,
    }
    
    // First interaction
    output1, err := workflow.ExecuteActivity(ctx, LLMSessionActivity, sessionInput).Get(ctx, &output1)
    
    // Subsequent interactions use the same session
    sessionInput.CreateSession = false
    sessionInput.Prompt = "Can you make the introduction more engaging?"
    output2, err := workflow.ExecuteActivity(ctx, LLMSessionActivity, sessionInput).Get(ctx, &output2)
    
    // Continue with more interactions...
    // The session context is automatically maintained
}
```

### Migration Strategy

1. **Phase 1**: Add session state to workflow struct
2. **Phase 2**: Implement session-aware activity
3. **Phase 3**: Add query and signal handlers
4. **Phase 4**: Implement provider-specific adapters
5. **Phase 5**: Update existing workflows to use sessions where beneficial

### Testing Strategy

```go
func TestTemporalSessionWorkflow(t *testing.T) {
    suite := testsuite.WorkflowTestSuite{}
    env := suite.NewTestWorkflowEnvironment()
    
    // Register activities
    env.RegisterActivity(LLMSessionActivity)
    
    // Test workflow with sessions
    env.ExecuteWorkflow(RecipeWorkflowWithSessions, input)
    
    // Query session history
    var history SessionHistoryResponse
    err := env.QueryWorkflow("get-session-history", GetSessionHistoryQuery{SessionID: "test"})
    assert.NoError(t, err)
    assert.Len(t, history.Messages, expectedCount)
}
```

### Monitoring

Since we're using Temporal, we get monitoring for free:
- Session count = number of active workflows with sessions
- Message count = visible in workflow state
- Token usage = tracked in workflow state
- Session duration = workflow execution time
- Failures = Temporal's built-in metrics

### Future Enhancements

1. **Session Forking**: Use Temporal's child workflows for branching conversations
2. **Cross-Workflow Sessions**: Use Temporal signals to share sessions
3. **Session Templates**: Use workflow templates for common patterns
4. **Continuous Sessions**: Use Temporal's continue-as-new for very long sessions