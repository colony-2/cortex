import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { listJobChildren, listJobs } from '@colony2/shared/api';
import type { RecipeJob, ListRecipeJobsResponse } from '@colony2/shared/types';
import JobRelationships from './JobRelationships';

vi.mock('@colony2/shared/api', () => ({ listJobs: vi.fn(), listJobChildren: vi.fn() }));
const api = vi.mocked(listJobChildren);
const job = { tenant_id: 'tenant', job_id: 'current', goal_id: 'current-goal' } as RecipeJob;
const child = (id: string, extra = {}) => ({ ...job, job_id: id, goal_id: 'shared', cell_name: 'platform', status: 'READY', store: 'ACTIVE', ...extra } as RecipeJob);
function mount(current = job, tenant = 'tenant', id = 'current') {
  const result = render(<MemoryRouter><JobRelationships projectId={tenant} jobId={id} job={current} /></MemoryRouter>);
  return result;
}
beforeEach(() => { api.mockReset().mockResolvedValue({ jobs: [] }); vi.mocked(listJobs).mockReset().mockResolvedValue({jobs:[]}); });
afterEach(() => vi.clearAllMocks());

it.each([
  [{ job_id: 'p /?', goal_id: 'g /?', tenant_id: 'other /?' }, 'other /?'],
  [{ job_id: 'p /?', goal_id: 'g /?' }, 'tenant'],
])('REL-002,009 parent links use encoded parent identity without inspection %#', async (parent, tenant) => {
  mount({ ...job, parent });
  expect(screen.getByRole('link', { name: 'Job · p /?' })).toHaveAttribute('href', `/project/${encodeURIComponent(tenant)}/jobs/p%20%2F%3F/story`);
  expect(screen.getByRole('link', { name: 'Goal · g /?' })).toHaveAttribute('href', `/project/${encodeURIComponent(tenant)}/goals/g%20%2F%3F`);
  await screen.findByText('No child jobs recorded in this tenant');
  expect(api).toHaveBeenCalledTimes(1);
});
it.each([undefined, {}, { job_id: 'p' }, { goal_id: 'g' }])('REL-003,004 partial or absent parent %#', async parent => {
  mount({ ...job, parent });
  expect(screen.queryByRole('link', { name: 'Job · p' }) !== null).toBe(!!parent?.job_id);
  expect(screen.queryByRole('link', { name: 'Goal · g' }) !== null).toBe(!!parent?.goal_id);
  expect(screen.queryByText('No parent recorded') !== null).toBe(!parent?.job_id && !parent?.goal_id);
  expect(screen.queryByRole('link', { name: /current-goal/ })).toBeNull();
  await screen.findByText('No child jobs recorded in this tenant');
});
it('REL-005,006,007,008,017 renders distinct child identities, statuses, shared goals and absent goals', async () => {
  api.mockResolvedValue({ jobs: [child('one', { title: 'First', goal_id: 'current-goal' }), child('two'), child('three'), child('four', { goal_id: undefined, status: 'COMPLETED', completion_status: 'success', store: 'ARCHIVED' })] });
  mount();
  await screen.findByRole('link', { name: 'Job · First' });
  expect(screen.getByText('one · platform')).toBeVisible();
  expect(screen.getByRole('link', { name: 'Job · two' })).toHaveAttribute('href','/project/tenant/jobs/two/story');
  expect(screen.getAllByRole('link', { name: 'Goal · shared' })).toHaveLength(2);
  expect(screen.getByRole('link', { name: 'Goal · current-goal' })).toBeVisible();
  expect(screen.getByText('No goal recorded')).toBeVisible();
  expect(screen.getByText('Succeeded')).toBeVisible();
  expect(screen.getAllByText('Ready')).toHaveLength(3);
  expect(screen.getByText('Child jobs in this tenant')).toBeVisible();
});
it('REL-023,024,025 handles bounded paging, duplicates and empty intermediate pages', async () => {
  let finish!: (page: ListRecipeJobsResponse) => void;
  api.mockResolvedValueOnce({ jobs: [child('one'), child('two')], next_page_token: 'next+/=' })
    .mockImplementationOnce(() => new Promise(done => { finish = done; }))
    .mockResolvedValueOnce({ jobs: [], next_page_token: 'last' })
    .mockResolvedValueOnce({ jobs: [] });
  mount();
  const more = await screen.findByRole('button', { name: 'Load more child jobs' });
  expect(api).toHaveBeenCalledTimes(1);
  expect(api).toHaveBeenLastCalledWith('tenant','current',{pageSize:20,pageToken:undefined});
  fireEvent.click(more); expect(more).toBeDisabled();
  await act(async () => finish({jobs:[child('two'),child('three')],next_page_token:'empty'}));
  const rows = within(screen.getByRole('list')).getAllByRole('listitem');
  expect(rows.map(row => within(row).getByRole('link', {name:/Job ·/}).textContent)).toEqual(['one','two','three']);
  expect(screen.getByText('3 child jobs shown · More available')).toBeVisible();
  fireEvent.click(more);
  await waitFor(() => expect(api).toHaveBeenCalledTimes(3));
  await waitFor(() => expect(more).not.toBeDisabled());
  expect(screen.queryByText('No child jobs recorded in this tenant')).toBeNull();
  fireEvent.click(more);
  await waitFor(() => expect(screen.queryByRole('button',{name:'Load more child jobs'})).toBeNull());
  expect(api.mock.calls.map(call=>call[2]?.pageToken)).toEqual([undefined,'next+/=','empty','last']);
});
it('REL-025 retains Load more for an empty first page with a token', async () => {
  api.mockResolvedValueOnce({ jobs: [], next_page_token: 'next' }).mockResolvedValueOnce({jobs:[]});
  mount(); fireEvent.click(await screen.findByRole('button',{name:'Load more child jobs'}));
  await screen.findByText('No child jobs recorded in this tenant');
});
it('REL-026,027,028 retains parents and rows through failures and resets on refresh', async () => {
  api.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({jobs:[child('one')],next_page_token:'next'})
    .mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({jobs:[child('two')]}).mockResolvedValueOnce({jobs:[]});
  mount({...job,parent:{job_id:'parent'}});
  expect(screen.getByRole('link',{name:'Job · parent'})).toBeVisible();
  expect(screen.getByRole('status')).toHaveTextContent('Loading child jobs');
  fireEvent.click(await screen.findByRole('button',{name:'Retry'}));
  fireEvent.click(await screen.findByRole('button',{name:'Load more child jobs'}));
  fireEvent.click(await screen.findByRole('button',{name:'Retry'}));
  await screen.findByRole('link',{name:'Job · two'});
  expect(screen.getByRole('link',{name:'Job · one'})).toBeVisible();
  expect(api.mock.calls.map(call=>call[2]?.pageToken)).toEqual([undefined,undefined,'next','next']);
  fireEvent.click(screen.getByRole('button',{name:'Refresh relationships'}));
  expect(screen.queryByRole('link',{name:'Job · one'})).toBeNull();
  await screen.findByText('No child jobs recorded in this tenant');
  expect(api.mock.calls.at(-1)?.[2]?.pageToken).toBeUndefined();
  expect(api).toHaveBeenCalledTimes(5);
});
it('REL-029 clears identity and ignores obsolete responses', async () => {
  let finish!: (page: ListRecipeJobsResponse) => void;
  api.mockImplementationOnce(() => new Promise(done => {finish=done;})).mockResolvedValueOnce({jobs:[child('new')]});
  const {rerender}=mount();
  rerender(<MemoryRouter><JobRelationships projectId="other" jobId="different" job={{...job,parent:{job_id:'new-parent'}}} /></MemoryRouter>);
  expect(document.querySelector('details')).toBeNull();
  await screen.findByRole('link',{name:'Job · new'});
  await act(async()=>finish({jobs:[child('obsolete')]}));
  expect(screen.queryByRole('link',{name:'Job · obsolete'})).toBeNull();
  expect(api).toHaveBeenLastCalledWith('other','different',{pageSize:20,pageToken:undefined});
});

it('REL-040,042 DAG-16 shows content directly and preserves rows and cursor through rerenders', async () => {
  api.mockResolvedValueOnce({jobs:[child('one')],next_page_token:'cursor+/='}).mockResolvedValueOnce({jobs:[child('two')],next_page_token:'last'});
  const view=render(<MemoryRouter><JobRelationships projectId="tenant" jobId="current" job={job} /></MemoryRouter>);
  expect(screen.getByRole('heading', {name:'Related jobs and goals'})).toBeVisible();
  expect(document.querySelector('details')).toBeNull();
  await waitFor(()=>expect(screen.getByText('one · platform')).toBeInTheDocument());
  expect(screen.getByRole('link',{name:'Job · one'})).toBeVisible();
  expect(api).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole('button',{name:'Load more child jobs'}));
  await screen.findByRole('link',{name:'Job · two'});
  view.rerender(<MemoryRouter><JobRelationships projectId="tenant" jobId="current" job={{...job,title:'Updated',parent:{job_id:'parent'}}} /></MemoryRouter>);
  expect(screen.getByText('2 child jobs shown · More available')).toBeVisible();
  expect(screen.getByRole('link',{name:'Job · one'})).toBeVisible();
  expect(api).toHaveBeenCalledTimes(2);
  fireEvent.click(screen.getByRole('button',{name:'Load more child jobs'}));
  await waitFor(()=>expect(api).toHaveBeenCalledTimes(3));
  expect(api.mock.calls.at(-1)?.[2]?.pageToken).toBe('last');
});

it('JS-024,025,034 requests all attempts, keeps current-only continuation and retries the same page independently', async () => {
  const siblings = vi.mocked(listJobs);
  siblings.mockResolvedValueOnce({ jobs: [child('current', {goal_id:'originating'})], next_page_token: 'first' })
    .mockResolvedValueOnce({ jobs: [child('archived', { store: 'ARCHIVED', goal_id: 'originating', status: 'COMPLETED' })], next_page_token: 'last' })
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValueOnce({ jobs: [child('archived',{goal_id:'originating'}), child('ready',{goal_id:'originating'})] });
  render(<MemoryRouter><JobRelationships projectId="tenant" jobId="current" job={{...job,parent:{job_id:'parent'}}} showAttempts currentGoal="originating" /></MemoryRouter>);
  const group = within(screen.getByRole('region', { name: 'Other attempts at this goal' }));
  expect(group.getByRole('status')).toHaveTextContent('Loading other attempts');
  fireEvent.click(await group.findByRole('button', { name: 'Load more attempts' }));
  await group.findByRole('link', { name: 'archived' });
  expect(group.queryByRole('link', { name: 'current', exact: true })).toBeNull();
  expect(group.getByRole('link', { name: 'Goal · originating' })).toHaveAttribute('href', '/project/tenant/goals/originating');
  fireEvent.click(group.getByRole('button', { name: 'Load more attempts' }));
  fireEvent.click(await group.findByRole('button', { name: 'Retry other attempts' }));
  await group.findByRole('link', { name: 'ready' });
  expect(group.getAllByRole('listitem')).toHaveLength(2);
  expect(screen.getByRole('link', { name: 'Job · parent' })).toBeVisible();
  expect(siblings.mock.calls.map(call => call[1])).toEqual([undefined, 'first', 'last', 'last'].map(pageToken => ({ goal: 'originating', status: [], includeHidden: true, pageSize: 20, pageToken })));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh relationships' }));
  await group.findByText('No other attempts at this goal');
  expect(siblings.mock.calls.at(-1)?.[1]?.pageToken).toBeUndefined();
});
it('JS-034 no goal makes no sibling request', async () => {
  render(<MemoryRouter><JobRelationships showAttempts projectId="tenant" jobId="current" job={job} /></MemoryRouter>);
  await screen.findByText('No child jobs recorded in this tenant');
  expect(listJobs).not.toHaveBeenCalled();
});
it.each(['tenant','job','goal'].flatMap(change => ['success','failure'].map(outcome => ({change,outcome}))))('JS-035 ignores stale sibling $outcome after $change changes', async ({change,outcome}) => {
  let resolve!: (value: ListRecipeJobsResponse) => void, reject!: (error: Error) => void;
  vi.mocked(listJobs).mockReturnValueOnce(new Promise((yes,no) => {resolve=yes; reject=no;})).mockResolvedValue({jobs:[child('fresh', {tenant_id:change==='tenant'?'other':'tenant', goal_id:change==='goal'?'other':'goal'})]});
  const view = render(<MemoryRouter><JobRelationships projectId="tenant" jobId="current" job={job} showAttempts currentGoal="goal" /></MemoryRouter>);
  view.rerender(<MemoryRouter><JobRelationships projectId={change==='tenant'?'other':'tenant'} jobId={change==='job'?'other':'current'} job={job} showAttempts currentGoal={change==='goal'?'other':'goal'} /></MemoryRouter>);
  await screen.findByRole('link', {name:'fresh'});
  await act(async () => outcome==='success' ? resolve({jobs:[child('stale')]}) : reject(new Error('obsolete')));
  expect(screen.queryByRole('link', {name:'stale'})).toBeNull();
  expect(screen.queryByText('Other attempts unavailable')).toBeNull();
});
