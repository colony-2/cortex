import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import ReviewDocuments from './ReviewDocuments';
import type { ReviewDocument } from '@colony2/shared';

const documents: Record<string, ReviewDocument> = {
  design: { kind: 'stored', name: 'design.md', stored: { key: { jobId: 'child', taskOrdinal: 5, name: 'design.md', sizeBytes: 20 } } },
};
afterEach(() => vi.unstubAllGlobals());

describe('review documents', () => {
  it('renders Markdown safely and exposes the exact immutable source', async () => {
    const source = '# Design\n\n**Bold**\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))';
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => source }));
    const { container } = render(<ReviewDocuments projectId="tenant" jobId="job" requestId="request" documents={documents} />);
    expect(await screen.findByRole('heading', { name: 'Design' })).toBeInTheDocument();
    expect(container.querySelector('script')).toBeNull();
    expect(screen.getByText('bad').getAttribute('href')).not.toMatch(/^javascript:/);
    expect(container.querySelector('[contenteditable]')).toBeNull();
    fireEvent.click(screen.getByRole('radio', { name: 'Markdown', exact: true }));
    expect(screen.getByLabelText('Markdown source').textContent).toBe(source);
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining('request_id=request&document_id=design'), expect.anything());
  });

  it('can retry document failures and handles reviews without documents', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: false }).mockResolvedValueOnce({ ok: true, text: async () => '# Recovered' });
    vi.stubGlobal('fetch', fetchMock);
    const { rerender } = render(<ReviewDocuments projectId="tenant" jobId="job" requestId="request" documents={documents} />);
    await screen.findByText(/Could not load this document/);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await screen.findByRole('heading', { name: 'Recovered' });
    rerender(<ReviewDocuments projectId="tenant" jobId="job" requestId="request" documents={{}} />);
    expect(screen.getByText('No documents attached')).toBeInTheDocument();
  });

  it('does not display a late response from another request', async () => {
    let resolveOld!: (value: unknown) => void;
    vi.stubGlobal('fetch', vi.fn().mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve; }))
      .mockResolvedValueOnce({ ok: true, text: async () => '# Current' }));
    const { rerender } = render(<ReviewDocuments projectId="tenant" jobId="job" requestId="old" documents={documents} />);
    rerender(<ReviewDocuments projectId="tenant" jobId="job" requestId="new" documents={documents} />);
    await screen.findByRole('heading', { name: 'Current' });
    resolveOld({ ok: true, text: async () => '# Stale' });
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Stale' })).not.toBeInTheDocument());
  });
});
