import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import CellDetailPage from './CellDetailPage';
import { CellsService, TicketsService, TicketState, ActorType } from '@colony2/openapi-client';
import type { ManagedCell, Ticket } from '@colony2/openapi-client';

// Mock the services
vi.mock('@colony2/openapi-client', async () => {
  const actual = await vi.importActual('@colony2/openapi-client');
  return {
    ...actual,
    CellsService: {
      getApiProjectsCells1: vi.fn(),
      patchApiProjectsCells: vi.fn(),
    },
    TicketsService: {
      getApiProjectsTickets: vi.fn(),
      getApiProjectsTicketsStages: vi.fn(),
      getApiProjectsTicketsStates: vi.fn(),
      postApiProjectsTickets: vi.fn(),
    },
  };
});

// Mock react-router-dom hooks
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useParams: () => ({ cellId: 'test-cell-id' }),
  };
});

describe('CellDetailPage', () => {
  const mockCell: ManagedCell = {
    id: 'test-cell-id',
    version: 1,
    projectId: 'test-project',
    name: 'Test Cell',
    description: 'Test cell description',
    workingPath: '/path/to/cell',
    populator: 'moon',
    populatorId: 'external-123',
    dependencies: ['dep1', 'dep2'],
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-02T00:00:00Z',
  };

  const mockTickets: Ticket[] = [
    {
      id: 'ticket-1',
      version: 1,
      projectId: 'test-project',
      cellId: 'test-cell-id',
      cellName: 'Test Cell',
      title: 'Test Ticket 1',
      description: 'Ticket description',
      stage: 'todo',
      state: TicketState.WAITING_USER,
      creator: {
        type: ActorType.USER,
        user: { email: 'test@example.com' },
      },
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-02T00:00:00Z',
      validFrom: '2024-01-01T00:00:00Z',
      validUntil: '9999-12-31T23:59:59Z',
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();

    // Setup default mock responses
    vi.mocked(CellsService.getApiProjectsCells1).mockResolvedValue(mockCell);
    vi.mocked(TicketsService.getApiProjectsTickets).mockResolvedValue(mockTickets);
    vi.mocked(TicketsService.getApiProjectsTicketsStages).mockResolvedValue(['backlog', 'todo', 'done']);
    vi.mocked(TicketsService.getApiProjectsTicketsStates).mockResolvedValue(Object.values(TicketState));
  });

  afterEach(() => {
    cleanup();
  });

  const renderComponent = () => {
    return render(
      <BrowserRouter>
        <CellDetailPage projectId="test-project" />
      </BrowserRouter>
    );
  };

  it('should render cell details correctly', async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    expect(screen.getByText('Test cell description')).toBeDefined();
    expect(screen.getByText('/path/to/cell')).toBeDefined();
    expect(screen.getByText('moon')).toBeDefined();
    expect(screen.getByText('external-123')).toBeDefined();
    expect(screen.getByText('dep1')).toBeDefined();
    expect(screen.getByText('dep2')).toBeDefined();
  });

  it('should display loading state initially', () => {
    renderComponent();
    // Check for the Ant Design Spin component by class
    const { container } = render(
      <BrowserRouter>
        <CellDetailPage projectId="test-project" />
      </BrowserRouter>
    );
    expect(container.querySelector('.ant-spin')).toBeDefined();
  });

  it('should display error when cell fetch fails', async () => {
    vi.mocked(CellsService.getApiProjectsCells1).mockRejectedValue(new Error('Failed to load cell'));

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Unable to load cell')).toBeDefined();
    });
  });

  it('should display tickets list', async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('Test Ticket 1')).toBeDefined();
    });

    expect(screen.getByText('Ticket description')).toBeDefined();
    expect(screen.getByText('todo')).toBeDefined();
  });

  it('should show empty state when no tickets exist', async () => {
    vi.mocked(TicketsService.getApiProjectsTickets).mockResolvedValue([]);

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText('No tickets found for this cell')).toBeDefined();
    });
  });

  it('should open edit modal when edit button is clicked', async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    const editButton = screen.getByRole('button', { name: /edit cell/i });
    fireEvent.click(editButton);

    await waitFor(() => {
      expect(screen.getAllByText('Edit Cell').length).toBeGreaterThan(0);
    });
  });

  it('should submit edit form and update cell', async () => {
    const updatedCell = { ...mockCell, name: 'Updated Cell Name' };
    vi.mocked(CellsService.patchApiProjectsCells).mockResolvedValue(updatedCell);
    vi.mocked(CellsService.getApiProjectsCells1).mockResolvedValueOnce(mockCell).mockResolvedValueOnce(updatedCell);

    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    const editButton = screen.getByRole('button', { name: /edit cell/i });
    fireEvent.click(editButton);

    await waitFor(() => {
      const modalTitle = screen.getAllByText('Edit Cell');
      expect(modalTitle.length).toBeGreaterThan(0);
    });

    // Find the input within the modal
    const nameInput = screen.getByDisplayValue('Test Cell');
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, 'Updated Cell Name');

    // Click Save button
    const saveButton = screen.getByRole('button', { name: /save/i });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(CellsService.patchApiProjectsCells).toHaveBeenCalledWith(
        'test-project',
        'test-cell-id',
        expect.objectContaining({
          name: 'Updated Cell Name',
        })
      );
    });
  });

  it('should open create ticket modal when create button is clicked', async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    const createButton = screen.getByRole('button', { name: /create ticket/i });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getAllByText('Create Ticket').length).toBeGreaterThan(0);
    });
  });

  it('should submit create ticket form successfully', async () => {
    const newTicket: Ticket = {
      id: 'ticket-2',
      version: 1,
      projectId: 'test-project',
      cellId: 'test-cell-id',
      cellName: 'Test Cell',
      title: 'New Ticket',
      description: 'New ticket description',
      stage: 'todo',
      state: TicketState.WAITING_USER,
      creator: {
        type: ActorType.USER,
        user: { email: 'test@example.com' },
      },
      createdAt: '2024-01-03T00:00:00Z',
      updatedAt: '2024-01-03T00:00:00Z',
      validFrom: '2024-01-03T00:00:00Z',
      validUntil: '9999-12-31T23:59:59Z',
    };

    vi.mocked(TicketsService.postApiProjectsTickets).mockResolvedValue(newTicket);

    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    const createButton = screen.getByRole('button', { name: /create ticket/i });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getAllByText('Create Ticket').length).toBeGreaterThan(0);
    });

    // Fill in the form
    const titleInput = screen.getByPlaceholderText('Ticket title');
    await userEvent.type(titleInput, 'New Ticket');

    const emailInput = screen.getByPlaceholderText('you@example.com');
    await userEvent.type(emailInput, 'test@example.com');

    // Click Create button using role
    const submitButtons = screen.getAllByRole('button', { name: /create/i });
    const submitButton = submitButtons[submitButtons.length - 1]; // Get the last one (OK button in modal)
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(TicketsService.postApiProjectsTickets).toHaveBeenCalledWith(
        'test-project',
        expect.objectContaining({
          title: 'New Ticket',
          cell: 'Test Cell',
        })
      );
    });
  });

  it('should navigate back to cells list when back button is clicked', async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    const backButton = screen.getByRole('button', { name: /back to cells/i });
    fireEvent.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith('/project/test-project/cells/list');
  });

  it('should display cell with no description correctly', async () => {
    const cellWithoutDescription = { ...mockCell, description: undefined };
    vi.mocked(CellsService.getApiProjectsCells1).mockResolvedValue(cellWithoutDescription);

    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    expect(screen.getByText('No description')).toBeDefined();
  });

  it('should display cell with no dependencies correctly', async () => {
    const cellWithoutDeps = { ...mockCell, dependencies: [] };
    vi.mocked(CellsService.getApiProjectsCells1).mockResolvedValue(cellWithoutDeps);

    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    expect(screen.getByText('No dependencies')).toBeDefined();
  });

  it('should handle edit form validation errors', async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    const editButton = screen.getByRole('button', { name: /edit cell/i });
    fireEvent.click(editButton);

    await waitFor(() => {
      const modalTitle = screen.getAllByText('Edit Cell');
      expect(modalTitle.length).toBeGreaterThan(0);
    });

    // Clear the name field (required field)
    const nameInput = screen.getByDisplayValue('Test Cell');
    await userEvent.clear(nameInput);

    // Try to submit
    const saveButton = screen.getByRole('button', { name: /save/i });
    fireEvent.click(saveButton);

    // Should not call the API if validation fails
    await waitFor(() => {
      expect(CellsService.patchApiProjectsCells).not.toHaveBeenCalled();
    });
  });

  it('should handle ticket creation validation errors', async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Test Cell' })).toBeDefined();
    });

    const createButton = screen.getByRole('button', { name: /create ticket/i });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getAllByText('Create Ticket').length).toBeGreaterThan(0);
    });

    // Try to submit without filling required fields
    const submitButtons = screen.getAllByRole('button', { name: /create/i });
    const submitButton = submitButtons[submitButtons.length - 1];
    fireEvent.click(submitButton);

    // Should not call the API if validation fails
    await waitFor(() => {
      expect(TicketsService.postApiProjectsTickets).not.toHaveBeenCalled();
    });
  });
});
