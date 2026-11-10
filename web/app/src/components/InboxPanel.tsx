import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Empty, List, Modal, Space, Spin, Typography } from 'antd';
import { inboxValueUrl } from '@colony2/shared/api';
import { API_BASE } from '@colony2/shared';
import type { InboxEntry, StoryNode } from '../utils/storyTree';
import ArtifactContents from './ArtifactContents';
import { resolveArtifactLink, type NavigationIntent } from './review/artifactLinks';

export function inboxCount(node: StoryNode) {
  return node.inbox?.status === 'available' && (node.inbox.entries === null || Array.isArray(node.inbox.entries))
    ? ` (${node.inbox.entries?.length || 0})` : '';
}
export function resolveInboxLink(reference: string, current: InboxEntry, entries: InboxEntry[]) {
  // Include unsupported destinations in ambiguity checks. Never alias by source name.
  const result = resolveArtifactLink(reference, current.path || '', entries.filter(e => !!e.path).map((e) => ({key: e.binding, path: e.path!})));
  if (result.kind === 'target' && (!current.path || !entries.some(e => e.binding === result.key && ['stored', 'value'].includes(e.content)))) return {kind:'unavailable', reason:'missing'} as const;
  return result;
}

export default function InboxPanel({ node, projectId, jobId, jobAttempt, retry, loading = false }: {
  node: StoryNode; projectId: string; jobId: string; jobAttempt: number; retry: () => void; loading?: boolean;
}) {
  const [selected, setSelected] = useState<InboxEntry>();
  const [intent, setIntent] = useState<NavigationIntent>();
  const [downloadError, setDownloadError] = useState('');
  const reads = useRef(new Set<AbortController>());
  useEffect(() => () => { reads.current.forEach(c => c.abort()); }, []);
  const opener = useRef<HTMLElement | null>(null);
  useEffect(() => { if (!selected) opener.current?.focus(); }, [selected]);
  const inbox = node.inbox;
  const entries = inbox?.entries || [];
  const url = (entry: InboxEntry) => {
    if (entry.content === 'stored' && entry.source_key) {
      const key = entry.source_key;
      return `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(key.jobId)}/tasks/${key.taskOrdinal}/artifacts/${encodeURIComponent(key.name)}`;
    }
    if (entry.content === 'value' && node.invocation_id && node.task_ordinal && node.attempt && jobAttempt) return inboxValueUrl(projectId, jobId, {
      jobAttempt, invocationId: node.invocation_id, taskOrdinal: node.task_ordinal, taskAttempt: node.attempt, binding: entry.binding,
    });
    return undefined;
  };
  const open = (entry: InboxEntry, fragment?: string) => {
    if (!url(entry)) return false;
    if (!selected) opener.current = document.activeElement as HTMLElement;
    setSelected(entry); setIntent({token: Date.now(), fragment}); return true;
  };
  const download = async (entry: InboxEntry) => {
    const href = url(entry); if (!href) return;
    const controller = new AbortController(); reads.current.add(controller); setDownloadError('');
    try {
      const response = await fetch(href, {signal:controller.signal});
      if (!response.ok) throw new Error('Could not download recorded inbox content. Retry the download.');
      const blob = await response.blob();
      if (controller.signal.aborted) return;
      const objectUrl = URL.createObjectURL(blob), a = document.createElement('a');
      a.href = objectUrl; a.download = (entry.path || entry.binding).split('/').pop() || 'inbox'; a.click();
      setTimeout(() => URL.revokeObjectURL(objectUrl), 0);
    } catch (error) { if (!controller.signal.aborted) setDownloadError((error as Error).message); }
    finally { reads.current.delete(controller); }
  };
  const containers = ['recipe', 'sequence', 'op', 'stateMachine', 'state'];
  const unavailable = inbox?.reason || (containers.includes(node.kind) ? 'Inbox does not apply to this node' : 'Recorded inbox metadata unavailable');
  if (loading) return <Spin aria-label="Loading recorded inbox metadata" />;
  return <Space direction="vertical" style={{width:'100%'}}>
    <Typography.Text type="secondary">Recorded inputs; delivery not verified.</Typography.Text>
    {inbox?.status !== 'available' || (inbox.entries !== null && !Array.isArray(inbox.entries)) ? <Alert type="info" message={inbox?.status === 'not_applicable' || (!inbox && containers.includes(node.kind)) ? 'Inbox does not apply to this node' : unavailable} action={<Button onClick={retry}>Retry</Button>} />
      : !entries.length ? <Empty description="No recorded inbox artifacts for this step." />
      : <List dataSource={entries} renderItem={entry => <List.Item actions={url(entry) ? [<Button key="download" onClick={() => download(entry)} aria-label={`Download ${entry.binding}`}>Download</Button>] : []}>
        <List.Item.Meta title={url(entry) ? <Button type="link" onClick={() => open(entry)}>{entry.path || entry.binding}</Button> : <Typography.Text strong>{entry.path || entry.binding}</Typography.Text>} description={<Space direction="vertical" size={0}>
          <span>Binding: {entry.binding}</span>
          <span>Source: {entry.source_kind}{entry.source_key ? ` · job ${entry.source_key.jobId} · task ${entry.source_key.taskOrdinal} · ${entry.source_key.name}${entry.source_key.sizeBytes !== undefined ? ` · ${entry.source_key.sizeBytes} bytes` : ''}` : ''}</span>
          {entry.content === 'unsupported' && <span>{entry.reason || 'Content unavailable'}.{entry.source_kind === 'external' ? ' External content and archive member inventories are unavailable.' : ''}</span>}
          {entry.content === 'value' && !url(entry) && <span>Recorded attempt selectors unavailable.</span>}
        </Space>} />
      </List.Item>} />}
    {downloadError && <Alert type="error" message={downloadError} />}
    {selected && <Modal open className="story-artifact-modal" title={selected.path || selected.binding} onCancel={() => setSelected(undefined)} width={1000} footer={<><Button onClick={() => download(selected)}>Download</Button><Button onClick={() => setSelected(undefined)}>Close</Button></>}>
      <ArtifactContents key={selected.binding} url={url(selected)!} name={selected.path || selected.binding} navigationIntent={intent}
        navigation={{resolve: reference => resolveInboxLink(reference, selected, entries), open: (binding, fragment) => { const entry = entries.find(e => e.binding === binding); return entry ? open(entry, fragment) : false; }}}
        resolveAttachment={reference => { const result = resolveInboxLink(reference, selected, entries); const entry = result.kind === 'target' ? entries.find(e => e.binding === result.key) : undefined; const href = entry && url(entry); return entry && href ? {key:entry.binding,name:entry.path || entry.binding,url:href} : undefined; }} />
    </Modal>}
  </Space>;
}
