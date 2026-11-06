import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, it, vi } from 'vitest';
import { getAdmin, updateAdmin } from '@colony2/shared';
import type { AdminSnapshot } from '@colony2/shared';
import AdminPage, { AdminPanel } from './AdminPage';

vi.mock('@colony2/shared', async original => ({ ...await original<object>(), getAdmin: vi.fn(), updateAdmin: vi.fn() }));
const snapshot: AdminSnapshot = { revision: 'rev1', configs: [{ name: 'model', value: 'initial', source: 'tenant' }], hooks: [{ name: '.c2/hooks/build.yaml', source: 'tenant', value: { perms: { jobdb: {} } } }], git: [], secretReferences: ['token'], secretWrites: true };
beforeEach(() => { vi.mocked(getAdmin).mockReset().mockResolvedValue(snapshot); vi.mocked(updateAdmin).mockReset().mockResolvedValue(); });
it('opens a deep-linked tab and sends a revision with configuration edits', async () => {
 render(<MemoryRouter initialEntries={['/admin?tab=configs']}><AdminPage projectId="tenant" /></MemoryRouter>);
 await screen.findByText('model');
 fireEvent.click(screen.getByRole('button', { name: 'Edit', exact: true }));
 const dialog = screen.getByRole('dialog');
 fireEvent.change(within(dialog).getByLabelText('Value (JSON)'), { target: { value: '"updated"' } });
 fireEvent.click(within(dialog).getByRole('button', { name: 'Save', exact: true }));
 await waitFor(() => expect(updateAdmin).toHaveBeenCalledWith('tenant', 'configs', { name: 'model', operation: 'set', value: 'updated', revision: 'rev1' }, undefined));
});
it('preserves JSON edits after a conflict and never silently overwrites', async () => {
 vi.mocked(updateAdmin).mockRejectedValue(new Error('Settings changed; refresh and review.'));
 render(<MemoryRouter><AdminPanel projectId="tenant" kind="configs" /></MemoryRouter>);
 fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
 fireEvent.change(screen.getByLabelText('Value (JSON)'), { target: { value: '"my edit"' } });
 fireEvent.click(screen.getByRole('button', { name: 'Save', exact: true }));
 await screen.findByText('Settings changed; refresh and review.');
 expect(screen.getByLabelText('Value (JSON)')).toHaveValue('"my edit"');
 expect(updateAdmin).toHaveBeenCalledTimes(1);
});
it('distinguishes clearing a cell policy from restoring tenant inheritance', async () => {
 vi.mocked(getAdmin).mockResolvedValue({ ...snapshot, hooks: [{ ...snapshot.hooks[0], source: 'cell' }] });
 render(<MemoryRouter><AdminPanel projectId="tenant" cellId="cell-1" kind="hooks" /></MemoryRouter>);
 fireEvent.click(await screen.findByRole('button', { name: 'Clear permissions' }));
 fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
 await waitFor(() => expect(updateAdmin).toHaveBeenCalledWith('tenant', 'hooks', { name: '.c2/hooks/build.yaml', operation: 'clear', revision: 'rev1' }, 'cell-1'));
 await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
 fireEvent.click(await screen.findByRole('button', { name: 'Use tenant default' }));
 fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
 await waitFor(() => expect(updateAdmin).toHaveBeenLastCalledWith('tenant', 'hooks', { name: '.c2/hooks/build.yaml', operation: 'remove', revision: 'rev1' }, 'cell-1'));
});
it('shows tenant settings read-only from a cell with a scoped admin link', async () => {
 render(<MemoryRouter><AdminPanel projectId="other tenant" cellId="cell-1" kind="configs" /></MemoryRouter>);
 await screen.findByText('model');
 expect(getAdmin).toHaveBeenCalledWith('other tenant', 'cell-1');
 expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
 expect(screen.queryByRole('button', { name: 'Add config' })).not.toBeInTheDocument();
 expect(screen.getByRole('link', { name: 'Manage configs in Admin' })).toHaveAttribute('href', '/project/other%20tenant/admin?tab=configs');
});
it('never fetches existing secret values, masks replacements and discards failed drafts', async () => {
 vi.mocked(updateAdmin).mockRejectedValue(new Error('Secret update failed.'));
 render(<MemoryRouter><AdminPanel projectId="tenant" kind="secrets" /></MemoryRouter>);
 await screen.findByText('token');
 expect(screen.getByText(/not an inventory or proof/)).toBeVisible();
 fireEvent.click(screen.getByRole('button', { name: 'Replace value' }));
 const input = screen.getByLabelText('New secret value');
 expect(input).toHaveAttribute('type', 'password');
 expect(input).toHaveValue('');
 fireEvent.change(input, { target: { value: 'sensitive-new-value' } });
 fireEvent.click(screen.getByRole('button', { name: 'Save', exact: true }));
 await screen.findByText('Secret update failed.');
 expect(input).toHaveValue('');
 expect(screen.queryByText('sensitive-new-value')).not.toBeInTheDocument();
 expect(updateAdmin).toHaveBeenCalledWith('tenant', 'secrets', { name: 'token', operation: 'set', value: 'sensitive-new-value' }, undefined);
 fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
 fireEvent.click(screen.getByRole('button', { name: 'Add or replace secret' }));
 expect(screen.getByLabelText('New secret value')).toHaveValue('');
});
it('preserves an explicit null configuration and rejects accidental duplicate additions', async () => {
 vi.mocked(getAdmin).mockResolvedValue({ ...snapshot, configs: [{ name: 'git-profile-ro', value: null }] });
 render(<MemoryRouter><AdminPanel projectId="tenant" kind="configs" /></MemoryRouter>);
 fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
 expect(screen.getByLabelText('Value (JSON)')).toHaveValue('null');
 fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
 fireEvent.click(screen.getByRole('button', { name: 'Add config' }));
 fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'git-profile-ro' } });
 fireEvent.click(screen.getByRole('button', { name: 'Save', exact: true }));
 await screen.findByText('This name already exists. Use Edit to change it.');
 expect(updateAdmin).not.toHaveBeenCalled();
});
