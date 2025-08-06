# Recipe Infrastructure Enhancements Specification

## Overview

This specification defines enhancements to the recipe infrastructure to support workflow-to-workflow invocation, enabling a parent workflow (running in Cortex) to submit and monitor child workflows in separate Temporal instances (Nucleus).

## Current State

The existing recipe infrastructure supports:
- Workflow definitions in YAML
- Activity execution within workflows
- Single Temporal instance execution
- Basic workflow inputs/outputs

## Required Enhancements

### 1. Cross-Temporal Workflow Invocation

#### New Activity Type: `temporal_workflow`

A new activity implementation type that submits workflows to remote Temporal instances:

```yaml
implementation:
  type: temporal_workflow
  config:
    temporal_host: "{{ .DynamicValue }}"     # Can be dynamically provided
    temporal_namespace: "nucleus"            # Target namespace
    task_queue: "recipe-worker"              # Target task queue
    workflow_type: "{{ .WorkflowName }}"     # Workflow to execute
    timeout: "30m"                           # Execution timeout
    retry_policy:
      maximum_attempts: 3
      initial_interval: "5s"
      backoff_coefficient: 2.0
```

#### Activity Definition Example

```yaml
activities:
  - name: submit_nucleus_workflow
    description: Submits a workflow to a Nucleus instance
    timeout: 35m  # Slightly longer than workflow timeout
    inputs:
      - name: temporal_host
        type: string
        required: true
        description: Host:port of the Nucleus Temporal instance
      - name: workflow_name
        type: string
        required: true
        description: Name of the workflow to execute
      - name: workflow_inputs
        type: object
        required: false
        description: Input parameters for the workflow
      - name: workflow_id
        type: string
        required: false
        description: Optional workflow ID (generated if not provided)
    outputs:
      - name: workflow_id
        type: string
        description: ID of the submitted workflow
      - name: run_id
        type: string
        description: Run ID of the workflow execution
      - name: result
        type: object
        description: Result from the workflow execution
      - name: status
        type: string
        description: Final status of the workflow
    implementation:
      type: temporal_workflow
      config:
        temporal_namespace: "nucleus"
        task_queue: "recipe-worker"
```

### 2. Dynamic Temporal Connection Management

#### Connection Configuration

Support for dynamic Temporal connection parameters:

```go
type TemporalWorkflowConfig struct {
    Host            string        `yaml:"temporal_host"`
    Namespace       string        `yaml:"temporal_namespace"`
    TaskQueue       string        `yaml:"task_queue"`
    WorkflowType    string        `yaml:"workflow_type"`
    Timeout         time.Duration `yaml:"timeout"`
    RetryPolicy     *RetryPolicy  `yaml:"retry_policy"`
    TLS             *TLSConfig    `yaml:"tls"`
    Identity        string        `yaml:"identity"`
}

type TLSConfig struct {
    CertPath string `yaml:"cert_path"`
    KeyPath  string `yaml:"key_path"`
    CAPath   string `yaml:"ca_path"`
}
```

#### Implementation Requirements

1. **Connection Pooling**: Reuse Temporal client connections when possible
2. **Health Checking**: Verify Temporal instance availability before submission
3. **Timeout Handling**: Properly propagate context timeouts
4. **Error Mapping**: Convert Temporal errors to recipe activity errors

### 3. Workflow Result Handling

#### Enhanced Output Structure

```yaml
outputs:
  - name: result
    type: object
    schema:
      type: object
      properties:
        workflow_outputs:
          type: object
          description: Outputs from the executed workflow
        execution_metadata:
          type: object
          properties:
            start_time:
              type: string
              format: date-time
            end_time:
              type: string
              format: date-time
            duration_ms:
              type: integer
            attempt_count:
              type: integer
```

### 4. Workflow Discovery Enhancement

#### Dynamic Workflow Resolution

The activity should support discovering available workflows in the target Temporal instance:

```yaml
activities:
  - name: list_available_workflows
    description: Lists workflows available in a Temporal instance
    implementation:
      type: temporal_workflow
      config:
        operation: "list_workflows"
```

### 5. Monitoring and Observability

#### Activity Execution Tracking

Enhanced tracking for cross-Temporal executions:

```go
type WorkflowExecutionEvent struct {
    ParentWorkflowID   string
    ParentRunID        string
    ChildWorkflowID    string
    ChildRunID         string
    ChildTemporalHost  string
    ChildNamespace     string
    Status             string
    StartTime          time.Time
    EndTime            *time.Time
}
```

## Implementation Plan

### Phase 1: Core Activity Implementation
1. Create `temporal_workflow` activity type
2. Implement basic workflow submission
3. Add result polling and retrieval

### Phase 2: Connection Management
1. Implement connection pooling
2. Add health checking
3. Support TLS configuration

### Phase 3: Enhanced Features
1. Add workflow discovery
2. Implement advanced error handling
3. Add monitoring/metrics

## Testing Requirements

1. **Unit Tests**: Mock Temporal client interactions
2. **Integration Tests**: Test with embedded Temporal instances
3. **E2E Tests**: Full Cortex-to-Nucleus workflow execution

## Security Considerations

1. **Authentication**: Support for Temporal authentication tokens
2. **Authorization**: Validate workflow submission permissions
3. **Network Security**: TLS support for cross-instance communication
4. **Input Validation**: Sanitize workflow inputs

## Backwards Compatibility

All enhancements must maintain compatibility with existing recipe definitions. The new `temporal_workflow` activity type is additive and doesn't affect existing activities.