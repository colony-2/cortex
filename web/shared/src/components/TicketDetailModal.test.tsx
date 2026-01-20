import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { TicketDetailModal } from './TicketDetailModal';
import { TicketsService } from '@colony2/openapi-client';
import type { Ticket } from '@colony2/openapi-client';

vi.mock('@colony2/openapi-client', () => ({
    TicketsService: {
        getApiProjectsTicketsEvents: vi.fn(),
    },
}));

describe('TicketDetailModal', () => {
    const mockTicket: Ticket = {
        id: 'ticket-123',
        version: 1,
        projectId: 'proj-1',
        cellId: 'cell-1',
        cellName: 'test-cell',
        title: 'Test Ticket',
        description: 'Test description',
        stage: 'todo',
        state: 'working',
        creator: { type: 'user', user: { email: 'test@example.com' } },
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-02T00:00:00Z',
        validFrom: '2024-01-01T00:00:00Z',
        validUntil: '9999-12-31T23:59:59Z',
    };

    beforeEach(() => {
        vi.mocked(TicketsService.getApiProjectsTicketsEvents).mockResolvedValue([]);
    });

    afterEach(() => {
        cleanup();
        vi.clearAllMocks();
    });

    it('renders ticket details', async () => {
        render(
            <TicketDetailModal
                visible={true}
                onClose={() => {}}
                ticket={mockTicket}
                projectId="proj-1"
            />
        );

        expect(screen.getByText('[test-cell] Test Ticket')).toBeInTheDocument();
        expect(screen.getByText('ticket-123')).toBeInTheDocument();
        expect(screen.getByText('test-cell')).toBeInTheDocument();
        expect(screen.getByText('todo')).toBeInTheDocument();
    });

    it('fetches and displays events', async () => {
        const mockEvents = [
            {
                id: 'event-1',
                ticketId: 'ticket-123',
                projectId: 'proj-1',
                kind: 'workflow' as const,
                timestamp: '2024-01-01T12:00:00Z',
                actor: { type: 'user' as const, user: { email: 'test@example.com' } },
                workflowPayload: {
                    type: 'completed' as const,
                    workflowId: 'wf-123',
                    runId: 'run-456',
                },
            },
        ];
        vi.mocked(TicketsService.getApiProjectsTicketsEvents).mockResolvedValue(mockEvents);

        render(
            <TicketDetailModal
                visible={true}
                onClose={() => {}}
                ticket={mockTicket}
                projectId="proj-1"
            />
        );

        await waitFor(() => {
            expect(screen.getByText('Workflow completed')).toBeInTheDocument();
        });
    });

    it('shows error message on fetch failure', async () => {
        vi.mocked(TicketsService.getApiProjectsTicketsEvents).mockRejectedValue(new Error('Network error'));

        render(
            <TicketDetailModal
                visible={true}
                onClose={() => {}}
                ticket={mockTicket}
                projectId="proj-1"
            />
        );

        await waitFor(() => {
            expect(screen.getByText(/Failed to load ticket events/i)).toBeInTheDocument();
        });
    });

    it('calls onClose when modal is closed', async () => {
        const onClose = vi.fn();
        render(
            <TicketDetailModal
                visible={true}
                onClose={onClose}
                ticket={mockTicket}
                projectId="proj-1"
            />
        );

        const closeButton = screen.getByRole('button', { name: /close/i });
        await userEvent.click(closeButton);

        expect(onClose).toHaveBeenCalled();
    });

    it('does not render when ticket is null', () => {
        const { container } = render(
            <TicketDetailModal
                visible={true}
                onClose={() => {}}
                ticket={null}
                projectId="proj-1"
            />
        );

        expect(container.firstChild).toBeNull();
    });

    it('shows empty state when no events exist', async () => {
        vi.mocked(TicketsService.getApiProjectsTicketsEvents).mockResolvedValue([]);

        render(
            <TicketDetailModal
                visible={true}
                onClose={() => {}}
                ticket={mockTicket}
                projectId="proj-1"
            />
        );

        await waitFor(() => {
            expect(screen.getByText(/No events found for this ticket/i)).toBeInTheDocument();
        });
    });

    it('displays completed date when present', () => {
        const completedTicket = {
            ...mockTicket,
            completedAt: '2024-01-03T00:00:00Z',
        };

        render(
            <TicketDetailModal
                visible={true}
                onClose={() => {}}
                ticket={completedTicket}
                projectId="proj-1"
            />
        );

        expect(screen.getByText('Completed')).toBeInTheDocument();
    });
});
