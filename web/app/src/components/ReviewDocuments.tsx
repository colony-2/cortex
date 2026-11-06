import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Drawer, Empty, Select, Space, Spin, Typography } from 'antd';
import { useCompact } from '../hooks/useCompact';
import { inputActivityService, type ReviewDocument } from '@colony2/shared';
import DocumentPreview from './review/DocumentPreview';
import { resolveArtifactLink, type NavigationIntent } from './review/artifactLinks';
import { loadDocument, type DocumentData } from './review/document';

interface Props { projectId: string; jobId: string; requestId: string; documents: Record<string, ReviewDocument> }
export default function ReviewDocuments(props: Props) {
  // A fresh review identity owns all preview state; response drafts live above this component.
  return <ReviewReader key={JSON.stringify([props.projectId, props.jobId, props.requestId])} {...props} />;
}
function ReviewReader({ projectId, jobId, requestId, documents }: Props) {
  const compact = useCompact();
  const [readerOpen, setReaderOpen] = useState(false);
  const ids = Object.keys(documents).sort();
  const [selection, setSelection] = useState(ids[0]);
  const selected = selection && documents[selection] ? selection : ids[0];
  const [navigationIntent, setNavigationIntent] = useState<(NavigationIntent & { documentId: string })>();
  const [dual, setDual] = useState(false);
  const [markdownSource, setMarkdownSource] = useState(false);
  const [content, setContent] = useState<{ url: string; data: DocumentData }>();
  const cached = useRef(content);
  const [error, setError] = useState<string>();
  const [retry, setRetry] = useState(0);
  const opener = useRef<HTMLElement>();
  const doc = documents[selected];
  const name = doc?.name || doc?.stored.key.name || selected;
  const url = selected ? inputActivityService.reviewDocumentURL(projectId, jobId, requestId, selected) : '';
  const previewActive = !compact || readerOpen;
  useEffect(() => {
    if (selection !== selected) {
      // Membership changes are a fallback, not a request to navigate to the old target.
      setSelection(selected);
      setNavigationIntent(undefined);
    }
  }, [selection, selected]);
  useEffect(() => {
    setError(undefined);
    if (!url || !previewActive || cached.current?.url === url) return;
    setContent(undefined); cached.current = undefined;
    const controller = new AbortController();
    loadDocument(url, name, controller.signal).then(data => {
      if (!controller.signal.aborted) { cached.current = { url, data }; setContent(cached.current); }
    }).catch(e => { if (!controller.signal.aborted) setError(e.message); });
    return () => controller.abort();
  }, [url, name, retry, previewActive]);
  const toolbar = <div className="review-document-toolbar">
    <Select aria-label="Review document" value={selected} onChange={id => { setNavigationIntent(undefined); setSelection(id); }} style={{ minWidth: 220, maxWidth: '100%' }} options={ids.map(id => ({ value: id, label: `${id} · ${documents[id].name || documents[id].stored.key.name}` }))} />
    <Typography.Text type="secondary">{doc?.stored.key.sizeBytes?.toLocaleString() ?? 'Unknown'} bytes</Typography.Text>
    <Button href={url} download={name}>Download</Button>
  </div>;
  const preview = error ? <Alert type="error" showIcon message={error} action={<Button onClick={() => { cached.current = undefined; setContent(undefined); setRetry(v => v + 1); }}>Retry</Button>} />
    : !content || content.url !== url ? <Spin aria-label="Loading document" />
    : <DocumentPreview key={url} data={content.data} name={name} markdownSource={markdownSource} setMarkdownSource={setMarkdownSource} dual={dual} setDual={setDual} navigationIntent={navigationIntent?.documentId === selected ? navigationIntent : undefined} navigation={{
      resolve: reference => resolveArtifactLink(reference, doc.stored.key.name, Object.entries(documents).map(([key, d]) => ({key, path: d.stored.key.name, aliases: [key, ...(d.name ? [d.name] : [])]}))),
      open: (key, fragment) => {
        if (!Object.prototype.hasOwnProperty.call(documents, key)) return false;
        setMarkdownSource(false); setSelection(key);
        setNavigationIntent(previous => ({documentId: key, token: (previous?.token || 0) + 1, fragment}));
        return true;
      },
    }} resolveAttachment={reference => {
      const matches = Object.entries(documents).filter(([id, d]) => reference === id || reference === d.name || reference === d.stored.key.name);
      if (matches.length !== 1) return undefined;
      const [id, d] = matches[0];
      return { key: id, name: d.name || d.stored.key.name, url: inputActivityService.reviewDocumentURL(projectId, jobId, requestId, id) };
    }} />;
  return <>
    <Card title={`Documents (${ids.length})`} extra={<Typography.Text type="secondary">Read only</Typography.Text>}>
      {!selected ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No documents attached" />
        : compact ? <Space direction="vertical" style={{ width: '100%' }}>{ids.map(id => <Button key={id} block onClick={e => { opener.current = e.currentTarget; setNavigationIntent(undefined); setSelection(id); setReaderOpen(true); }}>Read {documents[id].name || documents[id].stored.key.name}</Button>)}</Space>
        : <Space direction="vertical" size="middle" style={{ width: '100%' }}>{toolbar}{preview}</Space>}
    </Card>
    <Drawer rootClassName="review-reader" title={name} width="100%" open={readerOpen} destroyOnHidden closable={false} onClose={() => setReaderOpen(false)} afterOpenChange={open => { if (!open && !readerOpen) opener.current?.focus(); }} extra={<Button onClick={() => setReaderOpen(false)}>Back to review</Button>}>
      <details className="reader-options"><summary>Document options</summary>{toolbar}</details>{preview}
    </Drawer>
  </>;
}
