import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Alert, Card, Empty, Input, List, Button, Space, Typography, Tooltip } from 'antd';
import { ReloadOutlined, FileSearchOutlined, SearchOutlined } from '@ant-design/icons';
import { requestTime, reviewAge, exactRequestTime } from '../utils/reviewAge';
import { readReviewFilters, writeReviewFilters, reviewReturnSearch, reviewCellOptions, reviewSortOptions, reviewTitle, reviewCell, visibleReviews, type ReviewFilters } from '../utils/reviewFilters';
import CompactFilter from './CompactFilter';
import { useCompact } from '../hooks/useCompact';
import './PendingInputsListPage.css';
import { abbreviatePrompt } from '../utils/goalPresentation';
import { useInputActivity } from '@colony2/shared';

export default function PendingInputsListPage({ projectId }: { projectId: string }) {
  const navigate = useNavigate();
  const compact = useCompact();
  const [controlsOpen, setControlsOpen] = useState(false);
  const [params, setParams] = useSearchParams();
  const filters = readReviewFilters(params);
  const change = (patch: Partial<ReviewFilters>, replace = false) => setParams(writeReviewFilters({ ...filters, ...patch }, params), { replace });
  const reset = () => setParams(writeReviewFilters(readReviewFilters(new URLSearchParams()), params));
  const { pendingInputs, refresh, setCurrentProjectId, currentProjectId, connectionError: activityConnectionError, loadError: activityLoadError, discoveryStatus: activityStatus } = useInputActivity();
  const discoveryStatus = currentProjectId === projectId ? activityStatus : 'loading';
  const connectionError = currentProjectId === projectId && activityConnectionError;
  const loadError = currentProjectId === projectId ? activityLoadError : undefined;
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
  const snapshot = currentProjectId === projectId ? pendingInputs : [];
  const items = visibleReviews(snapshot, filters);
  const options = reviewCellOptions(snapshot, filters.cells);
  const active = filters.cells !== null || !!filters.q || filters.sort !== 'requested' || filters.order !== 'ascend';
  useEffect(() => { setCurrentProjectId(projectId); }, [projectId, setCurrentProjectId]);
  const open = (id: string) => navigate(`/project/${encodeURIComponent(projectId)}/reviews/${encodeURIComponent(id)}${reviewReturnSearch(params)}`);
  return <div className="review-page" style={{ padding: 24 }}>
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
        <Typography.Title level={3}>Reviews</Typography.Title>
        <Button icon={<ReloadOutlined />} onClick={() => void refresh()}>Refresh</Button>
      </Space>
      <div>
        {compact && <Button aria-expanded={controlsOpen} aria-controls="reviews-controls" onClick={() => setControlsOpen(value => !value)}>Filters and sorting{active ? ' (active)' : ''}</Button>}
        <div id="reviews-controls" className="reviews-controls" hidden={compact && !controlsOpen}>
          <div className="reviews-control"><span>Cells</span><CompactFilter label={`Cells: ${filters.cells === null ? 'All' : filters.cells.length === 0 ? 'None' : filters.cells.length + ' selected'}`} options={options} selected={filters.cells} onApplySelection={cells => change({ cells })} /></div>
          <label className="reviews-control reviews-search">Search reviews<Input aria-label="Search reviews" placeholder="Match every search term" prefix={<SearchOutlined aria-hidden />} allowClear value={filters.q} onChange={event => change({ q: event.target.value }, true)} /></label>
          <label className="reviews-control">Sort by<select aria-label="Sort by" value={`${filters.sort}:${filters.order}`} onChange={event => {
            const [sort, order] = event.target.value.split(':');
            change({ sort: sort as ReviewFilters['sort'], order: order as ReviewFilters['order'] });
          }}>{reviewSortOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
          <Button onClick={reset}>Reset</Button>
        </div>
        <Typography.Text type="secondary" className="reviews-count">{discoveryStatus !== 'current' && !snapshot.length ? 'Pending request count unavailable' : <>{discoveryStatus !== 'current' ? 'Last known: ' : ''}Showing {items.length} of {snapshot.length} pending requests</>}</Typography.Text>
      </div>
      {connectionError && <Typography.Text type="warning">Live updates unavailable. Refresh to check for requests.</Typography.Text>}
      {loadError && <Alert type="error" showIcon message="Could not load reviews" description={loadError} action={<Button onClick={() => void refresh()}>Retry reviews</Button>} />}
      {discoveryStatus === 'loading' && <Alert type="info" message="Checking pending requests…" />}
      {discoveryStatus !== 'current' && !snapshot.length || loadError && !items.length ? null : !items.length ? <Card><Empty description={!snapshot.length ? 'No pending reviews' : filters.cells?.length === 0 ? 'No cells selected' : 'No requests match these filters'}>{!snapshot.length ? <Typography.Text type="secondary">Reviews and input requests from jobs will appear here</Typography.Text> : <Button onClick={reset}>Reset</Button>}</Empty></Card>
        : <List dataSource={items} renderItem={input => <Card key={input.id} style={{ marginBottom: 16, overflowWrap: 'anywhere' }}>
          <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
            <Space direction="vertical">
              <Space><FileSearchOutlined /><Typography.Text strong>{abbreviatePrompt(reviewTitle(input))}</Typography.Text></Space>
              <Typography.Text type="secondary">Job ID: {input.id}</Typography.Text>
              <Typography.Text>Cell: {reviewCell(input)}</Typography.Text>
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
