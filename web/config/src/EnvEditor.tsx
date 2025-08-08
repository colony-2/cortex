import { useState, useEffect } from 'react';
import { Button, Space, message, Spin, Alert, Card, Empty, Modal } from 'antd';
import { 
  PlayCircleOutlined, 
  ReloadOutlined, 
  StopOutlined, 
  DeleteOutlined, 
  EditOutlined, 
  SaveOutlined, 
  CloseOutlined, 
  FileAddOutlined
} from '@ant-design/icons';
import Editor from '@monaco-editor/react';
import type { DependencyCell } from '@vibethis/shared';


export interface EnvEditorProps {
  cell: DependencyCell;
}

export default function EnvEditor({ cell }: EnvEditorProps) {
  const [content, setContent] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [originalContent, setOriginalContent] = useState<string>('');
  const [containerStatus, setContainerStatus] = useState<'stopped' | 'running' | 'none'>('none');
  const [containerId, setContainerId] = useState<string | null>(null);
  const [hasDevcontainer, setHasDevcontainer] = useState(false);

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


  // Load container status and devcontainer.json
  useEffect(() => {
    loadContainerStatus();
  }, [cell]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadContainerStatus = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/cells/${encodeURIComponent(cell.id)}/container/status`);
      if (response.ok) {
        const data = await response.json();
        setContainerStatus(data.status);
        setContainerId(data.containerId);
        setHasDevcontainer(data.hasDevcontainer);
        if (data.devcontainerContent) {
          setContent(data.devcontainerContent);
          setOriginalContent(data.devcontainerContent);
        } else {
          setContent('');
          setOriginalContent('');
        }
      } else {
        console.error('Error loading container status');
      }
    } catch (error) {
      console.error('Error loading container status:', error);
    } finally {
      setLoading(false);
    }
  };

  const createDevcontainerFile = () => {
    const defaultConfig = {
      name: cell.name || "Dev Container",
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
    setOriginalContent('');
    setEditMode(true);
  };

  const saveDevcontainerFile = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/cells/${encodeURIComponent(cell.id)}/container/devcontainer`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ content }),
      });

      if (response.ok) {
        setOriginalContent(content);
        setEditMode(false);
        setHasDevcontainer(true);
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
    if (!hasDevcontainer && originalContent === '') {
      // If we were creating a new file, clear the content
      setContent('');
    } else {
      setContent(originalContent);
    }
    setEditMode(false);
  };

  const createContainer = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/cells/${encodeURIComponent(cell.id)}/container/create`, {
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
      const response = await fetch(`/api/cells/${encodeURIComponent(cell.id)}/container/start`, {
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
      const response = await fetch(`/api/cells/${encodeURIComponent(cell.id)}/container/restart`, {
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
      const response = await fetch(`/api/cells/${encodeURIComponent(cell.id)}/container/reset`, {
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
      {hasDevcontainer && (
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
          {hasDevcontainer && (
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
        ) : !hasDevcontainer && !editMode ? (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', padding: '40px' }}>
            <Empty
              description="No local devcontainer.json configuration"
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