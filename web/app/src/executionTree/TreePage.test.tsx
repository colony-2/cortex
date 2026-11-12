import { StrictMode } from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { getTreeNode, getTreeChildren } from '@colony2/shared';
import { TreeWorkspace } from './TreePage';
vi.mock('@colony2/shared', () => ({getTreeNode:vi.fn(), getTreeChildren:vi.fn()}));
it('DAG-05,06,07 retains presentation while paused, and isolates keyed tenant/job identities under effect replay', async () => {
 const signals: AbortSignal[]=[];
 vi.mocked(getTreeNode).mockImplementation(async (tenant,id,signal) => {
  signals.push(signal!);
  return {node:{tenant_id:tenant,job_id:id,job_type:'recipe',status:'READY'},observed_at:'now'};
 });
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:'now'});
 const view = (tenant:string, entry:string, active=true) => <StrictMode><MemoryRouter><TreeWorkspace key={JSON.stringify([tenant,entry])} tenant={tenant} entry={entry} active={active} embedded /></MemoryRouter></StrictMode>;
 const {rerender} = render(view('t','one'));
 await screen.findByText('Discovery finished:',{exact:false});
 fireEvent.change(screen.getByRole('searchbox'),{target:{value:'saved'}});
 fireEvent.click(screen.getByRole('button',{name:'Zoom in'}));
 const count=vi.mocked(getTreeNode).mock.calls.length;
 rerender(view('t','one',false)); rerender(view('t','one'));
 expect(screen.getByRole('searchbox')).toHaveValue('saved');
 expect(screen.getByLabelText('Zoom',{exact:true})).toHaveTextContent('120%');
 expect(getTreeNode).toHaveBeenCalledTimes(count);
 rerender(view('other','two'));
 await waitFor(()=>expect(screen.getByRole('complementary')).toHaveTextContent('two'));
 expect(signals.slice(0,count).every(signal=>signal.aborted)).toBe(true);
 expect(screen.getByRole('searchbox')).toHaveValue('');
 expect(screen.getByLabelText('Zoom',{exact:true})).toHaveTextContent('100%');
 expect(screen.queryByText('one', {exact:true})).toBeNull();
});

it.each(['entry','ancestor','children'])('DAGUI-020,022 automatically resumes interrupted %s on tab return and rejects stale work', async stage => {
 let finish!:(v:any)=>void;
 const entry={tenant_id:'t',job_id:'opened',job_type:'recipe',status:'READY',title:'Opened title',parent_job_id:stage==='ancestor'?'parent':undefined};
 const parent={...entry,job_id:'parent',parent_job_id:undefined};
 let paused=false;
 vi.mocked(getTreeNode).mockImplementation(async(_,id)=>({node:id==='parent'?parent:entry,observed_at:'now'}));
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:'now'});
 if(stage==='children') vi.mocked(getTreeChildren).mockImplementationOnce(()=>{paused=true;return new Promise(r=>finish=r);});
 else vi.mocked(getTreeNode).mockImplementation(async(_,id)=> {
  if(!paused && (stage==='entry'||id==='parent')) {paused=true;return new Promise(r=>finish=r);}
  return {node:id==='parent'?parent:entry,observed_at:'now'};
 });
 const view=(active:boolean)=><MemoryRouter><TreeWorkspace tenant="t" entry="opened" active={active} embedded /></MemoryRouter>;
 const {rerender}=render(view(true));await waitFor(()=>expect(paused).toBe(true));
 fireEvent.change(screen.getByRole('searchbox'),{target:{value:'saved'}});
 rerender(view(false));rerender(view(true));
 await screen.findByText('Discovery finished:',{exact:false});
 finish(stage==='children'?{nodes:[{...entry,job_id:'stale'}],observed_at:'old'}:{node:{...entry,job_id:'stale'},observed_at:'old'});
 await Promise.resolve();
 expect(screen.getByRole('searchbox')).toHaveValue('saved');
 expect(screen.getByRole('complementary')).toHaveTextContent('opened');
 expect(screen.queryByText('stale')).toBeNull();
});

it('DAGUI-004,005,015,033 shows safe full details and unavailable offsets for incomplete ancestry',async()=>{
 vi.mocked(getTreeNode).mockImplementation(async(_,id)=> {
  if(id==='missing') throw Error();
  return {node:{tenant_id:'t',job_id:'identity',job_type:'generic',status:'READY',title:'  A readable title  ',prompt:'<script>literal</script>\nFull text 雪',parent_job_id:'missing',created_at:new Date(Date.now()-60000).toISOString()},observed_at:'now'};
 });
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:'now'});
 render(<MemoryRouter><TreeWorkspace tenant="t" entry="identity" /></MemoryRouter>);
 await screen.findByText('Partial discovery');
 const card=screen.getByRole('button',{name:'A readable title · Unknown cell · Opened job'});
 expect(card).toHaveTextContent('Submitted Unavailable');expect(card).toHaveTextContent(/Age (59s|1m)/);
 expect(card).not.toHaveTextContent('identity');expect(card).not.toHaveTextContent('generic');
 expect(screen.getByRole('complementary')).toHaveTextContent('<script>literal</script>');
 expect(document.querySelector('aside script')).toBeNull();
});

it('DAGUI-016 visible clock advances ages, suspends inactive, recomputes on return, freezes finished span', async()=>{
 let clock=Date.parse('2026-11-12T09:18:42Z');const date=vi.spyOn(Date,'now').mockImplementation(()=>clock);
 let tick!:()=>void;
 const interval=vi.spyOn(window,'setInterval').mockImplementation((fn:any,delay?:number)=>{if(delay===1000) tick=fn;return 99;});
 const clear=vi.spyOn(window,'clearInterval');
 vi.mocked(getTreeNode).mockResolvedValue({node:{tenant_id:'t',job_id:'live',job_type:'recipe',status:'READY',created_at:'2026-11-12T09:00:10Z'},observed_at:'now'});
 vi.mocked(getTreeChildren).mockImplementation(async(_,id)=>({nodes:id==='live'?[{tenant_id:'t',job_id:'done',parent_job_id:'live',job_type:'generic',status:'COMPLETED',created_at:'2026-11-12T09:01:48Z',archived_at:'2026-11-12T09:07:20Z'}]:[],observed_at:'now'}));
 const view=(active:boolean)=><MemoryRouter><TreeWorkspace tenant="t" entry="live" active={active} /></MemoryRouter>;
 const {rerender,unmount}=render(view(true));await screen.findByText('Discovery finished:',{exact:false});
 expect(screen.getByText('Age 18m 32s')).toBeInTheDocument();
 clock+=1000;act(()=>tick());
 expect(screen.getByText('Age 18m 33s')).toBeInTheDocument();
 rerender(view(false));expect(clear).toHaveBeenCalledWith(99);clock+=60000;rerender(view(true));
 expect(screen.getByText('Age 19m 33s')).toBeInTheDocument();expect(screen.getByText('Time to finish 5m 32s')).toBeInTheDocument();
 const visibility=vi.spyOn(document,'visibilityState','get').mockReturnValue('hidden');
 const reads=vi.mocked(getTreeNode).mock.calls.length;
 fireEvent(document,new Event('visibilitychange'));clock+=60000;
 expect(screen.getByText('Age 19m 33s')).toBeInTheDocument();
 visibility.mockReturnValue('visible');fireEvent(document,new Event('visibilitychange'));
 expect(screen.getByText('Age 20m 33s')).toBeInTheDocument();expect(screen.getByText('Time to finish 5m 32s')).toBeInTheDocument();
 expect(getTreeNode).toHaveBeenCalledTimes(reads);
 unmount();visibility.mockRestore();interval.mockRestore();clear.mockRestore();date.mockRestore();
});

it.each([
 { initial: 'hidden document', strict: false },
 { initial: 'inactive workspace', strict: false },
 { initial: 'hidden document', strict: true },
 { initial: 'inactive workspace', strict: true },
])('DAG-REVIEW-001, DAGUI-018,020 initially $initial starts automatically (StrictMode=$strict)', async ({initial,strict}) => {
 const visibility=vi.spyOn(document,'visibilityState','get').mockReturnValue(initial==='hidden document'?'hidden':'visible');
 const scrollDescriptor=Object.getOwnPropertyDescriptor(HTMLElement.prototype,'scrollIntoView');
 const revealed:string[]=[];
 Object.defineProperty(HTMLElement.prototype,'scrollIntoView',{configurable:true,value:function(this:HTMLElement){
  revealed.push(this.closest<HTMLElement>('[data-job-id]')?.dataset.jobId || '');
 }});
 const records=[
  {tenant_id:'t',job_id:'root',job_type:'generic',status:'COMPLETED',store:'ARCHIVED',title:'Family root',created_at:'2026-11-09T00:00:00Z'},
  {tenant_id:'t',job_id:'middle',parent_job_id:'root',job_type:'generic',status:'READY',title:'Intermediate job',created_at:'2026-11-09T00:01:00Z'},
  {tenant_id:'t',job_id:'opened',parent_job_id:'middle',job_type:'recipe',status:'READY',title:'Opened child',created_at:'2026-11-09T00:02:10Z'},
  {tenant_id:'t',job_id:'sibling',parent_job_id:'root',job_type:'recipe',status:'READY',title:'Sibling job'},
 ];
 vi.mocked(getTreeNode).mockReset().mockImplementation(async(_,id)=>({node:records.find(n=>n.job_id===id)!,observed_at:'now'}));
 vi.mocked(getTreeChildren).mockReset().mockImplementation(async(_,id)=>({nodes:records.filter(n=>n.parent_job_id===id),observed_at:'now'}));
 const view=(active:boolean)=>{
  const workspace=<MemoryRouter><TreeWorkspace tenant="t" entry="opened" active={active} /></MemoryRouter>;
  return strict?<StrictMode>{workspace}</StrictMode>:workspace;
 };
 const {rerender,unmount}=render(view(initial!=='inactive workspace'));
 try {
  expect(getTreeNode).not.toHaveBeenCalled();expect(getTreeChildren).not.toHaveBeenCalled();
  visibility.mockReturnValue('visible');
  if(initial==='hidden document') fireEvent(document,new Event('visibilitychange'));
  else rerender(view(true));
  await screen.findByText('Discovery finished:',{exact:false});
  expect(screen.getByRole('region',{name:'Discovery coverage'})).toHaveTextContent('4 loaded jobs');
  expect(vi.mocked(getTreeNode).mock.calls.map(call=>call[1])).toEqual(['opened','middle','root']);
  expect(screen.getByRole('button',{name:'Family root · Unknown cell'})).toBeInTheDocument();
  expect(screen.getByRole('button',{name:'Intermediate job · Unknown cell'})).toBeInTheDocument();
  expect(screen.getByRole('button',{name:'Sibling job · Unknown cell'})).toBeInTheDocument();
  const opened=screen.getByRole('button',{name:'Opened child · Unknown cell · Opened job'});
  expect(opened).toHaveAttribute('aria-pressed','true');expect(opened).toHaveTextContent('Submitted +2m 10s');
  expect(revealed).toEqual(['opened']);expect(opened).not.toHaveFocus();
  const reads=[vi.mocked(getTreeNode).mock.calls.length,vi.mocked(getTreeChildren).mock.calls.length];
  visibility.mockReturnValue('hidden');fireEvent(document,new Event('visibilitychange'));
  visibility.mockReturnValue('visible');fireEvent(document,new Event('visibilitychange'));
  expect([vi.mocked(getTreeNode).mock.calls.length,vi.mocked(getTreeChildren).mock.calls.length]).toEqual(reads);
  expect(revealed).toEqual(['opened']);
 } finally {
  unmount();visibility.mockRestore();
  if(scrollDescriptor) Object.defineProperty(HTMLElement.prototype,'scrollIntoView',scrollDescriptor);
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
 }
});
