# 02 - Workflow Service Implementation

**Project**: `server/workflow` (new package)

**Dependencies**: `00-workflow-openapi-spec.md`, `01-workflow-openapi-bindings.md`

**Implements**: Business logic for querying and aggregating workflow data using swf-go tenant field

---

## Overview

Create `server/workflow` package to encapsulate workflow data retrieval from PGWF (swf-go) and correlation with tickets/cells.

**Key Concept**: Uses swf-go's `Tenant` field to scope workflows to projects. When listing/fetching workflows, always filter by `Tenant = projectID`.

---

## Service Interface

**Key Method**: `ListWorkflows` - filters by `Tenant = projectID` in swf-go

```go
func (s *workflowService) ListWorkflows(ctx context.Context, req workflow.ListWorkflowsRequest) ([]workflow.WorkflowSummary, error) {
    listReq := swf.ListJobsRequest{
        Tenant: string(req.ProjectID), // KEY: Use tenant field
        Limit:  req.Limit,
        Offset: req.Offset,
    }
    
    jobs, _, err := s.engine.ListJobs(ctx, listReq)
    // ...
}
```

**Key Method**: `GetWorkflow` - validates `job.Tenant == projectID`

```go
func (s *workflowService) GetWorkflow(ctx context.Context, req workflow.GetWorkflowRequest) (*workflow.WorkflowDetail, error) {
    job, err := s.engine.DescribeJob(ctx, swf.JobId(req.WorkflowID))
    
    // KEY: Validate tenant matches
    if job.Tenant != string(req.ProjectID) {
        return nil, workflow.ErrWorkflowNotInProject
    }
    // ...
}
```

---

## Ticket Service Changes

When starting workflows, set the Tenant field:

**File**: `server/ticket/internal/service/service.go`

```go
startJobReq := workflowctl.StartJob{
    Tenant:     string(ticket.ProjectID), // Set tenant = projectID
    RecipeName: recipeName,
    // ...
}
```

---

## Next Steps

Proceed to **03-workflow-api-handlers.md**
