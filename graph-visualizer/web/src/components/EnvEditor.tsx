import { useState, useEffect } from 'react';
import { Button, Space, message, Spin, Alert, Card, Empty, Modal, Radio, Tooltip } from 'antd';
import { 
  PlayCircleOutlined, 
  ReloadOutlined, 
  StopOutlined, 
  DeleteOutlined, 
  EditOutlined, 
  SaveOutlined, 
  CloseOutlined, 
  FileAddOutlined,
  AppstoreOutlined,
  CodeOutlined
} from '@ant-design/icons';
import Editor from '@monaco-editor/react';
import Form from '@rjsf/core';
import validator from '@rjsf/validator-ajv8';
import type { RJSFSchema } from '@rjsf/utils';
import type { DependencyNode } from '../types';
import { ConfigProvider, theme } from 'antd';
import { 
  Input, 
  Select, 
  Switch, 
  InputNumber, 
  Form as AntForm
} from 'antd';

// Custom Ant Design widgets for RJSF
const AntDTextWidget = (props: any) => {
  return (
    <Input 
      value={props.value || ''} 
      onChange={(e) => props.onChange(e.target.value)}
      placeholder={props.placeholder}
      disabled={props.disabled}
    />
  );
};

const AntDSelectWidget = (props: any) => {
  const { enumOptions } = props.options;
  return (
    <Select
      value={props.value}
      onChange={props.onChange}
      disabled={props.disabled}
      style={{ width: '100%' }}
    >
      {enumOptions?.map((option: any) => (
        <Select.Option key={option.value} value={option.value}>
          {option.label}
        </Select.Option>
      ))}
    </Select>
  );
};

const AntDBooleanWidget = (props: any) => {
  return (
    <Switch
      checked={props.value}
      onChange={props.onChange}
      disabled={props.disabled}
    />
  );
};

const AntDNumberWidget = (props: any) => {
  return (
    <InputNumber
      value={props.value}
      onChange={props.onChange}
      disabled={props.disabled}
      style={{ width: '100%' }}
    />
  );
};

// Custom field template for Ant Design styling
const CustomFieldTemplate = (props: any) => {
  const { label, help, required, description, errors, children } = props;
  return (
    <AntForm.Item
      label={label}
      required={required}
      help={help || description}
      validateStatus={errors && errors.length > 0 ? 'error' : ''}
      extra={errors}
    >
      {children}
    </AntForm.Item>
  );
};

const widgets = {
  TextWidget: AntDTextWidget,
  SelectWidget: AntDSelectWidget,
  CheckboxWidget: AntDBooleanWidget,
  NumberWidget: AntDNumberWidget,
};

// JSON Schema for devcontainer configuration
const devcontainerSchema: RJSFSchema = {
  type: 'object',
  properties: {
    name: {
      type: 'string',
      title: 'Container Name',
      description: 'A human-readable name for the dev container',
    },
    image: {
      type: 'string',
      title: 'Base Image',
      description: 'Docker image to use as the base',
      default: 'mcr.microsoft.com/devcontainers/base:ubuntu',
    },
    features: {
      type: 'object',
      title: 'Features',
      description: 'Dev container features to install',
      additionalProperties: true,
    },
    forwardPorts: {
      type: 'array',
      title: 'Forward Ports',
      items: {
        type: 'number',
      },
      description: 'Ports to forward from the container to the host',
    },
    postCreateCommand: {
      type: 'string',
      title: 'Post Create Command',
      description: 'Command to run after creating the container',
    },
    customizations: {
      type: 'object',
      title: 'Customizations',
      properties: {
        vscode: {
          type: 'object',
          title: 'VS Code',
          properties: {
            extensions: {
              type: 'array',
              title: 'Extensions',
              items: {
                type: 'string',
              },
              description: 'VS Code extensions to install',
            },
          },
        },
      },
    },
  },
  required: ['name', 'image'],
};

interface EnvEditorProps {
  node: DependencyNode;
}

export default function EnvEditor({ node }: EnvEditorProps) {
  const [content, setContent] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [originalContent, setOriginalContent] = useState<string>('');
  const [containerStatus, setContainerStatus] = useState<'stopped' | 'running' | 'none'>('none');
  const [containerId, setContainerId] = useState<string | null>(null);
  const [fileExists, setFileExists] = useState(false);
  const [viewMode, setViewMode] = useState<'gui' | 'ide'>('gui');
  const [formData, setFormData] = useState<any>({});

  // Helper function to show detailed error messages
  const showError = (title: string, error: any) => {
    let errorMessage = error.message || 'An unknown error occurred';
    let errorDetails = '';
    
    // Extract the actual error message from the format "Failed to create container: error message"
    const match = errorMessage.match(/Failed to \w+ container: (.+)/);
    if (match) {
      errorMessage = match[1];
    }
    
    // Get error details for expandable section
    if (error.stack) {
      errorDetails = error.stack;
    } else if (error.toString() !== errorMessage) {
      errorDetails = error.toString();
    }
    
    Modal.error({
      title: title,
      content: (
        <div>
          <p>{errorMessage}</p>
          {errorDetails && (
            <details style={{ marginTop: '10px' }}>
              <summary style={{ cursor: 'pointer', color: '#1890ff', userSelect: 'none' }}>
                Show technical details
              </summary>
              <pre style={{ 
                marginTop: '10px', 
                padding: '10px', 
                backgroundColor: '#f5f5f5',
                borderRadius: '4px',
                fontSize: '12px',
                overflow: 'auto',
                maxHeight: '300px',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word'
              }}>
                {errorDetails}
              </pre>
            </details>
          )}
        </div>
      ),
      width: 600,
    });
  };

  // Parse JSON content to form data
  const parseJsonToFormData = (json: string) => {
    try {
      const parsed = JSON.parse(json);
      setFormData(parsed);
      return true;
    } catch (error) {
      return false;
    }
  };

  // Convert form data to JSON string
  const formDataToJson = (data: any) => {
    return JSON.stringify(data, null, 2);
  };

  // Load devcontainer.json file
  useEffect(() => {
    loadDevcontainerFile();
    checkContainerStatus();
  }, [node]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadDevcontainerFile = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/files/.devcontainer/devcontainer.json`);
      if (response.ok) {
        const data = await response.text();
        setContent(data);
        setOriginalContent(data);
        setFileExists(true);
        parseJsonToFormData(data);
      } else if (response.status === 404) {
        // File doesn't exist
        setFileExists(false);
        setContent('');
        setOriginalContent('');
        setFormData({});
      } else {
        const errorText = await response.text();
        throw new Error(`HTTP ${response.status}: ${errorText}`);
      }
    } catch (error: any) {
      console.error('Error loading devcontainer.json:', error);
      showError('Failed to load devcontainer configuration', error);
    } finally {
      setLoading(false);
    }
  };

  const checkContainerStatus = async () => {
    try {
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/container/status`);
      if (response.ok) {
        const data = await response.json();
        setContainerStatus(data.status);
        setContainerId(data.containerId);
      }
    } catch (error) {
      console.error('Error checking container status:', error);
    }
  };

  const createDevcontainerFile = () => {
    const defaultConfig = {
      name: node.name || "Dev Container",
      image: "mcr.microsoft.com/devcontainers/base:ubuntu",
      features: {},
      customizations: {
        vscode: {
          extensions: []
        }
      },
      forwardPorts: [],
      postCreateCommand: ""
    };
    const defaultContent = JSON.stringify(defaultConfig, null, 2);
    setContent(defaultContent);
    setFormData(defaultConfig);
    setOriginalContent('');
    setEditMode(true);
  };

  const saveDevcontainerFile = async () => {
    // If in GUI mode, convert form data to JSON
    if (viewMode === 'gui') {
      setContent(formDataToJson(formData));
    }
    
    setLoading(true);
    try {
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/files/.devcontainer/devcontainer.json`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ content: viewMode === 'gui' ? formDataToJson(formData) : content }),
      });

      if (response.ok) {
        const savedContent = viewMode === 'gui' ? formDataToJson(formData) : content;
        setOriginalContent(savedContent);
        setContent(savedContent);
        setEditMode(false);
        setFileExists(true);
        message.success('Devcontainer configuration saved');
      } else {
        const errorText = await response.text();
        throw new Error(`HTTP ${response.status}: ${errorText}`);
      }
    } catch (error: any) {
      console.error('Error saving devcontainer.json:', error);
      showError('Failed to save devcontainer configuration', error);
    } finally {
      setLoading(false);
    }
  };

  const cancelEdit = () => {
    if (!fileExists && originalContent === '') {
      // If we were creating a new file, clear the content
      setContent('');
      setFormData({});
    } else {
      setContent(originalContent);
      parseJsonToFormData(originalContent);
    }
    setEditMode(false);
  };

  const createContainer = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/container/create`, {
        method: 'POST',
      });

      if (response.ok) {
        const data = await response.json();
        setContainerId(data.containerId);
        setContainerStatus('stopped');
        message.success('Container created successfully');
      } else {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP ${response.status}`);
      }
    } catch (error: any) {
      console.error('Error creating container:', error);
      showError('Failed to create container', error);
    } finally {
      setLoading(false);
    }
  };

  const startContainer = async () => {
    if (!containerId) return;
    
    setLoading(true);
    try {
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/container/start`, {
        method: 'POST',
      });

      if (response.ok) {
        setContainerStatus('running');
        message.success('Container started successfully');
      } else {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP ${response.status}`);
      }
    } catch (error: any) {
      console.error('Error starting container:', error);
      showError('Failed to start container', error);
    } finally {
      setLoading(false);
    }
  };

  const restartContainer = async () => {
    if (!containerId) return;
    
    setLoading(true);
    try {
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/container/restart`, {
        method: 'POST',
      });

      if (response.ok) {
        setContainerStatus('running');
        message.success('Container restarted successfully');
      } else {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP ${response.status}`);
      }
    } catch (error: any) {
      console.error('Error restarting container:', error);
      showError('Failed to restart container', error);
    } finally {
      setLoading(false);
    }
  };

  const resetContainer = async () => {
    if (!containerId) return;
    
    setLoading(true);
    try {
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/container/reset`, {
        method: 'POST',
      });

      if (response.ok) {
        setContainerStatus('none');
        setContainerId(null);
        message.success('Container reset successfully');
      } else {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP ${response.status}`);
      }
    } catch (error: any) {
      console.error('Error resetting container:', error);
      showError('Failed to reset container', error);
    } finally {
      setLoading(false);
    }
  };

  const handleViewModeChange = (newMode: 'gui' | 'ide') => {
    // If switching from IDE to GUI, validate JSON first
    if (viewMode === 'ide' && newMode === 'gui') {
      if (!parseJsonToFormData(content)) {
        message.error('Cannot switch to GUI mode: Invalid JSON format');
        return;
      }
    }
    
    // If switching from GUI to IDE, update content
    if (viewMode === 'gui' && newMode === 'ide') {
      setContent(formDataToJson(formData));
    }
    
    setViewMode(newMode);
  };

  const editorOptions = {
    readOnly: false,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    wordWrap: 'on' as const,
    formatOnPaste: true,
    formatOnType: true,
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: '16px' }}>
      {fileExists && (
        <div style={{ marginBottom: '16px' }}>
          <h3>Devcontainer Service Controls</h3>
          <Space wrap>
            {containerStatus === 'none' && (
              <Button
                icon={<PlayCircleOutlined />}
                onClick={createContainer}
                loading={loading}
                type="primary"
              >
                Create Container
              </Button>
            )}
            {containerStatus === 'stopped' && (
              <Button
                icon={<PlayCircleOutlined />}
                onClick={startContainer}
                loading={loading}
                type="primary"
              >
                Start Container
              </Button>
            )}
            {containerStatus === 'running' && (
              <>
                <Button
                  icon={<ReloadOutlined />}
                  onClick={restartContainer}
                  loading={loading}
                >
                  Restart Container
                </Button>
                <Button
                  icon={<StopOutlined />}
                  onClick={resetContainer}
                  loading={loading}
                  danger
                >
                  Stop Container
                </Button>
              </>
            )}
            {containerId && (
              <Button
                icon={<DeleteOutlined />}
                onClick={resetContainer}
                loading={loading}
                danger
              >
                Reset Container
              </Button>
            )}
          </Space>
          {containerStatus !== 'none' && (
            <Alert
              message={`Container Status: ${containerStatus}`}
              type={containerStatus === 'running' ? 'success' : 'info'}
              style={{ marginTop: '8px' }}
            />
          )}
        </div>
      )}

      <div style={{ marginBottom: '8px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3>devcontainer.json Configuration</h3>
        <Space>
          {fileExists && editMode && (
            <Radio.Group value={viewMode} onChange={(e) => handleViewModeChange(e.target.value)}>
              <Tooltip title="Graphical User Interface">
                <Radio.Button value="gui">
                  <AppstoreOutlined /> GUI
                </Radio.Button>
              </Tooltip>
              <Tooltip title="Integrated Development Environment">
                <Radio.Button value="ide">
                  <CodeOutlined /> IDE
                </Radio.Button>
              </Tooltip>
            </Radio.Group>
          )}
          {fileExists && (
            <Space>
              {!editMode ? (
                <Button
                  icon={<EditOutlined />}
                  onClick={() => setEditMode(true)}
                  disabled={loading}
                >
                  Edit
                </Button>
              ) : (
                <>
                  <Button
                    icon={<SaveOutlined />}
                    onClick={saveDevcontainerFile}
                    loading={loading}
                    type="primary"
                  >
                    Save
                  </Button>
                  <Button
                    icon={<CloseOutlined />}
                    onClick={cancelEdit}
                    disabled={loading}
                  >
                    Cancel
                  </Button>
                </>
              )}
            </Space>
          )}
        </Space>
      </div>

      <div style={{ flex: 1, border: '1px solid #d9d9d9', borderRadius: '4px', overflow: 'hidden' }}>
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
            <Spin size="large" />
          </div>
        ) : !fileExists && !editMode ? (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', padding: '40px' }}>
            <Empty
              description="No devcontainer.json file exists"
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            >
              <Button
                type="primary"
                icon={<FileAddOutlined />}
                onClick={createDevcontainerFile}
                size="large"
              >
                Create devcontainer.json
              </Button>
            </Empty>
          </div>
        ) : editMode ? (
          viewMode === 'gui' ? (
            <div style={{ padding: '20px', overflow: 'auto', height: '100%' }}>
              <ConfigProvider
                theme={{
                  algorithm: theme.defaultAlgorithm,
                }}
              >
                <Form
                  schema={devcontainerSchema}
                  validator={validator}
                  formData={formData}
                  onChange={(e) => setFormData(e.formData)}
                  widgets={widgets}
                  templates={{ FieldTemplate: CustomFieldTemplate }}
                  uiSchema={{
                    'ui:submitButtonOptions': {
                      norender: true,
                    },
                  }}
                />
              </ConfigProvider>
            </div>
          ) : (
            <Editor
              height="100%"
              language="json"
              value={content}
              onChange={(value) => setContent(value || '')}
              options={editorOptions}
              theme="vs-light"
              beforeMount={(monaco) => {
                // Configure JSON schema validation for devcontainer.json
                monaco.languages.json.jsonDefaults.setDiagnosticsOptions({
                  validate: true,
                  schemas: [{
                    uri: 'https://raw.githubusercontent.com/devcontainers/spec/main/schemas/devContainer.schema.json',
                    fileMatch: ['**/devcontainer.json', '**/.devcontainer/devcontainer.json'],
                  }],
                });
              }}
            />
          )
        ) : (
          <Card style={{ height: '100%', overflow: 'auto' }}>
            <pre style={{ 
              margin: 0, 
              padding: '16px',
              backgroundColor: '#f5f5f5',
              borderRadius: '4px',
              fontSize: '14px',
              lineHeight: '1.5',
              fontFamily: 'Consolas, Monaco, "Courier New", monospace'
            }}>
              {content}
            </pre>
          </Card>
        )}
      </div>
    </div>
  );
}