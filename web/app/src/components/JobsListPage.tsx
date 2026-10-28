import { cellLabel } from '../utils/cellPresentation';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Alert, Button, Form, Input, Modal, Select, Space, Spin, Table, Typography, message } from 'antd';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { listCells, listJobs, submitJob } from '@colony2/shared/api';
import type { CortexCell, RecipeJob } from '@colony2/shared/types';
import { jobStatus, schedulingStatuses } from '../utils/jobStatus';
import { jobTime } from './JobListDetails';
import GoalJobOverview, { AttemptStatus } from './GoalJobOverview';
import { abbreviatePrompt, latestActiveJob } from '../utils/goalPresentation';
import { jobFilterSearch, readJobFilters } from '../utils/jobFilters';
import JobActionDialog, { type JobAction } from './JobActionDialog';
import JobWaitLinks, { goalPath, jobPath } from './JobWaitLinks';
import './JobsListPage.css';

const { Text, Title } = Typography;
type Props = { projectId: string; goalId?: string };
type JobGroup = { key: string; goalId?: string; jobs: RecipeJob[] };
type Row = { key: string; group: JobGroup; job: RecipeJob };
export default function JobsListPage(props: Props) {
  return <JobsList key={`${props.projectId}/${props.goalId || ''}`} {...props} />;
}

function JobsList({ projectId, goalId }: Props) {
  const navigate = useNavigate();
  const location = useLocation();
  const homePath = `/project/${encodeURIComponent(projectId)}/goals`;
  const savedHome = location.state?.goalsReturnTo;
  const goalsReturnTo = goalId
    ? (typeof savedHome === 'string' && [homePath, `/project/${encodeURIComponent(projectId)}/jobs`].includes(savedHome.split('?')[0]) ? savedHome : homePath)
    : location.pathname + location.search;
  const navigationState = (id?: string) => ({ goalsReturnTo, goalId: id,
    ...(goalId ? { goalReturnTo: location.pathname + location.search } : {}) });
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
        listCells(projectId), listJobs(projectId, { goal: goalId, status: goalId ? [] : JSON.parse(statusesKey), cell: goalId ? undefined : filters.cell, pageToken }),
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
  const rows: Row[] = groups.map(group => ({ key: group.key, group, job: latestActiveJob(group.jobs) }));
  const setStatuses = (statuses: string[]) => {
    setSelectedIds([]);
    changeQuery(query => {
      query.delete('pageToken'); query.delete('status');
      statuses.forEach(status => query.append('status', status));
    });
  };
  const fullPrompt = (group: JobGroup) => latestActiveJob(group.jobs).prompt || group.jobs.find(job => job.prompt?.trim())?.prompt;
  const setSelection = (ids: string[], selected: boolean) => setSelectedIds(previous => selected
    ? [...new Set([...previous, ...ids])] : previous.filter(id => !ids.includes(id)));
  const columns: ColumnsType<Row> = [
    { title: 'Goal', key: 'prompt', render: (_, { group, job }) => <Space direction="vertical" size={2}>
      <Link className="goal-prompt" to={`${jobPath(projectId, job.job_id)}${jobFilterSearch(params)}`} state={navigationState(group.goalId)}>
        {abbreviatePrompt(fullPrompt(group) || 'Untitled goal')}
      </Link>
      {group.jobs.length > 1 && <Text type="secondary" className="goal-attempt-count">{group.jobs.length} attempts on this page</Text>}
      <JobWaitLinks job={job} projectId={projectId} />
      {job.completion_detail && <Text type={job.completion_status?.startsWith('failed_') ? 'danger' : 'secondary'} className="job-completion">{job.completion_detail}</Text>}
    </Space> },
    { title: 'Status', width: 160, render: (_, { job, group }) => <Link to={`${jobPath(projectId, job.job_id)}${jobFilterSearch(params)}`} state={navigationState(group.goalId)} aria-label={`Job ${job.job_id}: ${jobStatus(job).label}`}><AttemptStatus job={job} /></Link> },
    { title: 'Cell', width: 120, render: (_, { job }) => cellLabel(job.cell_name) },
    { title: 'Submitted', width: 185, render: (_, { job }) => jobTime(job.submitted_at || job.created_at) || '—' },
    { title: 'Links', width: 180, render: (_, { job, group }) => <Space direction="vertical" size={2}>
      {group.goalId && <Link to={goalPath(projectId, group.goalId)} state={{ goalsReturnTo }}>Goal overview</Link>}
      {job.parent?.job_id && <Link to={jobPath(job.parent.tenant_id || projectId, job.parent.job_id)}>Parent</Link>}
      {job.repo && <RepositoryLink repo={job.repo} />}
    </Space> },
  ];

  const submit = async () => {
    let values: { prompt: string };
    try { values = await form.validateFields(); } catch { return; }
    setSubmitting(true);
    try {
      const job = await submitJob(projectId, { cell: filters.cell, inputs: { prompt: values.prompt, type: 'build' } });
      message.success('Goal created'); setSubmitOpen(false); form.resetFields();
      navigate(goalPath(projectId, job.goal_id!), { state: { goalsReturnTo } });
    } catch (reason) { message.error(reason instanceof Error ? reason.message : 'Could not create goal'); }
    finally { setSubmitting(false); }
  };

  return <div style={{ padding: 24 }}>
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <div className="goal-toolbar">
        <Title level={3} style={{ margin: 0 }}>{goalId ? 'Goal overview' : 'Goals'}</Title>
        <Space wrap>
          {goalId && <Link to={goalsReturnTo}>Back to Goals</Link>}
          {!goalId && <Select mode="multiple" aria-label="Status filter" value={filters.status} style={{ minWidth: 240 }} placeholder="All statuses" maxTagCount="responsive" allowClear
            options={Object.entries(schedulingStatuses).map(([value, status]) => ({ value, label: status.label }))} onChange={setStatuses} />}
          {!goalId && <Select aria-label="Cell filter" value={filters.cell} options={cells.map(cell => ({ value: cell.name || cell.repository_source, label: cell.name }))}
            style={{ minWidth: 160 }} placeholder="All cells" allowClear showSearch onChange={cell => {
              setSelectedIds([]); changeQuery(query => { query.delete('cell'); if (cell) query.set('cell', cell); query.delete('pageToken'); query.delete('expanded'); });
            }} />}
          <Button icon={<ReloadOutlined aria-hidden />} onClick={() => setRefresh(value => value + 1)}>Refresh</Button>
          <Button type="primary" icon={<PlusOutlined aria-hidden />} onClick={() => setSubmitOpen(true)}>New goal</Button>
        </Space>
      </div>
      {error && <Alert type="error" showIcon message={error} />}
      {goalId ? <Spin spinning={loading}>
        <section aria-label="Goal overview">
          <GoalJobOverview jobs={jobs} projectId={projectId} navigationState={navigationState(goalId)} />
        </section>
      </Spin> : <>
        <Space wrap>
          <Text>{selectedIds.length} selected</Text>
          <Button danger disabled={loading || !selectedIds.length} onClick={() => setOperation({ action: 'cancel', jobIds: selectedIds })}>Cancel selected</Button>
          <Button danger disabled={loading || !selectedIds.length} onClick={() => setOperation({ action: 'delete', jobIds: selectedIds })}>Delete selected</Button>
          {!!selectedIds.length && <Button onClick={() => setSelectedIds([])}>Clear selection</Button>}
        </Space>
        <Table<Row> rowKey="key" columns={columns} dataSource={rows} loading={loading} pagination={false} size="small" scroll={{ x: 1000 }}
          rowSelection={{
            selectedRowKeys: rows.filter(row => row.group.jobs.every(job => selectedIds.includes(job.job_id))).map(row => row.key),
            onSelect: (row, selected) => setSelection(row.group.jobs.map(job => job.job_id), selected),
            onSelectAll: (selected, _, changed) => setSelection(changed.flatMap(row => row.group.jobs.map(job => job.job_id)), selected),
            getCheckboxProps: row => ({ disabled: loading,
              indeterminate: row.group.jobs.some(job => selectedIds.includes(job.job_id)) && !row.group.jobs.every(job => selectedIds.includes(job.job_id)),
              'aria-label': row.group.jobs.length === 1 ? `Select job ${row.job.job_id}` : `Select attempts for ${abbreviatePrompt(fullPrompt(row.group) || 'Untitled goal')}` }),
          }}
          expandable={{
            expandedRowKeys: rows.filter(row => expanded.includes(row.key) || row.group.jobs.some(job => expanded.includes(job.job_id))).map(row => row.key),
            onExpand: (open, row) => changeQuery(query => {
              const ids = new Set(query.getAll('expanded')); row.group.jobs.forEach(job => ids.delete(job.job_id));
              if (open) ids.add(row.key); else ids.delete(row.key);
              query.delete('expanded'); ids.forEach(id => query.append('expanded', id));
            }, true),
            expandedRowRender: row => <GoalJobOverview jobs={row.group.jobs} projectId={projectId} navigationState={navigationState(row.group.goalId)}
              selectedIds={selectedIds} onSelect={setSelection} />,
            columnWidth: 40,
          }} />
      </>}
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
