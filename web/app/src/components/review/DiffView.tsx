import { useMemo, useRef, useState } from 'react';
import { Alert, Select } from 'antd';
import { parseDiff, alignDiff, type DiffLine } from './diff';
import SourceView from './SourceView';
export default function DiffView({ text, dual }: { text: string; dual: boolean }) {
  const result = useMemo(() => { try { return { files: parseDiff(text) }; } catch (e) { return { error: (e as Error).message }; } }, [text]);
  const [selected, setSelected] = useState(0);
  const sections = useRef<(HTMLElement | null)[]>([]);
  if (!result.files) return <><Alert type="warning" message={result.error} /><SourceView text={text} /></>;
  const cell = (line: DiffLine | undefined, side: 'old' | 'next') => <><td className="diff-number">{line?.[side]}</td><td className={`diff-${line?.kind || 'blank'}`}>{line?.text || ''}</td></>;
  return <div>
    {!!result.files.length && <Select aria-label="Patch file" value={selected} style={{ maxWidth: '100%', width: 400 }} options={result.files.map((f,i) => ({ label: f.name, value: i }))} onChange={i => { setSelected(i); sections.current[i]?.scrollIntoView?.({ block: 'nearest' }); }} />}
    {result.files.map((file, i) => <section key={i} ref={el => { sections.current[i] = el; }}>
      <h4>{file.name}</h4>
      {file.lines.some(l => /^(GIT binary patch|Binary files )/.test(l.text)) && <Alert type="info" message="Binary patch metadata only; image contents cannot be reconstructed." />}
      <div className="review-diff-scroll" tabIndex={0} role="region" aria-label={dual ? 'Side by side diff' : 'Unified diff'}>
        <table className="review-diff"><thead><tr>{dual ? <><th colSpan={2}>Old</th><th colSpan={2}>New</th></> : <><th>Old</th><th>New</th><th>Change</th></>}</tr></thead>
          <tbody>{dual ? alignDiff(file.lines).map((r,n) => <tr key={n}>{cell(r.left,'old')}{cell(r.right,'next')}</tr>) : file.lines.map((l,n) => <tr key={n}><td className="diff-number">{l.old}</td><td className="diff-number">{l.next}</td><td className={`diff-${l.kind}`}>{l.text}</td></tr>)}</tbody>
        </table>
      </div>
    </section>)}
  </div>;
}
