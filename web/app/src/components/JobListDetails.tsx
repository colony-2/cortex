import { requirementSummary } from '../utils/executionRequirements';
import GoalPrompt from './GoalPrompt';
import JobLeaseTimeline from './JobLeaseTimeline';
import type { ReactNode } from 'react';
import { Alert, Descriptions, Space, Typography } from 'antd';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import type { RecipeJob, RecipeJobSummary } from '@colony2/shared/types';
import { stringifyJobJSON } from '@colony2/shared/jobJSON';
import dayjs from 'dayjs';
import { jobFilterSearch } from '../utils/jobFilters';
import JobWaitLinks from './JobWaitLinks';
import JobExecutionRequirements from './JobExecutionRequirements';
import JobEnvironment from './JobEnvironment';

const { Text } = Typography;

export function jobTime(value?: string) {
  if (!value || value.startsWith('0001-') || !dayjs(value).isValid()) return undefined;
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss Z');
}

export default function JobListDetails({ job, inspection, projectId, showPrompt = true, navigationState }: { job: RecipeJobSummary; inspection?: RecipeJob; projectId: string; showPrompt?: boolean; navigationState?: Record<string, unknown> }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const location = useLocation();
  const detailKey = (section: string) => JSON.stringify([job.job_id, section]);
  const isOpen = (section: string) => searchParams.getAll('detail').includes(detailKey(section));
  const toggle = (section: string) => setSearchParams(previous => {
    const next = new URLSearchParams(previous);
    next.set('executionState', 'cells-v1');
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
    <Link className="compact-touch-link" key={id} state={{ goalsReturnTo: navigationState?.goalsReturnTo || location.pathname + location.search }} to={`/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(id)}/story${tenant === projectId ? jobFilterSearch(searchParams) : ''}`}>{label}</Link>
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
  add('Dependencies', job.wait_for?.length ? <Space wrap>{job.wait_for.map((id, index) => jobLink(id, projectId, `Dependency ${index + 1}`))}</Space> : undefined);
  add('Parent job', job.parent?.job_id ? jobLink(job.parent.job_id, job.parent.tenant_id || projectId, 'View parent execution') : undefined);
  add('Parent step', job.parent?.op_step);
  add('Cancellation', job.cancel_requested ? 'Requested' : undefined);
  const wait = inspection?.blocking_wait;
  const dependencies = wait?.dependencies.filter(dependency => !dependency.target.goalId);
  const goalOnly = !!wait?.dependencies.length && !dependencies?.length;
  const visibleWait = wait && dependencies && !goalOnly ? { ...wait, dependencies } : undefined;
  const summary = goalOnly ? undefined : job.blocking_wait_summary;
  add('Wait state', visibleWait?.state);
  add('Wait deadline', jobTime(summary?.deadline || visibleWait?.deadline));

  // Keep the displayed data consistent with the dependency list.
  const rawData = { ...job, blocking_wait: visibleWait, blocking_wait_summary: summary };
  delete (rawData as Partial<RecipeJob>).other_goal_ids;

  return <Space direction="vertical" size="middle" style={{ width: '100%', overflowWrap: 'anywhere' }}>
    {showPrompt && job.prompt && <GoalPrompt projectId={projectId} jobId={job.job_id} text={job.prompt} />}
    <JobLeaseTimeline projectId={projectId} jobId={job.job_id} createdAt={job.created_at} />
    <JobWaitLinks job={job} projectId={projectId} details={inspection} />
    {visibleWait?.dependencies.map((dependency, index) => dependency.target.jobId && <div key={index}>
      {jobLink(dependency.target.jobId)}
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
    {requirementSummary(job) && <JobExecutionRequirements job={job} projectId={projectId} navigationState={navigationState} />}
    <details open={isOpen('execution')}>
      <summary onClick={event => { event.preventDefault(); toggle('execution'); }}>Execution requirements</summary>
      <JobEnvironment job={job} showHeading={false} />
    </details>
    <details open={isOpen('technical')}>
      <summary onClick={event => { event.preventDefault(); toggle('technical'); }}>Technical details</summary>
      <Descriptions size="small" column={1} items={[
        { key: 'id', label: 'Job ID', children: <Text code>{job.job_id}</Text> },
        { key: 'route', label: 'Task route', children: job.next_route?.taskType || job.next_route?.jobType || '—' },
        ...(summary ? [{ key: 'wait', label: 'Wait ID', children: summary.waitId }] : []),
        ...(job.task_wait ? [
          { key: 'input', label: 'Task input ordinal', children: String(job.task_wait.inputOrdinal) },
          { key: 'output', label: 'Task output ordinal', children: String(job.task_wait.outputOrdinal) },
          { key: 'hash', label: 'Task input hash', children: job.task_wait.inputHash },
          { key: 'resume', label: 'Resume job type', children: job.task_wait.resumeJobType },
        ] : []),
      ]} />
      {job.completion_detail && <div><Text strong>Full completion message</Text><pre style={{ whiteSpace: 'pre-wrap' }}>{job.completion_detail}</pre></div>}
    </details>
    <details open={isOpen('data')}>
      <summary onClick={event => { event.preventDefault(); toggle('data'); }}>All job data</summary>
      <pre style={{ whiteSpace: 'pre-wrap', marginTop: 8 }}>{stringifyJobJSON(rawData, 2)}</pre>
    </details>
  </Space>;
}
