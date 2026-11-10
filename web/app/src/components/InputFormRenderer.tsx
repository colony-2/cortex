import { useEffect, useRef, useState } from 'react';
import { 
  Form, 
  Input, 
  Radio, 
  Checkbox, 
  Select, 
  Slider, 
  DatePicker, 
  TimePicker, 
  Upload, 
  Button, 
  Space, 
  Alert,
} from 'antd';
import { UploadOutlined } from '@ant-design/icons';
import type { UploadFile } from 'antd/es/upload/interface';
import type { Rule } from 'antd/es/form';
import type { Dayjs } from 'dayjs';
import { conventionalReviewFeedback, reviewActionField, reviewFeedbackField } from '../utils/reviewForm';
import type { 
  InputForm, 
  InputField, 
  FormResponse,
  FormContext
} from '@colony2/shared';

const { TextArea } = Input;

interface InputFormRendererProps {
  form: InputForm;
  context?: FormContext;
  onSubmit: (response: FormResponse) => void;
  onCancel?: () => void;
  loading?: boolean;
  reviewPromptShown?: boolean;
  submissionError?: string | null;
}

export default function InputFormRenderer({ 
  form, 
  context,
  onSubmit, 
  onCancel, 
  loading = false,
  reviewPromptShown = false,
  submissionError,
}: InputFormRendererProps) {
  // Context is accepted for future use (display/telemetry) even if not rendered currently.
  void context;
  const [antForm] = Form.useForm();
  const [fileList, setFileList] = useState<Record<string, UploadFile[]>>({});
  const [validationError, setValidationError] = useState(false);
  const actionField = reviewActionField(form);
  const feedback = reviewFeedbackField(form);
  const conventional = !!conventionalReviewFeedback(form);
  const [pending, setPending] = useState<string>();
  const actions = useRef<HTMLDivElement>(null);
  const opener = useRef<string>();
  const explicitOK = useRef(false);
  useEffect(() => {
    if (pending === undefined) return;
    if (feedback) antForm.getFieldInstance(feedback.id)?.focus();
    else actions.current?.querySelector<HTMLButtonElement>('[data-confirm]')?.focus();
  }, [pending, feedback?.id, antForm]);
  useEffect(() => {
    if (form.kind !== 'review' || !actions.current) return;
    const root = document.documentElement;
    const measure = () => {
      const viewport = window.visualViewport;
      const bottom = viewport ? Math.max(0, window.innerHeight - viewport.height - viewport.offsetTop) : 0;
      actions.current!.style.maxHeight = `${(viewport?.height ?? window.innerHeight) * 0.75}px`;
      actions.current!.style.bottom = `${bottom}px`;
      root.style.setProperty('--review-action-height', `${actions.current!.getBoundingClientRect().height + bottom}px`);
    };
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(measure);
    observer?.observe(actions.current);
    measure();
    window.addEventListener('resize', measure);
    window.visualViewport?.addEventListener('resize', measure);
    window.visualViewport?.addEventListener('scroll', measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', measure);
      window.visualViewport?.removeEventListener('resize', measure);
      window.visualViewport?.removeEventListener('scroll', measure);
      root.style.removeProperty('--review-action-height');
    };
  }, [form.kind]);

  const handleFinish = (values: any) => {
    if (loading || (actionField && (!explicitOK.current || pending === undefined))) return;
    explicitOK.current = false;
    setValidationError(false);
    // Transform values based on field types
    const transformedValues: Record<string, any> = {};
    const attachments: Record<string, File> = {};
    const review = form.kind === 'review';
    
    form.fields.forEach((field: InputField) => {
      const value = values[field.id];
      
      if (value === undefined || value === null) {
        if (!review) transformedValues[field.id] = null;
        return;
      }
      
      switch (field.type) {
        case 'date':
          transformedValues[field.id] = value ? (value as Dayjs).format('YYYY-MM-DD') : null;
          break;
        case 'time':
          transformedValues[field.id] = value ? (value as Dayjs).format('HH:mm:ss') : null;
          break;
        case 'file_upload':
          if (review) {
            const file = fileList[field.id]?.[0];
            const raw = file?.originFileObj || file;
            if (raw instanceof File) attachments[field.id] = raw;
            break;
          }
          transformedValues[field.id] = fileList[field.id]?.map(file => ({
            name: file.name,
            size: file.size,
            type: file.type,
            uid: file.uid,
          })) || [];
          break;
        case 'checkboxes':
          transformedValues[field.id] = Array.isArray(value) ? value : [];
          break;
        default:
          transformedValues[field.id] = value;
      }
    });
    
    const response: FormResponse = {
      fields: transformedValues,
      submitted_at: new Date().toISOString(),
    };
    if (review) response.attachments = attachments;
    if (form.responseField) {
      response.response = transformedValues[form.responseField];
    }
    
    onSubmit(response);
  };

  const renderField = (field: InputField) => {
    const rules: Rule[] = [
      {
        required: field.required || (conventional && pending === 'revise' && field === feedback),
        ...(field === feedback && ((conventional && pending === 'revise') || field.required) ? { whitespace: true } : {}),
        message: `${field.label} is required`,
      },
    ];
    
    if (field.validation?.pattern) {
      rules.push({
        pattern: new RegExp(field.validation.pattern),
        message: field.validation.message || `Invalid format for ${field.label}`,
      });
    }
    
    switch (field.type) {
      case 'boolean':
        return <Form.Item key={field.id} name={field.id} label={field.label} rules={rules}><Radio.Group options={[{ label: 'Yes', value: true }, { label: 'No', value: false }]} /></Form.Item>;
      case 'short_answer':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
          >
            <Input placeholder={field.placeholder} />
          </Form.Item>
        );
      
      case 'paragraph_text':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
          >
            <TextArea aria-label={field.label}
              rows={4} 
              placeholder={field.placeholder}
              showCount
              maxLength={1000}
            />
          </Form.Item>
        );
      
      case 'multiple_choice':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
          >
            <Radio.Group>
              {field.options?.map((option: string) => (
                <Radio key={option} value={option}>
                  {field.optionLabels?.[option] || option}
                </Radio>
              ))}
            </Radio.Group>
          </Form.Item>
        );
      
      case 'checkboxes':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
          >
            <Checkbox.Group>
              {field.options?.map((option: string) => (
                <Checkbox key={option} value={option}>
                  {field.optionLabels?.[option] || option}
                </Checkbox>
              ))}
            </Checkbox.Group>
          </Form.Item>
        );
      
      case 'dropdown':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
          >
            <Select placeholder={field.placeholder || 'Select an option'}>
              {field.options?.map((option: string) => (
                <Select.Option key={option} value={option}>
                  {field.optionLabels?.[option] || option}
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
        );
      
      case 'linear_scale':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
          >
            <Slider
              min={field.min || 1}
              max={field.max || 10}
              marks={{
                [field.min || 1]: field.min || 1,
                [field.max || 10]: field.max || 10,
              }}
            />
          </Form.Item>
        );
      
      case 'date':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
          >
            <DatePicker 
              style={{ width: '100%' }}
              format="YYYY-MM-DD"
            />
          </Form.Item>
        );
      
      case 'time':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
          >
            <TimePicker 
              style={{ width: '100%' }}
              format="HH:mm"
            />
          </Form.Item>
        );
      
      case 'file_upload':
        return (
          <Form.Item
            key={field.id}
            name={field.id}
            label={field.label}
            rules={rules}
            valuePropName="fileList"
            getValueFromEvent={(e) => {
              if (Array.isArray(e)) {
                return e;
              }
              return e?.fileList;
            }}
          >
            <Upload
              maxCount={form.kind === 'review' ? 1 : undefined}
              beforeUpload={(file) => {
                // Store file locally, don't auto-upload
                const fieldFiles = fileList[field.id] || [];
                setFileList({
                  ...fileList,
                  [field.id]: form.kind === 'review' ? [file] : [...fieldFiles, file],
                });
                return false;
              }}
              onRemove={(file) => {
                const fieldFiles = fileList[field.id] || [];
                setFileList({
                  ...fileList,
                  [field.id]: fieldFiles.filter(f => f.uid !== file.uid),
                });
              }}
              fileList={fileList[field.id] || []}
            >
              <Button icon={<UploadOutlined />}>Click to upload</Button>
            </Upload>
          </Form.Item>
        );
      
      default:
        return null;
    }
  };

  return (
    <Form
      form={antForm}
      layout="vertical"
      onFinish={handleFinish}
      onSubmitCapture={event => {
        if (actionField && !explicitOK.current) { event.preventDefault(); event.stopPropagation(); }
      }}
      onFinishFailed={() => {
        explicitOK.current = false;
        setValidationError(true);
      }}
      scrollToFirstError={form.kind === 'review' ? { block: 'center', focus: true } : true}
      disabled={loading}
    >
      {actionField && !reviewPromptShown && <div style={{ whiteSpace: 'pre-wrap', marginBottom: 24 }}>{actionField.label}</div>}
      {form.description && (
        <div style={{ marginBottom: '24px', color: '#666' }}>
          {form.description}
        </div>
      )}
      
      {form.fields.filter(field => field !== actionField && field !== feedback && !(actionField && form.kind === 'review' && field.type === 'file_upload')).map((field: InputField) => renderField(field))}
      {actionField && <Form.Item name={actionField.id} hidden rules={[{ required: true }]}><Input /></Form.Item>}
      <div ref={actions} role={form.kind === 'review' ? 'region' : undefined} className={form.kind === 'review' ? 'review-actions' : undefined} aria-label={form.kind === 'review' ? 'Review decisions' : undefined}>
      {actionField && <div hidden={pending === undefined}>
        <p aria-live="polite">Selected response: {actionField.optionLabels?.[pending ?? ''] || pending}</p>
        {feedback && renderField(feedback)}
        {form.kind === 'review' && form.fields.filter(field => field.type === 'file_upload').map(renderField)}
      </div>}
      {submissionError && <Alert type="error" showIcon message={submissionError} />}
      {validationError && <Alert type="error" showIcon message="Please complete the required fields and correct any errors before submitting." />}
      
      <Form.Item style={{ marginTop: form.kind === 'review' ? 0 : 32 }}>
        <Space wrap>
          {onCancel && (
            <Button onClick={onCancel} disabled={loading}>
              Cancel
            </Button>
          )}
          {/* Keep action names stable while Ant Design animates its loading icon. */}
          {actionField ? pending !== undefined ? <>
            <Button data-confirm disabled={loading} onClick={() => {
              setPending(undefined); explicitOK.current = false;
              requestAnimationFrame(() => {
                const button = Array.from(actions.current?.querySelectorAll<HTMLButtonElement>('[data-response]') ?? []).find(button => button.dataset.response === opener.current);
                button?.focus();
              });
            }}>Back</Button>
            <Button type="primary" disabled={loading} loading={loading} aria-busy={loading} aria-label="OK" onClick={() => {
              if (loading || explicitOK.current) return;
              explicitOK.current = true; antForm.submit();
            }}>OK</Button>
          </> : actionField.options!.map(option => {
            const label = conventional ? (option === 'approve' ? 'Approve' : 'Reject') : actionField.optionLabels?.[option] || option;
            return <Button key={option} type={option === 'revise' ? 'default' : 'primary'}
              data-response={option} disabled={loading} aria-label={label} onClick={() => {
                opener.current = option;
                antForm.setFieldValue(actionField.id, option);
                setPending(option);
              }}>{label}</Button>;
          }) : <Button type="primary" htmlType="submit" loading={loading} aria-label="Submit" aria-busy={loading}>
            Submit
          </Button>}
        </Space>
      </Form.Item>
      </div>
    </Form>
  );
}
