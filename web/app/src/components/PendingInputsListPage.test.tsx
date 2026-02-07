import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { setupServer } from 'msw/node';
import { InputActivityProvider, inputActivityService } from '@colony2/shared';
import PendingInputsListPage from './PendingInputsListPage';
import {
  inputApiHandlers,
  resetMockInputStore,
  setupMockInputs,
  createMockPendingInput,
  createMockSingleQuestionDetails,
} from '../test/mocks/inputApiHandlers';
import { mockEventSource, waitForSSEConnection } from '../test/mocks/mockSSE';

const server = setupServer(...inputApiHandlers);

describe('PendingInputsListPage', () => {
  const projectId = 'test-project-123';
  let eventSourceMock: ReturnType<typeof mockEventSource>;

  beforeEach(() => {
    server.listen({ onUnhandledRequest: 'error' });
    resetMockInputStore();
    eventSourceMock = mockEventSource();
  });

  afterEach(() => {
    inputActivityService.disconnect(); // Clear service state
    inputActivityService.clearCaches(); // Clear caches between tests
    server.close();
    server.resetHandlers();
    eventSourceMock.restore();
    vi.clearAllMocks();
  });

  const renderComponent = () => {
    return render(
      <BrowserRouter>
        <InputActivityProvider>
          <PendingInputsListPage projectId={projectId} />
        </InputActivityProvider>
      </BrowserRouter>
    );
  };

  it('should show loading state initially', () => {
    renderComponent();
    expect(screen.getByText('Connecting to input stream...')).toBeInTheDocument();
  });

  it('should show empty state when no pending inputs', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();

    // Wait for SSE connection
    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(() => {
      expect(screen.getByText('No pending inputs')).toBeInTheDocument();
    });

    expect(screen.getByText('Pending input requests from workflows will appear here')).toBeInTheDocument();
  });

  it('should display list of pending inputs', async () => {
    const jobId1 = 'job-abc-123';
    const jobId2 = 'job-def-456';

    setupMockInputs(
      projectId,
      [createMockPendingInput(jobId1), createMockPendingInput(jobId2)],
      [createMockSingleQuestionDetails(jobId1), createMockSingleQuestionDetails(jobId2)]
    );

    renderComponent();

    // Wait for SSE connection
    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(
      () => {
        expect(screen.getByText(`Job ID: ${jobId1}`)).toBeInTheDocument();
        expect(screen.getByText(`Job ID: ${jobId2}`)).toBeInTheDocument();
      },
      { timeout: 3000 }
    );

    expect(screen.getAllByText('Input Request').length).toBeGreaterThanOrEqual(2);
  });

  it('should show refresh button', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument();
    });
  });

  it('should display view buttons for each input', async () => {
    const jobId = 'job-abc-123';

    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);

    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(
      () => {
        const viewButtons = screen.getAllByRole('button', { name: /open in workflow/i });
        expect(viewButtons).toHaveLength(1);
      },
      { timeout: 3000 }
    );
  });

  it('should have correct page title', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(() => {
      expect(screen.getByText('Pending Inputs')).toBeInTheDocument();
    });
  });
});
