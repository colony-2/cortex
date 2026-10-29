import type { ReactNode } from 'react';
import { Alert, Descriptions, Space, Typography } from 'antd';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import type { RecipeJob } from '@colony2/shared/types';
import dayjs from 'dayjs';
import { jobFilterSearch } from '../utils/jobFilters';
import JobWaitLinks, { goalPath } from './JobWaitLinks';

const { Text } = Typography;

export function jobTime(value?: string) {
  if (!value || value.startsWith('0001-') || !dayjs(value).isValid()) return undefined;
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss Z');
}

export default function JobListDetails({ job, projectId, showPrompt = true, navigationState }: { job: RecipeJob; projectId: string; showPrompt?: boolean; navigationState?: Record<string, unknown> }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const detailKey = (section: string) => JSON.stringify([job.job_id, section]);
  const isOpen = (section: string) => searchParams.getAll('detail').includes(detailKey(section));
  const toggle = (section: string) => setSearchParams(previous => {
    const next = new URLSearchParams(previous);
    const values = new Set(next.getAll('detail'));
    const key = detailKey(section);
    if (values.has(key)) values.delete(key); else values.add(key);
    next.delete('detail'); values.forEach(value => next.append('detail', value));
    return next;
  }, { replace: true, state: location.state });
  const items: { key: string; label: string; children: ReactNode }[] = [];
  const add = (label: string, value: ReactNode) => {
    if (value !== undefined && value !== null && value !== '') {
      items.push({ key: label, label, children: value });
    }
  };
  const jobLink = (id: string, tenant = projectId, label = 'View execution') => (
    <Link key={id} state={{ goalsReturnTo: navigationState?.goalsReturnTo || location.pathname + location.search }} to={`/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(id)}/story${tenant === projectId ? jobFilterSearch(searchParams) : ''}`}>{label}</Link>
  );
  add('Recipe', job.recipe);
  add('Cell', job.cell_name);
  add('Submitted', jobTime(job.submitted_at));
  add('Available from', jobTime(job.available_at));
  add('Finished', jobTime(job.archived_at));
  add('Lease expires', jobTime(job.lease_expires_at));
  add('Deadline', jobTime(job.expires_at));
  add('Repository', job.repo);
  add('Ref', job.git_ref);
  add('Waiting for jobs', job.wait_for?.length ? <Space wrap>{job.wait_for.map((id, index) => jobLink(id, projectId, `Dependency ${index + 1}`))}</Space> : undefined);
  add('Parent job', job.parent?.job_id ? jobLink(job.parent.job_id, job.parent.tenant_id || projectId, 'View parent execution') : undefined);
  add('Parent step', job.parent?.op_step);
  add('Cancellation', job.cancel_requested ? 'Requested' : undefined);
  add('Wait state', job.blocking_wait?.state);
  add('Wait deadline', jobTime(job.blocking_wait?.deadline));

  return <Space direction="vertical" size="middle" style={{ width: '100%', overflowWrap: 'anywhere' }}>
    {showPrompt && job.prompt && <div style={{ whiteSpace: 'pre-wrap' }}>{job.prompt}</div>}
    <JobWaitLinks job={job} projectId={projectId} details={job} />
    {job.blocking_wait?.dependencies.map((dependency, index) => <div key={index}>
      {dependency.target.goalId ? <Link to={goalPath(projectId, dependency.target.goalId)}>View goal</Link> : dependency.target.jobId ? jobLink(dependency.target.jobId) : null}
      {` · ${dependency.condition} · ${dependency.state}`}
      {dependency.blockedReason && ` · ${dependency.blockedReason}`}
      {dependency.satisfiedByJobId && <> · Result from {jobLink(dependency.satisfiedByJobId)}</>}
    </div>)}
    {job.completion_detail && <Alert
      type={job.completion_status?.startsWith('failed_') ? 'error' : 'info'}
      message="Completion details"
      description={<div style={{ whiteSpace: 'pre-wrap' }}>{job.completion_detail}</div>}
      showIcon
    />}
    <Descriptions size="small" column={{ xs: 1, sm: 2, lg: 3 }} items={items} />
    {job.execution && <details open={isOpen('execution')}>
      <summary onClick={event => { event.preventDefault(); toggle('execution'); }}>Execution requirements</summary>
      <Space direction="vertical" style={{ width: '100%', marginTop: 8 }}>
        <Text>Status: {job.execution.status.replace(/_/g, ' ')}</Text>
        <Text type="secondary">Source: {job.execution.source.replace(/_/g, ' ')}</Text>
        {job.execution.diagnostic && <Alert type="warning" message={job.execution.diagnostic} />}
        {(job.execution.demand || job.execution.initial) && <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>
          {JSON.stringify({ current: job.execution.demand, initial: job.execution.initial }, null, 2)}
        </pre>}
      </Space>
    </details>}
    <details open={isOpen('technical')}>
      <summary onClick={event => { event.preventDefault(); toggle('technical'); }}>Technical details</summary>
      <Descriptions size="small" column={1} items={[
        { key: 'id', label: 'Job ID', children: <Text code>{job.job_id}</Text> },
        { key: 'route', label: 'Task route', children: job.next_route?.taskType || job.next_route?.jobType || '—' },
        ...(job.task_wait ? [
          { key: 'input', label: 'Task input ordinal', children: job.task_wait.inputOrdinal },
          { key: 'output', label: 'Task output ordinal', children: job.task_wait.outputOrdinal },
          { key: 'resume', label: 'Resume job type', children: job.task_wait.resumeJobType },
        ] : []),
      ]} />
      {job.completion_detail && <div><Text strong>Full completion message</Text><pre style={{ whiteSpace: 'pre-wrap' }}>{job.completion_detail}</pre></div>}
    </details>
    <details open={isOpen('data')}>
      <summary onClick={event => { event.preventDefault(); toggle('data'); }}>All job data</summary>
      <pre style={{ whiteSpace: 'pre-wrap', marginTop: 8 }}>{JSON.stringify(job, null, 2)}</pre>
    </details>
  </Space>;
}
