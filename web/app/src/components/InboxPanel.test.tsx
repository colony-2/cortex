import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import InboxPanel, { inboxCount, resolveInboxLink } from './InboxPanel';
import type { InboxEntry, StoryNode } from '../utils/storyTree';
const stored: InboxEntry = {binding:'docs/start.md',path:'docs/start.md',source_kind:'stored',content:'stored',source_key:{jobId:'producer',taskOrdinal:19,name:'original.md',sizeBytes:71}};
const value: InboxEntry = {binding:'docs/value.json',path:'docs/value.json',source_kind:'value',content:'value'};
const external: InboxEntry = {binding:'archive/',source_kind:'external',content:'unsupported',reason:'external_content_unavailable'};
const node: StoryNode = {id:'task',invocation_id:'durable/task/attempt:2',kind:'opStep',title:'Read inputs',path:[],status:'succeeded',attempt:2,job_attempt:3,task_ordinal:41,inbox:{status:'available',entries:[stored,value,external]}};
const props = {node,projectId:'tenant',jobId:'receiver',jobAttempt:3,retry:vi.fn()};
const bytes = (text:string) => new Response(text,{headers:{'Content-Type':'text/plain'}});
afterEach(() => {cleanup(); vi.unstubAllGlobals();});
describe('recorded inbox', () => {
 it('CX-IN-02,05,06,13 uses source identity and lazy durable value selectors', async () => {
  const fetch = vi.fn().mockImplementation((url:string) => Promise.resolve(bytes(url.includes('/inbox/value') ? '{"large":9007199254740993}\n' : '# Stored source\n[value](value.json)'))); vi.stubGlobal('fetch',fetch);
  render(<InboxPanel {...props}/>);
  expect(fetch).not.toHaveBeenCalled();
  expect(screen.getByText(/job producer · task 19 · original.md · 71 bytes/)).toBeVisible();
  expect(screen.getByText(/delivery not verified/)).toBeVisible();
  expect(screen.queryByRole('button',{name:'Download archive/'})).toBeNull();
  fireEvent.click(screen.getByRole('button',{name:'docs/start.md'}));
  expect(await screen.findByRole('heading',{name:'Stored source'})).toBeVisible();
  expect(new URL(fetch.mock.calls[0][0], 'http://test').pathname).toBe('/api/projects/tenant/jobs/producer/tasks/19/artifacts/original.md');
  fireEvent.click(screen.getByRole('button',{name:'value',exact:true}));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  const url = new URL(fetch.mock.calls[1][0],'http://test');
  expect(url.pathname).toBe('/api/projects/tenant/jobs/receiver/inbox/value');
  expect(Object.fromEntries(url.searchParams)).toEqual({job_attempt:'3',invocation_id:node.invocation_id,task_ordinal:'41',task_attempt:'2',binding:value.binding});
  expect(await screen.findByText(/9007199254740993/)).toBeVisible();
 });
 it('CX-IN-03,04 distinguishes empty, missing, unsupported and nonaggregating containers', () => {
  const {rerender} = render(<InboxPanel {...props} node={{...node,inbox:{status:'available',entries:null}}}/>);
  expect(inboxCount({...node,inbox:{status:'available',entries:null}})).toBe(' (0)');
  expect(screen.getByText('No recorded inbox artifacts for this step.')).toBeVisible();
  for (const reason of ['history_missing','pending_request','unsupported_node','history_malformed']) {
   rerender(<InboxPanel {...props} node={{...node,inbox:{status:'unavailable',reason,entries:[]}}}/>);
   expect(screen.getByText(reason)).toBeVisible(); expect(inboxCount({...node,inbox:{status:'unavailable',reason,entries:[]}})).toBe('');
  }
  rerender(<InboxPanel {...props} node={{...node,kind:'sequence',inbox:undefined,children:[node]}}/>);
  expect(screen.getByText('Inbox does not apply to this node')).toBeVisible();
  expect(screen.queryByText('docs/start.md')).toBeNull();
 });
 it('CX-IN-09 keeps metadata and retries failed content without stale bytes',async () => {
  const fetch = vi.fn().mockResolvedValueOnce(new Response('',{status:403})).mockImplementation(() => Promise.resolve(bytes('Recovered content'))); vi.stubGlobal('fetch',fetch);
  render(<InboxPanel {...props}/>); fireEvent.click(screen.getByRole('button',{name:'docs/start.md'}));
  expect(await screen.findByText(/Could not load this document/)).toBeVisible();
  expect(screen.getByText(/job producer/)).toBeVisible();
  fireEvent.click(screen.getByRole('button',{name:'Retry',exact:true}));
  expect(await screen.findByText('Recovered content')).toBeVisible();
 });
 it.each([['stored',stored],['value',value]] as const)('CX-IN-08,10,15 drops late %s responses when snapshot identity changes',async (_kind, entry) => {
  let resolve!: (r:Response)=>void;
  const fetch = vi.fn().mockImplementation(() => new Promise<Response>(r=>{resolve=r;})); vi.stubGlobal('fetch',fetch);
  const {rerender} = render(<InboxPanel key="old" {...props}/>); fireEvent.click(screen.getByRole('button',{name:entry.path!}));
  await waitFor(()=>expect(fetch).toHaveBeenCalledTimes(1));
  const signal = fetch.mock.calls[0][1].signal;
  rerender(<InboxPanel key="new-attempt" {...props} node={{...node,attempt:3,inbox:{status:'available',entries:[value]}}}/>);
  expect(signal.aborted).toBe(true);
  resolve(bytes('OLD PRIVATE CONTENT'));
  await waitFor(()=>expect(screen.queryByText('OLD PRIVATE CONTENT')).toBeNull());
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(screen.getByRole('button',{name:'docs/value.json'})).toBeVisible();
 });
 it('CX-IN-07,17 resolves destinations only, preserving aliases and ambiguity boundaries', () => {
  expect(resolveInboxLink('value.json',stored,[stored,value])).toEqual({kind:'target',key:value.binding,fragment:undefined});
  expect(resolveInboxLink('original.md',stored,[stored,value]).kind).toBe('unavailable');
  expect(resolveInboxLink('../../escape',stored,[stored,value]).kind).toBe('unavailable');
  expect(resolveInboxLink('value.json',stored,[value,{...value,binding:'alias'}])).toEqual({kind:'unavailable',reason:'ambiguous'});
  expect(resolveInboxLink('../archive/member',stored,[external]).kind).toBe('unavailable');
  expect(resolveInboxLink('value.json',stored,[{...external,path:value.path}]).kind).toBe('unavailable');
  expect(resolveInboxLink('original.md',stored,[{...stored,binding:'docs/',path:'docs/original.md'}])).toEqual({kind:'target',key:'docs/',fragment:undefined});
 });
});

it.each([403,404,410])('CX-IN-09 value content HTTP %s preserves metadata and offers retry',async status => {
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response('',{status})));
  render(<InboxPanel {...props}/>);fireEvent.click(screen.getByRole('button',{name:'docs/value.json'}));
  expect(await screen.findByText(/Could not load this document/)).toBeVisible();
  expect(screen.getByRole('button',{name:'Retry',exact:true})).toBeVisible();
  expect(screen.getByText('Binding: docs/value.json')).toBeVisible();
});
it('CX-IN-09,13 downloads exact value bytes lazily and preserves metadata on failures',async () => {
  const fetch = vi.fn().mockResolvedValueOnce(new Response('',{status:410})).mockResolvedValueOnce(new Response('{"large":9007199254740993}\n'));
  vi.stubGlobal('fetch',fetch);
  const create=vi.fn().mockReturnValue('blob:recorded');
  vi.stubGlobal('URL',Object.assign(URL,{createObjectURL:create,revokeObjectURL:vi.fn()}));
  const click=vi.spyOn(HTMLAnchorElement.prototype,'click').mockImplementation(()=>{});
  render(<InboxPanel {...props}/>);
  fireEvent.click(screen.getByRole('button',{name:'Download docs/value.json'}));
  expect(await screen.findByText(/Could not download recorded inbox content/)).toBeVisible();
  fireEvent.click(screen.getByRole('button',{name:'Download docs/value.json'}));
  await waitFor(()=>expect(create).toHaveBeenCalledTimes(1));expect(click).toHaveBeenCalledOnce();
  const blob=create.mock.calls[0][0] as Blob;expect(await new Promise<string>(resolve => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.readAsText(blob); })).toBe('{"large":9007199254740993}\n');
  expect(new URL(fetch.mock.calls[0][0]).searchParams.get('binding')).toBe(value.binding);
  expect(screen.getByText('Binding: docs/value.json')).toBeVisible();
  click.mockRestore();
});

it('CX-IN-15 navigates from recorded value Markdown to a stored destination',async () => {
 const markdown = {...value,binding:'docs/value.md',path:'docs/value.md'};
 vi.stubGlobal('fetch',vi.fn().mockImplementation((url:string)=>Promise.resolve(bytes(url.includes('/inbox/value') ? '# Recorded value\n[source](start.md)' : '# Stored destination'))));
 render(<InboxPanel {...props} node={{...node,inbox:{status:'available',entries:[markdown,stored]}}}/>);
 fireEvent.click(screen.getByRole('button',{name:'docs/value.md'}));
 expect(await screen.findByRole('heading',{name:'Recorded value'})).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'source',exact:true}));
 expect(await screen.findByRole('heading',{name:'Stored destination'})).toBeVisible();
});
