import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import RunnersPage from './RunnersPage';

const root = '/project/t/runners';
const instance = {
  launch_id: 'live',
  id: 'i-1',
  provider: 'mac',
  state: 'running',
  job: { job_id: 'job', tenant_id: 't', instance_id: 'target', jobdb: 'http://db/t' },
  refs: [],
  started_at: '2026-01-02T00:00:00Z',
  created_at: '2026-01-01T00:00:00Z',
};
const mac = {
  name: 'mac',
  type: 'reverse',
  discovery: 'known',
  bases_complete: false,
  bases: [
    { name: 'darwin', capabilities: ['gpu'] },
    { name: 'darwin-minimal', capabilities: [] },
  ],
  health: {
    name: 'mac',
    state: 'healthy',
    implementation: 'mac-runner',
    version: '1.4.2',
    protocol: 1,
    generation: 9,
    restart_count: 0,
    last_failure: 'Session interrupted',
  },
};
function setup(path: string, handler: (p: string, q: URLSearchParams) => unknown) {
  const fetch = vi.fn(async (url: string) => {
    const u = new URL(url, 'http://local');
    const result = handler(u.pathname.replace('/api', ''), u.searchParams);
    return result instanceof Response ? result : Response.json(result);
  });
  vi.stubGlobal('fetch', fetch);
  render(
    <MemoryRouter initialEntries={[root + path]}>
      <Routes>
        <Route path="/project/:projectId/runners/*" element={<RunnersPage />} />
      </Routes>
    </MemoryRouter>,
  );
  return fetch;
}
afterEach(() => vi.unstubAllGlobals());
it('shows live catalogs and health even when the instance inventory is partial', async () => {
  const requests = setup('', (p) => {
    if (p.endsWith('/providers'))
      return {
        items: [
          mac,
          {
            name: 'offline',
            type: 'reverse',
            discovery: 'unavailable',
            bases: [],
            bases_complete: false,
            health: { ...mac.health, name: 'offline', state: 'unavailable' },
          },
          { name: 'idle', type: 'docker', discovery: 'known', bases: [], bases_complete: true },
        ],
      };
    if (p.endsWith('/status'))
      return { version: '1', status: 'ok', providers: 3, poll: { last_finished: '2026-01-01T01:00:00Z' } };
    return Response.json(
      { complete: false, items: [instance], errors: [{ provider: 'offline', error: 'timeout' }] },
      { status: 502 },
    );
  });
  const card = (await screen.findByRole('link', { name: 'mac' })).closest('article')!;
  expect(card).toHaveTextContent('healthy');
  expect(card).toHaveTextContent('Partial catalog');
  expect(within(card).getByText('darwin').closest('tr')).toHaveTextContent('gpu');
  expect(within(card).getByText('darwin-minimal').closest('tr')).toHaveTextContent(
    'No optional capabilities',
  );
  expect(screen.getByRole('link', { name: 'offline' }).closest('article')).toHaveTextContent(
    'Provider unavailable for catalog discovery.',
  );
  const idle = screen.getByRole('link', { name: 'idle' }).closest('article')!;
  expect(idle).toHaveTextContent('Complete catalog');
  expect(idle).toHaveTextContent('No bases advertised.');
  expect(idle).toHaveTextContent('Health unreported');
  expect(requests).toHaveBeenCalledTimes(3);
  expect(screen.getByRole('alert')).toHaveTextContent('offline');
  expect(screen.getByRole('link', { name: 'idle' }).closest('article')).toHaveTextContent(
    'Active instances0',
  );
});
it('keeps configured providers visible on older Pulse controllers', async () => {
  setup('', (p) => {
    if (p.endsWith('/providers')) return new Response(null, { status: 404 });
    if (p.endsWith('/config')) return { providers: { old: { bases: ['legacy-base'] } } };
    if (p.endsWith('/health')) return { items: [{ name: 'old', state: 'ready' }] };
    if (p.endsWith('/status')) return { status: 'ok', providers: 1, poll: {} };
    return { items: [], errors: [], complete: true };
  });
  const card = (await screen.findByRole('link', { name: 'old' })).closest('article')!;
  expect(card).toHaveTextContent('From provider configuration');
  expect(card).toHaveTextContent('legacy-base');
  expect(card).toHaveTextContent('ready');
});
it('loads retained launches for the job and deduplicates repeated acquisitions', async () => {
  const calls = setup('/instances?jobId=job&status=all', (p, q) => {
    if (p.endsWith('/runner-launches')) {
      expect(q.get('jobId')).toBe('job');
      const event = { workerId: 'past', jobKey: { tenantId: 't', jobId: 'job' }, occurredAt: '2026-01-01' };
      return { events: [event, event] };
    }
    return {
      items: [instance, { ...instance, launch_id: 'other', job: { ...instance.job, tenant_id: 'other' } }],
      complete: true,
      errors: [],
    };
  });
  await screen.findByRole('link', { name: 'past' });
  expect(screen.getAllByRole('link', { name: 'past' })).toHaveLength(1);
  expect(screen.queryByRole('link', { name: 'other' })).toBeNull();
  expect(screen.getByRole('link', { name: 'live' })).toHaveAttribute(
    'href',
    root + '/launches/live?provider=mac',
  );
  expect(calls.mock.calls.length).toBe(2);
});
it('discovers a retained log across configured providers and limits the initial preview', async () => {
  const logPage = {
    records: Array.from({ length: 3 }, (_, i) => ({ sequence: i + 1, data: btoa('x'.repeat(4096)) })),
    next_page_token: 'p.end',
    has_more: false,
    finalized: true,
    incomplete: false,
  };
  const requests = setup('/launches/past?jobId=job', (p, q) => {
    if (p.endsWith('/config')) return { providers: { first: {}, second: {} } };
    if (p.endsWith('/instances')) return { items: [], complete: true, errors: [] };
    if (p.endsWith('/runner-launches')) {
      expect(q.get('launchId')).toBe('past');
      return {
        events: [
          {
            eventId: '1',
            jobKey: { tenantId: 't', jobId: 'job' },
            cell: 'alpha',
            route: { jobType: 'recipe' },
            leaseId: 'recorded-lease',
          },
        ],
      };
    }
    if (p.includes('/first/')) return Response.json({ error: 'expired' }, { status: 404 });
    expect(p).toBe('/pulse/providers/second/launches/past/logs');
    return q.get('page_size') === '1' ? { ...logPage, records: logPage.records.slice(0, 1) } : logPage;
  });
  const preview = await screen.findByText(/^x{10240}$/);
  expect(screen.getByRole('table', { name: 'Recorded launch leases' })).toHaveTextContent('recorded-lease');
  expect(screen.getByRole('link', { name: 'Download log' })).toHaveAttribute(
    'href',
    expect.stringContaining('/second/launches/past/download'),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Show more log' }));
  expect(preview.textContent).toHaveLength(12288);
  expect(requests.mock.calls.filter(([u]) => u.includes('/logs'))).toHaveLength(3);
});
it('continues an empty provider page using its opaque cursor', async () => {
  setup('/providers/mac', (p, q) => {
    if (p.endsWith('/config')) return { providers: { mac: { type: 'reverse' } } };
    if (p.endsWith('/mac/capabilities')) return mac;
    return q.get('page_token') === 'p.+/=' ? { items: [instance] } : { items: [], next_page_token: 'p.+/=' };
  });
  expect(await screen.findByText('mac-runner')).toBeVisible();
  expect(screen.getByText('generation').closest('tr')).toHaveTextContent('9');
  expect(screen.getByText('restart count').closest('tr')).toHaveTextContent('0');
  expect(screen.getByText('Session interrupted')).toBeVisible();
  expect(screen.getByText('darwin').closest('tr')).toHaveTextContent('gpu');
  fireEvent.click(await screen.findByRole('button', { name: 'Load more instances' }));
  expect(
    await within(screen.getByRole('table', { name: 'Runner instances' })).findByRole('link', {
      name: 'live',
    }),
  ).toBeVisible();
});

it('discovers a recorded launch log without reading job history', async () => {
  const pages = {
    records: [{ data: btoa('retained output') }],
    next_page_token: 'end',
    has_more: false,
    finalized: true,
    incomplete: false,
  };
  const fetch = setup('/launches/past/logs?jobId=job', (path) => {
    if (path.endsWith('/instances')) return { items: [], complete: true, errors: [] };
    if (path.endsWith('/config')) return { providers: { mac: {} } };
    if (path.endsWith('/mac/launches/past/logs')) return pages;
    throw Error('Unexpected request ' + path);
  });
  expect(await screen.findByText('retained output')).toBeVisible();
  expect(fetch.mock.calls.map(([url]) => url)).not.toEqual(
    expect.arrayContaining([expect.stringContaining('runner-launches')]),
  );
});

it('opens a known provider log without scanning instances', async () => {
  const requests = setup('/launches/live/logs?provider=mac', (path) => {
    expect(path).toBe('/pulse/providers/mac/launches/live/logs');
    return {
      records: [{ data: btoa('live output') }],
      next_page_token: 'end',
      has_more: false,
      finalized: true,
      incomplete: false,
    };
  });
  expect(await screen.findByText('live output')).toBeVisible();
  expect(requests).toHaveBeenCalledTimes(1);
});
