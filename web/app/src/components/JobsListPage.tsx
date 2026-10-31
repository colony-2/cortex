import { cellFilterValue, cellLabel } from '../utils/cellPresentation';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { Alert, Button, Card, Checkbox, Empty, Input, Space, Spin, Table, Typography, theme } from 'antd';
import { DownOutlined, UpOutlined, FilterFilled, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import type { ColumnsType, TableProps } from 'antd/es/table';
import type { SortOrder } from 'antd/es/table/interface';
import { getJob, listJobs } from '@colony2/shared/api';
import type { RecipeJobSummary } from '@colony2/shared/types';
import { jobStatus, jobStatusOptions } from '../utils/jobStatus';
import { jobTime } from './JobListDetails';
import GoalJobOverview, { AttemptStatus } from './GoalJobOverview';
import { abbreviatePrompt, jobTimestamp, latestActiveJob } from '../utils/goalPresentation';
import { jobFilterSearch, nextFilterSelection, readJobFilters } from '../utils/jobFilters';
import JobActionDialog, { type JobAction } from './JobActionDialog';
import JobWaitLinks, { goalPath, jobPath } from './JobWaitLinks';
import JobCompletionMessage from './JobCompletionMessage';
import JobExecutionRequirements from './JobExecutionRequirements';
import './JobsListPage.css';
import NewGoalDialog from './NewGoalDialog';
import CompactFilter from './CompactFilter';
import { useCompact } from '../hooks/useCompact';

const { Text, Title } = Typography;
type Props = { projectId: string; goalId?: string };
type JobGroup = { key: string; goalId?: string; jobs: RecipeJobSummary[] };
type Row = { key: string; group: JobGroup; job: RecipeJobSummary };
export default function JobsListPage(props: Props) {
  return <JobsList key={`${props.projectId}/${props.goalId || ''}`} {...props} />;
}

function JobsList({ projectId, goalId }: Props) {
  const { token } = theme.useToken();
  const compact = useCompact();
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
  const [nextPage, setNextPage] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [controlsOpen, setControlsOpen] = useState(false);
  const [submitOpen, setSubmitOpen] = useState(false);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [operation, setOperation] = useState<{ action: JobAction; jobIds: string[] } | null>(null);
  const [refresh, setRefresh] = useState(0);
  const loadVersion = useRef(0);
  const inspectionRequests = useRef(new Map<string, ReturnType<typeof getJob>>());
  const listRequest = useRef<{ key: string; promise: ReturnType<typeof listJobs> }>();
  const [params, setParams] = useSearchParams();
  const filters = useMemo(() => readJobFilters(params), [params]);
  const pageToken = params.get('pageToken') || undefined;
  const statusesKey = JSON.stringify(filters.status);
  const cellsKey = JSON.stringify(filters.cell);
  const noStatuses = !goalId && filters.status?.length === 0;
  const noCells = !goalId && filters.cell?.length === 0;
  const search = params.get('q') || '';
  const searchTerm = search.trim().replace(/\s+/g, ' ').toLocaleLowerCase();
  const sortOrder = (column: string): SortOrder => params.get('sort') === column
    && (params.get('order') === 'ascend' || params.get('order') === 'descend') ? params.get('order') as SortOrder : null;
  const expanded = params.getAll('expanded');
  const changeQuery = (change: (query: URLSearchParams) => void, replace = false) => setParams(previous => {
    const query = new URLSearchParams(previous); change(query); return query;
  }, { replace });

  // Also clear selections when browser history restores a different local filter.
  useEffect(() => { setSelectedIds([]); }, [search, cellsKey]);

  useEffect(() => {
    const version = ++loadVersion.current;
    setLoading(true); setError(undefined);
    (async () => {
      const key = JSON.stringify([projectId, goalId, statusesKey, pageToken, refresh]);
      if (listRequest.current?.key !== key) listRequest.current = { key, promise: noStatuses ? Promise.resolve({ jobs: [] })
        : listJobs(projectId, { goal: goalId, displayStatus: goalId ? [] : JSON.parse(statusesKey) || [], pageToken }) };
      const page = await listRequest.current.promise;
      if (version !== loadVersion.current) return;
      setJobs(page.jobs); setNextPage(page.next_page_token);
      setSelectedIds(previous => previous.filter(id => page.jobs.some(job => job.job_id === id)));
    })().catch(reason => {
      if (version === loadVersion.current) setError(reason instanceof Error ? reason.message : 'Could not load goals');
    }).finally(() => { if (version === loadVersion.current) setLoading(false); });
    return () => { loadVersion.current++; };
  }, [projectId, goalId, pageToken, statusesKey, noStatuses, refresh]);

  // Cell choices come from the loaded jobs, independent of local configuration.
  // Filter attempts before grouping, matching the status filter's grouping behavior.
  // A goal link lists all its attempts; a page is not an authoritative goal count or state.
  const groups = useMemo(() => {
    const grouped = new Map<string, JobGroup>();
    for (const job of jobs) {
      if (!goalId && filters.cell !== null && !filters.cell.includes(cellFilterValue(job.cell_name))) continue;
      const goals = goalId ? [goalId] : job.goal_id ? [job.goal_id] : [];
      for (const id of goals.length ? goals : [undefined]) {
        const key = id ? `goal:${id}` : `job:${job.job_id}`;
        const group = grouped.get(key) || { key, goalId: id, jobs: [] };
        group.jobs.push(job);
        grouped.set(key, group);
      }
    }
    return [...grouped.values()];
  }, [jobs, goalId, filters.cell]);
  // Search complete prompts (including other attempts), even when the row uses
  // a title or an abbreviated prompt. Keep the entire matching goal together.
  const rows: Row[] = noStatuses || noCells ? [] : groups.filter(group => !searchTerm || group.jobs.some(job =>
    [job.prompt, job.title].some(text => text?.replace(/\s+/g, ' ').toLocaleLowerCase().includes(searchTerm)),
  )).map(group => ({ key: group.key, group, job: latestActiveJob(group.jobs) }));
  const statusOptions = jobStatusOptions(jobs, filters.status || []);
  // Use the unfiltered page so unchecked cells stay available; preserve selections
  // from saved links even when another page/status has no jobs in those cells.
  const cellOptions = [...new Set([...jobs.map(job => cellFilterValue(job.cell_name)), ...filters.cell || []])]
    .sort((a, b) => cellLabel(a).localeCompare(cellLabel(b))).map(value => ({ value, label: cellLabel(value) }));
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
      const selectedStatuses = nextFilterSelection(statuses, statusOptions.map(option => option.value), filters.status);
      const cells = columnFilters.cell?.map(String) || [];
      const selectedCells = nextFilterSelection(cells, cellOptions.map(option => option.value), filters.cell);
      setSelectedIds([]);
      changeQuery(query => {
        if (JSON.stringify(selectedStatuses) !== statusesKey) query.delete('pageToken');
        query.delete('status'); query.delete('cell'); query.delete('cellSelection');
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
      <Link className="goal-prompt compact-touch-link" to={`${jobPath(projectId, job.job_id)}${jobFilterSearch(params)}`} state={navigationState(group.goalId)}>
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
      render: (_, { job, group }) => <Link className="compact-touch-link" to={`${jobPath(projectId, job.job_id)}${jobFilterSearch(params)}`} state={navigationState(group.goalId)} aria-label={`Job ${job.job_id}: ${jobStatus(job).label}`}><AttemptStatus job={job} /></Link> },
    { title: 'Cell', key: 'cell', width: 140, sorter: (a, b) => cellLabel(a.job.cell_name).localeCompare(cellLabel(b.job.cell_name)), sortOrder: sortOrder('cell'),
      ...checkboxFilter('Filter cells', cellOptions, filters.cell), render: (_, { job }) => cellLabel(job.cell_name) },
    { title: 'Submitted', key: 'submitted', width: 185, sorter: (a, b) => jobTimestamp(a.job) - jobTimestamp(b.job), sortOrder: sortOrder('submitted'), render: (_, { job }) => jobTime(job.submitted_at || job.created_at) || '—' },
    { title: 'Links', width: 180, render: (_, { job, group }) => <Space direction="vertical" size={2}>
      {group.goalId && <Link className="compact-touch-link" to={goalPath(projectId, group.goalId)} state={{ goalsReturnTo }}>Goal overview</Link>}
      {job.parent?.job_id && <Link className="compact-touch-link" to={jobPath(job.parent.tenant_id || projectId, job.parent.job_id)}>Parent</Link>}
      {job.repo && <RepositoryLink repo={job.repo} />}
    </Space> },
  ];

  const isExpanded = (row: Row) => expanded.includes(row.key) || row.group.jobs.some(job => expanded.includes(job.job_id));
  const expand = (open: boolean, row: Row) => changeQuery(query => {
    const ids = new Set(query.getAll('expanded')); row.group.jobs.forEach(job => ids.delete(job.job_id));
    if (open) ids.add(row.key); else ids.delete(row.key);
    query.delete('expanded'); ids.forEach(id => query.append('expanded', id));
  }, true);
  const details = (row: Row) => <GoalJobOverview inspectionRequests={inspectionRequests.current} jobs={row.group.jobs} projectId={projectId} refresh={refresh} navigationState={navigationState(row.group.goalId)} selectedIds={selectedIds} onSelect={setSelection} />;
  const visibleInspectionIds = (goalId ? [{ jobs }] : rows.filter(isExpanded).map(row => row.group))
    .map(group => group.jobs.find(job => params.getAll('inspect').includes(job.job_id)) || latestActiveJob(group.jobs))
    .filter(Boolean).map(job => job.job_id);
  const visibleInspectionKey = JSON.stringify(visibleInspectionIds);
  useEffect(() => {
    // Keep requests across presentation changes, but revalidate a selection
    // after it has actually been hidden, closed, or replaced.
    for (const key of inspectionRequests.current.keys()) {
      const [, id, revision] = JSON.parse(key);
      if (!visibleInspectionIds.includes(id) || revision !== refresh) inspectionRequests.current.delete(key);
    }
  }, [visibleInspectionKey, refresh]);
  const mobileSort = columns.find(column => column.key === params.get('sort'));
  const mobileRows = [...rows];
  if (mobileSort && typeof mobileSort.sorter === 'function' && sortOrder(String(mobileSort.key))) {
    const compare = mobileSort.sorter;
    mobileRows.sort((a, b) => compare(a, b, sortOrder(String(mobileSort.key))) * (sortOrder(String(mobileSort.key)) === 'descend' ? -1 : 1));
  }
  const applyCompactFilter = (key: 'status' | 'cell', values: string[]) => {
    // Reuse the table's filter transition, including URL and selection semantics.
    onTableChange?.({}, {
      status: key === 'status' ? values : filters.status ?? statusOptions.map(option => option.value),
      cell: key === 'cell' ? values : filters.cell ?? cellOptions.map(option => option.value),
    }, {}, { action: 'filter', currentDataSource: rows });
  };
  const setSort = (key: string, order: string) => changeQuery(query => {
    query.delete('sort'); query.delete('order');
    if (key) { query.set('sort', key); query.set('order', order); }
  });

  return <div className="app-page" style={{ padding: 24 }}>
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <div className="goal-toolbar">
        <Title level={3} style={{ margin: 0 }}>{goalId ? 'Goal overview' : 'Goals'}</Title>
        <Space wrap>
          {goalId && <Link className="compact-touch-link" to={goalsReturnTo}>Back to Goals</Link>}
          {!compact && <Button icon={<ReloadOutlined aria-hidden />} onClick={() => setRefresh(value => value + 1)}>Refresh</Button>}
          <Button type="primary" icon={<PlusOutlined aria-hidden />} onClick={() => setSubmitOpen(true)}>New goal</Button>
        </Space>
      </div>
      {!goalId && !compact && <Text type="secondary">Search, cell filtering, and sorting apply to the current page.</Text>}
      {error && <Alert type="error" showIcon message={error} />}
      {goalId ? <Spin spinning={loading}>
        <section aria-label="Goal overview">
          <GoalJobOverview inspectionRequests={inspectionRequests.current} jobs={jobs} projectId={projectId} refresh={refresh} navigationState={navigationState(goalId)} />
        </section>
      </Spin> : <>
        {compact && <Button aria-expanded={controlsOpen} onClick={() => setControlsOpen(value => !value)}>Filters and sorting{filters.cell !== null || filters.status !== null || search || params.get('sort') ? ' (active)' : ''}</Button>}
        <div className="goal-list-actions">
          {(!compact || selectedIds.length > 0) && <Space wrap>
            <Text>{selectedIds.length} selected</Text>
            <Button danger disabled={loading || !selectedIds.length} onClick={() => setOperation({ action: 'cancel', jobIds: selectedIds })}>Cancel selected</Button>
            <Button danger disabled={loading || !selectedIds.length} onClick={() => setOperation({ action: 'delete', jobIds: selectedIds })}>Delete selected</Button>
            {!!selectedIds.length && <Button onClick={() => setSelectedIds([])}>Clear selection</Button>}
          </Space>}
          <div hidden={compact && !controlsOpen} className="goal-search-control"><Input className="goal-search" aria-label="Search goals" placeholder="Search full prompts" prefix={<SearchOutlined aria-hidden />} allowClear value={search}
            onChange={event => {
              const value = event.target.value;
              changeQuery(query => { if (value) query.set('q', value); else query.delete('q'); }, true);
            }} /></div>
        </div>
        {compact ? <Spin spinning={loading}>
          <div hidden={!controlsOpen}><Space wrap className="compact-controls">
            <CompactFilter label="Filter statuses" options={statusOptions} selected={filters.status} onApply={values => applyCompactFilter('status', values)} />
            <CompactFilter label="Filter cells" options={cellOptions} selected={filters.cell} onApply={values => applyCompactFilter('cell', values)} />
            <select className="compact-sort" aria-label="Sort goals" value={params.get('sort') || ''} onChange={event => setSort(event.target.value, params.get('order') || 'ascend')}>
              <option value="">Original order</option><option value="prompt">Goal</option><option value="status">Status</option><option value="cell">Cell</option><option value="submitted">Submitted</option>
            </select>
            <select className="compact-sort" aria-label="Sort direction" value={params.get('order') || 'ascend'} disabled={!params.get('sort')} onChange={event => setSort(params.get('sort') || '', event.target.value)}><option value="ascend">Ascending</option><option value="descend">Descending</option></select>
            <Button icon={<ReloadOutlined aria-hidden />} onClick={() => setRefresh(value => value + 1)}>Refresh</Button>
          </Space><p>Search, cell filtering, and sorting apply to the current page.</p></div>
          {!!rows.length && <Checkbox disabled={loading} checked={rows.every(row => row.group.jobs.every(job => selectedIds.includes(job.job_id)))} indeterminate={!!selectedIds.length && !rows.every(row => row.group.jobs.every(job => selectedIds.includes(job.job_id)))} onChange={event => setSelection(rows.flatMap(row => row.group.jobs.map(job => job.job_id)), event.target.checked)}>Select all goals on this page</Checkbox>}
          <div className="compact-cards" aria-label="Goals">
            {mobileRows.map((row, index) => <Card key={row.key} size="small" className="goal-card">
              <div className="goal-card-summary">
                <Checkbox disabled={loading} checked={row.group.jobs.every(job => selectedIds.includes(job.job_id))} indeterminate={row.group.jobs.some(job => selectedIds.includes(job.job_id)) && !row.group.jobs.every(job => selectedIds.includes(job.job_id))} onChange={event => setSelection(row.group.jobs.map(job => job.job_id), event.target.checked)} aria-label={row.group.jobs.length === 1 ? `Select job ${row.job.job_id}` : `Select attempts for ${abbreviatePrompt(fullPrompt(row.group) || 'Untitled goal')}`} />
                <Link className="goal-prompt compact-touch-link" to={`${jobPath(projectId, row.job.job_id)}${jobFilterSearch(params)}`} state={navigationState(row.group.goalId)}>
                  <span className="goal-card-title">{abbreviatePrompt(fullPrompt(row.group) || 'Untitled goal')}</span>
                </Link>
                <Button aria-label={`${isExpanded(row) ? 'Collapse' : 'Expand'} goal details`} aria-expanded={isExpanded(row)} icon={isExpanded(row) ? <UpOutlined /> : <DownOutlined />} onClick={() => expand(!isExpanded(row), row)} />
              </div>
              <div className="goal-card-meta">
                {columns[1].render?.(undefined, row, index) as React.ReactNode}
                <Text type="secondary">{cellLabel(row.job.cell_name)}</Text>
                {row.group.jobs.length > 1 && <Text type="secondary" className="goal-attempt-count">{row.group.jobs.length} attempts on this page</Text>}
              </div>
              <JobWaitLinks job={row.job} projectId={projectId} />
              {isExpanded(row) && <div className="goal-card-secondary">
                {columns.slice(3).map((column, columnIndex) => <div key={columnIndex} className="goal-card-field"><Text type="secondary">{String(column.title)}: </Text>{column.render?.(undefined, row, index) as React.ReactNode}</div>)}
                <JobCompletionMessage job={row.job} /><JobExecutionRequirements execution={row.job.execution} />
              </div>}
              {isExpanded(row) && details(row)}
            </Card>)}
            {!rows.length && !loading && <Empty description={noStatuses ? 'No statuses selected' : noCells ? 'No cells selected' : searchTerm ? 'No goals match this search on this page' : 'No goals found'} />}
          </div>
        </Spin> :         <Table<Row> rowKey="key" columns={columns} dataSource={rows} loading={loading} pagination={false} size="small" scroll={{ x: 1000 }}
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
            onExpand: expand,
            expandedRowRender: row => isExpanded(row) ? details(row) : null,
            columnWidth: 40,
          }} />}
      </>}
      <Space>
        {pageToken && <Button onClick={() => changeQuery(query => { query.delete('pageToken'); })}>First page</Button>}
        {nextPage && <Button disabled={loading} onClick={() => changeQuery(query => { query.set('pageToken', nextPage); })}>Next jobs</Button>}
      </Space>
    </Space>
    {operation && <JobActionDialog projectId={projectId} {...operation} onClose={() => setOperation(null)} onComplete={async succeeded => {
      setSelectedIds(previous => previous.filter(id => !succeeded.includes(id))); setRefresh(value => value + 1);
    }} />}
    {submitOpen && <NewGoalDialog projectId={projectId} jobs={jobs} filters={filters.cell} onClose={() => setSubmitOpen(false)} onCreated={id => {
      setSubmitOpen(false); navigate(goalPath(projectId, id), { state: { goalsReturnTo } });
    }} />}
  </div>;
}

export function RepositoryLink({ repo }: { repo: string }) {
  // Never turn arbitrary git selectors into executable browser URLs.
  const url = repo.replace(/^git\+/, '');
  const label = repo.replace(/\/$/, '').split('/').slice(-1)[0]?.replace(/\.git$/, '') || repo;
  return /^https?:\/\//i.test(url) ? <a className="compact-touch-link" href={url} target="_blank" rel="noreferrer" title={repo}>{label}</a> : <Text title={repo}>{label}</Text>;
}
