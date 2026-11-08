import { useState } from 'react';
import { isJobJSONNumber, stringifyJobJSON } from '@colony2/shared';
import './NodePayloadView.css';

type Props = { value: unknown; label: 'Input' | 'Output'; identity: string };

// Remount on selection/attempt changes, while tab visibility preserves local state.
export default function NodePayloadView(props: Props) {
  return <Payload key={props.identity} {...props} />;
}

function Payload({ value, label }: Props) {
  const [mode, setMode] = useState('structured');
  const [feedback, setFeedback] = useState('');
  if (value === undefined) return <p>No {label.toLowerCase()} payload</p>;
  const json = () => stringifyJobJSON(value, 2) ?? '';
  async function copy() {
    try {
      await navigator.clipboard.writeText(json());
      setFeedback('JSON copied');
    } catch {
      setFeedback('Could not copy JSON');
    }
  }
  return <section className="node-payload" aria-label={`${label} payload`}>
    <div className="node-payload-toolbar">
      <div role="group" aria-label={`${label} display`}>
        <button type="button" aria-pressed={mode === 'structured'} onClick={() => setMode('structured')}>Structured</button>
        <button type="button" aria-pressed={mode === 'json'} onClick={() => setMode('json')}>JSON</button>
      </div>
      <button type="button" onClick={copy}>Copy JSON</button>
      <span role="status">{feedback}</span>
    </div>
    {mode === 'json' && <pre aria-label={`${label} JSON`}>{json()}</pre>}
    <div hidden={mode !== 'structured'}><Value value={value} name={label} root depth={0} showJSON={() => setMode('json')} /></div>
  </section>;
}

function Value({ value, name, root = false, depth, showJSON }: {
  value: unknown; name: string; root?: boolean; depth: number; showJSON: () => void;
}) {
  const [open, setOpen] = useState(root);
  const [limit, setLimit] = useState(100);
  const [full, setFull] = useState(false);
  const number = isJobJSONNumber(value);
  if (value !== null && typeof value === 'object' && !number) {
    const array = Array.isArray(value);
    const count = array ? value.length : Object.keys(value).length;
    const type = array ? 'Array' : 'Object';
    const summary = `${type} · ${count} ${array ? (count === 1 ? 'item' : 'items') : (count === 1 ? 'field' : 'fields')}`;
    // Enumerate only the visible slice; collapsed groups never mount descendants.
    const keys = open ? (array ? Array.from({ length: Math.min(count, limit) }, (_, i) => String(i)) : Object.keys(value).slice(0, limit)) : [];
    return <div className="node-payload-group">
      <button className="node-payload-disclosure" type="button" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span aria-hidden="true">{open ? '▾' : '▸'} </span><span className="node-payload-name">{name}</span> <span>{summary}</span>
      </button>
      {open && (depth >= 30 ? <button type="button" onClick={showJSON}>Nesting limit reached — View JSON</button> : <div className="node-payload-children">
        {keys.map(key => <Value key={JSON.stringify([array ? 'index' : 'field', key])} name={key} value={(value as Record<string, unknown>)[key]} depth={depth + 1} showJSON={showJSON} />)}
        {count > limit && <button type="button" onClick={() => setLimit(limit + 100)}>Show 100 more</button>}
      </div>)}
    </div>;
  }
  const text = number ? stringifyJobJSON(value)! : typeof value === 'string' ? value : String(value);
  const preview = typeof value === 'string' && text.length > 4000;
  return <div className="node-payload-row">
    <span className="node-payload-name">{name}</span>
    <div className="node-payload-value">{value === '' ? <em>Empty string</em> : <span>{preview && !full ? text.slice(0, 4000) : text}</span>}
      {preview && <><span> {full ? '' : '(Preview: first 4,000 characters)'} </span><button type="button" onClick={() => setFull(!full)}>{full ? 'Show less' : 'Show full'}</button></>}
    </div>
  </div>;
}
