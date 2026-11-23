import './JobsListPage.css';
import { useEffect, useMemo, useState } from 'react';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import { Alert, Button, Checkbox, Empty, Input, Spin, Tag } from 'antd';
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import { useJobInbox } from '../hooks/useJobInbox';
import { inboxUpdate, type InboxJob } from '../utils/jobInboxData';
import { jobStatus } from '../utils/jobStatus';
import { matchesSearchTerms, searchTerms } from '../utils/searchTerms';
import JobActionDialog, { type JobAction } from './JobActionDialog';
import { canArchiveImmediately } from '../utils/jobStatus';
import CopyJobId from './CopyJobId';
import JobExecutionRequirements from './JobExecutionRequirements';

export default function JobsListPage({ projectId }: { projectId: string; goalId?: string }) {
  const inbox = useJobInbox();
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const [operation, setOperation] = useState<{action: JobAction; ids: string[]; job?: InboxJob}>();
  const [selected, setSelected] = useState<string[]>([]);
  useEffect(() => { setSelected([]); }, [location.pathname, location.search]);
  const folder = params.get('inbox') ?? 'all', cellId = params.get('cellId');
  const search = params.get('q') ?? '';
  const label = folder === 'archive' ? 'Archive' : folder === 'cell' ? inbox.ranked.find(cell => cell.id === cellId)?.name ?? 'Cell inbox' : 'All';
  const rows = useMemo(() => {
    const terms = searchTerms(search);
    return inbox.jobs.filter(job => (folder === 'archive' ? job.inboxArchived : !job.inboxArchived && (folder !== 'cell' || job.cell_id === cellId))
      && [job.title, job.prompt, job.cell_name].some(text => matchesSearchTerms(terms, text))).sort((a, b) => {
      const time = (job: InboxJob) => Date.parse(inboxUpdate(job, inbox.pending.find(input => input.id === job.job_id)?.requested_at) ?? '') || 0;
      return time(b) - time(a) || a.job_id.localeCompare(b.job_id);
    });
  }, [inbox.jobs, inbox.pending, folder, cellId, search]);
  return <div className="app-page inbox-page">
    <div className="inbox-list-toolbar"><h1>{label}</h1><Button icon={<ReloadOutlined />} aria-label="Refresh inbox" loading={inbox.loading} onClick={inbox.refresh} /><span className="inbox-list-count">{inbox.loaded ? `${rows.length} jobs` : 'Loading jobs…'}</span>
      <Input prefix={<SearchOutlined />} aria-label="Search jobs" placeholder="Search jobs" allowClear value={search} onChange={event => setParams(previous => { const next = new URLSearchParams(previous); if (event.target.value) next.set('q', event.target.value); else next.delete('q'); return next; }, { replace: true })} />
    </div>
    {inbox.error && <Alert type="warning" showIcon message={inbox.loaded ? 'Inbox refresh failed; showing the previous result' : 'Inbox unavailable'} description={inbox.error} action={<Button onClick={inbox.refresh}>Retry</Button>} />}
    {selected.length > 0 && <div className="inbox-selection-actions"><span>{selected.length} selected</span><Button size="small" disabled={!!operation} onClick={() => setOperation({action:'cancel',ids:[...selected]})}>Cancel selected</Button><Button size="small" disabled={!!operation} onClick={() => setOperation({action:'archive',ids:[...selected]})}>Archive selected</Button><Button size="small" onClick={() => setSelected([])}>Clear selection</Button></div>}
    <Spin spinning={inbox.loading}>
      <div className="inbox-column-labels"><span><Checkbox aria-label="Select all jobs" disabled={!!operation || !rows.length} checked={!!rows.length && rows.every(job=>selected.includes(job.job_id))} indeterminate={selected.length>0 && !rows.every(job=>selected.includes(job.job_id))} onChange={event=>setSelected(event.target.checked?rows.map(job=>job.job_id):[])} /> Cell</span><span>Job</span><span>Last update</span></div>
      <ul className="inbox-job-list" aria-label="Jobs">
        {rows.map(job => {
          const updated = inboxUpdate(job, inbox.pending.find(input => input.id === job.job_id)?.requested_at);
          const status = jobStatus(job), title = job.title || job.prompt?.split('\n')[0] || 'Untitled job';
          const unread = inbox.isUnread(job);
          return <li key={job.job_id} className={`inbox-job-row${unread ? ' unread' : ''}`}>
            <Checkbox className="inbox-row-select" aria-label={`Select job ${job.job_id}`} disabled={!!operation} checked={selected.includes(job.job_id)} onChange={event=>setSelected(previous=>event.target.checked?[...previous,job.job_id]:previous.filter(id=>id!==job.job_id))} />
            <div className="inbox-job-body"><Link className="inbox-job-link" to={`/project/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(job.job_id)}/story${location.search}`} state={{ inboxReturnTo: location.pathname + location.search }} onClick={() => inbox.markRead(job)} aria-label={`${job.cell_name}: ${title}`}>
              <span className="inbox-correspondent" title={job.cell_name}>{unread && <span className="inbox-unread-dot" aria-label="Unread" />}{job.cell_name}</span>
              <span className="inbox-job-subject"><span className="inbox-job-title">{title}</span><Tag color={status.color}>{status.label}</Tag><span className="inbox-job-snippet">{job.prompt?.replace(/\s+/g, ' ')}</span></span>
              <time dateTime={updated} title={updated ? new Date(updated).toLocaleString(undefined, { timeZoneName: 'long' }) : 'Execution activity times are not available from this runtime'}>{updated ? new Date(updated).toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : 'Update unavailable'}</time>
            </Link>
            <JobExecutionRequirements job={job} projectId={projectId} navigationState={{inboxReturnTo:location.pathname + location.search}} />
            </div>
            <div className="inbox-row-actions"><CopyJobId jobId={job.job_id} title={null} compact />{!job.inboxArchived && <Button size="small" type="text" aria-label={`Archive job ${job.job_id}`} onClick={() => setOperation({action:'archive', ids:[job.job_id], job})}>Archive</Button>}</div>
          </li>;
        })}
      </ul>
      {!inbox.loading && !rows.length && <Empty description={inbox.error ? 'Jobs could not be loaded' : search ? 'No jobs match this search' : 'No jobs in this inbox'} />}
    </Spin>
    {operation && <JobActionDialog projectId={projectId} action={operation.action} jobIds={operation.ids} job={operation.job} immediate={operation.action==='archive' && canArchiveImmediately(operation.ids,inbox.jobs)} onClose={() => setOperation(undefined)} onComplete={async succeeded => { setSelected(previous=>previous.filter(id=>!succeeded.includes(id))); inbox.refresh(); }} />}

  </div>;
}

export function RepositoryLink({ repo }: { repo: string }) {
  const source = repo.replace(/^git\+/, ''), url = /^[A-Za-z0-9.-]+\/[^\s?#]+$/.test(source) ? `https://${source}` : source;
  const label = repo.replace(/\/$/, '').split('/').slice(-1)[0]?.replace(/\.git$/, '') || repo;
  return /^https?:\/\//i.test(url) ? <a href={url} target="_blank" rel="noreferrer" title={repo}>{label}</a> : <span title={repo}>{label}</span>;
}
