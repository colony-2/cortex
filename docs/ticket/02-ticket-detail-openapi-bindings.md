# 02 - Ticket Detail OpenAPI Bindings

**Cell**: `server/openapi`
**Dependencies**: 01
**Purpose**: Generate Go type bindings from updated OpenAPI specification

---

## Overview

Generate Go types and server interfaces from the updated OpenAPI specification that includes ticket event schemas. This creates type-safe Go structs and interfaces that the backend will use to implement ticket detail endpoints.

---

## Changes Required

### 1. Regenerate Bindings

The `server/openapi` cell uses `oapi-codegen` to generate Go bindings from the OpenAPI spec. After updating the spec in **01**, regenerate the bindings.

**Command**:
```bash
cd /src/server/openapi
go generate ./...
```

This will regenerate `/src/server/openapi/pkg/openapi/generated.go` with:
- New event schemas as Go structs
- Updated server interface with events endpoint handler
- Request/response parameter types

### 2. Verify Generated Types

After generation, verify the following types exist in `pkg/openapi/generated.go`:

#### Event Enums
```go
type TicketEventKind string
const (
    TicketEventKindTicket      TicketEventKind = "ticket"
    TicketEventKindWorkflow    TicketEventKind = "workflow"
    TicketEventKindMarkdownDoc TicketEventKind = "markdown_doc"
    TicketEventKindChangeset   TicketEventKind = "changeset"
    TicketEventKindReset       TicketEventKind = "reset"
)

type WorkflowEventType string
const (
    WorkflowEventTypeRunning   WorkflowEventType = "running"
    WorkflowEventTypeCompleted WorkflowEventType = "completed"
    WorkflowEventTypeFailed    WorkflowEventType = "failed"
)

type MarkdownEventType string
const (
    MarkdownEventTypeAttached   MarkdownEventType = "attached"
    MarkdownEventTypeOverridden MarkdownEventType = "overridden"
    MarkdownEventTypeRemoved    MarkdownEventType = "removed"
)

type ChangeSetEventType string
const (
    ChangeSetEventTypeAttached   ChangeSetEventType = "attached"
    ChangeSetEventTypeOverridden ChangeSetEventType = "overridden"
    ChangeSetEventTypeRemoved    ChangeSetEventType = "removed"
)
```

#### Event Payload Structs
```go
type WorkflowEventPayload struct {
    Type       WorkflowEventType `json:"type"`
    WorkflowId string            `json:"workflowId"`
    RunId      string            `json:"runId"`
}

type MarkdownEventPayload struct {
    Type MarkdownEventType `json:"type"`
    Name string            `json:"name"`
    Path string            `json:"path"`
}

type ChangeSetEventPayload struct {
    Type          ChangeSetEventType `json:"type"`
    Path          string             `json:"path"`
    CommitMessage *string            `json:"commit_message,omitempty"`
    BaseHash      *string            `json:"base_hash,omitempty"`
    ParentHash    *string            `json:"parent_hash,omitempty"`
    TipHash       *string            `json:"tip_hash,omitempty"`
}

type TicketEventPayload struct {
    Stage       *string      `json:"stage,omitempty"`
    State       *TicketState `json:"state,omitempty"`
    Description *string      `json:"description,omitempty"`
    CompletedAt *time.Time   `json:"completedAt,omitempty"`
    Notes       *string      `json:"notes,omitempty"`
}

type ResetEventPayload struct {
    ResetId        string  `json:"resetId"`
    AnchorEventId  *string `json:"anchorEventId,omitempty"`
    Reason         string  `json:"reason"`
}
```

#### Main Event Struct
```go
type TicketEvent struct {
    Id               string                  `json:"id"`
    TicketId         string                  `json:"ticketId"`
    ProjectId        string                  `json:"projectId"`
    Kind             TicketEventKind         `json:"kind"`
    Timestamp        time.Time               `json:"timestamp"`
    Actor            Actor                   `json:"actor"`
    ResetId          *string                 `json:"resetId,omitempty"`
    WorkflowPayload  *WorkflowEventPayload   `json:"workflowPayload,omitempty"`
    MarkdownPayload  *MarkdownEventPayload   `json:"markdownPayload,omitempty"`
    ChangesetPayload *ChangeSetEventPayload  `json:"changesetPayload,omitempty"`
    TicketPayload    *TicketEventPayload     `json:"ticketPayload,omitempty"`
    ResetPayload     *ResetEventPayload      `json:"resetPayload,omitempty"`
}
```

#### Query Parameters
```go
type ListTicketEventsParams struct {
    Kind         *TicketEventKind `form:"kind,omitempty" json:"kind,omitempty"`
    Since        *time.Time       `form:"since,omitempty" json:"since,omitempty"`
    Until        *time.Time       `form:"until,omitempty" json:"until,omitempty"`
    IncludeReset *bool            `form:"includeReset,omitempty" json:"includeReset,omitempty"`
}
```

#### Server Interface Method
```go
type ServerInterface interface {
    // ... existing methods ...

    // List ticket events
    // (GET /api/projects/{projectId}/tickets/{ticketId}/events)
    ListTicketEvents(w http.ResponseWriter, r *http.Request, projectId string, ticketId string, params ListTicketEventsParams)
}
```

---

## File Locations

- **Generated file**: `/src/server/openapi/pkg/openapi/generated.go`
- **Generator config**: `/src/server/openapi/tools.go` or `generate.go` (contains `//go:generate` directive)

---

## Testing

After regenerating bindings:

1. **Compile check**:
```bash
cd /src/server/openapi
go build ./...
```

2. **Run tests**:
```bash
go test ./...
```

3. **Verify imports**:
   - Check that `time` package is imported for `time.Time` fields
   - Verify JSON tags match OpenAPI property names

---

## Implementation Notes

### oapi-codegen Configuration

The generation is typically configured via a `//go:generate` directive like:
```go
//go:generate go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen --config=config.yaml ../../api/openapi/colony2-api.yaml
```

Or directly in code:
```go
//go:generate oapi-codegen -package openapi -generate types,server,spec -o pkg/openapi/generated.go ../../api/openapi/colony2-api.yaml
```

### Generated Code Structure

The generator creates:
- **Types**: All schema definitions as Go structs
- **Server Interface**: Method signatures for HTTP handlers
- **Spec**: Embedded OpenAPI spec as Go variable

### Pointer vs Value Types

- Required fields → value types (`string`, `time.Time`)
- Optional fields → pointer types (`*string`, `*time.Time`)
- Nullable fields → pointer types with `omitempty` JSON tag

---

## Troubleshooting

### Issue: Generation fails with "cannot resolve reference"
**Solution**: Ensure all `$ref` paths in spec 01 are correct. Check schema names match exactly (case-sensitive).

### Issue: Generated types don't include new schemas
**Solution**: Verify OpenAPI spec validates. Run `yamllint` and `openapi-generator-cli validate` on the spec file.

### Issue: Time fields generate as `string` instead of `time.Time`
**Solution**: Ensure OpenAPI spec uses `format: date-time` for timestamp fields.

### Issue: Enum constants missing
**Solution**: Check that enum schemas use `type: string` with `enum: [...]` array. oapi-codegen requires this format.

---

## Next Steps

After completing this spec:
1. Proceed to **03-ticket-detail-service.md** to understand existing service methods
2. The generated `ServerInterface` will be implemented in **04-ticket-detail-api-handlers.md**

---

## Success Criteria

- [ ] Bindings regenerated successfully
- [ ] `go build ./...` succeeds without errors
- [ ] All event types and structs present in generated code
- [ ] Server interface includes `ListTicketEvents` method
- [ ] Tests pass
- [ ] Changes committed to version control

---

## Estimated Duration

**~30 minutes** - Generation and verification
