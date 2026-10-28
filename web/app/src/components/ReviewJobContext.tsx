import { Alert, Button, Card, Descriptions, Space, Typography, Tag } from 'antd';
import { Link } from 'react-router-dom';
import { useReviewJobMetadata } from '../hooks/useReviewJobMetadata';
import { cellLabel } from '../utils/cellPresentation';
import { jobStatus } from '../utils/jobStatus';
import { jobTime } from './JobListDetails';
import { goalPath, jobPath } from './JobWaitLinks';

export default function ReviewJobContext({ projectId, jobId }: { projectId: string; jobId: string }) {
  const { metadata, refresh } = useReviewJobMetadata(projectId, [jobId]);
  const entry = metadata.get(jobId);
  if (entry?.status === 'error') return <Alert type="warning" message="Job context unavailable"
    description="Cell unavailable" action={<Button onClick={refresh}>Retry job context</Button>} />;
  if (!entry?.job) return <Typography.Text type="secondary">Loading job context...</Typography.Text>;
  const job = entry.job;
  const items = [
    { key: 'cell', label: 'Cell', children: cellLabel(job.cell_name) },
    { key: 'status', label: 'Job status', children: <Tag color={jobStatus(job).color}>{jobStatus(job).label}</Tag> },
    ...Object.entries({ Submitted: jobTime(job.submitted_at || job.created_at), Recipe: job.recipe,
      'Job type': job.job_type, Repository: job.repo, Ref: job.git_ref, 'Goal ID': job.goal_id })
      .filter(([, value]) => !!value).map(([label, value]) => ({ key: label, label, children: value! })),
  ];
  return <Card style={{ overflowWrap: 'anywhere' }}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Descriptions size="small" column={{ xs: 1, sm: 2, lg: 3 }} items={items} />
      <Space wrap>
        <Link to={jobPath(projectId, jobId)}>Open execution</Link>
        {job.goal_id && <Link to={goalPath(projectId, job.goal_id)}>Goal overview</Link>}
      </Space>
      <section aria-label="Goal prompt" style={{ width: '100%' }}>
        <Typography.Title level={5}>Goal prompt</Typography.Title>
        <div style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{job.prompt?.trim() ? job.prompt : 'No prompt provided'}</div>
      </section>
    </Space>
  </Card>;
}
