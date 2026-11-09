import { StrictMode } from 'react';
import { renderToString } from 'react-dom/server';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import JobLeaseTimeline from './JobLeaseTimeline';
import { APIError, getJobLeaseEvents } from '@colony2/shared/api';
import { clearUserEmail, setUserEmail } from '@colony2/shared/auth';
import { leaseEvent, preciseTime } from '@colony2/shared/test/leaseEventFixtures';
import type { JobLeaseEventPage } from '@colony2/shared/types';

vi.mock('@colony2/shared/api', async original => ({ ...await original<typeof import('@colony2/shared/api')>(), getJobLeaseEvents: vi.fn() }));
const getEvents = vi.mocked(getJobLeaseEvents);
const createdAt = '2026-11-07T00:00:00.000000001Z';
const initial = { projectId: 'tenant', jobId: 'job', createdAt };
function mount(props = initial) { return render(<StrictMode><MemoryRouter><JobLeaseTimeline {...props} /></MemoryRouter></StrictMode>); }
function click(name = 'Timeline') { fireEvent.click(screen.getByRole('button', { name, exact: true })); }
function deferred() {
  let resolve!: (value: JobLeaseEventPage) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<JobLeaseEventPage>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
beforeEach(() => { getEvents.mockReset().mockResolvedValue({ events: [] }); clearUserEmail(); });
afterEach(() => { vi.useRealTimers(); });

it('SSR and inactive StrictMode rendering make zero requests; explicit opening makes exactly one', async () => {
  expect(renderToString(<MemoryRouter><JobLeaseTimeline {...initial} /></MemoryRouter>)).toContain('Timeline');
  mount();
  expect(getEvents).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: 'Timeline' })).toHaveAttribute('aria-expanded', 'false');
  click();
  await screen.findByText('No retained lease events are available.');
  expect(getEvents).toHaveBeenCalledTimes(1);
  expect(getEvents.mock.calls[0].slice(0,3)).toEqual(['tenant','job',{ pageSize: 100, pageToken: undefined }]);
  expect(screen.getByText('Job created')).toBeVisible();
  expect(screen.getByText(createdAt)).toBeVisible();
  expect(screen.getByText(/Collection and retention/)).toBeVisible();
});
it('retains opaque identities, timestamp ties, shortened renewal, historical snapshots and escaped text', async () => {
  getEvents.mockResolvedValue({ events: [leaseEvent(), leaseEvent('1', { eventType:'lease_acquired', acquisitionSource:'targeted', previousLeaseId:'old-lease' })] });
  const { container } = mount(); click();
  const rows = await screen.findAllByRole('listitem');
  expect(rows.map(row => within(row).getByRole('group').textContent)).toEqual(expect.arrayContaining([expect.stringContaining('900719925474099312345678901234567890'),expect.stringContaining('old-lease')]));
  expect(within(rows[0]).getByText('Lease renewed')).toBeVisible();
  expect(within(rows[1]).getByText('Lease acquired')).toBeVisible();
  expect(within(rows[0]).getByText(preciseTime)).toBeInTheDocument();
  for (const text of ['historical-cell','github.com/old/repo','primary-goal','original-type','2026-11-08T06:00:00.999999999Z','2026-11-08T05:01:00.987654321Z']) expect(within(rows[0]).getByText(text)).toBeInTheDocument();
  expect(container.querySelector('img,script')).toBeNull();
  expect(container.textContent).not.toMatch(/Lease released|Lease completed|Lease cancelled|Current owner/);
});
it('does not invent a recorded time for missing/invalid timestamps', async () => {
  mount({ ...initial, createdAt: 'not-a-date' }); click();
  await screen.findByText('No retained lease events are available.');
  expect(screen.getByText(/Recorded time unavailable/)).toBeVisible();
  expect(screen.queryByText('not-a-date')).toBeNull();
});
it('loads older only on action, blocks overlap, and deduplicates event IDs in unchanged server order', async () => {
  const next = deferred();
  getEvents.mockResolvedValueOnce({ events:[leaseEvent('9'),leaseEvent('2')],nextPageToken:'opaque+/=' }).mockReturnValueOnce(next.promise);
  mount(); click(); await screen.findAllByText('Lease renewed');
  expect(getEvents).toHaveBeenCalledTimes(1);
  click('Load older lease events'); click('Load older lease events');
  expect(getEvents).toHaveBeenCalledTimes(2);
  expect(getEvents.mock.calls[1][2]).toEqual({ pageSize:100, pageToken:'opaque+/=' });
  await act(async () => next.resolve({ events:[leaseEvent('2'),leaseEvent('1'),leaseEvent('1')]}));
  expect(screen.getAllByRole('listitem').map(row => within(row).getByText(/Historical details/).textContent)).toEqual(['Historical details · 9','Historical details · 2','Historical details · 1']);
  expect(screen.queryByRole('button',{name:'Load older lease events'})).toBeNull();
});
it('keeps rows after older-page failure, retries the same cursor and refreshes invalid cursors from page one', async () => {
  getEvents.mockResolvedValueOnce({events:[leaseEvent('first')],nextPageToken:'cursor'}).mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new APIError('stale',400,'invalid_lease_event_page_token')).mockResolvedValueOnce({events:[]});
  mount(); click(); await screen.findByText('Lease renewed'); click('Load older lease events');
  await screen.findByRole('alert');
  expect(screen.getAllByRole('listitem')).toHaveLength(1); expect(screen.getByText('Job created')).toBeVisible();
  click('Retry lease history'); await screen.findByRole('button',{name:'Refresh from first page'});
  expect(getEvents.mock.calls[2][2]?.pageToken).toBe('cursor');
  click('Refresh from first page');
  expect(screen.queryAllByRole('listitem')).toHaveLength(0);
  await screen.findByText('No retained lease events are available.');
  expect(getEvents.mock.calls[3][2]?.pageToken).toBeUndefined();
});
it.each([[403,'lease_permission_denied'],[401,'authentication_required'],[501,'unsupported_lease_events'],[502,'incompatible_response']])('keeps status %s %s as a local error and exposes retry', async (status,code) => {
  getEvents.mockRejectedValue(new APIError('denied or incompatible',Number(status),String(code)));
  mount(); click(); await screen.findByRole('alert');
  expect(screen.queryByText('No retained lease events are available.')).toBeNull();
  expect(screen.getByRole('button',{name:'Retry lease history'})).toBeEnabled();
  expect(screen.getByRole('button',{name:'Timeline'})).toBeEnabled();
});
it('refresh clears rows and cursor immediately, aborts older work, and ignores late responses from transports that ignore abort', async () => {
  const older=deferred(), fresh=deferred();
  getEvents.mockResolvedValueOnce({events:[leaseEvent('first')],nextPageToken:'old-cursor'}).mockReturnValueOnce(older.promise).mockReturnValueOnce(fresh.promise);
  mount(); click(); await screen.findByText('Lease renewed'); click('Load older lease events');
  const signal=getEvents.mock.calls[1][3]!;
  click('Refresh timeline'); expect(signal.aborted).toBe(true);
  expect(screen.queryAllByRole('listitem')).toHaveLength(0);
  expect(getEvents.mock.calls[2][2]?.pageToken).toBeUndefined();
  await act(async()=>older.resolve({events:[leaseEvent('stale')],nextPageToken:'stale-cursor'}));
  expect(screen.queryAllByRole('listitem')).toHaveLength(0);
  await act(async()=>fresh.resolve({events:[leaseEvent('fresh')]}));
  expect(screen.getByText('Historical details · fresh')).toBeInTheDocument();
  expect(screen.queryByText('Historical details · stale')).toBeNull();
});
it('close cancels supported requests, discards pages, and reopening revalidates without polling', async () => {
  const late=deferred(); getEvents.mockReturnValueOnce(late.promise).mockResolvedValue({events:[]});
  mount(); click(); const signal=getEvents.mock.calls[0][3]!; click();
  expect(signal.aborted).toBe(true);
  await act(async()=>late.resolve({events:[leaseEvent('stale')]}));
  expect(screen.queryByText('Job created')).toBeNull();
  click(); await screen.findByText('No retained lease events are available.');
  vi.useFakeTimers(); await act(async()=>vi.advanceTimersByTimeAsync(300_000));
  expect(getEvents).toHaveBeenCalledTimes(2);
});
it.each(['tenant','job','options','identity','navigation'])('clears local state and cancels on %s changes', async change => {
  const late=deferred(); getEvents.mockReturnValueOnce(late.promise).mockResolvedValue({events:[]});
  function Harness({projectId='tenant',jobId='job',pageSize=100}) {
    const navigate=useNavigate();
    return <><button onClick={()=>navigate('/other')}>Navigate</button><JobLeaseTimeline projectId={projectId} jobId={jobId} pageSize={pageSize} /></>;
  }
  const view=render(<StrictMode><MemoryRouter><Harness /></MemoryRouter></StrictMode>);
  click(); const signal=getEvents.mock.calls[0][3]!;
  if(change==='identity') act(()=>setUserEmail('changed@example.com'));
  else if(change==='navigation') click('Navigate');
  else view.rerender(<StrictMode><MemoryRouter><Harness projectId={change==='tenant'?'other':'tenant'} jobId={change==='job'?'other':'job'} pageSize={change==='options'?2:100} /></MemoryRouter></StrictMode>);
  expect(signal.aborted).toBe(true);
  expect(screen.getByRole('button',{name:'Timeline'})).toHaveAttribute('aria-expanded','false');
  await act(async()=>late.resolve({events:[leaseEvent('stale')],nextPageToken:'stale'}));
  expect(screen.queryByText('Historical details · stale')).toBeNull();
  click(); await waitFor(()=>expect(getEvents).toHaveBeenCalledTimes(2));
  expect(getEvents.mock.calls[1][2]?.pageToken).toBeUndefined();
  expect(getEvents.mock.calls[1][0]).toBe(change==='tenant'?'other':'tenant');
  expect(getEvents.mock.calls[1][1]).toBe(change==='job'?'other':'job');
  expect(getEvents.mock.calls[1][2]?.pageSize).toBe(change==='options'?2:100);
});
it('unmount aborts pending work', () => {
  getEvents.mockReturnValue(deferred().promise);
  const view=mount(); click(); const signal=getEvents.mock.calls[0][3]!;
  view.unmount(); expect(signal.aborted).toBe(true);
});
