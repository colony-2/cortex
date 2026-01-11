# Spec: Cell Relative Path in TaskExecutionContext

## Overview

This specification describes the changes required to add the cell's relative path to `TaskExecutionContext` alongside the existing cell name, enabling operations and git state management to access both the cell name and its full path during recipe execution.

This is a non-breaking change. Existing jobs will continue to work, and new jobs can optionally provide the cell path.

## Problem Statement

Currently, `TaskExecutionContext` only carries the cell's name via `WorkflowContext.CellName`. This limits the ability of operations and git-related functionality to determine the full relative path of the cell within the repository structure.

### Current Limitation

```go
type WorkflowContext struct {
	CellName string `json:"cell,omitempty"`     // Only the cell name
	JobID    string `json:"job_id,omitempty"`
}
```

This means that if a cell is located at `path/to/my/cell`, only the cell name is available in the execution context, not the full path `"path/to/my/cell"`.

## Goals

1. Add cell relative path to `TaskExecutionContext` alongside the existing cell name
2. Make the relative path available throughout the execution pipeline when provided
3. Propagate both cell name and cell path information to all derived contexts (e.g., `GitTaskContext`)
4. Update git persist scope logic to use cell path instead of cell name
5. Both values serve different purposes

## Proposed Changes

### 1. Contextual Object Changes

#### 1.1 Update `WorkflowContext`

**File:** `server/recipe-core/pkg/contextual/context.go`

**Current:**
```go
type WorkflowContext struct {
	CellName string `json:"cell,omitempty"`
	JobID    string `json:"job_id,omitempty"`
}
```

**Proposed:**
```go
type WorkflowContext struct {
	CellName string `json:"cell,omitempty"`
	CellPath string `json:"cell_path,omitempty"`      // Cell relative path from repo root
	JobID    string `json:"job_id,omitempty"`
}
```

**Rationale:**
- Keeps existing `CellName` field unchanged
- Adds `CellPath` as an optional field to carry the full relative path
- `CellName` contains the cell name (e.g., "my-cell")
- `CellPath` contains the full relative path (e.g., "cells/my-cell" or "path/to/cell")
- **IMPORTANT:** These are two completely distinct, unrelated values - never use one as a fallback for the other

#### 1.2 Update `GitTaskContext`

**File:** `server/git/pkg/gitstate/git_task_context.go`

**Current:**
```go
type GitTaskContext struct {
	BaseRepo         string
	BaseRef          string
	ResolvedBaseHash string
	PersistHash      string
	ParentHash       string
	WorktreePath     string
	ThinPackPath     string
	TicketID         string
	CellName         string              // Only cell name
	GitAuthor        string
	NodePath         string
	InvokeSeq        int64
	InvokeHash       string
}
```

**Proposed:**
```go
type GitTaskContext struct {
	BaseRepo         string
	BaseRef          string
	ResolvedBaseHash string
	PersistHash      string
	ParentHash       string
	WorktreePath     string
	ThinPackPath     string
	TicketID         string
	CellName         string              // Cell name
	CellPath         string              // Cell relative path from repo root
	GitAuthor        string
	NodePath         string
	InvokeSeq        int64
	InvokeHash       string
}
```

**Update Constructor:**

**Current:**
```go
func NewGitTaskContext(tec contextual.TaskExecutionContext) *GitTaskContext {
	return &GitTaskContext{
		BaseRepo:         tec.GitBase.BaseRepo,
		BaseRef:          tec.GitBase.BaseRef,
		ResolvedBaseHash: tec.GitBase.ResolvedBaseHash,
		WorktreePath:     tec.Environment.WorktreePath,
		ThinPackPath:     tec.Environment.ThinPackPath,
		TicketID:         tec.Actor.TicketID,
		CellName:         tec.Workflow.CellName,
		GitAuthor:        tec.GitBase.GitAuthor,
		NodePath:         tec.Invocation.NodePath,
		InvokeSeq:        tec.Invocation.InvokeSeq,
		InvokeHash:       tec.Invocation.Hash(),
		// ... git commit context fields
	}
}
```

**Proposed:**
```go
func NewGitTaskContext(tec contextual.TaskExecutionContext) *GitTaskContext {
	return &GitTaskContext{
		BaseRepo:         tec.GitBase.BaseRepo,
		BaseRef:          tec.GitBase.BaseRef,
		ResolvedBaseHash: tec.GitBase.ResolvedBaseHash,
		WorktreePath:     tec.Environment.WorktreePath,
		ThinPackPath:     tec.Environment.ThinPackPath,
		TicketID:         tec.Actor.TicketID,
		CellName:         tec.Workflow.CellName,
		CellPath:         tec.Workflow.CellPath,    // Add cell path mapping
		GitAuthor:        tec.GitBase.GitAuthor,
		NodePath:         tec.Invocation.NodePath,
		InvokeSeq:        tec.Invocation.InvokeSeq,
		InvokeHash:       tec.Invocation.Hash(),
		// ... git commit context fields
	}
}
```

### 2. Job Start Call Changes

Cell path should be provided when creating and starting jobs, in addition to the existing cell name. This affects locations where `JobContext` is populated.

#### 2.1 Client-Side Changes

Wherever jobs are started, callers should populate both `CellName` and `CellPath`.

**Pattern - Current:**
```go
jobCtx := contextual.JobContext{
	Actor: contextual.ActorContext{
		TicketID:   ticketID,
		ActorName:  userName,
		ActorEmail: userEmail,
	},
	Environment: contextual.EnvironmentContext{
		WorktreePath: worktreePath,
		ThinPackPath: thinpackPath,
	},
	Workflow: contextual.WorkflowContext{
		CellName: cellName,        // Only cell name
		JobID:    jobID,
	},
	GitBase: contextual.GitBaseContext{
		BaseRepo:         repoURL,
		BaseRef:          baseRef,
		ResolvedBaseHash: baseHash,
		GitAuthor:        author,
	},
}

startJob := workflowctl.StartJob{
	TenantId:   tenantID,
	RecipeName: recipeName,
	Inputs:     inputs,
	JobContext: jobCtx,
	GitRef:     gitRef,
}
```

**Pattern - Proposed:**
```go
jobCtx := contextual.JobContext{
	Actor: contextual.ActorContext{
		TicketID:   ticketID,
		ActorName:  userName,
		ActorEmail: userEmail,
	},
	Environment: contextual.EnvironmentContext{
		WorktreePath: worktreePath,
		ThinPackPath: thinpackPath,
	},
	Workflow: contextual.WorkflowContext{
		CellName: cellName,        // Cell name (e.g., "my-cell")
		CellPath: cellPath,        // Cell relative path (e.g., "cells/my-cell")
		JobID:    jobID,
	},
	GitBase: contextual.GitBaseContext{
		BaseRepo:         repoURL,
		BaseRef:          baseRef,
		ResolvedBaseHash: baseHash,
		GitAuthor:        author,
	},
}

startJob := workflowctl.StartJob{
	TenantId:   tenantID,
	RecipeName: recipeName,
	Inputs:     inputs,
	JobContext: jobCtx,
	GitRef:     gitRef,
}
```

#### 2.2 Affected Call Sites

The following locations should be updated to populate `CellPath`:

1. **Ticket Service** - `server/ticket/internal/service/service.go`
   - This is where jobs are started in production
   - Change: Populate `JobContext.Workflow.CellPath` when creating `StartJob`
   - Current code sets `CellName` from `ticket.CellName`, needs to also set `CellPath`

2. **Standalone Executor** - `server/recipe-worker/pkg/executor/standalone.go`
   - Used for standalone recipe execution
   - Change: Add `CellPath` to `JobContext` when creating `StartJob`

3. **Integration Tests** - Test harnesses that create job contexts
   - `server/ops/pkg/codex/op_integration_test.go`
   - `server/ops/pkg/codex/op_resume_integration_test.go`
   - `server/recipe-worker/pkg/compiler/compiler_test.go`
   - `server/recipe-worker/pkg/compiler/capability_integration_test.go`
   - `server/recipe-input/pkg/input/activity_test.go`
   - Change: Update test setup to provide cell path
   ```go
   jobCtx, gitCtx := compiler.GenerateTestContext()
   jobCtx.Workflow.CellName = cellName   // Cell name
   jobCtx.Workflow.CellPath = cellRel    // Cell relative path
   ```

### 3. Data Flow

#### 3.1 Context Propagation Flow

```
Client/API Request
    ↓
Populates JobContext with:
  - CellName (cell name)
  - CellPath (relative path) - optional
    ↓
workflowctl.StartJob created
    ↓
Serialized and submitted to SWF Engine
    ↓
Job Worker deserializes StartJob
    ↓
ExecuteRecipe() receives JobContext
    ↓
NewRecipeResolutionContext() creates TaskExecutionContext
  (combines JobContext + TaskContext)
    ↓
TaskExecutionContext available in template resolution
  - context.workflow.cell_name (name)
  - context.workflow.cell_path (path)
    ↓
NewGitTaskContext() extracts to GitTaskContext
  - CellName
  - CellPath
    ↓
ActivityInvocationRequest passes GitTaskContext to operations
    ↓
Operations have access to both cell name and cell path
```

#### 3.2 Template Expression Access

Once the fields are added to the context objects, templates will automatically have access to both cell name and cell path (no template rendering code changes needed):

```yaml
# CEL expressions
some_name: ${context.workflow.cell_name}       # Returns "my-cell"
some_path: ${context.workflow.cell_path}       # Returns "cells/my-cell"

# Go templates
{{ .Context.Workflow.CellName }}               # Returns "my-cell"
{{ .Context.Workflow.CellPath }}               # Returns "cells/my-cell"
```

**Note:** The new fields will be automatically exposed through the existing template resolution mechanism. No changes to template rendering code are required.

### 4. Git Persist Scope Changes

The key change is updating git persist logic to use cell path for scoping instead of cell name.

**File:** `server/git/pkg/gitstate/workspace_controller.go` (or wherever scope is determined)

#### 4.1 Current Behavior

Currently, git persist automatically clears everything not in scope, where scope is based on cell name:

```go
// Current - uses CellName for scope
scope := gitTaskContext.CellName
// Git persist clears files outside this scope
```

**Problem:** Using cell name for scope doesn't provide the full path, leading to potential conflicts when multiple cells have the same name in different directories.

#### 4.2 Proposed Behavior

Git persist must fail if cell path is not provided:

```go
// Proposed - CellPath is required for git persist
if gitTaskContext.CellPath == "" {
	return fmt.Errorf("cell_path is required for git persist operations")
}

scope := gitTaskContext.CellPath
// Use cell path for scoping
```

**Rationale:**
- Cell path provides the full relative path for precise scoping
- Scoping by path allows proper isolation of cells in different directories
- Git persist operations require accurate scope information
- **IMPORTANT:** Never use CellName as a fallback for CellPath - they are distinct, unrelated values
- Failing when CellPath is missing ensures correct scoping behavior

#### 4.3 Impact

Git persist behavior:
- **CellPath is required** - git persist will fail/error if CellPath is not provided
- A cell at `cells/my-cell` will have scope `cells/my-cell`
- A cell at `path/to/cell` will have scope `path/to/cell`
- Git persist will clear files outside the cell's path
- Multiple cells with the same name in different paths won't conflict

Error handling:
- If CellPath is empty, git persist operations return an error
- Callers must provide CellPath to use git persist functionality

### 5. Testing Requirements

#### 5.1 Unit Tests

1. **Context Construction Tests**
   - Verify `NewTaskExecutionContext` preserves both `CellName` and `CellPath`
   - Verify `NewGitTaskContext` extracts both fields correctly

2. **Serialization Tests**
   - Verify `WorkflowContext` serializes/deserializes with both `CellName` and `CellPath`
   - Verify `StartJob` JSON includes cell path when provided

3. **Git Persist Scope Tests**
   - Test scope uses `CellPath` when provided
   - Test git persist fails/errors when `CellPath` is empty
   - Test that git persist clears files based on cell path scope
   - Verify error message when CellPath is missing

#### 5.2 Integration Tests

1. **End-to-End Job Execution**
   - Start a job with both `CellName` and `CellPath` populated
   - Verify both values are available in operation execution
   - Verify git operations can access both cell name and path

2. **Template Resolution**
   - Test CEL expressions can access both `context.workflow.cell_name` and `context.workflow.cell_path`
   - Test Go templates can access both `.Context.Workflow.CellName` and `.Context.Workflow.CellPath`
   - Verify fields are automatically exposed (no template rendering code changes needed)

3. **Git Persist Scope Integration**
   - Test that git persist uses cell path for scoping when CellPath is provided
   - Test that files outside cell path are cleared when CellPath is provided
   - Test that git persist fails when CellPath is not provided
   - Verify appropriate error is returned when CellPath is missing

## Implementation Checklist

- [ ] Add `CellPath` field to `WorkflowContext` (with `omitempty`)
- [ ] Add `CellPath` field to `GitTaskContext`
- [ ] Update `NewGitTaskContext` constructor to map both `CellName` and `CellPath`
- [ ] Update git persist scope logic to require `CellPath` and fail if not provided (do NOT use CellName as fallback)
- [ ] Update job start locations to populate `CellPath`:
  - [ ] Ticket service
  - [ ] Standalone executor
  - [ ] Integration tests
- [ ] Add unit tests for context construction and serialization
- [ ] Add unit tests for git persist scope logic
- [ ] Add integration tests for end-to-end propagation of cell path
- [ ] Add integration tests for git persist scope behavior
- [ ] Add integration tests to verify template access (fields automatically exposed, no code changes needed)

## References

- `server/recipe-core/pkg/contextual/context.go` - Context definitions
- `server/git/pkg/gitstate/git_task_context.go` - Git task context
- `server/recipe-worker/pkg/compiler/job_worker.go` - Job execution entry
- `server/recipe-worker/pkg/compiler/compiler.go` - Recipe compilation
- `server/recipe-template/pkg/template/template_resolver.go` - Template resolution
