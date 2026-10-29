import { Typography } from 'antd';
import type { RecipeJobSummary } from '@colony2/shared/types';

export default function JobCompletionMessage({ job }: { job: RecipeJobSummary }) {
  if (!job.completion_detail) return null;
  return <Typography.Paragraph className="job-completion" style={{ whiteSpace: 'pre-wrap', margin: 0 }}
    type={job.completion_status?.startsWith('failed_') ? 'danger' : undefined}
    ellipsis={{ rows: 2, expandable: 'collapsible', symbol: expanded => expanded ? 'Less' : 'Full message' }}>
    {job.completion_detail}
  </Typography.Paragraph>;
}
