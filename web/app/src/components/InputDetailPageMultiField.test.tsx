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
  createMockMultiFieldDetails,
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

describe('InputDetailPage - Multi-field Forms', () => {
  const projectId = 'test-project-123';
  const jobId = 'job-multifield-789';
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

  it('should display multi-field form title', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Deployment Approval')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );
  });

  it('should render all form fields', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Deployment Approval')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Check all field labels are present
    expect(screen.getByText('Select environment')).toBeInTheDocument();
    expect(screen.getByText('Approver name')).toBeInTheDocument();
    expect(screen.getByText('Additional notes')).toBeInTheDocument();
  });

  it('should render dropdown field with options', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    const user = userEvent.setup();
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Select environment')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Find and click dropdown
    const dropdown = screen.getByRole('combobox', { name: /select environment/i });
    expect(dropdown).toBeInTheDocument();
  });

  it('should render text input fields', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Approver name')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Should have text input for approver
    const approverInput = screen.getByRole('textbox', { name: /approver name/i });
    expect(approverInput).toBeInTheDocument();
  });

  it('should render textarea for paragraph text field', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Additional notes')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Should have textarea for notes
    const notesInput = screen.getByRole('textbox', { name: /additional notes/i });
    expect(notesInput).toBeInTheDocument();
  });

  it('should submit multi-field form with all values', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Deployment Approval')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Fill text fields first
    const approverInput = screen.getByRole('textbox', { name: /approver name/i });
    await user.type(approverInput, 'John Doe');

    const notesInput = screen.getByRole('textbox', { name: /additional notes/i });
    await user.type(notesInput, 'Approved for deployment');

    // For dropdown - click to open
    const dropdown = screen.getByRole('combobox', { name: /select environment/i });
    await user.click(dropdown);

    // Wait for dropdown menu to render (Ant Design renders in body)
    await waitFor(async () => {
      const optionElements = document.querySelectorAll('.ant-select-item-option');
      expect(optionElements.length).toBeGreaterThan(0);
    }, { timeout: 2000 });

    // Find and click the production option
    const optionElements = document.querySelectorAll('.ant-select-item-option');
    // Click the second option (production)
    if (optionElements[1]) {
      await user.click(optionElements[1] as Element);
    }

    // Submit form
    const submitButton = screen.getByRole('button', { name: /submit/i });
    await user.click(submitButton);

    // Should navigate to workflow details
    await waitFor(
      () => {
      expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/workflows/${jobId}`);
    });
  });

  it('should handle required field validation', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Deployment Approval')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Try to submit without filling required fields
    const submitButton = screen.getByRole('button', { name: /submit/i });
    await user.click(submitButton);

    // Should show validation error (depends on form implementation)
    // At minimum, should not navigate
    await waitFor(
      () => {
      expect(mockNavigate).not.toHaveBeenCalled();
    },
      { timeout: 3000 }
    );
  });

  it('should allow optional fields to be empty', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Deployment Approval')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Fill required approver name
    const approverInput = screen.getByRole('textbox', { name: /approver name/i });
    await user.type(approverInput, 'Jane Smith');

    // Leave notes empty (it's optional)

    // Fill dropdown - click to open
    const dropdown = screen.getByRole('combobox', { name: /select environment/i });
    await user.click(dropdown);

    // Wait for dropdown menu to render (Ant Design renders in body)
    await waitFor(async () => {
      const optionElements = document.querySelectorAll('.ant-select-item-option');
      expect(optionElements.length).toBeGreaterThan(0);
    }, { timeout: 2000 });

    // Find and click the staging option
    const optionElements = document.querySelectorAll('.ant-select-item-option');
    // Click the first option (staging)
    if (optionElements[0]) {
      await user.click(optionElements[0] as Element);
    }

    // Submit should work
    const submitButton = screen.getByRole('button', { name: /submit/i });
    await user.click(submitButton);

    await waitFor(
      () => {
      expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/workflows/${jobId}`);
    });
  });

  it('should display form field count', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockMultiFieldDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Deployment Approval')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    // Should have 3 fields total
    const labels = screen.getAllByText(/select environment|approver name|additional notes/i);
    expect(labels).toHaveLength(3);
  });
});
