import { useState, useEffect } from 'react';
import { Tabs, Button, Badge, Space, Typography, Empty, Spin, List, Tag, message } from 'antd';
import { SyncOutlined, FileAddOutlined, EditOutlined, DeleteOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import type { DependencyCell } from '@vibethis/shared';

const { Title, Text } = Typography;

export interface GitChangesProps {
  cell: DependencyCell | null;
  activeTab?: string;
  onTabChange?: (key: string) => void;
}

interface GitStatus {
  added: string[];
  modified: string[];
  deleted: string[];
  untracked: string[];
  totalCount: number;
}

interface GitCommit {
  hash: string;
  author: string;
  email: string;
  message: string;
  timestamp: string;
}

interface GitFileDiff {
  path: string;
  status: string;
  additions: number;
  deletions: number;
  patch: string;
}

interface GitDiff {
  files: GitFileDiff[];
}

export default function GitChanges({ cell, activeTab = 'summary', onTabChange }: GitChangesProps) {
  const [status, setStatus] = useState<GitStatus | null>(null);
  const [diff, setDiff] = useState<GitDiff | null>(null);
  const [history, setHistory] = useState<GitCommit[]>([]);
  const [loading, setLoading] = useState(false);
  const [committing, setCommitting] = useState(false);

  // Get the cell ID
  const cellId = cell?.id;

  const handleTabChange = (key: string) => {
    onTabChange?.(key);
  };

  // Fetch git status
  const fetchStatus = async () => {
    if (!cellId) return;
    try {
      const response = await fetch(`/api/cells/${cellId}/git/status`);
      if (response.ok) {
        const data = await response.json();
        // Transform the backend response to match the expected format
        const transformedStatus: GitStatus = {
          added: [],
          modified: [],
          deleted: [],
          untracked: [],
          totalCount: 0
        };
        
        if (data.files && Array.isArray(data.files)) {
          data.files.forEach((file: { path: string; status: string }) => {
            switch (file.status) {
              case 'A':
                transformedStatus.added.push(file.path);
                break;
              case 'M':
                transformedStatus.modified.push(file.path);
                break;
              case 'D':
                transformedStatus.deleted.push(file.path);
                break;
              case '?':
                transformedStatus.untracked.push(file.path);
                break;
            }
            transformedStatus.totalCount++;
          });
        }
        
        setStatus(transformedStatus);
      } else {
        setStatus(null);
      }
    } catch (error) {
      console.error('Failed to fetch git status:', error);
      setStatus(null);
    }
  };

  // Fetch git diff
  const fetchDiff = async () => {
    if (!cellId) return;
    try {
      const response = await fetch(`/api/cells/${cellId}/git/diff`);
      if (response.ok) {
        const diffText = await response.text();
        // Parse the plain text diff into a structured format
        const parsedDiff = parseDiff(diffText);
        setDiff(parsedDiff);
      } else {
        setDiff(null);
      }
    } catch (error) {
      console.error('Failed to fetch git diff:', error);
      setDiff(null);
    }
  };

  // Parse git diff text into structured format
  const parseDiff = (diffText: string): GitDiff => {
    const files: GitFileDiff[] = [];
    
    if (!diffText || diffText.trim() === '') {
      return { files };
    }

    // Split by file markers
    const fileChunks = diffText.split(/^diff --git/m).filter(chunk => chunk.trim());
    
    for (const chunk of fileChunks) {
      const lines = chunk.split('\n');
      const fileMatch = lines[0]?.match(/a\/(.+) b\/(.+)/);
      
      if (fileMatch) {
        const path = fileMatch[2];
        let status = 'M'; // Default to modified
        let additions = 0;
        let deletions = 0;
        
        // Check for new file
        if (chunk.includes('new file mode')) {
          status = 'A';
        } else if (chunk.includes('deleted file mode')) {
          status = 'D';
        }
        
        // Count additions and deletions
        lines.forEach(line => {
          if (line.startsWith('+') && !line.startsWith('+++')) {
            additions++;
          } else if (line.startsWith('-') && !line.startsWith('---')) {
            deletions++;
          }
        });
        
        // Extract the patch content
        const patchStart = lines.findIndex(line => line.startsWith('@@'));
        const patch = patchStart >= 0 ? lines.slice(patchStart).join('\n') : chunk;
        
        files.push({
          path,
          status,
          additions,
          deletions,
          patch
        });
      }
    }
    
    return { files };
  };

  // Fetch git history
  const fetchHistory = async () => {
    if (!cellId) return;
    try {
      const response = await fetch(`/api/cells/${cellId}/git/history`);
      if (response.ok) {
        const data = await response.json();
        setHistory(data);
      } else {
        setHistory([]);
      }
    } catch (error) {
      console.error('Failed to fetch git history:', error);
      setHistory([]);
    }
  };

  // Load data based on active tab
  useEffect(() => {
    setLoading(true);
    
    const loadData = async () => {
      if (activeTab === 'summary') {
        await fetchStatus();
      } else if (activeTab === 'details') {
        await fetchDiff();
      } else if (activeTab === 'history') {
        await fetchHistory();
      }
      setLoading(false);
    };

    loadData();
  }, [cellId, activeTab]);

  // Handle commit
  const handleCommit = async () => {
    setCommitting(true);
    try {
      const response = await fetch(`/api/cells/${cellId}/git/commit`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          message: 'Update changes', // TODO: Add UI for custom commit message
          files: [] // Empty array means stage all changes
        }),
      });

      if (response.ok) {
        message.success('Commit created successfully');
        // Refresh status after commit
        await fetchStatus();
        if (activeTab === 'history') {
          await fetchHistory();
        }
      } else {
        const errorText = await response.text();
        message.error(`Failed to create commit: ${errorText}`);
      }
    } catch (error) {
      console.error('Failed to commit:', error);
      message.error('Failed to create commit');
    } finally {
      setCommitting(false);
    }
  };

  // Render file icon based on status
  const getFileIcon = (status: string) => {
    switch (status.toLowerCase()) {
      case 'added':
      case 'untracked':
        return <FileAddOutlined style={{ color: '#52c41a' }} />;
      case 'modified':
        return <EditOutlined style={{ color: '#1890ff' }} />;
      case 'deleted':
        return <DeleteOutlined style={{ color: '#ff4d4f' }} />;
      default:
        return <QuestionCircleOutlined />;
    }
  };

  // Render summary tab
  const renderSummary = () => {
    if (loading) {
      return <Spin size="large" style={{ display: 'block', margin: '40px auto' }} />;
    }

    if (!status) {
      return <Empty description="Not a git repository" />;
    }

    if (status.totalCount === 0) {
      return <Empty description="No changes to commit" />;
    }

    return (
      <div>
        <Space direction="vertical" style={{ width: '100%' }} size="large">
          <div>
            <Title level={5}>Change Summary</Title>
            <Space wrap>
              {status.added.length > 0 && (
                <Badge count={status.added.length} style={{ backgroundColor: '#52c41a' }}>
                  <Tag icon={<FileAddOutlined />} color="success">Added</Tag>
                </Badge>
              )}
              {status.modified.length > 0 && (
                <Badge count={status.modified.length} style={{ backgroundColor: '#1890ff' }}>
                  <Tag icon={<EditOutlined />} color="processing">Modified</Tag>
                </Badge>
              )}
              {status.deleted.length > 0 && (
                <Badge count={status.deleted.length} style={{ backgroundColor: '#ff4d4f' }}>
                  <Tag icon={<DeleteOutlined />} color="error">Deleted</Tag>
                </Badge>
              )}
              {status.untracked.length > 0 && (
                <Badge count={status.untracked.length} style={{ backgroundColor: '#faad14' }}>
                  <Tag icon={<FileAddOutlined />} color="warning">Untracked</Tag>
                </Badge>
              )}
            </Space>
          </div>

          <div>
            <Title level={5}>Files ({status.totalCount})</Title>
            <List
              size="small"
              dataSource={[
                ...status.added.map(f => ({ path: f, status: 'added' })),
                ...status.modified.map(f => ({ path: f, status: 'modified' })),
                ...status.deleted.map(f => ({ path: f, status: 'deleted' })),
                ...status.untracked.map(f => ({ path: f, status: 'untracked' })),
              ]}
              renderItem={item => (
                <List.Item>
                  <Space>
                    {getFileIcon(item.status)}
                    <Text code>{item.path}</Text>
                  </Space>
                </List.Item>
              )}
            />
          </div>

          <Button
            type="primary"
            icon={<SyncOutlined />}
            onClick={handleCommit}
            loading={committing}
            block
          >
            Commit All Changes
          </Button>
        </Space>
      </div>
    );
  };

  // Render details tab
  const renderDetails = () => {
    if (loading) {
      return <Spin size="large" style={{ display: 'block', margin: '40px auto' }} />;
    }

    if (!diff || diff.files.length === 0) {
      return <Empty description="No diff available" />;
    }

    return (
      <div>
        <Title level={5}>Detailed Changes</Title>
        <List
          dataSource={diff.files}
          renderItem={file => (
            <List.Item>
              <div style={{ width: '100%' }}>
                <Space>
                  {getFileIcon(file.status)}
                  <Text code>{file.path}</Text>
                  <Text type="success">+{file.additions}</Text>
                  <Text type="danger">-{file.deletions}</Text>
                </Space>
                <pre style={{ 
                  marginTop: '8px', 
                  padding: '12px', 
                  backgroundColor: '#f5f5f5',
                  borderRadius: '4px',
                  overflow: 'auto',
                  fontSize: '12px',
                  lineHeight: '1.5'
                }}>
                  {file.patch}
                </pre>
              </div>
            </List.Item>
          )}
        />
      </div>
    );
  };

  // Render history tab
  const renderHistory = () => {
    if (loading) {
      return <Spin size="large" style={{ display: 'block', margin: '40px auto' }} />;
    }

    if (history.length === 0) {
      return <Empty description="No commits for this cell" />;
    }

    return (
      <div>
        <Title level={5}>Commit History</Title>
        <List
          dataSource={history}
          renderItem={commit => (
            <List.Item>
              <div>
                <Space direction="vertical" size="small">
                  <Space>
                    <Text code>{commit.hash}</Text>
                    <Text type="secondary">{new Date(commit.timestamp).toLocaleString()}</Text>
                  </Space>
                  <Text>{commit.message}</Text>
                  <Text type="secondary" style={{ fontSize: '12px' }}>
                    by {commit.author} ({commit.email})
                  </Text>
                </Space>
              </div>
            </List.Item>
          )}
        />
      </div>
    );
  };

  const items = [
    {
      key: 'summary',
      label: 'Summary',
      children: renderSummary(),
    },
    {
      key: 'details',
      label: 'Details',
      children: renderDetails(),
    },
    {
      key: 'history',
      label: 'History',
      children: renderHistory(),
    },
  ];

  return (
    <div style={{ padding: '16px' }}>
      <Tabs
        activeKey={activeTab}
        onChange={handleTabChange}
        items={items}
      />
    </div>
  );
}