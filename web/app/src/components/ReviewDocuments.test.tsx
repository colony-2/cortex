import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import ReviewDocuments from './ReviewDocuments';
import type { ReviewDocument } from '@colony2/shared';

const documents: Record<string, ReviewDocument> = {
  design: { kind: 'stored', name: 'design.md', stored: { key: { jobId: 'child', taskOrdinal: 5, name: 'design.md', sizeBytes: 20 } } },
};
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe('review documents', () => {
  it('renders Markdown safely and exposes the exact immutable source', async () => {
    const source = '# Design\n\n**Bold**\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))';
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(source)));
    const { container } = render(<ReviewDocuments projectId="tenant" jobId="job" requestId="request" documents={documents} />);
    expect(await screen.findByRole('heading', { name: 'Design' })).toBeInTheDocument();
    expect(container.querySelector('script')).toBeNull();
    expect(screen.getByText('bad')).not.toHaveAttribute('href', expect.stringMatching(/^javascript:/));
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

describe('MAL review file navigation', () => {
  const linked = {
    a: {kind:'stored' as const,name:'display.md',stored:{key:{jobId:'producer-A',taskOrdinal:4,name:'docs/bar.md'}}},
    'b/雪': {kind:'stored' as const,name:'foo.md',stored:{key:{jobId:'producer-B',taskOrdinal:99,name:'docs/foo.md'}}},
    z: {kind:'stored' as const,name:'data.json',stored:{key:{jobId:'producer-C',taskOrdinal:3,name:'docs/data.json'}}},
  };
  it('MAL-013,015,021,022,028,031 navigates by canonical path through review-owned encoded IDs', async () => {
    const mock=vi.fn(async(url:string)=>new Response(url.includes('document_id=a')?'# Bar\n[next](foo.md?ignored=1#target)':url.includes('document_id=z')?'{"n":42}':'# Target\n[self](foo.md#target) [json](data.json) [missing](outside.md)'));
    vi.stubGlobal('fetch',mock);
    render(<ReviewDocuments projectId="tenant" jobId="job" requestId="request" documents={linked}/>);
    fireEvent.click(await screen.findByRole('button',{name:'next'}));
    await screen.findByRole('heading',{name:'Target'});
    expect(screen.getByRole('link',{name:'Download'})).toHaveAttribute('download','foo.md');
    expect(screen.getByRole('link',{name:'Download'}).getAttribute('href')).toContain('document_id=b%2F%E9%9B%AA');
    expect(screen.getByRole('button', { name: /foo.md b\/雪/ })).toHaveAttribute('aria-current', 'true');
    fireEvent.click(screen.getByRole('button',{name:'self'}));expect(mock).toHaveBeenCalledTimes(2);
    fireEvent.click(screen.getByRole('button',{name:'missing'}));expect(mock).toHaveBeenCalledTimes(2);
    expect(screen.getByRole('status')).toHaveTextContent('missing');
    fireEvent.click(screen.getByRole('button',{name:'json'}));
    await screen.findByLabelText('Formatted JSON');expect(mock).toHaveBeenCalledTimes(3);
    for(const [url] of mock.mock.calls) {expect(url).toContain('/projects/tenant/reviews/job/');expect(url).not.toMatch(/producer|ignored/);}
  });
  it('MAL-023 rejects ambiguous and escaping links without fetching', async () => {
    const mock=vi.fn(async()=>new Response('[ambiguous](same) [escape](../../x.md) [bad](bad%FF)'));
    vi.stubGlobal('fetch',mock);
    render(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={{...linked,'b/雪':{...linked['b/雪'],name:'same'},z:{...linked.z,name:'same'}}}/>);
    for(const name of ['ambiguous','escape','bad']) {fireEvent.click(await screen.findByRole('button',{name}));expect(screen.getByRole('status')).toHaveTextContent('Artifact unavailable');}
    expect(mock).toHaveBeenCalledTimes(1);
  });
  it('MAL-033 keeps failed target selected and retries only its endpoint', async () => {
    let recover=false;
    const mock=vi.fn(async(url:string)=>url.includes('document_id=a')?new Response('[next](foo.md)'):new Response(recover?'# Recovered':'failure',{status:recover?200:503}));
    vi.stubGlobal('fetch',mock);render(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={linked}/>);
    fireEvent.click(await screen.findByRole('button',{name:'next'}));await screen.findByText(/Could not load/);
    expect(screen.getByRole('link',{name:'Download'})).toHaveAttribute('download','foo.md');
    recover=true;fireEvent.click(screen.getByRole('button',{name:'Retry'}));await screen.findByRole('heading',{name:'Recovered'});
    expect(mock.mock.calls.at(-1)?.[0]).toContain('document_id=b%2F%E9%9B%AA');
  });
  it('MAL-032 suppresses pending navigation content after context change', async () => {
    let finish!:(r:Response)=>void;let signal!:AbortSignal;
    const mock=vi.fn(async(url:string,options:RequestInit)=>{
      if(url.includes('document_id=b')) {signal=options.signal as AbortSignal;return new Promise<Response>(r=>{finish=r;});}
      return new Response(url.includes('request_id=new')?'# New':'[next](foo.md)');
    });
    vi.stubGlobal('fetch',mock);
    const {rerender}=render(<ReviewDocuments projectId="tenant" jobId="job" requestId="old" documents={linked}/>);
    fireEvent.click(await screen.findByRole('button',{name:'next'}));await screen.findByLabelText('Loading document');
    rerender(<ReviewDocuments projectId="tenant" jobId="job" requestId="new" documents={linked}/>);
    await screen.findByRole('heading',{name:'New'});expect(signal.aborted).toBe(true);
    finish(new Response('# Stale'));await waitFor(()=>expect(screen.queryByRole('heading',{name:'Stale'})).toBeNull());
    expect(screen.queryByRole('status')).toBeNull();
  });
});

it('MAL-032 cancels a pending linked target when selecting another document and suppresses its late error', async()=>{
  const docs={a:{kind:'stored' as const,stored:{key:{jobId:'p',taskOrdinal:1,name:'a.md'}}},b:{kind:'stored' as const,stored:{key:{jobId:'p',taskOrdinal:2,name:'b.md'}}},c:{kind:'stored' as const,stored:{key:{jobId:'p',taskOrdinal:3,name:'c.md'}}}};
  let fail!:(e:Error)=>void;let signal!:AbortSignal;
  vi.stubGlobal('fetch',vi.fn((url:string,options:RequestInit)=>{
    if(url.includes('document_id=b')) {signal=options.signal as AbortSignal;return new Promise<Response>((_,reject)=>{fail=reject;});}
    return Promise.resolve(new Response(url.includes('document_id=c')?'# Current C':'[next](b.md#stale)'));
  }));
  render(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={docs}/>);
  fireEvent.click(await screen.findByRole('button',{name:'next'}));await screen.findByLabelText('Loading document');
  fireEvent.click(screen.getByRole('button',{name:'c.md c'}));
  await screen.findByRole('heading',{name:'Current C'});expect(signal.aborted).toBe(true);
  fail(new Error('Obsolete failure'));await waitFor(()=>expect(screen.queryByText('Obsolete failure')).toBeNull());
  expect(screen.queryByRole('status')).toBeNull();
});

it('MAL-020 revalidates review membership after an attached target is removed',async()=>{
  const doc=(name:string)=>({kind:'stored' as const,stored:{key:{jobId:'p',taskOrdinal:1,name}}});
  const mock=vi.fn(async()=>new Response('[next](foo.md)'));vi.stubGlobal('fetch',mock);
  const {rerender}=render(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={{a:doc('bar.md'),b:doc('foo.md')}}/>);
  await screen.findByRole('button',{name:'next'});
  rerender(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={{a:doc('bar.md')}}/>);
  fireEvent.click(screen.getByRole('button',{name:'next'}));expect(screen.getByRole('status')).toHaveTextContent('missing');
  expect(mock).toHaveBeenCalledTimes(1);
});

describe('CTX-001 cancelled review navigation intent', () => {
  const a = {kind:'stored' as const,stored:{key:{jobId:'producer',taskOrdinal:1,name:'a.md'}}};
  const b = {kind:'stored' as const,stored:{key:{jobId:'producer',taskOrdinal:2,name:'b.md'}}};
  it.each(['pending', 'loaded'] as const)('MAL-026,032 clears intent when the %s target disappears', async phase => {
    let finishB!: (response: Response) => void;
    let finishFallback!: (response: Response) => void;
    let signalB!: AbortSignal;
    let aLoads = 0;
    const scroll = vi.fn();
    const previousScroll = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView');
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {configurable:true,value:scroll});
    const mock = vi.fn((url:string, options:RequestInit) => {
      if (url.includes('document_id=b')) {
        signalB = options.signal as AbortSignal;
        return new Promise<Response>(resolve => { finishB = resolve; });
      }
      if (++aLoads === 1) return Promise.resolve(new Response('# Initial A\n[next](b.md#only-b)'));
      return new Promise<Response>(resolve => { finishFallback = resolve; });
    });
    vi.stubGlobal('fetch', mock);
    const reader = (documents: Record<string, ReviewDocument>) => <>
      <input aria-label="Response draft" defaultValue="Keep my response" />
      <ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={documents}/>
    </>;
    try {
      const {rerender} = render(reader({a,b}));
      fireEvent.click(await screen.findByRole('button', {name:'next'}));
      await screen.findByLabelText('Loading document');
      if (phase === 'loaded') {
        await act(async () => finishB(new Response('# Only B')));
        await screen.findByRole('heading', {name:'Only B'});
        expect(scroll).toHaveBeenCalledTimes(1);
      }
      rerender(reader({a}));
      expect(signalB.aborted).toBe(true);
      const response = screen.getByRole('textbox', {name:'Response draft'});
      response.focus();
      scroll.mockClear();
      // The loaded variant deliberately shares B's heading ID; stale intent must not scroll to it.
      await act(async () => finishFallback(new Response('# Fallback A\n'+(phase === 'loaded' ? '## Only B' : ''))));
      await screen.findByRole('heading', {name:'Fallback A'});
      await act(async () => finishB(new Response('# Stale B')));
      expect(screen.queryByRole('heading', {name:'Stale B'})).toBeNull();
      expect(screen.queryByRole('status')).toBeNull();
      expect(scroll).not.toHaveBeenCalled();
      expect(response).toHaveFocus();
      expect(response).toHaveValue('Keep my response');
      // Reattaching B must not resurrect the cancelled selection or its fragment/focus work.
      rerender(reader({a,b}));
      expect(screen.getByRole('heading', {name:'Fallback A'})).toBeInTheDocument();
      expect(mock).toHaveBeenCalledTimes(3);
      expect(response).toHaveFocus();
    } finally {
      if (previousScroll) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', previousScroll);
      else delete (HTMLElement.prototype as {scrollIntoView?:unknown}).scrollIntoView;
    }
  });
});

it('CTX-001 MAL-026,028,033 preserves valid target focus and fragment through a retry', async () => {
  const doc = (name:string) => ({kind:'stored' as const,stored:{key:{jobId:'producer',taskOrdinal:1,name}}});
  let recovered = false;
  const mock = vi.fn(async (url:string) => url.includes('document_id=a')
    ? new Response('[next](b.md#only-b)')
    : new Response(recovered ? '# Only B' : 'Unavailable', {status:recovered ? 200 : 503}));
  const scroll = vi.fn();
  const previousScroll = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView');
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {configurable:true,value:scroll});
  vi.stubGlobal('fetch', mock);
  try {
    render(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={{a:doc('a.md'),b:doc('b.md')}}/>);
    fireEvent.click(await screen.findByRole('button', {name:'next'}));
    await screen.findByText(/Could not load/);
    recovered = true;
    fireEvent.click(screen.getByRole('button', {name:'Retry'}));
    await screen.findByRole('heading', {name:'Only B'});
    expect(screen.getByLabelText('Document preview')).toHaveFocus();
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('status')).toBeNull();
    expect(mock.mock.calls.at(-1)?.[0]).toContain('document_id=b');
  } finally {
    if (previousScroll) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', previousScroll);
    else delete (HTMLElement.prototype as {scrollIntoView?:unknown}).scrollIntoView;
  }
});

describe('RDM metadata catalog', () => {
  const doc = (name: string) => ({kind:'stored' as const, name, stored:{key:{jobId:'child',taskOrdinal:2,name,sizeBytes:12}}});
  const docs = { z:doc('z.md'), b:doc('b.md'), a:doc('a.md'), '雪/target':doc('target.md') };
  it('RDM-001–009,019–025 orders and opens overview with descriptive exact-ID source links', async () => {
    const fetcher=vi.fn(async()=>new Response('# Contents'));vi.stubGlobal('fetch',fetcher);
    const metadata = { z:{title:'Overview',description:'Read this first',role:'primary' as const,order:10,source_document_ids:['雪/target','missing','b.md','z','雪/target']}, b:{title:'Supporting notes',role:'supporting' as const,order:2}, a:{order:0}, '雪/target':{title:'Diagnostic report',role:'diagnostic' as const,order:2,source_document_ids:['z']}, orphan:{title:'Never attached',order:0} };
    render(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={docs} documentMetadata={metadata} overviewDocumentId="z"/>);
    await screen.findByRole('heading',{name:'Contents'});
    const nav=screen.getByRole('navigation',{name:'Review documents'});
    expect([...nav.querySelectorAll('button')].map(b=>b.textContent)).toEqual(['a.mda','Supporting notesSupportingb · b.md','Diagnostic reportDiagnostic雪/target · target.md','OverviewPrimaryz · z.mdRead this first']);
    expect(fetcher.mock.calls).toHaveLength(1);
    expect(fetcher.mock.calls[0][0]).toContain('document_id=z');
    expect(screen.queryByText('Never attached')).toBeNull();
    const source=screen.getByRole('link',{name:'Diagnostic report · target.md (雪/target)'});
    expect(source.getAttribute('href')).toContain('document_id=%E9%9B%AA%2Ftarget');
    expect(screen.getByText('missing (unavailable)')).toBeInTheDocument();
    expect(screen.getByText('b.md (unavailable)')).toBeInTheDocument();
    expect(screen.getAllByRole('link',{name:'Diagnostic report · target.md (雪/target)'})).toHaveLength(1);
    const modified = new MouseEvent('click', {bubbles:true,cancelable:true,ctrlKey:true});
    source.addEventListener('click', event => { expect(event.defaultPrevented).toBe(false); event.preventDefault(); }, {once:true});
    source.dispatchEvent(modified);expect(fetcher).toHaveBeenCalledTimes(1);
    fireEvent.click(source);
    await waitFor(()=>expect(fetcher).toHaveBeenCalledTimes(2));
    expect(screen.getByRole('button',{name:/Diagnostic report Diagnostic 雪/})).toHaveAttribute('aria-current','true');
    await waitFor(()=>expect(screen.getByLabelText('Document preview')).toHaveFocus());
  });
  it('RDM-004,007,010,011,026,027 handles absent overview, invalid metadata, stable selection and removal', async () => {
    vi.stubGlobal('fetch',vi.fn(async()=>new Response('# Loaded')));
    const props={projectId:'t',jobId:'j',requestId:'r'};
    const bad={a:{title:'  ',order:-1},b:{order:1.5},z:{order:1_000_001},'雪/target':{order:0,title:'<script>bad()</script>',description:'**plain**',role:'unknown'}};
    const {rerender,container}=render(<ReviewDocuments {...props} documents={docs} documentMetadata={bad as never} overviewDocumentId="missing"/>);
    await screen.findByRole('heading',{name:'Loaded'});
    expect(container.querySelector('script')).toBeNull();
    expect(screen.getAllByText('**plain**')).toHaveLength(2);
    const nav=screen.getByRole('navigation');
    expect(nav.querySelector('button')).toHaveAttribute('aria-current','true');
    fireEvent.click(screen.getByRole('button',{name:'b.md b'}));
    rerender(<ReviewDocuments {...props} documents={docs} documentMetadata={{a:{order:0}}} overviewDocumentId="a"/>);
    expect(screen.getByRole('button',{name:'b.md b'})).toHaveAttribute('aria-current','true');
    const {b:removed,...remaining}=docs;
    rerender(<ReviewDocuments {...props} documents={remaining} overviewDocumentId="z"/>);
    expect(screen.getByRole('button',{name:'z.md z'})).toHaveAttribute('aria-current','true');
    rerender(<ReviewDocuments {...props} documents={docs} overviewDocumentId="a"/>);
    expect(screen.getByRole('button',{name:'z.md z'})).toHaveAttribute('aria-current','true');
  });
  it.each([undefined, '', 'missing'])('RDM-004 falls back to first ordered document for overview %s', async overview => {
    const fetcher=vi.fn(async()=>new Response('# Loaded'));vi.stubGlobal('fetch',fetcher);
    render(<ReviewDocuments projectId="t" jobId="j" requestId="r" documents={docs} overviewDocumentId={overview} documentMetadata={{b:{order:0}}}/>);
    await screen.findByRole('heading',{name:'Loaded'});
    expect(fetcher.mock.calls[0][0]).toContain('document_id=b');
  });
  it.each([null,'0',-1,1.1,1_000_001])('RDM-010 treats invalid order %s as absent', async order => {
    vi.stubGlobal('fetch',vi.fn(async()=>new Response('# Loaded')));
    render(<ReviewDocuments projectId="t" jobId="j" requestId="r" documents={docs} documentMetadata={{z:{order:order as never},b:{order:1_000_000}}}/>);
    await screen.findByRole('heading',{name:'Loaded'});
    expect(screen.getByRole('navigation').querySelector('button')).toHaveTextContent('b.md');
  });
});

it('RDM-007,008,019,024 preserves fallback labels and disables sources after membership changes', async () => {
  const doc=(name:string)=>({kind:'stored' as const,stored:{key:{jobId:'p',taskOrdinal:1,name,sizeBytes:1}}});
  const docs={a:doc('same.md'),b:doc('same.md'),c:doc('')};
  const fetcher=vi.fn(async()=>new Response('# Loaded'));vi.stubGlobal('fetch',fetcher);
  const props={projectId:'t',jobId:'j',requestId:'r',documentMetadata:{a:{source_document_ids:['b','c']}}};
  const {rerender}=render(<ReviewDocuments {...props} documents={docs}/>);
  await screen.findByRole('heading',{name:'Loaded'});
  expect(screen.getByRole('button',{name:'same.md a'})).toBeInTheDocument();
  expect(screen.getByRole('button',{name:'same.md b'})).toBeInTheDocument();
  expect(screen.getByRole('button',{name:'c c'})).toBeInTheDocument();
  expect(screen.getByRole('link',{name:'same.md (b)'})).toBeInTheDocument();
  expect(screen.getByRole('link',{name:'c (c)'})).toBeInTheDocument();
  rerender(<ReviewDocuments {...props} documents={{a:docs.a,c:docs.c}}/>);
  expect(screen.queryByRole('link',{name:'same.md (b)'})).toBeNull();
  expect(screen.getByText('b (unavailable)')).toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it.each([false,true])('RDM-014,016 recovers inline catalog focus when the selection disappears, empty=%s', async empty => {
  const previous=window.innerWidth;Object.defineProperty(window,'innerWidth',{value:375,configurable:true});
  vi.stubGlobal('fetch',vi.fn(async()=>new Response('# Loaded')));
  const doc=(name:string)=>({kind:'stored' as const,stored:{key:{jobId:'p',taskOrdinal:1,name,sizeBytes:1}}});
  const props={projectId:'t',jobId:'j',requestId:'r'};
  try {
    const {rerender}=render(<ReviewDocuments {...props} documents={{a:doc('a.md'),b:doc('b.md')}} overviewDocumentId="b"/>);
    fireEvent.click(screen.getByRole('button',{name:'Read b.md'}));await screen.findByRole('heading',{name:'Loaded'});
    rerender(<ReviewDocuments {...props} documents={empty ? {} : {a:doc('a.md')}}/>);
    await waitFor(()=>expect(document.querySelector('.review-document-catalog')).toHaveFocus());
  } finally {Object.defineProperty(window,'innerWidth',{value:previous,configurable:true});}
});


describe('RGI attached nested galleries', () => {
  const doc = (path: string, name?: string): ReviewDocument => ({kind:'stored', name, stored:{key:{jobId:'producer',taskOrdinal:7,name:path}}});
  const filenames = ['review-collapsed-desktop.png','review-confirm-desktop.png','review-confirm-320.png','review-keyboard-geometry-320.png','review-200.png','review-error-320.png'];
  const png = Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII='), c => c.charCodeAt(0));
  function fixture(source: string) {
    const OriginalURL = URL;
    vi.stubGlobal('URL', class extends OriginalURL { static createObjectURL = vi.fn(()=>'blob:fixture'); static revokeObjectURL = vi.fn(); });
    const fetcher = vi.fn(async (url: string) => new Response(new URL(url,'http://local').searchParams.get('document_id') === 'gallery' ? source : png));
    vi.stubGlobal('fetch', fetcher);
    return fetcher;
  }
  const props = {projectId:'tenant',jobId:'job',requestId:'request',overviewDocumentId:'gallery'};
  it('RGI-001–004,009 resolves siblings, normalized paths and exact aliases with canonical precedence', async () => {
    const refs = [...filenames,'./review-200.png','nested/../review-200.png','../root.png','a%20%E9%9B%AA.png','id/雪','Display.png','captures/review-200.png','review-200.png?ignored=1#fragment'];
    const fetcher = fixture(refs.map((ref,i)=>`![image${i}](${ref})`).join('\n'));
    const docs: Record<string,ReviewDocument> = {gallery:doc('captures/gallery.md','Different title.md'),root:doc('root.png'),space:doc('captures/a 雪.png'),'id/雪':doc('elsewhere/id.png','Display.png'),conflict:doc('elsewhere/conflict.png','review-200.png')};
    filenames.forEach((name,i)=>{docs[`image/${i}`]=doc(`captures/${name}`);});
    render(<ReviewDocuments {...props} documents={docs}/>);
    await waitFor(()=>expect(screen.getAllByRole('img').filter(el=>el.tagName==='IMG')).toHaveLength(refs.length));
    expect(fetcher).toHaveBeenCalledTimes(refs.length+1);
    const ids = fetcher.mock.calls.slice(1).map(([url])=>new URL(url,'http://local').searchParams.get('document_id'));
    expect(ids).toEqual(expect.arrayContaining(['image/0','image/1','image/2','image/3','image/4','image/5','root','space','id/雪']));
    expect(ids).not.toContain('conflict');
    for (const [url] of fetcher.mock.calls) {
      expect(url).toContain('/projects/tenant/reviews/job/documents?');
      expect(url).toContain('request_id=request');
      expect(url).not.toMatch(/producer|artifacts|ignored|fragment/);
    }
    expect(fetcher.mock.calls.some(([url])=>url.includes('document_id=id%2F%E9%9B%AA'))).toBe(true);
  });
  it('RGI-005–008 blocks collisions, missing, unsafe references and alias bypasses without fetching', async () => {
    const refs = ['duplicate.png','alias.png','missing.png','basename.png','https://blocked.invalid/x','//blocked.invalid/x','/absolute.png','data:image/png;base64,AA','javascript:alert','../../escape.png','bad%FF','a%2fb','a%5Cb','a\\b','#fragment'];
    const fetcher = fixture(refs.map((ref,i)=>`![blocked${i}](${ref})`).join('\n'));
    const docs: Record<string,ReviewDocument> = {gallery:doc('captures/gallery.md'),a:doc('captures/duplicate.png'),b:doc('captures/duplicate.png'),c:doc('elsewhere/c.png','alias.png'),d:doc('elsewhere/d.png','alias.png'),e:doc('elsewhere/basename.png'),uniqueAlias:doc('elsewhere/alias.png','duplicate.png')};
    refs.slice(4).forEach((ref,i)=>{docs[`unsafe${i}`]=doc(`elsewhere/unsafe${i}.png`,ref);});
    render(<ReviewDocuments {...props} documents={docs}/>);
    await waitFor(()=>expect(screen.getAllByText(/Blocked external or ambiguous image/).map(el=>el.textContent)).toEqual(refs.map((_,i)=>`Blocked external or ambiguous image: blocked${i}`)));
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('RGI-010 uses the selected stored directory and current attachment membership', async () => {
    const fetcher = fixture('![sibling](image.png)');
    const docs = {gallery:doc('captures/gallery.md','First.md'),second:doc('other/gallery.md','Second.md'),one:doc('captures/image.png'),two:doc('other/image.png')};
    // Both Markdown documents share the same gallery source.
    fetcher.mockImplementation(async(url:string)=>new Response(['gallery','second'].includes(new URL(url,'http://local').searchParams.get('document_id')!) ? '![sibling](image.png)' : png));
    const {rerender}=render(<ReviewDocuments {...props} documents={docs}/>);
    await waitFor(()=>expect(fetcher.mock.calls.some(([url])=>url.includes('document_id=one'))).toBe(true));
    fireEvent.click(screen.getByRole('button',{name:'Second.md second'}));
    await waitFor(()=>expect(fetcher.mock.calls.some(([url])=>url.includes('document_id=two'))).toBe(true));
    rerender(<ReviewDocuments {...props} documents={{gallery:docs.gallery,second:docs.second,one:docs.one}}/>);
    await screen.findByText(/Blocked external or ambiguous image/);
    const count=fetcher.mock.calls.length;
    rerender(<ReviewDocuments {...props} documents={{gallery:docs.gallery,second:docs.second,one:docs.one}}/>);
    expect(fetcher).toHaveBeenCalledTimes(count);
  });
});


it.each(['fetch-error','not-raster'])('RGI-011 retains inline attachment %s handling', async mode => {
  const doc = (name:string): ReviewDocument => ({kind:'stored',stored:{key:{jobId:'producer',taskOrdinal:1,name}}});
  const fetcher=vi.fn(async(url:string)=>url.includes('document_id=gallery')
    ? new Response('![image](image.png)')
    : new Response('plain text',{status:mode==='fetch-error'?503:200}));
  vi.stubGlobal('fetch',fetcher);
  render(<ReviewDocuments projectId="t" jobId="j" requestId="r" overviewDocumentId="gallery" documents={{gallery:doc('captures/gallery.md'),image:doc('captures/image.png')}}/>);
  await screen.findByText(mode==='fetch-error'?/Could not load/:/Attachment is not a raster image/);
  expect(fetcher).toHaveBeenCalledTimes(2);
  expect(document.querySelector('.review-markdown img')).toBeNull();
});

describe('CC historical document source', () => {
 const source = (identity='occurrence:snapshot') => ({identity,url:(id:string)=>`/history/artifact?entry_id=old&artifact_id=${id}&snapshot_token=${identity}`});
 it('CC-011 CC-013 CC-014 uses original filenames, metadata and scoped links despite text/plain', async () => {
  vi.stubGlobal('fetch',vi.fn(async (url:string) => new Response(url.includes('artifact_id=design') ? '# Original\n[next](data.json)' : '{"original":true}',{headers:{'Content-Type':'text/plain'}})));
  render(<ReviewDocuments projectId="t" jobId="j" requestId="r" source={source()} documents={{...documents,data:{kind:'stored',name:'data.json',stored:{key:{jobId:'p',taskOrdinal:9,name:'data.json',sizeBytes:17}}}}} overviewDocumentId="design" documentMetadata={{design:{order:0,title:'Saved overview',role:'primary',description:'Original description',source_document_ids:['data']}}} />);
  await screen.findByRole('heading',{name:'Original'});
  const link=screen.getByRole('link',{name:'next'});
  expect(link).toHaveAttribute('href',source().url('data'));
  fireEvent.click(link,{ctrlKey:true});expect(screen.getByRole('heading',{name:'Original'})).toBeInTheDocument();
  fireEvent.click(link);await screen.findByLabelText('Formatted JSON');
  expect(screen.getByRole('link',{name:'Download',exact:true})).toHaveAttribute('href',source().url('data'));
  expect(vi.mocked(fetch).mock.calls.every(([url])=>String(url).startsWith('/history/artifact?'))).toBe(true);
 });
 it('CC-018 aborts and clears previews on snapshot identity changes', async () => {
  let release!: (v:Response)=>void;
  const fetchMock=vi.fn().mockImplementationOnce(()=>new Promise(r=>{release=r;})).mockResolvedValueOnce(new Response('# New snapshot'));
  vi.stubGlobal('fetch',fetchMock);
  const view=render(<ReviewDocuments projectId="t" jobId="j" requestId="r" documents={documents} source={source('old')} />);
  view.rerender(<ReviewDocuments projectId="t" jobId="j" requestId="r" documents={documents} source={source('new')} />);
  await screen.findByRole('heading',{name:'New snapshot'});
  expect(fetchMock.mock.calls[0][1].signal.aborted).toBe(true);
  await act(async()=>release(new Response('# Obsolete snapshot')));
  expect(screen.queryByRole('heading',{name:'Obsolete snapshot'})).toBeNull();
 });
});
