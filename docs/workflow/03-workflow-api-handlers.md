# 03 - Workflow API Handlers

**Project**: `server/api`

**Dependencies**: `01-workflow-openapi-bindings.md`, `02-workflow-service.md`

**Implements**: HTTP handlers for workflow endpoints (tenant-based validation)

---

## Overview

Implement HTTP handlers that expose workflow data via REST API. Handlers use workflow service which filters by tenant field.

---

## Handler Implementation

**File**: `server/api/internal/handlers/workflows.go`

### ListWorkflowsHandler

```go
func (h *Handler) ListWorkflowsHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    projectID := project.ID(mux.Vars(r)["projectId"])

    // Parse query params
    // ... (status, ticket_id, cell_id, since, until, limit, offset)

    // Call service - it filters by tenant automatically
    req := workflow.ListWorkflowsRequest{
        ProjectID: projectID, // Service uses this as tenant filter
        Statuses:  statuses,
        // ...
    }

    summaries, err := h.workflowService.ListWorkflows(ctx, req)
    if err != nil {
        h.handleWorkflowError(w, err)
        return
    }

    // Convert to OpenAPI types and return
    response := make([]openapi.WorkflowSummary, len(summaries))
    for i, s := range summaries {
        response[i] = convertToOpenAPIWorkflowSummary(s)
    }

    writeJSON(w, http.StatusOK, response)
}
```

### GetWorkflowHandler

```go
func (h *Handler) GetWorkflowHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    projectID := project.ID(mux.Vars(r)["projectId"])
    workflowID := mux.Vars(r)["workflowId"]

    // Call service - it validates tenant matches
    req := workflow.GetWorkflowRequest{
        ProjectID:  projectID, // Service validates tenant == projectID
        WorkflowID: workflowID,
    }

    detail, err := h.workflowService.GetWorkflow(ctx, req)
    if err != nil {
        h.handleWorkflowError(w, err)
        return
    }

    response := convertToOpenAPIWorkflowDetail(*detail)
    writeJSON(w, http.StatusOK, response)
}
```

### Error Handling

```go
func (h *Handler) handleWorkflowError(w http.ResponseWriter, err error) {
    switch err {
    case workflow.ErrNotFound:
        writeJSON(w, http.StatusNotFound, openapi.ErrorResponse{Message: "Workflow not found"})
    case workflow.ErrWorkflowNotInProject:
        writeJSON(w, http.StatusForbidden, openapi.ErrorResponse{Message: "Workflow does not belong to this project"})
    default:
        writeJSON(w, http.StatusInternalServerError, openapi.ErrorResponse{Message: "Internal server error"})
    }
}
```

---

## Key Points

- **No JobID validation needed** - service layer handles tenant checking
- **403 Forbidden** - returned when workflow tenant doesn't match project
- **Simpler than JobID prefix approach** - no parsing/validation logic

---

## Next Steps

Proceed to **04-workflow-ui-components.md**
