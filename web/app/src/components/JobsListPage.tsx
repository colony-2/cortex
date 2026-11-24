import './JobsListPage.css';
import { useEffect, useMemo, useState } from 'react';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { Alert, Button, Checkbox, Dropdown, Empty, Spin, Tag, Tooltip } from 'antd';
import { InboxOutlined, ReloadOutlined, SortAscendingOutlined } from '@ant-design/icons';
import { useJobInbox } from '../hooks/useJobInbox';
import { formatInboxUpdate, inboxDate, type InboxJob } from '../utils/jobInboxData';
import { inboxFolder, inboxSortOptions, isInInbox, readInboxSort, threadTitle, type InboxSelection } from '../utils/jobInbox';
import { useSavedReviewDrafts } from '../hooks/useReviewDraft';
import { canArchiveImmediately, isFinishedJob, jobStatus, jobStatusValue } from '../utils/jobStatus';
import { matchesSearchTerms, searchTerms } from '../utils/searchTerms';
import JobActionDialog, { type JobAction } from './JobActionDialog';
import CopyJobId from './CopyJobId';
import JobExecutionRequirements from './JobExecutionRequirements';
import InboxSelectionToolbar, { type InboxSelectionMode } from './InboxSelectionToolbar';

const byText = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true }).compare;

export default function JobsListPage({ projectId }: { projectId: string; goalId?: string }) {
  const inbox = useJobInbox();
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const [operation, setOperation] = useState<{action: JobAction; ids: string[]; job?: InboxJob}>();
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const folder = inboxFolder(params.get('inbox')), cellId = params.get('cellId');
  const draftJobs = new Set(useSavedReviewDrafts(projectId).map(draft => draft.job));
  const search = params.get('q') ?? '';
  const sort = readInboxSort(params.get('sort')) ?? 'newest';
  useEffect(() => { setSelected(new Set()); }, [location.pathname, folder, cellId, search]);
  const folderLabels = {reviews:'Reviews',active:'Active',waiting:'Waiting',failed:'Failed',archive:'Archive',all:'All',drafts:'Drafts'};
  const label = folder === 'cell' ? inbox.ranked.find(cell => cell.id === cellId)?.name ?? 'Cell inbox' : folderLabels[folder];
  const pendingByJob = useMemo(() => new Map(inbox.pending.map(input => [input.id, input])), [inbox.pending]);
  const rows = useMemo(() => {
    const terms = searchTerms(search);
    const selection: InboxSelection = folder === 'cell' ? { kind: 'cell', cellId: cellId ?? '' }
      : { kind: folder };
    return inbox.jobs.filter(job => isInInbox({ archived: job.inboxArchived, cellId: job.cell_id,
      waiting: pendingByJob.has(job.job_id) || (folder === 'active' && job.status === 'READY' && inbox.counts.status !== 'current'), status: jobStatusValue(job) }, selection)
      && [job.title, job.prompt, job.cell_name, job.job_id].some(text => matchesSearchTerms(terms, text))).map(job => {
      const pending = pendingByJob.get(job.job_id);
      const date = inboxDate(job, pending?.requested_at);
      const time = date === undefined ? undefined : Date.parse(date);
      return { job, date, time, title: threadTitle(job), status: jobStatus(job, !job.inboxArchived && !!pending) };
    }).sort((a, b) => {
      const dimension = sort === 'cell' ? byText(a.job.cell_name, b.job.cell_name)
        : sort === 'status' ? byText(a.status.label, b.status.label)
        : sort === 'subject' ? byText(a.title, b.title) : 0;
      const missing = Number(a.time === undefined) - Number(b.time === undefined);
      const chronological = ((b.time ?? 0) - (a.time ?? 0)) * (sort === 'oldest' ? -1 : 1);
      return dimension || missing || chronological || a.job.job_id.localeCompare(b.job.job_id);
    });
  }, [inbox.jobs, inbox.counts.status, pendingByJob, folder, cellId, search, sort]);
  const selectedJobs = rows.filter(({ job }) => selected.has(job.job_id)).map(({ job }) => job);
  const cancellable = selectedJobs.filter(job => !isFinishedJob(job));
  const selectJobs = (mode: InboxSelectionMode) => {
    const matching = mode === 'all' ? rows : mode === 'none' ? [] : rows.filter(({ job }) => isFinishedJob(job) === (mode === 'finished'));
    setSelected(new Set(matching.map(({ job }) => job.job_id)));
  };
  const today = new Date();
  const reviewsUnavailable = folder === 'reviews' && inbox.counts.status === 'unavailable';
  return <div className="app-page inbox-page">
    <div className="inbox-list-toolbar"><h1>{label}</h1><span className="inbox-list-count">{inbox.loaded ? `${rows.length} jobs${inbox.nextPageToken ? ' shown' : ''}` : inbox.loading ? 'Loading jobs…' : ''}</span>
      <Dropdown trigger={['click']} menu={{ items: inboxSortOptions, selectable: true, selectedKeys: [sort], onClick: ({ key }) => {
        setParams(previous => {
          const next = new URLSearchParams(previous);
          next.set('sort', key);
          return next;
        }, { replace: true });
      } }}><Button type="text" className="inbox-sort" aria-label="Sort threads" icon={<SortAscendingOutlined aria-hidden />}>Sort</Button></Dropdown>
    </div>
    {inbox.error && <Alert type="warning" showIcon message={inbox.loaded ? 'Inbox refresh failed; showing the previous result' : 'Inbox unavailable'} description={inbox.error} action={<Button onClick={inbox.retry}>Retry</Button>} />}
    {reviewsUnavailable && !!rows.length && <Alert type="warning" showIcon message="Showing last known review requests" action={<Button onClick={inbox.refresh}>Retry</Button>} />}
    <InboxSelectionToolbar items={rows.length} selected={selectedJobs.length} kind="jobs" disabled={!!operation} onSelect={selectJobs}>
      {!selectedJobs.length || folder === 'archive'
        ? <Button type="text" icon={<ReloadOutlined />} aria-label="Refresh inbox" loading={inbox.loading} onClick={inbox.refresh} />
        : <>
          {cancellable.length > 0 && <Button type="text" aria-label="Cancel selected" disabled={!!operation} onClick={() => setOperation({ action: 'cancel', ids: cancellable.map(job => job.job_id) })}>
            <span>Cancel<span className="inbox-action-suffix"> selected</span></span>
          </Button>}
          <Button type="text" aria-label="Archive selected" disabled={!!operation} onClick={() => setOperation({ action: 'archive', ids: selectedJobs.map(job => job.job_id), job: selectedJobs.length === 1 ? selectedJobs[0] : undefined })}>
            <span>Archive<span className="inbox-action-suffix"> selected</span></span>
          </Button>
        </>}
    </InboxSelectionToolbar>
    <Spin spinning={inbox.loading && !inbox.loaded}>
      <ul className="inbox-job-list" aria-label="Jobs">
        {rows.map(({ job, date, status, title }) => {
          const unread = inbox.isUnread(job);
          const checked = selected.has(job.job_id);
          return <li key={job.job_id} className={`inbox-job-row${unread ? ' unread' : ''}${!job.inboxArchived && pendingByJob.has(job.job_id) ? ' waiting' : ''}${checked ? ' selected' : ''}`}>
            <Checkbox className="inbox-row-select" aria-label={`Select job ${job.job_id}`} disabled={!!operation} checked={checked} onChange={event => {
              const next = new Set(selected);
              if (event.target.checked) next.add(job.job_id);
              else next.delete(job.job_id);
              setSelected(next);
            }} />
            <div className="inbox-job-body"><Link className="inbox-job-link" to={`/project/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(job.job_id)}/story${location.search}`} state={{ inboxReturnTo: location.pathname + location.search }} onClick={() => inbox.markRead(job)} aria-label={`${job.cell_name}: ${title}`}>
              <span className="inbox-correspondent" title={job.cell_name}>{unread && <span className="inbox-unread-dot" aria-label="Unread" />}{job.cell_name}</span>
              <span className="inbox-job-subject"><span className="inbox-job-title">{title}</span><Tag color={status.color} className={status.label === 'Needs review' ? 'inbox-needs-review' : undefined}>{status.label}</Tag>{draftJobs.has(job.job_id) && <Tag color="magenta">Draft</Tag>}{job.title?.trim() && <span className="inbox-job-snippet">{job.prompt?.replace(/\s+/g, ' ')}</span>}</span>
              <Tooltip title={date ? new Date(date).toLocaleString(undefined, { timeZoneName: 'long' }) : 'No activity or submission date was recorded'}>
                <time dateTime={date}>{date ? formatInboxUpdate(date, today) : 'Date unavailable'}</time>
              </Tooltip>
            </Link>
            <JobExecutionRequirements job={job} projectId={projectId} navigationState={{inboxReturnTo:location.pathname + location.search}} />
            </div>
            <div className="inbox-row-actions"><CopyJobId jobId={job.job_id} title={null} compact />{!job.inboxArchived && <Tooltip title="Archive"><Button size="small" type="text" aria-label={`Archive job ${job.job_id}`} icon={<InboxOutlined aria-hidden />} onClick={() => setOperation({action:'archive', ids:[job.job_id], job})} /></Tooltip>}</div>
          </li>;
        })}
      </ul>
      {inbox.nextPageToken && !inbox.error && <div className="inbox-more-jobs">
        <Button onClick={inbox.loadMore} loading={inbox.loading}>Load more jobs</Button>
      </div>}
      {!inbox.loading && !inbox.nextPageToken && !rows.length && <Empty description={inbox.error ? 'Jobs could not be loaded' : reviewsUnavailable ? 'Review requests unavailable' : search ? 'No jobs match this search' : 'No jobs in this inbox'} />}
    </Spin>
    {operation && <JobActionDialog projectId={projectId} action={operation.action} jobIds={operation.ids} job={operation.job} immediate={operation.action==='archive' && canArchiveImmediately(operation.ids,inbox.jobs)} onClose={() => setOperation(undefined)} onComplete={succeeded => {
      setSelected(previous => {
        const next = new Set(previous);
        succeeded.forEach(id => next.delete(id));
        return next;
      });
      inbox.refresh();
    }} />}

  </div>;
}

export function RepositoryLink({ repo }: { repo: string }) {
  const source = repo.replace(/^git\+/, ''), url = /^[A-Za-z0-9.-]+\/[^\s?#]+$/.test(source) ? `https://${source}` : source;
  const label = repo.replace(/\/$/, '').split('/').slice(-1)[0]?.replace(/\.git$/, '') || repo;
  return /^https?:\/\//i.test(url) ? <a href={url} target="_blank" rel="noreferrer" title={repo}>{label}</a> : <span title={repo}>{label}</span>;
}
