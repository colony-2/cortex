import { useState } from 'react';
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
} from 'antd';
import { UploadOutlined } from '@ant-design/icons';
import type { UploadFile } from 'antd/es/upload/interface';
import type { Rule } from 'antd/es/form';
import type { Dayjs } from 'dayjs';
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
}

export default function InputFormRenderer({ 
  form, 
  context,
  onSubmit, 
  onCancel, 
  loading = false 
}: InputFormRendererProps) {
  // Context is accepted for future use (display/telemetry) even if not rendered currently.
  void context;
  const [antForm] = Form.useForm();
  const [fileList, setFileList] = useState<Record<string, UploadFile[]>>({});

  const handleFinish = (values: any) => {
    // Transform values based on field types
    const transformedValues: Record<string, any> = {};
    
    form.fields.forEach((field: InputField) => {
      const value = values[field.id];
      
      if (value === undefined || value === null) {
        transformedValues[field.id] = null;
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
    if (form.responseField) {
      response.response = transformedValues[form.responseField];
    }
    
    onSubmit(response);
  };

  const renderField = (field: InputField) => {
    const rules: Rule[] = [
      {
        required: field.required,
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
            <TextArea 
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
                  {option}
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
                  {option}
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
                  {option}
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
              beforeUpload={(file) => {
                // Store file locally, don't auto-upload
                const fieldFiles = fileList[field.id] || [];
                setFileList({
                  ...fileList,
                  [field.id]: [...fieldFiles, file],
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
      disabled={loading}
    >
      {form.description && (
        <div style={{ marginBottom: '24px', color: '#666' }}>
          {form.description}
        </div>
      )}
      
      {form.fields.map((field: InputField) => renderField(field))}
      
      <Form.Item style={{ marginTop: '32px' }}>
        <Space>
          {onCancel && (
            <Button onClick={onCancel} disabled={loading}>
              Cancel
            </Button>
          )}
          <Button type="primary" htmlType="submit" loading={loading}>
            Submit
          </Button>
        </Space>
      </Form.Item>
    </Form>
  );
}
