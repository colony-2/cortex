import { cellLabel } from '../utils/cellPresentation';
import { Button, Card, Empty, Space, Table, Tag, Typography } from 'antd';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import type { RecipeJob } from '@colony2/shared/types';
import JobListDetails, { jobTime } from './JobListDetails';
import JobWaitLinks, { jobPath } from './JobWaitLinks';
import { latestActiveJob, newestFirst } from '../utils/goalPresentation';
import { jobStatus } from '../utils/jobStatus';

export function AttemptStatus({ job }: { job: RecipeJob }) {
  const status = jobStatus(job);
  return <Tag color={status.color}>{status.label}</Tag>;
}

export default function GoalJobOverview({ jobs, projectId, navigationState, showPrompt = true, selectedIds, onSelect }: {
  jobs: RecipeJob[]; projectId: string; navigationState: Record<string, unknown>; showPrompt?: boolean; selectedIds?: string[]; onSelect?: (ids: string[], selected: boolean) => void;
}) {
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const current = latestActiveJob(jobs);
  if (!current) return <Empty description="No attempts found" />;
  const prompt = current.prompt || jobs.find(job => job.prompt?.trim())?.prompt;
  const selected = jobs.find(job => params.getAll('inspect').includes(job.job_id)) || current;
  const choose = (job: RecipeJob) => setParams(previous => {
    const next = new URLSearchParams(previous);
    const other = next.getAll('inspect').filter(id => !jobs.some(candidate => candidate.job_id === id));
    next.delete('inspect'); [...other, job.job_id].forEach(id => next.append('inspect', id));
    return next;
  }, { replace: true, state: location.state });
  const storyLink = (job: RecipeJob) => jobPath(projectId, job.job_id);
  return <div className="goal-overview-content">
    {showPrompt && <Typography.Paragraph className="goal-full-prompt">{prompt || 'No prompt provided'}</Typography.Paragraph>}
    {jobs.length > 1 && <section aria-label="Attempts"><Table<RecipeJob> size="small" pagination={false} rowKey="job_id" dataSource={newestFirst(jobs)}
      rowSelection={onSelect ? {
        selectedRowKeys: selectedIds,
        onSelect: (job, selected) => onSelect([job.job_id], selected),
        onSelectAll: (selected, _, changed) => onSelect(changed.map(job => job.job_id), selected),
        getCheckboxProps: job => ({ disabled: false, 'aria-label': `Select job ${job.job_id}` }),
      } : undefined}
      aria-label="Attempts" rowClassName={job => job.job_id === selected.job_id ? 'goal-selected-attempt' : ''}
      columns={[
        { title: 'Submitted', render: (_, job) => <Link to={storyLink(job)} state={navigationState}>{jobTime(job.submitted_at || job.created_at) || 'Open execution'}</Link> },
        { title: 'Status', render: (_, job) => <Link to={storyLink(job)} state={navigationState}><AttemptStatus job={job} /></Link> },
        { title: 'Cell', render: (_, job) => cellLabel(job.cell_name) },
        { title: 'Progress', render: (_, job) => <><JobWaitLinks job={job} projectId={projectId} />{job.completion_detail && <Typography.Text className="job-completion" type={job.completion_status?.startsWith('failed_') ? 'danger' : 'secondary'}>{job.completion_detail}</Typography.Text>}</> },
        { title: '', render: (_, job) => <Button size="small" type="text" aria-pressed={job.job_id === selected.job_id} onClick={() => choose(job)}>Details</Button> },
      ]} /></section>}
    <Card size="small" className="goal-attempt-detail" title={<Space wrap>
      <AttemptStatus job={selected} /><Typography.Text type="secondary">{jobTime(selected.submitted_at || selected.created_at)}</Typography.Text>
    </Space>} extra={<Link to={storyLink(selected)} state={navigationState}>Open execution</Link>}>
      <JobListDetails job={selected} projectId={projectId} showPrompt={!!selected.prompt && selected.prompt !== prompt} navigationState={navigationState} />
    </Card>
  </div>;
}
