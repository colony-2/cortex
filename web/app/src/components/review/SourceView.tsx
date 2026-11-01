import { useMemo, useState } from 'react';
import { Button, Checkbox, Input, Space } from 'antd';
const PAGE = 24000;
export default function SourceView({ text, label = 'Document source' }: { text: string; label?: string }) {
  const [query, setQuery] = useState('');
  const [match, setMatch] = useState(0);
  const [page, setPage] = useState(0);
  const [wrap, setWrap] = useState(false);
  const matches = useMemo(() => {
    if (!query) return [];
    const found: number[] = []; let p = 0;
    while ((p = text.indexOf(query, p)) !== -1) { found.push(p); p += Math.max(1, query.length); }
    return found;
  }, [query, text]);
  const pages = Math.max(1, Math.ceil(text.length / PAGE));
  const currentPage = Math.min(page, pages - 1);
  const start = currentPage * PAGE, end = start + PAGE;
  const excerpt = text.slice(start, end);
  const firstLine = text.slice(0, start).split('\n').length;
  const activeOffset = matches[match % Math.max(1, matches.length)];
  const local = activeOffset === undefined ? -1 : activeOffset - start;
  const display = (part: string) => part.replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g, c => `\\x${c.charCodeAt(0).toString(16).padStart(2,'0')}`);
  function jump(delta: number) {
    if (!matches.length) return;
    const index = (match + delta + matches.length) % matches.length;
    setMatch(index); setPage(Math.floor(matches[index] / PAGE));
  }
  return <div className="review-source">
    <Space wrap>
      <Input aria-label="Search document" placeholder="Search loaded content" value={query} onChange={e => { setQuery(e.target.value); setMatch(0); const p = text.indexOf(e.target.value); if (p >= 0) setPage(Math.floor(p / PAGE)); }} style={{ width: 210 }} />
      <span aria-live="polite">{matches.length ? `${match % matches.length + 1} / ${matches.length}` : '0 matches'}</span>
      <Button disabled={!matches.length} onClick={() => jump(-1)}>Previous match</Button><Button disabled={!matches.length} onClick={() => jump(1)}>Next match</Button>
      <Checkbox checked={wrap} onChange={e => setWrap(e.target.checked)}>Wrap lines</Checkbox>
    </Space>
    {pages > 1 && <Space wrap><Button disabled={!currentPage} onClick={() => setPage(currentPage - 1)}>Previous portion</Button><span>Portion {currentPage + 1} / {pages}</span><Button disabled={currentPage + 1 >= pages} onClick={() => setPage(currentPage + 1)}>Next portion</Button></Space>}
    <div className="review-source-body" tabIndex={0}>
      <pre aria-hidden="true" className="review-line-numbers">{excerpt.split('\n').map((_, i) => firstLine + i).join('\n')}</pre>
      <pre role="region" tabIndex={0} aria-label={label} style={{ whiteSpace: wrap ? 'pre-wrap' : 'pre', overflowWrap: wrap ? 'anywhere' : undefined }}>{local >= 0 && local < excerpt.length ? <>{display(excerpt.slice(0,local))}<mark>{display(excerpt.slice(local, local + query.length))}</mark>{display(excerpt.slice(local + query.length))}</> : display(excerpt)}</pre>
    </div>
    {pages > 1 && <p>Source is displayed in bounded portions; search covers all loaded content.</p>}
  </div>;
}
