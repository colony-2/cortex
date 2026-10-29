import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, Empty, List, Button, Space, Typography } from 'antd';
import { ReloadOutlined, FileSearchOutlined } from '@ant-design/icons';
import { cellLabel } from '../utils/cellPresentation';
import { abbreviatePrompt } from '../utils/goalPresentation';
import { useInputActivity } from '@colony2/shared';

export default function PendingInputsListPage({ projectId }: { projectId: string }) {
  const navigate = useNavigate();
  const { pendingInputs, refresh, setCurrentProjectId, currentProjectId, connectionError } = useInputActivity();
  const items = currentProjectId === projectId ? pendingInputs : [];
  useEffect(() => { setCurrentProjectId(projectId); }, [projectId, setCurrentProjectId]);
  const open = (id: string) => navigate(`/project/${encodeURIComponent(projectId)}/reviews/${encodeURIComponent(id)}`);
  return <div className="review-page" style={{ padding: 24 }}>
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Space style={{ width: '100%', justifyContent: 'space-between' }}>
        <Typography.Title level={3}>Reviews</Typography.Title>
        <Button icon={<ReloadOutlined />} onClick={() => void refresh()}>Refresh</Button>
      </Space>
      {connectionError && <Typography.Text type="warning">Live updates unavailable. Refresh to check for requests.</Typography.Text>}
      {!items.length ? <Card><Empty description="No pending reviews"><Typography.Text type="secondary">Reviews and input requests from jobs will appear here</Typography.Text></Empty></Card>
        : <List dataSource={items} renderItem={input => <Card key={input.id} style={{ marginBottom: 16, overflowWrap: 'anywhere' }}>
          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
            <Space direction="vertical">
              <Space><FileSearchOutlined /><Typography.Text strong>{abbreviatePrompt(input.title || input.prompt || 'Review')}</Typography.Text></Space>
              <Typography.Text type="secondary">Job ID: {input.id}</Typography.Text>
              <Typography.Text>Cell: {cellLabel(input.cell_name)}</Typography.Text>
            </Space>
            <Button type="primary" onClick={() => open(input.id)}>Open Review</Button>
          </Space>
        </Card>} />}
    </Space>
  </div>;
}
