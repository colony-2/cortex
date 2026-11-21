// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest';
import { API_BASE } from '../apiBase';
vi.mock('../apiBase', () => ({ API_BASE: '/api' }));
import { ChangesReadError, reviewChangesService, type WorkspaceDiffPreview } from './reviewChangesService';
// Also asserted against the actual Go handler in TestWorkspaceDiffHTTPPathEncoding.
import pathFixtures from '../../../../server/internal/cortex/testdata/workspace-diff-paths.json';

function preview(): WorkspaceDiffPreview {
  return { metadata: { version: 1, status: 'available', requested: { job_id: 'j' }, resolved: { status: 'persisted', boundary: 'after', workspace: 'main' }, observation: 'obs', cutoff: 'cutoff', workspace: { repository: 'example/repo', scope_id: 'main', base_commit: '1'.repeat(40), head_commit: '2'.repeat(40) }, state_producer: { job_id: 'j', task: 'state', attempt: 1 }, source: { producer: { job_id: 'producer', task: 'source', attempt: 1 }, key: { jobId: 'producer', taskOrdinal: 7, name: 'diff_from_base.diff', sizeBytes: 5 } }, bytes: 5 }, patch: 'patch', preview_complete: true, download_url: `${API_BASE}/projects/t/jobs/producer/tasks/7/artifacts/diff_from_base.diff` };
}
const signal = () => new AbortController().signal;
afterEach(() => vi.unstubAllGlobals());
function serve(p: unknown) { const fetcher = vi.fn().mockImplementation(async () => new Response(JSON.stringify(p))); vi.stubGlobal('fetch', fetcher); return fetcher; }
it.each(pathFixtures)('accepts the Go handler download for tenant $tenant', async ({ tenant, producer, download_url }) => {
  const p = preview();
  p.metadata.source!.producer.job_id = producer;
  p.metadata.source!.key.jobId = producer;
  p.download_url = download_url;
  serve(p);
  await expect(reviewChangesService.workspaceDiff(tenant, 'j', {}, signal())).resolves.toEqual(p);
});
it.each([
  '/api/projects/t+plus/jobs/producer%2Fa/tasks/7/artifacts/diff_from_base.diff',
  '/api/projects/t%2bplus/jobs/producer%2fa/tasks/7/artifacts/diff_from_base.diff',
])('accepts equivalent segment escapes in %s', async download_url => {
  const p = preview();
  p.metadata.source!.producer.job_id = 'producer/a'; p.metadata.source!.key.jobId = 'producer/a';
  p.download_url = download_url;
  serve(p);
  await expect(reviewChangesService.workspaceDiff('t+plus', 'j', {}, signal())).resolves.toEqual(p);
});
it.each([
  '/api/projects/t%20plus/jobs/producer%2Fa/tasks/7/artifacts/diff_from_base.diff',
  '/api/projects/t%252Bplus/jobs/producer%2Fa/tasks/7/artifacts/diff_from_base.diff',
  '/api/projects/t+plus/jobs/producer/a/tasks/7/artifacts/diff_from_base.diff',
  '/api/projects/t+plus/jobs/producer%252Fa/tasks/7/artifacts/diff_from_base.diff',
  '/api/projects/t+plus/jobs/producer%ZZa/tasks/7/artifacts/diff_from_base.diff',
  '/api/projects/t+plus/jobs/producer%FFa/tasks/7/artifacts/diff_from_base.diff',
  '/api/projects/t+plus/jobs/producer%2Fa/tasks/7/artifacts/diff_from_base.diff?',
  '/api/projects/t+plus/jobs/producer%2Fa/tasks/7/artifacts/diff_from_base.diff#',
  `http://user:pass@${window.location.host}/api/projects/t+plus/jobs/producer%2Fa/tasks/7/artifacts/diff_from_base.diff`,
  'https://evil.test/api/projects/t+plus/jobs/producer%2Fa/tasks/7/artifacts/diff_from_base.diff',
])('rejects a different source, malformed escape or URL suffix in %s', async download_url => {
  const p = preview();
  p.metadata.source!.producer.job_id = 'producer/a'; p.metadata.source!.key.jobId = 'producer/a';
  p.download_url = download_url;
  serve(p);
  await expect(reviewChangesService.workspaceDiff('t+plus', 'j', {}, signal())).rejects.toMatchObject({ diagnostic: { code: 'protocol' } });
});
it('encodes opaque point selectors and forwards abort without adding omitted defaults', async () => {
  const p = preview();
  const selectors = { scope: '/scope:"x/μ"#2', workspace: 'w/+?', attempt: 2, boundary: 'before' as const };
  p.metadata.requested = { job_id: 'j?b', ...selectors }; p.metadata.resolved = { ...p.metadata.resolved, ...selectors };
  p.metadata.state_producer!.job_id = 'j?b'; p.metadata.workspace!.scope_id = selectors.workspace;
  p.download_url = `${API_BASE}/projects/t%2Fa/jobs/producer/tasks/7/artifacts/diff_from_base.diff`;
  const fetcher = serve(p); const abort = signal();
  await reviewChangesService.workspaceDiff('t/a', 'j?b', selectors, abort);
  const url = new URL(fetcher.mock.calls[0][0], window.location.origin);
  expect(url.pathname).toBe('/api/projects/t%2Fa/jobs/j%3Fb/workspace-diff');
  expect(Object.fromEntries(url.searchParams)).toEqual({ ...selectors, attempt: '2' });
  expect(fetcher.mock.calls[0][1].signal).toBe(abort);
  serve(preview()); await reviewChangesService.workspaceDiff('t', 'j', {}, abort);
});
it.each(['empty', 'unavailable'])('preserves explicit %s without inventing a download', async status => {
  const p = preview(); delete p.metadata.source; delete p.download_url; p.patch = '';
  if (status === 'empty') { p.metadata.status = 'empty'; p.metadata.bytes = 0; p.metadata.workspace!.head_commit = p.metadata.workspace!.base_commit; }
  else { p.metadata.status = 'unavailable'; delete p.metadata.bytes; p.metadata.reasons = ['missing recorded evidence']; }
  serve(p); expect((await reviewChangesService.workspaceDiff('t', 'j', {}, signal())).metadata.status).toBe(status);
});
it.each([
  (p: any) => { p.metadata.version = 2; },
  (p: any) => { p.metadata.status = 'unknown'; },
  (p: any) => { p.metadata.requested.job_id = 'another'; },
  (p: any) => { p.metadata.resolved.boundary = 'before'; },
  (p: any) => { p.metadata.bytes = -1; },
  (p: any) => { p.metadata.source.key.sizeBytes = 6; },
  (p: any) => { p.metadata.source.producer.job_id = 'other'; },
  (p: any) => { p.metadata.workspace = null; },
  (p: any) => { p.download_url = 'https://evil.test/api/projects/t/jobs/producer/tasks/7/artifacts/diff_from_base.diff'; },
  (p: any) => { p.download_url += '?redirect=evil'; },
  (p: any) => { p.download_url = '/api/projects/other/jobs/producer/tasks/7/artifacts/diff_from_base.diff'; },
  (p: any) => { p.download_url = '/api/projects/t/jobs/other/tasks/7/artifacts/diff_from_base.diff'; },
  (p: any) => { p.metadata.status = 'empty'; },
  (p: any) => { p.metadata.status = 'unavailable'; p.metadata.reasons = ['missing']; },
])('rejects malformed status, identity and download responses %#', async change => {
  const p = preview(); change(p); serve(p);
  await expect(reviewChangesService.workspaceDiff('t', 'j', {}, signal())).rejects.toMatchObject({ diagnostic: { code: 'protocol' } });
});
it('preserves typed upgrade errors and cancellation', async () => {
  const diagnostic = { version: 1, code: 'unsupported', detail: 'Upgrade helper' };
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'Upgrade helper', diagnostic }), { status: 501 })));
  await expect(reviewChangesService.workspaceDiff('t', 'j', {}, signal())).rejects.toMatchObject({ message: 'Upgrade helper', diagnostic });
  const error = new DOMException('Aborted', 'AbortError'); vi.stubGlobal('fetch', vi.fn().mockRejectedValue(error));
  await expect(reviewChangesService.workspaceDiff('t', 'j', {}, signal())).rejects.toBe(error);
  expect(new ChangesReadError('failure', 502)).toBeInstanceOf(Error);
});
