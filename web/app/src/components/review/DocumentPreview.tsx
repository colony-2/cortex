import { useMemo, useRef, useState } from 'react';
import { Alert, Button, Radio, Space } from 'antd';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useEffect } from 'react';
import { decodeText, TEXT_LIMIT, hexDump, loadDocument, structuredAllowed, type DocumentData } from './document';
import { formatJSON, jsonLineErrors } from './structured';
import DocumentOptions from './DocumentOptions';
import SourceView from './SourceView';
import DiffView from './DiffView';
import ImageView from './ImageView';
import MarkupView from './MarkupView';
import { artifactHeadingIds, resolveArtifactLink, type PreviewNavigation, type NavigationIntent } from './artifactLinks';

function AttachedImage({ id, name, url, onError }: { id: string; name: string; url: string; onError?: (error: unknown) => void }) {
  const [data, setData] = useState<DocumentData>();
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    loadDocument(url,name,controller.signal).then(d => { if (!controller.signal.aborted) { if (d.format === 'image') setData(d); else setError('Attachment is not a raster image.'); } }).catch(e => { if (!controller.signal.aborted) {setError(e.message); onError?.(e);} });
    return () => controller.abort();
  }, [url, name]);
  return error ? <span>{error}</span> : data ? <ImageView data={data} name={name || id} /> : <span>Loading attached image…</span>;
}
export type PreviewAttachment = { key: string; name: string; url: string; onError?: (error: unknown) => void };
export type ResolvePreviewAttachment = (reference: string) => PreviewAttachment | undefined;
interface Props { reviewOptions?: { storedBytes?: number }; markdownSource?: boolean; setMarkdownSource?: (source: boolean) => void; data: DocumentData; name: string; dual: boolean; setDual: (value: boolean) => void; resolveAttachment?: ResolvePreviewAttachment; navigation?: PreviewNavigation; navigationIntent?: NavigationIntent }
export default function DocumentPreview({ reviewOptions, data, name, markdownSource = false, setMarkdownSource, dual, setDual, resolveAttachment, navigation, navigationIntent }: Props) {
  const root = useRef<HTMLDivElement>(null);
  const [linkNotice, setLinkNotice] = useState('');
  const scrollToFragment = (fragment: string) => {
    const heading = [...(root.current?.querySelectorAll('.review-markdown [id]') || [])].find(el => el.id === fragment);
    if (heading) { heading.scrollIntoView?.({ block: 'start' }); setLinkNotice(''); }
    else setLinkNotice(`Anchor unavailable: ${fragment || '(empty)'}`);
  };
  useEffect(() => {
    setLinkNotice('');
    if (!navigationIntent) return;
    root.current?.focus({ preventScroll: true });
    if (navigationIntent.fragment !== undefined) scrollToFragment(navigationIntent.fragment);
    else root.current?.scrollIntoView?.({ block: 'start' });
  }, [data, navigationIntent]);
  const [mode, setMode] = useState('rendered');
  const [textOverride, setTextOverride] = useState(false);
  useEffect(() => { setMode('rendered'); setTextOverride(false); }, [data]);
  const structured = structuredAllowed(data);
  const json = useMemo(() => {
    if (data.format !== 'json' || !structured) return {};
    try { return { formatted: formatJSON(data.text) }; } catch (e) { return { error: (e as Error).message }; }
  }, [data, structured]);
  const lineErrors = useMemo(() => data.format === 'jsonl' && structured ? jsonLineErrors(data.text) : [], [data, structured]);
  const format = textOverride ? 'source' : data.format;
  const overrideLimited = textOverride && data.bytes.length > TEXT_LIMIT;
  const overrideText = useMemo(() => textOverride ? decodeText(data.bytes.subarray(0, TEXT_LIMIT)) : data.text, [data, textOverride]);
  const source = <SourceView text={overrideText} label={format === 'markdown' ? 'Markdown source' : 'Document source'} />;
  const controls = <>
    <Space wrap>{reviewOptions && <span>Media type: {data.mime || 'Unknown'}</span>}<span>{format} · {data.bytes.length.toLocaleString()} loaded bytes</span>
      <Button onClick={() => setTextOverride(v => !v)}>{textOverride ? 'Detect format' : 'View as text'}</Button>
      {format === 'markdown' && <Radio.Group aria-label="Document display" value={markdownSource ? 'source' : 'rendered'} onChange={e => setMarkdownSource?.(e.target.value === 'source')} optionType="button" options={[{ label: 'Rendered', value: 'rendered' },{ label: 'Markdown', value: 'source' }]} />}
      {format === 'diff' && structured && <><Radio.Group aria-label="Diff layout" value={dual ? 'dual' : 'unified'} onChange={e => { setDual(e.target.value === 'dual'); setMode('rendered'); }} optionType="button" options={[{ label: 'Unified', value: 'unified' }, { label: 'Side by side', value: 'dual' }]} /><Button onClick={() => setMode(mode === 'source' ? 'rendered' : 'source')}>{mode === 'source' ? 'Show diff' : 'Source'}</Button></>}
      {format === 'json' && structured && <Radio.Group aria-label="JSON display" value={mode} onChange={e => setMode(e.target.value)} optionType="button" options={[{label:'Formatted',value:'rendered'},{label:'Source',value:'source'}]} />}
    </Space>
  </>;
  return <div ref={root} tabIndex={-1} aria-label="Document preview" className="review-document-preview">
    {linkNotice && <div role="status">{linkNotice}</div>}
    {reviewOptions ? <DocumentOptions storedBytes={reviewOptions.storedBytes}>{controls}</DocumentOptions> : controls}

    {format === 'source' && /\.(svg|html?)$/i.test(name) && <Alert type="info" message="Markup preview unavailable: unrecognized or conflicting document root; showing source." />}
    {(data.truncated || overrideLimited) && <Alert type="warning" message="Preview truncated. Search covers only loaded content. Download the complete original." />}
    {['json','diff','jsonl','markdown'].includes(format) && !structured && <Alert type="info" message="Structured preview limit exceeded or input incomplete; showing bounded source." />}
    {data.bytes.length === 0 ? <Alert type="info" message="Empty document" /> : format === 'container' ? <Alert type="info" message="Container data: download the original file to inspect its contents. Archives are not extracted." />
      : format === 'image' ? <ImageView data={data} name={name} />
      : format === 'binary' ? <><p>Binary preview: first 64 KiB at most.</p><SourceView text={hexDump(data.bytes)} label="Binary bytes" /></>
      : format === 'html' || format === 'svg' ? <MarkupView key={name} data={data} name={name} />
      : format === 'markdown' && !markdownSource && structured ? <article tabIndex={0} className="review-markdown" aria-label="Rendered document"><Markdown remarkPlugins={[remarkGfm, artifactHeadingIds]} skipHtml components={{
        p: ({ children }) => <div className="review-markdown-paragraph">{children}</div>,
        a: ({ children, href }) => {
          // href has already passed react-markdown's default URL sanitization.
          const resolution = navigation?.resolve(href || '') || resolveArtifactLink(href || '', name, []);
          if (resolution.kind === 'browser') return <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>;
          if (resolution.kind === 'target' && navigation?.url) {
            const url = navigation.url(resolution.key);
            return url ? <a href={url} onClick={event => {
              if (event.button || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
              event.preventDefault(); navigation.open(resolution.key, resolution.fragment);
            }}>{children}</a> : <span>{children} (unavailable)</span>;
          }
          return <button type="button" className="review-artifact-link" onClick={() => {
            const current = navigation?.resolve(href || '') || resolveArtifactLink(href || '', name, []);
            if (current.kind === 'fragment') { scrollToFragment(current.fragment); return; }
            if (current.kind === 'target' && navigation?.open(current.key, current.fragment)) { setLinkNotice(''); return; }
            setLinkNotice(`Artifact unavailable: ${current.kind === 'unavailable' ? current.reason : 'outside-context'}`);
          }}>{children}</button>;
        },
        img: ({ src, alt }) => {
          const attachment = src && !/^(?:[a-z][a-z\d+.-]*:|\/\/|\/)/i.test(src) ? resolveAttachment?.(src) : undefined;
          if (!attachment) return <span>Blocked external or ambiguous image: {alt || src}</span>;
          return <AttachedImage key={JSON.stringify([attachment.key, attachment.url])} id={attachment.key} name={attachment.name} url={attachment.url} onError={attachment.onError} />;
        },
      }}>{data.text}</Markdown></article>
      : format === 'diff' && structured && mode !== 'source' ? <DiffView text={data.text} dual={dual} />
      : format === 'json' && structured && mode !== 'source' ? json.error ? <><Alert type="warning" message={`JSON preview unavailable: ${json.error}`} />{source}</> : <details open><summary>Formatted JSON — collapse / expand</summary><SourceView text={json.formatted || ''} label="Formatted JSON" /></details>
      : format === 'jsonl' ? <>{!!lineErrors.length && <Alert type="warning" message={lineErrors.slice(0,100).join('; ')} />}{source}</>
      : source}
  </div>;
}
