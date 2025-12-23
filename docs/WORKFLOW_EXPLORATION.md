# Workflow Exploration UI - Implementation Guide

Enable users to explore workflow (recipe job) executions through the colony2 web UI using **swf-go's tenant field** for project scoping.

---

## Quick Links

- **[Implementation Specifications](specs/README.md)** - Start here for step-by-step implementation guide
- **Individual Specs**:
  - [00 - Workflow OpenAPI Spec](specs/00-workflow-openapi-spec.md) - `api/openapi`
  - [01 - Workflow OpenAPI Bindings](specs/01-workflow-openapi-bindings.md) - `server/openapi`
  - [02 - Workflow Service](specs/02-workflow-service.md) - `server/workflow` (new)
  - [03 - API Handlers](specs/03-workflow-api-handlers.md) - `server/api`
  - [04 - UI Components](specs/04-workflow-ui-components.md) - `web/app`

---

## Feature Overview

### Workflow List View
- Browse all workflows for a project (filtered by tenant field)
- Filter by status, associated ticket, cell, time range
- Pagination support

### Workflow Detail View
- Comprehensive workflow information
- Associated ticket context
- Chapter-by-chapter execution breakdown
- Operation inputs/outputs (formatted JSON)
- Artifact links

---

## Architecture

```
┌─────────────────┐
│  React UI       │
└────────┬────────┘
         │ HTTP
         ↓
┌─────────────────┐
│  API Handlers   │
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  Workflow       │  - ListWorkflows(projectID)
│  Service        │    → Filter by Tenant = projectID
│                 │  - GetWorkflow(workflowID, projectID)
│                 │    → Validate job.Tenant == projectID
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  PGWF Database  │  - Tenant field = projectID
│  (swf-go)       │  - JobID = KSUID (no prefix)
└─────────────────┘
```

---

## Key Technical Approach: Tenant Field

### Why Tenant Field?

✅ **Standard pattern** - swf-go's built-in multi-tenancy feature
✅ **Simpler** - no custom JobID parsing/validation
✅ **Direct filtering** - query PGWF by tenant
✅ **Clean validation** - compare tenant field

### Implementation

**Setting Tenant** (ticket service):
```go
startJobReq := workflowctl.StartJob{
    Tenant:     string(projectID),
    RecipeName: recipeName,
    // ...
}
```

**Filtering by Tenant** (workflow service):
```go
jobs, err := engine.ListJobs(ctx, swf.ListJobsRequest{
    Tenant: string(projectID),
    // ...
})
```

**Validating Tenant** (workflow service):
```go
job, err := engine.DescribeJob(ctx, workflowID)
if job.Tenant != string(projectID) {
    return ErrWorkflowNotInProject
}
```

---

## Implementation Timeline

| Phase | Specs | Duration |
|-------|-------|----------|
| 1 | 00-01 | 1.5-2.5h | API contract
| 2 | 02-03 | 7-10h | Backend service + handlers
| 3 | 04 | 6-8h | React UI
| 4 | - | 4-6h | Integration testing

**Total**: ~19-26 hours (2-3 days)

---

## Getting Started

### Prerequisites
- Familiarity with colony2 architecture
- Access to PGWF database
- swf-go supports Tenant field

### Implementation Steps

1. **Read specs**: [specs/README.md](specs/README.md)
2. **Implement in order**: 00 → 04
3. **Test after each spec**
4. **Integration testing**

---

## Success Criteria

✅ Users can view all workflows for a project
✅ Filtering works (status, ticket, cell, time range)
✅ Workflows scoped by tenant (project isolation verified)
✅ Detail view shows all chapters
✅ Chapter inputs/outputs readable
✅ All tests pass
✅ Performance acceptable with 1000+ workflows

---

For detailed implementation, proceed to [**specs/README.md**](specs/README.md).
