import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Alert, Button, Form, Input, Modal, Select, Space, Table, Tag, Typography, message } from 'antd';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { listCells, listJobs, submitJob } from '@colony2/shared/api';
import type { CortexCell, RecipeJob } from '@colony2/shared/types';
import { jobStatus, schedulingStatuses } from '../utils/jobStatus';
import JobListDetails, { jobTime } from './JobListDetails';
import { jobFilterSearch, readJobFilters } from '../utils/jobFilters';
import JobActionDialog, { type JobAction } from './JobActionDialog';
import JobWaitLinks, { goalPath, jobPath } from './JobWaitLinks';
import './JobsListPage.css';

const { Text, Title } = Typography;
type Props = { projectId: string; goalId?: string };
type JobGroup = { key: string; goalId?: string; jobs: RecipeJob[] };
type Row = { key: string; group: JobGroup; job?: RecipeJob; header?: boolean; single?: boolean };
export function abbreviatePrompt(prompt: string, words = 75) {
  const parts = prompt.trim().split(/\s+/);
  return parts.length > words ? `${parts.slice(0, words).join(' ')}…` : parts.join(' ');
}
export default function JobsListPage(props: Props) {
  return <JobsList key={`${props.projectId}/${props.goalId || ''}`} {...props} />;
}

function JobsList({ projectId, goalId }: Props) {
  const navigate = useNavigate();
  const location = useLocation();
  const returnState = { returnTo: location.pathname + location.search };
  const [jobs, setJobs] = useState<RecipeJob[]>([]);
  const [cells, setCells] = useState<CortexCell[]>([]);
  const [nextPage, setNextPage] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  const [submitOpen, setSubmitOpen] = useState(false);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [operation, setOperation] = useState<{ action: JobAction; jobIds: string[] } | null>(null);
  const [refresh, setRefresh] = useState(0);
  const loadVersion = useRef(0);
  const [params, setParams] = useSearchParams();
  const filters = readJobFilters(params);
  const pageToken = params.get('pageToken') || undefined;
  const statusesKey = JSON.stringify(filters.status);
  const expanded = params.getAll('expanded');
  const [form] = Form.useForm<{ prompt: string }>();
  const changeQuery = (change: (query: URLSearchParams) => void, replace = false) => setParams(previous => {
    const query = new URLSearchParams(previous); change(query); return query;
  }, { replace });

  useEffect(() => {
    const version = ++loadVersion.current;
    setLoading(true); setError(undefined);
    (async () => {
      const [cellData, page] = await Promise.all([
        listCells(projectId), listJobs(projectId, { goal: goalId, status: JSON.parse(statusesKey), cell: filters.cell, pageToken }),
      ]);
      if (version !== loadVersion.current) return;
      setCells(cellData); setJobs(page.jobs); setNextPage(page.next_page_token);
      setSelectedIds(previous => previous.filter(id => page.jobs.some(job => job.job_id === id)));
    })().catch(reason => {
      if (version === loadVersion.current) setError(reason instanceof Error ? reason.message : 'Could not load goals');
    }).finally(() => { if (version === loadVersion.current) setLoading(false); });
    return () => { loadVersion.current++; };
  }, [projectId, goalId, filters.cell, pageToken, statusesKey, refresh]);

  // Group only the matching jobs returned by the server. A goal link lists all
  // its attempts; a page is not an authoritative goal count or goal state.
  const groups = useMemo(() => {
    const grouped = new Map<string, JobGroup>();
    for (const job of jobs) {
      const goals = goalId ? [goalId] : [...new Set([job.goal_id, ...(job.other_goal_ids || [])].filter((id): id is string => !!id))];
      for (const id of goals.length ? goals : [undefined]) {
        const key = id ? `goal:${id}` : `job:${job.job_id}`;
        const group = grouped.get(key) || { key, goalId: id, jobs: [] };
        group.jobs.push(job);
        grouped.set(key, group);
      }
    }
    return [...grouped.values()];
  }, [jobs, goalId]);
  const rows: Row[] = groups.flatMap(group => {
    const single = group.jobs.length === 1;
    return [
      ...(!single ? [{ key: group.key, group, header: true }] : []),
      ...group.jobs.map(job => ({ key: JSON.stringify([group.key, job.job_id]), group, job, single })),
    ];
  });
  const setStatuses = (statuses: string[]) => {
    setSelectedIds([]);
    changeQuery(query => {
      query.delete('pageToken'); query.delete('status');
      statuses.forEach(status => query.append('status', status));
    });
  };
  const fullPrompt = (group: JobGroup) => group.jobs.find(job => job.prompt?.trim())?.prompt;
  const promptLink = (group: JobGroup, prompt = fullPrompt(group)) => <Link className="goal-prompt"
    to={group.goalId ? goalPath(projectId, group.goalId) : `${jobPath(projectId, group.jobs[0].job_id)}${jobFilterSearch(params)}`}
    state={returnState}>
    {prompt?.trim() ? abbreviatePrompt(prompt) : group.goalId || group.jobs[0].job_id}
  </Link>;
  const rowPrompt = (row: Row) => {
    const { job, group } = row;
    if (row.header) return <Space direction="vertical" size={2}>
      <Space wrap>{promptLink(group)}<Text type="secondary">{group.jobs.length} attempts on this page</Text></Space>
    </Space>;
    if (!job) return null;
    return <Space direction="vertical" size={2}>
      {row.single ? promptLink(group, job.prompt) : <Link to={`${jobPath(projectId, job.job_id)}${jobFilterSearch(params)}`} state={returnState} aria-label={`Open job ${job.job_id}`}>{job.prompt && job.prompt !== fullPrompt(group) ? abbreviatePrompt(job.prompt) : job.job_id}</Link>}
      <JobWaitLinks job={job} projectId={projectId} />
      {job.completion_detail && <Text type={job.completion_status?.startsWith('failed_') ? 'danger' : 'secondary'} className="job-completion">{job.completion_detail}</Text>}
    </Space>;
  };
  const columns: ColumnsType<Row> = [
    { title: 'Goal / attempt', key: 'prompt', render: (_, row) => rowPrompt(row), onCell: row => ({ colSpan: row.header ? 5 : 1 }) },
    { title: 'Status', width: 160, render: (_, { job }) => {
      if (!job) return null;
      const status = jobStatus(job);
      return <Space size={4}><Link to={`${jobPath(projectId, job.job_id)}${jobFilterSearch(params)}`} state={returnState} aria-label={`Job ${job.job_id}: ${status.label}`}><Tag color={status.color}>{status.label}</Tag></Link>
      </Space>;
    }, onCell: row => ({ colSpan: row.header ? 0 : 1 }) },
    { title: 'Cell', width: 120, render: (_, { job }) => job?.cell_name || (job ? 'self' : null), onCell: row => ({ colSpan: row.header ? 0 : 1 }) },
    { title: 'Submitted', width: 185, render: (_, { job }) => job ? jobTime(job.submitted_at || job.created_at) || '—' : null, onCell: row => ({ colSpan: row.header ? 0 : 1 }) },
    { title: 'Links', width: 180, render: (_, { job }) => job && <Space direction="vertical" size={2}>
      {job.parent?.job_id && <Link to={jobPath(job.parent.tenant_id || projectId, job.parent.job_id)} title={job.parent.job_id}>Parent</Link>}
      {job.repo && <RepositoryLink repo={job.repo} />}
    </Space>, onCell: row => ({ colSpan: row.header ? 0 : 1 }) },
  ];

  const submit = async () => {
    let values: { prompt: string };
    try { values = await form.validateFields(); } catch { return; }
    setSubmitting(true);
    try {
      const job = await submitJob(projectId, { cell: filters.cell, inputs: { prompt: values.prompt, type: 'build' } });
      message.success('Goal created'); setSubmitOpen(false); form.resetFields();
      navigate(goalPath(projectId, job.goal_id!));
    } catch (reason) { message.error(reason instanceof Error ? reason.message : 'Could not create goal'); }
    finally { setSubmitting(false); }
  };

  return <div style={{ padding: 24 }}>
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <div className="goal-toolbar">
        <Title level={3} style={{ margin: 0 }}>{goalId ? 'Goal' : 'Goals'}</Title>
        <Space wrap>
          {goalId && <Link to={`/project/${encodeURIComponent(projectId)}/goals`}>All goals</Link>}
          <Select mode="multiple" aria-label="Status filter" value={filters.status} style={{ minWidth: 240 }} placeholder="All statuses" maxTagCount="responsive" allowClear
            options={Object.entries(schedulingStatuses).map(([value, status]) => ({ value, label: status.label }))} onChange={setStatuses} />
          {!goalId && <Select aria-label="Cell filter" value={filters.cell} options={cells.map(cell => ({ value: cell.name || cell.repository_source, label: cell.name }))}
            style={{ minWidth: 160 }} placeholder="All cells" allowClear showSearch onChange={cell => {
              setSelectedIds([]); changeQuery(query => { query.delete('cell'); if (cell) query.set('cell', cell); query.delete('pageToken'); query.delete('expanded'); });
            }} />}
          <Button icon={<ReloadOutlined aria-hidden />} onClick={() => setRefresh(value => value + 1)}>Refresh</Button>
          <Button type="primary" icon={<PlusOutlined aria-hidden />} onClick={() => setSubmitOpen(true)}>New goal</Button>
        </Space>
      </div>
      {error && <Alert type="error" showIcon message={error} />}
      <Space wrap>
        <Text>{selectedIds.length} selected</Text>
        <Button danger disabled={loading || !selectedIds.length} onClick={() => setOperation({ action: 'cancel', jobIds: selectedIds })}>Cancel selected</Button>
        <Button danger disabled={loading || !selectedIds.length} onClick={() => setOperation({ action: 'delete', jobIds: selectedIds })}>Delete selected</Button>
        {!!selectedIds.length && <Button onClick={() => setSelectedIds([])}>Clear selection</Button>}
      </Space>
      <Table<Row> rowKey="key" columns={columns} dataSource={rows} loading={loading} pagination={false} size="small" scroll={{ x: 1000 }}
        rowClassName={row => row.header ? 'goal-group-header' : ''}
        rowSelection={{
          selectedRowKeys: rows.filter(row => row.job && selectedIds.includes(row.job.job_id)).map(row => row.key),
          onSelect: (row, selected) => row.job && setSelectedIds(previous => selected
            ? [...new Set([...previous, row.job!.job_id])] : previous.filter(id => id !== row.job!.job_id)),
          onSelectAll: (selected, _, changed) => setSelectedIds(previous => {
            const ids = new Set(changed.flatMap(row => row.job ? [row.job.job_id] : []));
            return selected ? [...new Set([...previous, ...ids])] : previous.filter(id => !ids.has(id));
          }),
          getCheckboxProps: row => ({ disabled: loading || !row.job, 'aria-label': row.job ? `Select job ${row.job.job_id}` : 'Goal group' }),
          renderCell: (_, row, __, node) => row.job ? node : null,
        }}
        expandable={{
          expandedRowKeys: rows.filter(row => row.job && expanded.includes(row.job.job_id)).map(row => row.key),
          onExpand: (open, row) => changeQuery(query => {
            const ids = new Set(query.getAll('expanded')); if (open) ids.add(row.job!.job_id); else ids.delete(row.job!.job_id);
            query.delete('expanded'); ids.forEach(id => query.append('expanded', id));
          }, true),
          rowExpandable: row => !!row.job,
          expandedRowRender: row => row.job ? <JobListDetails job={row.job} projectId={projectId} /> : null,
          columnWidth: 40,
        }} />
      <Space>
        {pageToken && <Button onClick={() => changeQuery(query => { query.delete('pageToken'); })}>First page</Button>}
        {nextPage && <Button disabled={loading} onClick={() => changeQuery(query => { query.set('pageToken', nextPage); })}>Next jobs</Button>}
      </Space>
    </Space>
    {operation && <JobActionDialog projectId={projectId} {...operation} onClose={() => setOperation(null)} onComplete={async succeeded => {
      setSelectedIds(previous => previous.filter(id => !succeeded.includes(id))); setRefresh(value => value + 1);
    }} />}
    <Modal title="New goal" open={submitOpen} onCancel={() => setSubmitOpen(false)} onOk={submit} okText="Create goal" okButtonProps={{ loading: submitting }} destroyOnHidden>
      <Text type="secondary">Cell: {filters.cell || cells.find(cell => cell.kind === 'self')?.name || 'self'}</Text>
      <Form form={form} layout="vertical"><Form.Item label="Prompt" name="prompt" rules={[{ required: true, whitespace: true, message: 'Enter a prompt' }]}><Input.TextArea rows={6} /></Form.Item></Form>
    </Modal>
  </div>;
}

export function RepositoryLink({ repo }: { repo: string }) {
  // Never turn arbitrary git selectors into executable browser URLs.
  const url = repo.replace(/^git\+/, '');
  const label = repo.replace(/\/$/, '').split('/').slice(-1)[0]?.replace(/\.git$/, '') || repo;
  return /^https?:\/\//i.test(url) ? <a href={url} target="_blank" rel="noreferrer" title={repo}>{label}</a> : <Text title={repo}>{label}</Text>;
}
