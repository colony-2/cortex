import { listJobs } from '@colony2/shared/api';
import type { RecipeJobSummary } from '@colony2/shared/types';

export type InboxJob = RecipeJobSummary & { inboxArchived: boolean };
const inboxScanLimit = 5000;
const inboxPageSize = 200;

export async function loadVisibleInboxPage(tenant: string, cell: { id: string; name: string } | undefined, pageToken: string | undefined, signal: AbortSignal) {
  const page = await listJobs(tenant, { includeHidden: false, status: [], pageSize: inboxPageSize, pageToken,
    ...(cell && { cellNames: [cell.name] }),
  }, signal);
  if (page.jobs.some(job => job.tenant_id !== tenant)) throw new Error('Inbox response did not match this tenant.');
  return {
    jobs: page.jobs.filter(job => !cell || job.cell_id === cell.id).map(job => ({ ...job, inboxArchived: false })),
    nextPageToken: page.next_page_token,
  };
}

// V1 compatibility adapter: existing runtimes have no hidden-only projection.
// Bracket the inclusive read with visible reads; never subtract partial pages.
// This detects ordinary membership races, but is not an atomic runtime snapshot.
export async function loadArchivedInbox(tenant: string, signal?: AbortSignal): Promise<InboxJob[]> {
  async function scan(includeHidden: boolean) {
    const jobs = new Map<string, RecipeJobSummary>();
    const tokens = new Set<string>();
    let pageToken: string | undefined;
    let pages = 0;
    do {
      if (++pages > 50) throw new Error('Inbox has too many pages. Narrower runtime queries are required.');
      const page = await listJobs(tenant, { includeHidden, status: [], pageSize: inboxPageSize, pageToken }, signal);
      if (signal?.aborted) throw new Error('Inbox read superseded');
      for (const job of page.jobs) {
        if (job.tenant_id !== tenant || jobs.has(job.job_id)) throw new Error('Inbox changed while loading. Refresh to try again.');
        jobs.set(job.job_id, job);
      }
      if (jobs.size > inboxScanLimit) throw new Error(`This inbox exceeds the first-version limit of ${inboxScanLimit} jobs.`);
      pageToken = page.next_page_token;
      if (pageToken && tokens.has(pageToken)) throw new Error('Inbox cursor did not advance. Refresh to try again.');
      if (pageToken) tokens.add(pageToken);
    } while (pageToken);
    return jobs;
  }
  const before = await scan(false);
  const all = await scan(true);
  const visible = await scan(false);
  if (before.size !== visible.size || [...before.keys()].some(id => !visible.has(id))
    || [...visible.keys()].some(id => !all.has(id))) throw new Error('Inbox membership changed while loading. Refresh to try again.');
  return [...all.values()].filter(job => !visible.has(job.job_id)).map(job => ({ ...job, inboxArchived: true }));
}

export function inboxUpdate(job: RecipeJobSummary, requestedAt?: string): string | undefined {
  // These are persisted activity timestamps. Submission and observation time
  // cannot stand in for the missing execution-activity projection.
  return [job.archived_at, requestedAt].filter((value): value is string => !!value && Number.isFinite(Date.parse(value)) && !value.startsWith('0001-'))
    .sort((a, b) => Date.parse(b) - Date.parse(a))[0];
}

export function formatInboxUpdate(timestamp: string, today: Date): string {
  const date = new Date(timestamp);
  if (date.toDateString() === today.toDateString()) {
    return date.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
  }
  return date.toLocaleDateString(undefined, {
    month: 'short', day: 'numeric', year: date.getFullYear() === today.getFullYear() ? undefined : 'numeric',
  });
}
export function inboxRevision(job: RecipeJobSummary, requestedAt?: string, requestId?: string): string {
  return JSON.stringify([job.status, job.completion_status, job.cancel_requested, job.client_payload_revision?.toString(), inboxUpdate(job, requestedAt), requestId]);
}
export function inboxReadKey(user: string, tenant: string, job: string): string {
  return `cortex:inbox:seen:${JSON.stringify([user, tenant, job])}`;
}
