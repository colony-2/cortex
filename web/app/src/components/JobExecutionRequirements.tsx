import { Alert, Space, Typography } from 'antd';
import type { RecipeJobSummary } from '@colony2/shared/types';
import { stringifyJobJSON } from '@colony2/shared/jobJSON';

export default function JobExecutionRequirements({ execution }: { execution: RecipeJobSummary['execution'] }) {
  if (!execution || execution.status === 'in_flight' || execution.status === 'not_waiting') return null;
  const labels: Record<string, string> = {
    specified: 'Requirements',
    unspecified: 'No requirements specified',
    unresolved: 'Requirements not yet resolved',
    malformed: 'Invalid execution requirements',
    unsupported: 'Unsupported execution requirements',
  };
  return <Space direction="vertical" size={2}>
    <Typography.Text>{labels[execution.status] || `Requirements: ${execution.status.replace(/_/g, ' ')}`}</Typography.Text>
    {execution.status === 'specified' && execution.demand?.effective !== undefined &&
      <pre style={{ whiteSpace: 'pre-wrap', margin: 0 }}>{stringifyJobJSON(execution.demand.effective, 2)}</pre>}
    {execution.diagnostic && <Alert type="warning" message={<span style={{ whiteSpace: 'pre-wrap' }}>{execution.diagnostic}</span>} />}
  </Space>;
}
