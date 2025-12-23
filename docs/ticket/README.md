# Ticket Detail View - Implementation Specifications

Enable users to click on tickets in any view (Kanban, table) and review ticket details along with associated events (workflow started, ticket completed, etc.).

---

## Overview

This feature adds a ticket detail modal that displays:
- **Ticket information**: ID, cell, stage, state, description, creator, timestamps
- **Event timeline**: Workflow executions, ticket updates, document attachments, code changesets, and history resets

The modal is accessible from both the Kanban board and the table view in the App cell.

---

## Implementation Order

Each spec corresponds to a single moon cell and must be implemented independently.

### [01 - Ticket Detail OpenAPI Specification](01-ticket-detail-openapi-spec.md)
**Cell**: `api/openapi`
**Dependencies**: None

Extend OpenAPI spec with ticket events endpoint and event schemas.

**Adds**:
- Event schemas: `TicketEvent`, `TicketEventKind`, payload types
- Endpoint: `GET /api/projects/{projectId}/tickets/{ticketId}/events`

---

### [02 - Ticket Detail OpenAPI Bindings](02-ticket-detail-openapi-bindings.md)
**Cell**: `server/openapi`
**Dependencies**: 01

Generate Go type bindings from updated OpenAPI specification.

---

### [03 - Ticket Detail Service](03-ticket-detail-service.md)
**Cell**: `server/ticket`
**Dependencies**: 01, 02

Documents usage of existing service methods for ticket detail feature. No code changes required.

---

### [04 - Ticket Detail API Handlers](04-ticket-detail-api-handlers.md)
**Cell**: `server/api`
**Dependencies**: 01, 02, 03

Implement HTTP handler for ticket events endpoint.

---

### [05 - Ticket Detail Web Bindings](05-ticket-detail-web-bindings.md)
**Cell**: `web/openapi`
**Dependencies**: 01, 04

Regenerate TypeScript client bindings from updated OpenAPI specification.

---

### [06 - Ticket Detail UI Shared Component](06-ticket-detail-ui-shared.md)
**Cell**: `web/shared`
**Dependencies**: 05

Create reusable `TicketDetailModal` component for displaying ticket details and events.

---

### [07 - Ticket Detail Kanban Integration](07-ticket-detail-kanban.md)
**Cell**: `web/kanban`
**Dependencies**: 06

Make Kanban ticket cards clickable and show detail modal.

---

### [08 - Ticket Detail App Integration](08-ticket-detail-app.md)
**Cell**: `web/app`
**Dependencies**: 06

Make ticket table rows clickable and show detail modal.

---

## Timeline

| Spec | Cell | Duration | Cumulative |
|------|------|----------|------------|
| 01   | api/openapi | 1h | 1h |
| 02   | server/openapi | 0.5h | 1.5h |
| 03   | server/ticket | 0.5h | 2h |
| 04   | server/api | 3-4h | 5-6h |
| 05   | web/openapi | 0.5h | 5.5-6.5h |
| 06   | web/shared | 4-5h | 9.5-11.5h |
| 07   | web/kanban | 2-3h | 11.5-14.5h |
| 08   | web/app | 2-3h | 13.5-17.5h |

**Total**: ~14-18 hours (2 days)

---

## Quick Start

```bash
# 01: OpenAPI Spec
cd /src/api/openapi
# Edit colony2-api.yaml to add event schemas and endpoint
yamllint colony2-api.yaml

# 02: Generate Go Bindings
cd /src/server/openapi
go generate ./...
go build ./...

# 03: Review Service Documentation
# No code changes - just understand existing ListEvents method

# 04: API Handlers
cd /src/server/api
# Implement ListTicketEvents handler
go test ./internal/handlers/...

# 05: Generate TypeScript Bindings
cd /src/web/openapi
pnpm run generate
pnpm run build

# 06: Shared UI Component
cd /src/web/shared
# Create TicketDetailModal component
pnpm add dayjs
pnpm test

# 07: Kanban Integration
cd /src/web/kanban
# Make cards clickable
pnpm test

# 08: App Integration
cd /src/web/app
# Make table rows clickable
pnpm test
```

---

## Key Features

### Shared TicketDetailModal Component

The `TicketDetailModal` component (spec 06) is shared between Kanban and App cells, providing:
- **Details Tab**: Ticket ID, cell, stage, state, description, creator, timestamps
- **Events Tab**: Timeline of all events (newest first)
- Color-coded events by type
- Loading and error states
- Keyboard accessible

### Event Types

The system tracks 5 event types:
1. **Workflow** - Job started, completed, or failed
2. **Ticket** - Ticket field updates
3. **Markdown Doc** - Documentation attached, overridden, or removed
4. **ChangeSet** - Code changes attached
5. **Reset** - Event history reset

---

## Testing Strategy

### Unit Tests
- API handler conversion logic (spec 04)
- TicketDetailModal component rendering (spec 06)
- Kanban board modal integration (spec 07)
- App table modal integration (spec 08)

### Integration Tests
- Events endpoint returns correct data (spec 04)
- Query parameter filtering works (spec 04)
- Modal fetches and displays events (spec 06)

### End-to-End Tests
- Click ticket in Kanban → modal opens with details and events
- Click ticket in table → modal opens with details and events
- Events display correctly for all event types
- Modal closes properly
- Keyboard navigation works

---

## Implementation Checklist

- [ ] **01-ticket-detail-openapi-spec.md** - Event schemas and endpoint
- [ ] **02-ticket-detail-openapi-bindings.md** - Go bindings
- [ ] **03-ticket-detail-service.md** - Service documentation
- [ ] **04-ticket-detail-api-handlers.md** - Events handler
- [ ] **05-ticket-detail-web-bindings.md** - TypeScript bindings
- [ ] **06-ticket-detail-ui-shared.md** - TicketDetailModal component
- [ ] **07-ticket-detail-kanban.md** - Kanban integration
- [ ] **08-ticket-detail-app.md** - App table integration

**Integration Testing**:
- [ ] Click ticket in Kanban → modal opens with details and events
- [ ] Click ticket in table → modal opens with details and events
- [ ] Events display correctly for all event types
- [ ] Modal closes properly
- [ ] Keyboard navigation works

---

## Architecture

```
┌─────────────────┐
│  Kanban View    │──┐
│  (web/kanban)   │  │
└─────────────────┘  │
                     ├──> TicketDetailModal
┌─────────────────┐  │    (web/shared)
│  Table View     │──┘         │
│  (web/app)      │            │ API Call
└─────────────────┘            ↓
                     GET /api/.../events
                               │
                               ↓
                    ┌──────────────────┐
                    │  API Handler     │
                    │  (server/api)    │
                    └────────┬─────────┘
                             │
                             ↓
                    ┌──────────────────┐
                    │  Ticket Service  │
                    │  (server/ticket) │
                    └────────┬─────────┘
                             │
                             ↓
                    ┌──────────────────┐
                    │  Database        │
                    │  (ticket_events) │
                    └──────────────────┘
```

---

## Support

For questions or issues:
1. Review individual spec files for detailed implementation guidance
2. Check existing codebase for similar patterns
3. Consult main [specs documentation](../specs/README.md)

---

## Success Criteria

Implementation is complete when:

✅ Users can click tickets in Kanban view to see details
✅ Users can click tickets in table view to see details
✅ Modal displays all ticket information
✅ Events tab shows timeline of all events
✅ Events are color-coded by type
✅ Workflow events show workflow ID and status
✅ Modal handles loading and error states
✅ Keyboard navigation works
✅ All tests pass (unit + integration)
✅ Performance is acceptable with 100+ events
