# Ono Recipe System Implementation Plan

## Overview
Transform Ono from exposing Temporal-specific concepts (workflows, activities, events) to presenting its own abstractions: recipes and jobs. This creates a cleaner, more intuitive interface that hides Temporal implementation details.

## Core Concepts
- **Recipe**: A YAML-based project definition that describes a process (replaces "project" terminology)
- **Job**: An execution instance of a recipe (replaces "workflow execution")
- **Recipe Worker**: A dedicated worker process that handles job executions for a specific recipe

## CLI API Specification

### Current vs New Command Structure

| Current Command | New Command | Description |
|-----------------|-------------|-------------|
| `ono workflow list` | `ono recipe list` | List all available recipes |
| `ono workflow create` | *(removed)* | Recipe discovery is automatic |
| `ono workflow run` | `ono recipe run` | Execute a recipe (creates a job) |
| `ono workflow describe` | `ono recipe describe` | Show recipe overview and structure |
| `ono workflow history` | `ono recipe history` | Show job history of a recipe |
| `ono workflow activities` | *(removed)* | Activities is an internal implementation detail |
| N/A | `ono job describe` | Show detailed job execution info |
| `ono workflow restart` | `ono job restart` | Restart a job from failure point |
| N/A | `ono job cancel` | Cancel/stop a job execution |

### Detailed Command Specifications

#### `ono recipe list`
Lists all discovered recipes from the recipe configuration directory.
```bash
ono recipe list [flags]

Flags:
  --format string   Output format: table, json (default "table")
  --status string   Filter by worker status: current, removed, all (default "all")
```
Output columns: Recipe Name, Version, Status, Worker Status, Last Updated


Note: removed will look at past temporal workflows that we have state for that are tagged as recipes to enable historical analysis.

#### `ono recipe describe <recipe-name>`
Shows recipe structure and configuration.
```bash
ono recipe describe <recipe-name> [flags]

Flags:
  --format string   Output format: text, json, yaml (default "text")
```
Output includes:
- Recipe metadata (name, version, description)
- Input parameters with types and defaults
- Expected outputs
- Step structure (activities and their purposes)


Note: for removed workflows, describe will output a subset of information since the recipe yaml will no longer be available. 

#### `ono recipe run <recipe-name>`
Executes a recipe, creating a new job.
```bash
ono recipe run <recipe-name> [flags]

Flags:
  --input string       Input parameters as JSON
  --input-file string  Input parameters from file
  --job-id string      Custom job ID (default: auto-generated)
  --wait               Wait for job completion (default true)
  --output string      Output format: text, json (default "text")
```
Returns: Job ID and execution status

#### `ono recipe history <recipe-name>`
Shows job execution history for a recipe.
```bash
ono recipe history <recipe-name> [flags]

Flags:
  --limit int       Number of jobs to show (default 20)
  --status string   Filter by status: running, completed, failed, all (default "all")
  --format string   Output format: table, json (default "table")
```
Output columns: Job ID, Status, Start Time, Duration, Result Summary

#### `ono job describe <recipe-name> <job-id>`
Shows detailed information about a specific job execution.
```bash
ono job describe <recipe-name> <job-id> [flags]

Flags:
  --format string   Output format: text, json (default "text")
  --verbose         Show detailed activity information
```
Output includes:
- Job metadata (ID, status, timing)
- Input parameters used
- Current/final outputs
- Activity execution list with:
  - Activity name (from YAML definition)
  - Status (pending, running, completed, failed)
  - Duration
  - Result summary (contextualized by YAML)

#### `ono job restart <recipe-name> <job-id>`
Restarts a failed job from a specific point.
```bash
ono job restart <recipe-name> <job-id> [flags]

Flags:
  --from-activity string   Restart from specific activity name
  --input string          Override input parameters as JSON
  --new-job-id string     Custom ID for restarted job
```

## Recipe Configuration Directory Structure

### Directory Layout
```
~/.ono/recipes/              # Default recipe directory (configurable)
├── recipe1/
│   ├── recipe.yaml         # Recipe manifest (renamed from project.yaml)
│   ├── workflow.yaml       # Workflow definition
│   ├── activities.yaml     # Activity definitions
│   └── agents.yaml         # Agent definitions
├── recipe2.yaml            # Single-file recipe
└── .onoignore             # Patterns to ignore during discovery
```

### Recipe Manifest Schema Update
```yaml
# recipe.yaml (formerly project.yaml)
recipe:
  name: "data-processing"
  version: "1.0.0"
  description: "Process and analyze data files"
  files:
    workflow: "workflow.yaml"
    activities: "activities.yaml"
    agents: "agents.yaml"
```

## Implementation Steps

### Phase 1: Recipe Discovery and Management
1. **Create Recipe Registry Service**
   - Implement `internal/recipe/registry.go` with recipe discovery logic
   - Watch recipe directory for new/modified/deleted recipes using filesystem watcher
   - Maintain in-memory registry of available recipes with their canonical hashes
   - Support both single-file and multi-file recipe formats
   - Implement file watching with debouncing to handle rapid changes

2. **Implement Recipe Parser**
   - Update `pkg/yaml/parser.go` to support "recipe" terminology
   - Add recipe validation and schema enforcement
   - Create recipe model types in `internal/recipe/types.go`

3. **Build Recipe Worker Manager**
   - Create `internal/recipe/worker_manager.go` for lifecycle management
   - Start workers automatically when recipes are discovered
   - Stop/restart workers when recipes are modified
   - Track worker status per recipe

4. **Implement Change Detection System**
   - Create `internal/recipe/hash.go` for canonicalized hashing
   - Generate deterministic hash of recipe content that:
     - Normalizes YAML structure (ignores formatting/whitespace)
     - Strips comments and non-functional content
     - Orders maps/dictionaries consistently
     - Combines all files in multi-file recipes
   - Store hash->worker mappings to detect actual changes
   - Only restart workers when canonical hash changes

### Phase 2: CLI Command Migration
1. **Create Recipe Commands**
   - Implement `pkg/cli/recipe_commands.go` as main command group
   - Create `pkg/cli/recipe_list.go` - queries recipe registry
   - Create `pkg/cli/recipe_describe.go` - shows recipe structure
   - Create `pkg/cli/recipe_run.go` - adapts workflow run logic
   - Create `pkg/cli/recipe_history.go` - adapts workflow list with filtering

2. **Create Job Commands**
   - Implement `pkg/cli/job_commands.go` as command group
   - Create `pkg/cli/job_describe.go` - contextualizes Temporal data
   - Create `pkg/cli/job_restart.go` - adapts workflow restart

3. **Remove Workflow Commands**
   - Mark workflow commands as deprecated initially
   - Remove `workflow create` command entirely
   - Hide `workflow activities` from user-facing CLI

### Phase 3: Data Transformation Layer
1. **Create Recipe/Job Abstraction Layer**
   - Implement `internal/recipe/transformer.go`
   - Map Temporal workflow executions to jobs
   - Map Temporal activities to recipe-defined names
   - Hide Temporal-specific details (events, signals, etc.)

2. **Update Output Formatting**
   - Create recipe-aware formatters in `pkg/cli/format/`
   - Show activity names from YAML definitions, not Temporal names
   - Present job status in user-friendly terms

### Phase 4: Configuration and Storage
1. **Recipe Configuration Management**
   - Add recipe directory configuration to Ono settings
   - Support environment variable `ONO_RECIPE_DIR`
   - Default to `~/.ono/recipes/`

2. **Recipe State Persistence**
   - Store recipe-to-worker mappings
   - Track recipe versions and update history
   - Maintain job-to-recipe associations

## Migration Considerations

### Backward Compatibility
- Keep workflow commands available but deprecated for 2 releases
- Add warnings when workflow commands are used
- Provide migration guide in documentation

### Worker Management Strategy
- Workers start automatically on recipe discovery
- Workers restart only when canonical hash changes:
  - File watcher detects any file system changes
  - System computes new canonical hash of recipe content
  - If hash differs from stored hash, gracefully shutdown old worker and start new one
  - If hash is same (e.g., only formatting/comments changed), no worker restart but update last updated date for workflow list command
- Failed workers retry with exponential backoff
- Worker health checks integrated into `recipe list` output
- Hash computation details:
  - Parse YAML into normalized structure
  - Sort all maps/dictionaries by key
  - Strip comments and empty lines
  - For multi-file recipes, combine all files in deterministic order
  - Use SHA-256 for final hash generation

### Error Handling
- Recipe validation errors prevent worker startup
- Malformed recipes logged but don't crash the system
- Worker failures reported in recipe status

## Testing Strategy
1. Unit tests for recipe discovery and parsing
2. Integration tests for worker lifecycle management
3. E2E tests for complete recipe execution flow
4. Migration tests to ensure backward compatibility

## Documentation Updates
1. Update CLI help text for all commands
2. Create recipe authoring guide
3. Add recipe examples to example directory
4. Update README with new concepts and commands

## Success Criteria
- Users can define and run recipes without knowing about Temporal
- Recipe discovery and worker management is automatic
- Job details are presented in recipe context, not Temporal context
- Zero downtime migration from workflow to recipe commands
