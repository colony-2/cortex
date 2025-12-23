# Colony2 UI - Implementation Specifications

This directory contains numbered specifications for implementing features in the colony2 UI. Each spec focuses on a single moon project/cell and must be edited independently.

---

## Overview

Enable users to browse, filter, and inspect workflow executions through the colony2 web UI. Workflows are scoped to projects using **swf-go's tenant field** (tenant = projectID).

**Key Technical Approach**: Use swf-go's built-in `Tenant` field for multi-tenancy instead of custom JobID prefixing.

---

## Implementation Order

### [00 - Workflow OpenAPI Specification](00-workflow-openapi-spec.md)
**Project**: `api/openapi`
**Dependencies**: None
**Duration**: ~1-2 hours

Extend OpenAPI spec with workflow schemas and endpoints.

**Adds**:
- Schemas: `WorkflowSummary`, `WorkflowDetail`, `ChapterDetail`, `ArtifactReference`
- Enums: `WorkflowStatus`, `ChapterStatus`
- Endpoints: `GET /api/projects/{projectId}/workflows`, `GET /api/projects/{projectId}/workflows/{workflowId}`

---

### [01 - Workflow OpenAPI Bindings](01-workflow-openapi-bindings.md)
**Project**: `server/openapi`
**Dependencies**: 00
**Duration**: ~30 minutes

Generate Go type bindings from OpenAPI spec using oapi-codegen.

---

### [02 - Workflow Service Implementation](02-workflow-service.md)
**Project**: `server/workflow` (new package)
**Dependencies**: 00, 01
**Duration**: ~4-6 hours

Create workflow service layer to query PGWF using tenant field.

**Key Implementation**:
- `ListWorkflows` - Filter by `Tenant = projectID`
- `GetWorkflow` - Validate `job.Tenant == projectID`
- Update ticket service to set Tenant when starting jobs

---

### [03 - Workflow API Handlers](03-workflow-api-handlers.md)
**Project**: `server/api`
**Dependencies**: 01, 02
**Duration**: ~3-4 hours

Implement HTTP handlers for workflow endpoints.

**Implementation**: Handlers delegate to workflow service which handles tenant-based filtering/validation.

---

### [04 - Workflow UI Components](04-workflow-ui-components.md)
**Project**: `web/app`
**Dependencies**: 03
**Duration**: ~6-8 hours

Build React components for workflow exploration.

---

## Quick Start

```bash
# Spec 00: OpenAPI Spec
cd api/openapi
# Edit openapi.yaml...

# Spec 01: Generate Bindings
cd ../../server/openapi
go generate ./...

# Spec 02: Workflow Service
cd ../workflow
# Create package, implement with tenant filtering...
go test ./...

# Spec 03: API Handlers
cd ../api
# Implement handlers...
go test ./internal/handlers/...

# Spec 04: UI Components
cd ../../web/app
pnpm run generate-client
pnpm test
```

---

## Estimated Timeline

| Spec | Duration | Cumulative |
|------|----------|------------|
| 00   | 1-2h     | 1-2h       |
| 01   | 0.5h     | 1.5-2.5h   |
| 02   | 4-6h     | 5.5-8.5h   |
| 03   | 3-4h     | 8.5-12.5h  |
| 04   | 6-8h     | 14.5-20.5h |

**Total**: ~15-21 hours (2-3 days)

**Buffer**: Add 25% → ~19-26 hours

---

## Key Design: Tenant Field

### Why Tenant Field?

✅ **Standard multi-tenant pattern** - swf-go's built-in feature
✅ **Simpler implementation** - no custom JobID parsing
✅ **Direct filtering** - query PGWF by tenant
✅ **Clean validation** - compare tenant field directly

### Tenant Field Usage

**Setting Tenant** (in ticket service):
```go
startJobReq := workflowctl.StartJob{
    Tenant:     string(projectID), // Set tenant = projectID
    RecipeName: recipeName,
    // ...
}
```

**Filtering by Tenant** (in workflow service):
```go
jobs, err := engine.ListJobs(ctx, swf.ListJobsRequest{
    Tenant: string(projectID), // Filter by tenant
    // ...
})
```

**Validating Tenant** (in workflow service):
```go
job, err := engine.DescribeJob(ctx, workflowID)
if job.Tenant != string(projectID) {
    return ErrWorkflowNotInProject
}
```

---

## Checklist - Workflow Exploration

- [ ] **00-workflow-openapi-spec.md** - OpenAPI schema extensions
- [ ] **01-workflow-openapi-bindings.md** - Go bindings generation
- [ ] **02-workflow-service.md** - Workflow service with tenant filtering
- [ ] **03-workflow-api-handlers.md** - HTTP handlers
- [ ] **04-workflow-ui-components.md** - React components

**Integration Testing**:
- [ ] End-to-end: ticket creation → workflow UI
- [ ] Verify tenant isolation (workflows from project A not visible in project B)
- [ ] Performance test: 1000+ workflows

---

# Ticket Detail View - Implementation Specifications

Enable users to click on tickets in any view (Kanban, table) and review ticket details along with associated events.

**Location**: [../ticket/](../ticket/) folder contains 8 specifications (numbered 01-08)

**Quick Links**:
- [Ticket Specifications README](../ticket/README.md)
- [01 - OpenAPI Spec](../ticket/01-ticket-detail-openapi-spec.md)
- [02 - OpenAPI Bindings](../ticket/02-ticket-detail-openapi-bindings.md)
- [03 - Ticket Service](../ticket/03-ticket-detail-service.md)
- [04 - API Handlers](../ticket/04-ticket-detail-api-handlers.md)
- [05 - Web Bindings](../ticket/05-ticket-detail-web-bindings.md)
- [06 - UI Shared Component](../ticket/06-ticket-detail-ui-shared.md)
- [07 - Kanban Integration](../ticket/07-ticket-detail-kanban.md)
- [08 - App Integration](../ticket/08-ticket-detail-app.md)

**Estimated Duration**: ~14-18 hours (2 days)

---

## Deployment

- [ ] Staging deployment
- [ ] Production deployment
