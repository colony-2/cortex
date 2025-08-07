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

## UI Integration

### Overview
The UI needs to display pending user input requests across all boxes and allow users to respond. This uses a single Server-Sent Events (SSE) endpoint for real-time updates across all boxes, avoiding the need to poll multiple endpoints.

### Cortex API Endpoints

#### Server-Sent Events Stream (Comet Pattern)
```
GET /api/user-inputs/stream
Headers:
  Authorization: Bearer {token}
  Accept: text/event-stream

Event Stream Format:
event: initial
data: {
  "pending_inputs": [
    {
      "response_id": "resp_456",
      "box_id": "box_123",
      "box_name": "Production Deploy",
      "activity_id": "act_123",
      "workflow_id": "wf_789",
      "type": "question|review|decision",
      "prompt": "Select deployment strategy",
      "priority": "normal|high|urgent",
      "created_at": "2024-01-01T10:00:00Z",
      "expires_at": "2024-01-01T10:05:00Z"
    }
  ]
}

event: user_input_requested
data: {
  "response_id": "resp_789",
  "box_id": "box_456",
  "box_name": "Data Pipeline",
  "type": "review",
  "prompt": "Review data transformation",
  "priority": "high",
  "created_at": "2024-01-01T10:10:00Z"
}

event: user_input_completed
data: {
  "response_id": "resp_456",
  "box_id": "box_123",
  "responded_by": "user_123",
  "response": "approved"
}

event: user_input_expired
data: {
  "response_id": "resp_456",
  "box_id": "box_123"
}

event: heartbeat
data: {"timestamp": "2024-01-01T10:15:00Z"}
```

#### List All Pending User Inputs (Fallback/Initial Load)
```
GET /api/user-inputs/pending
Headers:
  Authorization: Bearer {token}
Query Parameters:
  box_ids: comma-separated list of box IDs (optional, defaults to all user's boxes)
  priority: normal|high|urgent (optional)
  type: question|review|decision (optional)

Response:
{
  "pending_inputs": [
    {
      "response_id": "resp_456",
      "box_id": "box_123",
      "box_name": "Production Deploy",
      "activity_id": "act_123",
      "workflow_id": "wf_789",
      "type": "decision",
      "prompt": "Select deployment strategy",
      "context_summary": "3 artifacts available",
      "priority": "high",
      "created_at": "2024-01-01T10:00:00Z",
      "expires_at": "2024-01-01T10:05:00Z"
    },
    {
      "response_id": "resp_789",
      "box_id": "box_456",
      "box_name": "Data Pipeline",
      "activity_id": "act_456",
      "workflow_id": "wf_012",
      "type": "review",
      "prompt": "Review ETL configuration",
      "context_summary": "2 artifacts available",
      "priority": "normal",
      "created_at": "2024-01-01T09:55:00Z",
      "expires_at": "2024-01-01T10:10:00Z"
    }
  ],
  "total": 2,
  "by_box": {
    "box_123": 1,
    "box_456": 1
  },
  "by_priority": {
    "high": 1,
    "normal": 1
  }
}
```

#### Get Single User Input Details
```
GET /api/user-inputs/{response_id}
Headers:
  Authorization: Bearer {token}

Response:
{
  "response_id": "resp_456",
  "box_id": "box_123",
  "box_name": "Production Deploy",
  "activity_id": "act_123",
  "workflow_id": "wf_789",
  "type": "review",
  "prompt": "Review generated configuration",
  "context": {
    "artifacts": [
      {
        "path": "config/generated.yaml",
        "content": "...",
        "diff": "...",
        "highlights": []
      }
    ],
    "metadata": {
      "activity_name": "Config Review",
      "created_by": "system"
    }
  },
  "response_schema": {
    "type": "choice",
    "options": [
      {"id": "approve", "label": "Approve"},
      {"id": "reject", "label": "Reject"},
      {"id": "request_changes", "label": "Request Changes"}
    ],
    "require_justification": true
  },
  "created_at": "2024-01-01T10:00:00Z",
  "expires_at": "2024-01-01T10:05:00Z",
  "status": "pending"
}
```

#### Submit User Response
```
POST /api/user-inputs/{response_id}/respond
Headers:
  Authorization: Bearer {token}
Body:
{
  "response": "approved" | "rejected" | {custom_value},
  "justification": "Looks good, proceeding with blue-green",
  "metadata": {
    "reviewed_at": "2024-01-01T10:02:00Z",
    "client": "web-ui"
  }
}

Response:
{
  "success": true,
  "box_id": "box_123",
  "activity_id": "act_123",
  "workflow_status": "resuming"
}
```

#### Cancel User Input Request
```
POST /api/user-inputs/{response_id}/cancel
Headers:
  Authorization: Bearer {token}
Body:
{
  "reason": "Workflow cancelled by user"
}

Response:
{
  "success": true,
  "box_id": "box_123",
  "activity_id": "act_123"
}
```

### UI Implementation

#### SSE Connection Management
```typescript
// Single SSE connection for all user inputs across all boxes
class UserInputStreamManager {
  private eventSource: EventSource;
  private reconnectTimeout: number = 5000;
  
  connect() {
    this.eventSource = new EventSource('/api/user-inputs/stream', {
      withCredentials: true
    });
    
    this.eventSource.addEventListener('initial', (e) => {
      const data = JSON.parse(e.data);
      this.handleInitialLoad(data.pending_inputs);
    });
    
    this.eventSource.addEventListener('user_input_requested', (e) => {
      const input = JSON.parse(e.data);
      this.notifyNewInput(input);
    });
    
    this.eventSource.addEventListener('user_input_completed', (e) => {
      const data = JSON.parse(e.data);
      this.removeInput(data.response_id);
    });
    
    this.eventSource.addEventListener('user_input_expired', (e) => {
      const data = JSON.parse(e.data);
      this.handleExpired(data.response_id);
    });
    
    this.eventSource.onerror = () => {
      this.reconnect();
    };
  }
  
  private reconnect() {
    setTimeout(() => this.connect(), this.reconnectTimeout);
  }
}
```

#### Global Notification Component
```typescript
// Displays aggregated user inputs from all boxes
interface GlobalUserInputIndicator {
  totalPending: number;
  byBox: Map<string, number>;
  highPriority: number;
  onClick: () => void; // Opens input panel
}

// Individual input in the panel
interface UserInputItem {
  responseId: string;
  boxId: string;
  boxName: string;
  type: 'question' | 'review' | 'decision';
  prompt: string;
  priority: 'normal' | 'high' | 'urgent';
  expiresIn: number;
  onSelect: () => void;
}
```

#### User Input Panel
```typescript
// Panel showing all pending inputs across boxes
interface UserInputPanel {
  inputs: UserInputItem[];
  selectedInput?: {
    responseId: string;
    boxName: string;
    prompt: string;
    context: {
      artifacts: ArtifactView[];
      metadata: Record<string, any>;
    };
    responseSchema: {
      type: 'choice' | 'text' | 'structured';
      options?: Option[];
      requireJustification?: boolean;
    };
  };
  onSubmit: (responseId: string, response: any, justification?: string) => void;
  onCancel: (responseId: string) => void;
  onFilter: (boxId?: string, priority?: string) => void;
}
```

### Signal Flow with UI

1. Activity sends `UserInputRequested` signal to Cortex
2. Cortex stores request and sends `UserInputReady` signal back to activity
3. Cortex pushes SSE event to all connected UI clients for that user
4. UI receives event and updates notification badge/counter
5. User clicks notification to see all pending inputs
6. UI fetches full details for selected input via REST API
7. User submits response through UI
8. Cortex sends `UserInputReceived` signal to activity
9. Cortex pushes SSE event to update all UI clients
10. Activity resumes with user response

### Connection and Performance Considerations

#### SSE Connection Management
- Single persistent connection per user session
- Automatic reconnection with exponential backoff
- Heartbeat events every 30 seconds to detect stale connections
- Initial event sends all pending inputs on connection

#### Scalability
- SSE connection handled by dedicated streaming service
- Redis pub/sub for distributing events across server instances
- Connection limit per user (prevent multiple tabs from overwhelming)
- Graceful degradation to polling if SSE unavailable

#### Caching Strategy
```yaml
cache:
  pending_inputs:
    ttl: 60 # seconds
    invalidate_on:
      - user_input_requested
      - user_input_completed
      - user_input_expired
  input_details:
    ttl: 300 # seconds
    key_pattern: "user_input:{response_id}"
```

### Cortex Server SSE Implementation
```go
// server/cortex/pkg/api/user_input_stream.go
type UserInputStreamer struct {
    Store        UserInputStore
    PubSub       pubsub.Client
    Connections  map[string]*SSEConnection
}

func (s *UserInputStreamer) HandleStream(w http.ResponseWriter, r *http.Request) {
    userID := getUserFromContext(r.Context())
    
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    // Create SSE connection
    conn := &SSEConnection{
        Writer:  w,
        Flusher: w.(http.Flusher),
        UserID:  userID,
    }
    
    // Send initial state
    pending := s.Store.GetPendingForUser(userID)
    conn.SendEvent("initial", map[string]interface{}{
        "pending_inputs": pending,
    })
    
    // Subscribe to updates
    sub := s.PubSub.Subscribe(fmt.Sprintf("user:%s:inputs", userID))
    defer sub.Close()
    
    // Handle events
    for {
        select {
        case msg := <-sub.Channel():
            conn.SendEvent(msg.Type, msg.Data)
        case <-r.Context().Done():
            return
        case <-time.After(30 * time.Second):
            conn.SendEvent("heartbeat", map[string]interface{}{
                "timestamp": time.Now(),
            })
        }
    }
}

// Triggered when new user input is created
func (s *UserInputStreamer) NotifyUserInputRequested(input UserInput) {
    s.PubSub.Publish(
        fmt.Sprintf("user:%s:inputs", input.UserID),
        Message{
            Type: "user_input_requested",
            Data: input.ToSummary(),
        },
    )
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
      - "sse"  # Server-sent events to UI
      - "email" # Email notifications for urgent inputs
  sse:
    endpoint: "/api/user-inputs/stream"
    heartbeat_interval: 30
    max_connections_per_user: 5
    redis_pubsub:
      endpoint: "redis://localhost:6379"
      channel_prefix: "user_inputs"
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