import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, Empty, List, Button, Space, Typography, Spin } from 'antd';
import { ReloadOutlined, FormOutlined } from '@ant-design/icons';
import { useInputActivity } from '@colony2/shared';

const { Title, Text } = Typography;

interface PendingInputsListPageProps {
  projectId: string;
  reviews?: boolean;
}

export default function PendingInputsListPage({ projectId, reviews = false }: PendingInputsListPageProps) {
  const navigate = useNavigate();
  const { pendingInputs, isConnected, refresh, setCurrentProjectId } = useInputActivity();

  const items = pendingInputs.filter(input => (input.kind === 'review') === reviews);

  useEffect(() => {
    // Set current project for SSE connection
    setCurrentProjectId(projectId);
  }, [projectId, setCurrentProjectId]);

  const handleRefresh = async () => {
    await refresh();
  };

  const handleViewInput = (jobId: string) => {
    navigate(reviews ? `/project/${projectId}/reviews/${jobId}` : `/project/${projectId}/jobs/${jobId}/story?input=1`);
  };

  if (!isConnected) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>
          <Text type="secondary">Connecting to input stream...</Text>
        </div>
      </div>
    );
  }

  return (
    <div style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Title level={3} style={{ margin: 0 }}>
            {reviews ? 'Reviews' : 'Pending Inputs'}
          </Title>
          <Button icon={<ReloadOutlined />} onClick={handleRefresh}>
            Refresh
          </Button>
        </div>

        {items.length === 0 ? (
          <Card>
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={reviews ? 'No pending reviews' : 'No pending inputs'}
              style={{ padding: '48px 0' }}
            >
              <Text type="secondary">
                {reviews ? 'Document reviews from jobs will appear here' : 'Pending input requests from jobs will appear here'}
              </Text>
            </Empty>
          </Card>
        ) : (
          <List
            dataSource={items}
            renderItem={(input) => (
              <Card
                key={input.id}
                style={{ marginBottom: 16 }}
                hoverable
                onClick={() => handleViewInput(input.id)}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Space direction="vertical" size="small">
                      <Space>
                        <FormOutlined style={{ fontSize: 20, color: '#1890ff' }} />
                      <Text strong>{reviews ? input.title || 'Review' : 'Input Request'}</Text>
                    </Space>
                    <Text type="secondary" style={{ fontSize: 13 }}>
                      Job ID: {input.id}{reviews ? ` · ${input.document_count || 0} documents` : ''}
                    </Text>
                  </Space>
                  <Button type="primary" onClick={() => handleViewInput(input.id)}>
                    {reviews ? 'Open Review' : 'Open Job'}
                  </Button>
                </div>
              </Card>
            )}
          />
        )}
      </Space>
    </div>
  );
}
