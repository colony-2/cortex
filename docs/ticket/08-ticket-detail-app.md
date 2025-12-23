# 08 - Ticket Detail App Integration

**Cell**: `web/app`
**Dependencies**: 12
**Purpose**: Make ticket table rows clickable and show detail modal

---

## Overview

Integrate the `TicketDetailModal` component from `@colony2/shared` into the App cell's CellDetailPage. Make ticket table rows clickable so users can view ticket details and events from the table view.

---

## Changes Required

### 1. Update CellDetailPage Component

Modify `/src/web/app/src/components/CellDetailPage.tsx`:

```typescript
import React, { useState } from 'react';
import { Table, Tag, Typography } from 'antd';
import { Ticket } from '@colony2/openapi';
import { TicketDetailModal } from '@colony2/shared';
import dayjs from 'dayjs';

const { Text } = Typography;

interface CellDetailPageProps {
    projectId: string;
    cellId: string;
    // ... other existing props
}

export const CellDetailPage: React.FC<CellDetailPageProps> = ({
    projectId,
    cellId,
    ...otherProps
}) => {
    // ... existing state ...

    // Add state for modal
    const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null);

    // ... existing ticket fetching logic ...

    // Update table columns to make rows clickable
    const columns = [
        {
            title: 'Title',
            dataIndex: 'title',
            key: 'title',
            render: (text: string) => <Text strong>{text}</Text>,
        },
        {
            title: 'Stage',
            dataIndex: 'stage',
            key: 'stage',
            render: (stage: string) => <Tag color="blue">{stage}</Tag>,
        },
        {
            title: 'State',
            dataIndex: 'state',
            key: 'state',
            render: (state: string) => (
                <Tag color={state === 'working' ? 'green' : 'default'}>
                    {state}
                </Tag>
            ),
        },
        {
            title: 'Description',
            dataIndex: 'description',
            key: 'description',
            ellipsis: true,
        },
        {
            title: 'Updated',
            dataIndex: 'updatedAt',
            key: 'updatedAt',
            render: (date: string) => dayjs(date).format('YYYY-MM-DD HH:mm'),
        },
    ];

    return (
        <div>
            {/* Existing page header and other content */}

            <Table
                dataSource={tickets}
                columns={columns}
                rowKey="id"
                pagination={{ pageSize: 10 }}
                loading={loading}
                onRow={(record) => ({
                    onClick: () => setSelectedTicket(record),
                    style: { cursor: 'pointer' },
                    onKeyPress: (e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                            setSelectedTicket(record);
                        }
                    },
                    tabIndex: 0,
                })}
                rowClassName="clickable-row"
            />

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

### 2. Add Row Hover Styling

Add CSS for clickable rows in `/src/web/app/src/components/CellDetailPage.css` (or appropriate style file):

```css
.clickable-row {
    cursor: pointer;
}

.clickable-row:hover {
    background-color: #f5f5f5;
}

.clickable-row:focus {
    outline: 2px solid #1890ff;
    outline-offset: -2px;
}
```

Or use inline styles with Ant Design's Table component:

```typescript
<Table
    // ... other props ...
    onRow={(record) => ({
        onClick: () => setSelectedTicket(record),
        style: { cursor: 'pointer' },
        className: 'hover:bg-gray-50',
    })}
/>
```

### 3. Ensure projectId is Available

Verify that `projectId` is available in the `CellDetailPage` component. It should be extracted from the route:

```typescript
import { useParams } from 'react-router-dom';

function CellDetailPage() {
    const { projectId, cellId } = useParams<{ projectId: string; cellId: string }>();

    // ... rest of component
}
```

---

## File Locations

- **Component**: `/src/web/app/src/components/CellDetailPage.tsx`
- **Styles** (optional): `/src/web/app/src/components/CellDetailPage.css`

---

## Testing

### Manual Testing

1. **Run the application**:
```bash
cd /src/web/app
pnpm dev
```

2. **Test scenarios**:
   - [ ] Navigate to Cell Detail page with tickets
   - [ ] Hover over table rows - background should change
   - [ ] Click on a table row - modal should open
   - [ ] Modal should display ticket details in Details tab
   - [ ] Switch to Events tab - should show events
   - [ ] Close modal by clicking X or outside modal
   - [ ] Modal should close properly
   - [ ] Click another row - modal should show new ticket
   - [ ] Test keyboard navigation (Tab to row, Enter to open)

### Unit Tests

Update or create `/src/web/app/src/components/CellDetailPage.test.tsx`:

```typescript
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { CellDetailPage } from './CellDetailPage';
import { TicketsService } from '@colony2/openapi';
import { BrowserRouter } from 'react-router-dom';

jest.mock('@colony2/openapi');
jest.mock('@colony2/shared', () => ({
    TicketDetailModal: ({ visible, ticket }: any) =>
        visible ? <div data-testid="ticket-modal">{ticket?.title}</div> : null,
}));

const renderWithRouter = (component: React.ReactElement) => {
    return render(<BrowserRouter>{component}</BrowserRouter>);
};

describe('CellDetailPage', () => {
    const mockTickets = [
        {
            id: 'ticket-1',
            title: 'Test Ticket',
            stage: 'todo',
            state: 'working',
            description: 'Test description',
            updatedAt: '2024-01-01T00:00:00Z',
        },
    ];

    beforeEach(() => {
        (TicketsService.searchTickets as jest.Mock).mockResolvedValue({
            items: mockTickets,
        });
    });

    it('opens detail modal when table row is clicked', async () => {
        renderWithRouter(<CellDetailPage projectId="proj-1" cellId="cell-1" />);

        // Wait for tickets to load
        await waitFor(() => {
            expect(screen.getByText('Test Ticket')).toBeInTheDocument();
        });

        // Click on table row
        const row = screen.getByText('Test Ticket').closest('tr');
        fireEvent.click(row!);

        // Modal should open
        await waitFor(() => {
            expect(screen.getByTestId('ticket-modal')).toBeInTheDocument();
        });
    });

    it('displays tickets in table format', async () => {
        renderWithRouter(<CellDetailPage projectId="proj-1" cellId="cell-1" />);

        await waitFor(() => {
            expect(screen.getByText('Test Ticket')).toBeInTheDocument();
            expect(screen.getByText('todo')).toBeInTheDocument();
            expect(screen.getByText('working')).toBeInTheDocument();
            expect(screen.getByText('Test description')).toBeInTheDocument();
        });
    });

    it('closes modal when onClose is called', async () => {
        renderWithRouter(<CellDetailPage projectId="proj-1" cellId="cell-1" />);

        await waitFor(() => {
            expect(screen.getByText('Test Ticket')).toBeInTheDocument();
        });

        const row = screen.getByText('Test Ticket').closest('tr');
        fireEvent.click(row!);

        await waitFor(() => {
            expect(screen.getByTestId('ticket-modal')).toBeInTheDocument();
        });

        // Test modal close functionality
        // (Implementation depends on how modal close is exposed in tests)
    });
});
```

Run tests:
```bash
cd /src/web/app
pnpm test
```

---

## Implementation Notes

### Table onRow Handler

The `onRow` prop in Ant Design Table allows adding event handlers and props to each row:
- `onClick`: Opens modal when row is clicked
- `style`: Adds cursor pointer
- `onKeyPress`: Keyboard accessibility
- `tabIndex`: Makes row keyboard-focusable

### Row Styling

Two approaches for styling:
1. **CSS class**: Add `.clickable-row` class with hover styles
2. **Inline styles**: Use `style` prop in `onRow` handler

Choose based on project's styling conventions.

### State Management

Similar to Kanban integration:
- `selectedTicket` is `null` when modal is closed
- Set to `Ticket` object when row is clicked
- Reset to `null` when modal closes

### Performance

- Table already uses pagination (10 items per page)
- Modal lazy-loads events only when opened
- No performance concerns for typical usage

---

## Accessibility

The implementation includes keyboard accessibility:
- `tabIndex={0}` makes rows focusable
- `onKeyPress` handler responds to Enter/Space keys
- Focus outline shows which row is selected

**Best practice**: Consider adding ARIA attributes for screen readers:

```typescript
onRow={(record) => ({
    // ... other props
    'aria-label': `View details for ${record.title}`,
    role: 'button',
})}
```

---

## Alternative: Action Column

If you prefer an explicit "View" button instead of clickable rows:

```typescript
const columns = [
    // ... existing columns ...
    {
        title: 'Actions',
        key: 'actions',
        render: (_: any, record: Ticket) => (
            <Button
                type="link"
                size="small"
                onClick={(e) => {
                    e.stopPropagation(); // Prevent row click
                    setSelectedTicket(record);
                }}
            >
                View Details
            </Button>
        ),
    },
];
```

This approach is more explicit but takes up additional table space.

---

## Next Steps

After completing this spec:
1. Both Kanban (spec 07) and App (spec 08) integrations are complete
2. Consider user testing to gather feedback on the detail modal
3. Monitor for any performance issues with large event lists

---

## Success Criteria

- [ ] Table rows in Cell Detail page are clickable
- [ ] Clicking a row opens the detail modal
- [ ] Modal displays correct ticket information
- [ ] Events tab loads and displays events
- [ ] Modal closes properly
- [ ] Rows have hover effect
- [ ] Keyboard navigation works (Tab + Enter)
- [ ] projectId is correctly passed to modal
- [ ] Tests updated and passing
- [ ] Changes committed to version control

---

## Estimated Duration

**~2-3 hours** - Integration, styling, and testing
