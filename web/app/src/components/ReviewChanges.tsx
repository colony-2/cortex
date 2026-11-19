import { useEffect, useMemo, useReducer, useRef, useState } from 'react';
import { Alert, Button, Card, Pagination, Select, Space, Spin, Tag } from 'antd';
import { ChangesReadError, reviewChangesService, type ChangesCommit, type ChangesComparison, type ChangesHistory, type ChangesPreview } from '@colony2/shared';
import { changeSelection, initialChangeSelection } from './review/changeSelection';
import DocumentPreview from './review/DocumentPreview';

interface Props { projectId: string; jobId: string }

function ReadError({ error, retry, refresh }: { error: unknown; retry: () => void; refresh: () => void }) {
  const diagnostic = error instanceof ChangesReadError ? error.diagnostic : undefined;
  const stale = diagnostic?.code === 'stale_selection';
  const unavailable = ['workspace_unavailable', 'workspace_ambiguous', 'missing_source'].includes(diagnostic?.code || '');
  return <Alert type={unavailable ? 'info' : 'error'} showIcon
    message={stale ? 'Recorded workspace changed. Refresh changes to view its latest status.' : unavailable ? 'Change history is unavailable for this job.' : 'Changes could not be read.'}
    description={<>{error instanceof Error ? error.message : 'The read failed.'}{diagnostic && <div>{diagnostic.code}{diagnostic.limit && ` · ${diagnostic.limit}`}{diagnostic.missing?.map(hash => <code className="changes-hash" key={hash}>{hash}</code>)}</div>}</>}
    action={<Button onClick={stale ? refresh : retry}>{stale ? 'Refresh changes' : 'Retry'}</Button>} />;
}

export default function ReviewChanges({ projectId, jobId }: Props) {
  const [loaded, setLoaded] = useState<{ history: ChangesHistory; commits: ChangesCommit[] }>();
  const [error, setError] = useState<unknown>();
  const [revision, setRevision] = useState(0);
  const generation = useRef(0);
  const refresh = () => { generation.current++; setLoaded(undefined); setError(undefined); setRevision(v => v + 1); };
  useEffect(() => {
    const current = ++generation.current;
    const controller = new AbortController();
    setLoaded(undefined); setError(undefined);
    const load = async () => {
      const history = await reviewChangesService.history(projectId, jobId, controller.signal);
      const commits = [...history.commits];
      const seen = new Set<string>();
      let cursor = history.next_cursor;
      while (cursor) {
        if (seen.has(cursor) || commits.length >= history.total) throw new Error('Invalid changes pagination.');
        seen.add(cursor);
        const page = await reviewChangesService.history(projectId, jobId, controller.signal, cursor);
        if (page.selection !== history.selection || page.total !== history.total || page.history !== history.history) throw new Error('Changes pages refer to different workspace states. Refresh changes.');
        commits.push(...page.commits); cursor = page.next_cursor;
      }
      if (history.complete && (commits.length !== history.total || commits.some((c, i) => c.position !== i + 1))) throw new Error('Commit metadata is incomplete.');
      if (!controller.signal.aborted && generation.current === current) setLoaded({ history, commits });
    };
    load().catch(e => { if (!controller.signal.aborted && generation.current === current) setError(e); });
    return () => { controller.abort(); generation.current++; };
  }, [projectId, jobId, revision]);
  return <section aria-label="Job changes" className="review-changes">
    <Space wrap className="changes-heading"><span>Latest recorded main workspace</span><Button onClick={refresh}>Refresh changes</Button></Space>
    {error ? <ReadError error={error} retry={refresh} refresh={refresh} /> : loaded
      ? <ChangesContent key={JSON.stringify([projectId, jobId, loaded.history.selection])} {...loaded} projectId={projectId} jobId={jobId} refresh={refresh} />
      : <Spin aria-label="Loading change history" />}
  </section>;
}

function ChangesContent({ history, commits, projectId, jobId, refresh }: Props & { history: ChangesHistory; commits: ChangesCommit[]; refresh: () => void }) {
  const [selection, dispatch] = useReducer(changeSelection, initialChangeSelection(history.selection, commits.length));
  const [page, setPage] = useState(1);
  const [dual, setDual] = useState(false);
  const [patch, setPatch] = useState<{ key: string; result: ChangesPreview }>();
  const [error, setError] = useState<{ key: string; error: unknown }>();
  const [retry, setRetry] = useState(0);
  const rows = useRef(new Map<number, HTMLButtonElement>());
  const sequence = useRef(0);
  const range = selection.range;
  const comparison: ChangesComparison = range ? range.from === range.through ? { commit: commits[range.from].hash }
    : { from: commits[range.from].hash, through: commits[range.through].hash } : { all: true };
  const key = JSON.stringify(comparison);
  const canDiff = history.diff === 'complete' && !history.unborn;
  useEffect(() => {
    setPatch(undefined); setError(undefined);
    if (!canDiff) return;
    const current = ++sequence.current;
    const controller = new AbortController();
    reviewChangesService.diff(projectId, jobId, history.selection, comparison, controller.signal).then(result => {
      if (result.metadata.selection !== history.selection) throw new Error('Patch belongs to another workspace state.');
      if (!controller.signal.aborted && sequence.current === current) setPatch({ key, result });
    }).catch(e => { if (!controller.signal.aborted && sequence.current === current) setError({ key, error: e }); });
    return () => { controller.abort(); sequence.current++; };
  }, [projectId, jobId, history.selection, key, retry, canDiff]);
  const result = patch?.key === key ? patch.result : undefined;
  const data = useMemo(() => result ? { bytes: new TextEncoder().encode(result.patch), text: result.patch, format: 'diff' as const, truncated: !result.preview_complete } : undefined, [result]);
  const choose = (position: number, shift = false) => dispatch({ type: 'select', position, shift });
  const chooseEndpoints = (from: number, through: number) => {
    dispatch({ type: 'endpoints', from, through });
    // Endpoint selection focuses the normalized upper endpoint. Render its page
    // so Tab can reach that row while the selector retains DOM focus.
    setPage(Math.floor(Math.max(from, through) / 50) + 1);
  };
  const options = commits.map((c, i) => ({ value: i, label: `${i + 1}. ${c.subject} (${c.hash.slice(0, 8)})` }));
  return <>
    <Card size="small" className="changes-identity">
      <strong>{history.repository}</strong><div>Scope: {history.scope || 'main'}{history.cell && ` · ${history.cell}`}</div>
      <div>Base <Tag>{history.base_origin === 'explicit' ? 'Explicit' : 'Active'}</Tag><code className="changes-hash">{history.empty_base ? 'Empty tree' : history.base}</code></div>
      <div>Head <code className="changes-hash">{history.unborn ? 'Unborn workspace' : history.head}</code></div>
      {history.complete && <div>{history.total} {history.total === 1 ? 'commit' : 'commits'} since base</div>}
      <details><summary>Recorded source</summary><div>Task: <code className="changes-hash">{history.state.task}</code>Attempt: {history.state.attempt} · {history.boundary}</div><div>Source: <code className="changes-hash">{history.source_id}</code></div></details>
    </Card>
    {history.history === 'unsupported' && <Alert type="warning" showIcon message="Unsupported history: merge commit in captured range" description="Commit and range selection is unavailable. All changes shows only the net difference between base and head." />}
    {!history.complete && history.history !== 'unsupported' && <Alert type="warning" message="History is incomplete; commit selection is unavailable." />}
    {history.unborn ? <Alert type="info" message="No committed changes in this unborn workspace." /> : history.complete && history.total === 0 && <Alert type="info" message="No commits since base." />}
    <Space wrap className="changes-controls">
      <Button type={!range ? 'primary' : 'default'} onClick={() => dispatch({ type: 'all' })}>All changes</Button>
      {history.complete && !!commits.length && <>
        <label>From <Select aria-label="From commit" value={range?.from} placeholder="First commit" options={options} onChange={from => chooseEndpoints(from, range?.through ?? from)} /></label>
        <label>Through <Select aria-label="Through commit" value={range?.through} placeholder="Last commit" options={options} onChange={through => chooseEndpoints(range?.from ?? through, through)} /></label>
      </>}
    </Space>
    {!!commits.length && history.complete && <>
      <p className="changes-hint">Select a commit. Shift-click or Shift+Arrow selects a range; From / Through also works on touch.</p>
      <div className="changes-commit-list" role="group" aria-label="Commits oldest first">
        {commits.slice((page - 1) * 50, page * 50).map((commit, offset) => {
          const i = (page - 1) * 50 + offset;
          return <button key={commit.hash} type="button" ref={el => { if (el) rows.current.set(i, el); else rows.current.delete(i); }}
            tabIndex={selection.focus === i ? 0 : -1} aria-pressed={!!range && i >= range.from && i <= range.through}
            onFocus={() => { if (selection.focus !== i) dispatch({ type: 'focus', position: i }); }}
            onClick={event => choose(i, event.shiftKey)}
            onKeyDown={event => {
              if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
              event.preventDefault();
              const direction = event.key === 'ArrowDown' ? 1 : -1;
              const next = Math.max(0, Math.min(commits.length - 1, i + direction));
              dispatch({ type: 'arrow', direction, shift: event.shiftKey });
              setPage(Math.floor(next / 50) + 1);
              requestAnimationFrame(() => rows.current.get(next)?.focus());
            }}>
            <span><strong>{i + 1}. {commit.subject}</strong><code title={commit.hash}>{commit.hash.slice(0, 10)}</code></span>
            <span>{commit.author.name} · <time dateTime={commit.author.time}>{new Date(commit.author.time).toLocaleString()}</time></span>
          </button>;
        })}
      </div>
      {commits.length > 50 && <Pagination current={page} total={commits.length} pageSize={50} showSizeChanger={false} onChange={value => { setPage(value); dispatch({ type: 'focus', position: (value - 1) * 50 }); }} />}
    </>}
    <p role="status">{range ? `Selected commits ${range.from + 1} through ${range.through + 1} · net changes` : 'All changes · base to head net changes'}</p>
    {range && <details className="changes-identity"><summary>Selected commit details</summary>
      {[...new Set([range.from, range.through])].map(i => <div key={commits[i].hash}>
        <strong>{i + 1}. {commits[i].subject}</strong><br />
        <code className="changes-hash">{commits[i].hash}</code><br />
        {commits[i].author.name} &lt;{commits[i].author.email}&gt; · <time dateTime={commits[i].author.time}>{new Date(commits[i].author.time).toLocaleString()}</time>
      </div>)}
    </details>}
    {error?.key === key ? <ReadError error={error.error} retry={() => setRetry(v => v + 1)} refresh={refresh} /> : canDiff && (!result || !data) ? <Spin aria-label="Loading changes diff" /> : result && data ? <Card>
      <div className="changes-comparison" aria-label="Actual comparison">Compare <code>{result.metadata.from_commit || result.metadata.from_tree}</code> → <code>{result.metadata.to_commit}</code></div>
      <Button href={reviewChangesService.downloadURL(projectId, jobId, history.selection, comparison)} download="changes.patch">Download patch</Button>
      {!result.patch && result.preview_complete ? <Alert type="info" message="No net file changes." />
        : <DocumentPreview key={key} name="changes.patch" data={data} dual={dual} setDual={setDual} />}
    </Card> : null}
  </>;
}
