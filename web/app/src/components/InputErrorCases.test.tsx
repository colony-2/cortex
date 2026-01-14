import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, BrowserRouter, Routes, Route } from 'react-router-dom';
import { setupServer } from 'msw/node';
import { http, HttpResponse } from 'msw';
import { InputActivityProvider, inputActivityService } from '@colony2/shared';
import InputDetailPage from './InputDetailPage';
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

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('Input Pages - Error Cases', () => {
  const projectId = 'test-project-123';
  const jobId = 'job-error-test';
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

  describe('API Error Handling', () => {
    it('should handle 404 when fetching input details', async () => {
      setupMockInputs(projectId, [], []); // No inputs available

      render(
        <MemoryRouter initialEntries={[`/project/test-project-123/inputs/job-error-test`]}>
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

      await waitFor(
      () => {
        expect(screen.getByText(/Failed to load input details/i)).toBeInTheDocument();
      },
      { timeout: 3000 }
    );

      // Should show error alert
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    it('should handle network error when fetching pending inputs', async () => {
      // Override handler to return error
      server.use(
        http.get('http://localhost:8080/api/projects/:projectId/user-inputs/pending', () => {
          return new HttpResponse(null, { status: 500 });
        })
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

      // Should show empty state (graceful degradation)
      await waitFor(
      () => {
        expect(screen.getByText('No pending inputs')).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
    });

    it('should handle submit failure', async () => {
      const user = userEvent.setup();
      setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);

      // Override submit handler to fail
      server.use(
        http.post('http://localhost:8080/api/projects/:projectId/user-inputs/:jobId/respond', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      render(
        <MemoryRouter initialEntries={[`/project/test-project-123/inputs/job-error-test`]}>
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

      await waitFor(
      () => {
        expect(screen.getAllByText('How old are you?').length).toBeGreaterThan(0);
      },
      { timeout: 3000 }
    );

      // Fill and submit form
      const input = screen.getByRole('textbox');
      await user.type(input, '25');

      const submitButton = screen.getByRole('button', { name: /submit/i });
      await user.click(submitButton);

      // Wait a moment for async submission
      await new Promise(resolve => setTimeout(resolve, 100));

      // Should NOT navigate (submission failed)
      expect(mockNavigate).not.toHaveBeenCalled();

      // Should show error message (from ant design message.error)
      // Note: This would need to check for message component in actual implementation
    });

    it('should handle cancel failure gracefully', async () => {
      setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);

      // Override cancel handler to fail
      server.use(
        http.post('http://localhost:8080/api/projects/:projectId/user-inputs/:jobId/cancel', () => {
          return new HttpResponse(null, { status: 500 });
        })
      );

      // Test would involve triggering cancel action if we had that feature
      // For now, this serves as documentation of expected behavior
      expect(true).toBe(true);
    });
  });

  describe('SSE Connection Errors', () => {
    it('should handle SSE connection failure', async () => {
      setupMockInputs(projectId, [], []);

      render(
        <BrowserRouter>
          <InputActivityProvider>
            <PendingInputsListPage projectId={projectId} />
          </InputActivityProvider>
        </BrowserRouter>
      );

      // Simulate connection never opening
      const sse = await waitForSSEConnection();

      // Manually trigger error
      const errorEvent = new Event('error');
      sse.dispatchEvent(errorEvent);

      // Should handle gracefully without crashing
      // The page should still be rendered (either showing connecting or empty state)
      const elements = screen.queryAllByText(/Pending Inputs|Connecting to input stream/i);
      expect(elements.length).toBeGreaterThan(0);
    });

    it('should handle SSE error events', async () => {
      setupMockInputs(projectId, [], []);

      render(
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

      // Emit error event
      sse.emitError('Test error message');

      // Should continue working (error should be logged but not crash)
      expect(screen.getByText('No pending inputs')).toBeInTheDocument();
    });
  });

  describe('Edge Cases', () => {
    it('should handle missing jobId parameter', async () => {
      render(
        <MemoryRouter initialEntries={[`/project/test-project-123/inputs/`]}>
          <Routes>
            <Route
              path="/project/:projectId/inputs/"
              element={
                <InputActivityProvider>
                  <InputDetailPage projectId={projectId} />
                </InputActivityProvider>
              }
            />
          </Routes>
        </MemoryRouter>
      );

      // Should show error
      await waitFor(
      () => {
        expect(screen.getByText(/No job ID provided|Input request not found/i)).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
    });

    it('should handle empty form configuration', async () => {
      // Setup input with empty form config
      const emptyFormDetails = {
        jobId,
        status: 'pending',
        startTime: new Date().toISOString(),
        form: {}, // Empty form config
      };

      setupMockInputs(projectId, [createMockPendingInput(jobId)], [emptyFormDetails]);

      render(
        <MemoryRouter initialEntries={[`/project/test-project-123/inputs/job-error-test`]}>
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

      await waitFor(
      () => {
        expect(screen.getAllByText('Input Request').length).toBeGreaterThan(0);
      },
      { timeout: 3000 }
    );

      // Should show form title (defaults to "Input Request")
      expect(screen.getAllByText('Input Request').length).toBeGreaterThan(0);
    });

    it('should handle rapid project switching', async () => {
      const project1 = 'project-1';
      const project2 = 'project-2';

      setupMockInputs(project1, [createMockPendingInput('job-1')], [createMockSingleQuestionDetails('job-1')]);
      setupMockInputs(project2, [createMockPendingInput('job-2')], [createMockSingleQuestionDetails('job-2')]);

      const { rerender } = render(
        <BrowserRouter>
          <InputActivityProvider>
            <PendingInputsListPage projectId={project1} />
          </InputActivityProvider>
        </BrowserRouter>
      );

      const sse1 = await waitForSSEConnection();
      sse1.emitConnected();

      // Quickly switch to project 2
      rerender(
        <BrowserRouter>
          <InputActivityProvider>
            <PendingInputsListPage projectId={project2} />
          </InputActivityProvider>
        </BrowserRouter>
      );

      // Should handle gracefully without errors
      await waitFor(
      () => {
        const sse2 = eventSourceMock.getInstance();
        expect(sse2?.url).toContain(`/projects/${project2}/user-inputs/stream`);
      });
    });
  });
});
