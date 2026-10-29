import { Link } from 'react-router-dom';
import { Space, Typography } from 'antd';
import type { RecipeJob, RecipeJobSummary } from '@colony2/shared/types';

export const goalPath = (tenant: string, goal: string) => `/project/${encodeURIComponent(tenant)}/goals/${encodeURIComponent(goal)}`;
export const jobPath = (tenant: string, job: string) => `/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/story`;

export default function JobWaitLinks({ job, projectId, details }: { job: RecipeJobSummary; projectId: string; details?: RecipeJob }) {
  if (job.store !== 'ACTIVE') return null;
  if (job.next_route?.taskType === 'input:collect_user_input') {
    return <Link to={`/project/${encodeURIComponent(projectId)}/reviews/${encodeURIComponent(job.job_id)}`}>Open input/review</Link>;
  }
  const dependencies = details?.blocking_wait?.state === 'PENDING'
    ? details?.blocking_wait.dependencies.filter(item => item.state === 'PENDING') : [];
  if (dependencies.length) return <Space wrap size="small">
    <Typography.Text type="secondary">Waiting for</Typography.Text>
    {dependencies.map(({ target }, index) => target.goalId
      ? <Link key={`goal:${target.goalId}`} to={goalPath(projectId, target.goalId)}>Goal{dependencies.length > 1 ? ` ${index + 1}` : ''}</Link>
      : target.jobId ? <Link key={`job:${target.jobId}`} to={jobPath(projectId, target.jobId)}>Execution{dependencies.length > 1 ? ` ${index + 1}` : ''}</Link> : null)}
  </Space>;
  if (!details?.blocking_wait && job.status === 'PENDING_JOBS' && job.wait_for?.length) return <Space wrap>
    <Typography.Text type="secondary">Waiting for</Typography.Text>
    {job.wait_for.map((id, index) => <Link key={`${index}:${id}`} to={jobPath(projectId, id)}>Execution{job.wait_for!.length > 1 ? ` ${index + 1}` : ''}</Link>)}
  </Space>;
  if (job.blocking_wait_summary || job.status === 'PENDING_JOBS') {
    return <Typography.Text type="secondary" title={job.blocking_wait_summary?.waitId}>Waiting for dependencies
      {job.blocking_wait_summary?.deadline && <> · Deadline: {job.blocking_wait_summary.deadline}</>}
    </Typography.Text>;
  }
  return null;
}
