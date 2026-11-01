import { useMemo, useState } from 'react';
import { Alert, Button, Radio, Space } from 'antd';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { ReviewDocument } from '@colony2/shared';
import { inputActivityService } from '@colony2/shared';
import { useEffect } from 'react';
import { decodeText, TEXT_LIMIT, hexDump, loadDocument, structuredAllowed, type DocumentData } from './document';
import { formatJSON, jsonLineErrors } from './structured';
import SourceView from './SourceView';
import DiffView from './DiffView';
import ImageView from './ImageView';

function AttachedImage({ id, name, url }: { id: string; name: string; url: string }) {
  const [data, setData] = useState<DocumentData>();
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    loadDocument(url,name,controller.signal).then(d => { if (!controller.signal.aborted) { if (d.format === 'image') setData(d); else setError('Attachment is not a raster image.'); } }).catch(e => { if (!controller.signal.aborted) setError(e.message); });
    return () => controller.abort();
  }, [url, name]);
  return error ? <span>{error}</span> : data ? <ImageView data={data} name={name || id} /> : <span>Loading attached image…</span>;
}
interface Props { markdownSource?: boolean; setMarkdownSource?: (source: boolean) => void; data: DocumentData; name: string; dual: boolean; setDual: (value: boolean) => void; documents: Record<string, ReviewDocument>; projectId: string; jobId: string; requestId: string }
export default function DocumentPreview({ data, name, markdownSource = false, setMarkdownSource, dual, setDual, documents, projectId, jobId, requestId }: Props) {
  const [mode, setMode] = useState('rendered');
  const [textOverride, setTextOverride] = useState(false);
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
  return <div className="review-document-preview">
    <Space wrap><span>{format} · {data.bytes.length.toLocaleString()} loaded bytes</span>
      <Button onClick={() => setTextOverride(v => !v)}>{textOverride ? 'Detect format' : 'View as text'}</Button>
      {format === 'markdown' && <Radio.Group aria-label="Document display" value={markdownSource ? 'source' : 'rendered'} onChange={e => setMarkdownSource?.(e.target.value === 'source')} optionType="button" options={[{ label: 'Rendered', value: 'rendered' },{ label: 'Markdown', value: 'source' }]} />}
      {format === 'diff' && structured && <><Radio.Group aria-label="Diff layout" value={dual ? 'dual' : 'unified'} onChange={e => { setDual(e.target.value === 'dual'); setMode('rendered'); }} optionType="button" options={[{ label: 'Unified', value: 'unified' }, { label: 'Side by side', value: 'dual' }]} /><Button onClick={() => setMode(mode === 'source' ? 'rendered' : 'source')}>{mode === 'source' ? 'Show diff' : 'Source'}</Button></>}
      {format === 'json' && structured && <Radio.Group aria-label="JSON display" value={mode} onChange={e => setMode(e.target.value)} optionType="button" options={[{label:'Formatted',value:'rendered'},{label:'Source',value:'source'}]} />}
    </Space>
    {(data.truncated || overrideLimited) && <Alert type="warning" message="Preview truncated. Search covers only loaded content. Download the complete original." />}
    {['json','diff','jsonl','markdown'].includes(format) && !structured && <Alert type="info" message="Structured preview limit exceeded or input incomplete; showing bounded source." />}
    {data.bytes.length === 0 ? <Alert type="info" message="Empty document" /> : format === 'container' ? <Alert type="info" message="Container data: download the original file to inspect its contents. Archives are not extracted." />
      : format === 'image' ? <ImageView data={data} name={name} />
      : format === 'binary' ? <><p>Binary preview: first 64 KiB at most.</p><SourceView text={hexDump(data.bytes)} label="Binary bytes" /></>
      : format === 'markup' ? <><Alert type="info" message="HTML and SVG are shown as source only; active markup is not rendered." />{source}</>
      : format === 'markdown' && !markdownSource && structured ? <article tabIndex={0} className="review-markdown" aria-label="Rendered document"><Markdown remarkPlugins={[remarkGfm]} skipHtml components={{
        p: ({ children }) => <div className="review-markdown-paragraph">{children}</div>,
        a: ({ children, href }) => <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>,
        img: ({ src, alt }) => {
          // Resolve only exact current-review IDs or unambiguous stored names. Never load external URLs.
          const matches = Object.entries(documents).filter(([id, d]) => src === id || src === d.name || src === d.stored.key.name);
          if (!src || /^(?:[a-z][a-z\d+.-]*:|\/\/|\/)/i.test(src) || matches.length !== 1) return <span>Blocked external or ambiguous image: {alt || src}</span>;
          const [id, d] = matches[0];
          return <AttachedImage key={id} id={id} name={d.name || d.stored.key.name} url={inputActivityService.reviewDocumentURL(projectId,jobId,requestId,id)} />;
        },
      }}>{data.text}</Markdown></article>
      : format === 'diff' && structured && mode !== 'source' ? <DiffView text={data.text} dual={dual} />
      : format === 'json' && structured && mode !== 'source' ? json.error ? <><Alert type="warning" message={`JSON preview unavailable: ${json.error}`} />{source}</> : <details open><summary>Formatted JSON — collapse / expand</summary><SourceView text={json.formatted || ''} label="Formatted JSON" /></details>
      : format === 'jsonl' ? <>{!!lineErrors.length && <Alert type="warning" message={lineErrors.slice(0,100).join('; ')} />}{source}</>
      : source}
  </div>;
}
