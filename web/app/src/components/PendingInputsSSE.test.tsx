import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { setupServer } from 'msw/node';
import { http, HttpResponse } from 'msw';
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
    window.history.replaceState({}, '', '/project/test-project-123/reviews');
    eventSourceMock = mockEventSource();
  });

  afterEach(() => {
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

  it('retains last-known rows while retrying a closed connection', async () => {
    const row = createMockPendingInput('retained');
    setupMockInputs(projectId, [row], []);
    renderComponent();
    await screen.findByText('Job ID: retained');
    const sse = await waitForSSEConnection();
    act(() => { sse.readyState = sse.CLOSED; sse.emitError('Connection lost'); });
    let release!: () => void;
    const gate = new Promise<void>(resolve => { release = resolve; });
    server.use(http.get(`http://localhost:8080/api/projects/${projectId}/user-inputs/pending`, async () => {
      await gate;
      return HttpResponse.json([row]);
    }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    await screen.findByText('Checking pending requests…');
    expect(screen.getByText('Job ID: retained')).toBeVisible();
    expect(screen.getByText('Last known: Showing 1 of 1 pending requests')).toBeVisible();
    await act(async () => { release(); });
    await screen.findByText('Showing 1 of 1 pending requests');
  });

  it('reapplies controls to Refresh and replacement occurrences without retaining old time', async () => {
    window.history.replaceState({}, '', '/project/test-project-123/reviews?q=foo+bar&cellId=one&sort=requested&order=descend');
    const initial = { ...createMockPendingInput('same'), title: 'Food bar', cell_id: 'one', cell_name: 'Old name', task_ordinal: 1, requested_at: '2026-11-01T12:00:00Z' };
    setupMockInputs(projectId, [initial], []);
    renderComponent();
    const sse = await waitForSSEConnection(); sse.emitConnected();
    await screen.findByText('Job ID: same');
    expect(document.querySelector('time')).not.toBeNull();
    const replacement = { ...initial, task_ordinal: 2, requested_at: undefined, cell_name: 'Renamed' };
    const other = { ...createMockPendingInput('other'), title: 'Food bar', cell_id: 'two', cell_name: 'Other' };
    act(() => sse.dispatchEvent(new MessageEvent('inputs_changed', { data: JSON.stringify({ inputs: [replacement, other] }) })));
    expect(screen.getByText('Request time unavailable')).toBeVisible();
    expect(document.querySelector('time')).toBeNull();
    expect(screen.getByText('Cell: Renamed')).toBeVisible();
    expect(screen.getByText('Showing 1 of 2 pending requests')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Cells: 1 selected' }));
    expect(within(screen.getByRole('dialog')).getByRole('checkbox', { name: 'Renamed' })).toBeChecked();
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    mockPendingInputsStore.set(projectId, [{ ...replacement, title: 'Does not match' }, other]);
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    await screen.findByText('No requests match these filters');
    expect(screen.getByRole('textbox', { name: 'Search reviews' })).toHaveValue('foo bar');
    expect(screen.getByRole('combobox', { name: 'Sort by' })).toHaveValue('requested:descend');
    expect(window.location.search).toContain('cellId=one');
  });

  it('should update list when new input arrives via SSE', async () => {
    // Start with no inputs
    setupMockInputs(projectId, [], []);
    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    // Verify empty state
    await waitFor(
      () => {
      expect(screen.getByText('No pending reviews')).toBeInTheDocument();
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
        expect(screen.getByText(`Job ID: ${newJobId}`)).toBeInTheDocument();
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
      expect(screen.getByText(`Job ID: ${jobId}`)).toBeInTheDocument();
    });

    // Simulate input being cancelled
    mockPendingInputsStore.set(projectId, []); // Remove from store

    // Emit cancelled event
    sse.emitInputCancelled(jobId);

    // Should refresh and show empty state
    await waitFor(
      () => {
        expect(screen.getByText('No pending reviews')).toBeInTheDocument();
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
      expect(screen.getByText('No pending reviews')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Add first input
    const jobId1 = 'job-1';
    mockPendingInputsStore.set(projectId, [createMockPendingInput(jobId1)]);
    sse.emitInputPending(jobId1);

    await waitFor(
      () => {
      expect(screen.getByText(`Job ID: ${jobId1}`)).toBeInTheDocument();
    });

    // Add second input
    const jobId2 = 'job-2';
    mockPendingInputsStore.set(projectId, [createMockPendingInput(jobId1), createMockPendingInput(jobId2)]);
    sse.emitInputPending(jobId2, 100);

    await waitFor(
      () => {
      expect(screen.getByText(`Job ID: ${jobId2}`)).toBeInTheDocument();
    });

    // Remove first input
    mockPendingInputsStore.set(projectId, [createMockPendingInput(jobId2)]);
    sse.emitInputCancelled(jobId1, 'completed', 100);

    await waitFor(
      () => {
      expect(screen.queryByText(`Job ID: ${jobId1}`)).not.toBeInTheDocument();
    });

    // Second should still be there
    expect(screen.getByText(`Job ID: ${jobId2}`)).toBeInTheDocument();
  });

  it('should handle heartbeat events without errors', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(
      () => {
      expect(screen.getByText('No pending reviews')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Emit heartbeat - should not cause any issues
    sse.emitHeartbeat();
    sse.emitHeartbeat();
    sse.emitHeartbeat();

    // Page should still be functional
    expect(screen.getByText('No pending reviews')).toBeInTheDocument();
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
