# VIBETHIS

## Project: `recipe-history`

**Description:**

This project, `recipe-history`, is a Go library that serves as an adapter between the Temporal.io history service and the data model defined in `recipe-core`. Its primary function is to provide a "recipe-centric" view of execution history, abstracting away the low-level details of Temporal's event-based history and presenting it in a more user-friendly and understandable format.

**Key Components:**

*   **`pkg/history`**: This is the main package containing all the logic for the library.
    *   `Client`: The entry point for interacting with the library. It takes a Temporal client as input and provides methods for querying the history of recipe executions.
    *   `Transformer`: This is the core of the library's logic. It is responsible for the complex task of transforming Temporal's native data structures (like `WorkflowExecutionInfo` and `HistoryEvent`) into the more abstract `recipe.Job` and `recipe.ActivityExecution` structs defined in `recipe-core`.

**Functionality:**

1.  **List Jobs**: The `Client` can list all the execution "jobs" for a specific recipe. It does this by querying Temporal for workflow executions on the task queue associated with that recipe. It supports filtering by status (e.g., "running", "completed", "failed").
2.  **Get Job Details**: It can retrieve the detailed information for a single job (a specific workflow execution). This includes the job's status, start and end times, inputs, outputs, and any errors.
3.  **Reconstruct Activity History**: A key feature of the `Transformer` is its ability to parse the full event history of a workflow and reconstruct the sequence of activity executions. It can determine the status, duration, and result of each activity that was part of the job.
4.  **Data Abstraction**: The library provides a clean, high-level API for accessing recipe history, hiding the underlying complexity of the Temporal API and its event-sourcing model.

**How it Works:**

1.  A `history.Client` is instantiated with a `client.Client` from the Temporal SDK.
2.  To list jobs for a recipe (e.g., "my-recipe"), the `ListJobs` method queries Temporal for workflow executions on the "ono-recipes-my-recipe" task queue.
3.  The `Transformer` takes the list of `WorkflowExecutionInfo` objects from Temporal and converts each one into a `recipe.Job` object.
4.  To get the details of a specific job, the `GetJob` method first describes the workflow execution to get the basic information.
5.  If full activity details are requested, it then fetches the complete event history for that workflow execution.
6.  The `Transformer.HistoryToActivityExecutions` method processes the stream of history events, tracking the state of each activity (scheduled, started, completed, failed) to build a list of `recipe.ActivityExecution` objects.

**Use Cases:**

This library is an essential component for any part of the system that needs to display historical information about recipe executions. This could include:

*   A web UI that shows a list of past and present jobs for a recipe.
*   An API endpoint for retrieving the details of a specific job.
*   A monitoring or alerting system that needs to check the status of recent jobs.

**Example Usage (Conceptual):**

```go
package main

import (
    "context"
    "fmt"
    "log"

    "go.temporal.io/sdk/client"
    "go.uber.org/zap"
    "github.com/vibethis/server/recipe-history/pkg/history"
    "github.com/vibethis/server/recipe-core/pkg/recipe"
)

// A function to get a recipe definition (would be implemented by a recipe registry)
func getRecipe(name string) (*recipe.Recipe, error) {
    // ... implementation to fetch recipe from a store ...
    return nil, fmt.Errorf("not implemented")
}

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    // Create a Temporal client
    temporalClient, err := client.Dial(client.Options{})
    if err != nil {
        log.Fatalf("Failed to create Temporal client: %v", err)
    }
    defer temporalClient.Close()

    // Create a history client
    historyClient := history.NewClient(temporalClient, getRecipe, logger)

    // List completed jobs for a recipe
    jobs, err := historyClient.ListJobs(context.Background(), "my-data-pipeline", &history.JobFilter{Status: "completed"})
    if err != nil {
        log.Fatalf("Failed to list jobs: %v", err)
    }

    fmt.Printf("Found %d completed jobs for 'my-data-pipeline'\n", len(jobs))
    for _, job := range jobs {
        fmt.Printf("- Job ID: %s, Status: %s\n", job.ID, job.Status)
    }
}
```
