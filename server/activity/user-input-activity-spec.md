# User Input Activity Specification

## Overview
User Input Activities are interactive activities that pause workflow execution to gather user responses through the Cortex server (vibethis core server). These activities enable human-in-the-loop workflows where user feedback, decisions, or reviews are required. The activity server communicates with Cortex to coordinate user interactions and manage workflow state.

## Core Components

### Activity Types

#### 1. Question Activity (`user_input_question`)
Presents a question to the user and waits for their response.

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
  display_mode: "diff" # How to display artifacts in UI
```

#### 3. Decision Activity (`user_input_decision`)
Presents a structured decision point to the user.

```yaml
type: user_input_decision
config:
  decision_prompt: "Select deployment strategy"
  context_artifacts:
    - "metrics/current_state.json"
    - "deployment/options.yaml"
  options:
    - id: "blue_green"
      description: "Blue-green deployment"
      details: "Switch traffic all at once to new version"
    - id: "canary"
      description: "Canary deployment (10% initial)"
      details: "Gradually roll out to percentage of users"
    - id: "rolling"
      description: "Rolling update"
      details: "Update instances one by one"
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
    DisplayMode    string
}

type UserResponse struct {
    ActivityID     string
    UserID         string
    Response       interface{}
    Timestamp      time.Time
    Justification  string
}
```

## Signal-based Communication Architecture

User input activities use signals for all communication between the Activity server and Cortex. This provides asynchronous, event-driven coordination that naturally fits with workflow pause/resume patterns.

### Signal Definitions

#### Activity → Cortex Signals
```go
// UserInputRequested - Activity requests user input
type UserInputRequested struct {
    ActivityID     string
    WorkflowID     string
    Type          UserInputType
    Prompt        string
    Context       map[string]interface{}
    ResponseSchema ResponseSchema
    Timeout       time.Duration
}

// UserInputCancelled - Activity cancels pending input
type UserInputCancelled struct {
    ActivityID string
    ResponseID string
    Reason     string
}
```

#### Cortex → Activity Signals
```go
// UserInputReady - Cortex has prepared the user input session
type UserInputReady struct {
    ActivityID  string
    ResponseID  string
    SessionID   string
    DisplayURL  string
    ExpiresAt   time.Time
}

// UserInputReceived - User has provided response
type UserInputReceived struct {
    ActivityID    string
    ResponseID    string
    Response      interface{}
    Justification string
    UserID        string
    RespondedAt   time.Time
}

// UserInputTimeout - Input request has timed out
type UserInputTimeout struct {
    ActivityID string
    ResponseID string
    TimedOutAt time.Time
}

// UserInputError - Error processing user input
type UserInputError struct {
    ActivityID string
    ResponseID string
    Error      string
    ErrorCode  string
}
```

### Signal Flow
1. Activity sends `UserInputRequested` signal to Cortex
2. Cortex responds with `UserInputReady` signal containing session info
3. Activity enters wait state, listening for response signals
4. When user responds, Cortex sends `UserInputReceived` signal
5. Activity processes response and continues workflow

### Signal Implementation Details

#### Signal Handler Registration
```go
// server/activity/pkg/activity/user_input_handler.go
func RegisterUserInputHandlers(dispatcher *signals.Dispatcher) {
    dispatcher.RegisterHandler("UserInputReady", HandleUserInputReady)
    dispatcher.RegisterHandler("UserInputReceived", HandleUserInputReceived)
    dispatcher.RegisterHandler("UserInputTimeout", HandleUserInputTimeout)
    dispatcher.RegisterHandler("UserInputError", HandleUserInputError)
}

func HandleUserInputReceived(ctx context.Context, signal UserInputReceived) error {
    // Resume activity with user response
    activity := GetActivity(signal.ActivityID)
    return activity.Resume(signal.Response, signal.Justification)
}
```

#### Signal Retry and Reliability
```go
type SignalConfig struct {
    MaxRetries     int
    RetryBackoff   time.Duration
    Timeout        time.Duration
    PersistHistory bool
}

// Signals are persisted and retried automatically
var UserInputSignalConfig = SignalConfig{
    MaxRetries:     3,
    RetryBackoff:   time.Second * 5,
    Timeout:        time.Minute * 5,
    PersistHistory: true,
}
```

### Cortex Server Integration
```go
// server/cortex/pkg/activity/user_input.go
type UserInputManager struct {
    Store         UserInputStore
    Notifier      UserNotifier
    SignalClient  signals.Client
    SignalHandler signals.Handler
}

// Initialize signal handlers on Cortex side
func (m *UserInputManager) RegisterHandlers() {
    m.SignalHandler.Register("UserInputRequested", m.HandleUserInputRequest)
    m.SignalHandler.Register("UserInputCancelled", m.HandleUserInputCancelled)
}

func (m *UserInputManager) HandleUserInputRequest(ctx context.Context, signal signals.Signal) error {
    req := signal.Payload.(UserInputRequested)
    
    // Store request with metadata
    session := m.Store.CreateSession(SessionConfig{
        ActivityID:     req.ActivityID,
        WorkflowID:     req.WorkflowID,
        Type:          req.Type,
        Prompt:        req.Prompt,
        Context:       req.Context,
        ResponseSchema: req.ResponseSchema,
        Timeout:       req.Timeout,
    })
    
    // Notify user through available channels
    m.Notifier.NotifyPendingInput(session)
    
    // Send ready signal back to activity
    return m.SignalClient.Send(ctx, signals.Signal{
        Type: "UserInputReady",
        Payload: UserInputReady{
            ActivityID:  req.ActivityID,
            ResponseID:  session.ResponseID,
            SessionID:   session.ID,
            DisplayURL:  m.generateDisplayURL(session),
            ExpiresAt:   session.ExpiresAt,
        },
    })
}

// Called when user submits response through UI
func (m *UserInputManager) ProcessUserResponse(ctx context.Context, responseID string, response UserResponse) error {
    session := m.Store.GetSessionByResponseID(responseID)
    if session == nil {
        return fmt.Errorf("session not found")
    }
    
    // Validate response against schema
    if err := m.validateResponse(response, session.ResponseSchema); err != nil {
        return err
    }
    
    // Send response signal to activity
    return m.SignalClient.Send(ctx, signals.Signal{
        Type: "UserInputReceived",
        Payload: UserInputReceived{
            ActivityID:    session.ActivityID,
            ResponseID:    responseID,
            Response:      response.Value,
            Justification: response.Justification,
            UserID:        response.UserID,
            RespondedAt:   time.Now(),
        },
    })
}
```

### Activity Server Integration
```go
// server/activity/pkg/activity/user_input_activity.go
type UserInputActivity struct {
    SignalClient  signals.Client
    SignalWaiter  signals.Waiter
    Config        UserInputConfig
}

func (a *UserInputActivity) Execute(ctx context.Context, input ActivityInput) (ActivityOutput, error) {
    // Send signal to Cortex requesting user input
    requestSignal := signals.Signal{
        Type: "UserInputRequested",
        Payload: UserInputRequested{
            ActivityID:     a.ID,
            WorkflowID:     a.WorkflowID,
            Type:          a.Config.Type,
            Prompt:        a.Config.Prompt,
            Context:       a.prepareContext(input),
            ResponseSchema: a.Config.ResponseSchema,
            Timeout:       a.Config.Timeout,
        },
    }
    
    if err := a.SignalClient.Send(ctx, requestSignal); err != nil {
        return nil, fmt.Errorf("failed to request user input: %w", err)
    }
    
    // Wait for response signal
    responseSignal, err := a.SignalWaiter.WaitFor(ctx, []string{
        "UserInputReceived",
        "UserInputTimeout",
        "UserInputError",
    }, a.Config.Timeout)
    
    if err != nil {
        return nil, fmt.Errorf("failed waiting for user response: %w", err)
    }
    
    // Handle response based on signal type
    switch responseSignal.Type {
    case "UserInputReceived":
        received := responseSignal.Payload.(UserInputReceived)
        return ActivityOutput{
            "response": received.Response,
            "justification": received.Justification,
            "user_id": received.UserID,
        }, nil
        
    case "UserInputTimeout":
        if a.Config.DefaultOnTimeout != nil {
            return ActivityOutput{"response": a.Config.DefaultOnTimeout}, nil
        }
        return nil, fmt.Errorf("user input timeout")
        
    case "UserInputError":
        errorPayload := responseSignal.Payload.(UserInputError)
        return nil, fmt.Errorf("user input error: %s", errorPayload.Error)
        
    default:
        return nil, fmt.Errorf("unexpected signal type: %s", responseSignal.Type)
    }
}
```

## Workflow Integration

### Execution Flow
1. Activity executor encounters user input activity
2. Activity sends `UserInputRequested` signal to Cortex
3. Cortex prepares user session and sends `UserInputReady` signal back
4. Activity enters wait state, listening for response signals
5. User receives notification and accesses Cortex UI
6. User submits response through Cortex UI
7. Cortex validates response and sends `UserInputReceived` signal
8. Activity receives signal and resumes with user response as output

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

## User Interface Integration

### Cortex UI Endpoints
While the activity-to-Cortex communication uses signals, the Cortex server provides UI endpoints for users to interact with pending inputs:

#### View Pending User Input
```
GET /ui/user-inputs/{response_id}
```
Displays formatted prompt, context, and response options to the user.

#### Submit User Response
```
POST /ui/user-inputs/{response_id}/respond
Body:
{
  "response": "approved" | {custom_object},
  "justification": "Optional justification"
}
```
Triggers `UserInputReceived` signal to activity server.

## Configuration

### Activity Registration
```go
// In activity registry
RegisterActivity("user_input_question", NewUserInputQuestionHandler)
RegisterActivity("user_input_review", NewUserInputReviewHandler)
RegisterActivity("user_input_decision", NewUserInputDecisionHandler)
```

### Signal Configuration
```yaml
signals:
  broker: "redis" # or "kafka", "rabbitmq"
  endpoint: "redis://localhost:6379"
  namespace: "vibethis"
  user_input:
    queue: "user_input_signals"
    retry_policy:
      max_attempts: 3
      backoff_seconds: 5
    timeout_seconds: 300
    persist_history: true

# Cortex server configuration for handling user inputs
cortex:
  signal_endpoint: "signals://cortex"
  user_input:
    max_context_size: 10000
    default_timeout: 300
    notification_channels:
      - "ui"
      - "email"
```

## Error Handling

### Timeout Behavior
- Configurable timeout per activity via signal metadata
- Default timeout: 5 minutes
- On timeout: Cortex sends `UserInputTimeout` signal
- Activity can handle timeout signal to fail, retry, or use default

### Signal Failures
- Automatic retry with exponential backoff (configured in signal broker)
- Dead letter queue for persistent failures
- `UserInputError` signal sent for non-recoverable errors
- Activity receives error signal and decides how to proceed

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