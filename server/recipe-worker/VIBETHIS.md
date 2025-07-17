# VIBETHIS

## Project: `recipe-worker`

**Description:**

This project, `recipe-worker`, is the runtime engine responsible for executing the recipes defined by the `recipe-core` library. It is a Go application that functions as a Temporal worker, dynamically discovering, monitoring, and running workflows based on the YAML definitions found in a specified directory.

**Key Components:**

*   **`pkg/worker`**: This package contains the core logic for managing the lifecycle of Temporal workers.
    *   `Worker`: The main struct that orchestrates the entire system, bringing together the registry and the worker manager.
    *   `WorkerManager`: Responsible for the direct management of Temporal workers. For each recipe, it creates a unique task queue and starts a worker that is registered with the specific workflows and activities defined in that recipe.
    *   `Registry`: This component handles the discovery and monitoring of recipes. It scans a directory for recipe files, parses them using `recipe-core`, and watches for any changes (creations, deletions, or modifications) using `fsnotify`. When a change is detected, it instructs the `WorkerManager` to start, stop, or restart the corresponding worker.

*   **`pkg/workflows`**: This package provides the implementation for dynamic workflow execution.
    *   `DynamicWorkflow`: This is the heart of the recipe execution logic. It's a Temporal workflow that can interpret a `recipe.WorkflowDefinition` at runtime. It iterates through the steps defined in the recipe's YAML file, executes the required activities, and manages the flow of data between them.
    *   `CreateDynamicWorkflow`: A factory function that takes a parsed `WorkflowDefinition` and returns a function that can be registered with a Temporal worker.

*   **`pkg/activities`**: This package is responsible for the execution of individual activities.
    *   `Executor`: A dynamic activity executor. It takes a parsed `recipe.ActivityDefinition` and, based on the specified implementation type (e.g., `http`, `grpc`, `script`), it calls the appropriate execution logic.
    *   `RegisterActivities`: A helper function that facilitates the registration of dynamic activities with a Temporal worker.

**Functionality:**

1.  **Automated Recipe Discovery**: The `recipe-worker` continuously scans a designated directory to find and parse recipe definitions.
2.  **Live-Reloading**: When a recipe file is added, removed, or modified, the worker automatically updates its internal state. It will start a new worker for a new recipe, stop the worker for a removed recipe, and restart the worker for a modified recipe, ensuring that the running workflows always reflect the latest definitions.
3.  **Dynamic Execution**: The worker does not have statically compiled workflows or activities. Instead, it uses dynamic implementations that interpret the YAML definitions at runtime. This allows for a highly flexible and extensible system where new workflows and activities can be added without recompiling or restarting the worker process itself.
4.  **Temporal Integration**: It leverages the power of Temporal.io for orchestration, providing robust, reliable, and scalable execution of recipes. Each recipe runs in its own isolated task queue, ensuring that different recipes do not interfere with each other.

**How it Works:**

1.  On startup, the `Registry` scans the recipes directory and parses all valid recipes.
2.  For each recipe, the `Registry` instructs the `WorkerManager` to start a new Temporal worker.
3.  The `WorkerManager` creates a unique task queue for the recipe and registers the `DynamicWorkflow` and the recipe's activities with the new worker.
4.  The `Registry` then watches the recipes directory for any file system changes.
5.  If a recipe is updated, its content hash changes, and the `Registry` tells the `WorkerManager` to restart the worker with the new definition.
6.  When a job for a recipe is started (e.g., via a Temporal client), the `DynamicWorkflow` is executed. It reads the recipe's steps and calls the necessary activities via `workflow.ExecuteActivity`.
7.  The `Executor` in the `pkg/activities` package receives the activity execution request and dispatches it to the correct implementation based on the activity's type.

**Example Invocation (Conceptual):**

While the worker itself runs as a long-running process, a client would interact with it via Temporal. To start a job for a recipe named "my-data-pipeline", a client would execute a workflow with the name "my-data-pipeline-workflow" on the task queue "ono-recipes-my-data-pipeline".
