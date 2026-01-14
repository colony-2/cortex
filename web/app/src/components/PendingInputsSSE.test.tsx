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
  mockPendingInputsStore,
} from '../test/mocks/inputApiHandlers';
import { mockEventSource, waitForSSEConnection } from '../test/mocks/mockSSE';

const server = setupServer(...inputApiHandlers);

describe('PendingInputsListPage - Real-time SSE Updates', () => {
  const projectId = 'test-project-123';
  let eventSourceMock: ReturnType<typeof mockEventSource>;

  beforeEach(() => {
    server.listen({ onUnhandledRequest: 'error' });
    resetMockInputStore();
    eventSourceMock = mockEventSource();
  });

  afterEach(() => {
    inputActivityService.clearCaches(); // Clear caches between tests
    server.close();
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

  it('should update list when new input arrives via SSE', async () => {
    // Start with no inputs
    setupMockInputs(projectId, [], []);
    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    // Verify empty state
    await waitFor(
      () => {
      expect(screen.getByText('No pending inputs')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Simulate new input arriving via SSE
    const newJobId = 'job-new-123';
    const newInput = createMockPendingInput(newJobId);
    const newDetails = createMockSingleQuestionDetails(newJobId);

    // Update mock store
    mockPendingInputsStore.set(projectId, [newInput]);

    // Emit SSE event
    sse.emitInputPending(newJobId);

    // Should fetch and display new input
    await waitFor(
      () => {
        expect(screen.getByText(`Input Request #${newJobId}`)).toBeInTheDocument();
      },
      { timeout: 2000 }
    );
  });

  it('should remove input from list when cancelled via SSE', async () => {
    const jobId = 'job-abc-123';
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    // Verify input is displayed
    await waitFor(
      () => {
      expect(screen.getByText(`Input Request #${jobId}`)).toBeInTheDocument();
    });

    // Simulate input being cancelled
    mockPendingInputsStore.set(projectId, []); // Remove from store

    // Emit cancelled event
    sse.emitInputCancelled(jobId);

    // Should refresh and show empty state
    await waitFor(
      () => {
        expect(screen.getByText('No pending inputs')).toBeInTheDocument();
      },
      { timeout: 2000 }
    );
  });

  it('should handle multiple SSE events in sequence', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(
      () => {
      expect(screen.getByText('No pending inputs')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Add first input
    const jobId1 = 'job-1';
    mockPendingInputsStore.set(projectId, [createMockPendingInput(jobId1)]);
    sse.emitInputPending(jobId1);

    await waitFor(
      () => {
      expect(screen.getByText(`Input Request #${jobId1}`)).toBeInTheDocument();
    });

    // Add second input
    const jobId2 = 'job-2';
    mockPendingInputsStore.set(projectId, [createMockPendingInput(jobId1), createMockPendingInput(jobId2)]);
    sse.emitInputPending(jobId2, 100);

    await waitFor(
      () => {
      expect(screen.getByText(`Input Request #${jobId2}`)).toBeInTheDocument();
    });

    // Remove first input
    mockPendingInputsStore.set(projectId, [createMockPendingInput(jobId2)]);
    sse.emitInputCancelled(jobId1, 'completed', 100);

    await waitFor(
      () => {
      expect(screen.queryByText(`Input Request #${jobId1}`)).not.toBeInTheDocument();
    });

    // Second should still be there
    expect(screen.getByText(`Input Request #${jobId2}`)).toBeInTheDocument();
  });

  it('should handle heartbeat events without errors', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(
      () => {
      expect(screen.getByText('No pending inputs')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Emit heartbeat - should not cause any issues
    sse.emitHeartbeat();
    sse.emitHeartbeat();
    sse.emitHeartbeat();

    // Page should still be functional
    expect(screen.getByText('No pending inputs')).toBeInTheDocument();
  });

  it('should connect to correct SSE endpoint for project', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();

    const sse = await waitForSSEConnection();

    // Verify URL contains project ID
    expect(sse.url).toContain(`/projects/${projectId}/user-inputs/stream`);
  });

  it('should reconnect SSE when project changes', async () => {
    const { rerender } = render(
      <BrowserRouter>
        <InputActivityProvider>
          <PendingInputsListPage projectId={projectId} />
        </InputActivityProvider>
      </BrowserRouter>
    );

    const sse1 = await waitForSSEConnection();
    expect(sse1.url).toContain(`/projects/${projectId}/user-inputs/stream`);

    // Change to different project
    const newProjectId = 'different-project-456';
    setupMockInputs(newProjectId, [], []);

    rerender(
      <BrowserRouter>
        <InputActivityProvider>
          <PendingInputsListPage projectId={newProjectId} />
        </InputActivityProvider>
      </BrowserRouter>
    );

    // Should connect to new project's SSE endpoint
    await waitFor(
      () => {
      const sse2 = eventSourceMock.getInstance();
      expect(sse2?.url).toContain(`/projects/${newProjectId}/user-inputs/stream`);
    });
  });
});
