// Browser inbox presentation. Pending counts come from tenant discovery.
export type InboxJobIdentity = { tenantId: string; jobId: string };
export type InboxMembership = { archived: boolean; cellId: string; waiting?: boolean };
export type InboxSelection = { kind: 'all' | 'waiting' | 'drafts' } | { kind: 'archive' } | { kind: 'cell'; cellId: string };

export const inboxSortOptions = [
  { key: 'newest', label: 'Date (newest first)' },
  { key: 'oldest', label: 'Date (oldest first)' },
  { key: 'cell', label: 'Cell' },
  { key: 'status', label: 'Status' },
  { key: 'subject', label: 'Subject' },
];
export const readInboxSort = (value: string | null) => inboxSortOptions.find(option => option.key === value)?.key;

export function inboxJobKey({ tenantId, jobId }: InboxJobIdentity): string {
  return JSON.stringify([tenantId, jobId]);
}

export function isInInbox(job: InboxMembership, selection: InboxSelection): boolean {
  if (selection.kind === 'archive') return job.archived;
  if (job.archived || selection.kind === 'drafts') return false;
  if (selection.kind === 'waiting') return !!job.waiting;
  return selection.kind !== 'cell' || job.cellId === selection.cellId;
}

export type RankedInboxCell = { id: string; name: string; unarchivedJobs: number };
const compareText = (a: string, b: string) => a < b ? -1 : a > b ? 1 : 0;

export function rankInboxCells<T extends RankedInboxCell>(cells: readonly T[]): T[] {
  const seen = new Set<string>();
  for (const cell of cells) {
    if (!cell.id.trim() || !cell.name.trim() || seen.has(cell.id)
      || !Number.isSafeInteger(cell.unarchivedJobs) || cell.unarchivedJobs < 0) {
      throw new Error('Cell totals are unavailable');
    }
    seen.add(cell.id);
  }
  return [...cells].sort((a, b) => b.unarchivedJobs - a.unarchivedJobs
    || compareText(a.name, b.name) || compareText(a.id, b.id));
}

// The selected cell remains reachable even when it ranks below the default
// eight. Expansion/search belong to navigation and must not change this order.
export function visibleInboxCells<T extends RankedInboxCell>(ranked: readonly T[], selectedId?: string): T[] {
  const visible = ranked.slice(0, 8);
  const selected = ranked.find(cell => cell.id === selectedId);
  if (selected && !visible.some(cell => cell.id === selected.id)) visible.push(selected);
  return visible;
}

type PendingJob = { id: string; cell_id: string };
export type InboxPendingSnapshot = {
  tenantId: string;
  status: 'loading' | 'current' | 'unavailable';
  inputs: readonly PendingJob[];
};
export type InboxPendingCounts =
  | { status: 'loading' | 'unavailable' }
  | { status: 'current'; total: number; byCell: ReadonlyMap<string, number> };

// Use pending-input discovery, never scheduling state, unread flags, or the
// current list page. The existing discovery endpoint excludes hidden jobs.
export function inboxPendingCounts(tenantId: string, snapshot: InboxPendingSnapshot): InboxPendingCounts {
  if (snapshot.tenantId !== tenantId) return { status: 'loading' };
  if (snapshot.status !== 'current') return { status: snapshot.status };
  const seen = new Set<string>();
  const byCell = new Map<string, number>();
  for (const input of snapshot.inputs) {
    if (!input.id.trim() || !input.cell_id.trim() || seen.has(input.id)) return { status: 'unavailable' };
    seen.add(input.id);
    byCell.set(input.cell_id, (byCell.get(input.cell_id) ?? 0) + 1);
  }
  return { status: 'current', total: seen.size, byCell };
}

// Undefined means there is no badge. Unknown data remains explicit rather than
// looking like confirmed zero. Archive never shows a pending badge.
export function inboxPendingBadge(counts: InboxPendingCounts, selection: InboxSelection): number | '?' | undefined {
  if (selection.kind === 'archive' || selection.kind === 'drafts') return undefined;
  if (counts.status !== 'current') return '?';
  const count = selection.kind === 'cell' ? counts.byCell.get(selection.cellId) ?? 0 : counts.total;
  return count || undefined;
}

export function composeJobPrompt(subject: string, body: string): string {
  if (!body.trim()) throw new Error('Write a message for this cell');
  const title = subject.trim();
  return title ? `${title}\n\n${body}` : body;
}

export function messagePreview(text?: string): string {
  const line = text?.trim().replace(/\s+/g, ' ') ?? '';
  return line.length > 120 ? line.slice(0, 117).trimEnd() + '…' : line;
}

export function threadTitle(job: { title?: string; prompt?: string }): string {
  return job.title?.trim() || messagePreview(job.prompt) || 'Untitled thread';
}
