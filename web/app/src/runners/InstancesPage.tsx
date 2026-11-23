import { useState } from 'react';
import { Button, Flex, Space, Table, Tag, Tooltip, Typography } from 'antd';
import type { TableColumnsType, TableProps } from 'antd';
import { FileTextOutlined, ReloadOutlined } from '@ant-design/icons';
import { Link, useSearchParams } from 'react-router-dom';
import { launchHistory, pulseInstances, type JobLeaseEvent } from '@colony2/shared';
import { dateColumn, textColumn } from '../components/tableColumns';
import { usePages, useRead } from './data';
import { jobStoryPath, launchPath, providerPath } from './paths';
import { PartialErrors, ReadState } from './ReadState';

type Row = {
  launch_id: string;
  provider?: string;
  state: string;
  id?: string;
  created_at?: string;
  started_at?: string;
  leasedAt?: string;
  job: { job_id: string; tenant_id: string };
};
function shortLaunchId(id: string) {
  return id.length > 10 ? id.slice(0, 10) + '…' : id;
}
const statuses = ['queued', 'starting', 'running', 'paused', 'stopping'];
const rowPath = (tenant: string, row: Row, logsOnly = false) =>
  launchPath(
    tenant,
    row.launch_id,
    { provider: row.provider, jobId: row.provider ? undefined : row.job.job_id },
    logsOnly,
  );

export function InstancesTable({
  tenant,
  rows,
  empty = true,
  query,
  onChange,
}: {
  tenant: string;
  rows: Row[];
  empty?: boolean;
  query?: URLSearchParams;
  onChange?: TableProps<Row>['onChange'];
}) {
  const columns: TableColumnsType<Row> = [
    {
      ...textColumn<Row>('Launch ID', 'launchId', (r) => r.launch_id),
      render: (_, r) => (
        <Link to={rowPath(tenant, r)} title={r.launch_id} aria-label={r.launch_id}>
          {shortLaunchId(r.launch_id)}
        </Link>
      ),
    },
    {
      ...textColumn<Row>('Provider', 'provider', (r) => r.provider),
      render: (_, r) => (r.provider ? <Link to={providerPath(tenant, r.provider)}>{r.provider}</Link> : '—'),
    },
    {
      ...textColumn<Row>('Job ID', 'jobId', (r) => r.job.job_id),
      render: (_, r) => (
        <Space direction="vertical" size={0}>
          <Link to={jobStoryPath(r.job.tenant_id, r.job.job_id)}>{r.job.job_id}</Link>
          <Typography.Text type="secondary">{r.job.tenant_id}</Typography.Text>
        </Space>
      ),
    },
    {
      title: 'Status',
      key: 'status',
      filterMultiple: false,
      filters: (query ? ['active', 'all', ...statuses, 'Recorded'] : statuses).map((value) => ({
        text: value === 'active' ? 'Active' : value === 'all' ? 'All launches' : value,
        value,
      })),
      sorter: (a, b) => a.state.localeCompare(b.state),
      onFilter: (v, r) => v === 'all' || (v === 'active' ? r.state !== 'Recorded' : r.state === v),
      render: (_, r) => <Tag>{r.state}</Tag>,
    },
    {
      ...dateColumn<Row>('Start', 'date', (r) => r.created_at || r.started_at || r.leasedAt, 'range'),
      defaultSortOrder: 'descend',
    },
    {
      title: 'Log',
      key: 'log',
      render: (_, r) => (
        <Tooltip title="View log">
          <Link aria-label={`View log for ${r.launch_id}`} to={rowPath(tenant, r, true)}>
            <FileTextOutlined />
          </Link>
        </Tooltip>
      ),
    },
  ];
  if (query) {
    for (const column of columns) {
      const key = String(column.key);
      if (key === 'log') continue;
      const filter = query.get(key) || (key === 'status' ? 'active' : null);
      column.filteredValue = filter ? [filter] : null;
      const sorted = query.get('sort') ?? 'date';
      column.sortOrder = sorted === key ? (query.get('order') === 'ascend' ? 'ascend' : 'descend') : null;
    }
  }
  return (
    <Table<Row>
      size="small"
      className="runner-instances-table"
      aria-label="Runner instances"
      dataSource={rows}
      columns={columns}
      rowKey={(r) => JSON.stringify([r.provider, r.id, r.launch_id, r.job.tenant_id, r.job.job_id])}
      scroll={{ x: 'max-content' }}
      pagination={{ pageSize: 25, hideOnSinglePage: true, showSizeChanger: false }}
      onChange={onChange}
      locale={{ emptyText: empty ? 'No matching instances.' : ' ' }}
    />
  );
}

function Listing({
  tenant,
  query,
  active,
  activeAvailable,
  onChange,
}: {
  tenant: string;
  query: URLSearchParams;
  active: Row[];
  activeAvailable: boolean;
  onChange: TableProps<Row>['onChange'];
}) {
  const status = query.get('status');
  const historyRequested = status === 'all' || status === 'Recorded';
  const job = query.get('jobId') || '';
  const launch = query.get('launchId') || '';
  const history = usePages<JobLeaseEvent>(async (signal, token) => {
    if (!historyRequested) return { items: [] };
    const page = await launchHistory(tenant, signal, job, launch, token);
    return { items: page.events, next: page.nextPageToken };
  });
  const rows = active.filter((row) => !job || (row.job.job_id === job && row.job.tenant_id === tenant));
  const identity = (r: Row) => JSON.stringify([r.job.tenant_id, r.job.job_id, r.launch_id]);
  const seen = new Set(rows.map(identity));
  for (const event of history.items) {
    const row: Row = {
      launch_id: event.workerId,
      state: 'Recorded',
      leasedAt: event.occurredAt,
      job: { job_id: event.jobKey.jobId, tenant_id: event.jobKey.tenantId },
    };
    if (!seen.has(identity(row))) {
      rows.push(row);
      seen.add(identity(row));
    }
  }
  return (
    <>
      {historyRequested && (
        <>
          <Typography.Paragraph type="secondary">
            Recorded launches use the lease acquisition time in {tenant} as their start time.
          </Typography.Paragraph>
          <ReadState {...history} retry={history.retry} />
        </>
      )}
      <InstancesTable
        tenant={tenant}
        rows={rows}
        query={query}
        onChange={onChange}
        empty={activeAvailable && !history.busy && !history.error}
      />
      {historyRequested && history.more && (
        <Button disabled={history.busy} onClick={history.more}>
          Load older launches
        </Button>
      )}
    </>
  );
}

export function InstancesPage({ tenant }: { tenant: string }) {
  const [query, setQuery] = useSearchParams();
  const [revision, refresh] = useState(0);
  const launch = query.get('launchId') || '';
  const active = useRead(launch, (signal) => pulseInstances(signal, launch || undefined));
  const change: TableProps<Row>['onChange'] = (_, filters, sort, { action }) => {
    if (action === 'paginate') return;
    const next = new URLSearchParams(query);
    for (const [key, values] of Object.entries(filters)) {
      if (values?.length) next.set(key, String(values[0]));
      else next.delete(key);
    }
    if (action === 'sort' && !Array.isArray(sort)) {
      next.set('sort', sort.order ? String(sort.columnKey) : '');
      if (sort.order) next.set('order', sort.order);
      else next.delete('order');
    }
    setQuery(next);
  };
  const status = query.get('status') || 'active';
  const history = ['all', 'Recorded'].includes(query.get('status') || '');
  return (
    <>
      <Flex justify="space-between" align="center">
        <Typography.Title level={2}>Instances</Typography.Title>
        <Space>
          {query.size > 0 && <Button onClick={() => setQuery({})}>Reset filters</Button>}
          <Button
            icon={<ReloadOutlined />}
            onClick={() => {
              active.refresh();
              refresh((v) => v + 1);
            }}
          >
            Refresh
          </Button>
        </Space>
      </Flex>
      <Space wrap aria-label="Instance filters">
        <Tag>{status === 'active' ? 'Active launches' : status === 'all' ? 'All launches' : status}</Tag>
        {query.get('jobId') && <Tag>Job ID · {query.get('jobId')}</Tag>}
        {launch && <Tag title={launch}>Launch ID · {shortLaunchId(launch)}</Tag>}
      </Space>
      <ReadState {...active} retry={active.refresh} />
      <PartialErrors result={active.value} />
      <Listing
        key={JSON.stringify([tenant, query.get('jobId'), launch, history, revision])}
        tenant={tenant}
        query={query}
        active={active.value?.items || []}
        activeAvailable={!active.busy && !active.error}
        onChange={change}
      />
    </>
  );
}
