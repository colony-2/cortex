import { useState, useEffect } from 'react';
import { Button, Space, message, Spin, Alert, Card, Empty } from 'antd';
import { PlayCircleOutlined, ReloadOutlined, StopOutlined, DeleteOutlined, EditOutlined, SaveOutlined, CloseOutlined, FileAddOutlined } from '@ant-design/icons';
import Editor from '@monaco-editor/react';
import type { DependencyNode } from '../types';


interface DevcontainerEditorProps {
  node: DependencyNode;
}

export default function DevcontainerEditor({ node }: DevcontainerEditorProps) {
  const [content, setContent] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [originalContent, setOriginalContent] = useState<string>('');
  const [containerStatus, setContainerStatus] = useState<'stopped' | 'running' | 'none'>('none');
  const [containerId, setContainerId] = useState<string | null>(null);
  const [fileExists, setFileExists] = useState(false);

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
      } else if (response.status === 404) {
        // File doesn't exist
        setFileExists(false);
        setContent('');
        setOriginalContent('');
      }
    } catch (error) {
      console.error('Error loading devcontainer.json:', error);
      message.error('Failed to load devcontainer configuration');
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
    setOriginalContent('');
    setEditMode(true);
  };

  const saveDevcontainerFile = async () => {
    setLoading(true);
    try {
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/files/.devcontainer/devcontainer.json`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ content }),
      });

      if (response.ok) {
        setOriginalContent(content);
        setEditMode(false);
        setFileExists(true);
        message.success('Devcontainer configuration saved');
      } else {
        throw new Error('Failed to save');
      }
    } catch (error) {
      console.error('Error saving devcontainer.json:', error);
      message.error('Failed to save devcontainer configuration');
    } finally {
      setLoading(false);
    }
  };

  const cancelEdit = () => {
    if (!fileExists && originalContent === '') {
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
      const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}/container/create`, {
        method: 'POST',
      });

      if (response.ok) {
        const data = await response.json();
        setContainerId(data.containerId);
        setContainerStatus('stopped');
        message.success('Container created successfully');
      } else {
        throw new Error('Failed to create container');
      }
    } catch (error) {
      console.error('Error creating container:', error);
      message.error('Failed to create container');
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
        throw new Error('Failed to start container');
      }
    } catch (error) {
      console.error('Error starting container:', error);
      message.error('Failed to start container');
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
        throw new Error('Failed to restart container');
      }
    } catch (error) {
      console.error('Error restarting container:', error);
      message.error('Failed to restart container');
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
        throw new Error('Failed to reset container');
      }
    } catch (error) {
      console.error('Error resetting container:', error);
      message.error('Failed to reset container');
    } finally {
      setLoading(false);
    }
  };

  const editorOptions = {
    readOnly: !editMode,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    wordWrap: 'on' as const,
    formatOnPaste: true,
    formatOnType: true,
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', padding: '16px' }}>
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

      <div style={{ marginBottom: '8px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3>devcontainer.json Configuration</h3>
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