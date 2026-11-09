import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, cleanup } from '@testing-library/react';
import DocumentPreview from './DocumentPreview';
import ReviewDocuments from '../ReviewDocuments';
import SourceView from './SourceView';
import ImageView from './ImageView';
import { type DocumentData, classify, TEXT_LIMIT } from './document';
const props = { name: 'sample', dual: false, setDual: vi.fn() };
const data = (text: string,name='sample'): DocumentData => { const bytes = new TextEncoder().encode(text); return { text,bytes,format:classify(name,bytes),truncated:false }; };
const docs = { design: {kind:'stored' as const,name:'design.md',stored:{key:{jobId:'producer',taskOrdinal:1,name:'design.md',sizeBytes:10}}},log:{kind:'stored' as const,name:'tests.log',stored:{key:{jobId:'producer',taskOrdinal:1,name:'tests.log',sizeBytes:10}}} };
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
describe('V04,V06-V10,J02-J04,D01,D05,D06 visible viewers', () => {
  it('HSV-006,016 isolates HTML and preserves searchable original source', () => {
    const OriginalURL = URL;
    vi.stubGlobal('URL', class extends OriginalURL { static createObjectURL = vi.fn(() => 'blob:test'); static revokeObjectURL = vi.fn(); });
    const text='<p style="color:red">Hello</p><script>alert(1)</script><meta http-equiv="refresh" content="0;url=https://evil.test"><svg onload="alert(1)"/>';
    const {container}=render(<DocumentPreview {...props} data={data(text,'mock.html')} />);
    const frame=container.querySelector('iframe')!;
    expect(frame).toHaveAttribute('sandbox','');
    expect(frame).toHaveAttribute('referrerpolicy','no-referrer');
    expect(frame.srcdoc).toContain('Content-Security-Policy');
    expect(frame.srcdoc).not.toMatch(/<script|onload|http-equiv="refresh"/);
    expect(screen.getByText(/Static preview:/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('radio',{name:'Source',exact:true}));
    expect(screen.getByLabelText('Document source').textContent).toBe(text);
    fireEvent.change(screen.getByLabelText('Search document'),{target:{value:'alert'}});
    expect(screen.getByText('1 / 2')).toBeInTheDocument();
    expect(container.querySelector('iframe,script,svg,meta,a')).toBeNull();
  });
  it('exposes text override for binary without removing original content', () => {
    render(<DocumentPreview {...props} data={{...data('readable'),format:'binary'}} />);
    expect(screen.getByLabelText('Binary bytes')).toHaveTextContent('00000000');
    fireEvent.click(screen.getByRole('button',{name:'View as text'})); expect(screen.getByLabelText('Document source')).toHaveTextContent('readable');
  });
  it.each([['notes.pack','Review notes\n'],['unknown','PACK review notes\n'],['unknown','BM review notes\n']])('recovers false detection for %s containing %s', (name,text) => {
    const document = data(text,name);
    expect(['container','image']).toContain(document.format);
    const {container} = render(<DocumentPreview {...props} name={name} data={document} />);
    fireEvent.click(screen.getByRole('button',{name:'View as text'}));
    expect(screen.getByLabelText('Document source').textContent).toBe(text);
    expect(container.querySelector('img,iframe,script')).toBeNull();
    fireEvent.click(screen.getByRole('button',{name:'Detect format'}));
    expect(screen.queryByLabelText('Document source')).toBeNull();
  });
  it('bounds image text overrides and keeps markup inert', () => {
    const text = 'BM<script>alert(1)</script>'+'a'.repeat(TEXT_LIMIT)+'OUTSIDE_LIMIT';
    const {container} = render(<DocumentPreview {...props} data={data(text)} />);
    fireEvent.click(screen.getByRole('button',{name:'View as text'}));
    expect(screen.getByText(/Preview truncated/)).toBeInTheDocument();
    expect(screen.getByLabelText('Document source').textContent).toContain('<script>');
    expect(container.querySelector('script,img,iframe')).toBeNull();
    fireEvent.change(screen.getByLabelText('Search document'),{target:{value:'OUTSIDE_LIMIT'}});
    expect(screen.getByText('0 matches')).toBeInTheDocument();
    expect(screen.getByLabelText('Document source').textContent!.length).toBeLessThanOrEqual(24000);
  });
  it.each([[true,false],[false,true],[true,true]])('renders side-owned missing-newline markers old=%s/new=%s', (oldMarker,newMarker) => {
    const marker = '\\ No newline at end of file';
    const text = ['--- a/file','+++ b/file','@@ -1 +1 @@','-old',...(oldMarker ? [marker] : []),'+new',...(newMarker ? [marker] : [])].join('\n');
    render(<DocumentPreview {...props} dual data={data(text,'file.patch')} />);
    const rows = screen.getByLabelText('Side by side diff').querySelectorAll('tbody tr');
    const replacement = Array.from(rows).find(row => row.textContent?.includes('-old'))!;
    expect(Array.from(replacement.children).map(c => c.textContent)).toEqual(['1','-old','1','+new']);
    expect(Array.from(replacement.nextElementSibling!.children).map(c => c.textContent)).toEqual(['',oldMarker ? marker : '','',newMarker ? marker : '']);
  });
  it('searches and wraps source while preserving text, including long single lines', () => {
    const text='needle '+ 'a'.repeat(50000)+' needle';
    render(<SourceView text={text} />);
    fireEvent.change(screen.getByLabelText('Search document'),{target:{value:'needle'}});
    expect(screen.getByText('1 / 2')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Next match')); expect(screen.getByText('2 / 2')).toBeInTheDocument();
    expect(screen.getByLabelText('Document source').textContent).toContain('needle');
    fireEvent.click(screen.getByText('Previous match')); expect(screen.getByText('1 / 2')).toBeInTheDocument();
    fireEvent.click(screen.getByText('Wrap lines')); expect(screen.getByLabelText('Document source').style.whiteSpace).toBe('pre-wrap');
  });
  it('formats JSON without loss and preserves source after switching modes', () => {
    const text='{"key":9007199254740993,"key":1e99}';
    render(<DocumentPreview {...props} data={data(text,'sample.json')} />);
    expect(screen.getByLabelText('Formatted JSON')).toHaveTextContent('9007199254740993');
    fireEvent.click(screen.getByRole('radio',{name:'Source',exact:true}));expect(screen.getByLabelText('Document source').textContent).toBe(text);
  });
  it('keeps malformed JSON and JSONL readable', () => {
    const {rerender}=render(<DocumentPreview {...props} data={data('{bad','sample.json')} />);
    expect(screen.getByText(/JSON preview unavailable/)).toBeInTheDocument();expect(screen.getByLabelText('Document source').textContent).toBe('{bad');
    rerender(<DocumentPreview {...props} data={data('{}\nwrong\n{}','sample.jsonl')} />);expect(screen.getByText('Line 2: invalid JSON')).toBeInTheDocument();
  });
  it('renders unified, side-by-side and binary patch states', () => {
    const d=data('diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-old\n+new\n','sample.patch');
    const {rerender}=render(<DocumentPreview {...props} data={d} />);expect(screen.getByLabelText('Unified diff')).toHaveTextContent('-old');
    rerender(<DocumentPreview {...props} dual data={d} />);expect(screen.getByLabelText('Side by side diff')).toHaveTextContent('+new');
    rerender(<DocumentPreview {...props} data={data('diff --git a/a b/a\nBinary files a/a and b/a differ\n','sample.diff')} />);expect(screen.getByText(/Binary patch metadata only/)).toBeInTheDocument();
    rerender(<DocumentPreview {...props} data={data('@@@ invalid','sample.diff')} />);expect(screen.getByLabelText('Document source')).toHaveTextContent('@@@ invalid');
  });
  it('renders logs and control sequences as visible inert source', () => {
    render(<DocumentPreview {...props} data={data('!!python/object\n\x1b[31mred\n<script/>','tests.log')} />);
    expect(screen.getByLabelText('Document source').textContent).toBe('!!python/object\n\\x1b[31mred\n<script/>');
  });
  it('blocks external Markdown images without fetching', () => {
    const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);
    render(<DocumentPreview {...props} data={data('![outside](https://evil.test/x.png)','sample.md')} />);
    expect(screen.getByText(/Blocked external/)).toBeInTheDocument();
  });
});
describe('L01-L03,L12 loader integration', () => {
  it.each([['notes.pack','Review notes\n'],['unknown','PACK review notes\n'],['image','BM review notes\n']])('retains original download and loaded bytes when overriding %s', async (name,text) => {
    const fetcher = vi.fn(async () => new Response(text)); vi.stubGlobal('fetch',fetcher);
    const documents = { notes: {...docs.log,name,stored:{key:{...docs.log.stored.key,name}}} };
    render(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={documents} />);
    const override = await screen.findByRole('button',{name:'View as text'});
    const download = screen.getByRole('link',{name:'Download'});
    const href = download.getAttribute('href');
    fireEvent.click(override);
    expect(screen.getByLabelText('Document source').textContent).toBe(text);
    expect(download).toHaveAttribute('href',href);
    expect(download).toHaveAttribute('download',name);
    fireEvent.click(screen.getByRole('button',{name:'Detect format'}));
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('loads only selected document and does not refetch display modes', async () => {
    const fetcher=vi.fn(async()=>new Response('# Current')); vi.stubGlobal('fetch',fetcher);
    render(<ReviewDocuments projectId="tenant" jobId="job" requestId="r" documents={docs} />);
    await screen.findByRole('heading',{name:'Current'});expect(fetcher).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('radio',{name:'Markdown',exact:true}));expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('cancels on unmount and never exposes stale tenant/job content', async () => {
    const signals:AbortSignal[]=[];let old!:(r:Response)=>void;
    const fetcher=vi.fn((_url:string,options:RequestInit)=>{signals.push(options.signal as AbortSignal);return signals.length===1?new Promise<Response>(resolve=>{old=resolve;}):Promise.resolve(new Response('# New'));});
    vi.stubGlobal('fetch',fetcher);
    const {rerender,unmount}=render(<ReviewDocuments projectId="old" jobId="job" requestId="r" documents={docs} />);
    rerender(<ReviewDocuments projectId="new" jobId="job2" requestId="r" documents={docs} />);await screen.findByRole('heading',{name:'New'});
    old(new Response('# Stale'));await waitFor(()=>expect(screen.queryByRole('heading',{name:'Stale'})).toBeNull());expect(signals[0].aborted).toBe(true);
    unmount();expect(signals[1].aborted).toBe(true);
  });
});
describe('I01,I03,I05,L09 image component', () => {
  it('checks dimensions, supports zoom, handles decode failure and releases URLs', async () => {
    const bytes=new Uint8Array(32),view=new DataView(bytes.buffer);view.setUint32(16,10);view.setUint32(20,20);
    const create=vi.fn(()=> 'blob:passive'),revoke=vi.fn();const OriginalURL=URL;vi.stubGlobal('URL',class extends OriginalURL { static createObjectURL=create;static revokeObjectURL=revoke; });
    const {unmount}=render(<ImageView name="screenshot" data={{bytes,text:'',format:'image',truncated:false,mime:'image/png'}} />);
    const img=await screen.findByRole('img',{name:'screenshot'});fireEvent.click(screen.getByText('Actual size'));expect(img).toHaveAttribute('width','10');
    fireEvent.click(screen.getByText('Zoom in'));expect(img).toHaveAttribute('width','12.5');fireEvent.error(img);expect(screen.getByText(/Could not decode/)).toBeInTheDocument();
    unmount();expect(revoke).toHaveBeenCalledWith('blob:passive');
  });
});

describe('L01 compact loading and L05-L07 limited states', () => {
  it('loads compact previews inline and reuses the selected document', async () => {
    vi.stubGlobal('innerWidth',320);const fetcher=vi.fn(async()=>new Response('# Phone'));vi.stubGlobal('fetch',fetcher);
    render(<ReviewDocuments projectId="tenant" jobId="job" requestId="phone" documents={docs} />);
    const read=await screen.findByRole('button',{name:'Read design.md',exact:true});
    fireEvent.click(read);await screen.findByRole('heading',{name:'Phone'});expect(fetcher).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
  it('shows truncation and structured fallback explicitly', () => {
    render(<DocumentPreview {...props} data={{...data('{"partial":','sample.json'),truncated:true}} />);
    expect(screen.getByText(/Preview truncated/)).toBeInTheDocument();expect(screen.getByText(/Structured preview limit/)).toBeInTheDocument();
    expect(screen.getByLabelText('Document source').textContent).toBe('{"partial":');expect(screen.queryByLabelText('Formatted JSON')).toBeNull();
  });
  it('identifies an empty document', () => {
    render(<DocumentPreview {...props} data={data('','empty.txt')} />);expect(screen.getByText('Empty document')).toBeInTheDocument();
  });
});

describe('D07 file navigation', () => {
  it('selects a file section without requesting repository contents', async () => {
    const scroll=vi.fn();Object.defineProperty(HTMLElement.prototype,'scrollIntoView',{configurable:true,value:scroll});
    const patch='diff --git a/a b/a\nold mode 100644\nnew mode 100755\ndiff --git a/b b/b\nold mode 100644\nnew mode 100755\n';
    render(<DocumentPreview {...props} data={data(patch,'files.patch')} />);
    fireEvent.mouseDown(screen.getByRole('combobox',{name:'Patch file'}));
    fireEvent.click(screen.getByTitle('a/b b/b'));
    await waitFor(()=>expect(scroll).toHaveBeenCalled());
    delete (HTMLElement.prototype as {scrollIntoView?:unknown}).scrollIntoView;
  });
});

describe('L02 unambiguous identity boundaries', () => {
  it('resets viewer state for tenant/job tuples containing slashes', async () => {
    vi.stubGlobal('fetch',vi.fn(async()=>new Response('# Identity')));
    const {rerender}=render(<ReviewDocuments projectId="a/b" jobId="c" requestId="r" documents={docs} />);
    await screen.findByRole('heading',{name:'Identity'});
    fireEvent.click(screen.getByRole('radio',{name:'Markdown',exact:true}));expect(screen.getByLabelText('Markdown source')).toBeInTheDocument();
    rerender(<ReviewDocuments projectId="a" jobId="b/c" requestId="r" documents={docs} />);
    await screen.findByRole('heading',{name:'Identity'});expect(screen.queryByLabelText('Markdown source')).toBeNull();
  });
});

describe('SAV-023 attachment cancellation', () => {
  it('aborts an attachment fetch when the shared preview unmounts', async () => {
    const mock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal('fetch', mock);
    const {unmount} = render(<DocumentPreview {...props} data={data('![attachment](image.png)','notes.md')} resolveAttachment={() => ({key:'image',name:'image.png',url:'/trusted/image'})} />);
    await screen.findByText('Loading attached image…');
    const signal = (mock.mock.calls[0] as unknown as [string,{signal:AbortSignal}])[1].signal;
    unmount(); expect(signal.aborted).toBe(true);
  });
});

describe('MAL local Markdown controls', () => {
  it('MAL-023,024,025,034,035 preserves browser links and blocks unresolved local actions', () => {
    const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);
    render(<DocumentPreview {...props} data={data('[local](foo.md) [web](https://example.com) [mail](mailto:a@b) [root](/foo.md) [protocol](//example.com) [unsafe](javascript:alert(1))','bar.md')} />);
    fireEvent.click(screen.getByRole('button',{name:'local'}),{ctrlKey:true});
    expect(screen.getByRole('status')).toHaveTextContent('Artifact unavailable: missing');
    expect(fetcher).not.toHaveBeenCalled();
    for(const name of ['web','mail','root','protocol']) {
      expect(screen.getByRole('link',{name})).toHaveAttribute('target','_blank');
      expect(screen.getByRole('link',{name})).toHaveAttribute('rel','noopener noreferrer');
    }
    expect(screen.getByText('unsafe')).not.toHaveAttribute('href', expect.stringMatching(/^javascript:/));
  });
  it('MAL-027,028,029,030 generates stable headings, scopes scroll and decodes fragments once', () => {
    const scroll=vi.fn();Object.defineProperty(HTMLElement.prototype,'scrollIntoView',{configurable:true,value:scroll});
    const document=data('# Hello, World!\n# Hello, World!\n# Hello, World!-1\n[go](#hello-world-1) [missing](#hello%2520world) [selector](#%5Bx%5D)','bar.md');
    render(<DocumentPreview {...props} data={document}/>);
    expect(screen.getAllByRole('heading').map(e=>e.id)).toEqual(['hello-world','hello-world-1','hello-world-1-1']);
    fireEvent.click(screen.getByRole('button',{name:'go'}));expect(scroll).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button',{name:'missing'}));expect(screen.getByRole('status')).toHaveTextContent('hello%20world');
    fireEvent.click(screen.getByRole('button',{name:'selector'}));expect(screen.getByRole('status')).toHaveTextContent('[x]');
    expect(scroll).toHaveBeenCalledTimes(1);
    delete (HTMLElement.prototype as {scrollIntoView?:unknown}).scrollIntoView;
  });
  it('MAL-020 re-resolves links on activation against changed membership', () => {
    let available=true;const open=vi.fn();
    render(<DocumentPreview {...props} data={data('[target](foo.md)','bar.md')} navigation={{resolve:()=>available?{kind:'target',key:'foo'}:{kind:'unavailable',reason:'missing'},open}} />);
    available=false;fireEvent.click(screen.getByRole('button',{name:'target'}));
    expect(open).not.toHaveBeenCalled();expect(screen.getByRole('status')).toHaveTextContent('missing');
  });
});

it('MAL-026 focuses explicit navigation but preserves focus during background rerenders',()=>{
  const document=data('# Document','bar.md');const intent={token:1};
  const {rerender}=render(<><button>Outside</button><DocumentPreview {...props} data={document} navigationIntent={intent}/></>);
  expect(screen.getByLabelText('Document preview')).toHaveFocus();
  screen.getByRole('button',{name:'Outside'}).focus();
  rerender(<><button>Outside</button><DocumentPreview {...props} data={document} navigationIntent={intent} navigation={{resolve:()=>({kind:'unavailable',reason:'missing'}),open:()=>false}}/></>);
  expect(screen.getByRole('button',{name:'Outside'})).toHaveFocus();
});


describe('HSV-003,018,019 markup fallback and lifetime', () => {
  function urls() {
    const OriginalURL = URL;
    const create = vi.fn(() => 'blob:generated'); const revoke=vi.fn();
    vi.stubGlobal('URL', class extends OriginalURL { static createObjectURL=create; static revokeObjectURL=revoke; });
    return {create,revoke};
  }
  it.each([
    {...data('<p>partial','a.html'),truncated:true},
    data('<p>'+'x'.repeat(1048576),'a.html'),
    data('<p>\n'+'\n'.repeat(20000),'a.html'),
  ])('never renders incomplete or excessive input', d => {
    urls();const {container}=render(<DocumentPreview {...props} data={d}/>);
    expect(screen.getByText(/Rendering limit exceeded/)).toBeInTheDocument();
    expect(screen.getByLabelText('Document source').textContent!.length).toBeLessThanOrEqual(24000);
    expect(container.querySelector('iframe,img')).toBeNull();
  });
  it('keeps source when rendering is unavailable', () => {
    const {container}=render(<DocumentPreview {...props} data={data('<p>hello</p>','a.html')}/>);
    expect(screen.getByText(/Rendering unavailable/)).toBeInTheDocument();
    expect(container.querySelector('iframe')).toBeNull();
    expect(screen.getByLabelText('Document source')).toHaveTextContent('hello');
  });
  it('resets source/zoom, revokes URLs and handles decode failure and retry', () => {
    const {create,revoke}=urls();
    const a=data('<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>','a.svg');
    const b=data('<p>second</p>','b.html');
    const {rerender,unmount}=render(<DocumentPreview {...props} data={a}/>);
    expect(create).toHaveBeenCalledOnce();
    fireEvent.error(screen.getByRole('img'));
    expect(screen.getByText(/Could not display/)).toBeInTheDocument();
    expect(screen.getByLabelText('Document source')).toHaveTextContent('svg');
    fireEvent.click(screen.getByRole('button',{name:'Retry rendering'}));
    expect(screen.getByRole('img')).toBeInTheDocument();expect(revoke).toHaveBeenCalled();
    fireEvent.click(screen.getByRole('radio',{name:'Source',exact:true}));
    rerender(<DocumentPreview {...props} data={b}/>);
    expect(screen.getByTitle('Static HTML preview: sample')).toHaveAttribute('srcdoc',expect.stringContaining('second'));
    expect(screen.queryByRole('img')).toBeNull();
    unmount();expect(revoke).toHaveBeenCalledTimes(2);
  });
});

it('HSV-024 renders validated embedded HTML without allocating blob images', () => {
  const OriginalURL=URL;const revoke=vi.fn();
  vi.stubGlobal('URL',class extends OriginalURL {static createObjectURL=vi.fn(()=>'blob:embedded'); static revokeObjectURL=revoke;});
  const content='<img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=">';
  const {container,unmount}=render(<DocumentPreview {...props} data={data(content,'a.html')}/>);
  expect(container.querySelector('iframe')).toHaveAttribute('sandbox','');
  expect(container.querySelector('iframe')?.getAttribute('srcdoc')).toContain(content);
  fireEvent.click(screen.getByRole('radio',{name:'Source',exact:true}));
  expect(screen.getByLabelText('Document source').textContent).toBe(content);
  unmount();expect(revoke).not.toHaveBeenCalled();
});
