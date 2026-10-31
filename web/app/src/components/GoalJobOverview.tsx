import { useCompact } from '../hooks/useCompact';
import type { getJob } from '@colony2/shared/api';
import { cellLabel } from '../utils/cellPresentation';
import { Button, Card, Checkbox, Empty, Space, Table, Tag, Typography } from 'antd';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import type { RecipeJobSummary } from '@colony2/shared/types';
import { jobTime } from './JobListDetails';
import SelectedJobDetails from './SelectedJobDetails';
import JobWaitLinks, { jobPath } from './JobWaitLinks';
import { latestActiveJob, newestFirst } from '../utils/goalPresentation';
import { jobStatus } from '../utils/jobStatus';
import JobCompletionMessage from './JobCompletionMessage';
import JobExecutionRequirements from './JobExecutionRequirements';

export function AttemptStatus({ job }: { job: RecipeJobSummary }) {
  const status = jobStatus(job);
  return <Tag color={status.color}>{status.label}</Tag>;
}

export default function GoalJobOverview({ jobs, projectId, navigationState, showPrompt = true, selectedIds, onSelect, refresh = 0, inspectionRequests }: {
  inspectionRequests?: Map<string, ReturnType<typeof getJob>>; jobs: RecipeJobSummary[]; projectId: string; refresh?: number; navigationState: Record<string, unknown>; showPrompt?: boolean; selectedIds?: string[]; onSelect?: (ids: string[], selected: boolean) => void;
}) {
  const compact = useCompact();
  const [params, setParams] = useSearchParams();
  const location = useLocation();
  const current = latestActiveJob(jobs);
  if (!current) return <Empty description="No attempts found" />;
  const prompt = current.title || current.prompt || jobs.find(job => job.prompt?.trim())?.prompt;
  const selected = jobs.find(job => params.getAll('inspect').includes(job.job_id)) || current;
  const choose = (job: RecipeJobSummary) => setParams(previous => {
    const next = new URLSearchParams(previous);
    const other = next.getAll('inspect').filter(id => !jobs.some(candidate => candidate.job_id === id));
    next.delete('inspect'); [...other, job.job_id].forEach(id => next.append('inspect', id));
    return next;
  }, { replace: true, state: location.state });
  const storyLink = (job: RecipeJobSummary) => jobPath(projectId, job.job_id);
  return <div className="goal-overview-content">
    {showPrompt && <Typography.Paragraph className="goal-full-prompt">{prompt || 'No prompt provided'}</Typography.Paragraph>}
    {jobs.length > 1 && <section aria-label="Attempts">{compact ? <div className="compact-cards">
      {onSelect && <Checkbox checked={jobs.every(job => selectedIds?.includes(job.job_id))} indeterminate={jobs.some(job => selectedIds?.includes(job.job_id)) && !jobs.every(job => selectedIds?.includes(job.job_id))} onChange={event => onSelect(jobs.map(job => job.job_id), event.target.checked)}>Select all attempts</Checkbox>}
      {newestFirst(jobs).map(job => <Card size="small" key={job.job_id} className={job.job_id === selected.job_id ? 'goal-selected-attempt' : ''}>
        {onSelect && <Checkbox aria-label={`Select job ${job.job_id}`} checked={selectedIds?.includes(job.job_id)} onChange={event => onSelect([job.job_id], event.target.checked)}>Select</Checkbox>}
        <Space direction="vertical" style={{ width: '100%' }}>
          <Link className="compact-touch-link" to={storyLink(job)} state={navigationState}>{jobTime(job.submitted_at || job.created_at) || 'Open execution'}</Link>
          <Link className="compact-touch-link" to={storyLink(job)} state={navigationState}><AttemptStatus job={job} /></Link>
          <Typography.Text>Cell: {cellLabel(job.cell_name)}</Typography.Text>
          <JobWaitLinks job={job} projectId={projectId} /><JobCompletionMessage job={job} /><JobExecutionRequirements execution={job.execution} />
          <Button aria-pressed={job.job_id === selected.job_id} onClick={() => choose(job)}>Details</Button>
        </Space>
      </Card>)}
    </div> : <Table<RecipeJobSummary> size="small" pagination={false} rowKey="job_id" dataSource={newestFirst(jobs)}
      rowSelection={onSelect ? {
        selectedRowKeys: selectedIds,
        onSelect: (job, selected) => onSelect([job.job_id], selected),
        onSelectAll: (selected, _, changed) => onSelect(changed.map(job => job.job_id), selected),
        getCheckboxProps: job => ({ disabled: false, 'aria-label': `Select job ${job.job_id}` }),
      } : undefined}
      aria-label="Attempts" rowClassName={job => job.job_id === selected.job_id ? 'goal-selected-attempt' : ''}
      columns={[
        { title: 'Submitted', render: (_, job) => <Link className="compact-touch-link" to={storyLink(job)} state={navigationState}>{jobTime(job.submitted_at || job.created_at) || 'Open execution'}</Link> },
        { title: 'Status', render: (_, job) => <Link className="compact-touch-link" to={storyLink(job)} state={navigationState}><AttemptStatus job={job} /></Link> },
        { title: 'Cell', render: (_, job) => cellLabel(job.cell_name) },
        { title: 'Progress', render: (_, job) => <Space direction="vertical" size={2}>
          <JobWaitLinks job={job} projectId={projectId} />
          <JobCompletionMessage job={job} />
          <JobExecutionRequirements execution={job.execution} />
        </Space> },
        { title: '', render: (_, job) => <Button size="small" type="text" aria-pressed={job.job_id === selected.job_id} onClick={() => choose(job)}>Details</Button> },
      ]} />}</section>}
    <Card size="small" className="goal-attempt-detail" title={<Space wrap>
      <AttemptStatus job={selected} /><Typography.Text type="secondary">{jobTime(selected.submitted_at || selected.created_at)}</Typography.Text>
    </Space>} extra={<Link className="compact-touch-link" to={storyLink(selected)} state={navigationState}>Open execution</Link>}>
      <SelectedJobDetails inspectionRequests={inspectionRequests} key={JSON.stringify([projectId, selected.job_id])} summary={selected} jobId={selected.job_id} projectId={projectId} refresh={refresh} prompt={prompt} navigationState={navigationState} />
    </Card>
  </div>;
}
