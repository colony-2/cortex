# VIBETHIS

## Overview
Recipe-history provides a Go library that serves as an adapter between Temporal.io's workflow execution service and the recipe-core data model. It transforms low-level Temporal workflow events into user-friendly job execution history, abstracting away Temporal's event-sourcing complexity to present recipe executions in an intuitive format.

## Architecture

### Key Components
- **Client**: Entry point providing recipe-centric history querying interface
- **Transformer**: Core component converting Temporal data structures to recipe abstractions
- **JobFilter**: Configuration for filtering and limiting job queries

### Component Relationships
```
Client → Temporal.io SDK → Temporal Service
   ↓
Transformer → recipe-core types (Job, ActivityExecution)
```

The Client orchestrates history retrieval from Temporal and delegates transformation to the Transformer, which maps Temporal's workflow concepts to recipe-core's job abstractions.

## Key Interfaces

### Client Interface
```go
func NewClient(temporal client.Client, getRecipe GetRecipeFunc, logger *zap.Logger) *Client

func (c *Client) ListJobs(ctx context.Context, recipeName string, filter *JobFilter) ([]*recipe.Job, error)

func (c *Client) GetJob(ctx context.Context, recipeName, jobID string, includeActivities bool) (*recipe.Job, error)
```

### Transformer Interface
```go
func NewTransformer(getRecipe GetRecipeFunc) *Transformer

func (t *Transformer) WorkflowExecutionsToJobs(executions []*workflow.WorkflowExecutionInfo, recipeName string) ([]*recipe.Job, error)

func (t *Transformer) HistoryToActivityExecutions(history *history.History, recipeName string) ([]*recipe.ActivityExecution, error)
```

### Types
```go
type GetRecipeFunc func(name string) (*recipe.Recipe, error)

type JobFilter struct {
    Status string  // "running", "completed", "failed", "all"
    Limit  int     // Maximum number of jobs to return
}
```

## Usage Examples

### Basic Job Listing
```go
package main

import (
    "context"
    "log"
    
    "go.temporal.io/sdk/client"
    "go.uber.org/zap"
    "github.com/colony-2/colony2/server/recipe-history/pkg/history"
)

func getRecipe(name string) (*recipe.Recipe, error) {
    // Implementation to fetch recipe from registry
    return recipeRegistry.Get(name)
}

func main() {
    logger, _ := zap.NewProduction()
    
    temporalClient, err := client.Dial(client.Options{})
    if err != nil {
        log.Fatal(err)
    }
    defer temporalClient.Close()
    
    historyClient := history.NewClient(temporalClient, getRecipe, logger)
    
    // List completed jobs with limit
    jobs, err := historyClient.ListJobs(context.Background(), "data-pipeline", 
        &history.JobFilter{Status: "completed", Limit: 10})
    if err != nil {
        log.Fatal(err)
    }
    
    for _, job := range jobs {
        log.Printf("Job %s: %s (started: %v)", job.ID, job.Status, job.StartTime)
    }
}
```

### Detailed Job Analysis
```go
func analyzeJob(client *history.Client, recipeName, jobID string) {
    job, err := client.GetJob(context.Background(), recipeName, jobID, true)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Job %s status: %s", job.ID, job.Status)
    if job.Duration != nil {
        log.Printf("Duration: %v", *job.Duration)
    }
    
    for _, activity := range job.Activities {
        log.Printf("Activity %s: %s", activity.Name, activity.Status)
        if activity.Error != "" {
            log.Printf("  Error: %s", activity.Error)
        }
    }
}
```

### Filtering Jobs by Status
```go
func monitorFailedJobs(client *history.Client, recipeName string) {
    failedJobs, err := client.ListJobs(context.Background(), recipeName,
        &history.JobFilter{Status: "failed", Limit: 20})
    if err != nil {
        log.Fatal(err)
    }
    
    for _, job := range failedJobs {
        log.Printf("Failed job %s: %s", job.ID, job.Error)
    }
}
```

## Configuration

### Dependencies
- `go.temporal.io/sdk v1.34.0`: Temporal Go SDK for workflow interactions
- `go.temporal.io/api v1.50.0`: Temporal API types and protobuf definitions
- `go.uber.org/zap v1.27.0`: Structured logging
- `github.com/colony-2/colony2/server/recipe-core`: Core recipe types and interfaces

### Environment Requirements
- Temporal server connection (configured via client.Options)
- Recipe registry implementation for GetRecipeFunc
- Go 1.24.1+

### Task Queue Naming Convention
Jobs are queried using task queue format: `ono-recipes-{recipeName}`