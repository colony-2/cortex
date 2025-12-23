# 01 - Ticket Detail OpenAPI Specification

**Cell**: `api/openapi`
**Dependencies**: None
**Purpose**: Extend OpenAPI specification with ticket events endpoint and event schemas

---

## Overview

Add schemas and endpoints to the OpenAPI specification to support retrieving detailed ticket information including associated events (workflow started, ticket completed, etc.). This spec defines the contract that backend will implement and frontend will consume for ticket detail viewing.

---

## Changes Required

### 1. Add Event Schemas

Add the following schemas to `components/schemas` section in `/src/api/openapi/colony2-api.yaml`:

####Ticket EventKind Enum

```yaml
TicketEventKind:
  type: string
  enum: [ticket, workflow, markdown_doc, changeset, reset]
  description: Type of event associated with a ticket
```

#### WorkflowEventType Enum

```yaml
WorkflowEventType:
  type: string
  enum: [running, completed, failed]
  description: Workflow execution status
```

#### MarkdownEventType Enum

```yaml
MarkdownEventType:
  type: string
  enum: [attached, overridden, removed]
  description: Markdown document operation type
```

#### ChangeSetEventType Enum

```yaml
ChangeSetEventType:
  type: string
  enum: [attached, overridden, removed]
  description: ChangeSet operation type
```

#### WorkflowEventPayload Schema

```yaml
WorkflowEventPayload:
  type: object
  properties:
    type:
      $ref: '#/components/schemas/WorkflowEventType'
    workflowId:
      type: string
      description: Workflow/job identifier
    runId:
      type: string
      description: Workflow run identifier
  required: [type, workflowId, runId]
```

#### MarkdownEventPayload Schema

```yaml
MarkdownEventPayload:
  type: object
  properties:
    type:
      $ref: '#/components/schemas/MarkdownEventType'
    name:
      type: string
      description: Document name
    path:
      type: string
      description: File path to document
  required: [type, name, path]
```

#### ChangeSetEventPayload Schema

```yaml
ChangeSetEventPayload:
  type: object
  properties:
    type:
      $ref: '#/components/schemas/ChangeSetEventType'
    path:
      type: string
      description: Repository path
    commit_message:
      type: string
      description: Git commit message
    base_hash:
      type: string
      description: Base commit hash
    parent_hash:
      type: string
      description: Parent commit hash
    tip_hash:
      type: string
      description: Tip commit hash
  required: [type, path]
```

#### TicketEventPayload Schema

```yaml
TicketEventPayload:
  type: object
  properties:
    stage:
      type: string
      description: Updated stage
    state:
      $ref: '#/components/schemas/TicketState'
    description:
      type: string
      description: Updated description
    completedAt:
      type: string
      format: date-time
      nullable: true
    notes:
      type: string
      description: Optional notes about the change
```

#### ResetEventPayload Schema

```yaml
ResetEventPayload:
  type: object
  properties:
    resetId:
      type: string
      description: Unique reset identifier
    anchorEventId:
      type: string
      nullable: true
      description: Event ID marking the boundary (events after this are invalidated)
    reason:
      type: string
      description: Reason for reset
  required: [resetId, reason]
```

#### TicketEvent Schema

```yaml
TicketEvent:
  type: object
  properties:
    id:
      type: string
      description: Unique event identifier
    ticketId:
      type: string
      description: Associated ticket ID
    projectId:
      type: string
      description: Project ID
    kind:
      $ref: '#/components/schemas/TicketEventKind'
    timestamp:
      type: string
      format: date-time
      description: When the event occurred
    actor:
      $ref: '#/components/schemas/Actor'
      description: Who triggered the event
    resetId:
      type: string
      nullable: true
      description: Associated reset ID if event has been invalidated
    workflowPayload:
      $ref: '#/components/schemas/WorkflowEventPayload'
    markdownPayload:
      $ref: '#/components/schemas/MarkdownEventPayload'
    changesetPayload:
      $ref: '#/components/schemas/ChangeSetEventPayload'
    ticketPayload:
      $ref: '#/components/schemas/TicketEventPayload'
    resetPayload:
      $ref: '#/components/schemas/ResetEventPayload'
  required: [id, ticketId, projectId, kind, timestamp, actor]
  description: "Event associated with a ticket. Check `kind` field and use corresponding payload field (e.g., kind=workflow → use workflowPayload)"
```

### 2. Add Events Endpoint

Add the following endpoint to the `paths` section in `/src/api/openapi/colony2-api.yaml`:

```yaml
/api/projects/{projectId}/tickets/{ticketId}/events:
  parameters:
    - name: projectId
      in: path
      required: true
      description: Project identifier
      schema:
        type: string
    - name: ticketId
      in: path
      required: true
      description: Ticket identifier
      schema:
        type: string
  get:
    tags: [Tickets]
    summary: List ticket events
    description: Returns all events associated with a ticket, including workflow executions, ticket updates, document attachments, and changesets.
    parameters:
      - name: kind
        in: query
        required: false
        description: Filter by event kind
        schema:
          $ref: '#/components/schemas/TicketEventKind'
      - name: since
        in: query
        required: false
        description: Only return events after this timestamp
        schema:
          type: string
          format: date-time
      - name: until
        in: query
        required: false
        description: Only return events before this timestamp
        schema:
          type: string
          format: date-time
      - name: includeReset
        in: query
        required: false
        description: Include events that have been reset/invalidated
        schema:
          type: boolean
          default: false
    responses:
      '200':
        description: List of ticket events
        content:
          application/json:
            schema:
              type: array
              items:
                $ref: '#/components/schemas/TicketEvent'
      '404':
        description: Ticket not found
        content:
          text/plain:
            schema:
              type: string
```

---

## File Location

- **File**: `/src/api/openapi/colony2-api.yaml`

---

## Testing

After making changes:

1. **Validate YAML syntax**:
```bash
cd /src/api/openapi
yamllint colony2-api.yaml
```

2. **Verify schema references**:
```bash
# Use openapi-generator-cli or similar tool to validate
npx @openapitools/openapi-generator-cli validate -i colony2-api.yaml
```

3. **Manual verification**:
   - Check all `$ref` paths point to existing schemas
   - Verify enum values match backend constants
   - Ensure required fields are marked correctly

---

## Implementation Notes

### Event Payload Design

- Events use discriminated union pattern via `kind` field
- Only one payload field should be populated based on `kind`:
  - `kind=workflow` → populate `workflowPayload`
  - `kind=markdown_doc` → populate `markdownPayload`
  - `kind=changeset` → populate `changesetPayload`
  - `kind=ticket` → populate `ticketPayload`
  - `kind=reset` → populate `resetPayload`

### Filtering Capabilities

The events endpoint supports filtering by:
- **kind**: Only return specific event types (e.g., only workflow events)
- **since/until**: Time range filtering
- **includeReset**: Whether to include invalidated events (default: false)

### Null vs Omitted Fields

- Nullable fields use `nullable: true` in schema
- Optional fields are not in `required` array
- Events may have null `resetId` if not invalidated
- ChangeSet optional fields (`commit_message`, `base_hash`, etc.) may be omitted

---

## Next Steps

After completing this spec:
1. Proceed to **02-ticket-detail-openapi-bindings.md** to generate Go type bindings
2. The generated types will be used in backend service implementation (spec 03)

---

## Success Criteria

- [ ] All event schemas added to `components/schemas`
- [ ] Events endpoint added to `paths`
- [ ] YAML validates without errors
- [ ] All `$ref` references resolve correctly
- [ ] File committed to version control

---

## Estimated Duration

**~1 hour** - YAML editing and validation
