import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { PendingInput } from '@colony2/shared';
import { compareReviewRequests, reviewAge } from '../utils/reviewAge';
import PendingInputsListPage from './PendingInputsListPage';
const state = vi.hoisted(() => ({ pendingInputs: [] as PendingInput[], currentProjectId: 'tenant', setCurrentProjectId: vi.fn(), refresh: vi.fn(), connectionError: false, loadError: undefined as string | undefined }));
vi.mock('@colony2/shared', () => ({ useInputActivity: () => state }));
const row = (id: string, requested_at?: string, task_ordinal?: number, request_id?: string): PendingInput => ({ id, requested_at, task_ordinal, request_id, cell_id:'cell', cell_name:'platform', repo:'github.com/test/platform' });
const now = Date.parse('2026-11-10T12:00:00Z');
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(now); state.pendingInputs=[]; state.connectionError=false; state.loadError=undefined; state.refresh.mockClear(); });
afterEach(() => { cleanup(); vi.useRealTimers(); });
it('PR-007,008 sorts complete immutable snapshots with offsets and deterministic unknown identities', () => {
 const inputs=[row('unknown-b','invalid'),row('new','2026-11-10T11:00:00Z'),row('old','2026-11-10T12:00:00+03:00'),row('tie-b','2026-11-10T11:00:00Z'),row('unknown-a'),row('tie-a','2026-11-10T11:00:00Z')];
 const before=[...inputs];
 expect([...inputs].sort(compareReviewRequests).map(x=>x.id)).toEqual(['old','new','tie-a','tie-b','unknown-a','unknown-b']);
 expect(inputs).toEqual(before);
 expect([row('same',undefined,2,'b'),row('same',undefined,1,'z'),row('same',undefined,2,'a')].sort(compareReviewRequests).map(x=>x.request_id)).toEqual(['z','a','b']);
});
it.each([[0,'just now'],[59999,'just now'],[60000,'1 minute ago'],[120000,'2 minutes ago'],[3599999,'59 minutes ago'],[3600000,'1 hour ago'],[7200000,'2 hours ago'],[86400000,'1 day ago'],[172800000,'2 days ago'],[-60000,'just now']])('PR-010 age boundary %s', (elapsed,label) => { expect(reviewAge(now-elapsed,now)).toBe(`Requested ${label}`); });
it('PR-011,013 updates each minute and on focus/visibility without reads; cleans up', () => {
 state.pendingInputs=[row('a',new Date(now).toISOString()),row('unknown')];
 const removeWindow=vi.spyOn(window,'removeEventListener'),removeDocument=vi.spyOn(document,'removeEventListener');
 const {unmount}=render(<MemoryRouter><PendingInputsListPage projectId="tenant" /></MemoryRouter>);
 expect(screen.getByRole('combobox', { name: 'Sort by' })).toBeVisible();
 const age=screen.getByText('Requested just now');
 expect(age).toHaveAttribute('datetime','2026-11-10T12:00:00.000Z'); expect(age).toHaveAttribute('tabindex','0'); expect(age.getAttribute('title')).toMatch(/2026/);
 expect(screen.getByText('Request time unavailable')).toBeVisible();
 act(()=>{vi.advanceTimersByTime(60000)}); expect(screen.getByText('Requested 1 minute ago')).toBeVisible();
 vi.setSystemTime(now+3600000); fireEvent.focus(window); expect(screen.getByText('Requested 1 hour ago')).toBeVisible();
 vi.setSystemTime(now+86400000); fireEvent(document,new Event('visibilitychange')); expect(screen.getByText('Requested 1 day ago')).toBeVisible();
 expect(state.refresh).not.toHaveBeenCalled();
 unmount(); expect(removeWindow).toHaveBeenCalledWith('focus',expect.any(Function)); expect(removeDocument).toHaveBeenCalledWith('visibilitychange',expect.any(Function)); expect(vi.getTimerCount()).toBe(0);
});

it('preserves active controls and ordering as age labels advance', () => {
 state.pendingInputs=[{...row('a',new Date(now-60000).toISOString()),title:'Food Alpha bar'},{...row('z',new Date(now).toISOString()),title:'Food Zeta bar'}];
 render(<MemoryRouter initialEntries={['/?q=foo+bar&cellId=cell&sort=title&order=descend']}><PendingInputsListPage projectId="tenant" /></MemoryRouter>);
 const order=()=>Array.from(document.querySelectorAll('.ant-list .ant-card')).map(card=>card.textContent);
 expect(order()[0]).toContain('Job ID: z');
 act(()=>vi.advanceTimersByTime(60000));
 expect(order()[0]).toContain('Job ID: z');
 expect(screen.getByRole('textbox',{name:'Search reviews'})).toHaveValue('foo bar');
 expect(screen.getByRole('combobox',{name:'Sort by'})).toHaveValue('title:descend');
 expect(screen.getByText('Requested 1 minute ago')).toHaveAttribute('datetime',new Date(now).toISOString());
 expect(state.refresh).not.toHaveBeenCalled();
});
it('shows load failures and connection warnings with active controls instead of a successful empty state', () => {
 state.connectionError=true; state.loadError='required cell_id is missing';
 render(<MemoryRouter initialEntries={['/?q=missing&cellSelection=none']}><PendingInputsListPage projectId="tenant" /></MemoryRouter>);
 expect(screen.getByRole('alert')).toHaveTextContent('required cell_id is missing');
 expect(screen.getByText('Live updates unavailable. Refresh to check for requests.')).toBeVisible();
 expect(screen.getByRole('button',{name:'Retry reviews'})).toBeVisible();
 expect(screen.queryByText('No pending reviews')).not.toBeInTheDocument();
 expect(screen.getByRole('textbox',{name:'Search reviews'})).toHaveValue('missing');
});
