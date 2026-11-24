import { beforeEach, expect, it, vi } from 'vitest';
import { AppShell } from './App';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { listCells } from '@colony2/shared/api';
import { registryCell } from './test/cellFixtures';
const activity = vi.hoisted(() => ({ pendingInputs: [] as {id:string;cell_id:string}[], discoveryStatus:'current', currentProjectId:'navigation', setCurrentProjectId: vi.fn(), refresh: vi.fn() }));
vi.mock('@colony2/shared', async original => ({ ...await original<typeof import('@colony2/shared')>(), isAuthenticated: () => true, useInputActivity: () => activity }));
vi.mock('@colony2/shared/api', async original => ({ ...await original<typeof import('@colony2/shared/api')>(), listJobs: vi.fn().mockResolvedValue({jobs:[]}), listCells: vi.fn() }));
vi.mock('./hooks/useCompact', () => ({ useCompact: () => false }));
vi.mock('./components/JobsListPage', () => ({ default: () => <input aria-label="Page draft" /> }));
vi.mock('./components/JobStoryPage', () => ({ default: () => <input aria-label="Review draft" /> }));
function shell(path='/project/navigation/jobs') { return <MemoryRouter initialEntries={[path]}><AppShell defaultTenantId="navigation" /></MemoryRouter>; }
beforeEach(() => { vi.mocked(listCells).mockResolvedValue({cells:[registryCell('platform','navigation')]}); activity.pendingInputs=[]; activity.currentProjectId='navigation'; activity.discoveryStatus='current'; });
it('omits zero badges while keeping All, cells and Archive reachable', async () => {
  render(shell());
  const all=screen.getByRole('link',{name:'All',exact:true});
  expect(all.querySelector('.inbox-badge')).toBeNull();
  expect(await screen.findByRole('link',{name:'platform',exact:true})).toBeVisible();
  expect(screen.getByRole('link',{name:'Archive',exact:true}).querySelector('.inbox-badge')).toBeNull();
  expect(screen.queryByRole('link',{name:'Goals',exact:true})).toBeNull();
  expect(screen.getByRole('link',{name:'Runners',exact:true})).toHaveAttribute('href','/project/navigation/runners');
  expect(screen.getByRole('link',{name:'Settings',exact:true})).toHaveAttribute('href','/project/navigation/admin');
});
it('shows pending counts without changing them on navigation width changes', () => {
  activity.pendingInputs=[{id:'one',cell_id:'cell-platform'}]; render(shell());
  fireEvent.click(screen.getByRole('button',{name:'Collapse navigation'}));
  expect(screen.getAllByLabelText('1 pending responses')[0]).toBeVisible();
  fireEvent.click(screen.getByRole('button',{name:'Expand navigation'}));
  expect(screen.getAllByLabelText('1 pending responses')[0]).toBeVisible();
});
it('never shows another tenant count as current or empty', () => {
  activity.currentProjectId='other'; activity.pendingInputs=[{id:'one',cell_id:'cell-platform'}]; render(shell());
  expect(screen.getByRole('link',{name:'Reviews',exact:true})).toHaveTextContent('?');
});
it('starts stories expanded and preserves mounted review text through width toggles', () => {
  render(shell('/project/navigation/jobs/one/story'));
  fireEvent.change(screen.getByLabelText('Review draft'),{target:{value:'Unsent feedback'}});
  fireEvent.click(screen.getByRole('button',{name:'Collapse navigation'}));
  fireEvent.click(screen.getByRole('button',{name:'Expand navigation'}));
  expect(screen.getByLabelText('Review draft')).toHaveValue('Unsent feedback');
});
it('explains retired goal links and offers All instead of opening an arbitrary job', () => {
  render(shell('/project/navigation/goals/legacy'));
  expect(screen.getByText('Goal links have been retired')).toBeVisible();
  expect(screen.getByRole('link',{name:'Open All jobs'})).toHaveAttribute('href','/project/navigation/jobs');
});
