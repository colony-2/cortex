import { Alert, Button, Spin } from 'antd';
import type { PulseInstances } from '@colony2/shared';

export function ReadState({ error, busy, retry }: { error?: string; busy?: boolean; retry: () => void }) {
  if (error)
    return (
      <Alert
        showIcon
        type="warning"
        message={error}
        action={
          <Button size="small" onClick={retry}>
            Retry
          </Button>
        }
      />
    );
  return busy ? (
    <span role="status" aria-label="Loading">
      <Spin size="small" />
    </span>
  ) : null;
}
export function PartialErrors({ result }: { result?: PulseInstances }) {
  return result && !result.complete ? (
    <Alert
      showIcon
      type="warning"
      message="Partial inventory"
      description={result.errors.map((e, i) => (
        <div key={i}>
          {e.provider} · {e.error}
        </div>
      ))}
    />
  ) : null;
}
