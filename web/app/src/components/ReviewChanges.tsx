import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Card, Space, Spin } from 'antd';
import { ChangesReadError, reviewChangesService, type WorkspaceDiffPreview } from '@colony2/shared';
import DocumentPreview from './review/DocumentPreview';

interface Props { projectId: string; jobId: string }
export default function ReviewChanges({ projectId, jobId }: Props) {
  const [revision, setRevision] = useState(0);
  const [dual, setDual] = useState(false);
  const [loaded, setLoaded] = useState<{ key: string; result: WorkspaceDiffPreview }>();
  const [error, setError] = useState<{ key: string; error: unknown }>();
  const generation = useRef(0);
  const key = JSON.stringify([projectId, jobId, revision]);
  const refresh = () => { setLoaded(undefined); setError(undefined); setRevision(v => v + 1); };
  useEffect(() => {
    const current = ++generation.current;
    const controller = new AbortController();
    setLoaded(undefined); setError(undefined);
    reviewChangesService.workspaceDiff(projectId, jobId, {}, controller.signal).then(result => {
      if (!controller.signal.aborted && generation.current === current) setLoaded({ key, result });
    }).catch(error => {
      if (!controller.signal.aborted && generation.current === current) setError({ key, error });
    });
    return () => { controller.abort(); generation.current++; };
  }, [projectId, jobId, revision]);
  const result = loaded?.key === key ? loaded.result : undefined;
  const failure = error?.key === key ? error.error : undefined;
  const data = useMemo(() => result ? { bytes: new TextEncoder().encode(result.patch), text: result.patch, format: 'diff' as const, truncated: !result.preview_complete } : undefined, [result]);
  const metadata = result?.metadata;
  const workspace = metadata?.workspace;
  return <section aria-label="Job changes" className="review-changes">
    <Space wrap className="changes-heading"><span>Latest saved workspace diff · Main workspace</span><Button onClick={refresh}>Refresh changes</Button></Space>
    {failure ? <Alert type="error" showIcon message="Could not read saved workspace diff" description={<>{failure instanceof Error ? failure.message : 'The read failed.'}{failure instanceof ChangesReadError && failure.diagnostic && <div>{failure.diagnostic.code}</div>}</>} action={<Button onClick={refresh}>Retry</Button>} />
      : !result ? <Spin aria-label="Loading saved workspace diff" /> : <>
        {workspace && <Card size="small" className="changes-identity">
          <strong>{workspace.repository}</strong>
          <div className="changes-comparison">Base <code title={workspace.base_commit}>{workspace.empty_base ? 'Empty tree' : workspace.base_commit?.slice(0, 10) || 'Not recorded'}</code> → Head <code title={workspace.head_commit}>{workspace.head_commit?.slice(0, 10) || 'Unborn workspace'}</code></div>
          {metadata?.resolved.status === 'in_progress' && <div>Saved progress · scope is still running</div>}
          {metadata?.resolved.status === 'diagnostic' && <div>Saved diagnostic state</div>}
        </Card>}
        {metadata?.status === 'unavailable' ? <Alert type="info" message="No saved workspace diff is available for this recorded point." />
          : <Card className="changes-preview">
            <Space wrap className="changes-heading">{result.download_url && <Button href={result.download_url} download>Download saved diff</Button>}<span>{metadata?.bytes === undefined ? 'Stored size unknown' : `${metadata.bytes.toLocaleString()} stored bytes`}</span></Space>
            {metadata?.status === 'empty' ? <Alert type="info" message="No file changes in this saved comparison." /> : data && <>
              {/^(Binary files |GIT binary patch)/m.test(result.patch) && !/^diff --git /m.test(result.patch) && <Alert type="info" message={/^GIT binary patch$/m.test(result.patch) ? 'Binary file changed. A binary patch is included in the saved diff; this preview shows its source.' : 'Binary file changed. Its contents were not included in this saved diff.'} />}
              <DocumentPreview key={key} name="changes.diff" data={data} dual={dual} setDual={setDual} />
            </>}
          </Card>}
      </>}
  </section>;
}
