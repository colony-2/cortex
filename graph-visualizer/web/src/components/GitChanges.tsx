import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Tabs, Button, Badge, Space, Typography, Empty, Spin, List, Tag, message } from 'antd';
import { SyncOutlined, FileAddOutlined, EditOutlined, DeleteOutlined, QuestionCircleOutlined } from '@ant-design/icons';
import type { DependencyNode } from '../types';
import { navigateToPath } from '../utils/urlState';

const { Title, Text } = Typography;

interface GitChangesProps {
  node: DependencyNode;
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

export default function GitChanges({ node }: GitChangesProps) {
  const { subtab = 'summary' } = useParams<{ subtab?: string }>();
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState(subtab);
  const [status, setStatus] = useState<GitStatus | null>(null);
  const [diff, setDiff] = useState<GitDiff | null>(null);
  const [history, setHistory] = useState<GitCommit[]>([]);
  const [loading, setLoading] = useState(false);
  const [committing, setCommitting] = useState(false);

  // Update active tab when URL changes
  useEffect(() => {
    setActiveTab(subtab);
  }, [subtab]);

  const handleTabChange = (key: string) => {
    setActiveTab(key);
    const path = navigateToPath({ boxId: node.id, tab: 'changes', subtab: key });
    navigate(path);
  };

  // Fetch git status
  const fetchStatus = async () => {
    try {
      const response = await fetch(`/api/nodes/${node.id}/git/status`);
      if (response.ok) {
        const data = await response.json();
        setStatus(data);
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
    try {
      const response = await fetch(`/api/nodes/${node.id}/git/diff`);
      if (response.ok) {
        const data = await response.json();
        setDiff(data);
      } else {
        setDiff(null);
      }
    } catch (error) {
      console.error('Failed to fetch git diff:', error);
      setDiff(null);
    }
  };

  // Fetch git history
  const fetchHistory = async () => {
    try {
      const response = await fetch(`/api/nodes/${node.id}/git/history`);
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
  }, [node.id, activeTab]);

  // Handle commit
  const handleCommit = async () => {
    setCommitting(true);
    try {
      const response = await fetch(`/api/nodes/${node.id}/git/commit`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (response.ok) {
        const result = await response.json();
        message.success(`Commit created: ${result.commitMessage}`);
        // Refresh status after commit
        await fetchStatus();
        if (activeTab === 'history') {
          await fetchHistory();
        }
      } else {
        message.error('Failed to create commit');
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
      return <Empty description="No commits for this box" />;
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