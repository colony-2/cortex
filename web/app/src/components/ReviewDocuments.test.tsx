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
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(source)));
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
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: false }).mockResolvedValueOnce(new Response('# Recovered'));
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
      .mockResolvedValueOnce(new Response('# Current')));
    const { rerender } = render(<ReviewDocuments projectId="tenant" jobId="job" requestId="old" documents={documents} />);
    rerender(<ReviewDocuments projectId="tenant" jobId="job" requestId="new" documents={documents} />);
    await screen.findByRole('heading', { name: 'Current' });
    resolveOld(new Response('# Stale'));
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Stale' })).not.toBeInTheDocument());
  });
});

describe('SAV-019 review attachment membership', () => {
  it('preserves IDs, display names, stored names and blocks ambiguous membership', async () => {
    const docs = {
      ...documents,
      imageID: {kind:'stored' as const,name:'display.png',stored:{key:{jobId:'child',taskOrdinal:5,name:'stored.png'}}},
      zconflict: {kind:'stored' as const,name:'duplicate.png',stored:{key:{jobId:'child',taskOrdinal:6,name:'other.png'}}},
      zconflict2: {kind:'stored' as const,name:'duplicate.png',stored:{key:{jobId:'child',taskOrdinal:7,name:'different.png'}}},
    };
    const source = '![id](imageID)\n![display](display.png)\n![stored](stored.png)\n![blocked](duplicate.png)';
    const mock = vi.fn(async (url:string) => String(url).includes('document_id=design') ? new Response(source) : new Response('not raster'));
    vi.stubGlobal('fetch',mock);
    render(<ReviewDocuments projectId="tenant" jobId="job" requestId="request" documents={docs} />);
    // Select design explicitly because document IDs sort independently of insertion order.
    await screen.findByText(/Blocked external or ambiguous image/);
    await waitFor(()=>expect(mock.mock.calls.filter(c=>String(c[0]).includes('document_id=imageID'))).toHaveLength(3));
    expect(mock.mock.calls.some(c=>/document_id=zconflict/.test(String(c[0])))).toBe(false);
  });
});
