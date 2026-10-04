import type { ReactNode } from 'react';
import { Alert, Descriptions, Space, Typography } from 'antd';
import { Link } from 'react-router-dom';
import type { RecipeJob } from '@colony2/shared/types';
import dayjs from 'dayjs';

const { Text } = Typography;

export function jobTime(value?: string) {
  if (!value || value.startsWith('0001-') || !dayjs(value).isValid()) return undefined;
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss Z');
}

export default function JobListDetails({ job, projectId }: { job: RecipeJob; projectId: string }) {
  const items: { key: string; label: string; children: ReactNode }[] = [];
  const add = (label: string, value: ReactNode) => {
    if (value !== undefined && value !== null && value !== '') {
      items.push({ key: label, label, children: value });
    }
  };
  const jobLink = (id: string, tenant = projectId) => (
    <Link key={id} to={`/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(id)}/story`}>{id}</Link>
  );
  add('Created', jobTime(job.created_at));
  add('Submitted', jobTime(job.submitted_at));
  add('Available from', jobTime(job.available_at));
  add('Finished', jobTime(job.archived_at));
  add('Lease expires', jobTime(job.lease_expires_at));
  add('Deadline', jobTime(job.expires_at));
  add('Repository', job.repo);
  add('Ref', job.git_ref);
  add('Task route', job.next_route?.taskType || job.next_route?.jobType);
  add('Waiting for jobs', job.wait_for?.length ? <Space wrap>{job.wait_for.map(id => jobLink(id))}</Space> : undefined);
  add('Parent job', job.parent?.job_id ? jobLink(job.parent.job_id, job.parent.tenant_id || projectId) : undefined);
  add('Parent step', job.parent?.op_step);
  add('Task input ordinal', job.task_wait?.inputOrdinal);
  add('Task output ordinal', job.task_wait?.outputOrdinal);
  add('Resume job type', job.task_wait?.resumeJobType);
  add('Cancellation', job.cancel_requested ? 'Requested' : undefined);

  return <Space direction="vertical" size="middle" style={{ width: '100%', overflowWrap: 'anywhere' }}>
    {job.completion_detail && <Alert
      type={job.completion_status?.startsWith('failed_') ? 'error' : 'info'}
      message="Completion details"
      description={<div style={{ whiteSpace: 'pre-wrap' }}>{job.completion_detail}</div>}
      showIcon
    />}
    <Descriptions size="small" column={{ xs: 1, sm: 2, lg: 3 }} items={items} />
    {job.execution && <details>
      <summary>Execution requirements</summary>
      <Space direction="vertical" style={{ width: '100%', marginTop: 8 }}>
        <Text>Status: {job.execution.status.replace(/_/g, ' ')}</Text>
        <Text type="secondary">Source: {job.execution.source.replace(/_/g, ' ')}</Text>
        {job.execution.diagnostic && <Alert type="warning" message={job.execution.diagnostic} />}
        {(job.execution.demand || job.execution.initial) && <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>
          {JSON.stringify({ current: job.execution.demand, initial: job.execution.initial }, null, 2)}
        </pre>}
      </Space>
    </details>}
    <details>
      <summary>All job data</summary>
      <pre style={{ whiteSpace: 'pre-wrap', marginTop: 8 }}>{JSON.stringify(job, null, 2)}</pre>
    </details>
  </Space>;
}
