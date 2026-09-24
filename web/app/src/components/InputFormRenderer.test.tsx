import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InputFormRenderer from './InputFormRenderer';
import type { InputForm, FormContext } from '@colony2/shared';

describe('InputFormRenderer', () => {
  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();

  const basicForm: InputForm = {
    id: 'form1',
    title: 'Test Form',
    description: 'This is a test form',
    fields: [
      {
        id: 'field1',
        label: 'Short Answer',
        type: 'short_answer',
        required: true,
        placeholder: 'Enter text',
      },
    ],
  };

  const context: FormContext = {
    jobName: 'Test Job',
    activityName: 'Test Activity',
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it.each([true, false])('preserves c2j response semantics (single question: %s)', async (singleQuestion) => {
    render(<InputFormRenderer form={{
      id: 'response-contract',
      title: 'Approval',
      responseField: singleQuestion ? 'response' : undefined,
      fields: [{ id: 'response', label: 'Answer', type: 'short_answer', required: true }],
    }} onSubmit={mockOnSubmit} />);
    const user = userEvent.setup();
    await user.type(screen.getByLabelText('Answer'), 'approved');
    await user.click(screen.getByRole('button', { name: 'Submit', exact: true }));
    await waitFor(() => expect(mockOnSubmit).toHaveBeenCalledTimes(1));
    const response = mockOnSubmit.mock.calls[0][0];
    expect(response.fields).toEqual({ response: 'approved' });
    expect(response.response).toBe(singleQuestion ? 'approved' : undefined);
  });

  it('should render form description when provided', () => {
    render(
      <InputFormRenderer
        form={basicForm}
        context={context}
        onSubmit={mockOnSubmit}
      />
    );

    expect(screen.getByText('This is a test form')).toBeDefined();
  });

  it('should render all field types correctly', () => {
    const complexForm: InputForm = {
      id: 'form2',
      title: 'Complex Form',
      fields: [
        {
          id: 'shortAnswer',
          label: 'Short Answer',
          type: 'short_answer',
          required: false,
        },
        {
          id: 'paragraph',
          label: 'Paragraph',
          type: 'paragraph_text',
          required: false,
        },
        {
          id: 'multipleChoice',
          label: 'Multiple Choice',
          type: 'multiple_choice',
          required: false,
          options: ['Option 1', 'Option 2'],
        },
        {
          id: 'checkboxes',
          label: 'Checkboxes',
          type: 'checkboxes',
          required: false,
          options: ['Check 1', 'Check 2'],
        },
        {
          id: 'dropdown',
          label: 'Dropdown',
          type: 'dropdown',
          required: false,
          options: ['Drop 1', 'Drop 2'],
        },
        {
          id: 'scale',
          label: 'Linear Scale',
          type: 'linear_scale',
          required: false,
          min: 1,
          max: 5,
        },
        {
          id: 'date',
          label: 'Date',
          type: 'date',
          required: false,
        },
        {
          id: 'time',
          label: 'Time',
          type: 'time',
          required: false,
        },
        {
          id: 'file',
          label: 'File Upload',
          type: 'file_upload',
          required: false,
        },
      ],
    };

    render(
      <InputFormRenderer
        form={complexForm}
        context={context}
        onSubmit={mockOnSubmit}
      />
    );

    // Check that all field labels are rendered
    expect(screen.getByText('Short Answer')).toBeDefined();
    expect(screen.getByText('Paragraph')).toBeDefined();
    expect(screen.getByText('Multiple Choice')).toBeDefined();
    expect(screen.getByText('Checkboxes')).toBeDefined();
    expect(screen.getByText('Dropdown')).toBeDefined();
    expect(screen.getByText('Linear Scale')).toBeDefined();
    expect(screen.getByText('Date')).toBeDefined();
    expect(screen.getByText('Time')).toBeDefined();
    expect(screen.getByText('File Upload')).toBeDefined();
  });

  it('should validate required fields', async () => {
    const { container } = render(
      <InputFormRenderer
        form={basicForm}
        context={context}
        onSubmit={mockOnSubmit}
      />
    );

    // Find the submit button within this specific form
    const form = container.querySelector('form');
    const submitButton = form?.querySelector('button[type="submit"]');
    
    if (submitButton) {
      fireEvent.click(submitButton);
    }

    await waitFor(() => {
      expect(screen.getByText('Short Answer is required')).toBeDefined();
    });

    expect(mockOnSubmit).not.toHaveBeenCalled();
  });

  it('should validate pattern when provided', async () => {
    const formWithValidation: InputForm = {
      id: 'form3',
      title: 'Validation Form',
      fields: [
        {
          id: 'email',
          label: 'Email',
          type: 'short_answer',
          required: true,
          validation: {
            pattern: '^[^@]+@[^@]+\\.[^@]+$',
            message: 'Please enter a valid email',
          },
        },
      ],
    };

    const { container } = render(
      <InputFormRenderer
        form={formWithValidation}
        context={context}
        onSubmit={mockOnSubmit}
      />
    );

    const input = container.querySelector('input#email');
    if (input) {
      await userEvent.type(input, 'invalid-email');
    }

    const submitButton = container.querySelector('button[type="submit"]');
    if (submitButton) {
      fireEvent.click(submitButton);
    }

    await waitFor(() => {
      expect(screen.getByText('Please enter a valid email')).toBeDefined();
    });
  });

  it('should call onSubmit with transformed values', async () => {
    const { container } = render(
      <InputFormRenderer
        form={basicForm}
        context={context}
        onSubmit={mockOnSubmit}
      />
    );

    const input = container.querySelector('input#field1');
    if (input) {
      await userEvent.type(input, 'Test value');
    }

    const submitButton = container.querySelector('button[type="submit"]');
    if (submitButton) {
      fireEvent.click(submitButton);
    }

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          fields: {
            field1: 'Test value',
          },
          submitted_at: expect.any(String),
        })
      );
    });
  });

  it('should call onCancel when cancel button is clicked', () => {
    render(
      <InputFormRenderer
        form={basicForm}
        context={context}
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelButton);

    expect(mockOnCancel).toHaveBeenCalled();
  });

  it('should disable form when loading', () => {
    const { container } = render(
      <InputFormRenderer
        form={basicForm}
        context={context}
        onSubmit={mockOnSubmit}
        loading={true}
      />
    );

    const input = container.querySelector('input#field1');
    expect(input).toHaveProperty('disabled', true);

    const submitButton = container.querySelector('button[type="submit"]');
    expect(submitButton).toHaveProperty('disabled', true);
  });

  it('should handle multiple choice selection', async () => {
    const formWithChoice: InputForm = {
      id: 'form4',
      title: 'Choice Form',
      fields: [
        {
          id: 'choice',
          label: 'Choose One',
          type: 'multiple_choice',
          required: true,
          options: ['Option A', 'Option B'],
        },
      ],
    };

    const { container } = render(
      <InputFormRenderer
        form={formWithChoice}
        context={context}
        onSubmit={mockOnSubmit}
      />
    );

    const optionA = screen.getByLabelText('Option A');
    fireEvent.click(optionA);

    const submitButton = container.querySelector('button[type="submit"]');
    if (submitButton) {
      fireEvent.click(submitButton);
    }

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          fields: {
            choice: 'Option A',
          },
        })
      );
    });
  });

  it('should handle checkbox selections', async () => {
    const formWithCheckboxes: InputForm = {
      id: 'form5',
      title: 'Checkbox Form',
      fields: [
        {
          id: 'checks',
          label: 'Select Multiple',
          type: 'checkboxes',
          required: false,
          options: ['Check A', 'Check B', 'Check C'],
        },
      ],
    };

    const { container } = render(
      <InputFormRenderer
        form={formWithCheckboxes}
        context={context}
        onSubmit={mockOnSubmit}
      />
    );

    const checkA = screen.getByLabelText('Check A');
    const checkC = screen.getByLabelText('Check C');
    
    fireEvent.click(checkA);
    fireEvent.click(checkC);

    const submitButton = container.querySelector('button[type="submit"]');
    if (submitButton) {
      fireEvent.click(submitButton);
    }

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          fields: {
            checks: ['Check A', 'Check C'],
          },
        })
      );
    });
  });

  it('should not show cancel button when onCancel is not provided', () => {
    const { container } = render(
      <InputFormRenderer
        form={basicForm}
        context={context}
        onSubmit={mockOnSubmit}
      />
    );

    const cancelButton = container.querySelector('button[type="button"]');
    expect(cancelButton).toBeNull();
  });
});
