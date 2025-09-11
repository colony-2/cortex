# Recipe Worker Execution Abstraction Specification

## Overview

This specification defines an execution abstraction layer that enables the recipe-worker system to support multiple execution environments beyond Temporal. The abstraction captures the core concepts and patterns currently leveraged from Temporal while providing a pluggable interface for alternative execution backends.

## Current Temporal Usage Analysis

### Key Temporal Concepts Leveraged

1. **Workflow Orchestration**: Long-running, durable workflows that coordinate multiple activities
2. **Activity Execution**: Individual units of work with retry policies and timeout handling
3. **Child Workflows**: Nested workflow execution with parent-child relationships
4. **Signal Handling**: Asynchronous communication between external systems and workflows
5. **State Management**: Automatic persistence and recovery of workflow state
6. **Task Queues**: Routing and load balancing of work across workers
7. **History & Observability**: Complete execution history, search attributes, and monitoring
8. **Timeouts & Retries**: Configurable timeout and retry behaviors
9. **Concurrency Control**: Parallel execution with coordination primitives

### Current Usage Patterns

#### 1. Recipe Execution (`recipe-worker`)
- **Pattern**: Main workflow orchestrator
- **Usage**: Compiles YAML recipes into executable workflows
- **Key Features**: State machines, sequences, parallel execution, activity registry

#### 2. Child Recipe Invocation (`server/ops/recipe`)
- **Pattern**: Child workflow execution
- **Usage**: `workflow.ExecuteChildWorkflow()` for nested recipe calls
- **Key Features**: Context propagation, timeout inheritance, result aggregation

#### 3. User Input Collection (`server/ops/input`)
- **Pattern**: Signal-based workflow with timeout
- **Usage**: `workflow.GetSignalChannel()` for user interaction
- **Key Features**: Timeout handling, search attributes, cancellation

#### 4. Execution History (`recipe-history`)
- **Pattern**: Query and transformation layer
- **Usage**: Temporal API calls for execution history retrieval
- **Key Features**: Job listing, activity tracking, status filtering

## Execution Abstraction API

### Core Interfaces

```go
// ExecutionEngine defines the main abstraction for workflow execution
type ExecutionEngine interface {
    // Start the execution engine
    Start(ctx context.Context) error
    
    // Stop the execution engine
    Stop(ctx context.Context) error
    
    // Register a recipe with the execution engine
    RegisterRecipe(recipe *recipe.Recipe) error
    
    // Unregister a recipe
    UnregisterRecipe(recipeName string) error
    
    // Execute a recipe workflow
    ExecuteRecipe(ctx context.Context, params ExecutionParams) (*ExecutionHandle, error)
    
    // Get execution history
    GetExecutionHistory(ctx context.Context, filter HistoryFilter) ([]*ExecutionRecord, error)
    
    // Get execution status
    GetExecutionStatus(ctx context.Context, executionID string) (*ExecutionStatus, error)
}

// WorkflowContext provides workflow-specific execution context
type WorkflowContext interface {
    // Get workflow information
    GetInfo() WorkflowInfo
    
    // Execute an activity
    ExecuteActivity(activityName string, input interface{}) ActivityFuture
    
    // Execute a child workflow
    ExecuteChildWorkflow(workflowName string, input interface{}) WorkflowFuture
    
    // Create a timer
    CreateTimer(duration time.Duration) TimerFuture
    
    // Handle signals
    GetSignalChannel(signalName string) SignalChannel
    
    // Send signals to external workflows
    SignalExternalWorkflow(executionID, signalName string, data interface{}) error
    
    // Update search attributes
    UpsertSearchAttributes(attributes map[string]interface{}) error
    
    // Logging
    GetLogger() Logger
    
    // Current time (deterministic)
    Now() time.Time
}

// ActivityContext provides activity-specific execution context
type ActivityContext interface {
    // Get activity information
    GetInfo() ActivityInfo
    
    // Heartbeat for long-running activities
    Heartbeat(details interface{})
    
    // Logging
    GetLogger() Logger
    
    // Check if context is cancelled
    Done() <-chan struct{}
    
    // Get cancellation error
    Err() error
}

// ExecutionParams defines parameters for recipe execution
type ExecutionParams struct {
    RecipeName      string                 `json:"recipe_name"`
    ExecutionID     string                 `json:"execution_id,omitempty"`
    Inputs          map[string]interface{} `json:"inputs"`
    Timeout         time.Duration          `json:"timeout,omitempty"`
    RetryPolicy     *RetryPolicy           `json:"retry_policy,omitempty"`
    SearchAttributes map[string]interface{} `json:"search_attributes,omitempty"`
}

// ExecutionHandle represents a running execution
type ExecutionHandle interface {
    // Get execution ID
    GetID() string
    
    // Wait for completion
    Get(ctx context.Context) (map[string]interface{}, error)
    
    // Cancel execution
    Cancel(ctx context.Context) error
    
    // Terminate execution
    Terminate(ctx context.Context, reason string) error
    
    // Send signal to execution
    Signal(signalName string, data interface{}) error
    
    // Query execution state
    Query(queryName string, data interface{}) (interface{}, error)
}

// Future interfaces for async operations
type ActivityFuture interface {
    Get(ctx WorkflowContext) (interface{}, error)
}

type WorkflowFuture interface {
    Get(ctx WorkflowContext) (interface{}, error)
}

type TimerFuture interface {
    Get(ctx WorkflowContext) error
}

type SignalChannel interface {
    Receive(ctx WorkflowContext, valuePtr interface{}) bool
    ReceiveAsync(valuePtr interface{}) bool
}

// Execution history interfaces
type HistoryFilter struct {
    RecipeName       string    `json:"recipe_name,omitempty"`
    Status           string    `json:"status,omitempty"` // running, completed, failed, cancelled
    StartTimeFrom    time.Time `json:"start_time_from,omitempty"`
    StartTimeTo      time.Time `json:"start_time_to,omitempty"`
    SearchAttributes map[string]interface{} `json:"search_attributes,omitempty"`
    Limit            int       `json:"limit,omitempty"`
    NextPageToken    string    `json:"next_page_token,omitempty"`
}

type ExecutionRecord struct {
    ID               string                 `json:"id"`
    RecipeName       string                 `json:"recipe_name"`
    Status           ExecutionStatus        `json:"status"`
    StartTime        time.Time              `json:"start_time"`
    EndTime          *time.Time             `json:"end_time,omitempty"`
    Duration         *time.Duration         `json:"duration,omitempty"`
    Inputs           map[string]interface{} `json:"inputs"`
    Outputs          map[string]interface{} `json:"outputs,omitempty"`
    Error            string                 `json:"error,omitempty"`
    SearchAttributes map[string]interface{} `json:"search_attributes,omitempty"`
    Activities       []*ActivityExecution   `json:"activities,omitempty"`
}

type ExecutionStatus struct {
    State      string    `json:"state"` // running, completed, failed, cancelled, terminated
    StartTime  time.Time `json:"start_time"`
    EndTime    *time.Time `json:"end_time,omitempty"`
    Error      string    `json:"error,omitempty"`
}

type ActivityExecution struct {
    ID         string         `json:"id"`
    Name       string         `json:"name"`
    Status     ExecutionStatus `json:"status"`
    StartTime  time.Time      `json:"start_time"`
    EndTime    *time.Time     `json:"end_time,omitempty"`
    Duration   *time.Duration `json:"duration,omitempty"`
    Input      interface{}    `json:"input,omitempty"`
    Output     interface{}    `json:"output,omitempty"`
    Error      string         `json:"error,omitempty"`
    AttemptCount int          `json:"attempt_count"`
}

// Configuration and policies
type RetryPolicy struct {
    MaxAttempts            int           `json:"max_attempts"`
    InitialInterval        time.Duration `json:"initial_interval"`
    BackoffCoefficient     float64       `json:"backoff_coefficient"`
    MaximumInterval        time.Duration `json:"maximum_interval"`
    MaximumAttempts        int           `json:"maximum_attempts"`
    NonRetryableErrorTypes []string      `json:"non_retryable_error_types,omitempty"`
}

type WorkflowInfo struct {
    WorkflowType     string
    ExecutionID      string
    RunID            string
    TaskQueue        string
    WorkflowTimeout  time.Duration
    RunTimeout       time.Duration
    ExecutionTimeout time.Duration
}

type ActivityInfo struct {
    ActivityType     string
    ActivityID       string
    TaskQueue        string
    ScheduleToStartTimeout time.Duration
    StartToCloseTimeout    time.Duration
    HeartbeatTimeout       time.Duration
    ScheduledTime    time.Time
    StartedTime      time.Time
    Deadline         time.Time
    Attempt          int32
}

type Logger interface {
    Debug(msg string, keyvals ...interface{})
    Info(msg string, keyvals ...interface{})
    Warn(msg string, keyvals ...interface{})
    Error(msg string, keyvals ...interface{})
}
```

### Execution Engine Implementations

#### 1. TemporalExecutionEngine

The default implementation using Temporal.io:

```go
type TemporalExecutionEngine struct {
    client       client.Client
    workers      map[string]worker.Worker
    workerManager *WorkerManager
    logger       Logger
}
```

#### 2. LocalExecutionEngine

A local execution engine for development/testing:

```go
type LocalExecutionEngine struct {
    executions   map[string]*LocalExecution
    registry     *LocalRegistry
    scheduler    *LocalScheduler
    logger       Logger
}
```

#### 3. KubernetesExecutionEngine

A Kubernetes-based execution engine:

```go
type KubernetesExecutionEngine struct {
    client       kubernetes.Interface
    namespace    string
    registry     *K8sRegistry
    jobManager   *K8sJobManager
    logger       Logger
}
```

## Migration Strategy

### Phase 1: Abstraction Layer Introduction

1. **Interface Definition**: Implement the core execution abstraction interfaces
2. **Temporal Adapter**: Create a Temporal implementation of the abstraction
3. **Wrapper Integration**: Integrate the abstraction layer with existing code
4. **Testing**: Comprehensive testing with Temporal backend

### Phase 2: Alternative Engine Implementation

1. **Local Engine**: Implement a local execution engine for development
2. **Feature Parity**: Ensure key features (workflows, activities, signals) work
3. **Testing Framework**: Create tests that work across all engines
4. **Documentation**: Update documentation with multi-engine support

### Phase 3: Production Deployment

1. **Configuration System**: Engine selection via configuration
2. **Migration Tools**: Tools for moving between execution engines
3. **Monitoring**: Engine-agnostic monitoring and observability
4. **Performance Testing**: Benchmark different engines

## Implementation Details

### Engine Selection

```go
type ExecutionEngineConfig struct {
    Type     string                 `yaml:"type"` // temporal, local, kubernetes
    Options  map[string]interface{} `yaml:"options"`
}

func NewExecutionEngine(config ExecutionEngineConfig) (ExecutionEngine, error) {
    switch config.Type {
    case "temporal":
        return NewTemporalExecutionEngine(config.Options)
    case "local":
        return NewLocalExecutionEngine(config.Options)
    case "kubernetes":
        return NewKubernetesExecutionEngine(config.Options)
    default:
        return nil, fmt.Errorf("unknown execution engine type: %s", config.Type)
    }
}
```

### Context Abstraction

The workflow and activity contexts provide a unified interface while hiding engine-specific implementations:

```go
type WorkflowContextWrapper struct {
    engine     ExecutionEngine
    engineCtx  interface{} // Engine-specific context
    logger     Logger
}

func (w *WorkflowContextWrapper) ExecuteActivity(name string, input interface{}) ActivityFuture {
    switch e := w.engine.(type) {
    case *TemporalExecutionEngine:
        return e.executeTemporalActivity(w.engineCtx, name, input)
    case *LocalExecutionEngine:
        return e.executeLocalActivity(w.engineCtx, name, input)
    default:
        panic("unsupported engine")
    }
}
```

### Signal Handling Abstraction

```go
type SignalManager interface {
    RegisterSignalHandler(signalName string, handler interface{})
    SendSignal(executionID, signalName string, data interface{}) error
    CreateSignalChannel(signalName string) SignalChannel
}
```

### Activity Registry Integration

The existing activity registry will be enhanced to work with the abstraction:

```go
type AbstractActivityRegistry struct {
    activities map[string]RegisterableOp
    engine     ExecutionEngine
}

func (r *AbstractActivityRegistry) RegisterActivity(activity RegisterableOp) error {
    // Register with underlying engine
    return r.engine.RegisterActivity(activity)
}
```

## Engine-Specific Features

### Temporal-Specific Features
- Advanced retry policies
- Continue-as-new workflows
- Workflow versioning
- Search attributes
- Schedule workflows

### Local Engine Features
- In-memory execution
- Debugging capabilities
- Deterministic testing
- Fast iteration

### Kubernetes Engine Features
- Container-based activities
- Resource management
- Horizontal scaling
- Cloud-native integration

## Backward Compatibility

The abstraction maintains backward compatibility by:

1. **Default Engine**: Temporal remains the default engine
2. **Existing APIs**: Current recipe-worker APIs remain unchanged
3. **Configuration**: Optional engine configuration with sensible defaults
4. **Feature Flags**: Gradual migration using feature flags

## Testing Strategy

### Multi-Engine Test Suite

```go
func TestRecipeExecution(t *testing.T) {
    engines := []ExecutionEngine{
        NewTemporalEngine(),
        NewLocalEngine(),
    }
    
    for _, engine := range engines {
        t.Run(engine.Name(), func(t *testing.T) {
            testRecipeExecution(t, engine)
        })
    }
}
```

### Engine Compatibility Matrix

| Feature | Temporal | Local | Kubernetes |
|---------|----------|-------|------------|
| Workflows | ✅ | ✅ | ✅ |
| Activities | ✅ | ✅ | ✅ |
| Child Workflows | ✅ | ✅ | ⚠️ |
| Signals | ✅ | ✅ | ⚠️ |
| Timers | ✅ | ✅ | ✅ |
| Retry Policies | ✅ | ✅ | ✅ |
| History Query | ✅ | ✅ | ⚠️ |
| Search Attributes | ✅ | ⚠️ | ⚠️ |

Legend: ✅ Full Support, ⚠️ Partial Support, ❌ Not Supported

## Benefits of This Abstraction

1. **Engine Flexibility**: Ability to switch execution engines based on requirements
2. **Development Experience**: Local engines for faster development and testing
3. **Cloud Agnostic**: Support for different cloud and on-premise environments
4. **Cost Optimization**: Choose cost-effective engines for different workloads
5. **Migration Path**: Gradual migration without breaking existing functionality
6. **Testing**: Engine-agnostic testing capabilities
7. **Innovation**: Easy integration of new execution paradigms

## Conclusion

This execution abstraction provides a clean separation between recipe logic and execution infrastructure, enabling the vibethis system to evolve beyond Temporal while maintaining existing functionality and providing new deployment options. The phased implementation approach ensures minimal disruption while maximizing flexibility for future development.