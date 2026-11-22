import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Alert, Button, Card, Empty, Select, Spin, Tabs, Tag, Typography } from 'antd';
import { useCompact } from '../hooks/useCompact';
import { inputActivityService, storedDocumentURL, type ReviewDocument, type ReviewDocumentMetadata } from '@colony2/shared';
import { orderedReviewDocumentIds, startingReviewDocument } from '../utils/reviewDocumentLocation';
import DocumentOptions from './review/DocumentOptions';
import DocumentPreview from './review/DocumentPreview';
import { resolveArtifactLink, type NavigationIntent } from './review/artifactLinks';
import { loadDocument, type DocumentData } from './review/document';

export interface DocumentSource { identity: string; contentIdentity?: (id: string) => string; url: (id: string) => string; onError?: (error: unknown) => void }
export type ReaderDocument = { name?: string; stored?: ReviewDocument['stored']; sizeBytes?: number };
export interface ReviewDocumentLayout { navigationVisible: boolean; stacked: boolean; navigationId: string; separator: ReactNode }
interface Props { changes?: ReactNode; layout?: ReviewDocumentLayout; navigationDefault?: boolean; stackNavigation?: boolean; source?: DocumentSource; projectId: string; jobId: string; requestId: string; documents: Record<string, ReaderDocument>; documentMetadata?: Record<string, ReviewDocumentMetadata>; overviewDocumentId?: string; response?: ReactNode; selection?: { documentId: string | undefined; onSelect: (id: string) => void } }
export default function ReviewDocuments(props: Props) {
  // A fresh review identity owns all preview state; response drafts live above this component.
  return <ReviewReader key={JSON.stringify([props.projectId, props.jobId, props.requestId, props.source?.identity])} {...props} />;
}
function ReviewReader({ projectId, jobId, requestId, documents, documentMetadata = {}, overviewDocumentId, source, response, navigationDefault=false, stackNavigation=false, layout, changes, selection: controlled }: Props) {
  const [readerTab, setReaderTab] = useState('documents');
  const small = useCompact();
  const compact = layout ? layout.stacked : small || stackNavigation;
  const [navigationPreference, setShowNavigation] = useState<boolean>();
  const showNavigation = layout ? layout.navigationVisible : navigationPreference ?? navigationDefault;
  const ids = orderedReviewDocumentIds(documents, documentMetadata);
  const attached = (id?: string): id is string => id !== undefined && Object.prototype.hasOwnProperty.call(documents, id);
  const initial = startingReviewDocument(documents, documentMetadata, overviewDocumentId);
  const [selection, setSelection] = useState(initial);
  const selected = controlled ? controlled.documentId : attached(selection) ? selection : initial;
  const catalog = useRef<HTMLDivElement>(null);
  const [navigationIntent, setNavigationIntent] = useState<(NavigationIntent & { documentId: string })>();
  const [dual, setDual] = useState(false);
  const [markdownSource, setMarkdownSource] = useState(false);
  const [content, setContent] = useState<{ url: string; data: DocumentData }>();
  const cached = useRef(content);
  const [failure, setFailure] = useState<{ key: string; message: string }>();
  const [retry, setRetry] = useState(0);
  const previewRoot = useRef<HTMLElement>(null);
  const reveal = useRef(false);
  const doc = selected === undefined ? undefined : documents[selected];
  const name = doc?.name || doc?.stored?.key.name || selected || '';
  const documentURL = (id: string) => source ? source.url(id) : documents[id]?.stored ? storedDocumentURL(projectId, documents[id].stored.key) : inputActivityService.reviewDocumentURL(projectId, jobId, requestId, id);
  const path = (id: string) => documents[id]?.stored?.key.name || documents[id]?.name || id;
  const url = selected === undefined ? '' : documentURL(selected);
  const contentKey = selected === undefined ? '' : source?.contentIdentity?.(selected) ?? url;
  const [contentIdentity, setContentIdentity] = useState('');
  const error = failure?.key === contentKey ? failure.message : undefined;
  const previousSelection = useRef(selected);
  const pendingSelection = useRef<string>();
  useEffect(() => {
    if (previousSelection.current !== selected) {
      if (controlled && pendingSelection.current !== selected) {
        // A parent-driven change (including Back/Forward) has no old link intent.
        setNavigationIntent(undefined);
        reveal.current = true;
      }
      setReaderTab('documents');
      previousSelection.current = selected;
      pendingSelection.current = undefined;
    }
  }, [selected, controlled]);
  useEffect(() => {
    if (reveal.current && (error || contentIdentity === contentKey)) {
      previewRoot.current?.scrollIntoView?.({ block: 'start' });
      reveal.current = false;
    }
  }, [content, contentIdentity, contentKey, error]);
  useEffect(() => {
    if (!controlled && selection !== selected) {
      // Membership changes are a fallback, not a request to navigate to the old target.
      setSelection(selected);
      catalog.current?.focus();
      setNavigationIntent(undefined);
    }
  }, [selection, selected, controlled]);
  useEffect(() => {
    setFailure(undefined);
    if (!url) { setFailure({ key: contentKey, message: 'Original document unavailable: no unambiguous retained mapping.' }); setContent(undefined); cached.current = undefined; return; }
    if (cached.current && contentIdentity === contentKey) return;
    setContent(undefined); cached.current = undefined;
    const controller = new AbortController();
    loadDocument(url, name, controller.signal).then(data => {
      if (!controller.signal.aborted) { cached.current = { url, data }; setContent(cached.current); setContentIdentity(contentKey); }
    }).catch(e => { if (!controller.signal.aborted) { setFailure({ key: contentKey, message: e.message }); source?.onError?.(e); } });
    return () => controller.abort();
  }, [contentKey, name, retry, !!url]);
  const filename = (id: string) => documents[id]?.name || documents[id]?.stored?.key.name || id;
  const title = (id: string) => documentMetadata[id]?.title?.trim() ? documentMetadata[id].title! : filename(id);
  const openDocument = (id: string, focus = false, fragment?: string) => {
    if (!attached(id)) return false;
    setReaderTab('documents');
    if (controlled) {
      if (id !== selected) { pendingSelection.current = id; controlled.onSelect(id); }
    } else {
      if (id !== selected) pendingSelection.current = id;
      setSelection(id);
    }
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
  const preview = selected === undefined ? null : error ? <Alert type="error" showIcon message={error} action={<Button onClick={() => { cached.current = undefined; setContent(undefined); setRetry(v => v + 1); }}>Retry</Button>} />
    : !content || contentIdentity !== contentKey ? <Spin aria-label="Loading document" />
    : <DocumentPreview htmlContext={{canonicalPath: path(selected), attachments: Object.keys(documents).filter(id => !!documentURL(id)).map(id => ({key:id, path:path(id), url:documentURL(id)}))}} reviewOptions={{ storedBytes: doc?.sizeBytes ?? doc?.stored?.key.sizeBytes }} key={contentKey} data={content.data} name={name} markdownSource={markdownSource} setMarkdownSource={setMarkdownSource} dual={dual} setDual={setDual} navigationIntent={navigationIntent?.documentId === selected ? navigationIntent : undefined} navigation={{
      resolve: reference => resolveArtifactLink(reference, path(selected), Object.entries(documents).map(([key, d]) => ({key, path: path(key), aliases: [key, ...(d.name ? [d.name] : [])]}))),
      open: (key, fragment) => openDocument(key, true, fragment),
      url: source ? documentURL : undefined,
    }} resolveAttachment={reference => {
      const target = resolveArtifactLink(reference, path(selected), Object.entries(documents).map(([key, d]) => ({ key, path: path(key), aliases: [key, ...(d.name ? [d.name] : []), path(key)] })));
      if (target.kind !== 'target' || !attached(target.key) || !documentURL(target.key)) return undefined;
      const id = target.key, d = documents[id];
      return { key: id, name: d.name || path(id), url: documentURL(id), onError: source?.onError };
    }} />;
  const details = selected !== undefined ? <div className="review-selected-details">{summary(selected)}{sources(selected)}<Button disabled={!url} href={url || undefined} download={name}>Download</Button></div> : null;
  const documentPanel = <Card><section ref={previewRoot} className="review-document-content" aria-label="Selected document">
    {selected === undefined ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No documents attached" /> : <>
      {(!content || contentIdentity !== contentKey || error) && <DocumentOptions key={url} storedBytes={doc?.sizeBytes ?? doc?.stored?.key.sizeBytes} />}
      {preview}{details}
    </>}
  </section></Card>;
  return <>
    {!layout && <Button aria-expanded={showNavigation} onClick={() => setShowNavigation(!showNavigation)}>{showNavigation ? 'Hide documents' : 'Show documents'}</Button>}
    <div className={`review-reading-layout ${layout ? 'review-v2-document-tracks' : ''} ${showNavigation && !compact ? 'with-navigation' : ''}`}>
      <div id={layout?.navigationId} className="review-navigation-track" hidden={!showNavigation}>
        <Card ref={catalog} tabIndex={-1} className="review-document-catalog" title={`Documents (${ids.length})`}>
          {compact && selected !== undefined && <Select aria-label="Review document" value={selected} onChange={id => openDocument(id)} style={{ width: '100%' }} options={ids.map(id => ({ value: id, label: `${id} · ${title(id)}` }))} />}
          <nav aria-label="Review documents" className={`review-document-list ${compact ? "review-document-cards" : ""}`}>{ids.map(id => <div key={id} className="review-document-entry">
            {compact ? <>{summary(id)}{id === initial && <Tag>Starting document</Tag>}{sources(id)}<Button block onClick={() => openDocument(id)}>Read {title(id)}</Button></>
              : <button type="button" aria-current={id === selected ? 'true' : undefined} onClick={() => openDocument(id)}>{summary(id)}</button>}
          </div>)}</nav>
        </Card>
      </div>
      {layout?.separator}
      <div className="review-reading-track">
        {changes ? <Tabs className="review-reader-tabs" activeKey={readerTab} onChange={setReaderTab} items={[
          { key: 'documents', label: 'Documents', children: documentPanel },
          { key: 'changes', label: 'Changes', children: changes },
        ]} /> : documentPanel}
        {response}
      </div>
    </div>
  </>;
}
