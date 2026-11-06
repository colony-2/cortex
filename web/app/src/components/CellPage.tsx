import { useEffect, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { Alert, Button, Descriptions, Space, Spin, Tabs, Tag, Typography } from 'antd';
import { getCell } from '@colony2/shared/api';
import type { CortexCell } from '@colony2/shared/types';
import { cellGoalsPath } from '../utils/goalTarget';
import NewGoalDialog from './NewGoalDialog';
import { AdminPanel } from './AdminPage';
import type { AdminKind } from '@colony2/shared';

export default function CellPage({ projectId, cellId }: { projectId: string; cellId: string }) {
  const [cell, setCell] = useState<CortexCell>();
  const [error, setError] = useState<string>();
  const [revision, setRevision] = useState(0);
  const [creating, setCreating] = useState(false);
  const navigate = useNavigate();
  const [search, setSearch] = useSearchParams();
  const tab = search.get('tab') || 'overview';
  useEffect(() => {
    let current = true;
    setCell(undefined); setError(undefined);
    getCell(projectId, cellId).then(value => { if (current) setCell(value); }).catch(reason => { if (current) setError(reason.message); });
    return () => { current = false; };
  }, [projectId, cellId, revision]);
  return <div className="app-page" style={{ padding: 24 }}><Space direction="vertical" size="large" style={{ width: '100%' }}>
    <Link to={`/project/${encodeURIComponent(projectId)}/cells`}>Back to Cells</Link>
    {error ? <Alert type="error" showIcon message={error} action={<Button onClick={() => setRevision(value => value + 1)}>Retry cell</Button>} /> : !cell ? <Spin /> : <>
      <Typography.Title level={3}>{cell.name}  <Tag>{cell.state}</Tag></Typography.Title>
      <Space wrap><Link className="compact-touch-link" to={cellGoalsPath(projectId, cell.id)}>Goals for this cell</Link>
        <Button type="primary" disabled={cell.state !== 'ACTIVE'} onClick={() => setCreating(true)}>New goal in this cell</Button>
        <Button onClick={() => setRevision(value => value + 1)}>Refresh</Button></Space>
      <Tabs activeKey={['overview', 'hooks', 'configs', 'git', 'secrets'].includes(tab) ? tab : 'overview'} onChange={value => setSearch(previous => { const next = new URLSearchParams(previous); next.set('tab', value); return next; })} destroyOnHidden items={[
        { key: 'overview', label: 'Overview', children: <Descriptions bordered column={1} items={[
        { key: 'repo', label: 'Repository', children: <Typography.Text code>{cell.repo}</Typography.Text> },
        { key: 'description', label: 'Description', children: cell.description || '—' },
        { key: 'id', label: 'Cell ID', children: cell.id },
        { key: 'aliases', label: 'Aliases', children: cell.aliases?.join(', ') || '—' },
        { key: 'metadata', label: 'Metadata', children: <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{JSON.stringify(cell.metadata, null, 2)}</pre> },
        ]} /> },
        ...([['hooks', 'Hook permissions'], ['configs', 'Configs'], ['git', 'Git'], ['secrets', 'Secrets']] as const).map(([kind, label]) => ({
          key: kind, label, children: <AdminPanel key={`${projectId}/${cellId}/${kind}`} projectId={projectId} cellId={cellId} kind={kind as AdminKind} />,
        })),
      ]} />
    </>}
  </Space>{creating && <NewGoalDialog projectId={projectId} filters={[cellId]} onClose={() => setCreating(false)} onCreated={id => {
    navigate(`/project/${encodeURIComponent(projectId)}/goals/${encodeURIComponent(id)}`);
  }} />}</div>;
}
