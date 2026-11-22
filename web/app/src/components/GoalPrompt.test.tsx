import { act, render, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import GoalPrompt, { promptImageBinding } from './GoalPrompt';
import { getJobInputArtifacts, type InputArtifact } from '@colony2/shared/api';
import { loadDocument } from './review/document';
import { rasterPreview } from '../utils/promptImages';
vi.mock('@colony2/shared/api', async importOriginal => ({ ...await importOriginal<typeof import('@colony2/shared/api')>(), getJobInputArtifacts: vi.fn() }));
vi.mock('./review/document', async importOriginal => ({ ...await importOriginal<typeof import('./review/document')>(), loadDocument: vi.fn() }));
vi.mock('../utils/promptImages', async importOriginal => ({ ...await importOriginal<typeof import('../utils/promptImages')>(), rasterPreview: vi.fn() }));
const artifact: InputArtifact = { name: 'pasted.png', sizeBytes: 10, kind: 'stored', contentBinding: 'authorized-binding' };
it('binds only a unique authorized relative input name and preserves the owning input-document identity', () => {
  expect(promptImageBinding('tenant', 'owner', 'pasted.png', [artifact])?.url).toContain('/jobs/owner/input-document?binding=authorized-binding&ordinal=0');
  expect(promptImageBinding('tenant', 'owner', 'pasted.png', [artifact, artifact])).toBeUndefined();
  for (const name of ['https://example.com/pasted.png', '//example.com/pasted.png', '/pasted.png', '../pasted.png', 'data:image/png,a', 'blob:x', 'pasted%2epng']) expect(promptImageBinding('t', 'j', name, [{ ...artifact, name }])).toBeUndefined();
  expect(promptImageBinding('t', 'j', 'pasted.png', [{ ...artifact, kind: 'external', reason: 'unavailable' } as InputArtifact])).toBeUndefined();
});
it('renders remote, HTML and ambiguous images as placeholders without image loads', async () => {
  vi.mocked(getJobInputArtifacts).mockResolvedValue({ artifacts: [artifact, artifact] });
  render(<GoalPrompt projectId="t" jobId="j" text={'![bound](pasted.png) ![remote](https://never-fetch.invalid/x.png)\n\n<img src="https://never-fetch.invalid/html.png">'} />);
  await waitFor(() => expect(getJobInputArtifacts).toHaveBeenCalledWith('t', 'j', expect.any(AbortSignal), 0));
  expect(screen.getByText('Image unavailable: bound')).toBeVisible();
  expect(screen.getByText('Image unavailable: remote')).toBeVisible();
  expect(document.querySelector('img')).toBeNull();
  expect(loadDocument).not.toHaveBeenCalled();
});
it('ownership changes discard late inventories, including fallback attempt inputs', async () => {
  let resolve!: (value: { artifacts: InputArtifact[] }) => void;
  vi.mocked(getJobInputArtifacts).mockImplementation((_p, job) => job === 'older' ? new Promise(done => { resolve = done; }) : Promise.resolve({ artifacts: [] }));
  const view = render(<GoalPrompt projectId="t" jobId="older" text="![old](pasted.png)" />);
  view.rerender(<GoalPrompt projectId="t" jobId="newer" text="![new](pasted.png)" />);
  await act(async () => resolve({ artifacts: [artifact] }));
  expect(screen.getByText('Image unavailable: new')).toBeVisible();
  expect(loadDocument).not.toHaveBeenCalled();
});
it('validates bounded bytes before preview and retains original download when preview fails', async () => {
  vi.mocked(getJobInputArtifacts).mockResolvedValue({ artifacts: [artifact] });
  vi.mocked(loadDocument).mockResolvedValue({ bytes: new Uint8Array([1]), text: '', format: 'image', mime: 'image/png', truncated: false });
  vi.mocked(rasterPreview).mockRejectedValue(new Error('invalid image'));
  render(<GoalPrompt projectId="t" jobId="owner" text="![diagram](pasted.png)" />);
  await waitFor(() => expect(rasterPreview).toHaveBeenCalled());
  await screen.findByText('Image unavailable: diagram');
  expect(screen.getByRole('link', { name: 'Download pasted.png' })).toHaveAttribute('href', expect.stringContaining('/jobs/owner/input-document?binding='));
  expect(document.querySelector('img')).toBeNull();
});
