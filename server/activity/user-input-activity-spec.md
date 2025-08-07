# User Input Activity Specification

## Overview
User Input Activities are interactive activities that pause workflow execution to gather user responses via Cortex LLM integration. These activities enable human-in-the-loop workflows where user feedback, decisions, or reviews are required.

## Core Components

### Activity Types

#### 1. Question Activity (`user_input_question`)
Presents an LLM-generated question to the user and waits for their response.

```yaml
type: user_input_question
config:
  prompt: "Based on the analysis results, which optimization strategy should we pursue?"
  context:
    - type: artifact
      path: "analysis/performance_report.json"
    - type: text
      content: "Current system metrics show 80% CPU utilization"
  response_type: "choice" | "text" | "structured"
  options: # For choice type only
    - value: "aggressive"
      label: "Aggressive optimization (may impact stability)"
    - value: "conservative"
      label: "Conservative optimization (safer)"
  timeout: 300 # seconds, optional
  cortex_model: "gpt-4" # optional, defaults to configured model
```

#### 2. Review Activity (`user_input_review`)
Requests user review of generated artifacts or outputs.

```yaml
type: user_input_review
config:
  review_prompt: "Please review the generated configuration files"
  artifacts:
    - path: "generated/config.yaml"
      description: "Main configuration"
    - path: "generated/deployment.yaml"
      description: "Deployment manifest"
  review_options:
    - approve
    - reject
    - request_changes
  change_request_prompt: "What changes are needed?" # If request_changes selected
  cortex_enhancement: true # Use Cortex to summarize/highlight key changes
```

#### 3. Decision Activity (`user_input_decision`)
Presents a structured decision point with Cortex-analyzed options.

```yaml
type: user_input_decision
config:
  decision_prompt: "Select deployment strategy"
  cortex_analysis:
    prompt: "Analyze the pros and cons of each deployment option"
    context_artifacts:
      - "metrics/current_state.json"
      - "deployment/options.yaml"
  options:
    - id: "blue_green"
      description: "Blue-green deployment"
      cortex_summary: true # Generate summary via Cortex
    - id: "canary"
      description: "Canary deployment (10% initial)"
      cortex_summary: true
    - id: "rolling"
      description: "Rolling update"
      cortex_summary: true
  require_justification: true
```

## Implementation Structure

### Activity Handler
```go
// server/activity/pkg/activity/user_input.go
type UserInputActivity struct {
    Type           string
    CortexClient   *cortex.Client
    ResponseStore  ResponseStore
}

type UserInputConfig struct {
    Prompt         string
    Context        []ContextItem
    ResponseType   ResponseType
    Options        []Option
    Timeout        int
    CortexModel    string
}

type UserResponse struct {
    ActivityID     string
    UserID         string
    Response       interface{}
    Timestamp      time.Time
    Justification  string
}
```

### Cortex Integration
```go
// server/activity/pkg/cortex/user_input.go
type UserInputRequest struct {
    SessionID      string
    Prompt         string
    Context        []byte // Serialized context
    ResponseSchema *ResponseSchema
}

type UserInputResponse struct {
    SessionID      string
    FormattedPrompt string // Cortex-enhanced prompt
    ResponseID     string  // For tracking
}
```

## Workflow Integration

### Execution Flow
1. Activity executor encounters user input activity
2. Prepares context and sends request to Cortex
3. Cortex formats prompt with context and returns session info
4. Activity pauses and stores state
5. User receives notification (webhook/polling)
6. User submits response via UI/API
7. Response validated and stored
8. Activity resumes with user response as output

### State Management
```yaml
# Activity state during user input wait
state:
  status: "waiting_for_user"
  cortex_session: "session_123"
  response_id: "resp_456"
  timeout_at: "2024-01-01T12:00:00Z"
  context_hash: "abc123" # For resumption validation
```

## API Endpoints

### Get Pending User Inputs
```
GET /api/activities/user-inputs/pending
Response:
{
  "pending": [
    {
      "activity_id": "act_123",
      "type": "user_input_question",
      "prompt": "Enhanced prompt from Cortex",
      "context": {...},
      "options": [...],
      "created_at": "2024-01-01T10:00:00Z",
      "timeout_at": "2024-01-01T10:05:00Z"
    }
  ]
}
```

### Submit User Response
```
POST /api/activities/user-inputs/{activity_id}/respond
Body:
{
  "response": "approved" | {custom_object},
  "justification": "Optional justification"
}
```

## Configuration

### Activity Registration
```go
// In activity registry
RegisterActivity("user_input_question", NewUserInputQuestionHandler)
RegisterActivity("user_input_review", NewUserInputReviewHandler)
RegisterActivity("user_input_decision", NewUserInputDecisionHandler)
```

### Cortex Configuration
```yaml
cortex:
  endpoint: "http://cortex-service:8080"
  default_model: "gpt-4"
  user_input:
    max_context_size: 10000
    response_timeout: 300
    enable_streaming: false
```

## Error Handling

### Timeout Behavior
- Configurable timeout per activity
- Default timeout: 5 minutes
- On timeout: Activity fails or uses default response (configurable)

### Cortex Failures
- Retry logic with exponential backoff
- Fallback to simple prompt without enhancement
- Activity can continue with degraded functionality

## Security Considerations

1. **Response Validation**: All user responses validated against schema
2. **Context Sanitization**: Sensitive data removed before Cortex calls
3. **Rate Limiting**: Per-user rate limits on response submissions
4. **Audit Trail**: All user inputs logged with timestamp and user ID

## Future Enhancements

1. **Multi-user Approval**: Require multiple users to respond
2. **Conditional Routing**: Different paths based on response
3. **Response Templates**: Pre-defined response structures
4. **Real-time Updates**: WebSocket support for instant notifications
5. **Mobile Support**: Push notifications for pending inputs
6. **Batch Operations**: Handle multiple similar inputs together