import { cellLabel } from '../utils/cellPresentation';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Alert, Button, Form, Input, Modal, Space, Spin, Table, Typography, message, theme } from 'antd';
import { FilterFilled, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import type { ColumnsType, TableProps } from 'antd/es/table';
import type { SortOrder } from 'antd/es/table/interface';
import { listCells, listJobs, submitJob } from '@colony2/shared/api';
import type { CortexCell, RecipeJobSummary } from '@colony2/shared/types';
import { jobStatus, jobStatusOptions } from '../utils/jobStatus';
import { jobTime } from './JobListDetails';
import GoalJobOverview, { AttemptStatus } from './GoalJobOverview';
import { abbreviatePrompt, jobTimestamp, latestActiveJob } from '../utils/goalPresentation';
import { jobFilterSearch, readJobFilters } from '../utils/jobFilters';
import JobActionDialog, { type JobAction } from './JobActionDialog';
import JobWaitLinks, { goalPath, jobPath } from './JobWaitLinks';
import JobCompletionMessage from './JobCompletionMessage';
import JobExecutionRequirements from './JobExecutionRequirements';
import './JobsListPage.css';

const { Text, Title } = Typography;
type Props = { projectId: string; goalId?: string };
type JobGroup = { key: string; goalId?: string; jobs: RecipeJobSummary[] };
type Row = { key: string; group: JobGroup; job: RecipeJobSummary };
export default function JobsListPage(props: Props) {
  return <JobsList key={`${props.projectId}/${props.goalId || ''}`} {...props} />;
}

function JobsList({ projectId, goalId }: Props) {
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const location = useLocation();
  const homePath = `/project/${encodeURIComponent(projectId)}/goals`;
  const savedHome = location.state?.goalsReturnTo;
  const goalsReturnTo = goalId
    ? (typeof savedHome === 'string' && [homePath, `/project/${encodeURIComponent(projectId)}/jobs`].includes(savedHome.split('?')[0]) ? savedHome : homePath)
    : location.pathname + location.search;
  const navigationState = (id?: string) => ({ goalsReturnTo, goalId: id,
    ...(goalId ? { goalReturnTo: location.pathname + location.search } : {}) });
  const [jobs, setJobs] = useState<RecipeJobSummary[]>([]);
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
  const listRequest = useRef<{ key: string; promise: Promise<[CortexCell[], Awaited<ReturnType<typeof listJobs>>]> }>();
  const [params, setParams] = useSearchParams();
  const filters = readJobFilters(params);
  const pageToken = params.get('pageToken') || undefined;
  const statusesKey = JSON.stringify(filters.status);
  const cellsKey = JSON.stringify(filters.cell);
  const noStatuses = !goalId && filters.status?.length === 0;
  const noCells = !goalId && filters.cell?.length === 0;
  const submissionCell = filters.cell?.length === 1 ? filters.cell[0] : undefined;
  const search = params.get('q') || '';
  const searchTerm = search.trim().replace(/\s+/g, ' ').toLocaleLowerCase();
  const sortOrder = (column: string): SortOrder => params.get('sort') === column
    && (params.get('order') === 'ascend' || params.get('order') === 'descend') ? params.get('order') as SortOrder : null;
  const expanded = params.getAll('expanded');
  const [form] = Form.useForm<{ prompt: string }>();
  const changeQuery = (change: (query: URLSearchParams) => void, replace = false) => setParams(previous => {
    const query = new URLSearchParams(previous); change(query); return query;
  }, { replace });

  // Also clear selections when browser history restores a different search.
  useEffect(() => { setSelectedIds([]); }, [search]);

  useEffect(() => {
    const version = ++loadVersion.current;
    setLoading(true); setError(undefined);
    (async () => {
      const key = JSON.stringify([projectId, goalId, statusesKey, cellsKey, pageToken, refresh]);
      if (listRequest.current?.key !== key) listRequest.current = { key, promise: Promise.all([
        listCells(projectId), noStatuses || noCells ? Promise.resolve({ jobs: [] }) : listJobs(projectId, { goal: goalId, displayStatus: goalId ? [] : JSON.parse(statusesKey) || [], cell: goalId ? undefined : JSON.parse(cellsKey) || undefined, pageToken }),
      ]) };
      const [cellData, page] = await listRequest.current.promise;
      if (version !== loadVersion.current) return;
      setCells(cellData); setJobs(page.jobs); setNextPage(page.next_page_token);
      setSelectedIds(previous => previous.filter(id => page.jobs.some(job => job.job_id === id)));
    })().catch(reason => {
      if (version === loadVersion.current) setError(reason instanceof Error ? reason.message : 'Could not load goals');
    }).finally(() => { if (version === loadVersion.current) setLoading(false); });
    return () => { loadVersion.current++; };
  }, [projectId, goalId, cellsKey, pageToken, statusesKey, noStatuses, noCells, refresh]);

  // Group only the matching jobs returned by the server. A goal link lists all
  // its attempts; a page is not an authoritative goal count or goal state.
  const groups = useMemo(() => {
    const grouped = new Map<string, JobGroup>();
    for (const job of jobs) {
      const goals = goalId ? [goalId] : job.goal_id ? [job.goal_id] : [];
      for (const id of goals.length ? goals : [undefined]) {
        const key = id ? `goal:${id}` : `job:${job.job_id}`;
        const group = grouped.get(key) || { key, goalId: id, jobs: [] };
        group.jobs.push(job);
        grouped.set(key, group);
      }
    }
    return [...grouped.values()];
  }, [jobs, goalId]);
  // Search complete prompts (including other attempts), even when the row uses
  // a title or an abbreviated prompt. Keep the entire matching goal together.
  const rows: Row[] = noStatuses || noCells ? [] : groups.filter(group => !searchTerm || group.jobs.some(job =>
    [job.prompt, job.title].some(text => text?.replace(/\s+/g, ' ').toLocaleLowerCase().includes(searchTerm)),
  )).map(group => ({ key: group.key, group, job: latestActiveJob(group.jobs) }));
  const statusOptions = jobStatusOptions(jobs, filters.status || []);
  const cellOptions = [...new Set([...cells.map(cell => cell.name || cell.repository_source), ...filters.cell || []])]
    .map(value => ({ value, label: value }));
  const checkboxFilter = (label: string, options: { value: string; label: string }[], selected: string[] | null) => ({
    filters: options.map(option => ({ text: option.label, value: option.value })),
    filteredValue: selected ?? options.map(option => option.value), filtered: selected !== null,
    filterMode: 'tree' as const, filterSearch: true, filterOnClose: false,
    filterIcon: <FilterFilled aria-label={label} style={{ color: selected === null ? token.colorTextDescription : token.colorPrimary }} tabIndex={0}
      onKeyDown={(event: React.KeyboardEvent<HTMLSpanElement>) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); event.stopPropagation(); event.currentTarget.click(); } }} />,
  });
  const fullPrompt = (group: JobGroup) => latestActiveJob(group.jobs).title || latestActiveJob(group.jobs).prompt || group.jobs.find(job => job.prompt?.trim())?.prompt;
  const setSelection = (ids: string[], selected: boolean) => setSelectedIds(previous => selected
    ? [...new Set([...previous, ...ids])] : previous.filter(id => !ids.includes(id)));
  const onTableChange: TableProps<Row>['onChange'] = (_, columnFilters, sorter, extra) => {
    if (extra.action === 'filter') {
      const statuses = columnFilters.status?.map(String) || [];
      const selectedStatuses = statuses.length === statusOptions.length ? null : statuses;
      const cells = columnFilters.cell?.map(String) || [];
      const selectedCells = cells.length === cellOptions.length ? null : cells;
      setSelectedIds([]);
      changeQuery(query => {
        query.delete('pageToken'); query.delete('status'); query.delete('cell'); query.delete('cellSelection');
        if (selectedStatuses?.length === 0) query.set('status', 'none');
        selectedStatuses?.forEach(status => query.append('status', status));
        if (selectedCells?.length === 0) query.set('cellSelection', 'none');
        selectedCells?.forEach(cell => query.append('cell', cell));
        if (JSON.stringify(selectedCells) !== cellsKey) query.delete('expanded');
      });
    } else if (extra.action === 'sort') {
      const active = Array.isArray(sorter) ? sorter[0] : sorter;
      changeQuery(query => {
        query.delete('sort'); query.delete('order');
        if (active.order && active.columnKey) {
          query.set('sort', String(active.columnKey)); query.set('order', active.order);
        }
      });
    }
  };
  const columns: ColumnsType<Row> = [
    { title: 'Goal', key: 'prompt', sorter: (a, b) => (fullPrompt(a.group) || 'Untitled goal').localeCompare(fullPrompt(b.group) || 'Untitled goal'), sortOrder: sortOrder('prompt'), render: (_, { group, job }) => <Space direction="vertical" size={2}>
      <Link className="goal-prompt" to={`${jobPath(projectId, job.job_id)}${jobFilterSearch(params)}`} state={navigationState(group.goalId)}>
        {abbreviatePrompt(fullPrompt(group) || 'Untitled goal')}
      </Link>
      {group.jobs.length > 1 && <Text type="secondary" className="goal-attempt-count">{group.jobs.length} attempts on this page</Text>}
      <JobWaitLinks job={job} projectId={projectId} />
      <JobCompletionMessage job={job} />
      <JobExecutionRequirements execution={job.execution} />
    </Space> },
    { title: 'Status', key: 'status', width: 180,
      sorter: (a, b) => jobStatus(a.job).label.localeCompare(jobStatus(b.job).label), sortOrder: sortOrder('status'),
      ...checkboxFilter('Filter statuses', statusOptions, filters.status),
      render: (_, { job, group }) => <Link to={`${jobPath(projectId, job.job_id)}${jobFilterSearch(params)}`} state={navigationState(group.goalId)} aria-label={`Job ${job.job_id}: ${jobStatus(job).label}`}><AttemptStatus job={job} /></Link> },
    { title: 'Cell', key: 'cell', width: 140, sorter: (a, b) => cellLabel(a.job.cell_name).localeCompare(cellLabel(b.job.cell_name)), sortOrder: sortOrder('cell'),
      ...checkboxFilter('Filter cells', cellOptions, filters.cell), render: (_, { job }) => cellLabel(job.cell_name) },
    { title: 'Submitted', key: 'submitted', width: 185, sorter: (a, b) => jobTimestamp(a.job) - jobTimestamp(b.job), sortOrder: sortOrder('submitted'), render: (_, { job }) => jobTime(job.submitted_at || job.created_at) || '—' },
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
      const job = await submitJob(projectId, { cell: submissionCell, inputs: { prompt: values.prompt, type: 'build' } });
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
          <Button icon={<ReloadOutlined aria-hidden />} onClick={() => setRefresh(value => value + 1)}>Refresh</Button>
          <Button type="primary" icon={<PlusOutlined aria-hidden />} onClick={() => setSubmitOpen(true)}>New goal</Button>
        </Space>
      </div>
      {!goalId && <Text type="secondary">Search and sorting apply to the current page.</Text>}
      {error && <Alert type="error" showIcon message={error} />}
      {goalId ? <Spin spinning={loading}>
        <section aria-label="Goal overview">
          <GoalJobOverview jobs={jobs} projectId={projectId} refresh={refresh} navigationState={navigationState(goalId)} />
        </section>
      </Spin> : <>
        <div className="goal-list-actions">
          <Space wrap>
            <Text>{selectedIds.length} selected</Text>
            <Button danger disabled={loading || !selectedIds.length} onClick={() => setOperation({ action: 'cancel', jobIds: selectedIds })}>Cancel selected</Button>
            <Button danger disabled={loading || !selectedIds.length} onClick={() => setOperation({ action: 'delete', jobIds: selectedIds })}>Delete selected</Button>
            {!!selectedIds.length && <Button onClick={() => setSelectedIds([])}>Clear selection</Button>}
          </Space>
          <Input className="goal-search" aria-label="Search goals" placeholder="Search full prompts" prefix={<SearchOutlined aria-hidden />} allowClear value={search}
            onChange={event => {
              const value = event.target.value;
              changeQuery(query => { if (value) query.set('q', value); else query.delete('q'); }, true);
            }} />
        </div>
        <Table<Row> rowKey="key" columns={columns} dataSource={rows} loading={loading} pagination={false} size="small" scroll={{ x: 1000 }}
          onChange={onTableChange} locale={{ filterCheckAll: 'Select all', filterReset: 'Select none', filterConfirm: 'Apply',
            emptyText: noStatuses ? 'No statuses selected' : noCells ? 'No cells selected' : searchTerm ? 'No goals match this search on this page' : undefined }}
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
            expandedRowRender: row => (expanded.includes(row.key) || row.group.jobs.some(job => expanded.includes(job.job_id))) ? <GoalJobOverview jobs={row.group.jobs} projectId={projectId} refresh={refresh} navigationState={navigationState(row.group.goalId)}
              selectedIds={selectedIds} onSelect={setSelection} /> : null,
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
      <Text type="secondary">Cell: {submissionCell || cells.find(cell => cell.kind === 'self')?.name || 'self'}</Text>
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
