import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { createCell, getCell, listCells } from '@colony2/shared/api';
import { registryCell } from '../test/cellFixtures';
import CreateCellDialog from './CreateCellDialog';
import CellsList from './CellsList';
import CellPage from './CellPage';
vi.mock('@colony2/shared/api', () => ({ createCell: vi.fn(), getCell: vi.fn(), listCells: vi.fn() }));
beforeEach(() => {
  vi.mocked(createCell).mockReset(); vi.mocked(getCell).mockReset(); vi.mocked(listCells).mockReset();
});
it('creates a cell without a job and opens its detail page with a stable goals link', async () => {
  const cell = registryCell('payments');
  vi.mocked(listCells).mockResolvedValue({ cells: [] });
  vi.mocked(createCell).mockResolvedValue(cell);
  vi.mocked(getCell).mockResolvedValue(cell);
  render(<MemoryRouter initialEntries={['/project/tenant/cells']}><Routes>
    <Route path="/project/:tenant/cells" element={<CellsList projectId="tenant" />} />
    <Route path="/project/:tenant/cells/:id" element={<CellPage projectId="tenant" cellId={cell.id} />} />
  </Routes></MemoryRouter>);
  fireEvent.click(screen.getByRole('button', { name: 'Create cell' }));
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'payments' } });
  fireEvent.change(screen.getByLabelText('Repository'), { target: { value: 'https://github.com/acme/payments.git' } });
  fireEvent.change(screen.getByLabelText('Metadata (optional JSON object)'), { target: { value: '{"nested":{"items":[null,true]}}' } });
  fireEvent.click(screen.getAllByRole('button', { name: 'Create cell' }).at(-1)!);
  const link = await screen.findByRole('link', { name: 'Goals for this cell' });
  expect(link).toHaveAttribute('href', '/project/tenant/goals?cellId=cell-payments&executionState=cells-v1');
  expect(createCell).toHaveBeenCalledWith('tenant', { name: 'payments', repo: 'https://github.com/acme/payments.git', description: '', metadata: { nested: { items: [null, true] } }, requestId: expect.any(String) });
});
it('keeps creation fields and the request ID when retrying a lost response', async () => {
  vi.mocked(createCell).mockRejectedValueOnce(new Error('Connection lost')).mockResolvedValue(registryCell());
  render(<CreateCellDialog projectId="tenant" onClose={vi.fn()} onCreated={vi.fn()} />);
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'platform' } });
  fireEvent.change(screen.getByLabelText('Repository'), { target: { value: 'github.com/acme/platform' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create cell' }));
  await screen.findByText('Connection lost');
  fireEvent.click(screen.getByRole('button', { name: 'Create cell' }));
  await waitFor(() => expect(createCell).toHaveBeenCalledTimes(2));
  expect(vi.mocked(createCell).mock.calls[1]).toEqual(vi.mocked(createCell).mock.calls[0]);
});
it('paginates registry cells with no jobs and sends lifecycle filters to the backend', async () => {
  vi.mocked(listCells).mockResolvedValueOnce({ cells: [registryCell('first')], nextPageToken: 'next' })
    .mockResolvedValueOnce({ cells: [registryCell('second')] }).mockResolvedValue({ cells: [] });
  render(<MemoryRouter><CellsList projectId="tenant" /></MemoryRouter>);
  await screen.findByRole('link', { name: 'first' });
  fireEvent.click(screen.getByRole('button', { name: 'Next cells' }));
  await screen.findByRole('link', { name: 'second' });
  expect(listCells).toHaveBeenLastCalledWith('tenant', { state: 'ACTIVE', pageSize: 50, pageToken: 'next' });
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Cell state' }));
  fireEvent.click(await screen.findByText('Retired'));
  await waitFor(() => expect(listCells).toHaveBeenLastCalledWith('tenant', expect.objectContaining({ state: 'RETIRED', pageToken: undefined })));
});
