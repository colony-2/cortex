import { useEffect, useState } from 'react';
import { Alert, Button, Spin } from 'antd';
import DocumentPreview, { type ResolvePreviewAttachment } from './review/DocumentPreview';
import { type PreviewNavigation, type NavigationIntent } from './review/artifactLinks';
import type { HTMLContext } from './review/htmlPreparation';
import { loadDocument, type DocumentData } from './review/document';

export default function ArtifactContents({ url, name, resolveAttachment, navigation, navigationIntent, htmlContext, unavailableMessage }: {
  unavailableMessage?: string; htmlContext?: HTMLContext; url: string; name: string; resolveAttachment?: ResolvePreviewAttachment; navigation?: PreviewNavigation; navigationIntent?: NavigationIntent;
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
  if (error) return <Alert type="error" message={unavailableMessage ? `${unavailableMessage}: ${error}` : error} action={<Button onClick={() => setRetry(v => v + 1)}>Retry</Button>} />;
  if (!data) return <Spin aria-label="Loading artifact" />;
  return <DocumentPreview htmlContext={htmlContext} data={data} name={name} markdownSource={markdownSource} setMarkdownSource={setMarkdownSource} dual={dual} setDual={setDual} resolveAttachment={resolveAttachment} navigation={navigation} navigationIntent={navigationIntent} />;
}
