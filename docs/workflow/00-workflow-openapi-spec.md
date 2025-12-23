# 00 - Workflow OpenAPI Specification

**Project**: `api/openapi`

**Dependencies**: None

**Implements**: REST API contract for workflow listing and detail retrieval

---

## Overview

Extend the OpenAPI specification to define workflow-related schemas and endpoints. Workflows are scoped to projects using swf-go's tenant field (tenant = projectId).

**Location**: `api/openapi/openapi.yaml`

---

## New Tag

```yaml
tags:
  - name: Workflows
    description: Workflow execution listing and detail retrieval
```

---

## New Schemas

### WorkflowStatus (Enum)

```yaml
WorkflowStatus:
  type: string
  enum:
    - running
    - completed
    - failed
    - canceled
    - terminated
    - timed_out
    - unknown
  description: Normalized workflow execution status
```

### ChapterStatus (Enum)

```yaml
ChapterStatus:
  type: string
  enum:
    - pending
    - running
    - completed
    - failed
    - skipped
  description: Chapter (operation) execution status
```

### ArtifactReference

```yaml
ArtifactReference:
  type: object
  required:
    - artifact_id
    - artifact_type
    - name
    - created_at
  properties:
    artifact_id:
      type: string
    artifact_type:
      type: string
    name:
      type: string
    size_bytes:
      type: integer
      format: int64
      nullable: true
    url:
      type: string
      nullable: true
    created_at:
      type: string
      format: date-time
```

### ChapterDetail

```yaml
ChapterDetail:
  type: object
  required:
    - chapter_number
    - chapter_type
    - status
  properties:
    chapter_number:
      type: integer
    chapter_type:
      type: string
    op_name:
      type: string
      nullable: true
    status:
      $ref: '#/components/schemas/ChapterStatus'
    start_time:
      type: string
      format: date-time
      nullable: true
    end_time:
      type: string
      format: date-time
      nullable: true
    input:
      type: object
      additionalProperties: true
    output:
      type: object
      additionalProperties: true
      nullable: true
    error:
      type: string
      nullable: true
    artifacts:
      type: array
      items:
        $ref: '#/components/schemas/ArtifactReference'
```

### WorkflowSummary

```yaml
WorkflowSummary:
  type: object
  required:
    - workflow_id
    - run_id
    - status
    - recipe_name
    - created_at
  properties:
    workflow_id:
      type: string
      description: "Workflow/job ID (KSUID)"
      example: "2U9V0W1X2Y3Z4A5B6C7D8E9F0G"
    run_id:
      type: string
    status:
      $ref: '#/components/schemas/WorkflowStatus'
    recipe_name:
      type: string
    ticket_id:
      type: string
      nullable: true
    ticket_title:
      type: string
      nullable: true
    cell_id:
      type: string
      nullable: true
    cell_name:
      type: string
      nullable: true
    start_time:
      type: string
      format: date-time
      nullable: true
    close_time:
      type: string
      format: date-time
      nullable: true
    actor:
      $ref: '#/components/schemas/Actor'
    created_at:
      type: string
      format: date-time
```

### WorkflowDetail

```yaml
WorkflowDetail:
  type: object
  required:
    - workflow_id
    - run_id
    - status
    - recipe_name
    - chapters
    - created_at
  properties:
    workflow_id:
      type: string
    run_id:
      type: string
    status:
      $ref: '#/components/schemas/WorkflowStatus'
    recipe_name:
      type: string
    ticket_id:
      type: string
      nullable: true
    ticket:
      $ref: '#/components/schemas/Ticket'
      nullable: true
    cell_id:
      type: string
      nullable: true
    cell_name:
      type: string
      nullable: true
    start_time:
      type: string
      format: date-time
      nullable: true
    close_time:
      type: string
      format: date-time
      nullable: true
    actor:
      $ref: '#/components/schemas/Actor'
    git_ref:
      type: string
      nullable: true
    git_commit:
      type: string
      nullable: true
    chapters:
      type: array
      items:
        $ref: '#/components/schemas/ChapterDetail'
    raw_job_data:
      type: object
      additionalProperties: true
      nullable: true
    created_at:
      type: string
      format: date-time
```

---

## New Endpoints

### GET /api/projects/{projectId}/workflows

```yaml
paths:
  /api/projects/{projectId}/workflows:
    get:
      summary: List workflows for a project
      description: Returns workflows scoped to the project (filtered by tenant field in swf-go)
      operationId: getApiProjectsWorkflows
      tags:
        - Workflows
      parameters:
        - name: projectId
          in: path
          required: true
          schema:
            type: string
        - name: status
          in: query
          schema:
            type: array
            items:
              $ref: '#/components/schemas/WorkflowStatus'
          style: form
          explode: true
        - name: ticket_id
          in: query
          schema:
            type: string
        - name: cell_id
          in: query
          schema:
            type: string
        - name: since
          in: query
          schema:
            type: string
            format: date-time
        - name: until
          in: query
          schema:
            type: string
            format: date-time
        - name: limit
          in: query
          schema:
            type: integer
            default: 50
            maximum: 200
        - name: offset
          in: query
          schema:
            type: integer
            default: 0
      responses:
        '200':
          description: List of workflow summaries
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/WorkflowSummary'
        '404':
          description: Project not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
```

### GET /api/projects/{projectId}/workflows/{workflowId}

```yaml
  /api/projects/{projectId}/workflows/{workflowId}:
    get:
      summary: Get workflow detail
      description: Returns workflow detail. Validates workflow belongs to project via tenant field.
      operationId: getApiProjectsWorkflows1
      tags:
        - Workflows
      parameters:
        - name: projectId
          in: path
          required: true
          schema:
            type: string
        - name: workflowId
          in: path
          required: true
          schema:
            type: string
        - name: includeRawJobData
          in: query
          schema:
            type: boolean
            default: false
      responses:
        '200':
          description: Workflow detail
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/WorkflowDetail'
        '403':
          description: Workflow does not belong to this project
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
        '404':
          description: Workflow not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
```

---

## Implementation Steps

1. Add schemas to `api/openapi/openapi.yaml`
2. Add endpoints under `/api/projects/{projectId}/workflows`
3. Validate spec: `npx @redocly/cli lint api/openapi/openapi.yaml`
4. Commit changes

---

## Next Steps

Proceed to **01-workflow-openapi-bindings.md** to generate Go types.

---

## Files Modified

- `api/openapi/openapi.yaml`
