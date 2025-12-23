# 01 - Workflow OpenAPI Bindings Generation

**Project**: `server/openapi`

**Dependencies**: `00-workflow-openapi-spec.md`

**Implements**: Go type bindings from OpenAPI specification

---

## Overview

Generate Go types and server interfaces from the extended OpenAPI specification. This provides type-safe request/response structs and handler interfaces for workflow endpoints.

**Tool**: `oapi-codegen`

**Output**: `server/openapi/pkg/openapi/generated.go`

---

## Code Generation

### Step 1: Verify Generator Configuration

**File**: `server/openapi/tools.go` or `server/openapi/generate.go`

Ensure oapi-codegen is configured:

```go
//go:generate oapi-codegen -package openapi -generate types,server,spec -o pkg/openapi/generated.go ../../api/openapi/openapi.yaml
```

**Verify configuration**:
- Package: `openapi`
- Generate: `types,server,spec`
- Input: `../../api/openapi/openapi.yaml` (relative path from server/openapi)
- Output: `pkg/openapi/generated.go`

### Step 2: Run Code Generation

```bash
cd server/openapi
go generate ./...
```

**Alternative (if using moon/nx)**:
```bash
moon run server-openapi:generate
```

### Step 3: Verify Generated Types

Check `server/openapi/pkg/openapi/generated.go` contains:

**Enums**:
```go
type WorkflowStatus string

const (
    WorkflowStatusRunning    WorkflowStatus = "running"
    WorkflowStatusCompleted  WorkflowStatus = "completed"
    WorkflowStatusFailed     WorkflowStatus = "failed"
    WorkflowStatusCanceled   WorkflowStatus = "canceled"
    WorkflowStatusTerminated WorkflowStatus = "terminated"
    WorkflowStatusTimedOut   WorkflowStatus = "timed_out"
    WorkflowStatusUnknown    WorkflowStatus = "unknown"
)

type ChapterStatus string

const (
    ChapterStatusPending   ChapterStatus = "pending"
    ChapterStatusRunning   ChapterStatus = "running"
    ChapterStatusCompleted ChapterStatus = "completed"
    ChapterStatusFailed    ChapterStatus = "failed"
    ChapterStatusSkipped   ChapterStatus = "skipped"
)
```

**Structs**:
```go
type WorkflowSummary struct {
    WorkflowId   string         `json:"workflow_id"`
    RunId        string         `json:"run_id"`
    Status       WorkflowStatus `json:"status"`
    RecipeName   string         `json:"recipe_name"`
    TicketId     *string        `json:"ticket_id,omitempty"`
    TicketTitle  *string        `json:"ticket_title,omitempty"`
    CellId       *string        `json:"cell_id,omitempty"`
    CellName     *string        `json:"cell_name,omitempty"`
    StartTime    *time.Time     `json:"start_time,omitempty"`
    CloseTime    *time.Time     `json:"close_time,omitempty"`
    Actor        Actor          `json:"actor"`
    CreatedAt    time.Time      `json:"created_at"`
}

type WorkflowDetail struct {
    WorkflowId   string          `json:"workflow_id"`
    RunId        string          `json:"run_id"`
    Status       WorkflowStatus  `json:"status"`
    RecipeName   string          `json:"recipe_name"`
    TicketId     *string         `json:"ticket_id,omitempty"`
    Ticket       *Ticket         `json:"ticket,omitempty"`
    CellId       *string         `json:"cell_id,omitempty"`
    CellName     *string         `json:"cell_name,omitempty"`
    StartTime    *time.Time      `json:"start_time,omitempty"`
    CloseTime    *time.Time      `json:"close_time,omitempty"`
    Actor        Actor           `json:"actor"`
    GitRef       *string         `json:"git_ref,omitempty"`
    GitCommit    *string         `json:"git_commit,omitempty"`
    Chapters     []ChapterDetail `json:"chapters"`
    RawJobData   *map[string]interface{} `json:"raw_job_data,omitempty"`
    CreatedAt    time.Time       `json:"created_at"`
}

type ChapterDetail struct {
    ChapterNumber int                    `json:"chapter_number"`
    ChapterType   string                 `json:"chapter_type"`
    OpName        *string                `json:"op_name,omitempty"`
    Status        ChapterStatus          `json:"status"`
    StartTime     *time.Time             `json:"start_time,omitempty"`
    EndTime       *time.Time             `json:"end_time,omitempty"`
    Input         map[string]interface{} `json:"input"`
    Output        *map[string]interface{} `json:"output,omitempty"`
    Error         *string                `json:"error,omitempty"`
    Artifacts     []ArtifactReference    `json:"artifacts"`
}

type ArtifactReference struct {
    ArtifactId   string     `json:"artifact_id"`
    ArtifactType string     `json:"artifact_type"`
    Name         string     `json:"name"`
    SizeBytes    *int64     `json:"size_bytes,omitempty"`
    Url          *string    `json:"url,omitempty"`
    CreatedAt    time.Time  `json:"created_at"`
}
```

**Server Interface**:
```go
type ServerInterface interface {
    // ... existing methods ...

    // List workflows for a project
    // (GET /api/projects/{projectId}/workflows)
    GetApiProjectsWorkflows(w http.ResponseWriter, r *http.Request, projectId string, params GetApiProjectsWorkflowsParams)

    // Get detailed workflow execution information
    // (GET /api/projects/{projectId}/workflows/{workflowId})
    GetApiProjectsWorkflows1(w http.ResponseWriter, r *http.Request, projectId string, workflowId string, params GetApiProjectsWorkflows1Params)
}

type GetApiProjectsWorkflowsParams struct {
    Status   *[]WorkflowStatus `form:"status,omitempty" json:"status,omitempty"`
    TicketId *string           `form:"ticket_id,omitempty" json:"ticket_id,omitempty"`
    CellId   *string           `form:"cell_id,omitempty" json:"cell_id,omitempty"`
    Since    *time.Time        `form:"since,omitempty" json:"since,omitempty"`
    Until    *time.Time        `form:"until,omitempty" json:"until,omitempty"`
    Limit    *int              `form:"limit,omitempty" json:"limit,omitempty"`
    Offset   *int              `form:"offset,omitempty" json:"offset,omitempty"`
}

type GetApiProjectsWorkflows1Params struct {
    IncludeRawJobData *bool `form:"includeRawJobData,omitempty" json:"includeRawJobData,omitempty"`
}
```

---

## Validation

### Step 1: Compilation Check

```bash
cd server/openapi
go build ./...
```

**Expected**: No compilation errors.

### Step 2: Type Verification Test

Create a quick test to verify types are usable:

**File**: `server/openapi/pkg/openapi/types_test.go`

```go
package openapi_test

import (
    "testing"
    "time"

    "github.com/colony-2/colony2/server/openapi/pkg/openapi"
    "github.com/stretchr/testify/assert"
)

func TestWorkflowTypesGenerated(t *testing.T) {
    // Test WorkflowSummary construction
    summary := openapi.WorkflowSummary{
        WorkflowId: "prj_123_abc",
        RunId:      "prj_123_abc",
        Status:     openapi.WorkflowStatusRunning,
        RecipeName: "test_recipe",
        CreatedAt:  time.Now(),
    }

    assert.Equal(t, "prj_123_abc", summary.WorkflowId)
    assert.Equal(t, openapi.WorkflowStatusRunning, summary.Status)
}

func TestWorkflowDetailTypesGenerated(t *testing.T) {
    // Test WorkflowDetail construction
    detail := openapi.WorkflowDetail{
        WorkflowId: "prj_123_abc",
        RunId:      "prj_123_abc",
        Status:     openapi.WorkflowStatusCompleted,
        RecipeName: "test_recipe",
        Chapters:   []openapi.ChapterDetail{},
        CreatedAt:  time.Now(),
    }

    assert.Equal(t, "prj_123_abc", detail.WorkflowId)
    assert.Equal(t, openapi.WorkflowStatusCompleted, detail.Status)
    assert.NotNil(t, detail.Chapters)
}

func TestChapterDetailTypesGenerated(t *testing.T) {
    // Test ChapterDetail construction
    chapter := openapi.ChapterDetail{
        ChapterNumber: 1,
        ChapterType:   "op",
        Status:        openapi.ChapterStatusCompleted,
        Input:         map[string]interface{}{"key": "value"},
        Artifacts:     []openapi.ArtifactReference{},
    }

    assert.Equal(t, 1, chapter.ChapterNumber)
    assert.Equal(t, "op", chapter.ChapterType)
    assert.Equal(t, openapi.ChapterStatusCompleted, chapter.Status)
}

func TestWorkflowStatusConstants(t *testing.T) {
    // Verify all status constants are defined
    statuses := []openapi.WorkflowStatus{
        openapi.WorkflowStatusRunning,
        openapi.WorkflowStatusCompleted,
        openapi.WorkflowStatusFailed,
        openapi.WorkflowStatusCanceled,
        openapi.WorkflowStatusTerminated,
        openapi.WorkflowStatusTimedOut,
        openapi.WorkflowStatusUnknown,
    }

    assert.Len(t, statuses, 7)
}

func TestChapterStatusConstants(t *testing.T) {
    // Verify all chapter status constants are defined
    statuses := []openapi.ChapterStatus{
        openapi.ChapterStatusPending,
        openapi.ChapterStatusRunning,
        openapi.ChapterStatusCompleted,
        openapi.ChapterStatusFailed,
        openapi.ChapterStatusSkipped,
    }

    assert.Len(t, statuses, 5)
}
```

Run test:
```bash
cd server/openapi
go test ./pkg/openapi/...
```

### Step 3: Interface Verification

Verify `ServerInterface` includes new methods:

```bash
cd server/openapi
grep -A 2 "GetApiProjectsWorkflows" pkg/openapi/generated.go
```

**Expected output**:
```go
GetApiProjectsWorkflows(w http.ResponseWriter, r *http.Request, projectId string, params GetApiProjectsWorkflowsParams)
GetApiProjectsWorkflows1(w http.ResponseWriter, r *http.Request, projectId string, workflowId string, params GetApiProjectsWorkflows1Params)
```

---

## Troubleshooting

### Issue: Generation Fails

**Error**: `oapi-codegen: command not found`

**Solution**: Install oapi-codegen:
```bash
go install github.com/deepmap/oapi-codegen/v2/cmd/oapi-codegen@latest
```

### Issue: Types Not Generated

**Check**:
1. OpenAPI spec is valid (no YAML syntax errors)
2. Input path in `//go:generate` is correct
3. Run generation with verbose output:
   ```bash
   oapi-codegen -package openapi -generate types,server,spec -o pkg/openapi/generated.go ../../api/openapi/openapi.yaml
   ```

### Issue: Time Import Missing

If `time.Time` types cause errors, ensure:
```go
import "time"
```
is present at top of generated file (oapi-codegen should add this automatically).

### Issue: Compilation Errors on Generated Code

**Common causes**:
- Missing dependencies (e.g., `Actor`, `Ticket` types not defined)
- Circular imports
- Invalid OpenAPI spec (additionalProperties issues)

**Solution**: Review OpenAPI spec for missing schema references, ensure all `$ref` targets exist.

---

## Commit Changes

```bash
cd server/openapi
git add pkg/openapi/generated.go
git commit -m "feat(openapi): regenerate bindings with workflow types

- Add WorkflowSummary, WorkflowDetail, ChapterDetail types
- Add GetApiProjectsWorkflows and GetApiProjectsWorkflows1 interfaces
- Generated from openapi.yaml with workflow endpoints"
```

---

## Next Steps

After completing this spec:
1. Generated types are ready for use in **02-workflow-service.md**
2. Handler interfaces are ready for implementation in **03-workflow-api-handlers.md**

---

## Files Modified

- `server/openapi/pkg/openapi/generated.go` - Auto-generated from openapi.yaml
- `server/openapi/pkg/openapi/types_test.go` - Verification tests (new)
