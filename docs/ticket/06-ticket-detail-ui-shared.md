# 06 - Ticket Detail UI Shared Component

**Cell**: `web/shared`
**Dependencies**: 11
**Purpose**: Create reusable TicketDetailModal component for displaying ticket details and events

---

## Overview

Create a reusable React component in the `web/shared` cell that displays ticket details and associated events in a modal dialog. This component will be used by both the Kanban board (spec 13) and the App cell's table view (spec 14).

---

## Changes Required

### 1. Create TicketDetailModal Component

Create `/src/web/shared/src/components/TicketDetailModal.tsx`:

```typescript
import React, { useEffect, useState } from 'react';
import { Modal, Descriptions, Timeline, Tag, Spin, Alert, Tabs } from 'antd';
import {
    Ticket,
    TicketEvent,
    TicketEventKind,
    TicketsService,
    WorkflowEventType,
} from '@colony2/openapi';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';

dayjs.extend(relativeTime);

export interface TicketDetailModalProps {
    /** Whether the modal is visible */
    visible: boolean;
    /** Callback when modal is closed */
    onClose: () => void;
    /** Ticket to display details for */
    ticket: Ticket | null;
    /** Project ID (required for fetching events) */
    projectId: string;
}

export const TicketDetailModal: React.FC<TicketDetailModalProps> = ({
    visible,
    onClose,
    ticket,
    projectId,
}) => {
    const [events, setEvents] = useState<TicketEvent[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // Fetch events when ticket changes
    useEffect(() => {
        if (!ticket || !visible) {
            setEvents([]);
            setError(null);
            return;
        }

        const fetchEvents = async () => {
            setLoading(true);
            setError(null);
            try {
                const fetchedEvents = await TicketsService.listTicketEvents(
                    projectId,
                    ticket.id,
                );
                // Sort by timestamp descending (newest first)
                const sorted = [...fetchedEvents].sort(
                    (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
                );
                setEvents(sorted);
            } catch (err) {
                console.error('Failed to fetch ticket events:', err);
                setError('Failed to load ticket events. Please try again.');
            } finally {
                setLoading(false);
            }
        };

        fetchEvents();
    }, [ticket, projectId, visible]);

    if (!ticket) {
        return null;
    }

    return (
        <Modal
            title={`Ticket: ${ticket.title}`}
            open={visible}
            onCancel={onClose}
            footer={null}
            width={800}
            destroyOnClose
        >
            <Tabs
                defaultActiveKey="details"
                items={[
                    {
                        key: 'details',
                        label: 'Details',
                        children: <TicketDetails ticket={ticket} />,
                    },
                    {
                        key: 'events',
                        label: `Events (${events.length})`,
                        children: loading ? (
                            <div style={{ textAlign: 'center', padding: '40px 0' }}>
                                <Spin size="large" />
                            </div>
                        ) : error ? (
                            <Alert type="error" message={error} showIcon />
                        ) : (
                            <TicketEventsTimeline events={events} />
                        ),
                    },
                ]}
            />
        </Modal>
    );
};

// Sub-component: Ticket Details Tab
const TicketDetails: React.FC<{ ticket: Ticket }> = ({ ticket }) => {
    return (
        <Descriptions bordered column={1} size="small">
            <Descriptions.Item label="ID">{ticket.id}</Descriptions.Item>
            <Descriptions.Item label="Cell">
                <Tag color="blue">{ticket.cellName}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Stage">
                <Tag color="blue">{ticket.stage}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="State">
                <Tag color={getStateColor(ticket.state)}>{ticket.state}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Description">
                {ticket.description || <span style={{ color: '#999' }}>No description</span>}
            </Descriptions.Item>
            <Descriptions.Item label="Creator">
                {formatActor(ticket.creator)}
            </Descriptions.Item>
            <Descriptions.Item label="Created">
                {dayjs(ticket.createdAt).format('YYYY-MM-DD HH:mm:ss')}
                {' '}
                <span style={{ color: '#999' }}>({dayjs(ticket.createdAt).fromNow()})</span>
            </Descriptions.Item>
            <Descriptions.Item label="Updated">
                {dayjs(ticket.updatedAt).format('YYYY-MM-DD HH:mm:ss')}
                {' '}
                <span style={{ color: '#999' }}>({dayjs(ticket.updatedAt).fromNow()})</span>
            </Descriptions.Item>
            {ticket.completedAt && (
                <Descriptions.Item label="Completed">
                    {dayjs(ticket.completedAt).format('YYYY-MM-DD HH:mm:ss')}
                    {' '}
                    <span style={{ color: '#999' }}>({dayjs(ticket.completedAt).fromNow()})</span>
                </Descriptions.Item>
            )}
        </Descriptions>
    );
};

// Sub-component: Events Timeline Tab
const TicketEventsTimeline: React.FC<{ events: TicketEvent[] }> = ({ events }) => {
    if (events.length === 0) {
        return (
            <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
                No events found for this ticket.
            </div>
        );
    }

    return (
        <Timeline
            style={{ marginTop: 16 }}
            items={events.map(event => ({
                color: getEventColor(event),
                children: <EventItem event={event} />,
            }))}
        />
    );
};

// Sub-component: Individual Event Item
const EventItem: React.FC<{ event: TicketEvent }> = ({ event }) => {
    const renderEventDetails = () => {
        switch (event.kind) {
            case TicketEventKind.WORKFLOW:
                if (!event.workflowPayload) return null;
                return (
                    <div>
                        <strong>Workflow {event.workflowPayload.type}</strong>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            Workflow ID: <code>{event.workflowPayload.workflowId}</code>
                        </div>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            Run ID: <code>{event.workflowPayload.runId}</code>
                        </div>
                    </div>
                );

            case TicketEventKind.TICKET:
                if (!event.ticketPayload) return null;
                return (
                    <div>
                        <strong>Ticket updated</strong>
                        {event.ticketPayload.stage && (
                            <div style={{ fontSize: '12px' }}>Stage: {event.ticketPayload.stage}</div>
                        )}
                        {event.ticketPayload.state && (
                            <div style={{ fontSize: '12px' }}>State: {event.ticketPayload.state}</div>
                        )}
                        {event.ticketPayload.notes && (
                            <div style={{ fontSize: '12px', fontStyle: 'italic' }}>
                                "{event.ticketPayload.notes}"
                            </div>
                        )}
                    </div>
                );

            case TicketEventKind.MARKDOWN_DOC:
                if (!event.markdownPayload) return null;
                return (
                    <div>
                        <strong>Document {event.markdownPayload.type}</strong>
                        <div style={{ fontSize: '12px' }}>{event.markdownPayload.name}</div>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            <code>{event.markdownPayload.path}</code>
                        </div>
                    </div>
                );

            case TicketEventKind.CHANGESET:
                if (!event.changesetPayload) return null;
                return (
                    <div>
                        <strong>Code {event.changesetPayload.type}</strong>
                        <div style={{ fontSize: '12px' }}>{event.changesetPayload.commit_message}</div>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            <code>{event.changesetPayload.path}</code>
                        </div>
                    </div>
                );

            case TicketEventKind.RESET:
                if (!event.resetPayload) return null;
                return (
                    <div>
                        <strong>History reset</strong>
                        <div style={{ fontSize: '12px' }}>{event.resetPayload.reason}</div>
                    </div>
                );

            default:
                return <div>Unknown event type</div>;
        }
    };

    return (
        <div>
            {renderEventDetails()}
            <div style={{ fontSize: '11px', color: '#999', marginTop: 4 }}>
                {dayjs(event.timestamp).format('YYYY-MM-DD HH:mm:ss')} ({dayjs(event.timestamp).fromNow()})
                {' · '}
                {formatActor(event.actor)}
            </div>
        </div>
    );
};

// Helper: Get color for ticket state
function getStateColor(state: string): string {
    switch (state) {
        case 'working':
            return 'green';
        case 'waiting_user':
            return 'orange';
        case 'waiting_dependency':
            return 'blue';
        case 'waiting_capacity':
            return 'purple';
        default:
            return 'default';
    }
}

// Helper: Get color for event timeline dot
function getEventColor(event: TicketEvent): string {
    switch (event.kind) {
        case TicketEventKind.WORKFLOW:
            if (event.workflowPayload?.type === WorkflowEventType.COMPLETED) return 'green';
            if (event.workflowPayload?.type === WorkflowEventType.FAILED) return 'red';
            return 'blue';
        case TicketEventKind.TICKET:
            return 'gray';
        case TicketEventKind.CHANGESET:
            return 'purple';
        case TicketEventKind.MARKDOWN_DOC:
            return 'cyan';
        case TicketEventKind.RESET:
            return 'orange';
        default:
            return 'gray';
    }
}

// Helper: Format actor for display
function formatActor(actor: any): string {
    if (actor.type === 'user' && actor.user) {
        return actor.user.email;
    }
    if (actor.type === 'agent' && actor.agent) {
        return `${actor.agent.cell} (agent)`;
    }
    return 'Unknown';
}
```

### 2. Export from Shared Package

Add export to `/src/web/shared/src/index.ts`:

```typescript
export { TicketDetailModal } from './components/TicketDetailModal';
export type { TicketDetailModalProps } from './components/TicketDetailModal';
```

### 3. Add Dependencies

Ensure `dayjs` is installed in `web/shared`:

```bash
cd /src/web/shared
pnpm add dayjs
```

---

## File Locations

- **Component**: `/src/web/shared/src/components/TicketDetailModal.tsx`
- **Export**: `/src/web/shared/src/index.ts`
- **Package**: `/src/web/shared/package.json`

---

## Component API

### Props

| Prop | Type | Required | Description |
|------|------|----------|-------------|
| `visible` | `boolean` | Yes | Controls modal visibility |
| `onClose` | `() => void` | Yes | Callback when modal is closed |
| `ticket` | `Ticket \| null` | Yes | Ticket to display (null hides modal) |
| `projectId` | `string` | Yes | Project ID for fetching events |

### Usage Example

```typescript
import { TicketDetailModal } from '@colony2/shared';
import { Ticket } from '@colony2/openapi';

function MyComponent() {
    const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null);
    const projectId = 'my-project-id';

    return (
        <>
            <button onClick={() => setSelectedTicket(someTicket)}>
                View Ticket
            </button>

            <TicketDetailModal
                visible={selectedTicket !== null}
                onClose={() => setSelectedTicket(null)}
                ticket={selectedTicket}
                projectId={projectId}
            />
        </>
    );
}
```

---

## Features

### Details Tab
- Ticket ID, cell, stage, state
- Description
- Creator information
- Timestamps (created, updated, completed)
- Formatted with relative times ("2 hours ago")

### Events Tab
- Timeline view of all ticket events
- Sorted newest first
- Color-coded by event type:
  - Workflow completed: green
  - Workflow failed: red
  - Workflow running: blue
  - Ticket update: gray
  - ChangeSet: purple
  - Markdown doc: cyan
  - Reset: orange
- Event-specific details based on type
- Actor and timestamp for each event
- Loading state with spinner
- Error handling with alert

---

## Testing

### Unit Tests

Create `/src/web/shared/src/components/TicketDetailModal.test.tsx`:

```typescript
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { TicketDetailModal } from './TicketDetailModal';
import { TicketsService } from '@colony2/openapi';

jest.mock('@colony2/openapi');

describe('TicketDetailModal', () => {
    const mockTicket = {
        id: 'ticket-123',
        title: 'Test Ticket',
        cellName: 'test-cell',
        stage: 'todo',
        state: 'working',
        creator: { type: 'user', user: { email: 'test@example.com' } },
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-02T00:00:00Z',
    };

    beforeEach(() => {
        (TicketsService.listTicketEvents as jest.Mock).mockResolvedValue([]);
    });

    it('renders ticket details', () => {
        render(
            <TicketDetailModal
                visible={true}
                onClose={() => {}}
                ticket={mockTicket}
                projectId="proj-1"
            />
        );

        expect(screen.getByText('Ticket: Test Ticket')).toBeInTheDocument();
        expect(screen.getByText('ticket-123')).toBeInTheDocument();
    });

    it('fetches and displays events', async () => {
        const mockEvents = [
            {
                id: 'event-1',
                kind: 'workflow',
                timestamp: '2024-01-01T12:00:00Z',
                workflowPayload: {
                    type: 'completed',
                    workflowId: 'wf-123',
                    runId: 'run-456',
                },
                actor: { type: 'user', user: { email: 'test@example.com' } },
            },
        ];
        (TicketsService.listTicketEvents as jest.Mock).mockResolvedValue(mockEvents);

        render(
            <TicketDetailModal
                visible={true}
                onClose={() => {}}
                ticket={mockTicket}
                projectId="proj-1"
            />
        );

        // Switch to events tab
        userEvent.click(screen.getByText(/Events/));

        await waitFor(() => {
            expect(screen.getByText('Workflow completed')).toBeInTheDocument();
        });
    });

    it('calls onClose when modal is closed', () => {
        const onClose = jest.fn();
        render(
            <TicketDetailModal
                visible={true}
                onClose={onClose}
                ticket={mockTicket}
                projectId="proj-1"
            />
        );

        userEvent.click(screen.getByLabelText('Close'));
        expect(onClose).toHaveBeenCalled();
    });
});
```

Run tests:
```bash
cd /src/web/shared
pnpm test
```

---

## Styling Notes

- Uses Ant Design components for consistent UI
- Responsive layout (800px modal width)
- Color-coded states and event types
- Monospace font for technical IDs and paths
- Relative time display for better UX

---

## Next Steps

After completing this spec:
1. Proceed to **07-ticket-detail-kanban.md** to integrate modal into Kanban board
2. Proceed to **08-ticket-detail-app.md** to integrate modal into App cell table view

---

## Success Criteria

- [ ] TicketDetailModal component created
- [ ] Exported from `@colony2/shared` package
- [ ] Details tab shows all ticket information
- [ ] Events tab fetches and displays events
- [ ] Timeline color-coded by event type
- [ ] Loading and error states handled
- [ ] Component tests written and passing
- [ ] TypeScript compiles without errors
- [ ] Changes committed to version control

---

## Estimated Duration

**~4-5 hours** - Component implementation, styling, and testing
