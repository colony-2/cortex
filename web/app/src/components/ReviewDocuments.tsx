import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Alert, Button, Card, Empty, Select, Spin, Tag, Typography } from 'antd';
import { useCompact } from '../hooks/useCompact';
import { inputActivityService, type ReviewDocument, type ReviewDocumentMetadata } from '@colony2/shared';
import DocumentOptions from './review/DocumentOptions';
import DocumentPreview from './review/DocumentPreview';
import { resolveArtifactLink, type NavigationIntent } from './review/artifactLinks';
import { loadDocument, type DocumentData } from './review/document';

export interface DocumentSource { identity: string; url: (id: string) => string; onError?: (error: unknown) => void }
export type ReaderDocument = { name?: string; stored?: ReviewDocument['stored'] };
interface Props { source?: DocumentSource; projectId: string; jobId: string; requestId: string; documents: Record<string, ReaderDocument>; documentMetadata?: Record<string, ReviewDocumentMetadata>; overviewDocumentId?: string; response?: ReactNode }
export default function ReviewDocuments(props: Props) {
  // A fresh review identity owns all preview state; response drafts live above this component.
  return <ReviewReader key={JSON.stringify([props.projectId, props.jobId, props.requestId, props.source?.identity])} {...props} />;
}
function ReviewReader({ projectId, jobId, requestId, documents, documentMetadata = {}, overviewDocumentId, source, response }: Props) {
  const compact = useCompact();
  const [showNavigation, setShowNavigation] = useState(false);
  const ids = Object.keys(documents).sort((a, b) => displayOrder(documentMetadata[a]?.order) - displayOrder(documentMetadata[b]?.order) || (a < b ? -1 : a > b ? 1 : 0));
  const attached = (id?: string): id is string => id !== undefined && Object.prototype.hasOwnProperty.call(documents, id);
  const initial = attached(overviewDocumentId) ? overviewDocumentId : ids[0];
  const [selection, setSelection] = useState(initial);
  const selected = attached(selection) ? selection : initial;
  const catalog = useRef<HTMLDivElement>(null);
  const [navigationIntent, setNavigationIntent] = useState<(NavigationIntent & { documentId: string })>();
  const [dual, setDual] = useState(false);
  const [markdownSource, setMarkdownSource] = useState(false);
  const [content, setContent] = useState<{ url: string; data: DocumentData }>();
  const cached = useRef(content);
  const [error, setError] = useState<string>();
  const [retry, setRetry] = useState(0);
  const previewRoot = useRef<HTMLElement>(null);
  const reveal = useRef(false);
  const doc = documents[selected];
  const name = doc?.name || doc?.stored?.key.name || selected;
  const documentURL = (id: string) => source ? source.url(id) : inputActivityService.reviewDocumentURL(projectId, jobId, requestId, id);
  const path = (id: string) => documents[id]?.stored?.key.name || documents[id]?.name || id;
  const url = selected ? documentURL(selected) : '';
  useEffect(() => {
    if (reveal.current && (error || content?.url === url)) {
      previewRoot.current?.scrollIntoView?.({ block: 'start' });
      reveal.current = false;
    }
  }, [content, url, error]);
  useEffect(() => {
    if (selection !== selected) {
      // Membership changes are a fallback, not a request to navigate to the old target.
      setSelection(selected);
      catalog.current?.focus();
      setNavigationIntent(undefined);
    }
  }, [selection, selected]);
  useEffect(() => {
    setError(undefined);
    if (!url) { setError('Original document unavailable: no unambiguous retained mapping.'); setContent(undefined); cached.current = undefined; return; }
    if (cached.current?.url === url) return;
    setContent(undefined); cached.current = undefined;
    const controller = new AbortController();
    loadDocument(url, name, controller.signal).then(data => {
      if (!controller.signal.aborted) { cached.current = { url, data }; setContent(cached.current); }
    }).catch(e => { if (!controller.signal.aborted) { setError(e.message); source?.onError?.(e); } });
    return () => controller.abort();
  }, [url, name, retry]);
  const filename = (id: string) => documents[id]?.name || documents[id]?.stored?.key.name || id;
  const title = (id: string) => documentMetadata[id]?.title?.trim() ? documentMetadata[id].title! : filename(id);
  const openDocument = (id: string, focus = false, fragment?: string) => {
    if (!attached(id)) return false;
    setSelection(id);
    setNavigationIntent(focus ? previous => ({ documentId: id, token: (previous?.token || 0) + 1, fragment }) : undefined);
    if (focus) setMarkdownSource(false);
    reveal.current = !focus;
    if (!focus && id === selected) previewRoot.current?.scrollIntoView?.({ block: 'start' });
    return true;
  };
  const summary = (id: string) => {
    const metadata = documentMetadata[id];
    const role = metadata?.role === 'primary' ? 'Primary' : metadata?.role === 'supporting' ? 'Supporting' : metadata?.role === 'diagnostic' ? 'Diagnostic' : undefined;
    return <div className="review-document-summary">
      <strong>{title(id)}</strong>{role && <Tag>{role}</Tag>}
      <Typography.Text type="secondary">{id}{title(id) !== filename(id) ? ` · ${filename(id)}` : ''}</Typography.Text>
      {metadata?.description && <p>{metadata.description}</p>}
    </div>;
  };
  const sources = (id: string) => {
    const sourceIds = [...new Set(documentMetadata[id]?.source_document_ids || [])].filter(source => source !== id);
    return sourceIds.length ? <div className="review-document-sources" aria-label="Source documents">Source documents: {sourceIds.map(source => attached(source) && !!documentURL(source)
      ? <a key={source} href={documentURL(source)} onClick={event => {
        if (event.button || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
        event.preventDefault();
        openDocument(source, true);
      }}>{title(source)}{title(source) !== filename(source) ? ` · ${filename(source)}` : ''} ({source})</a>
      : <span key={source}>{source} (unavailable)</span>)}</div> : null;
  };
  const preview = error ? <Alert type="error" showIcon message={error} action={<Button onClick={() => { cached.current = undefined; setContent(undefined); setRetry(v => v + 1); }}>Retry</Button>} />
    : !content || content.url !== url ? <Spin aria-label="Loading document" />
    : <DocumentPreview htmlContext={{canonicalPath: path(selected), attachments: Object.keys(documents).filter(id => !!documentURL(id)).map(id => ({key:id, path:path(id), url:documentURL(id)}))}} reviewOptions={{ storedBytes: doc?.stored?.key.sizeBytes }} key={url} data={content.data} name={name} markdownSource={markdownSource} setMarkdownSource={setMarkdownSource} dual={dual} setDual={setDual} navigationIntent={navigationIntent?.documentId === selected ? navigationIntent : undefined} navigation={{
      resolve: reference => resolveArtifactLink(reference, path(selected), Object.entries(documents).map(([key, d]) => ({key, path: path(key), aliases: [key, ...(d.name ? [d.name] : [])]}))),
      open: (key, fragment) => openDocument(key, true, fragment),
      url: source ? documentURL : undefined,
    }} resolveAttachment={reference => {
      const target = resolveArtifactLink(reference, path(selected), Object.entries(documents).map(([key, d]) => ({ key, path: path(key), aliases: [key, ...(d.name ? [d.name] : []), path(key)] })));
      if (target.kind !== 'target' || !attached(target.key) || !documentURL(target.key)) return undefined;
      const id = target.key, d = documents[id];
      return { key: id, name: d.name || path(id), url: documentURL(id), onError: source?.onError };
    }} />;
  const details = selected ? <div className="review-selected-details">{summary(selected)}{sources(selected)}<Button disabled={!url} href={url || undefined} download={name}>Download</Button></div> : null;
  return <>
    <Button aria-expanded={showNavigation} onClick={() => setShowNavigation(value => !value)}>{showNavigation ? 'Hide documents' : 'Show documents'}</Button>
    <div className={`review-reading-layout ${showNavigation && !compact ? 'with-navigation' : ''}`}>
      <div className="review-navigation-track" hidden={!showNavigation}>
        <Card ref={catalog} tabIndex={-1} className="review-document-catalog" title={`Documents (${ids.length})`}>
          {compact && selected && <Select aria-label="Review document" value={selected} onChange={id => openDocument(id)} style={{ width: '100%' }} options={ids.map(id => ({ value: id, label: `${id} · ${title(id)}` }))} />}
          <nav aria-label="Review documents" className={`review-document-list ${compact ? "review-document-cards" : ""}`}>{ids.map(id => <div key={id} className="review-document-entry">
            {compact ? <>{summary(id)}{id === initial && <Tag>Starting document</Tag>}{sources(id)}<Button block onClick={() => openDocument(id)}>Read {title(id)}</Button></>
              : <button type="button" aria-current={id === selected ? 'true' : undefined} onClick={() => openDocument(id)}>{summary(id)}</button>}
          </div>)}</nav>
        </Card>
      </div>
      <div className="review-reading-track">
        <Card><section ref={previewRoot} className="review-document-content" aria-label="Selected document">
          {!selected ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No documents attached" /> : <>
            {(!content || content.url !== url || error) && <DocumentOptions key={url} storedBytes={doc?.stored?.key.sizeBytes} />}
            {preview}{details}
          </>}
        </section></Card>
        {response}
      </div>
    </div>
  </>;
}


function displayOrder(order: unknown): number {
  return typeof order === 'number' && Number.isInteger(order) && order >= 0 && order <= 1_000_000 ? order : Infinity;
}
