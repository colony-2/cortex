import { afterEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import RestartContextPreview from './RestartContextPreview';

vi.mock('react-json-view', () => ({ default: ({ src }: { src: unknown }) => <pre>{JSON.stringify(src)}</pre> }));
afterEach(() => vi.unstubAllGlobals());

const context = (chapter_ordinal: number) => ({
  project_id: 'tenant', job_id: 'job', chapter_ordinal, scope_id: 'scope',
  job: { marker: `context-${chapter_ordinal}` }, visible: { sequence: {} }, redacted_paths: [],
});

it('aborts old requests and ignores their results when the restart point changes', async () => {
  let finishFirst!: (value: unknown) => void;
  const fetch = vi.fn().mockImplementationOnce(() => new Promise(resolve => { finishFirst = resolve; }))
    .mockResolvedValueOnce({ ok: true, json: async () => context(22) });
  vi.stubGlobal('fetch', fetch);
  const { rerender } = render(<RestartContextPreview projectId="tenant" jobId="job" chapterOrdinal={11} />);
  const firstSignal = fetch.mock.calls[0][1].signal as AbortSignal;
  rerender(<RestartContextPreview projectId="tenant" jobId="job" chapterOrdinal={22} />);
  expect(firstSignal.aborted).toBe(true);
  expect(await screen.findByRole('region', { name: 'Context before patching' })).toHaveTextContent('context-22');
  await act(async () => { finishFirst({ ok: true, json: async () => context(11) }); });
  expect(screen.getByRole('region', { name: 'Context before patching' })).not.toHaveTextContent('context-11');
  rerender(<RestartContextPreview projectId="tenant" jobId="job" />);
  expect(screen.queryByRole('region', { name: 'Context before patching' })).not.toBeInTheDocument();
});

it('shows inspection errors and retries without presenting an empty snapshot as success', async () => {
  vi.stubGlobal('fetch', vi.fn()
    .mockResolvedValueOnce({ ok: false, status: 422, statusText: 'Unprocessable Entity', text: async () => 'History unavailable' })
    .mockResolvedValueOnce({ ok: true, json: async () => context(11) }));
  render(<RestartContextPreview projectId="tenant" jobId="job" chapterOrdinal={11} />);
  expect(await screen.findByText(/History unavailable/)).toBeVisible();
  expect(screen.queryByRole('region', { name: 'Context before patching' })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Retry context' }));
  expect(await screen.findByRole('region', { name: 'Context before patching' })).toHaveTextContent('context-11');
});

it('rejects a snapshot for another boundary', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => context(99) }));
  render(<RestartContextPreview projectId="tenant" jobId="job" chapterOrdinal={11} />);
  expect(await screen.findByText('Restart context does not match the selected job and restart point.')).toBeVisible();
  expect(screen.queryByText(/context-99/)).not.toBeInTheDocument();
});
