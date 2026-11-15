import { http, HttpResponse } from 'msw';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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

const server = setupServer(...inputApiHandlers, http.get('http://localhost:8080/api/projects/:projectId/jobs/:jobId', () => HttpResponse.json({ cell_name: 'platform', prompt: 'Build a useful release' })));

describe('PendingInputsListPage', () => {
  const projectId = 'test-project-123';
  let eventSourceMock: ReturnType<typeof mockEventSource>;

  beforeEach(() => {
    server.listen({ onUnhandledRequest: 'error' });
    resetMockInputStore();
    window.history.replaceState({}, '', '/project/test-project-123/reviews');
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

  it('shows all requests together without fetching job details', async () => {
    setupMockInputs(projectId, [{ id: 'ordinary' }, { id: 'review', title: 'Review the design', cell_name: 'platform' }], []);
    const reads = vi.fn();
    server.use(http.get('http://localhost:8080/api/projects/:projectId/jobs/:jobId', () => { reads(); return HttpResponse.error(); }));
    renderComponent();
    (await waitForSSEConnection()).emitConnected();
    expect(await screen.findByText('Job ID: ordinary')).toBeVisible();
    expect(screen.getByText('Job ID: review')).toBeVisible();
    expect(screen.getAllByRole('button', { name: 'Open Review' })).toHaveLength(2);
    expect(reads).not.toHaveBeenCalled();
  });

  it('combines local controls, preserves snapshots and adds no reads from entry through Reset', async () => {
    const reads: string[] = [];
    const record = ({ request }: { request: Request }) => reads.push(new URL(request.url).pathname);
    server.events.on('request:start', record);
    setupMockInputs(projectId, [
      { id: 'ordinary', title: 'Food options at the bar', cell_id: 'one', cell_name: 'Alpha' },
      { id: 'review', title: 'Review design', prompt: 'bar with food', cell_id: 'two', cell_name: 'Beta' },
      { id: 'split', title: 'Food', prompt: 'bar', cell_id: 'two', cell_name: 'Beta' },
    ], []);
    renderComponent();
    const sse = await waitForSSEConnection(); sse.emitConnected();
    await screen.findByText('Job ID: ordinary');
    await waitFor(() => expect(reads).toHaveLength(1));
    expect(reads).toEqual([`/api/projects/${projectId}/user-inputs/pending`]);
    const search = screen.getByRole('textbox', { name: 'Search reviews' });
    const historyLength = window.history.length;
    fireEvent.change(search, { target: { value: ' BAR foo ' } });
    expect(window.history.length).toBe(historyLength);
    expect(screen.getByText('Showing 2 of 3 pending requests')).toBeVisible();
    expect(screen.queryByText('Job ID: split')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Cells: All' }));
    let dialog = screen.getByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Select none' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByText('Showing 2 of 3 pending requests')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Cells: All' }));
    dialog = screen.getByRole('dialog');
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Beta' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Apply' }));
    expect(window.history.length).toBe(historyLength + 1);
    expect(screen.getByText('Showing 1 of 3 pending requests')).toBeVisible();
    fireEvent.change(screen.getByRole('combobox', { name: 'Sort by' }), { target: { value: 'title:descend' } });
    expect(window.location.search).toContain('sort=title');
    act(() => sse.dispatchEvent(new MessageEvent('inputs_changed', { data: JSON.stringify({ inputs: [
      { ...createMockPendingInput('new'), title: 'Food bar', cell_id: 'three', cell_name: 'Gamma' },
    ] }) })));
    expect(screen.getByText('No requests match these filters')).toBeVisible();
    expect(search).toHaveValue(' BAR foo ');
    fireEvent.click(screen.getByRole('button', { name: 'Cells: 1 selected' }));
    dialog = screen.getByRole('dialog');
    expect(within(dialog).getByRole('checkbox', { name: 'Selected cell (one)' })).toBeChecked();
    expect(within(dialog).getByRole('checkbox', { name: 'Gamma' })).not.toBeChecked();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Select all' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Apply' }));
    expect(screen.getByText('Job ID: new')).toBeVisible();
    fireEvent.change(search, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Reset' }));
    expect(window.location.search).toBe('');
    expect(reads).toHaveLength(1);
    server.events.removeListener('request:start', record);
  });

  it('keeps None distinct from All on an empty queue and later snapshots', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();
    const sse = await waitForSSEConnection(); sse.emitConnected();
    await screen.findByText('No pending reviews');
    const apply = (action: string) => {
      fireEvent.click(screen.getByRole('button', { name: /^Cells:/ }));
      const dialog = screen.getByRole('dialog');
      fireEvent.click(within(dialog).getByRole('button', { name: action }));
      fireEvent.click(within(dialog).getByRole('button', { name: 'Apply' }));
    };
    apply('Select none'); expect(window.location.search).toBe('?cellSelection=none');
    act(() => sse.dispatchEvent(new MessageEvent('inputs_changed', { data: JSON.stringify({ inputs: [createMockPendingInput('arriving')] }) })));
    expect(screen.getByText('No cells selected')).toBeVisible();
    apply('Select all'); expect(window.location.search).toBe('');
    expect(screen.getByText('Job ID: arriving')).toBeVisible();
    act(() => sse.dispatchEvent(new MessageEvent('inputs_changed', { data: JSON.stringify({ inputs: [] }) })));
    apply('Select all'); expect(window.location.search).toBe('');
  });

  it('surfaces missing cell ownership from SSE and recovers through a fresh registry-era summary', async () => {
    setupMockInputs(projectId, [{ id: 'review' }], []);
    renderComponent();
    const sse = await waitForSSEConnection();
    sse.emitConnected();
    await screen.findByText('Job ID: review');
    act(() => sse.dispatchEvent(new MessageEvent('inputs_changed', { data: JSON.stringify({ inputs: [{ id: 'invalid' }] }) })));
    expect(await screen.findByRole('alert')).toHaveTextContent('required cell_id is missing');
    expect(screen.queryByText('Job ID: review')).not.toBeInTheDocument();
    expect(screen.queryByText('No pending reviews')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry reviews' }));
    await screen.findByText('Job ID: review');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('should show empty state when no pending inputs', async () => {
    setupMockInputs(projectId, [], []);
    renderComponent();

    // Wait for SSE connection
    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(() => {
      expect(screen.getByText('No pending reviews')).toBeInTheDocument();
    });

    expect(screen.getByText('Reviews and input requests from jobs will appear here')).toBeInTheDocument();
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

    expect(screen.getAllByText('Review').length).toBeGreaterThanOrEqual(2);
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
        const viewButtons = screen.getAllByRole('button', { name: /open review/i });
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
      expect(screen.getByRole('heading', { name: 'Reviews' })).toBeInTheDocument();
    });
  });
});
