# Artifact Retrieval Endpoint Specification

## Overview

Add support for retrieving specific artifacts from workflow chapters via a new REST API endpoint. The UI already supports downloading artifacts via URLs, so this change focuses on backend implementation only.

## Goals

1. Add OpenAPI endpoint definition for artifact retrieval
2. Implement artifact retrieval in workflow service
3. Add handler in testserver to support the endpoint
4. Populate artifact URLs in workflow details responses
5. Comprehensive testing using OpenAPI-generated bindings

## Current State

### Existing Artifacts Structure

From `/src/server/workflow/internal/model/types.go`:

```go
type ArtifactReference struct {
    ArtifactID   string    // unique artifact ID
    ArtifactType string    // type/category of artifact
    Name         string    // display name
    SizeBytes    *int64    // file size (optional)
    URL          *string   // URL to access artifact (optional) - CURRENTLY UNPOPULATED
    CreatedAt    time.Time // when artifact was created
}
```

**Current behavior**: `URL` field exists but is not populated. Artifacts are returned in `ChapterDetail.Artifacts[]` but cannot be downloaded.

**Note on mime types**: Artifacts do not have associated mime types stored. All artifacts will be served as `application/octet-stream` with a filename in the Content-Disposition header, allowing browsers to handle downloads appropriately.

**Note on SWF artifact API**: The SWF (Simple Workflow) system already provides built-in artifact storage and retrieval:
- Artifacts are retrieved via `strata.Chapter(ctx, storyKey, chapterNumber)` which returns chapter details
- Each chapter has `Artifacts()` method returning `[]swf.Artifact`
- Each `swf.Artifact` has a `Bytes(ctx)` method to retrieve content
- No new storage APIs need to be implemented

### Existing Workflow Endpoints

```
GET /api/projects/{projectId}/workflows                    - List workflows
GET /api/projects/{projectId}/workflows/{workflowId}      - Get workflow details (includes artifacts)
```

## Proposed Changes

### 1. OpenAPI Specification Changes

**File**: `/src/api/openapi/colony2-api.yaml`

#### Add New Endpoint

```yaml
paths:
  /api/projects/{projectId}/workflows/{workflowId}/chapters/{chapterNumber}/artifacts/{artifactId}:
    get:
      operationId: getWorkflowArtifact
      summary: Retrieve a specific artifact from a workflow chapter
      description: |
        Downloads a specific artifact file produced by a chapter in a workflow.
        Returns the artifact content as application/octet-stream (artifacts do not have associated mime types).
      tags:
        - Workflows
      parameters:
        - name: projectId
          in: path
          required: true
          schema:
            type: string
          description: Project ID
        - name: workflowId
          in: path
          required: true
          schema:
            type: string
          description: Workflow ID
        - name: chapterNumber
          in: path
          required: true
          schema:
            type: integer
            minimum: 0
          description: Chapter number (0-indexed)
        - name: artifactId
          in: path
          required: true
          schema:
            type: string
          description: Artifact ID
      responses:
        '200':
          description: Artifact content (binary download)
          content:
            application/octet-stream:
              schema:
                type: string
                format: binary
          headers:
            Content-Disposition:
              schema:
                type: string
              description: Suggested filename for download
        '404':
          description: Artifact not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
        '500':
          description: Internal server error
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
```

#### Update ArtifactReference Schema (if not already complete)

Ensure the `ArtifactReference` schema includes the `url` field:

```yaml
components:
  schemas:
    ArtifactReference:
      type: object
      required:
        - artifactId
        - artifactType
        - name
        - createdAt
      properties:
        artifactId:
          type: string
          description: Unique identifier for the artifact
        artifactType:
          type: string
          description: Type/category of artifact
        name:
          type: string
          description: Display name of the artifact
        sizeBytes:
          type: integer
          format: int64
          nullable: true
          description: Size of the artifact in bytes
        url:
          type: string
          nullable: true
          description: URL to download the artifact
        createdAt:
          type: string
          format: date-time
          description: When the artifact was created
```

### 2. Workflow Service Changes

**File**: `/src/server/workflow/pkg/workflow/workflow.go`

#### Add New Service Interface Method

```go
type Service interface {
    ListWorkflows(ctx context.Context, req ListWorkflowsRequest) ([]WorkflowSummary, error)
    GetWorkflow(ctx context.Context, req GetWorkflowRequest) (*WorkflowDetail, error)

    // NEW: Retrieve specific artifact
    GetWorkflowArtifact(ctx context.Context, req GetWorkflowArtifactRequest) (*ArtifactData, error)
}

// NEW: Request structure
type GetWorkflowArtifactRequest struct {
    ProjectID     string
    WorkflowID    string
    ChapterNumber int
    ArtifactID    string
}

// NEW: Response structure
type ArtifactData struct {
    Content   []byte            // The artifact file content
    Filename  string            // Suggested filename for Content-Disposition header
    SizeBytes int64             // Size of the content
    Metadata  map[string]string // Optional additional metadata
}
```

**File**: `/src/server/workflow/internal/service/service.go`

#### Implement GetWorkflowArtifact

```go
func (s *service) GetWorkflowArtifact(
    ctx context.Context,
    req workflow.GetWorkflowArtifactRequest,
) (*workflow.ArtifactData, error) {
    // 1. Validate request
    if req.ProjectID == "" || req.WorkflowID == "" || req.ArtifactID == "" {
        return nil, fmt.Errorf("missing required parameters")
    }
    if req.ChapterNumber < 0 {
        return nil, fmt.Errorf("invalid chapter number: %d", req.ChapterNumber)
    }

    // 2. Get workflow to verify existence and get run ID
    workflowDetail, err := s.GetWorkflow(ctx, workflow.GetWorkflowRequest{
        ProjectID:  req.ProjectID,
        WorkflowID: req.WorkflowID,
    })
    if err != nil {
        return nil, fmt.Errorf("workflow not found: %w", err)
    }

    // 3. Verify chapter exists
    if req.ChapterNumber >= len(workflowDetail.Chapters) {
        return nil, fmt.Errorf("chapter %d not found (workflow has %d chapters)",
            req.ChapterNumber, len(workflowDetail.Chapters))
    }

    // 4. Verify artifact exists in chapter
    chapter := workflowDetail.Chapters[req.ChapterNumber]
    var artifactRef *model.ArtifactReference
    for i := range chapter.Artifacts {
        if chapter.Artifacts[i].ArtifactID == req.ArtifactID {
            artifactRef = &chapter.Artifacts[i]
            break
        }
    }
    if artifactRef == nil {
        return nil, fmt.Errorf("artifact %s not found in chapter %d",
            req.ArtifactID, req.ChapterNumber)
    }

    // 5. Get the chapter from strata to access its artifacts
    jobKey := swf.JobKey{
        TenantID:   req.ProjectID,
        WorkflowID: req.WorkflowID,
        RunID:      workflowDetail.RunID,
    }

    chap, err := s.strata.Chapter(ctx, jobKey.ToStoryKey(), int64(req.ChapterNumber))
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve chapter: %w", err)
    }

    // 6. Find the artifact in the chapter's artifact list
    var targetArtifact swf.Artifact
    for _, art := range chap.Artifacts() {
        if art.ID() == req.ArtifactID {
            targetArtifact = art
            break
        }
    }
    if targetArtifact == nil {
        return nil, fmt.Errorf("artifact %s not found in chapter %d",
            req.ArtifactID, req.ChapterNumber)
    }

    // 7. Retrieve artifact bytes using SWF's built-in method
    content, err := targetArtifact.Bytes(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to read artifact content: %w", err)
    }

    // 8. Return artifact data
    return &workflow.ArtifactData{
        Content:   content,
        Filename:  artifactRef.Name,
        SizeBytes: int64(len(content)),
        Metadata: map[string]string{
            "artifactType":  artifactRef.ArtifactType,
            "chapterNumber": fmt.Sprintf("%d", req.ChapterNumber),
        },
    }, nil
}
```

**Note**: This implementation uses the existing SWF/Strata APIs:
- `s.strata.Chapter(ctx, storyKey, chapterNumber)` to retrieve chapter details
- `chap.Artifacts()` to get the list of artifacts for that chapter
- `artifact.Bytes(ctx)` to retrieve the artifact content (built-in SWF artifact method)

No new strata client methods are needed - we leverage existing SWF artifact retrieval capabilities.

#### Update GetWorkflow to Populate Artifact URLs

**File**: `/src/server/workflow/internal/service/service.go`

In the `GetWorkflow` implementation, update the artifact URL population logic:

```go
func (s *service) GetWorkflow(ctx context.Context, req workflow.GetWorkflowRequest) (*workflow.WorkflowDetail, error) {
    // ... existing implementation ...

    // After populating chapters and artifacts, populate URLs
    for chapterIdx := range detail.Chapters {
        for artifactIdx := range detail.Chapters[chapterIdx].Artifacts {
            artifact := &detail.Chapters[chapterIdx].Artifacts[artifactIdx]

            // Build URL for this artifact
            url := fmt.Sprintf("/api/projects/%s/workflows/%s/chapters/%d/artifacts/%s",
                req.ProjectID,
                req.WorkflowID,
                chapterIdx,
                artifact.ArtifactID,
            )
            artifact.URL = &url
        }
    }

    return detail, nil
}
```

### 3. TestServer Handler Changes

**File**: `/src/server/api/internal/handlers/workflows.go`

#### Add New Handler Method

```go
// GetWorkflowArtifact handles GET /api/projects/{projectId}/workflows/{workflowId}/chapters/{chapterNumber}/artifacts/{artifactId}
func (h *Handler) GetWorkflowArtifact(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Extract path parameters
    projectID := chi.URLParam(r, "projectId")
    workflowID := chi.URLParam(r, "workflowId")
    chapterNumberStr := chi.URLParam(r, "chapterNumber")
    artifactID := chi.URLParam(r, "artifactId")

    // Parse chapter number
    chapterNumber, err := strconv.Atoi(chapterNumberStr)
    if err != nil {
        writeError(w, http.StatusBadRequest, "invalid chapter number")
        return
    }

    // Call workflow service
    artifactData, err := h.workflowSvc.GetWorkflowArtifact(ctx, workflow.GetWorkflowArtifactRequest{
        ProjectID:     projectID,
        WorkflowID:    workflowID,
        ChapterNumber: chapterNumber,
        ArtifactID:    artifactID,
    })
    if err != nil {
        // Check for specific error types
        if strings.Contains(err.Error(), "not found") {
            writeError(w, http.StatusNotFound, err.Error())
            return
        }
        writeError(w, http.StatusInternalServerError, "failed to retrieve artifact")
        h.logger.Error("failed to get workflow artifact",
            "projectId", projectID,
            "workflowId", workflowID,
            "chapterNumber", chapterNumber,
            "artifactId", artifactID,
            "error", err)
        return
    }

    // Set response headers
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifactData.Filename))
    w.Header().Set("Content-Length", fmt.Sprintf("%d", artifactData.SizeBytes))

    // Write artifact content
    w.WriteHeader(http.StatusOK)
    if _, err := w.Write(artifactData.Content); err != nil {
        h.logger.Error("failed to write artifact response", "error", err)
    }
}
```

**File**: `/src/server/api/internal/handlers/api.go`

#### Register Route

```go
func (h *Handler) Routes() chi.Router {
    r := chi.NewRouter()

    // ... existing routes ...

    // Workflow routes
    r.Route("/api/projects/{projectId}/workflows", func(r chi.Router) {
        r.Get("/", h.ListWorkflows)
        r.Get("/{workflowId}", h.GetWorkflow)

        // NEW: Artifact retrieval
        r.Get("/{workflowId}/chapters/{chapterNumber}/artifacts/{artifactId}", h.GetWorkflowArtifact)
    })

    return r
}
```

### 4. Testing Strategy

#### Unit Tests

**File**: `/src/server/workflow/internal/service/service_test.go`

```go
func TestService_GetWorkflowArtifact(t *testing.T) {
    tests := []struct {
        name         string
        req          workflow.GetWorkflowArtifactRequest
        mockWorkflow *workflow.WorkflowDetail
        mockContent  []byte
        mockError    error
        wantErr      bool
        errContains  string
    }{
        {
            name: "successful artifact retrieval",
            req: workflow.GetWorkflowArtifactRequest{
                ProjectID:     "proj-123",
                WorkflowID:    "wf-456",
                ChapterNumber: 0,
                ArtifactID:    "artifact-789",
            },
            mockWorkflow: &workflow.WorkflowDetail{
                WorkflowID: "wf-456",
                RunID:      "run-abc",
                Chapters: []model.ChapterDetail{
                    {
                        ChapterNumber: 0,
                        Artifacts: []model.ArtifactReference{
                            {
                                ArtifactID:   "artifact-789",
                                ArtifactType: "log",
                                Name:         "output.log",
                            },
                        },
                    },
                },
            },
            mockContent: []byte("artifact content"),
            wantErr:     false,
        },
        {
            name: "artifact not found in chapter",
            req: workflow.GetWorkflowArtifactRequest{
                ProjectID:     "proj-123",
                WorkflowID:    "wf-456",
                ChapterNumber: 0,
                ArtifactID:    "nonexistent",
            },
            mockWorkflow: &workflow.WorkflowDetail{
                WorkflowID: "wf-456",
                Chapters: []model.ChapterDetail{
                    {
                        ChapterNumber: 0,
                        Artifacts:     []model.ArtifactReference{},
                    },
                },
            },
            wantErr:     true,
            errContains: "not found",
        },
        {
            name: "invalid chapter number",
            req: workflow.GetWorkflowArtifactRequest{
                ProjectID:     "proj-123",
                WorkflowID:    "wf-456",
                ChapterNumber: -1,
                ArtifactID:    "artifact-789",
            },
            wantErr:     true,
            errContains: "invalid chapter number",
        },
        {
            name: "chapter out of range",
            req: workflow.GetWorkflowArtifactRequest{
                ProjectID:     "proj-123",
                WorkflowID:    "wf-456",
                ChapterNumber: 10,
                ArtifactID:    "artifact-789",
            },
            mockWorkflow: &workflow.WorkflowDetail{
                WorkflowID: "wf-456",
                Chapters:   []model.ChapterDetail{{ChapterNumber: 0}},
            },
            wantErr:     true,
            errContains: "not found",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mocks
            // ... mock setup code ...

            // Execute
            result, err := svc.GetWorkflowArtifact(context.Background(), tt.req)

            // Assert
            if tt.wantErr {
                require.Error(t, err)
                if tt.errContains != "" {
                    assert.Contains(t, err.Error(), tt.errContains)
                }
                assert.Nil(t, result)
            } else {
                require.NoError(t, err)
                require.NotNil(t, result)
                assert.Equal(t, tt.mockContent, result.Content)
            }
        })
    }
}
```

#### Integration Tests

**File**: `/src/server/api/internal/handlers/workflows_test.go`

```go
func TestHandler_GetWorkflowArtifact(t *testing.T) {
    // Use OpenAPI-generated types for testing

    tests := []struct {
        name             string
        projectID        string
        workflowID       string
        chapterNumber    string
        artifactID       string
        mockArtifactData *workflow.ArtifactData
        mockError        error
        expectedStatus   int
        expectedContent  []byte
    }{
        {
            name:          "successful artifact download",
            projectID:     "proj-123",
            workflowID:    "wf-456",
            chapterNumber: "0",
            artifactID:    "artifact-789",
            mockArtifactData: &workflow.ArtifactData{
                Content:   []byte("test artifact content"),
                Filename:  "output.log",
                SizeBytes: 21,
            },
            expectedStatus:  http.StatusOK,
            expectedContent: []byte("test artifact content"),
        },
        {
            name:           "artifact not found",
            projectID:      "proj-123",
            workflowID:     "wf-456",
            chapterNumber:  "0",
            artifactID:     "nonexistent",
            mockError:      fmt.Errorf("artifact not found"),
            expectedStatus: http.StatusNotFound,
        },
        {
            name:           "invalid chapter number",
            projectID:      "proj-123",
            workflowID:     "wf-456",
            chapterNumber:  "invalid",
            artifactID:     "artifact-789",
            expectedStatus: http.StatusBadRequest,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            mockWorkflowSvc := &mockWorkflowService{}
            mockWorkflowSvc.On("GetWorkflowArtifact", mock.Anything, mock.Anything).
                Return(tt.mockArtifactData, tt.mockError)

            handler := &Handler{
                workflowSvc: mockWorkflowSvc,
                logger:      slog.Default(),
            }

            // Create request
            url := fmt.Sprintf("/api/projects/%s/workflows/%s/chapters/%s/artifacts/%s",
                tt.projectID, tt.workflowID, tt.chapterNumber, tt.artifactID)
            req := httptest.NewRequest(http.MethodGet, url, nil)

            // Setup chi context with URL params
            rctx := chi.NewRouteContext()
            rctx.URLParams.Add("projectId", tt.projectID)
            rctx.URLParams.Add("workflowId", tt.workflowID)
            rctx.URLParams.Add("chapterNumber", tt.chapterNumber)
            rctx.URLParams.Add("artifactId", tt.artifactID)
            req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

            // Execute
            w := httptest.NewRecorder()
            handler.GetWorkflowArtifact(w, req)

            // Assert
            assert.Equal(t, tt.expectedStatus, w.Code)

            if tt.expectedStatus == http.StatusOK {
                assert.Equal(t, tt.expectedContent, w.Body.Bytes())
                assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
                assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
            }
        })
    }
}
```

#### End-to-End Tests

**File**: `/src/server/api/cmd/testserver/e2e_test.go` (new file)

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func TestArtifactRetrievalE2E(t *testing.T) {
    // Assumes testserver is running or start it programmatically
    baseURL := "http://localhost:8080"
    projectID := "test-project"

    // 1. Create a workflow that produces artifacts
    // ... setup workflow execution ...

    // 2. Get workflow details using OpenAPI types
    resp, err := http.Get(fmt.Sprintf("%s/api/projects/%s/workflows/%s",
        baseURL, projectID, workflowID))
    require.NoError(t, err)
    defer resp.Body.Close()

    var workflowDetail openapi.WorkflowDetail
    err = json.NewDecoder(resp.Body).Decode(&workflowDetail)
    require.NoError(t, err)

    // 3. Verify artifact URLs are populated
    require.NotEmpty(t, workflowDetail.Chapters)
    require.NotEmpty(t, workflowDetail.Chapters[0].Artifacts)

    artifact := workflowDetail.Chapters[0].Artifacts[0]
    require.NotNil(t, artifact.Url, "artifact URL should be populated")

    // 4. Download artifact using the URL
    downloadURL := fmt.Sprintf("%s%s", baseURL, *artifact.Url)
    resp, err = http.Get(downloadURL)
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusOK, resp.StatusCode)
    assert.Equal(t, "application/octet-stream", resp.Header.Get("Content-Type"))
    assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")

    // 5. Verify content can be read
    content, err := io.ReadAll(resp.Body)
    require.NoError(t, err)
    assert.NotEmpty(t, content)
}

func TestArtifactNotFound(t *testing.T) {
    baseURL := "http://localhost:8080"

    // Try to get non-existent artifact
    url := fmt.Sprintf("%s/api/projects/proj-123/workflows/wf-456/chapters/0/artifacts/nonexistent",
        baseURL)
    resp, err := http.Get(url)
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, http.StatusNotFound, resp.StatusCode)

    var errorResp openapi.ErrorResponse
    err = json.NewDecoder(resp.Body).Decode(&errorResp)
    require.NoError(t, err)
    assert.NotEmpty(t, errorResp.Error)
}
```

### 5. Code Generation

After updating the OpenAPI spec, regenerate the bindings:

```bash
cd /src/server/openapi
oapi-codegen -config codegen.yml /src/api/openapi/colony2-api.yaml > pkg/openapi/generated.go
```

Or using moon:

```bash
cd /src
moon run openapi:generate
```

## Implementation Order

1. **Update OpenAPI spec** (`/src/api/openapi/colony2-api.yaml`)
   - Add GET endpoint for artifact retrieval
   - Ensure ArtifactReference schema is complete

2. **Regenerate OpenAPI bindings** (`/src/server/openapi`)
   - Run code generation
   - Verify new types are generated

3. **Extend workflow service interface** (`/src/server/workflow/pkg/workflow/workflow.go`)
   - Add `GetWorkflowArtifact` method
   - Add request/response types

4. **Implement workflow service** (`/src/server/workflow/internal/service/service.go`)
   - Implement `GetWorkflowArtifact` using existing SWF/Strata APIs
   - Update `GetWorkflow` to populate artifact URLs
   - Add unit tests

5. **Add testserver handler** (`/src/server/api/internal/handlers/workflows.go`)
   - Implement `GetWorkflowArtifact` handler
   - Register route in `api.go`
   - Add handler tests

6. **End-to-end testing** (`/src/server/api/cmd/testserver`)
   - Add e2e tests using OpenAPI-generated types
   - Test successful retrieval
   - Test error cases (404, invalid params)

7. **Manual testing**
   - Start testserver
   - Create/run a workflow that produces artifacts
   - Verify artifact URLs are populated in workflow details
   - Verify artifacts can be downloaded
   - Test in UI (should work without changes)

## Testing Checklist

- [ ] OpenAPI spec validates (`openapi-generator validate` or similar)
- [ ] Code generation succeeds without errors
- [ ] Unit tests for `GetWorkflowArtifact` service method
- [ ] Unit tests for HTTP handler
- [ ] Integration tests using OpenAPI types
- [ ] E2E tests for successful artifact retrieval
- [ ] E2E tests for error cases (404, invalid chapter, etc.)
- [ ] Manual test: Create workflow → verify URLs populated → download artifact
- [ ] UI test: Verify existing artifact download functionality works

## Success Criteria

1. **OpenAPI spec includes new endpoint** with proper request/response definitions
2. **Workflow service supports artifact retrieval** with proper validation
3. **Testserver handler works** and returns artifacts as application/octet-stream
4. **Artifact URLs are populated** in workflow detail responses
5. **All tests pass** including unit, integration, and e2e
6. **UI can download artifacts** without any code changes (using populated URLs)
7. **Error handling is robust** (proper 404s, 400s for invalid input, etc.)

## Non-Goals

- UI changes (already supports artifact download via URL)
- Artifact streaming for large files (can be added later if needed)
- Artifact caching (can be added later)
- Artifact compression (can be added later)
- Access control beyond existing project-level permissions

## Future Enhancements

- Streaming support for large artifacts
- Signed/expiring URLs for direct storage access
- Artifact metadata API (list all artifacts across workflows)
- Artifact search/filtering
- Thumbnail generation for image artifacts
