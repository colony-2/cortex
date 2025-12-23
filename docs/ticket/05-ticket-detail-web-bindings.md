# 05 - Ticket Detail Web OpenAPI Bindings

**Cell**: `web/openapi`
**Dependencies**: 07, 10
**Purpose**: Regenerate TypeScript client bindings from updated OpenAPI specification

---

## Overview

Regenerate TypeScript/JavaScript API client from the updated OpenAPI specification that includes ticket events endpoint and schemas. This creates type-safe client methods that the frontend will use to fetch ticket events.

---

## Changes Required

### 1. Regenerate Client Bindings

The `web/openapi` cell uses OpenAPI code generation tools to create TypeScript client from the API spec.

**Command**:
```bash
cd /src/web/openapi
pnpm run generate
```

Or if using a different generation command:
```bash
cd /src/web/openapi
pnpm run generate-client
```

This will regenerate client code in `/src/web/openapi/src/generated/` with:
- Event type definitions
- TicketsService with new `listTicketEvents` method
- Request/response types

### 2. Verify Generated Types

After generation, verify the following types exist:

#### Event Enums

In `/src/web/openapi/src/generated/models/TicketEventKind.ts`:
```typescript
export enum TicketEventKind {
    TICKET = 'ticket',
    WORKFLOW = 'workflow',
    MARKDOWN_DOC = 'markdown_doc',
    CHANGESET = 'changeset',
    RESET = 'reset',
}
```

In `/src/web/openapi/src/generated/models/WorkflowEventType.ts`:
```typescript
export enum WorkflowEventType {
    RUNNING = 'running',
    COMPLETED = 'completed',
    FAILED = 'failed',
}
```

Similarly for `MarkdownEventType` and `ChangeSetEventType`.

#### Event Payload Interfaces

In `/src/web/openapi/src/generated/models/`:

```typescript
// WorkflowEventPayload.ts
export interface WorkflowEventPayload {
    type: WorkflowEventType;
    workflowId: string;
    runId: string;
}

// MarkdownEventPayload.ts
export interface MarkdownEventPayload {
    type: MarkdownEventType;
    name: string;
    path: string;
}

// ChangeSetEventPayload.ts
export interface ChangeSetEventPayload {
    type: ChangeSetEventType;
    path: string;
    commit_message?: string;
    base_hash?: string;
    parent_hash?: string;
    tip_hash?: string;
}

// TicketEventPayload.ts
export interface TicketEventPayload {
    stage?: string;
    state?: TicketState;
    description?: string;
    completedAt?: string;
    notes?: string;
}

// ResetEventPayload.ts
export interface ResetEventPayload {
    resetId: string;
    anchorEventId?: string | null;
    reason: string;
}
```

#### Main Event Interface

In `/src/web/openapi/src/generated/models/TicketEvent.ts`:
```typescript
export interface TicketEvent {
    id: string;
    ticketId: string;
    projectId: string;
    kind: TicketEventKind;
    timestamp: string; // ISO 8601 date-time string
    actor: Actor;
    resetId?: string | null;
    workflowPayload?: WorkflowEventPayload;
    markdownPayload?: MarkdownEventPayload;
    changesetPayload?: ChangeSetEventPayload;
    ticketPayload?: TicketEventPayload;
    resetPayload?: ResetEventPayload;
}
```

#### Service Method

In `/src/web/openapi/src/generated/services/TicketsService.ts`:
```typescript
export class TicketsService {
    // ... existing methods ...

    /**
     * List ticket events
     * Returns all events associated with a ticket
     * @param projectId Project identifier
     * @param ticketId Ticket identifier
     * @param kind Filter by event kind
     * @param since Only return events after this timestamp
     * @param until Only return events before this timestamp
     * @param includeReset Include events that have been reset/invalidated
     * @returns Array of ticket events
     */
    public static async listTicketEvents(
        projectId: string,
        ticketId: string,
        kind?: TicketEventKind,
        since?: string,
        until?: string,
        includeReset?: boolean,
    ): Promise<TicketEvent[]> {
        const result = await __request({
            method: 'GET',
            url: '/api/projects/{projectId}/tickets/{ticketId}/events',
            path: {
                projectId,
                ticketId,
            },
            query: {
                kind,
                since,
                until,
                includeReset,
            },
        });
        return result.body;
    }
}
```

---

## File Locations

- **Generated files**: `/src/web/openapi/src/generated/`
  - `models/TicketEvent.ts`
  - `models/TicketEventKind.ts`
  - `models/WorkflowEventType.ts`
  - `models/WorkflowEventPayload.ts`
  - `models/MarkdownEventPayload.ts`
  - `models/ChangeSetEventPayload.ts`
  - `models/TicketEventPayload.ts`
  - `models/ResetEventPayload.ts`
  - `services/TicketsService.ts` (updated)
- **Generator config**: `/src/web/openapi/package.json` or `openapi-ts.config.ts`

---

## Testing

After regenerating bindings:

1. **Build check**:
```bash
cd /src/web/openapi
pnpm run build
```

2. **Type check**:
```bash
pnpm run typecheck
```

3. **Verify exports**:
```typescript
// Test that imports work
import {
    TicketEvent,
    TicketEventKind,
    WorkflowEventType,
    TicketsService,
} from '@colony2/openapi';
```

---

## Implementation Notes

### Generator Tool

The project likely uses one of:
- `openapi-typescript-codegen`
- `@openapitools/openapi-generator-cli`
- `openapi-typescript`

Check `/src/web/openapi/package.json` for the specific tool and generation script.

### Date-Time Fields

TypeScript generates `string` types for `format: date-time` fields. The UI will need to parse these using `new Date()` or a library like `dayjs` for display.

### Enum Usage

Generated enums can be used for type-safe filtering:
```typescript
// In React component
const events = await TicketsService.listTicketEvents(
    projectId,
    ticketId,
    TicketEventKind.WORKFLOW, // Type-safe!
);
```

### Nullable vs Optional

- `resetId?: string | null` - Can be undefined or null
- `notes?: string` - Can be undefined (but not null when present)

---

## Usage Example

```typescript
import { TicketsService, TicketEvent, TicketEventKind } from '@colony2/openapi';

async function fetchTicketEvents(projectId: string, ticketId: string) {
    try {
        const events: TicketEvent[] = await TicketsService.listTicketEvents(
            projectId,
            ticketId,
        );

        // Filter workflow events
        const workflowEvents = events.filter(
            e => e.kind === TicketEventKind.WORKFLOW
        );

        return events;
    } catch (error) {
        console.error('Failed to fetch ticket events:', error);
        throw error;
    }
}
```

---

## Troubleshooting

### Issue: Generation fails with "unknown format"
**Solution**: Ensure OpenAPI spec uses standard formats (`date-time`, `email`, etc.). Custom formats may need generator configuration.

### Issue: Generated types don't include new schemas
**Solution**: Verify the OpenAPI spec (spec 01) validates correctly. Run `yamllint` on the spec file.

### Issue: Import errors in consuming code
**Solution**: Rebuild the package (`pnpm run build`) and ensure dependent packages reinstall it.

### Issue: Method signature doesn't match expected types
**Solution**: Check that parameter names in OpenAPI spec match between path parameters and query parameters.

---

## Next Steps

After completing this spec:
1. Proceed to **06-ticket-detail-ui-shared.md** to create reusable TicketDetailModal component
2. The component will use the generated `TicketsService.listTicketEvents` method

---

## Success Criteria

- [ ] Client bindings regenerated successfully
- [ ] `pnpm run build` succeeds without errors
- [ ] All event types and interfaces present in generated code
- [ ] `TicketsService.listTicketEvents` method exists with correct signature
- [ ] Type checking passes
- [ ] Package exports new types correctly
- [ ] Changes committed to version control

---

## Estimated Duration

**~30 minutes** - Generation and verification
