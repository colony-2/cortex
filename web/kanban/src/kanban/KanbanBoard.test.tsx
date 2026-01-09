import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import KanbanBoard from './KanbanBoard';
import { TicketsService, CellsService } from '@colony2/openapi-client';

vi.mock('@colony2/openapi-client', () => ({
  TicketsService: {
    getApiProjectsTickets: vi.fn(),
    getApiProjectsTicketsStages: vi.fn(),
    getApiProjectsTicketsStates: vi.fn(),
    listTicketEvents: vi.fn(),
  },
  CellsService: {
    getApiProjectsCells: vi.fn(),
  },
  ActorType: {
    USER: 'user',
    AGENT: 'agent',
  },
  TicketState: {
    WORKING: 'working',
    WAITING_USER: 'waiting_user',
    WAITING_DEPENDENCY: 'waiting_dependency',
    WAITING_CAPACITY: 'waiting_capacity',
  },
}));

vi.mock('@colony2/shared', () => ({
  TicketDetailModal: ({ visible, onClose, ticket }: any) => {
    return visible ? (
      <div data-testid="ticket-detail-modal">
        <div>Modal for ticket: {ticket?.id}</div>
        <button onClick={onClose} data-testid="close-modal">
          Close
        </button>
      </div>
    ) : null;
  },
  getUserEmail: vi.fn(() => 'test@example.com'),
}));

describe('KanbanBoard', () => {
  const mockTickets = [
    {
      id: 'ticket-1',
      version: 1,
      projectId: 'proj-123',
      cellId: 'cell-1',
      cellName: 'web-app',
      title: 'Implement feature A',
      description: 'Add new feature',
      stage: 'todo',
      state: 'waiting_user',
      creator: { type: 'user', user: { email: 'user@example.com' } },
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-02T00:00:00Z',
      validFrom: '2024-01-01T00:00:00Z',
      validUntil: '9999-12-31T23:59:59Z',
    },
    {
      id: 'ticket-2',
      version: 1,
      projectId: 'proj-123',
      cellId: 'cell-2',
      cellName: 'api',
      title: 'Fix bug B',
      description: 'Critical bug fix',
      stage: 'doing',
      state: 'working',
      creator: { type: 'user', user: { email: 'user@example.com' } },
      createdAt: '2024-01-03T00:00:00Z',
      updatedAt: '2024-01-04T00:00:00Z',
      validFrom: '2024-01-03T00:00:00Z',
      validUntil: '9999-12-31T23:59:59Z',
    },
  ];

  beforeEach(() => {
    vi.mocked(TicketsService.getApiProjectsTickets).mockResolvedValue(mockTickets);
    vi.mocked(TicketsService.getApiProjectsTicketsStages).mockResolvedValue(['todo', 'doing', 'done']);
    vi.mocked(TicketsService.getApiProjectsTicketsStates).mockResolvedValue([
      'working',
      'waiting_user',
      'waiting_dependency',
      'waiting_capacity',
    ]);
    vi.mocked(CellsService.getApiProjectsCells).mockResolvedValue([]);
    vi.mocked(TicketsService.listTicketEvents).mockResolvedValue([]);
  });

  it('renders tickets in table', async () => {
    render(
      <BrowserRouter>
        <KanbanBoard projectId="proj-123" />
      </BrowserRouter>,
    );

    await waitFor(() => {
      expect(screen.getByText('Implement feature A')).toBeInTheDocument();
      expect(screen.getByText('Fix bug B')).toBeInTheDocument();
    });

    // Check that it's a table view, not kanban columns
    const table = screen.getByRole('table');
    expect(table).toBeInTheDocument();
  });

  it('opens ticket detail modal when row is clicked', async () => {
    const user = userEvent.setup();

    render(
      <BrowserRouter>
        <KanbanBoard projectId="proj-123" />
      </BrowserRouter>,
    );

    // Wait for tickets to load
    await waitFor(() => {
      expect(screen.getByText('Implement feature A')).toBeInTheDocument();
    });

    // Click on the first ticket row
    const ticketRow = screen.getByText('Implement feature A').closest('tr');
    expect(ticketRow).toBeInTheDocument();

    if (ticketRow) {
      await user.click(ticketRow);
    }

    // Modal should be visible
    await waitFor(() => {
      expect(screen.getByTestId('ticket-detail-modal')).toBeInTheDocument();
      expect(screen.getByText('Modal for ticket: ticket-1')).toBeInTheDocument();
    });
  });

  it('closes modal when close button is clicked', async () => {
    const user = userEvent.setup();

    render(
      <BrowserRouter>
        <KanbanBoard projectId="proj-123" />
      </BrowserRouter>,
    );

    // Wait for tickets to load
    await waitFor(() => {
      expect(screen.getByText('Implement feature A')).toBeInTheDocument();
    });

    // Click on a ticket row
    const ticketRow = screen.getByText('Implement feature A').closest('tr');
    if (ticketRow) {
      await user.click(ticketRow);
    }

    // Modal should be visible
    await waitFor(() => {
      expect(screen.getByTestId('ticket-detail-modal')).toBeInTheDocument();
    });

    // Close modal
    const closeButton = screen.getByTestId('close-modal');
    await user.click(closeButton);

    // Modal should be hidden
    await waitFor(() => {
      expect(screen.queryByTestId('ticket-detail-modal')).not.toBeInTheDocument();
    });
  });

  it('displays multiple tickets in different stages', async () => {
    render(
      <BrowserRouter>
        <KanbanBoard projectId="proj-123" />
      </BrowserRouter>,
    );

    await waitFor(() => {
      // Check both tickets are rendered
      expect(screen.getByText('Implement feature A')).toBeInTheDocument();
      expect(screen.getByText('Fix bug B')).toBeInTheDocument();

      // Check cell tags
      expect(screen.getByText('web-app')).toBeInTheDocument();
      expect(screen.getByText('api')).toBeInTheDocument();
    });
  });

  it('opens create ticket modal when New Ticket button is clicked', async () => {
    const user = userEvent.setup();

    render(
      <BrowserRouter>
        <KanbanBoard projectId="proj-123" />
      </BrowserRouter>,
    );

    // Wait for component to load
    await waitFor(() => {
      expect(screen.getByText('Tickets')).toBeInTheDocument();
    });

    // Click "New Ticket" button
    const newTicketButton = screen.getByRole('button', { name: /new ticket/i });
    await user.click(newTicketButton);

    // Modal should open - check for form fields instead of title
    await waitFor(() => {
      expect(screen.getByLabelText(/title/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /create/i })).toBeInTheDocument();
    });
  });

  it('closes create ticket modal and keeps it closed', async () => {
    const user = userEvent.setup();

    render(
      <BrowserRouter>
        <KanbanBoard projectId="proj-123" />
      </BrowserRouter>,
    );

    // Wait for component to load
    await waitFor(() => {
      expect(screen.getByText('Tickets')).toBeInTheDocument();
    });

    // Open modal
    const newTicketButton = screen.getByRole('button', { name: /new ticket/i });
    await user.click(newTicketButton);

    // Wait for modal to open
    await waitFor(() => {
      expect(screen.getByLabelText(/title/i)).toBeInTheDocument();
    });

    // Close modal using cancel button
    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    await user.click(cancelButton);

    // Modal should close and stay closed
    await waitFor(() => {
      expect(screen.queryByLabelText(/title/i)).not.toBeInTheDocument();
    });

    // Wait a bit to ensure it doesn't reopen (this would fail before the fix)
    await new Promise(resolve => setTimeout(resolve, 100));
    expect(screen.queryByLabelText(/title/i)).not.toBeInTheDocument();
  });
});
