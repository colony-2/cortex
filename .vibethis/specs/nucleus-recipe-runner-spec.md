# Nucleus Recipe Runner Specification

## Overview
This specification defines how the Nucleus process leverages the existing recipe-worker system to run multiple recipe-based workflows within a containerized environment, with support for template variables and recipe discovery.

## Architecture

### 1. Integration with Recipe-Worker

The Nucleus process extends the existing recipe-worker functionality with cell-specific context and lifecycle management.

#### Core Components
```
nucleus (main process)
├── Recipe-Worker (existing)
│   ├── Recipe Discovery
│   ├── Template Engine
│   ├── Workflow Registration
│   └── Activity Registration
├── Cell Context Provider
│   ├── Template Variables
│   ├── Cell Metadata
│   └── Environment Config
└── Lifecycle Manager
    ├── Worker Pool Manager
    ├── Health Monitor
    └── Graceful Shutdown
```

### 2. Recipe Discovery with Template Variables

#### Leveraging Existing Recipe-Worker Discovery
The recipe-worker already provides recipe discovery and loading. Nucleus configures it with cell-specific paths and template variables.

##### Template Context Variables
```go
type CellTemplateContext struct {
    // Core identifiers
    CellID          string `template:"CELL_ID"`
    CellPath        string `template:"CELL_PATH"`
    
    // Temporal configuration
    TemporalAddress   string `template:"TEMPORAL_ADDRESS"`
    TemporalNamespace string `template:"TEMPORAL_NAMESPACE"`
    TaskQueue         string `template:"TASK_QUEUE"`
    
    // Workspace paths
    CodebasePath    string `template:"CODEBASE_PATH"`
    GitPackPath     string `template:"GIT_PACK_PATH"`
    SpecPath        string `template:"SPEC_PATH"`
    RecipePath      string `template:"RECIPE_PATH"`
    
    // Runtime configuration
    Environment     string `template:"ENVIRONMENT"`
    Region          string `template:"REGION"`
    
    // Custom cell metadata
    CellType        string            `template:"CELL_TYPE"`
    CellTags        []string          `template:"CELL_TAGS"`
    CellConfig      map[string]string `template:"CELL_CONFIG"`
}
```

##### Recipe Configuration with Templates
```yaml
# recipe.yaml with template variables
apiVersion: v1
kind: Recipe
metadata:
  name: triage
  version: 1.0.0
  cell: {{.CELL_ID}}

spec:
  type: triage
  
  # Task queue uses cell-specific queue
  taskQueue: {{.CELL_ID}}-triage
  
  # Workflow configuration
  workflow:
    class: TriageWorkflow
    timeout: 30m
    retries: 3
    
  # Activities with templated configuration
  activities:
    - name: ParseRequest
      timeout: 5m
      config:
        cellPath: {{.CELL_PATH}}
    - name: DecomposeTask
      timeout: 10m
      config:
        maxSubtasks: {{.CELL_CONFIG.max_subtasks | default "50"}}
    - name: CreateSubtasks
      timeout: 5m
  
  # Configuration with template variables
  config:
    temporal:
      address: {{.TEMPORAL_ADDRESS}}
      namespace: {{.TEMPORAL_NAMESPACE}}
    workspace:
      codebase: {{.CODEBASE_PATH}}
      specs: {{.SPEC_PATH}}
    llm:
      model: {{.CELL_CONFIG.llm_model | default "gpt-4"}}
      temperature: {{.CELL_CONFIG.llm_temperature | default "0.3"}}
```

### 3. Nucleus Initialization

#### Startup Configuration
```go
type NucleusConfig struct {
    RecipeWorkerConfig recipe.WorkerConfig
    CellContext        CellTemplateContext
}

func NewNucleus(config NucleusConfig) (*Nucleus, error) {
    // Initialize recipe-worker with template context
    rw, err := recipe.NewWorker(recipe.WorkerConfig{
        RecipePaths: []string{
            fmt.Sprintf("/recipes/cell/%s", config.CellContext.CellID),
            "/recipes/global",
        },
        TemplateContext: config.CellContext,
        TemporalClient:  temporalClient,
    })
    
    if err != nil {
        return nil, err
    }
    
    return &Nucleus{
        recipeWorker: rw,
        cellContext:  config.CellContext,
    }, nil
}
```

#### Recipe Override Resolution
The existing recipe-worker system handles override resolution. Nucleus configures the search paths:

```go
func (n *Nucleus) ConfigureRecipePaths() []string {
    paths := []string{}
    
    // Cell-specific recipes (highest priority)
    cellRecipePath := fmt.Sprintf("/cells/%s/.colony2/recipes", n.cellContext.CellID)
    if exists(cellRecipePath) {
        paths = append(paths, cellRecipePath)
    }
    
    // Global recipes
    paths = append(paths, "/recipes/global")
    
    // Built-in recipes (lowest priority)
    paths = append(paths, "embedded://recipes")
    
    return paths
}
```

### 4. Worker Registration

#### Using Recipe-Worker's Registration System
```go
func (n *Nucleus) Start() error {
    // Configure recipe paths with overrides
    n.recipeWorker.SetRecipePaths(n.ConfigureRecipePaths())
    
    // Register workers for each recipe type
    recipeTypes := []string{"triage", "spec", "code", "doc"}
    
    for _, recipeType := range recipeTypes {
        // Recipe-worker handles discovery and registration
        if err := n.recipeWorker.RegisterRecipe(recipeType); err != nil {
            log.Warnf("Recipe %s not found or failed to load: %v", recipeType, err)
            continue
        }
        
        // Apply cell-specific configuration
        n.configureRecipeWorker(recipeType)
    }
    
    // Start all registered workers
    return n.recipeWorker.Start()
}

func (n *Nucleus) configureRecipeWorker(recipeType string) {
    // Special handling for code recipe (singleton)
    if recipeType == "code" {
        n.recipeWorker.SetWorkerOptions(recipeType, worker.Options{
            MaxConcurrentWorkflowTaskExecutions: 1,
            MaxConcurrentActivityExecutions: 1,
        })
    }
}
```

### 5. Simplified Recipe Definition

Since recipe-worker handles the complexity, recipes can be simple YAML definitions with activities:

```yaml
# /recipes/global/triage/recipe.yaml
apiVersion: v1
kind: Recipe
metadata:
  name: triage
  version: 1.0.0

spec:
  type: triage
  taskQueue: {{.CELL_ID}}-triage
  
  activities:
    - parseRequest
    - analyzeDependencies  
    - decomposeTask
    - createWorkItems
    - notifyStakeholders
  
  config:
    llm:
      model: {{.CELL_CONFIG.llm_model | default "gpt-4"}}
    decomposition:
      maxSubtasks: {{.CELL_CONFIG.max_subtasks | default "50"}}
```

The recipe-worker system handles:
- Activity discovery and registration
- Workflow orchestration
- Template variable substitution
- Configuration management

### 6. Inter-Recipe Communication

#### Shared Context
All recipes within a cell share a context object:
```go
type CellContext struct {
    CellID      string
    Workspace   *WorkspaceManager
    GitPacks    *GitPackManager
    Specs       *SpecManager
    Metadata    map[string]interface{}
    mu          sync.RWMutex
}
```

#### Message Passing
Recipes communicate via Temporal signals and queries:
```go
// Triage recipe creates work for spec recipe
func (t *TriageWorkflow) CreateSpecTask(ctx workflow.Context, task Task) error {
    return workflow.ExecuteChildWorkflow(ctx, "SpecWorkflow", task).Get(ctx, nil)
}

// Code recipe queries spec recipe for latest spec
func (c *CodeWorkflow) GetLatestSpec(ctx workflow.Context) (*Spec, error) {
    var spec Spec
    err := workflow.QueryWorkflow(ctx, "GetCurrentSpec").Get(&spec)
    return &spec, err
}
```

### 7. Resource Management

#### Workspace Isolation
Each recipe type gets its own workspace subdirectory:
```
/workspace/
├── codebase/     # Read-only snapshot
├── git-packs/    # Shared git thin packs
├── specs/        # Shared specifications
└── recipes/      # Recipe-specific workspace
    ├── triage/
    ├── spec/
    ├── code/
    └── doc/
```

#### Concurrency Control
```go
type ConcurrencyManager struct {
    semaphores map[string]*semaphore.Weighted
}

func (cm *ConcurrencyManager) AcquireForRecipe(recipeType string) error {
    if recipeType == "code" {
        // Code recipe is singleton
        return cm.semaphores["code"].Acquire(context.Background(), 1)
    }
    // Other recipes can run in parallel
    return nil
}
```

### 6. Configuration and Environment

#### Nucleus Startup Configuration
```yaml
# nucleus.yaml
apiVersion: v1
kind: NucleusConfig
metadata:
  cellID: ${CELL_ID}
  
spec:
  temporal:
    address: ${TEMPORAL_ADDRESS}
    namespace: ${TEMPORAL_NAMESPACE}
    identity: nucleus-${CELL_ID}
    
  recipes:
    globalPath: /recipes/global
    cellPath: /recipes/cell
    enabled:
      - triage
      - spec
      - code
      - doc
    
  workspace:
    basePath: /workspace
    codebasePath: /workspace/codebase
    gitPackPath: /workspace/git-packs
    specPath: /workspace/specs
    
  logging:
    level: info
    format: json
    output: stdout
    
  metrics:
    enabled: true
    port: 9090
    path: /metrics
```

#### Environment Variables
```bash
# Core configuration
NUCLEUS_CELL_ID=cell-123
NUCLEUS_CONFIG_PATH=/etc/nucleus/nucleus.yaml

# Temporal configuration
TEMPORAL_ADDRESS=temporal:7233
TEMPORAL_NAMESPACE=cortex-cells

# Recipe paths
RECIPE_GLOBAL_PATH=/recipes/global
RECIPE_CELL_PATH=/recipes/cell

# Workspace paths
WORKSPACE_PATH=/workspace
CODEBASE_PATH=/workspace/codebase
GIT_PACK_PATH=/workspace/git-packs
SPEC_PATH=/workspace/specs

# Feature flags
ENABLE_RECIPE_OVERRIDE=true
ENABLE_METRICS=true
ENABLE_TRACING=false
```

### 7. Lifecycle Management

#### Startup Sequence
1. Parse configuration and environment
2. Initialize workspace directories
3. Connect to Temporal
4. Load and validate recipes
5. Register workers for each recipe
6. Start health check endpoint
7. Begin processing workflows

#### Shutdown Sequence
1. Stop accepting new workflows
2. Wait for running workflows to complete (with timeout)
3. Persist any in-memory state
4. Cleanup temporary resources
5. Disconnect from Temporal
6. Exit cleanly

#### Health Checks
```go
type HealthChecker struct {
    temporal    client.Client
    workers     map[string]worker.Worker
    recipes     map[string]*Recipe
}

func (hc *HealthChecker) Check() HealthStatus {
    status := HealthStatus{Healthy: true}
    
    // Check Temporal connection
    if _, err := hc.temporal.CheckHealth(context.Background()); err != nil {
        status.Healthy = false
        status.Issues = append(status.Issues, "Temporal disconnected")
    }
    
    // Check each worker
    for name, worker := range hc.workers {
        if !worker.IsRunning() {
            status.Healthy = false
            status.Issues = append(status.Issues, fmt.Sprintf("Worker %s stopped", name))
        }
    }
    
    return status
}
```

### 8. Error Handling and Recovery

#### Recipe Failure Handling
```go
func (n *Nucleus) HandleRecipeFailure(recipeType string, err error) {
    // Log the error
    log.Errorf("Recipe %s failed: %v", recipeType, err)
    
    // Attempt recovery based on recipe type
    switch recipeType {
    case "code":
        // Code failures are critical - may need to rollback
        n.rollbackCodeChanges()
    case "triage":
        // Triage failures can be retried
        n.scheduleRetry(recipeType, 5*time.Minute)
    default:
        // Other recipes can continue
        n.markRecipeAsFailed(recipeType)
    }
}
```

#### Graceful Degradation
- If cell-specific recipe fails to load, fall back to global
- If global recipe fails, use built-in minimal implementation
- Continue operating with available recipes even if some fail

### 9. Monitoring and Observability

#### Metrics to Export
- Recipes loaded (global vs cell-specific)
- Workflow executions per recipe type
- Recipe execution duration
- Recipe failure rate
- Resource usage per recipe
- Queue depth per recipe type

#### Logging Strategy
- Structured logging with recipe context
- Separate log streams per recipe type
- Correlation IDs for tracing across recipes
- Log aggregation to cell-level view

### 10. Security Considerations

#### Recipe Validation
- Verify recipe signatures (if signed)
- Validate recipe schema before loading
- Sandbox recipe execution environment
- Limit resource consumption per recipe

#### Access Control
- Recipes can only access designated workspace areas
- No direct host system access
- Temporal credentials scoped to cell
- Audit log for recipe overrides