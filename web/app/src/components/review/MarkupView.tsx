import { useEffect, useState } from 'react';
import { Alert, Button, Radio, Space } from 'antd';
import type { DocumentData } from './document';
import { prepareMarkup, RESTRICTIONS, type MarkupPreview } from './markup';
import SourceView from './SourceView';

export default function MarkupView({ data, name }: { data: DocumentData; name: string }) {
  const [mode, setMode] = useState('rendered');
  const [preview, setPreview] = useState<MarkupPreview & { url?: string }>();
  const [error, setError] = useState('');
  const [zoom, setZoom] = useState(0);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    setMode('rendered'); setZoom(0); setError(''); setPreview(undefined);
    let result: MarkupPreview | undefined, url: string | undefined;
    try {
      result = prepareMarkup(data);
      if (data.format === 'svg') url = URL.createObjectURL(new Blob([result.content], { type: 'image/svg+xml' }));
      setPreview({ ...result, url });
    } catch (e) {
      if (url) { URL.revokeObjectURL(url); url = undefined; }
      result?.release(); result = undefined;
      const message = (e as Error).message;
      setError(/showing (?:bounded )?source/i.test(message) ? message : `${message} Showing source.`);
    }
    return () => { if (url) URL.revokeObjectURL(url); result?.release(); };
  }, [data, retry]);
  const failed = () => setError('Could not display this preview. Source and original download remain available.');
  return <>
    <Space wrap><Radio.Group aria-label="Markup display" value={mode} onChange={e => setMode(e.target.value)} optionType="button" options={[{ label: 'Rendered', value: 'rendered' }, { label: 'Source', value: 'source' }]} /></Space>
    <Alert type="info" message={RESTRICTIONS} />
    {error && <Alert type="warning" message={error} action={<Button onClick={() => setRetry(v => v + 1)}>Retry rendering</Button>} />}
    {mode === 'source' || error ? <div className="review-markup-source"><SourceView text={data.text} label="Document source" /></div> : preview ? data.format === 'html'
      ? <iframe title={`Static HTML preview: ${name}`} sandbox="" referrerPolicy="no-referrer" srcDoc={preview.content} onError={failed} style={{ width: '100%', height: 'min(65vh, 720px)', minHeight: 240, border: '1px solid #ddd', background: 'white' }} />
      : <>
        <Space wrap><span>{preview.width} × {preview.height}</span><Button onClick={() => setZoom(0)}>Fit</Button><Button onClick={() => setZoom(1)}>Actual size</Button><Button onClick={() => setZoom(Math.min(8, (zoom || 1) * 1.25))}>Zoom in</Button><Button onClick={() => setZoom(Math.max(.1, (zoom || 1) / 1.25))}>Zoom out</Button></Space>
        <div className="review-image-scroll" tabIndex={0} role="region" aria-label="SVG preview" style={{ maxHeight: '65vh', overflow: 'auto' }}>
          <img src={preview.url} alt={name} width={zoom ? preview.width * zoom : undefined} style={{ maxWidth: zoom ? 'none' : '100%', maxHeight: zoom ? undefined : '60vh', height: 'auto' }} onError={failed} />
        </div>
      </> : <span role="status">Preparing static preview…</span>}
  </>;
}
