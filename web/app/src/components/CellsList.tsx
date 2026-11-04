import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Alert, Button, Card, Empty, Input, Select, Space, Table, Tag, Typography } from 'antd';
import { listCells } from '@colony2/shared/api';
import type { CortexCell, ListCellsOptions } from '@colony2/shared/types';
import { useCompact } from '../hooks/useCompact';
import { cellGoalsPath, cellPath } from '../utils/goalTarget';
import CreateCellDialog from './CreateCellDialog';

export default function CellsList({ projectId }: { projectId: string }) {
  const compact = useCompact();
  const navigate = useNavigate();
  const [cells, setCells] = useState<CortexCell[]>([]);
  const [options, setOptions] = useState<ListCellsOptions>({ state: 'ACTIVE' });
  const [nextPage, setNextPage] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [revision, setRevision] = useState(0);
  const [creating, setCreating] = useState(false);
  useEffect(() => {
    let current = true;
    setLoading(true); setError(undefined); setCells([]); setNextPage(undefined);
    listCells(projectId, { ...options, pageSize: 50 }).then(page => {
      if (current) { setCells(page.cells); setNextPage(page.nextPageToken); }
    }).catch(reason => { if (current) setError(reason.message); }).finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [projectId, options, revision]);
  const change = (filter: Partial<ListCellsOptions>) => setOptions(previous => ({ ...previous, ...filter, pageToken: undefined }));
  const links = (cell: CortexCell) => <Link className="compact-touch-link" to={cellGoalsPath(projectId, cell.id)}>Goals for this cell</Link>;
  return <div className="app-page" style={{ padding: 24 }}><Space direction="vertical" size="large" style={{ width: '100%' }}>
    <div className="goal-toolbar"><div><Typography.Title level={3}>Cells</Typography.Title><Typography.Text type="secondary">Registered cells in this tenant</Typography.Text></div>
      <Button type="primary" onClick={() => setCreating(true)}>Create cell</Button></div>
    <Space wrap>
      <Input.Search aria-label="Find cell by name" placeholder="Exact name or alias" allowClear onSearch={name => change({ name: name.trim() })} />
      <Select aria-label="Cell state" value={options.state} onChange={state => change({ state })} options={[
        { value: 'ACTIVE', label: 'Active' }, { value: 'RETIRED', label: 'Retired' }, { value: 'ALL', label: 'All states' },
      ]} />
      <Button onClick={() => setRevision(value => value + 1)}>Refresh</Button>
    </Space>
    {error && <Alert type="error" showIcon message={error} action={<Button onClick={() => setRevision(value => value + 1)}>Retry cells</Button>} />}
    {compact ? <div className="compact-cards" aria-label="Cells" aria-busy={loading}>{cells.map(cell => <Card key={cell.id} size="small">
      <Link to={cellPath(projectId, cell.id)}>{cell.name}</Link>  <Tag>{cell.state}</Tag>
      <p>{cell.description}</p><p><Typography.Text code>{cell.repo}</Typography.Text></p>{links(cell)}
    </Card>)}{!cells.length && !loading && <Empty description="No cells found"><Button onClick={() => setCreating(true)}>Create cell</Button></Empty>}</div>
      : <Table rowKey="id" dataSource={cells} loading={loading} pagination={false} scroll={{ x: 700 }} locale={{ emptyText: 'No cells found' }} columns={[
        { title: 'Name', key: 'name', render: (_, cell) => <Space><Link to={cellPath(projectId, cell.id)}>{cell.name}</Link></Space> },
        { title: 'State', dataIndex: 'state' }, { title: 'Repository', dataIndex: 'repo' }, { title: 'Description', dataIndex: 'description' },
        { title: 'Goals', key: 'goals', render: (_, cell) => links(cell) },
      ]} />}
    <Space>{options.pageToken && <Button onClick={() => change({})}>First page</Button>}{nextPage && <Button disabled={loading} onClick={() => setOptions(previous => ({ ...previous, pageToken: nextPage }))}>Next cells</Button>}</Space>
  </Space>{creating && <CreateCellDialog projectId={projectId} onClose={() => setCreating(false)} onCreated={cell => {
    setCreating(false); navigate(cellPath(projectId, cell.id));
  }} />}</div>;
}
