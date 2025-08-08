# Input Op Specification

## Overview
Input ops use Temporal signals directly. Recipe context is available without configuration. Cortex monitors signals and manages UI interactions.

## Op Type: Input Form

### Single Question Format
```yaml
type: input
config:
  question: "Do you approve this deployment?"
  type: "multiple_choice"
  options:
    - value: "approve"
    - value: "reject"
```

### Multiple Question Format
```yaml
type: input
config:
  title: "Deployment Configuration Review"  # Optional
  context:
    artifacts:
      - path: "generated/config.yaml"
  fields:
    - id: "deployment_strategy"
      type: "multiple_choice"
      question: "Select deployment strategy"
      required: true
      options:
        - value: "blue_green"
          label: "Blue-green deployment"  # Optional label
        - value: "canary"
    - id: "urgency"
      type: "linear_scale"
      question: "How urgent is this deployment?"
      required: true
      scale:
        min: 1
        max: 5
        min_label: "Can wait"
        max_label: "Critical"
  timeout: 300
```

## Field Types
- `short_answer` - Single line text
- `paragraph_text` - Multi-line text
- `multiple_choice` - Single selection
- `checkboxes` - Multiple selections
- `dropdown` - Dropdown selection
- `linear_scale` - Numeric scale rating
- `multiple_choice_grid` - Grid of radio buttons
- `checkbox_grid` - Grid of checkboxes
- `date` - Date picker
- `time` - Time picker
- `file_upload` - File attachment

## Artifact Resolution

### Static
```yaml
artifacts:
  - path: "config.yaml"
```

### From Previous Activity
```yaml
artifacts_from_output: "previous_activity.output_field"
```

### Glob Patterns
```yaml
artifacts_glob:
  - pattern: "**/*.yaml"
```

## Response Data Access

### Single Question
```yaml
steps:
  - name: approval_check
    type: input
    config:
      question: "Do you approve?"
      type: "multiple_choice"
      options:
        - value: "approve"
        - value: "reject"
  
  - name: deploy
    type: deployment
    config:
      proceed: "{{ .Steps.approval_check.outputs.response }}"
```

### Multiple Questions
```yaml
steps:
  - name: deployment_review
    type: input
    config:
      fields:
        - id: "strategy"
          type: "multiple_choice"
          question: "Select strategy"
          options:
            - value: "blue_green"
            - value: "canary"

  - name: deploy
    type: deployment
    config:
      strategy: "{{ .Steps.deployment_review.outputs.fields.strategy }}"
```

## Temporal Signal Types

### Signals Sent by Activity
```go
// InputRequestSignal - Activity requests user input
type InputRequestSignal struct {
    ActivityID     string
    WorkflowID     string
    Form          InputForm
    Timeout       time.Duration
}

// InputCancelSignal - Activity cancels pending input
type InputCancelSignal struct {
    ActivityID string
    ResponseID string
    Reason     string
}
```

### Signals Sent by Cortex
```go
// InputResponseSignal - User submitted form
type InputResponseSignal struct {
    ActivityID    string
    ResponseID    string
    FormResponse  FormResponse
    UserID        string
    RespondedAt   time.Time
}

// InputTimeoutSignal - Input request timed out
type InputTimeoutSignal struct {
    ActivityID string
    ResponseID string
    TimedOutAt time.Time
}
```

## Go Types
```go
type InputForm struct {
    // Single question
    Question       string
    Type           FieldType
    Options        []Option
    
    // Multi-field
    Title          string
    Fields         []FormField
    
    // Common
    Context        FormContext
    Timeout        int
}

type FormField struct {
    ID             string
    Type           FieldType
    Question       string
    Required       bool
    Placeholder    string
    Options        []Option
    Validation     FieldValidation
}

type Option struct {
    Value string
    Label string  // Optional - defaults to Value
}

type FormResponse struct {
    ActivityID     string
    UserID         string
    Response       interface{}  // Single question
    Fields         map[string]interface{}  // Multi-field
    SubmittedAt    time.Time
    TimeToComplete int
}
```

## API Endpoints

### SSE Stream
```
GET /api/user-inputs/stream
Event: initial|input_requested|input_completed|input_expired|heartbeat
```

### List Pending
```
GET /api/user-inputs/pending
```

### Get Details
```
GET /api/user-inputs/{response_id}
```

### Submit Response
```
POST /api/user-inputs/{response_id}/respond
Body: {
  "fields": {
    "field_id": value
  }
}
```

### Cancel
```
POST /api/user-inputs/{response_id}/cancel
```

## Implementation

### Input Activity (Recipe Activity)
```go
// InputActivity is a RegisterableActivity executed by recipe-worker
type InputActivity struct {
    // Temporal context injected by framework
    temporalContext workflow.Context
}

// Execute runs within recipe workflow context
func (a *InputActivity) Execute(ctx context.Context, config InputConfig, input map[string]interface{}) (map[string]interface{}, error) {
    // Use injected workflow context to start child workflow
    childID := fmt.Sprintf("input-%s-%d", workflow.GetInfo(a.temporalContext).WorkflowExecution.ID, time.Now().Unix())
    
    childOptions := workflow.ChildWorkflowOptions{
        WorkflowID: childID,
        TaskQueue:  "input-handlers",
        SearchAttributes: map[string]interface{}{
            "InputWorkflowType": "user-input",
            "ParentWorkflowID":  workflow.GetInfo(a.temporalContext).WorkflowExecution.ID,
            "InputStatus":       "pending",
        },
    }
    
    wfCtx := workflow.WithChildOptions(a.temporalContext, childOptions)
    
    // Input workflow parameters
    inputParams := InputWorkflowParams{
        Form:       a.buildForm(config, input),
        Timeout:    config.Timeout,
        BoxID:      input["box_id"].(string),
        ActivityID: input["activity_id"].(string),
    }
    
    var result InputWorkflowResult
    err := workflow.ExecuteChildWorkflow(wfCtx, "InputCollectionWorkflow", inputParams).Get(wfCtx, &result)
    
    if err != nil {
        if config.DefaultOnTimeout != nil && temporal.IsTimeoutError(err) {
            return map[string]interface{}{"response": config.DefaultOnTimeout}, nil
        }
        return nil, err
    }
    
    // Return response in recipe format
    return map[string]interface{}{
        "response": result.FormResponse,
        "user_id":  result.UserID,
        "metadata": result.Metadata,
    }, nil
}

// GetManagementService returns optional management service
func (a *InputActivity) GetManagementService() ManagementService {
    return &InputManagementService{
        workflowType: "InputCollectionWorkflow",
    }
}

// InputCollectionWorkflow handles the actual input collection
func InputCollectionWorkflow(ctx workflow.Context, params InputWorkflowParams) (InputWorkflowResult, error) {
    workflowID := workflow.GetInfo(ctx).WorkflowExecution.ID
    
    // Update search attributes with form details
    workflow.UpsertSearchAttributes(ctx, map[string]interface{}{
        "InputFormTitle": params.Form.Title,
        "InputBoxID":     params.BoxID,
        "InputCreatedAt": workflow.Now(ctx).Format(time.RFC3339),
        "InputExpiresAt": workflow.Now(ctx).Add(params.Timeout).Format(time.RFC3339),
    })
    
    // Wait for user response signal
    responseChan := workflow.GetSignalChannel(ctx, "user-response")
    timeoutCtx, cancel := workflow.WithCancel(ctx)
    
    // Setup timeout
    workflow.Go(timeoutCtx, func(ctx workflow.Context) {
        workflow.Sleep(ctx, params.Timeout)
        cancel()
    })
    
    var response UserResponseSignal
    responseChan.Receive(timeoutCtx, &response)
    
    if timeoutCtx.Err() != nil {
        return InputWorkflowResult{}, temporal.NewApplicationError("input timeout", "TIMEOUT")
    }
    
    // Update status
    workflow.UpsertSearchAttributes(ctx, map[string]interface{}{
        "InputStatus": "completed",
    })
    
    return InputWorkflowResult{
        FormResponse: response.Fields,
        UserID:       response.UserID,
        Metadata:     response.Metadata,
    }, nil
}
```

### Management Service Interface
```go
// ManagementService is auto-discovered from activities
type ManagementService interface {
    // Returns HTTP routes this service provides
    GetRoutes() []Route
    
    // Initialize with injected dependencies
    Initialize(deps ServiceDependencies)
}

type ServiceDependencies struct {
    // Temporal client injected by framework
    TemporalClient client.Client
    // SSE manager for real-time updates
    SSEManager *SSEManager
}

type Route struct {
    Method  string
    Path    string
    Handler http.HandlerFunc
}

// InputManagementService implements ManagementService
type InputManagementService struct {
    workflowType string
    deps         ServiceDependencies
}

func (s *InputManagementService) Initialize(deps ServiceDependencies) {
    s.deps = deps
}

func (s *InputManagementService) GetRoutes() []Route {
    return []Route{
        {Method: "GET", Path: "/pending", Handler: s.ListPending},
        {Method: "GET", Path: "/stream", Handler: s.SSEStream},
        {Method: "GET", Path: "/{workflowID}", Handler: s.GetDetails},
        {Method: "POST", Path: "/{workflowID}/respond", Handler: s.SubmitResponse},
    }
}

func (s *InputManagementService) ListPending(w http.ResponseWriter, r *http.Request) {
    // Query for pending input workflows using injected client
    query := `InputWorkflowType = "user-input" AND InputStatus = "pending"`
    request := &workflowservice.ListWorkflowExecutionsRequest{
        Query: query,
    }
    
    var pending []PendingInput
    iter, _ := s.deps.TemporalClient.ListWorkflow(context.Background(), request)
    
    for iter.HasNext() {
        exec, _ := iter.Next()
        attrs := exec.SearchAttributes.IndexedFields
        
        pending = append(pending, PendingInput{
            WorkflowID:  exec.Execution.WorkflowId,
            BoxID:       attrs["InputBoxID"].(string),
            FormTitle:   attrs["InputFormTitle"].(string),
            CreatedAt:   attrs["InputCreatedAt"].(string),
            ExpiresAt:   attrs["InputExpiresAt"].(string),
        })
    }
    
    json.NewEncoder(w).Encode(pending)
}

func (s *InputManagementService) SubmitResponse(w http.ResponseWriter, r *http.Request) {
    workflowID := chi.URLParam(r, "workflowID")
    
    var submission struct {
        Fields   map[string]interface{} `json:"fields"`
        Metadata map[string]interface{} `json:"metadata"`
    }
    json.NewDecoder(r.Body).Decode(&submission)
    
    // Send signal using injected client
    err := s.deps.TemporalClient.SignalWorkflow(context.Background(), 
        workflowID, "", "user-response", 
        UserResponseSignal{
            Fields:      submission.Fields,
            UserID:      r.Header.Get("X-User-ID"),
            RespondedAt: time.Now(),
            Metadata:    submission.Metadata,
        })
    
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    
    // Notify via SSE
    s.deps.SSEManager.Broadcast(SSEEvent{
        Type: "input_completed",
        Data: map[string]string{"workflow_id": workflowID},
    })
    
    json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
```

### Cortex Auto-Discovery
```go
// Cortex auto-discovers management services from activities
type CortexServer struct {
    router     *chi.Mux
    registry   ActivityRegistry
    // Temporal client already available in context
}

func (s *CortexServer) Start() {
    // Auto-discover management services from registered activities
    activities := s.registry.GetActivities()
    
    for _, activity := range activities {
        if provider, ok := activity.(ManagementServiceProvider); ok {
            service := provider.GetManagementService()
            if service != nil {
                // Initialize with dependencies
                service.Initialize(ServiceDependencies{
                    TemporalClient: s.GetTemporalClient(), // Already available
                    SSEManager:     s.GetSSEManager(),
                })
                
                // Mount routes
                for _, route := range service.GetRoutes() {
                    s.router.Method(route.Method, route.Path, route.Handler)
                }
            }
        }
    }
    
    // Input workflow is registered automatically when InputActivity is discovered
}

// ActivityRegistry provides access to all registered activities
type ActivityRegistry interface {
    GetActivities() []RegisterableActivity
    RegisterActivity(activity RegisterableActivity)
}

// ManagementServiceProvider is optionally implemented by activities
type ManagementServiceProvider interface {
    GetManagementService() ManagementService
}
```

### Data Types
```go
type InputWorkflowParams struct {
    Form       InputForm
    Timeout    time.Duration
    BoxID      string
    ActivityID string
}

type InputWorkflowResult struct {
    FormResponse map[string]interface{}
    UserID       string
    Metadata     map[string]interface{}
}

type UserResponseSignal struct {
    Fields      map[string]interface{}
    UserID      string
    RespondedAt time.Time
    Metadata    map[string]interface{}
}

type PendingInput struct {
    WorkflowID string
    BoxID      string
    FormTitle  string
    CreatedAt  string
    ExpiresAt  string
}
```

## UI Requirements
1. Single SSE connection for all boxes
2. Display pending input count/badge on each box/cell in ui. 
3. Add new tabs to cell information that shows the pending input(s). Clicking on the badge opens the tab.
4. Fetch details and render form
5. Submit response via API that fires signal via management service.
