import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Alert, Card, Empty, List, Button, Space, Typography, Tooltip } from 'antd';
import { ReloadOutlined, FileSearchOutlined } from '@ant-design/icons';
import { compareReviewRequests, requestTime, reviewAge, exactRequestTime } from '../utils/reviewAge';
import { cellLabel } from '../utils/cellPresentation';
import { abbreviatePrompt } from '../utils/goalPresentation';
import { useInputActivity } from '@colony2/shared';

export default function PendingInputsListPage({ projectId }: { projectId: string }) {
  const navigate = useNavigate();
  const { pendingInputs, refresh, setCurrentProjectId, currentProjectId, connectionError, loadError } = useInputActivity();
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const update = () => setNow(Date.now());
    const interval = window.setInterval(update, 60000);
    window.addEventListener('focus', update);
    document.addEventListener('visibilitychange', update);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener('focus', update);
      document.removeEventListener('visibilitychange', update);
    };
  }, []);
  const items = currentProjectId === projectId ? [...pendingInputs].sort(compareReviewRequests) : [];
  useEffect(() => { setCurrentProjectId(projectId); }, [projectId, setCurrentProjectId]);
  const open = (id: string) => navigate(`/project/${encodeURIComponent(projectId)}/reviews/${encodeURIComponent(id)}`);
  return <div className="review-page" style={{ padding: 24 }}>
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
        <Space wrap align="baseline"><Typography.Title level={3}>Reviews</Typography.Title><Typography.Text type="secondary">Oldest first</Typography.Text></Space>
        <Button icon={<ReloadOutlined />} onClick={() => void refresh()}>Refresh</Button>
      </Space>
      {connectionError && <Typography.Text type="warning">Live updates unavailable. Refresh to check for requests.</Typography.Text>}
      {loadError ? <Alert type="error" showIcon message="Could not load reviews" description={loadError} action={<Button onClick={() => void refresh()}>Retry reviews</Button>} /> : !items.length ? <Card><Empty description="No pending reviews"><Typography.Text type="secondary">Reviews and input requests from jobs will appear here</Typography.Text></Empty></Card>
        : <List dataSource={items} renderItem={input => <Card key={input.id} style={{ marginBottom: 16, overflowWrap: 'anywhere' }}>
          <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
            <Space direction="vertical">
              <Space><FileSearchOutlined /><Typography.Text strong>{abbreviatePrompt(input.title || input.prompt || 'Review')}</Typography.Text></Space>
              <Typography.Text type="secondary">Job ID: {input.id}</Typography.Text>
              <Typography.Text>Cell: {cellLabel(input.cell_name)}</Typography.Text>
              {Number.isFinite(requestTime(input)) ? <Tooltip title={exactRequestTime(requestTime(input))} trigger={['hover', 'focus']}>
                <time tabIndex={0} dateTime={input.requested_at} title={exactRequestTime(requestTime(input))} aria-label={`${reviewAge(requestTime(input), now)}. ${exactRequestTime(requestTime(input))}`}>{reviewAge(requestTime(input), now)}</time>
              </Tooltip> : <Typography.Text type="secondary">Request time unavailable</Typography.Text>}
            </Space>
            <Button type="primary" onClick={() => open(input.id)}>Open Review</Button>
          </Space>
        </Card>} />}
    </Space>
  </div>;
}
