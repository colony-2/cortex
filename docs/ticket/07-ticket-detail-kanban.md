# 07 - Ticket Detail Kanban Integration

**Cell**: `web/kanban`
**Dependencies**: 12
**Purpose**: Make Kanban ticket cards clickable and show detail modal

---

## Overview

Integrate the `TicketDetailModal` component from `@colony2/shared` into the Kanban board. Make ticket cards clickable so users can view ticket details and events from the Kanban view.

---

## Changes Required

### 1. Update KanbanBoard Component

Modify `/src/web/kanban/src/kanban/KanbanBoard.tsx`:

```typescript
import React, { useState } from 'react';
import { Card, Typography, Tag } from 'antd';
import { Ticket } from '@colony2/openapi';
import { TicketDetailModal } from '@colony2/shared';

const { Title, Text } = Typography;

interface KanbanBoardProps {
    projectId: string;
    // ... other existing props
}

export const KanbanBoard: React.FC<KanbanBoardProps> = ({ projectId, ...otherProps }) => {
    // ... existing state ...

    // Add state for modal
    const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null);

    // ... existing render logic ...

    return (
        <div>
            {/* Existing Kanban board JSX */}
            {stages.map(stage => (
                <div key={stage.name} className="kanban-column">
                    <div className="kanban-header">
                        <Title level={4}>{stage.name}</Title>
                        <Text type="secondary">({stage.tickets.length})</Text>
                    </div>
                    <div className="kanban-cards">
                        {stage.tickets.map(ticket => (
                            <Card
                                key={ticket.id}
                                size="small"
                                style={{ marginBottom: 8, cursor: 'pointer' }}
                                hoverable
                                onClick={() => setSelectedTicket(ticket)}
                            >
                                <Title level={5} style={{ marginBottom: 4 }}>
                                    {ticket.title}
                                </Title>
                                <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>
                                    {ticket.state || 'unknown'}
                                </Text>
                                {ticket.cellName && (
                                    <div style={{ marginTop: 4 }}>
                                        <Tag color="blue" style={{ marginBottom: 4 }}>
                                            {ticket.cellName}
                                        </Tag>
                                    </div>
                                )}
                            </Card>
                        ))}
                    </div>
                </div>
            ))}

            {/* Add TicketDetailModal */}
            <TicketDetailModal
                visible={selectedTicket !== null}
                onClose={() => setSelectedTicket(null)}
                ticket={selectedTicket}
                projectId={projectId}
            />
        </div>
    );
};
```

### 2. Update Card Styling

The changes above add:
- `cursor: 'pointer'` - Shows pointer cursor on hover
- `hoverable` prop - Adds Ant Design hover effect
- `onClick` handler - Opens modal when card is clicked

### 3. Ensure projectId is Available

Verify that `projectId` is passed to the `KanbanBoard` component. It may need to be extracted from the route or passed as a prop from the parent component.

**Example parent component** (if not already passing projectId):

```typescript
// In the component that renders KanbanBoard
import { useParams } from 'react-router-dom';

function KanbanPage() {
    const { projectId } = useParams<{ projectId: string }>();

    return (
        <KanbanBoard
            projectId={projectId!}
            // ... other props
        />
    );
}
```

---

## File Locations

- **Component**: `/src/web/kanban/src/kanban/KanbanBoard.tsx`
- **Route/Page** (if applicable): `/src/web/kanban/src/pages/KanbanPage.tsx` or similar

---

## Testing

### Manual Testing

1. **Run the application**:
```bash
cd /src/web/kanban
pnpm dev
```

2. **Test scenarios**:
   - [ ] Navigate to Kanban board for a project
   - [ ] Hover over ticket cards - cursor should change to pointer
   - [ ] Click on a ticket card - modal should open
   - [ ] Modal should display ticket details in Details tab
   - [ ] Switch to Events tab - should show events (if any)
   - [ ] Close modal by clicking X or Cancel
   - [ ] Modal should close properly
   - [ ] Click another ticket - modal should show new ticket

### Unit Tests

Update or create `/src/web/kanban/src/kanban/KanbanBoard.test.tsx`:

```typescript
import { render, screen, fireEvent } from '@testing-library/react';
import { KanbanBoard } from './KanbanBoard';
import { TicketsService } from '@colony2/openapi';

jest.mock('@colony2/openapi');
jest.mock('@colony2/shared', () => ({
    TicketDetailModal: ({ visible, ticket }: any) =>
        visible ? <div data-testid="ticket-modal">{ticket?.title}</div> : null,
}));

describe('KanbanBoard', () => {
    const mockTickets = [
        {
            id: 'ticket-1',
            title: 'Test Ticket',
            stage: 'todo',
            state: 'working',
            cellName: 'test-cell',
        },
    ];

    beforeEach(() => {
        (TicketsService.searchTickets as jest.Mock).mockResolvedValue({
            items: mockTickets,
        });
    });

    it('opens detail modal when ticket card is clicked', async () => {
        render(<KanbanBoard projectId="proj-1" />);

        // Wait for tickets to load
        await screen.findByText('Test Ticket');

        // Click on ticket card
        fireEvent.click(screen.getByText('Test Ticket'));

        // Modal should open
        expect(screen.getByTestId('ticket-modal')).toBeInTheDocument();
        expect(screen.getByText('Test Ticket')).toBeInTheDocument();
    });

    it('closes modal when onClose is called', async () => {
        const { container } = render(<KanbanBoard projectId="proj-1" />);

        await screen.findByText('Test Ticket');
        fireEvent.click(screen.getByText('Test Ticket'));

        // Modal is open
        expect(screen.getByTestId('ticket-modal')).toBeInTheDocument();

        // Simulate close (in real component, this would be triggered by modal close button)
        // Test implementation depends on how the mock is set up
    });
});
```

Run tests:
```bash
cd /src/web/kanban
pnpm test
```

---

## Implementation Notes

### Cursor and Hover Effects

The `cursor: 'pointer'` and `hoverable` prop make it visually clear that cards are clickable. This improves UX by providing visual feedback.

### State Management

The modal state is managed locally in the `KanbanBoard` component:
- `selectedTicket` is `null` when modal is closed
- Set to a `Ticket` object when a card is clicked
- Reset to `null` when modal is closed

### ProjectId Availability

The `projectId` must be available in the component. It can come from:
- Route parameters (`useParams` from react-router)
- Props passed from parent component
- Context (if using React Context for project data)

### Performance Considerations

- Modal renders only when `visible={true}`
- Events are fetched only when modal opens (lazy loading)
- `destroyOnClose` prop on Modal ensures cleanup when closed

---

## Accessibility

Ensure keyboard accessibility:
- Cards should be keyboard-navigable (add `tabIndex={0}` if needed)
- Enter key should open modal (add `onKeyPress` handler)

**Enhanced accessibility example**:
```typescript
<Card
    key={ticket.id}
    size="small"
    style={{ marginBottom: 8, cursor: 'pointer' }}
    hoverable
    tabIndex={0}
    onClick={() => setSelectedTicket(ticket)}
    onKeyPress={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
            setSelectedTicket(ticket);
        }
    }}
    aria-label={`View details for ${ticket.title}`}
>
    {/* Card content */}
</Card>
```

---

## Next Steps

After completing this spec:
1. Proceed to **08-ticket-detail-app.md** to integrate modal into App cell table view
2. Both Kanban and App cells will share the same `TicketDetailModal` component

---

## Success Criteria

- [ ] Ticket cards in Kanban board are clickable
- [ ] Clicking a card opens the detail modal
- [ ] Modal displays correct ticket information
- [ ] Events tab loads and displays events
- [ ] Modal closes properly
- [ ] Cursor changes to pointer on hover
- [ ] Cards have hover effect
- [ ] projectId is correctly passed to modal
- [ ] Tests updated and passing
- [ ] Changes committed to version control

---

## Estimated Duration

**~2-3 hours** - Integration, styling, and testing
