# Git Workflow Integration Specification

## Overview
This specification describes the automatic decoration of Temporal activities with git state management during the recipe-to-temporal workflow compilation process. The goal is to ensure every recipe-driven workflow automatically maintains git state through activity decoration, enabling reliable workflow restarts and providing a complete audit trail of changes.

## Key Principles
- **Activity Decoration Pattern**: Each activity is decorated to handle git restore/persist internally, not as separate activities
- **Commit Chain Passing**: Activities pass the current git commit hash as part of their output to the next activity
- **No Direct Git Operations**: Persist/restore operations are internal only - not exposed as recipe ops
- **Transparent Integration**: The recipe structure remains unchanged; decoration happens at the compiler level
- **Simplified State Flow**: Git state flows through activity outputs rather than workflow state management

## Current State
- **Recipe Worker**: Located in `server/recipe-worker`, compiles recipes to Temporal workflows
- **Git Operations**: Located in `server/git`, provides internal git functionality:
  - `GitShallowClone`: Initial repository setup (remains as standalone activity)
  - `PersistCommit`: Internal function for committing changes (NOT exposed as recipe op)
  - `RestoreCommit`: Internal function for restoring state (NOT exposed as recipe op)

This specification proposes that persist/restore operations become internal-only functions used by the activity decorator, never directly accessible from recipes.

## Proposed Architecture

### 1. Activity Decorator Pattern

Instead of injecting separate git activities, each recipe operation activity is decorated with git functionality:

#### 1.1 Decorated Activity Structure
```go
// Each activity receives the previous commit hash and outputs the new one
type DecoratedActivityInput struct {
    // Git state
    PreviousCommit string                  // Commit hash from previous activity
    GitConfig      GitOperationConfig      // Git configuration
    
    // Original activity input
    OriginalInput  map[string]interface{}  // The actual recipe operation input
}

type DecoratedActivityOutput struct {
    // Git state
    CurrentCommit  string                  // Commit hash after this activity
    
    // Original activity output  
    OriginalOutput map[string]interface{}  // The actual recipe operation output
}
```

#### 1.2 Activity Decorator Implementation
```go
// Decorator wraps each activity with git restore/persist
func DecorateActivityWithGit(originalActivity interface{}, gitConfig GitOperationConfig) func(ctx context.Context, input DecoratedActivityInput) (DecoratedActivityOutput, error) {
    return func(ctx context.Context, input DecoratedActivityInput) (DecoratedActivityOutput, error) {
        // 1. Restore to previous commit (internal operation)
        if input.PreviousCommit != "" {
            err := restoreCommitInternal(gitConfig.WorkingDir, input.PreviousCommit, gitConfig)
            if err != nil {
                return DecoratedActivityOutput{}, fmt.Errorf("failed to restore: %w", err)
            }
        }
        
        // 2. Execute the original activity
        result, err := executeOriginalActivity(ctx, originalActivity, input.OriginalInput)
        if err != nil {
            return DecoratedActivityOutput{}, err
        }
        
        // 3. Persist current state (internal operation)
        commitHash, err := persistCommitInternal(gitConfig.WorkingDir, gitConfig)
        if err != nil {
            return DecoratedActivityOutput{}, fmt.Errorf("failed to persist: %w", err)
        }
        
        return DecoratedActivityOutput{
            CurrentCommit:  commitHash,
            OriginalOutput: result,
        }, nil
    }
}
```

#### 1.3 Workflow Start: Git Initialize Activity
```go
// Git initialization remains as a standalone activity
initOutput, err := workflow.ExecuteActivity(ctx, gitshallow.GitShallowClone,
    gitshallow.GitShallowCloneInput{
        SourceDir:  workflowInputs["__source_repo"].(string),
        TargetDir:  workflowInputs["__working_dir"].(string),
        CommitHash: workflowInputs["__base_commit"].(string),
    }).Get(ctx, &initOutput)

initialCommit := initOutput.CommitHash
```

#### 1.4 Workflow End: Git Merge Activity  
```go
// Git merge remains as a standalone activity
mergeOutput, err := workflow.ExecuteActivity(ctx, gitmerge.MergeChanges,
    gitmerge.MergeChangesInput{
        WorkingDir:   gitConfig.WorkingDir,
        FinalCommit:  lastCommit,
        TargetRepo:   gitConfig.SourceRepo,
        BranchName:   gitConfig.MergeBranch,
        PushRemote:   gitConfig.PushRemote,
    }).Get(ctx, &mergeOutput)
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

type GitOperationConfig struct {
    WorkingDir      string
    BaseCommit      string
    PackStorage     string
    GitAuthor       string
    CommitMessage   string
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

#### 2.3 Activity Decoration Logic
```go
// pkg/compiler/git_decoration.go

// Modified executeOperation to use decorated activities
func (c *Compiler) executeOperation(
    ctx workflow.Context, 
    op string, 
    nodeInputs map[string]interface{}, 
    workflowInputs map[string]interface{},
    previousCommit string, // Passed from previous activity or init
) (map[string]interface{}, string, error) { // Returns output AND commit hash
    
    // If git integration is disabled, execute the recipe op normally
    if !c.gitConfig.EnableGitIntegration {
        result, err := c.executeRecipeOperation(ctx, op, nodeInputs, workflowInputs)
        return result, "", err // No commit hash when git disabled
    }
    
    // Create decorated input
    decoratedInput := DecoratedActivityInput{
        PreviousCommit: previousCommit,
        GitConfig: GitOperationConfig{
            WorkingDir:    c.gitConfig.WorkingDir,
            BaseCommit:    c.gitConfig.BaseCommit,
            PackStorage:   c.gitConfig.PackStorage,
            GitAuthor:     c.gitConfig.GitAuthor,
            CommitMessage: fmt.Sprintf("After operation: %s", op),
        },
        OriginalInput: nodeInputs,
    }
    
    // Execute the decorated activity
    var decoratedOutput DecoratedActivityOutput
    decoratedActivity := c.getDecoratedActivity(op)
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
    
    future := workflow.ExecuteActivity(ctx, decoratedActivity, decoratedInput)
    if err := future.Get(ctx, &decoratedOutput); err != nil {
        return nil, previousCommit, fmt.Errorf("decorated activity %s failed: %w", op, err)
    }
    
    return decoratedOutput.OriginalOutput, decoratedOutput.CurrentCommit, nil
}

// getDecoratedActivity returns a decorated version of the recipe operation
func (c *Compiler) getDecoratedActivity(op string) interface{} {
    originalActivity := c.activityRegistry.GetActivity(op)
    if originalActivity == nil {
        return nil
    }
    
    // Return the decorated version that includes git operations
    return DecorateActivityWithGit(originalActivity, c.gitConfig)
}

// Internal git operations (not exposed as recipe ops)
func restoreCommitInternal(workingDir string, targetCommit string, config GitOperationConfig) error {
    // Implementation of git restore - internal only
    // This is NOT available as a recipe operation
    return gitcommit.RestoreCommitInternal(gitcommit.RestoreCommitParams{
        RepoPath:        workingDir,
        TargetCommit:    targetCommit,
        RootHash:        config.BaseCommit,
        StorageLocation: config.PackStorage,
        Force:           true,
    })
}

func persistCommitInternal(workingDir string, config GitOperationConfig) (string, error) {
    // Implementation of git persist - internal only
    // This is NOT available as a recipe operation
    return gitcommit.PersistCommitInternal(gitcommit.PersistCommitParams{
        RepoPath:        workingDir,
        RootHash:        config.BaseCommit,
        StorageLocation: config.PackStorage,
        CommitMessage:   config.CommitMessage,
        Author:          config.GitAuthor,
    })
}


// Execute workflow with commit chain passing
func (c *Compiler) ExecuteWorkflow(ctx workflow.Context, def *yamlpkg.RecipeDefinition, inputs map[string]interface{}) (map[string]interface{}, error) {
    
    var currentCommit string
    
    // 1. Git Initialize Activity at workflow start (if enabled)
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
        
        currentCommit = initOutput.CommitHash
    }
    
    // 2. Execute the recipe workflow (with decorated activities passing commits)
    var result map[string]interface{}
    var finalCommit string
    var err error
    
    if def.Op != "" {
        result, finalCommit, err = c.executeOperation(ctx, def.Op, def.Inputs, inputs, currentCommit)
    } else if len(def.Sequence) > 0 {
        result, finalCommit, err = c.executeSequenceNodes(ctx, def.Sequence, inputs, currentCommit)
    } else if len(def.Parallel) > 0 {
        result, finalCommit, err = c.executeParallelNodes(ctx, def.Parallel, inputs, currentCommit)
    } else if def.States != nil {
        if c.stateMachineCompiler != nil {
            result, finalCommit, err = c.executeStateMap(ctx, def.States, inputs, currentCommit)
        } else {
            return nil, fmt.Errorf("state machine compiler not initialized")
        }
    } else {
        return nil, fmt.Errorf("recipe must define one of: op, sequence, parallel, or states")
    }
    
    if err != nil {
        return nil, err
    }
    
    // 3. Git Merge Activity at workflow end (if enabled)
    if c.gitConfig.EnableGitIntegration && c.gitConfig.MergeBranch != "" {
        mergeInput := gitmerge.MergeChangesInput{
            WorkingDir:   c.gitConfig.WorkingDir,
            FinalCommit:  finalCommit,
            TargetRepo:   c.gitConfig.SourceRepo,
            BranchName:   c.gitConfig.MergeBranch,
            PushRemote:   c.gitConfig.PushRemote,
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

// Execute sequence with commit chain
func (c *Compiler) executeSequenceNodes(ctx workflow.Context, nodes []yamlpkg.RecipeNode, inputs map[string]interface{}, currentCommit string) (map[string]interface{}, string, error) {
    var result map[string]interface{}
    var err error
    
    for _, node := range nodes {
        result, currentCommit, err = c.executeNode(ctx, node, inputs, currentCommit)
        if err != nil {
            return nil, currentCommit, err
        }
    }
    
    return result, currentCommit, nil
}
```

### 3. Workflow State Management

#### 3.1 Git State Through Activity Outputs
```go
// No explicit workflow state management needed!
// Git state flows through activity outputs naturally

// Each activity in the chain receives the previous commit
// and outputs the new commit for the next activity
// This eliminates the need for complex state management

// Example flow:
// Init -> commit1 -> Activity1(commit1) -> commit2 -> Activity2(commit2) -> commit3 -> ...
```

### 4. Activity Registration

#### 4.1 Register Decorated Activities
```go
// pkg/worker/register_activities.go
func RegisterActivities(worker worker.Worker, gitConfig *GitWorkflowConfig) {
    // Only register git init and merge as standalone activities
    worker.RegisterActivity(gitshallow.GitShallowClone)
    worker.RegisterActivity(gitmerge.MergeChanges)
    
    // All recipe operations are registered as decorated activities
    for name, activity := range activityRegistry.GetAll() {
        if gitConfig.EnableGitIntegration {
            // Register the decorated version
            decorated := DecorateActivityWithGit(activity, gitConfig)
            worker.RegisterActivity(decorated)
        } else {
            // Register the original version
            worker.RegisterActivity(activity)
        }
    }
    
    // IMPORTANT: PersistCommit and RestoreCommit are NOT registered
    // They are internal functions only, not Temporal activities
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

#### 5.2 No Manual Git Operations Allowed
```yaml
name: recipe_with_operations
version: "1.0"

sequence:
  # Git operations are NOT available as recipe ops
  # The following would be INVALID:
  # - op: git_persist       # ERROR: Not a valid operation
  # - op: git_restore       # ERROR: Not a valid operation
  
  # Only regular operations are allowed:
  - id: do_work
    op: command_execution  # Automatically decorated with git operations
    inputs:
      run: "make build"
  
  - id: run_tests
    op: command_execution  # Automatically decorated with git operations
    inputs:
      run: "make test"
```

Git persist/restore are completely internal - users cannot reference them directly. Git state management happens transparently through the decorator pattern.

### 6. Error Handling and Recovery

#### 6.1 Git Operation Failures
- If internal restore fails: Activity fails and retries according to retry policy
- If internal persist fails: Activity fails and retries according to retry policy
- If initial clone fails: Fail workflow immediately
- If final merge fails: Store changes as thin packs, notify user for manual resolution

#### 6.2 Workflow Restart Behavior
When a workflow restarts:
1. Temporal replays completed activities (including their commit outputs)
2. The commit chain is reconstructed from activity outputs
3. Resume execution from the failed activity with the correct commit
4. No explicit state management needed - Temporal handles it

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
- Test activity decoration logic
- Test commit chain passing
- Test internal git operations (restore/persist)
- Verify git_persist/git_restore are not exposed

#### 8.2 Integration Tests
- Test full workflow with decorated activities
- Test workflow restart with commit chain reconstruction
- Test parallel operation handling with commit synchronization
- Test error recovery within decorated activities

#### 8.3 Example Test Recipe
```yaml
name: test_git_integration
version: "1.0"

git_config:
  enabled: true

sequence:
  - id: modify_file
    op: command_execution  # Decorated automatically
    inputs:
      run: "echo 'test' > test.txt"
  
  - id: check_file
    op: command_execution  # Receives commit from previous activity
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
1. Remove any manual git persist/restore operations - they are no longer valid
2. Git state management happens automatically through decoration
3. Add `git_config` section if you want to customize git behavior
4. Git shallow clone remains available for custom repository setup

### For New Recipes
1. DO NOT use git_persist or git_restore operations - they don't exist
2. Enable git integration in workflow configuration for automatic tracking
3. Every operation is automatically decorated with git functionality
4. Git state flows through activity outputs, not workflow state

## Security Considerations

- Working directories are isolated per workflow execution
- Thin packs are stored in secure location with access controls
- Git author information can be enforced at configuration level
- Support for signed commits (future enhancement)