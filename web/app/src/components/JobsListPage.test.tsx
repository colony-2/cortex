import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { JobInboxProvider } from '../hooks/useJobInbox';
import JobsListPage from './JobsListPage';
import { registryCell } from '../test/cellFixtures';
import { listJobs, listCells, getJob } from '@colony2/shared/api';
import { getUserEmail } from '@colony2/shared';
import { inboxReadKey } from '../utils/jobInboxData';
const activity = vi.hoisted(() => ({ pendingInputs: [{ id:'one',cell_id:'cell-platform',requested_at:'2026-11-21T12:00:00Z' }], discoveryStatus:'current', currentProjectId:'tenant', refresh: vi.fn() }));
vi.mock('@colony2/shared', async original => ({ ...await original<typeof import('@colony2/shared')>(), useInputActivity: () => activity }));
vi.mock('@colony2/shared/api', async original => ({ ...await original<typeof import('@colony2/shared/api')>(), listJobs:vi.fn(), listCells:vi.fn(), getJob:vi.fn() }));
const jobs = ['one','two'].map(job_id=>({tenant_id:'tenant',job_id,title:'Same title',prompt:'Same prompt',goal_id:'same',cell_id:'cell-platform',cell_name:'platform',repo:'github.com/acme/platform',status:'COMPLETED',store:'ARCHIVED'}));
beforeEach(() => {vi.mocked(listCells).mockResolvedValue({cells:[registryCell()]});localStorage.clear();vi.mocked(listJobs).mockImplementation(async(_,options)=>({jobs:options?.includeHidden?[...jobs,{...jobs[0],job_id:'hidden'}]:jobs} as any));});
function mount(path='/project/tenant/jobs') {render(<MemoryRouter initialEntries={[path]}><JobInboxProvider tenant="tenant"><JobsListPage projectId="tenant" /></JobInboxProvider></MemoryRouter>);}
it('renders individual job rows without goal grouping and opens exact Conversation links', async () => {
  mount(); const links=await screen.findAllByRole('link',{name:'platform: Same title'});
  expect(links).toHaveLength(2);
  expect(links[0]).toHaveAttribute('href','/project/tenant/jobs/one/story');
  expect(links[1]).toHaveAttribute('href','/project/tenant/jobs/two/story');
  expect(screen.getByText('Update unavailable')).toBeVisible();
  fireEvent.click(links[0]);
  expect(localStorage.getItem(inboxReadKey(getUserEmail() ?? '', 'tenant', 'one'))).not.toBeNull();
});
it('renders only explicitly hidden jobs in Archive and always as read', async () => {
  mount('/project/tenant/jobs?inbox=archive');
  const links=await screen.findAllByRole('link',{name:'platform: Same title'});expect(links).toHaveLength(1);
  expect(links[0]).toHaveAttribute('href','/project/tenant/jobs/hidden/story?inbox=archive');
  expect(document.querySelector('.inbox-job-row.unread')).toBeNull();
});
it('searches subsequent visible pages and leaves source pending counts alone', async () => {
  vi.mocked(listJobs).mockReset().mockResolvedValueOnce({ jobs: [jobs[0]] as any, next_page_token: 'next' })
    .mockResolvedValueOnce({ jobs: [{ ...jobs[1], title: 'Older match' }] as any });
  mount('/project/tenant/jobs?q=Older');
  expect(await screen.findByRole('link', { name: 'platform: Older match' })).toBeVisible();
  expect(screen.queryByRole('link',{name:'platform: Same title'})).toBeNull();
  expect(vi.mocked(listJobs).mock.calls.map(([, options]) => options?.pageToken)).toEqual([undefined, 'next']);
  expect(activity.pendingInputs).toHaveLength(1);
});
it('retains visible rows if refresh fails', async () => {
  mount();await screen.findAllByRole('link',{name:'platform: Same title'});
  vi.mocked(listJobs).mockRejectedValue(new Error('Offline'));
  fireEvent.click(screen.getByRole('button',{name:'Refresh inbox'}));
  await screen.findByText('Inbox refresh failed; showing the previous result');
  expect(screen.getAllByRole('link',{name:'platform: Same title'})).toHaveLength(2);
});
it('selects jobs independently of opening their stories and clears selection on folder changes', async () => {
  mount(); await screen.findAllByRole('link', { name: 'platform: Same title' });
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job two' }));
  expect(screen.getByText('1 selected')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Archive selected' })).toBeEnabled();
  expect(screen.getAllByLabelText('Unread')).toHaveLength(2);
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select all jobs' }));
  expect(screen.getByText('2 selected')).toBeVisible();
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select all jobs' }));
  expect(screen.queryByText('2 selected')).toBeNull();
  expect(screen.getByRole('checkbox', { name: 'Select job two' })).not.toBeChecked();
});

it('ENV-15–17 retains per-job requirement summaries and replaces removed execution values on refresh', async () => {
  let job: any = {...jobs[0],execution:{status:'specified',source:'yield',published:true,demand:{effective:{base:'linux',resources:{cpu:'4',memory:'8Gi'}}}}};
  vi.mocked(listJobs).mockImplementation(async()=>({jobs:[job]} as any));
  mount('/project/tenant/jobs?inbox=cell&cellId=cell-platform&q=Same');
  const summary=await screen.findByRole('link',{name:'req: linux 4c/8g'});
  const url=new URL(summary.getAttribute('href')!,'http://localhost');
  expect(url.pathname).toBe('/project/tenant/jobs/one/story');
  expect(url.searchParams.get('tab')).toBe('runtime');
  expect(summary.closest('a.inbox-job-link')).toBeNull();
  for(const status of ['in_flight','not_waiting']) {
    job={...job,execution:{status,source:'unavailable',published:false},recorded_execution:{status:'specified',source:'submission',published:false,demand:{effective:{base:status}}}};
    fireEvent.click(screen.getByRole('button',{name:'Refresh inbox'}));
    expect(await screen.findByRole('link',{name:`req: ${status}`})).toBeVisible();
    expect(screen.queryByRole('link',{name:'req: linux 4c/8g'})).toBeNull();
  }
  for(const status of ['unspecified','unresolved','malformed','unsupported','in_flight','not_waiting']) {
    job={...job,execution:{status,source:'absent',published:false,diagnostic:'invalid demand 雪'},recorded_execution:undefined};
    const before=vi.mocked(listJobs).mock.calls.length;
    fireEvent.click(screen.getByRole('button',{name:'Refresh inbox'}));
    await waitFor(()=>expect(vi.mocked(listJobs).mock.calls.length).toBeGreaterThan(before));
    await waitFor(()=>expect(document.querySelector('.job-requirements-summary')).toBeNull());
    expect(screen.queryByText('invalid demand 雪')).toBeNull();
  }
  expect(getJob).not.toHaveBeenCalled();
});
it('ENV-15,16 exposes each attempt requirement without goal grouping', async () => {
  vi.mocked(listJobs).mockResolvedValue({jobs:jobs.map((job,index)=>({...job,execution:{status:'specified',source:'submission',published:false,demand:{effective:{base:index?'new-base':'old-base'}}}}))} as any);
  mount();
  expect(await screen.findByRole('link',{name:'req: old-base'})).toHaveAttribute('href',expect.stringContaining('/jobs/one/story?'));
  expect(screen.getByRole('link',{name:'req: new-base'})).toHaveAttribute('href',expect.stringContaining('/jobs/two/story?'));
});
