import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { setupServer } from 'msw/node';
import { InputActivityProvider, inputActivityService } from '@colony2/shared';
import InputDetailPage from './InputDetailPage';
import {
  inputApiHandlers,
  resetMockInputStore,
  setupMockInputs,
  createMockPendingInput,
  createMockSingleQuestionDetails,
  mockInputDetailsStore,
} from '../test/mocks/inputApiHandlers';
import { mockEventSource } from '../test/mocks/mockSSE';

const server = setupServer(...inputApiHandlers);

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('InputDetailPage - Single Question Flow', () => {
  const projectId = 'test-project-123';
  const jobId = 'job-abc-123';
  let eventSourceMock: ReturnType<typeof mockEventSource>;

  beforeEach(() => {
    server.listen({ onUnhandledRequest: 'error' });
    resetMockInputStore();
    eventSourceMock = mockEventSource();
    mockNavigate.mockClear();
  });

  afterEach(() => {
    inputActivityService.clearCaches(); // Clear caches between tests
    server.close();
    eventSourceMock.restore();
    vi.clearAllMocks();
  });

  const renderComponent = () => {
    return render(
      <MemoryRouter initialEntries={[`/project/${projectId}/inputs/${jobId}`]}>
        <Routes>
          <Route
            path="/project/:projectId/inputs/:jobId"
            element={
              <InputActivityProvider>
                <InputDetailPage projectId={projectId} />
              </InputActivityProvider>
            }
          />
        </Routes>
      </MemoryRouter>
    );
  };

  it('should show loading state initially', () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    expect(screen.getByText('Loading input request...')).toBeInTheDocument();
  });

  it('should display input details for single question', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Input Request')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    expect(screen.getByText(`Job ID: ${jobId}`)).toBeInTheDocument();
    expect(screen.getByText(/Status:/)).toBeInTheDocument();
    expect(screen.getByText('pending')).toBeInTheDocument();
  });

  it('should render form with single question', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getAllByText('How old are you?').length).toBeGreaterThan(0);
    },
      { timeout: 3000 }
    );

    // Form should have input field
    const input = screen.getByRole('textbox');
    expect(input).toBeInTheDocument();
  });

  it('should submit response successfully', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getAllByText('How old are you?').length).toBeGreaterThan(0);
    },
      { timeout: 3000 }
    );

    // Fill out the form
    const input = screen.getByRole('textbox');
    await user.type(input, '25');

    // Submit form
    const submitButton = screen.getByRole('button', { name: /submit/i });
    await user.click(submitButton);

    // Should navigate to workflow details
    await waitFor(
      () => {
      expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/workflows/${jobId}`);
    });

    // Verify input was removed from store
    const key = `${projectId}:${jobId}`;
    expect(mockInputDetailsStore.has(key)).toBe(false);
  });

  it('should show back button', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByRole('button', { name: /back to inputs/i },
      { timeout: 3000 }
    )).toBeInTheDocument();
    });
  });

  it('should navigate back when back button clicked', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      const backButton = screen.getByRole('button', { name: /back to inputs/i },
      { timeout: 3000 }
    );
      expect(backButton).toBeInTheDocument();
    });

    const backButton = screen.getByRole('button', { name: /back to inputs/i });
    await user.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/inputs`);
  });

  it('should show cancel button in form', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByRole('button', { name: /cancel/i },
      { timeout: 3000 }
    )).toBeInTheDocument();
    });
  });

  it('should navigate back when cancel button clicked', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      const cancelButton = screen.getByRole('button', { name: /cancel/i },
      { timeout: 3000 }
    );
      expect(cancelButton).toBeInTheDocument();
    });

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    await user.click(cancelButton);

    expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/inputs`);
  });

  it('should show error when input not found', async () => {
    setupMockInputs(projectId, [], []); // No inputs
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText(/Failed to load input details/i)).toBeInTheDocument();
    },
      { timeout: 3000 }
    );
  });

  it('should display started time', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText(/Started:/)).toBeInTheDocument();
    },
      { timeout: 3000 }
    );
  });
});
