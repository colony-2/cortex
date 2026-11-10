import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Radio, Space } from 'antd';
import type { DocumentData } from './document';
import { prepareHTML, nativeHTMLSupported, HTML_NOTICE, HTML_PERMISSIONS, INTERACTIVE_POLICY, type HTMLContext, type PreparedHTML } from './htmlPreparation';
import SourceView from './SourceView';

export default function HTMLView({ data, name, context }: { data: DocumentData; name: string; context?: HTMLContext }) {
  const [mode, setMode] = useState('rendered');
  const [retry, setRetry] = useState(0);
  const [state, setState] = useState<{data: DocumentData; generation: number; prepared?: PreparedHTML; url?: string; error?: string}>();
  const generation = useRef(0);
  const frame = useRef<HTMLDivElement>(null);
  const identity = JSON.stringify(context);
  useEffect(() => {
    const current = ++generation.current;
    if (mode === 'source') { setState(undefined); return; }
    const controller = new AbortController();
    let url: string | undefined;
    setState({data, generation: current});
    const failed = (message: string) => { if (generation.current === current && !controller.signal.aborted) setState({data, generation: current, error: message}); };
    const timer = setTimeout(() => { failed('HTML preparation timed out. Source and original Download remain available.'); controller.abort(); }, 15_000);
    if (!nativeHTMLSupported()) {
      clearTimeout(timer);
      failed('Interactive HTML requires a browser with enforced iframe CSP and credentialless frames. Use a current Chromium browser; Source and original Download remain available.');
    } else prepareHTML(data, context, controller.signal).then(prepared => {
      if (generation.current !== current || controller.signal.aborted) return;
      clearTimeout(timer);
      url = URL.createObjectURL(new Blob([prepared.content], {type: 'text/html;charset=utf-8'}));
      setState({data, generation: current, prepared, url});
    }).catch(error => { clearTimeout(timer); failed(error.message); });
    return () => { ++generation.current; clearTimeout(timer); controller.abort(); frame.current?.replaceChildren(); if (url) URL.revokeObjectURL(url); };
  }, [data, identity, mode, retry]);
  const active = state?.data === data && mode !== 'source' ? state : undefined;
  useEffect(() => {
    if (!active?.url || !frame.current) return;
    const element = document.createElement('iframe');
    element.title = `Interactive HTML preview: ${name}`;
    element.setAttribute('sandbox', 'allow-scripts');
    element.setAttribute('referrerpolicy', 'no-referrer');
    element.setAttribute('credentialless', '');
    element.setAttribute('csp', INTERACTIVE_POLICY);
    element.setAttribute('allow', HTML_PERMISSIONS);
    element.style.cssText = 'width:100%;height:min(65vh,720px);min-height:240px;border:1px solid #ddd;background:white';
    // Configure the complete browser boundary before attaching or loading content.
    element.src = active.url;
    element.onerror = () => { if (generation.current === active.generation) { element.remove(); setState({...active, url: undefined, error: 'Could not display this preview. Source and original Download remain available.'}); } };
    frame.current.replaceChildren(element);
    return () => { element.onerror = null; element.remove(); };
  }, [active?.url, name]);
  return <>
    <Space wrap><Radio.Group aria-label="Markup display" value={mode} onChange={e => setMode(e.target.value)} optionType="button" options={[{label:'Rendered',value:'rendered'},{label:'Source',value:'source'}]} /></Space>
    <Alert type="info" message={HTML_NOTICE} />
    {active?.error && <Alert type="warning" message={active.error} action={<Button onClick={() => setRetry(v => v + 1)}>Retry rendering</Button>} />}
    {!!active?.prepared?.warnings.length && <Alert type="warning" message="Some images could not be prepared" description={<ul>{active.prepared.warnings.map((warning,i) => <li key={i}>{warning}</li>)}</ul>} />}
    {mode === 'source' || active?.error ? <div className="review-markup-source"><SourceView text={data.text} label="Document source" /></div> : <>{!active?.url && <span role="status">Preparing interactive preview…</span>}<div ref={frame} className="review-html-frame" /></>}
  </>;
}
