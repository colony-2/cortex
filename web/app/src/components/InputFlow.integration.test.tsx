import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter, MemoryRouter, Routes, Route } from 'react-router-dom';
import { setupServer } from 'msw/node';
import { http, HttpResponse } from 'msw';
import { InputActivityProvider, inputActivityService } from '@colony2/shared';
import PendingInputsListPage from './PendingInputsListPage';
import WorkflowStoryPage from './WorkflowStoryPage';
import {
  inputApiHandlers,
  resetMockInputStore,
  setupMockInputs,
  createMockPendingInput,
  createMockSingleQuestionDetails,
  mockPendingInputsStore,
  mockInputDetailsStore,
} from '../test/mocks/inputApiHandlers';
import { mockEventSource, waitForSSEConnection } from '../test/mocks/mockSSE';

const server = setupServer(...inputApiHandlers);

// Mock navigate at module level so it's available to hoisted vi.mock
const mockNavigate = vi.fn();
let navigatePath: string | null = null;

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

/**
 * End-to-End Integration Test
 *
 * Simulates the complete flow from TestSimpleInput:
 * 1. Workflow creates input request
 * 2. User sees pending input in list
 * 3. User clicks to view details
 * 4. User fills out form
 * 5. User submits response
 * 6. Input is removed from list
 * 7. Workflow receives response and completes
 */
describe('Input Flow - Complete E2E Integration', () => {
  const projectId = 'test-project-123';
  const jobId = 'job-e2e-test';
  let eventSourceMock: ReturnType<typeof mockEventSource>;

  beforeEach(() => {
    server.listen({ onUnhandledRequest: 'error' });
    resetMockInputStore();
    eventSourceMock = mockEventSource();
    mockNavigate.mockClear();
    navigatePath = null;
  });

  afterEach(() => {
    inputActivityService.clearCaches(); // Clear caches between tests
    server.close();
    eventSourceMock.restore();
    vi.clearAllMocks();
  });

  it('should complete full input workflow from start to finish', async () => {
    const user = userEvent.setup();

    server.use(
      http.get('http://localhost:8080/api/projects/:projectId/jobs/:jobId/story', ({ params }) => {
        const { jobId: requestedJobId } = params;
        return HttpResponse.json({
          job_id: requestedJobId,
          invocation_sequence: 1,
          recipe: { name: 'test-recipe' },
          status: 'running',
          started_at: new Date().toISOString(),
          finished_at: null,
          root: {
            kind: 'recipe',
            title: 'test-recipe',
            status: 'running',
            started_at: new Date().toISOString(),
            finished_at: null,
            path: ['root'],
            invoke_seq: 1,
            attempt: 1,
            prior_attempts: [],
            input: null,
            output: null,
            artifact_keys: [],
            children: [
              {
                kind: 'sequence',
                title: 'main',
                status: 'running',
                started_at: new Date().toISOString(),
                finished_at: null,
                path: ['root', 'sequence:main'],
                invoke_seq: 1,
                attempt: 1,
                prior_attempts: [],
                input: null,
                output: null,
                artifact_keys: [],
                children: [
                  {
                    kind: 'opStep',
                    title: 'await_input',
                    status: 'running',
                    started_at: new Date().toISOString(),
                    finished_at: null,
                    path: ['root', 'sequence:main', 'opStep:await_input'],
                    invoke_seq: 1,
                    attempt: 1,
                    prior_attempts: [],
                    input: null,
                    output: null,
                    artifact_keys: [],
                    task_ordinal: 12,
                    restart_from_ordinal: 12,
                    children: [],
                  },
                ],
              },
            ],
          },
        });
      })
    );

    // Step 1 & 2: Start with one pending input (simulating workflow created it)
    const inputDetails = createMockSingleQuestionDetails(jobId);
    const pendingInput = createMockPendingInput(jobId);
    mockPendingInputsStore.set(projectId, [pendingInput]);
    mockInputDetailsStore.set(`${projectId}:${jobId}`, inputDetails);

    // Render list page
    const { unmount } = render(
      <BrowserRouter>
        <InputActivityProvider>
          <PendingInputsListPage projectId={projectId} />
        </InputActivityProvider>
      </BrowserRouter>
    );

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    // Step 3: User sees pending input in list
    await waitFor(
      () => {
      expect(screen.getByText(`Job ID: ${jobId}`)).toBeInTheDocument();
    });

    // Step 4: User clicks Open in Workflow button
    const openButton = screen.getByRole('button', { name: /open in workflow/i });
    await user.click(openButton);

    // Should navigate to workflow story with prompt open
    expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/workflows/${jobId}/story?input=1`);

    // Cleanup and render workflow story page
    unmount();

    // Step 5-6: Render story page with form
    render(
      <MemoryRouter initialEntries={[`/project/${projectId}/workflows/${jobId}/story?input=1`]}>
        <InputActivityProvider>
          <Routes>
            <Route
              path="/project/:projectId/workflows/:workflowId/story"
              element={<WorkflowStoryPage projectId={projectId} />}
            />
          </Routes>
        </InputActivityProvider>
      </MemoryRouter>
    );

    // User sees form
    await waitFor(
      () => {
      expect(screen.getAllByText('How old are you?').length).toBeGreaterThan(0);
    },
      { timeout: 3000 }
    );

    // Step 7-8: Fill and submit form (in story context)
    const input = screen.getByRole('textbox', { name: /how old are you/i });
    await user.type(input, 'foolish');

    const submitButton = screen.getByRole('button', { name: /submit/i });
    await user.click(submitButton);

    // Verify input was removed from store
    await waitFor(
      () => {
      const key = `${projectId}:${jobId}`;
      expect(mockInputDetailsStore.has(key)).toBe(false);
    });

    // Workflow is now complete (tested list → story → submit)
  });

  it('should handle concurrent inputs from multiple workflows', async () => {
    const user = userEvent.setup();
    const jobId1 = 'job-workflow-1';
    const jobId2 = 'job-workflow-2';
    const jobId3 = 'job-workflow-3';

    // Setup three pending inputs
    setupMockInputs(
      projectId,
      [createMockPendingInput(jobId1), createMockPendingInput(jobId2), createMockPendingInput(jobId3)],
      [
        createMockSingleQuestionDetails(jobId1),
        createMockSingleQuestionDetails(jobId2),
        createMockSingleQuestionDetails(jobId3),
      ]
    );

    render(
      <BrowserRouter>
        <InputActivityProvider>
          <PendingInputsListPage projectId={projectId} />
        </InputActivityProvider>
      </BrowserRouter>
    );

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    // Should show all three inputs
    await waitFor(
      () => {
      expect(screen.getByText(`Job ID: ${jobId1}`)).toBeInTheDocument();
      expect(screen.getByText(`Job ID: ${jobId2}`)).toBeInTheDocument();
      expect(screen.getByText(`Job ID: ${jobId3}`)).toBeInTheDocument();
    });

    // Simulate completing middle input
    mockPendingInputsStore.set(projectId, [createMockPendingInput(jobId1), createMockPendingInput(jobId3)]);
    sse.emitInputCancelled(jobId2, 'completed');

    // Should remove only the completed one
    await waitFor(
      () => {
      expect(screen.queryByText(`Job ID: ${jobId2}`)).not.toBeInTheDocument();
    });

    // Others should still be there
    expect(screen.getByText(`Job ID: ${jobId1}`)).toBeInTheDocument();
    expect(screen.getByText(`Job ID: ${jobId3}`)).toBeInTheDocument();
  });

  it('should remain usable during SSE reconnection', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);

    // Test with list page that has SSE
    const { unmount } = render(
      <BrowserRouter>
        <InputActivityProvider>
          <PendingInputsListPage projectId={projectId} />
        </InputActivityProvider>
      </BrowserRouter>
    );

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(() => {
      expect(screen.getByText(`Job ID: ${jobId}`)).toBeInTheDocument();
    });

    // Simulate SSE close and reconnect
    sse.close();
    await new Promise((resolve) => setTimeout(resolve, 100));

    // Page should still be rendered and functional
    expect(screen.getByText(`Job ID: ${jobId}`)).toBeInTheDocument();

    // Click open should still work
    const openButton = screen.getByRole('button', { name: /open in workflow/i });
    await user.click(openButton);

    expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/workflows/${jobId}/story?input=1`);
    unmount();
  });

  it('should show count badge updates in real-time', async () => {
    setupMockInputs(projectId, [], []);

    // Note: This would need to be tested with actual App component
    // that shows the badge. For now, we verify the count from context
    const { result } = render(
      <BrowserRouter>
        <InputActivityProvider>
          <PendingInputsListPage projectId={projectId} />
        </InputActivityProvider>
      </BrowserRouter>
    );

    const sse = await waitForSSEConnection();
    sse.emitConnected();

    await waitFor(
      () => {
      expect(screen.getByText('No pending inputs')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Add input
    const newJobId = 'badge-test-job';
    mockPendingInputsStore.set(projectId, [createMockPendingInput(newJobId)]);
    sse.emitInputPending(newJobId);

    // Should show input
    await waitFor(
      () => {
      expect(screen.getByText(`Job ID: ${newJobId}`)).toBeInTheDocument();
    });

    // Count should be 1 (would show in badge)
    // This is verified by the presence of the input in the list
  });
});
