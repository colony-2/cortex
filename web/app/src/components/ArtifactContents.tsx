import { useEffect, useState } from 'react';
import { Alert, Button, Spin } from 'antd';
import DocumentPreview, { type ResolvePreviewAttachment } from './review/DocumentPreview';
import { loadDocument, type DocumentData } from './review/document';

export default function ArtifactContents({ url, name, resolveAttachment }: {
  url: string; name: string; resolveAttachment?: ResolvePreviewAttachment;
}) {
  const [data, setData] = useState<DocumentData>();
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);
  const [markdownSource, setMarkdownSource] = useState(false);
  const [dual, setDual] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    setData(undefined); setError('');
    loadDocument(url, name, controller.signal).then(value => {
      if (!controller.signal.aborted) setData(value);
    }).catch(e => { if (!controller.signal.aborted) setError(e.message); });
    return () => controller.abort();
  }, [url, name, retry]);
  if (error) return <Alert type="error" message={error} action={<Button onClick={() => setRetry(v => v + 1)}>Retry</Button>} />;
  if (!data) return <Spin aria-label="Loading artifact" />;
  return <DocumentPreview data={data} name={name} markdownSource={markdownSource} setMarkdownSource={setMarkdownSource} dual={dual} setDual={setDual} resolveAttachment={resolveAttachment} />;
}
