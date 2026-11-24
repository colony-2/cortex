import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Alert, Button, Card, Empty, Modal, Select, Spin, Tabs, Tag, Typography } from 'antd';
import { CodeOutlined, DiffOutlined, FileExcelOutlined, FileImageOutlined, FileMarkdownOutlined, FilePdfOutlined, FileTextOutlined, FileWordOutlined, FileZipOutlined, PaperClipOutlined } from '@ant-design/icons';
import { useCompact } from '../hooks/useCompact';
import { inputActivityService, storedDocumentURL, type ReviewDocument, type ReviewDocumentMetadata } from '@colony2/shared';
import { orderedReviewDocumentIds, startingReviewDocument } from '../utils/reviewDocumentLocation';
import DocumentOptions from './review/DocumentOptions';
import DocumentPreview from './review/DocumentPreview';
import { resolveArtifactLink, type NavigationIntent } from './review/artifactLinks';
import { loadDocument, type DocumentData } from './review/document';

export interface DocumentSource { identity: string; contentIdentity?: (id: string) => string; url: (id: string) => string; onError?: (error: unknown) => void }
export type ReaderDocument = { name?: string; stored?: ReviewDocument['stored']; sizeBytes?: number; details?: ReactNode; unavailableReason?: string; hideIdentifier?: boolean };
export interface ReviewDocumentLayout { navigationVisible: boolean; stacked: boolean; navigationId: string; separator: ReactNode }
interface DocumentSelection { documentId: string | undefined; onSelect: (id: string, options?: { replace?: boolean }) => void }
interface Props { changes?: ReactNode; layout?: ReviewDocumentLayout; attachmentLayout?: 'primary' | 'attachments'; attachmentsLabel?: string; primaryActions?: ReactNode; navigationDefault?: boolean; stackNavigation?: boolean; source?: DocumentSource; projectId: string; jobId: string; requestId: string; documents: Record<string, ReaderDocument>; documentMetadata?: Record<string, ReviewDocumentMetadata>; overviewDocumentId?: string; response?: ReactNode; selection?: DocumentSelection }
type DocumentIntent = NavigationIntent & { documentId: string };
interface ReaderProps extends Props {
  bare?: boolean;
  primary?: boolean;
  openLinkedDocument?: (id: string, fragment?: string, trigger?: HTMLElement) => void;
  linkedIntent?: DocumentIntent;
}

const filename = (id: string, doc?: ReaderDocument) => doc?.name || doc?.stored?.key.name || id;
const documentTitle = (id: string, doc?: ReaderDocument, metadata?: ReviewDocumentMetadata) => metadata?.title?.trim() || filename(id, doc);

const attachmentIcons: Record<string, typeof PaperClipOutlined> = {
  md: FileMarkdownOutlined, markdown: FileMarkdownOutlined,
  txt: FileTextOutlined, log: FileTextOutlined,
  png: FileImageOutlined, jpg: FileImageOutlined, jpeg: FileImageOutlined, gif: FileImageOutlined,
  svg: FileImageOutlined, webp: FileImageOutlined, bmp: FileImageOutlined, avif: FileImageOutlined,
  pdf: FilePdfOutlined, csv: FileExcelOutlined, xlsx: FileExcelOutlined, xls: FileExcelOutlined,
  doc: FileWordOutlined, docx: FileWordOutlined,
  zip: FileZipOutlined, tar: FileZipOutlined, gz: FileZipOutlined, tgz: FileZipOutlined,
  bz2: FileZipOutlined, xz: FileZipOutlined, '7z': FileZipOutlined, rar: FileZipOutlined,
  json: CodeOutlined, jsonl: CodeOutlined, ndjson: CodeOutlined, yaml: CodeOutlined, yml: CodeOutlined,
  xml: CodeOutlined, html: CodeOutlined, htm: CodeOutlined,
  patch: DiffOutlined, diff: DiffOutlined,
};
const attachmentColors = new Map<typeof PaperClipOutlined, string>([
  [FileMarkdownOutlined, '#1d4ed8'],
  [FileTextOutlined, '#475569'],
  [FileImageOutlined, '#7e22ce'],
  [FilePdfOutlined, '#b91c1c'],
  [FileExcelOutlined, '#047857'],
  [FileWordOutlined, '#0e7490'],
  [FileZipOutlined, '#806015'],
  [CodeOutlined, '#9d174d'],
  [DiffOutlined, '#b44b13'],
]);

function attachmentIcon(name: string) {
  const extension = name.toLowerCase().match(/\.([a-z\d]+)$/)?.[1] ?? '';
  const Icon = Object.prototype.hasOwnProperty.call(attachmentIcons, extension) ? attachmentIcons[extension] : PaperClipOutlined;
  return <Icon aria-hidden style={{ color: attachmentColors.get(Icon) }} />;
}

export default function ReviewDocuments(props: Props) {
  // A fresh review identity owns all preview state; response drafts live above this component.
  const Reader = props.attachmentLayout ? ReviewAttachmentReader : ReviewReader;
  return <Reader key={JSON.stringify([props.projectId, props.jobId, props.requestId, props.source?.identity])} {...props} />;
}

function ReviewAttachmentReader({ changes, selection, documents, documentMetadata = {}, overviewDocumentId, attachmentLayout, attachmentsLabel, primaryActions, ...props }: Props) {
  const primary = attachmentLayout === 'primary' ? startingReviewDocument(documents, documentMetadata, overviewDocumentId) : undefined;
  const [picked, setPicked] = useState<string>();
  const [showChanges, setShowChanges] = useState(false);
  const [intent, setIntent] = useState<DocumentIntent>();
  const nextIntent = useRef(0);
  const track = useRef<HTMLDivElement>(null);
  const opener = useRef<HTMLElement>();
  const selected = (selection ? selection.documentId : picked) ?? primary;
  const attachment = selected !== primary && selected !== undefined && Object.prototype.hasOwnProperty.call(documents, selected) ? selected : undefined;
  const open = attachment !== undefined || showChanges;
  const select = selection?.onSelect ?? setPicked;
  const title = (id: string) => documentTitle(id, documents[id], documentMetadata[id]);
  const openDocument = (id: string, fragment?: string, trigger?: HTMLElement) => {
    if (!open) opener.current = trigger;
    setShowChanges(false);
    setIntent({ documentId: id, fragment, token: ++nextIntent.current });
    select(id);
  };
  useEffect(() => {
    setIntent(previous => previous?.documentId === selected ? previous : undefined);
    setShowChanges(false);
  }, [selected]);
  const close = () => {
    setShowChanges(false);
    setIntent(undefined);
    if (attachment && primary !== undefined) select(primary, { replace: true });
    else setPicked(undefined);
  };
  const restoreFocus = () => {
    if (open) return;
    let trigger = opener.current;
    opener.current = undefined;
    if (trigger instanceof HTMLAnchorElement && !trigger.isConnected) {
      // The Markdown renderer can recreate links while the preview is open.
      const href = trigger.getAttribute('href');
      const label = trigger.textContent;
      trigger = Array.from(track.current?.querySelectorAll<HTMLAnchorElement>('a[href]') ?? [])
        .find(link => link.getAttribute('href') === href && link.textContent === label);
    }
    trigger?.focus({ preventScroll: true });
    if (!trigger || document.activeElement !== trigger) {
      track.current?.querySelector<HTMLButtonElement>('.review-primary-document .review-document-options button')?.focus({ preventScroll: true });
    }
  };
  const reader = { ...props, documents, documentMetadata, overviewDocumentId, openLinkedDocument: openDocument, bare: true };
  const attachments = orderedReviewDocumentIds(documents, documentMetadata).filter(id => id !== primary);
  return <div ref={track} className="review-reading-track">
    {primary !== undefined && <ReviewReader {...reader} primary primaryActions={primaryActions} selection={{ documentId: primary, onSelect: select }}
      linkedIntent={intent?.documentId === primary ? intent : undefined} />}
    {(attachments.length > 0 || changes) && <section className="review-attachments" aria-label={attachmentsLabel}>
      <Typography.Title level={5}>Attachments</Typography.Title>
      <nav aria-label={attachmentsLabel ?? 'Review attachments'}>
        {attachments.map(id => {
          const document = documents[id];
          const name = filename(id, document);
          return <Button key={id} icon={attachmentIcon(name)} disabled={!!document.unavailableReason}
            title={document.unavailableReason ?? name} onClick={event => openDocument(id, undefined, event.currentTarget)}>{title(id)}</Button>;
        })}
        {changes && <Button icon={<DiffOutlined aria-hidden style={{ color: attachmentColors.get(DiffOutlined) }} />} onClick={event => {
          opener.current = event.currentTarget;
          setShowChanges(true);
        }}>Changes</Button>}
      </nav>
    </section>}
    <Modal className="review-preview-modal" open={open} title={showChanges ? 'Changes' : attachment && title(attachment)}
      width="95vw" centered footer={null} destroyOnHidden onCancel={close} focusTriggerAfterClose={false} afterClose={restoreFocus}
      styles={{ content: { height: '95dvh', display: 'flex', flexDirection: 'column' },
        body: { minHeight: 0, maxHeight: 'none', overflow: 'auto' } }}>
      {showChanges ? changes : attachment && <ReviewReader {...reader} selection={{ documentId: attachment, onSelect: select }} linkedIntent={intent?.documentId === attachment ? intent : undefined} />}
    </Modal>
  </div>;
}

function ReviewReader({ projectId, jobId, requestId, documents, documentMetadata = {}, overviewDocumentId, source, response, navigationDefault=false, stackNavigation=false, layout, changes, selection: controlled, bare, primary, primaryActions, openLinkedDocument, linkedIntent }: ReaderProps) {
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
  const [navigationIntent, setNavigationIntent] = useState<DocumentIntent>();
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
        reveal.current = linkedIntent?.documentId !== selected;
      }
      setReaderTab('documents');
      previousSelection.current = selected;
      pendingSelection.current = undefined;
    }
  }, [selected, controlled, linkedIntent]);
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
  const nameOf = (id: string) => filename(id, documents[id]);
  const title = (id: string) => documentTitle(id, documents[id], documentMetadata[id]);
  const openDocument = (id: string, focus = false, fragment?: string, trigger?: HTMLElement) => {
    if (!attached(id)) return false;
    if (openLinkedDocument && id !== selected) {
      openLinkedDocument(id, fragment, trigger);
      return true;
    }
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
      {!documents[id]?.hideIdentifier && <Typography.Text type="secondary">{id}{title(id) !== nameOf(id) && id !== nameOf(id) ? ` · ${nameOf(id)}` : ''}</Typography.Text>}
      {metadata?.description && <p>{metadata.description}</p>}
      {documents[id]?.details}
    </div>;
  };
  const sources = (id: string) => {
    const sourceIds = [...new Set(documentMetadata[id]?.source_document_ids || [])].filter(source => source !== id);
    return sourceIds.length ? <div className="review-document-sources" aria-label="Source documents">Source documents: {sourceIds.map(source => attached(source) && !!documentURL(source)
      ? <a key={source} href={documentURL(source)} onClick={event => {
        if (event.button || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
        event.preventDefault();
        openDocument(source, true, undefined, event.currentTarget);
      }}>{title(source)}{title(source) !== nameOf(source) ? ` · ${nameOf(source)}` : ''} ({source})</a>
      : <span key={source}>{source} (unavailable)</span>)}</div> : null;
  };
  const details = selected !== undefined ? <div className={bare ? 'review-document-menu-details' : 'review-selected-details'}>{summary(selected)}{sources(selected)}<Button disabled={!url} href={url || undefined} download={name}>Download</Button></div> : null;
  const storedBytes = doc?.sizeBytes ?? doc?.stored?.key.sizeBytes;
  const activeIntent = navigationIntent?.documentId === selected ? navigationIntent : linkedIntent;
  const preview = selected === undefined ? null : error ? <Alert type="error" showIcon message={error} action={<Button onClick={() => { cached.current = undefined; setContent(undefined); setRetry(v => v + 1); }}>Retry</Button>} />
    : !content || contentIdentity !== contentKey ? <Spin aria-label="Loading document" />
    : <DocumentPreview htmlContext={{canonicalPath: path(selected), attachments: Object.keys(documents).filter(id => !!documentURL(id)).map(id => ({key:id, path:path(id), url:documentURL(id)}))}} reviewOptions={{ storedBytes, scrollMarkdownTables: bare || !!layout, details: bare ? details : undefined, actions: primaryActions }} key={contentKey} data={content.data} name={name} markdownSource={markdownSource} setMarkdownSource={setMarkdownSource} dual={dual} setDual={setDual} navigationIntent={activeIntent} navigation={{
      resolve: reference => resolveArtifactLink(reference, path(selected), Object.entries(documents).map(([key, d]) => ({key, path: path(key), aliases: [key, ...(d.name ? [d.name] : [])]}))),
      open: (key, fragment, trigger) => openDocument(key, true, fragment, trigger),
      url: source ? documentURL : undefined,
    }} resolveAttachment={reference => {
      const target = resolveArtifactLink(reference, path(selected), Object.entries(documents).map(([key, d]) => ({ key, path: path(key), aliases: [key, ...(d.name ? [d.name] : []), path(key)] })));
      if (target.kind !== 'target' || !attached(target.key) || !documentURL(target.key)) return undefined;
      const id = target.key, d = documents[id];
      return { key: id, name: d.name || path(id), url: documentURL(id), onError: source?.onError };
    }} />;
  const documentContent = <section ref={previewRoot} className={`review-document-content${bare ? ' review-v2-document' : ''}${primary ? ' review-primary-document' : ''}`} aria-label="Selected document">
    {selected === undefined ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No documents attached" /> : <>
      {(!content || contentIdentity !== contentKey || error) && <DocumentOptions key={url} storedBytes={storedBytes} details={bare ? details : undefined} actions={primaryActions} />}
      {preview}{!bare && details}
    </>}
  </section>;
  if (bare) return documentContent;
  const documentPanel = <Card>{documentContent}</Card>;
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
