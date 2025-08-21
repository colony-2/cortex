# Git Workflow Integration Specification

## Overview
This specification describes the automatic injection of git Temporal activities into the compiled workflow during the recipe-to-temporal workflow compilation process. The goal is to ensure every recipe-driven workflow automatically maintains git state through persist and restore activities, enabling reliable workflow restarts and providing a complete audit trail of changes.

## Key Principles
- **Temporal Activity Injection**: Git operations are injected as Temporal activities during compilation, not as recipe ops
- **Universal Wrapping**: EVERY operation gets wrapped with git activities, no exceptions
- **Transparent Integration**: The recipe structure remains unchanged; injection happens at the compiler level
- **Complete State Tracking**: Even manual git operations get wrapped, providing state tracking at every step
- **Maximum Simplicity**: No special cases or skip logic - everything gets the same treatment

## Current State
- **Recipe Worker**: Located in `server/recipe-worker`, compiles recipes to Temporal workflows
- **Git Operations**: Located in `server/git`, provides Temporal activities:
  - `GitShallowClone`: Initial repository setup activity
  - `PersistCommit`: Commits changes and creates thin packs activity
  - `RestoreCommit`: Restores repository to specific commit state activity

Currently, these operations can be manually inserted into recipes as ops. This specification proposes automatic Temporal activity injection during workflow compilation, which works orthogonally to any manual git ops in the recipe.

## Proposed Architecture

### 1. Temporal Activity Injection Points

The compiler will inject Temporal activities at these points during workflow compilation:

#### 1.1 Workflow Start: Git Initialize Activity
```go
// Injected as first Temporal activity in compiled workflow
gitInitActivity := workflow.ExecuteActivity(ctx, gitshallow.GitShallowClone, 
    gitshallow.GitShallowCloneInput{
        SourceDir:  workflowInputs["__source_repo"].(string),
        TargetDir:  workflowInputs["__working_dir"].(string),
        CommitHash: workflowInputs["__base_commit"].(string),
    })
```

#### 1.2 Pre-Operation: Git Restore Activity
```go
// Injected before each recipe operation's Temporal activity
restoreActivity := workflow.ExecuteActivity(ctx, gitcommit.RestoreCommit,
    gitcommit.RestoreCommitActivity{
        RepoPath:        gitConfig.WorkingDir,
        TargetCommit:    lastCommitHash, // From workflow state
        RootHash:        gitConfig.BaseCommit,
        StorageLocation: gitConfig.PackStorage,
    })
```

#### 1.3 Post-Operation: Git Persist Activity
```go
// Injected after each recipe operation's Temporal activity
persistActivity := workflow.ExecuteActivity(ctx, gitcommit.PersistCommit,
    gitcommit.PersistCommitActivity{
        RepoPath:        gitConfig.WorkingDir,
        RootHash:        gitConfig.BaseCommit,
        StorageLocation: gitConfig.PackStorage,
        CommitMessage:   fmt.Sprintf("After %s", operationName),
        Author:          gitConfig.GitAuthor,
    })
```

#### 1.4 Workflow End: Git Merge Activity
```go
// Injected as last Temporal activity in compiled workflow
mergeActivity := workflow.ExecuteActivity(ctx, gitmerge.MergeChanges,
    gitmerge.MergeChangesInput{
        WorkingDir:  gitConfig.WorkingDir,
        TargetRepo:  gitConfig.SourceRepo,
        BranchName:  gitConfig.MergeBranch,
        PushRemote:  gitConfig.PushRemote,
    })
```

### 2. Compiler Modifications

#### 2.1 New Compiler Configuration
```go
// pkg/compiler/git_config.go
type GitWorkflowConfig struct {
    EnableGitIntegration bool              // Feature flag
    SourceRepo          string             // Source repository path
    WorkingDir          string             // Working directory (auto-generated if empty)
    BaseCommit          string             // Starting commit
    PackStorage         string             // Storage location for thin packs
    GitAuthor           string             // Optional: Git author for commits
    MergeBranch         string             // Optional: Target branch for merge
    PushRemote          string             // Optional: Remote to push changes
}
```

#### 2.2 Modified Compiler Interface
```go
// pkg/compiler/compiler.go
type Compiler struct {
    activityRegistry     *ActivityRegistry
    stateMachineCompiler *statemachine.StateMachineCompiler
    gitConfig           *GitWorkflowConfig  // New field
}

func NewCompiler(registry *ActivityRegistry, gitConfig *GitWorkflowConfig) *Compiler {
    // ... existing initialization ...
    return &Compiler{
        activityRegistry:     registry,
        stateMachineCompiler: smCompiler,
        gitConfig:           gitConfig,
    }
}
```

#### 2.3 Activity Injection Logic
```go
// pkg/compiler/git_injection.go

// Modified executeOperation to inject git activities around EVERY operation
func (c *Compiler) executeOperation(
    ctx workflow.Context, 
    op string, 
    nodeInputs map[string]interface{}, 
    workflowInputs map[string]interface{},
) (map[string]interface{}, error) {
    
    // If git integration is disabled, execute the recipe op normally
    if !c.gitConfig.EnableGitIntegration {
        return c.executeRecipeOperation(ctx, op, nodeInputs, workflowInputs)
    }
    
    // 1. Execute Git Restore Activity (before the operation)
    if !c.isFirstOperation(ctx) {
        restoreInput := gitcommit.RestoreCommitActivity{
            RepoPath:        c.gitConfig.WorkingDir,
            TargetCommit:    c.getLastCommitHash(ctx),
            RootHash:        c.gitConfig.BaseCommit,
            StorageLocation: c.gitConfig.PackStorage,
            Force:           true, // Force restore to ensure clean state
        }
        
        var restoreOutput gitcommit.RestoreCommitOutput
        restoreActivity := workflow.ExecuteActivity(ctx, gitcommit.RestoreCommit, restoreInput)
        if err := restoreActivity.Get(ctx, &restoreOutput); err != nil {
            // Log warning but continue - might be first operation
            workflow.GetLogger(ctx).Warn("Git restore failed", "error", err)
        }
    }
    
    // 2. Execute the actual recipe operation (as Temporal activity)
    result, err := c.executeRecipeOperation(ctx, op, nodeInputs, workflowInputs)
    if err != nil {
        return nil, err
    }
    
    // 3. Execute Git Persist Activity (after the operation)
    persistInput := gitcommit.PersistCommitActivity{
        RepoPath:        c.gitConfig.WorkingDir,
        RootHash:        c.gitConfig.BaseCommit,
        StorageLocation: c.gitConfig.PackStorage,
        CommitMessage:   fmt.Sprintf("After operation: %s", op),
        Author:          c.gitConfig.GitAuthor,
    }
    
    var persistOutput gitcommit.PersistCommitOutput
    persistActivity := workflow.ExecuteActivity(ctx, gitcommit.PersistCommit, persistInput)
    if err := persistActivity.Get(ctx, &persistOutput); err != nil {
        return nil, fmt.Errorf("git persist failed after %s: %w", op, err)
    }
    
    // Store commit hash in workflow state for next operation
    c.setLastCommitHash(ctx, persistOutput.CommitHash)
    
    return result, nil
}

// executeRecipeOperation executes the actual recipe operation as a Temporal activity
func (c *Compiler) executeRecipeOperation(
    ctx workflow.Context,
    op string,
    nodeInputs map[string]interface{},
    workflowInputs map[string]interface{},
) (map[string]interface{}, error) {
    // This is the existing executeOperation logic
    // It compiles the recipe op into a Temporal activity
    
    state := &WorkflowState{
        Inputs:  workflowInputs,
        Steps:   make(map[string]StepResult),
        Outputs: make(map[string]interface{}),
    }
    
    resolver := NewTemplateResolver(state)
    resolvedNodeInputs := make(map[string]interface{})
    for k, v := range nodeInputs {
        resolved, err := resolver.ResolveValue(v)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve template in input %s: %w", k, err)
        }
        resolvedNodeInputs[k] = resolved
    }
    
    // Execute as Temporal activity
    activityOptions := workflow.ActivityOptions{
        StartToCloseTimeout: 5 * time.Minute,
        RetryPolicy: &temporal.RetryPolicy{
            InitialInterval:    10 * time.Second,
            MaximumInterval:    1 * time.Minute,
            BackoffCoefficient: 2.0,
            MaximumAttempts:    3,
        },
    }
    ctx = workflow.WithActivityOptions(ctx, activityOptions)
    
    var result map[string]interface{}
    future := workflow.ExecuteActivity(ctx, op, resolvedNodeInputs)
    if err := future.Get(ctx, &result); err != nil {
        return nil, fmt.Errorf("activity %s failed: %w", op, err)
    }
    
    return result, nil
}


// Inject git activities at workflow start and end
func (c *Compiler) ExecuteWorkflow(ctx workflow.Context, def *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
    
    // 1. Inject Git Initialize Activity at workflow start
    if c.gitConfig.EnableGitIntegration {
        initInput := gitshallow.GitShallowCloneInput{
            SourceDir:  c.gitConfig.SourceRepo,
            TargetDir:  c.gitConfig.WorkingDir,
            CommitHash: c.gitConfig.BaseCommit,
        }
        
        var initOutput gitshallow.GitShallowCloneOutput
        initActivity := workflow.ExecuteActivity(ctx, gitshallow.GitShallowClone, initInput)
        if err := initActivity.Get(ctx, &initOutput); err != nil {
            return nil, fmt.Errorf("git initialization failed: %w", err)
        }
        
        // Set initial commit hash
        c.setLastCommitHash(ctx, c.gitConfig.BaseCommit)
    }
    
    // 2. Execute the recipe workflow (with git activities injected around each op)
    var result map[string]interface{}
    var err error
    
    if def.Op != "" {
        result, err = c.executeOperation(ctx, def.Op, def.Inputs, inputs)
    } else if len(def.Sequence) > 0 {
        result, err = c.executeSequenceNodes(ctx, def.Sequence, inputs)
    } else if len(def.Parallel) > 0 {
        result, err = c.executeParallelNodes(ctx, def.Parallel, inputs)
    } else if def.States != nil {
        if c.stateMachineCompiler != nil {
            result, err = c.stateMachineCompiler.ExecuteStateMap(ctx, def.States, inputs)
        } else {
            return nil, fmt.Errorf("state machine compiler not initialized")
        }
    } else {
        return nil, fmt.Errorf("recipe must define one of: op, sequence, parallel, or states")
    }
    
    if err != nil {
        return nil, err
    }
    
    // 3. Inject Git Merge Activity at workflow end
    if c.gitConfig.EnableGitIntegration && c.gitConfig.MergeBranch != "" {
        mergeInput := gitmerge.MergeChangesInput{
            WorkingDir:  c.gitConfig.WorkingDir,
            TargetRepo:  c.gitConfig.SourceRepo,
            BranchName:  c.gitConfig.MergeBranch,
            PushRemote:  c.gitConfig.PushRemote,
        }
        
        var mergeOutput gitmerge.MergeChangesOutput
        mergeActivity := workflow.ExecuteActivity(ctx, gitmerge.MergeChanges, mergeInput)
        if err := mergeActivity.Get(ctx, &mergeOutput); err != nil {
            // Log error but don't fail workflow - changes are preserved in thin packs
            workflow.GetLogger(ctx).Error("Git merge failed", "error", err)
        }
    }
    
    return result, nil
}
```

### 3. Workflow State Management

#### 3.1 Git State Storage
```go
// Store git state in workflow context
type GitWorkflowState struct {
    LastCommitHash string
    CommitHistory  []string
    WorkingDir     string
    Initialized    bool
}

// Use Temporal's workflow state management
func (c *Compiler) getGitState(ctx workflow.Context) *GitWorkflowState {
    var state GitWorkflowState
    workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
        // Retrieve state from workflow execution
        return state
    })
    return &state
}
```

### 4. Activity Registration

#### 4.1 Auto-Register Git Activities
```go
// pkg/worker/register_activities.go
func RegisterGitActivities(worker worker.Worker) {
    worker.RegisterActivity(gitshallow.GitShallowClone)
    worker.RegisterActivity(gitcommit.PersistCommit)
    worker.RegisterActivity(gitcommit.RestoreCommit)
    worker.RegisterActivity(gitmerge.MergeChanges)  // New activity
}
```

### 5. Configuration Interface

#### 5.1 Recipe Metadata for Git Integration
```yaml
# Recipe can override git settings
name: my_recipe
version: "1.0"

git_config:  # Optional section
  enabled: true  # Can disable git integration
  commit_prefix: "MyRecipe"  # Prefix for commit messages
  
input_schema:
  # Git-related inputs are automatically added when git_config.enabled = true
  # Users can override these at execution time
```

#### 5.2 Manual Git Ops Also Get Wrapped
Manual git operations in recipes get wrapped just like any other operation:
```yaml
name: recipe_with_manual_git
version: "1.0"

sequence:
  - id: manual_clone
    op: git_shallow_clone  # Gets wrapped with restore/persist
    inputs:
      source_dir: "/custom/repo"
      target_dir: "/custom/target"
      commit_hash: "abc123"
  
  - id: do_work
    op: command_execution  # Gets wrapped with restore/persist
    inputs:
      run: "make build"
  
  - id: manual_persist  
    op: git_persist  # Gets wrapped with restore/persist
    inputs:
      repo_path: "/custom/target"
      commit_message: "Manual checkpoint"
```

Every operation gets the same treatment - automatic git state tracking before and after, ensuring maximum consistency and simplicity.

### 6. Error Handling and Recovery

#### 6.1 Git Operation Failures
- If restore fails: Log warning, continue with current state (might be first operation)
- If persist fails: Retry with exponential backoff, fail workflow if max retries exceeded
- If initial clone fails: Fail workflow immediately
- If final merge fails: Store changes as thin packs, notify user for manual resolution

#### 6.2 Workflow Restart Behavior
When a workflow restarts:
1. Skip the initial clone (working directory exists)
2. Find the last successful commit from thin packs
3. Restore to that commit
4. Resume execution from the failed operation

### 7. Implementation Phases

#### Phase 1: Core Integration (MVP)
- Implement git wrapper functions
- Add configuration structure
- Modify executeOperation to use wrapper
- Test with simple sequential recipes

#### Phase 2: Advanced Features
- Handle parallel operations (coordinate git state)
- Implement state machine support
- Add merge operation at workflow end
- Support for branching strategies

#### Phase 3: Optimization
- Cache git operations where possible
- Optimize thin pack storage
- Add compression for thin packs
- Implement cleanup policies

### 8. Testing Strategy

#### 8.1 Unit Tests
- Test git wrapper logic
- Test skip operation detection
- Test state management

#### 8.2 Integration Tests
- Test full workflow with git integration
- Test workflow restart scenarios
- Test parallel operation handling
- Test error recovery

#### 8.3 Example Test Recipe
```yaml
name: test_git_integration
version: "1.0"

git_config:
  enabled: true

sequence:
  - id: modify_file
    op: command_execution
    inputs:
      run: "echo 'test' > test.txt"
  
  - id: check_file
    op: command_execution
    inputs:
      run: "cat test.txt"
```

### 9. Backward Compatibility

- Git integration is opt-in via configuration flag
- Existing recipes continue to work without modification
- Manual git operations in recipes are detected and skipped by auto-wrapper
- Configuration can be provided at runtime or compile time

### 10. Performance Considerations

- Thin packs are incremental (only store changes)
- Git operations are wrapped in Temporal activities (can be distributed)
- Restore operations use local repository when possible
- Configurable parallelism for git operations

## Migration Guide

### For Existing Recipes
1. All operations (including manual git ops) get wrapped with automatic git tracking
2. Manual git operations still execute their intended function
3. Add `git_config` section if you want to customize automatic injection behavior
4. Ensure input schema includes git-related parameters for automatic injection

### For New Recipes
1. Enable git integration in workflow configuration for automatic git tracking
2. Every single operation gets wrapped with git activities
3. No need to think about what gets wrapped - everything does
4. Maximum simplicity and consistency across all workflows

## Security Considerations

- Working directories are isolated per workflow execution
- Thin packs are stored in secure location with access controls
- Git author information can be enforced at configuration level
- Support for signed commits (future enhancement)