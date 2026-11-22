import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import ReviewDocuments from './ReviewDocuments';
import type { ReviewDocumentMetadata } from '@colony2/shared';
import fixtures from '../test/fixtures/review-presentation.json';

// Exact serialized fixtures from c2j d031eb49c677, pkg/input/testdata/review-presentation.json.
const props = {projectId:'tenant', jobId:'job', requestId:'request'};
const originalWidth = window.innerWidth;
afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.innerWidth = originalWidth; });
const document = (name: string) => ({name, stored:{key:{jobId:'producer',taskOrdinal:1,name,sizeBytes:1}}});
const catalogIds = () => [...screen.getByRole('navigation', {name:'Review documents'})
  .querySelectorAll('.review-document-summary .ant-typography')].map(el => el.textContent!.split(' · ')[0]);
const selectedId = () => window.document.querySelector('.review-selected-details .ant-typography')?.textContent?.split(' · ')[0];
function fetchDocuments() {
  const fetcher = vi.fn(async () => new Response('# Retained document'));
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

it.each(fixtures.flatMap(fixture => [1280,390].map(width => ({...fixture,width}))))
('native $name at $width px selects and orders exact attachment IDs', async fixture => {
  window.innerWidth = fixture.width;
  const fetcher = fetchDocuments();
  render(<ReviewDocuments {...props} documents={fixture.form.documents}
    overviewDocumentId={fixture.form.overview_document_id}
    documentMetadata={fixture.form.document_metadata as Record<string, ReviewDocumentMetadata>} />);
  expect(screen.getByRole('button', {name:'Show documents'})).toHaveAttribute('aria-expanded','false');
  const initial = fixture.expected_initial_document_id;
  if (initial) {
    await screen.findByRole('heading', {name:'Retained document'});
    expect(new URL(fetcher.mock.calls[0][0]).searchParams.get('name')).toBe(Object.entries(fixture.form.documents).find(([id]) => id === initial)?.[1].stored.key.name);
    expect(selectedId()).toBe(initial);
  } else {
    expect(screen.getByText('No documents attached')).toBeVisible();
    expect(fetcher).not.toHaveBeenCalled();
  }
  fireEvent.click(screen.getByRole('button', {name:'Show documents'}));
  const expected = Object.values(fixture.expected_groups).flat();
  expect(catalogIds()).toEqual(expected);
  if (fixture.width === 390 && initial) {
    expect(screen.getByRole('combobox', {name:'Review document'})).toBeVisible();
    expect(screen.getByText('Starting document')).toBeVisible();
  }
});

it.each([1280,390])('role beats conflicting orders and UTF-8 breaks numeric and omitted ties at %s px', async width => {
  window.innerWidth = width;
  fetchDocuments();
  const docs = Object.fromEntries(['s','\u{10000}','z','\uE000','d','n','a'].map(id => [id,document(`${id}.md`)]));
  const metadata: Record<string,ReviewDocumentMetadata> = {
    s:{role:'supporting',order:0}, '\u{10000}':{role:'primary',order:10},
    z:{role:'primary'}, '\uE000':{role:'primary',order:10}, d:{role:'diagnostic',order:0},
    n:{order:0}, a:{role:'primary'},
  };
  render(<ReviewDocuments {...props} documents={docs} documentMetadata={metadata}/>);
  await screen.findByRole('heading',{name:'Retained document'});
  expect(selectedId()).toBe('\uE000');
  fireEvent.click(screen.getByRole('button',{name:'Show documents'}));
  expect(catalogIds()).toEqual(['\uE000','\u{10000}','a','z','s','d','n']);
});

it('uses UTF-8 IDs for legacy fallback without normalizing canonically equivalent IDs', async () => {
  fetchDocuments();
  const ids = ['\u{10000}','\uE000','é','e\u0301'];
  render(<ReviewDocuments {...props} documents={Object.fromEntries(ids.map(id => [id,document(`${id}.md`)]))}/>);
  await screen.findByRole('heading',{name:'Retained document'});
  expect(selectedId()).toBe('e\u0301');
  fireEvent.click(screen.getByRole('button',{name:'Show documents'}));
  expect(catalogIds()).toEqual(['e\u0301','é','\uE000','\u{10000}']);
});

it.each(['overview','primary','smallest'] as const)('uses %s fallback after removal and preserves valid selections', async fallback => {
  const fetcher = fetchDocuments();
  const docs = {a:document('a.md'),s:document('s.md'),p:document('p.md'),z:document('z.md')};
  const metadata: Record<string,ReviewDocumentMetadata> = {s:{role:'supporting',order:0},p:{role:'primary',order:10}};
  const view = (documents: typeof docs | Omit<typeof docs,'z'>, requestId = 'request', overviewDocumentId?: string,
    documentMetadata = metadata) => <ReviewDocuments {...props} {...{documents,requestId,overviewDocumentId,documentMetadata}}/>;
  const {rerender} = render(view(docs,'request','z'));
  await screen.findByRole('heading',{name:'Retained document'});
  // Changing overview, metadata and membership does not replace an attached selection.
  rerender(view({...docs,extra:document('extra.md')},'request','a',{...metadata,p:{role:'primary',order:0}}));
  expect(selectedId()).toBe('z');
  expect(fetcher).toHaveBeenCalledTimes(1);
  const {z:removed,...remaining} = docs;
  rerender(view(remaining,'request',fallback === 'overview' ? 's' : 'missing',fallback === 'smallest' ? {s:metadata.s} : metadata));
  const expected = fallback === 'overview' ? 's' : fallback === 'primary' ? 'p' : 'a';
  await waitFor(() => expect(selectedId()).toBe(expected));
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
  // Reattaching the old overview does not resurrect the removed selection.
  rerender(view(docs,'request','z'));
  expect(selectedId()).toBe(expected);
  // A new request does reset the reader, with collapsed navigation.
  rerender(view(docs,'next','z'));
  await waitFor(() => expect(selectedId()).toBe('z'));
  expect(screen.getByRole('button',{name:'Show documents'})).toHaveAttribute('aria-expanded','false');
});
