import { useEffect, useState } from 'react';
import { Alert, Button, Card, Space, Spin, Typography } from 'antd';
import ReactJson from 'react-json-view';
import { getRestartContext, type RestartContext } from '@colony2/shared';

interface Props { projectId: string; jobId: string; chapterOrdinal?: number; scopeId?: string }

export default function RestartContextPreview(props: Props) {
  // Never render a previous boundary's data, including during the render before
  // an effect can clear it. The inner request is also aborted on selection change.
  return <Card size="small" title="Context before patching">
    {props.scopeId == null && props.chapterOrdinal == null ? <Typography.Text type="secondary">Choose a restart point to inspect its context.</Typography.Text>
      : <Snapshot key={`${props.projectId}/${props.jobId}/${props.scopeId || props.chapterOrdinal}`} {...props} />}
  </Card>;
}

function Snapshot({ projectId, jobId, chapterOrdinal, scopeId }: Props) {
  const [snapshot, setSnapshot] = useState<RestartContext>();
  const [error, setError] = useState<string>();
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setSnapshot(undefined);
    setError(undefined);
    getRestartContext(projectId, jobId, scopeId ?? chapterOrdinal!, controller.signal).then(context => {
      if (!controller.signal.aborted) setSnapshot(context);
    }).catch(error => {
      if (!controller.signal.aborted) setError(error instanceof Error ? error.message : 'Unable to inspect restart context');
    });
    return () => controller.abort();
  }, [projectId, jobId, chapterOrdinal, scopeId, retry]);

  if (error) return <Alert type="error" showIcon message="Could not load context before patching" description={error}
    action={<Button onClick={() => setRetry(value => value + 1)}>Retry context</Button>} />;
  if (!snapshot) return <Spin aria-label="Loading restart context" />;
  return <section aria-label="Context before patching">
    <Space direction="vertical" style={{ width: '100%' }}>
      <Typography.Text type="secondary">Context at entry to this restart point, including unchanged values. Only your patch will be submitted.</Typography.Text>
      {!!snapshot.redacted_paths?.length && <Alert type="info" showIcon message="Some context values are redacted"
        description={<ul>{snapshot.redacted_paths.map(path => <li key={path}><Typography.Text code>{path}</Typography.Text></li>)}</ul>} />}
      <ReactJson src={{ job: snapshot.job, visible: snapshot.visible }} collapsed={2} displayDataTypes={false}
        enableClipboard theme="rjv-default" />
    </Space>
  </section>;
}
