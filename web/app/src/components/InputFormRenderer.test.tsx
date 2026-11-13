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

  it.each([
    { id: 'decision', options: ['revise'], value: 'revise', label: 'Revise the plan' },
    { id: 'decision', options: ['approve', 'revise', 'defer'], value: 'revise', label: 'Revise the plan' },
    { id: 'disposition', options: ['accept', 'return'], value: 'return', label: 'Send back for discussion' },
  ])('submits review action $id=$value with exact values', async ({ id, options, value, label }) => {
    const form: InputForm = { id: 'review', title: 'Review', kind: 'review', fields: [
      { id, type: 'multiple_choice', required: true, label: 'Agent summary and open questions',
        options, optionLabels: { [value]: label } },
      { id: 'feedback', type: 'paragraph_text', required: true, label: 'Feedback' },
    ] };
    render(<InputFormRenderer form={form} onSubmit={mockOnSubmit} />);
    expect(screen.queryByRole('radio')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Submit' })).toBeNull();
    expect(screen.getByText('Agent summary and open questions')).toBeInTheDocument();
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: label }));
    expect(mockOnSubmit).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'OK', exact: true }));
    await screen.findByText('Feedback is required');
    expect(mockOnSubmit).not.toHaveBeenCalled();
    await user.type(screen.getByLabelText('Feedback'), 'Please clarify ownership.');
    await user.click(screen.getByRole('button', { name: 'OK', exact: true }));
    await waitFor(() => expect(mockOnSubmit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      fields: { [id]: value, feedback: 'Please clarify ownership.' },
    })));
  });

  it.each([true, false])('REV-006/007 retains rejection draft and sends revise (required=%s)', async required => {
    const user = userEvent.setup();
    render(<InputFormRenderer form={{ id: 'review', title: 'Review', kind: 'review', fields: [
      { id: 'decision', label: 'Decision', type: 'multiple_choice', required: true, options: ['approve', 'revise'] },
      { id: 'feedback', label: 'Reason', type: 'paragraph_text', required },
    ] }} onSubmit={mockOnSubmit} />);
    await user.click(screen.getByRole('button', { name: 'Reject', exact: true }));
    expect(screen.getByLabelText('Reason')).toHaveFocus();
    expect(mockOnSubmit).not.toHaveBeenCalled();
    await user.type(screen.getByLabelText('Reason'), '   ');
    await user.click(screen.getByRole('button', { name: 'OK' }));
    await screen.findByText('Reason is required');
    expect(mockOnSubmit).not.toHaveBeenCalled();
    await user.clear(screen.getByLabelText('Reason'));
    await user.type(screen.getByLabelText('Reason'), 'Please clarify');
    await user.click(screen.getByRole('button', { name: 'Back', exact: true }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Reject', exact: true })).toHaveFocus());
    await user.click(screen.getByRole('button', { name: 'Reject', exact: true }));
    expect(screen.getByLabelText('Reason')).toHaveValue('Please clarify');
    await user.click(screen.getByRole('button', { name: 'OK' }));
    await waitFor(() => expect(mockOnSubmit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ fields: { decision: 'revise', feedback: 'Please clarify' } })));
  });


  it('RC-002/008/010/011 preserves answers and files through Back, validates patterns, and sends only on OK', async () => {
    const user = userEvent.setup();
    const { container } = render(<InputFormRenderer form={{id:'custom',title:'Custom review',kind:'review',responseField:'choice',fields:[
      {id:'choice',label:'Choose',type:'multiple_choice',required:true,options:['accept','return'],optionLabels:{accept:'Accept',return:'Return'}},
      {id:'feedback',label:'Explain the choice',type:'paragraph_text',required:true,validation:{pattern:'^Reason',message:'Start with Reason'}},
      {id:'owner',label:'Owner',type:'short_answer',required:true},
      {id:'file',label:'Attachment',type:'file_upload',required:false},
    ]}} onSubmit={mockOnSubmit} />);
    expect(screen.queryByText('Why?')).toBeNull();
    await user.type(screen.getByLabelText('Owner'),'Sam');
    const file = new File(['evidence'],'evidence.txt',{type:'text/plain'});
    await user.click(screen.getByRole('button',{name:'Accept',exact:true}));
    const editor = screen.getByLabelText('Explain the choice');
    expect(editor).toHaveFocus();
    await user.upload(container.querySelector('input[type=file]')!,file);
    expect(container.querySelectorAll('textarea')).toHaveLength(1);
    expect(editor).not.toHaveAttribute('maxlength');
    const invalid = 'Invalid\ndetails '.repeat(150) + '終わり';
    await user.click(editor);
    await user.paste(invalid);
    expect(editor).toHaveValue(invalid);
    expect(mockOnSubmit).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button',{name:'OK',exact:true}));
    await screen.findByText('Start with Reason');
    expect(screen.getByText('Selected response: Accept')).toBeVisible();
    await user.click(screen.getByRole('button',{name:'Back',exact:true}));
    await waitFor(()=>expect(screen.getByRole('button',{name:'Accept',exact:true})).toHaveFocus());
    await user.click(screen.getByRole('button',{name:'Return',exact:true}));
    expect(editor).toHaveValue(invalid);
    expect(screen.getByLabelText('Owner')).toHaveValue('Sam');
    expect(screen.getByText('evidence.txt')).toBeVisible();
    await user.clear(editor);
    const valid = 'Reason: ownership\n' + 'Please clarify café ownership. '.repeat(70) + '終わり';
    await user.paste(valid);
    await user.click(screen.getByRole('button',{name:'OK',exact:true}));
    await waitFor(()=>expect(mockOnSubmit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      fields:{choice:'return',feedback:valid,owner:'Sam'},response:'return',attachments:{file},
    })));
  });

  it('edits and counts feedback beyond 1,000 characters and submits only on explicit OK', async () => {
    const user = userEvent.setup();
    const { container } = render(<InputFormRenderer form={{ id: 'long-review', title: 'Review', kind: 'review', fields: [
      { id: 'decision', label: 'Decision', type: 'multiple_choice', required: true, options: ['approve', 'revise'] },
      { id: 'feedback', label: 'Feedback', type: 'paragraph_text', required: true },
    ] }} onSubmit={mockOnSubmit} />);
    await user.click(screen.getByRole('button', { name: 'Approve', exact: true }));
    const editor = screen.getByLabelText('Feedback');
    expect(editor).not.toHaveAttribute('maxlength');
    await user.paste('x'.repeat(995));
    await user.type(editor, ' past the boundary{Enter}');
    const pasted = 'Café — 日本語 🙂\n'.repeat(80) + 'Unique ending';
    await user.paste(pasted);
    const complete = 'x'.repeat(995) + ' past the boundary\n' + pasted;
    expect(editor).toHaveValue(complete);
    expect(container.querySelector('.ant-input-data-count')).toHaveTextContent(String(complete.length));
    expect(container.querySelector('.ant-input-data-count')?.textContent).not.toContain('/');
    await user.keyboard('{Backspace}');
    const edited = complete.slice(0, -1);
    expect(editor).toHaveValue(edited);
    expect(container.querySelector('.ant-input-data-count')?.textContent).toBe(String(edited.length));
    fireEvent.submit(container.querySelector('form')!);
    await user.click(screen.getByRole('button', { name: 'Back', exact: true }));
    expect(mockOnSubmit).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Reject', exact: true }));
    expect(editor).toHaveValue(edited);
    await user.click(screen.getByRole('button', { name: 'OK', exact: true }));
    await waitFor(() => expect(mockOnSubmit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      fields: { decision: 'revise', feedback: edited },
    })));
  });

  it('submits a long ordinary paragraph with another field ID intact', async () => {
    const user = userEvent.setup();
    render(<InputFormRenderer form={{ id: 'ordinary', title: 'Notes', fields: [
      { id: 'notes', label: 'Notes', type: 'paragraph_text', required: true },
    ] }} onSubmit={mockOnSubmit} />);
    const editor = screen.getByLabelText('Notes');
    expect(editor).not.toHaveAttribute('maxlength');
    const notes = 'Long ordinary paragraph.\n'.repeat(90) + 'Exact ending 日本語';
    await user.click(editor);
    await user.paste(notes);
    await user.click(screen.getByRole('button', { name: 'Submit', exact: true }));
    await waitFor(() => expect(mockOnSubmit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ fields: { notes } })));
  });

  it.each([false,true])('RC-005 confirms without paragraph feedback (other field=%s)',async other => {
    const user = userEvent.setup();
    render(<InputFormRenderer form={{id:'plain',title:'Review',kind:'review',fields:[
      {id:'decision',label:'Proceed?',type:'multiple_choice',required:true,options:['go']},
      ...(other ? [{id:'notes',label:'Notes',type:'paragraph_text' as const,required:false}] : []),
    ]}} onSubmit={mockOnSubmit}/>);
    if(other) await user.type(screen.getByLabelText('Notes'),'On page');
    await user.click(screen.getByRole('button',{name:'go',exact:true}));
    expect(screen.queryByText('Why?')).toBeNull();
    expect(screen.getByRole('button',{name:'Back',exact:true})).toHaveFocus();
    expect(mockOnSubmit).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button',{name:'OK',exact:true}));
    await waitFor(()=>expect(mockOnSubmit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({fields:{decision:'go',...(other?{notes:'On page'}:{})}})));
  });

  it('RC-007 optional approval feedback allows blank and implicit form submission sends nothing',async()=>{
    const user=userEvent.setup();
    const {container}=render(<InputFormRenderer form={{id:'approval',title:'Review',kind:'review',fields:[
      {id:'decision',label:'Decision',type:'multiple_choice',required:true,options:['approve','revise']},
      {id:'feedback',label:'Reason',type:'paragraph_text',required:false},
    ]}} onSubmit={mockOnSubmit}/>);
    fireEvent.submit(container.querySelector('form')!);
    await user.click(screen.getByRole('button',{name:'Approve',exact:true}));
    fireEvent.submit(container.querySelector('form')!);
    expect(mockOnSubmit).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button',{name:'OK',exact:true}));
    await waitFor(()=>expect(mockOnSubmit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({fields:{decision:'approve'}})));
  });

  it.each(['optional', 'multiple'])('keeps %s review choice questions as ordinary form fields', async kind => {
    const field = { id: 'choice', type: 'multiple_choice' as const, required: kind !== 'optional',
      label: 'Which version?', options: ['A', 'B'] };
    render(<InputFormRenderer form={{ id: 'review', title: 'Review', kind: 'review', fields: kind === 'optional'
      ? [field] : [field, { ...field, id: 'second', label: 'Which environment?' }],
    }} onSubmit={mockOnSubmit} />);
    expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();
    expect(screen.getAllByRole('radio')).toHaveLength(kind === 'optional' ? 2 : 4);
    if (kind === 'optional') {
      await userEvent.click(screen.getByRole('button', { name: 'Submit' }));
      await waitFor(() => expect(mockOnSubmit).toHaveBeenCalledWith(expect.objectContaining({ fields: {} })));
    }
  });

  it('keeps the review action accessible by the same name while submitting and after failure', () => {
    const form: InputForm = { id: 'review', title: 'Review', kind: 'review', fields: [
      { id: 'decision', type: 'multiple_choice', required: true, label: 'Proceed?',
        options: ['revise'], optionLabels: { revise: 'Revise the plan' } },
    ] };
    const { rerender } = render(<InputFormRenderer form={form} onSubmit={mockOnSubmit} loading />);
    expect(screen.getByRole('button', { name: 'Revise the plan', exact: true })).toBeDisabled();
    rerender(<InputFormRenderer form={form} onSubmit={mockOnSubmit} loading={false} />);
    expect(screen.getByRole('button', { name: 'Revise the plan', exact: true })).toBeEnabled();
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

it('RM-009/010/011 validates required uploads inside confirmation and retains their payload across Back', async () => {
  const user = userEvent.setup();
  const submit = vi.fn();
  const {container} = render(<InputFormRenderer form={{id:'required-upload',title:'Review',kind:'review',responseField:'decision',fields:[
    {id:'decision',label:'Choose',type:'multiple_choice',required:true,options:['approve','revise'],optionLabels:{approve:'Approve',revise:'Revise'}},
    {id:'attachment',label:'Evidence',type:'file_upload',required:true},
  ]}} onSubmit={submit} />);
  await user.click(screen.getByRole('button',{name:'Approve',exact:true}));
  expect(submit).not.toHaveBeenCalled();
  expect(container.querySelector('textarea')).toBeNull();
  await user.click(screen.getByRole('button',{name:'OK',exact:true}));
  await screen.findByText('Evidence is required');
  expect(submit).not.toHaveBeenCalled();
  const file = new File(['evidence'],'evidence.md',{type:'text/markdown'});
  await user.upload(container.querySelector('input[type=file]')!,file);
  await user.click(screen.getByRole('button',{name:'Back',exact:true}));
  await user.click(screen.getByRole('button',{name:'Approve',exact:true}));
  expect(screen.getByText('evidence.md')).toBeVisible();
  await user.click(screen.getByRole('button',{name:'OK',exact:true}));
  await waitFor(()=>expect(submit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({response:'approve',fields:{decision:'approve'},attachments:{attachment:file}})));
});
