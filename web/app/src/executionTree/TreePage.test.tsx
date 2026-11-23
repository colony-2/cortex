import { StrictMode } from 'react';
import { act, fireEvent, render, screen, within, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { getTreeNode, getTreeChildren, getTreeWaits } from '@colony2/shared';
import { TreeWorkspace } from './TreePage';
vi.mock('@colony2/shared', () => ({getTreeNode:vi.fn(), getTreeChildren:vi.fn(), getTreeWaits:vi.fn(async(tenant,job)=>({consumer:{tenant_id:tenant,job_id:job},waits:[],issues:[],observed_at:'2026-11-17T00:00:00Z',consumer_snapshot:{state:'available',observed_at:'2026-11-17T00:00:00Z',status:'READY'}}))}));
it('DAG-05,06,07 retains presentation while paused, and isolates keyed tenant/job identities under effect replay', async () => {
 const signals: AbortSignal[]=[];
 vi.mocked(getTreeNode).mockImplementation(async (tenant,id,signal) => {
  signals.push(signal!);
  return {node:{tenant_id:tenant,job_id:id,job_type:'recipe',status:'READY'},observed_at:'now'};
 });
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:'now'});
 const view = (tenant:string, entry:string, active=true) => <StrictMode><MemoryRouter><TreeWorkspace key={JSON.stringify([tenant,entry])} tenant={tenant} entry={entry} active={active} embedded /></MemoryRouter></StrictMode>;
 const {rerender} = render(view('t','one'));
 await screen.findByText('Discovery finished',{exact:false});
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
 await screen.findByText('Discovery finished',{exact:false});
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
 const {rerender,unmount}=render(view(true));await screen.findByText('Discovery finished',{exact:false});
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
  await screen.findByText('Discovery finished',{exact:false});
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

it('MRV-002 starts one entry read under StrictMode, aborts on departure and automatically resumes only once', async () => {
 vi.mocked(getTreeNode).mockReset();
 vi.mocked(getTreeChildren).mockReset();
 let releaseOld!: (value: any) => void;
 vi.mocked(getTreeNode).mockImplementationOnce(() => new Promise(resolve => { releaseOld = resolve; }))
  .mockResolvedValue({node:{tenant_id:'t',job_id:'one',job_type:'recipe',status:'READY'},observed_at:'current'});
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:'current'});
 const view = (active: boolean) => <StrictMode><MemoryRouter><TreeWorkspace tenant="t" entry="one" active={active} embedded /></MemoryRouter></StrictMode>;
 const {rerender} = render(view(true));
 await waitFor(() => expect(getTreeNode).toHaveBeenCalledTimes(1));
 const signal = vi.mocked(getTreeNode).mock.calls[0][2]!;
 rerender(view(false));
 expect(signal.aborted).toBe(true);
 expect(getTreeNode).toHaveBeenCalledTimes(1);
 rerender(view(true));
 await screen.findByText('Discovery finished',{exact:false});
 expect(getTreeNode).toHaveBeenCalledTimes(2);
 expect(getTreeChildren).toHaveBeenCalledTimes(1);
 await act(async () => { releaseOld({node:{tenant_id:'t',job_id:'obsolete',job_type:'recipe',status:'READY'},observed_at:'old'}); });
 await waitFor(() => expect(screen.getByRole('complementary')).toHaveTextContent('one'));
 expect(screen.queryByText('obsolete',{exact:true})).toBeNull();
 expect(getTreeNode).toHaveBeenCalledTimes(2);
});

it('MRV-002 does not start a queued discovery after leaving the tab', async () => {
 vi.mocked(getTreeNode).mockReset();
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:'now'});
 vi.mocked(getTreeNode).mockResolvedValue({node:{tenant_id:'t',job_id:'one',job_type:'recipe',status:'READY'},observed_at:'now'});
 const view = (active: boolean) => <StrictMode><MemoryRouter><TreeWorkspace tenant="t" entry="one" active={active} embedded /></MemoryRouter></StrictMode>;
 const {rerender} = render(view(true));
 rerender(view(false));
 await act(async () => {});
 expect(getTreeNode).not.toHaveBeenCalled();
 rerender(view(true));
 await screen.findByText('Discovery finished',{exact:false});
 expect(getTreeNode).toHaveBeenCalledTimes(1);
});

vi.mock('@colony2/shared/api', async original => ({...await original<typeof import('@colony2/shared/api')>(),cancelJob:vi.fn()}));
import { cancelJob } from '@colony2/shared/api';
import { within } from '@testing-library/react';

it.each(['recipe','generic'])('offers cancellation for all unfinished %s nodes and truthful terminal or pending details', async job_type => {
 for (const extra of [
  ...['READY','PENDING_JOBS','AWAITING_FUTURE','ACTIVE','CRASH_CONCERN','EXPIRED'].map(status=>({status,completion_status:'failed_app'})),
  {status:'ACTIVE',cancel_requested:true}, {status:'COMPLETED'}, {status:'COMPLETED',completion_status:'custom'},
  {status:'ACTIVE',store:'ARCHIVED'}, {status:'ACTIVE',archived_at:'now'}, {status:'CANCELLED'}, {status:'unknown'}, {status:''},
 ]) {
  const node={tenant_id:'t',job_id:'single',job_type,...extra};
  vi.mocked(getTreeNode).mockResolvedValue({node,observed_at:'now'});
  vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:'now'});
  const {unmount}=render(<MemoryRouter><TreeWorkspace tenant="t" entry="single" embedded /></MemoryRouter>);
  const pane=await screen.findByRole('complementary');
  const eligible=['READY','PENDING_JOBS','AWAITING_FUTURE','ACTIVE','CRASH_CONCERN','EXPIRED'].includes(extra.status) && !('cancel_requested' in extra) && !('store' in extra) && !('archived_at' in extra);
  expect(!!within(pane).queryByRole('button',{name:'Cancel job'})).toBe(eligible);
  if(extra.status==='CRASH_CONCERN') expect(pane).toHaveTextContent('Job keeps failing');
  if('cancel_requested' in extra) { expect(pane).toHaveTextContent('Cancellation requested');expect(within(pane).getByRole('button',{name:'Refresh status'})).toBeEnabled(); }
  unmount();
 }
});

it('cancels the captured child, locks through reconciliation, preserves context and protects the newer outcome from discovery', async () => {
 const opened={tenant_id:'t',job_id:'opened',job_type:'recipe',status:'ACTIVE',title:'Opened title'};
 const child={...opened,job_id:'child',job_type:'generic',parent_job_id:'opened',status:'CRASH_CONCERN',title:'Selected child'};
 let exact=false, release!:(value:any)=>void;
 vi.mocked(cancelJob).mockReset().mockResolvedValue();
 vi.mocked(getTreeNode).mockImplementation(async(_,id)=> {
  if(id==='child' && exact) {exact=false;return new Promise(resolve=>release=resolve);}
  return {node:id==='child'?child:opened,observed_at:'old'};
 });
 vi.mocked(getTreeChildren).mockImplementation(async(_,id)=>({nodes:id==='opened'?[child]:[],observed_at:'old'}));
 const changed=vi.fn();
 render(<MemoryRouter><TreeWorkspace tenant="t" entry="opened" embedded onJobChanged={changed} /></MemoryRouter>);
 await screen.findByText('Discovery finished',{exact:false});
 fireEvent.click(screen.getByRole('button',{name:'Selected child · Unknown cell'}));
 fireEvent.change(screen.getByRole('searchbox'),{target:{value:'Selected'}});
 fireEvent.click(screen.getByRole('button',{name:'Zoom in'}));
 const pane=screen.getByRole('complementary');
 fireEvent.click(within(pane).getByRole('button',{name:'Cancel job'}));
 const dialog=screen.getByRole('dialog');
 expect(dialog).toHaveTextContent('Tenant: t');expect(dialog).toHaveTextContent('Job: child');expect(dialog).toHaveTextContent('Child jobs already submitted will not be cancelled');
 fireEvent.change(within(dialog).getByRole('textbox'),{target:{value:' chosen reason 雪 '}});
 // A programmatic selection change while confirmation is open cannot retarget it.
 fireEvent.click(screen.getByRole('button',{name:'Opened title · Unknown cell · Opened job'}));
 exact=true;
 const confirm=within(dialog).getByRole('button',{name:'Cancel job',exact:true});
 fireEvent.click(confirm);fireEvent.click(confirm);
 await waitFor(()=>expect(getTreeNode).toHaveBeenCalledWith('t','child',expect.any(AbortSignal)));
 expect(cancelJob).toHaveBeenCalledTimes(1);expect(cancelJob).toHaveBeenCalledWith('t','child','chosen reason 雪');
 expect(confirm).toBeDisabled();
 await act(async()=>{release({node:{...child,status:'COMPLETED',store:'ARCHIVED',completion_status:'cancelled'},observed_at:'new'});});
 await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 fireEvent.click(screen.getByRole('button',{name:'Selected child · Unknown cell'}));
 expect(pane).toHaveTextContent('Cancelled');expect(within(pane).queryByRole('button',{name:'Cancel job'})).toBeNull();
 expect(screen.getByRole('searchbox')).toHaveValue('Selected');expect(screen.getByLabelText('Zoom',{exact:true})).toHaveTextContent('120%');
 expect(changed).toHaveBeenCalledWith(expect.objectContaining({tenant_id:'t',job_id:'child',completion_status:'cancelled'}));
 fireEvent.click(screen.getByRole('button',{name:'Refresh tree'}));
 await screen.findByText('Discovery finished',{exact:false});
 fireEvent.click(screen.getByRole('button',{name:'Selected child · Unknown cell'}));
 expect(screen.getByRole('complementary')).toHaveTextContent('Cancelled');
 expect(screen.getByRole('button',{name:'Selected child · Unknown cell'})).toHaveTextContent('Cancelled');
});

it('keeps failed cancellation retries tied to the captured job and message', async()=>{
 const node={tenant_id:'t',job_id:'child',job_type:'generic',status:'PENDING_JOBS',title:'Waiting child'};
 vi.mocked(getTreeNode).mockResolvedValue({node,observed_at:'now'});
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:'now'});
 vi.mocked(cancelJob).mockReset().mockRejectedValueOnce(Error('denied')).mockResolvedValue();
 render(<MemoryRouter><TreeWorkspace tenant="t" entry="child" /></MemoryRouter>);
 fireEvent.click(await screen.findByRole('button',{name:'Cancel job'}));
 const dialog=screen.getByRole('dialog');fireEvent.change(within(dialog).getByRole('textbox'),{target:{value:'saved message'}});
 fireEvent.click(within(dialog).getByRole('button',{name:'Cancel job',exact:true}));
 await screen.findByText('1 job operation failed');
 await waitFor(()=>expect(within(dialog).getByRole('button',{name:'Cancel job',exact:true})).toBeEnabled());
 expect(within(dialog).getByRole('textbox')).toHaveValue('saved message');
 fireEvent.click(within(dialog).getByRole('button',{name:'Cancel job',exact:true}));
 await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 expect(cancelJob).toHaveBeenNthCalledWith(2,'t','child','saved message');
 expect(screen.getByRole('complementary')).toHaveTextContent('Cancellation requested');
 fireEvent.click(screen.getByRole('button',{name:'Refresh tree'}));
 await screen.findByText('Discovery finished',{exact:false});
 expect(screen.getByRole('complementary')).toHaveTextContent('Cancellation requested');
});

it.each(['recipe','generic'])('DU01–07 complete %s wait target cards and details retain anchored discovery and incoming consumers',async job_type=>{
 const observed='2026-11-17T00:00:00Z';let resolved=false;
 const records=[{tenant_id:'t',job_id:'consumer',job_type:'generic',status:'PENDING_JOBS',title:'Waiting consumer'},
 {tenant_id:'t',job_id:'independent',job_type,status:'ACTIVE',cell:'worker',title:'Prepare dependency 雪',prompt:'Full prompt\n<script>literal</script>',goal_id:'goal',parent_job_id:'outside',recipe_parent_job_id:'attribution',created_at:'2026-11-01T00:00:00Z'}];
 vi.mocked(getTreeNode).mockImplementation(async(_,id)=>({node:records.find(n=>n.job_id===id)!,observed_at:observed}));
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:observed});
 vi.mocked(getTreeWaits).mockImplementation(async(tenant,id)=>({consumer:{tenant_id:tenant,job_id:id},waits:id==='consumer'&&!resolved?[{jobKey:{tenantId:tenant,jobId:id},waitId:'w',state:'PENDING',dependencies:[{target:{jobId:'independent'},condition:'complete',state:'PENDING'},{target:{goalId:'literal-goal'},condition:'success',state:'PENDING'}]}]:[],issues:[],observed_at:observed,consumer_snapshot:{state:'available',observed_at:observed,status:id==='consumer'?'PENDING_JOBS':'READY'}}));
 render(<MemoryRouter><TreeWorkspace tenant="t" entry="consumer"/></MemoryRouter>);
 const target=await screen.findByRole('button',{name:'Prepare dependency 雪 · worker'});
 await waitFor(()=>expect(screen.getByRole('region',{name:'Discovery coverage'})).toHaveTextContent('Partial discovery'));
 expect(document.querySelectorAll('.tree-card')).toHaveLength(2);expect(target).toHaveTextContent('Wait target');expect(target).toHaveTextContent('Submitted Nov 1, 2026');expect(target).toHaveTextContent('Running');
 const reads=vi.mocked(getTreeNode).mock.calls.length,waitReads=vi.mocked(getTreeWaits).mock.calls.length;
 fireEvent.click(screen.getByRole('button',{name:'Toggle branch Waiting consumer'}));expect(screen.queryByRole('button',{name:'Prepare dependency 雪 · worker'})).toBeNull();
 fireEvent.click(within(screen.getByRole('region',{name:'Waits for'})).getAllByRole('button',{name:'Select target'})[0]);expect(screen.getByRole('button',{name:'Prepare dependency 雪 · worker'})).toBeInTheDocument();
 const pane=screen.getByRole('complementary',{name:'Selected job details'});
 expect(pane).toHaveTextContent('Waiting job: consumer');expect(pane).toHaveTextContent('Full prompt');expect(pane).toHaveTextContent('<script>literal</script>');expect(pane).toHaveTextContent('attribution');expect(pane).toHaveTextContent('outside');expect(pane).toHaveTextContent('No unresolved waits');
 expect(within(pane).getByRole('button',{name:'Cancel job'})).toBeEnabled();expect(within(pane).getByRole('link',{name:'Goal overview'})).toHaveAttribute('href','/project/t/goals/goal');
 expect(within(pane).queryByRole('link',{name:'Open execution'})!==null).toBe(job_type==='recipe');
 expect(document.querySelectorAll('aside script')).toHaveLength(0);expect(document.querySelectorAll('.tree-card')).toHaveLength(2);
 await act(async()=>{});expect(vi.mocked(getTreeNode).mock.calls.length).toBe(reads);expect(vi.mocked(getTreeWaits).mock.calls.length).toBe(waitReads);
 fireEvent.click(within(pane).getByRole('button',{name:'Back to waiting job'}));expect(pane).toHaveTextContent('Waiting consumer');
 resolved=true;fireEvent.click(within(pane).getByRole('button',{name:'Refresh waits'}));
 await waitFor(()=>expect(screen.queryByRole('button',{name:'Prepare dependency 雪 · worker'})).toBeNull());expect(pane).toHaveTextContent('do not explain the waiting status');
});

it('DU03,04,06 context-only selection reads direct waits; unavailable summaries retain conditions and retry',async()=>{
 const at='2026-11-17T00:00:00Z';let available=false;
 const records=[{tenant_id:'t',job_id:'entry',parent_job_id:'parent',job_type:'generic',status:'READY'}, {tenant_id:'t',job_id:'parent',job_type:'generic',status:'READY'}, {tenant_id:'t',job_id:'target',job_type:'generic',status:'READY',title:'Recovered target'}];
 vi.mocked(getTreeNode).mockImplementation(async(_,id)=>{if(id==='target'&&!available)throw Error();return {node:records.find(n=>n.job_id===id)!,observed_at:at};});
 vi.mocked(getTreeChildren).mockImplementation(async(_,id)=>({nodes:records.filter(n=>n.parent_job_id===id),observed_at:at}));
 vi.mocked(getTreeWaits).mockImplementation(async(tenant,id)=>({consumer:{tenant_id:tenant,job_id:id},waits:id==='parent'?[{jobKey:{tenantId:tenant,jobId:id},waitId:'w',state:'PENDING',dependencies:[{target:{jobId:'target'},condition:'complete',state:'PENDING'}]}]:[],issues:[],observed_at:at,consumer_snapshot:{state:'available',observed_at:at,status:'READY'}}));
 render(<MemoryRouter><TreeWorkspace tenant="t" entry="entry"/></MemoryRouter>);await screen.findByText('Discovery finished',{exact:false});expect(vi.mocked(getTreeWaits).mock.calls.some(c=>c[1]==='parent')).toBe(false);
 fireEvent.click(screen.getByRole('button',{name:'Untitled job · Unknown cell'}));
 const missing=await screen.findByRole('button',{name:'Job wait target · target'});await waitFor(()=>expect(screen.getByRole('region',{name:'Discovery coverage'})).toHaveTextContent('Partial discovery'));
 fireEvent.click(missing);const pane=screen.getByRole('complementary');expect(pane).toHaveTextContent('complete');expect(within(pane).queryByRole('button',{name:'Cancel job'})).toBeNull();
 available=true;fireEvent.click(within(pane).getByRole('button',{name:'Retry summary'}));await screen.findByRole('button',{name:'Recovered target · Unknown cell'});
 await waitFor(()=>expect(vi.mocked(getTreeWaits).mock.calls.filter(c=>c[1]==='target')).toHaveLength(1));
 expect(vi.mocked(getTreeChildren).mock.calls.some(c=>c[1]==='target')).toBe(false);
});
it('DU08 restores selected wait reference focus when its summary merges, then returns to a surviving consumer after verified removal',async()=>{
 let release!:(r:any)=>void,resolved=false;const at='2026-11-17T00:00:00Z';
 vi.mocked(getTreeNode).mockImplementation(async(_,id)=>id==='target'?new Promise(r=>release=r):({node:{tenant_id:'t',job_id:id,job_type:'recipe',status:'READY',title:'Consumer'},observed_at:at}));
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:at});
 vi.mocked(getTreeWaits).mockImplementation(async(tenant,id)=>({consumer:{tenant_id:tenant,job_id:id},waits:id==='entry'&&!resolved?[{jobKey:{tenantId:tenant,jobId:id},waitId:'w',state:'PENDING',dependencies:[{target:{jobId:'target'},condition:'complete',state:'PENDING'}]}]:[],issues:[],observed_at:at,consumer_snapshot:{state:'available',observed_at:at,status:'READY'}}));
 render(<MemoryRouter><TreeWorkspace tenant="t" entry="entry"/></MemoryRouter>);
 const ref=await screen.findByRole('button',{name:'Job wait target · target'});ref.focus();fireEvent.click(ref);
 await act(async()=>{release({node:{tenant_id:'t',job_id:'target',job_type:'generic',status:'ACTIVE',title:'Hydrated target'},observed_at:at});});
 const card=await screen.findByRole('button',{name:'Hydrated target · Unknown cell'});await waitFor(()=>expect(card).toHaveFocus());
 fireEvent.click(screen.getByRole('button',{name:'Back to waiting job'}));resolved=true;fireEvent.click(screen.getByRole('button',{name:'Refresh waits'}));await waitFor(()=>expect(screen.queryByRole('button',{name:'Hydrated target · Unknown cell'})).toBeNull());
 expect(screen.getByRole('complementary')).toHaveTextContent('Consumer');
});
it('DU09 cancellation of a selected external wait job updates its card and details with the newer completion outcome',async()=>{
 const at='2026-11-17T00:00:00Z';let completed=false;
 vi.mocked(cancelJob).mockReset().mockImplementation(async()=>{completed=true;});
 vi.mocked(getTreeNode).mockImplementation(async(_,id)=>({node:{tenant_id:'t',job_id:id,job_type:'recipe',title:id==='entry'?'Consumer':'External dependency',status:id==='target'&&completed?'COMPLETED':'ACTIVE',completion_status:id==='target'&&completed?'success':undefined},observed_at:at}));
 vi.mocked(getTreeChildren).mockResolvedValue({nodes:[],observed_at:at});
 vi.mocked(getTreeWaits).mockImplementation(async(tenant,id)=>({consumer:{tenant_id:tenant,job_id:id},waits:id==='entry'?[{jobKey:{tenantId:tenant,jobId:id},waitId:'w',state:'PENDING',dependencies:[{target:{jobId:'target'},condition:'complete',state:'PENDING'}]}]:[],issues:[],observed_at:at,consumer_snapshot:{state:'available',observed_at:at,status:'READY'}}));
 render(<MemoryRouter><TreeWorkspace tenant="t" entry="entry"/></MemoryRouter>);
 const card=await screen.findByRole('button',{name:'External dependency · Unknown cell'});fireEvent.click(card);fireEvent.click(screen.getByRole('button',{name:'Cancel job'}));
 const dialog=screen.getByRole('dialog');expect(dialog).toHaveTextContent('Job: target');fireEvent.click(within(dialog).getByRole('button',{name:'Cancel job',exact:true}));
 await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());expect(cancelJob).toHaveBeenCalledWith('t','target','Cancelled from Cortex');expect(card).toHaveTextContent('Succeeded');expect(screen.getByRole('complementary')).toHaveTextContent('Succeeded');expect(document.querySelectorAll('.tree-card')).toHaveLength(2);
});
