import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import JobConversation, { HistoryPair, CurrentActivity } from './JobConversation';
import { APIError, getConversation, getConversationEntry, type HistoryEntry } from '@colony2/shared';
vi.mock('@colony2/shared', async () => ({ ...await vi.importActual('@colony2/shared'), getConversation: vi.fn(), getConversationEntry: vi.fn() }));
const entry = (id = 'one'): HistoryEntry => ({entry_id:id,kind:'review',title:{present:true,raw:'Original review'},summary:{present:false},state:'answered',requested_at:{present:false},locator:{invocation_id:'invoke'},form_fields:{question:'Proceed?'},accepted_response:{present:true,raw:{decision:'approve',feedback:'Keep original'}},response_source:'autofill',actor:{provenance:'reported_receipt',receipt:{actor:{kind:'human',id:'caller'}},legacy_user_id:{present:false}}});
const page = (entries = [entry()], next?: string) => ({tenant_id:'tenant',job_id:'job',entries,snapshot_revision:1,snapshot_token:'snapshot',observed_at:'now',completeness:'partial',next_page_token:next});
beforeEach(() => vi.clearAllMocks());
describe('conversation retained evidence', () => {
 it('CT-009 CT-010 CT-016 CT-034 renders exact accepted feedback and caller provenance', () => {
  render(<HistoryPair entry={entry()} />);
  expect(screen.getAllByText(/Keep original/)[0]).toBeInTheDocument();
  expect(screen.getAllByText(/Caller-reported identity/)[0]).toBeInTheDocument();
  expect(screen.getAllByText('Time not recorded')[0]).toBeInTheDocument();
  expect(screen.getByText(/State: answered/)).toBeInTheDocument();
 });
 it.each(['ordinary', 'structured', 'review'])('CT-010 %s never promotes a missing accepted outcome', kind => {
  const recorded = {...entry(), kind, state:'unknown', accepted_response:{present:false}, actor:{provenance:'automation_receipt',legacy_user_id:{present:false}}};
  render(<HistoryPair entry={recorded} />);
  expect(screen.getByText('Not recorded')).toBeInTheDocument();
  expect(screen.getByText('Automated response.')).toBeInTheDocument();
  expect(screen.queryByText(/Keep original/)).not.toBeInTheDocument();
  expect(screen.getByText(/State: unknown/)).toBeInTheDocument();
 });
 it('CT-013 deduplicates overlapping continuation pages by durable identity', async () => {
  vi.mocked(getConversation).mockResolvedValueOnce(page([entry()], 'next')).mockResolvedValueOnce(page([entry(),entry('two')]));
  render(<JobConversation tenant="tenant" job="job" refreshKey={0} onOpen={vi.fn()} />);
  fireEvent.click(await screen.findByText('Load more events'));
  await screen.findByText('Occurrence: two');
  expect(screen.getAllByText('Occurrence: one')).toHaveLength(1);
 });
 it('CT-011 CT-012 preserves supplied order and repeated request identities', async () => {
  vi.mocked(getConversation).mockResolvedValue(page(['two','one'].map(id => ({...entry(id), requested_at:{present:true,raw:'same timestamp'}, locator:{invocation_id:id,collection_ordinals:[7]}}))));
  render(<JobConversation tenant="tenant" job="job" refreshKey={0} onOpen={vi.fn()} />);
  await screen.findByText('Occurrence: two');
  expect(screen.getAllByText(/Occurrence:/).map(x => x.textContent)).toEqual(['Occurrence: two','Occurrence: one']);
 });
 it('CT-013 CT-033 retains entries on expiry and refreshes from a fresh snapshot', async () => {
  vi.mocked(getConversation).mockResolvedValueOnce(page([entry()], 'opaque')).mockRejectedValueOnce(new APIError('expired',410,'history_snapshot_expired')).mockResolvedValueOnce(page([entry(),entry('two')]));
  const view = render(<JobConversation tenant="tenant" job="job" refreshKey={0} onOpen={vi.fn()} />);
  fireEvent.click(await screen.findByText('Load more events'));
  await screen.findByText(/History snapshot expired.*page Refresh button/);
  expect(screen.getByText('Occurrence: one')).toBeInTheDocument();
  view.rerender(<JobConversation tenant="tenant" job="job" refreshKey={1} onOpen={vi.fn()} />);
  await screen.findByText('Occurrence: two');
  expect(vi.mocked(getConversation).mock.calls[2][2]?.pageToken).toBeUndefined();
 });
 it('CT-014 CT-019 opens exact unloaded detail, keeping missing requests unavailable', async () => {
  vi.mocked(getConversation).mockResolvedValue(page([]));
  vi.mocked(getConversationEntry).mockRejectedValue(new Error('not retained'));
  render(<JobConversation tenant="tenant" job="job" occurrence="old/?" onOpen={vi.fn()} />);
  await screen.findByText('Request unavailable');
  expect(getConversationEntry).toHaveBeenCalledWith('tenant','job','old/?',expect.anything());
  expect(screen.queryByText('Original review')).not.toBeInTheDocument();
 });
 it.each(['job','tenant'])('CT-022 CR-010 discards obsolete %s loads', async change => {
  let resolve!: (v: ReturnType<typeof page>) => void;
  vi.mocked(getConversation).mockImplementationOnce(() => new Promise(r => {resolve = r;})).mockResolvedValue({...page([entry('new')]),job_id:'other'});
  const view = render(<JobConversation tenant="tenant" job="job" onOpen={vi.fn()} />);
  view.rerender(<JobConversation tenant={change === 'tenant' ? 'other' : 'tenant'} job={change === 'job' ? 'other' : 'job'} onOpen={vi.fn()} />);
  await screen.findByText('Occurrence: new');
  resolve(page([entry('obsolete')]));
  await waitFor(() => expect(screen.queryByText('Occurrence: obsolete')).not.toBeInTheDocument());
 });
 it.each(['cancelled','timed_out','superseded','unknown'])('CT-016 preserves recorded %s closure without inventing an answer', state => {
  render(<HistoryPair entry={{...entry(), state, state_evidence:['Recorded closure evidence'], accepted_response:{present:false}, form_fields:{delay_until:'past eligibility'}}} />);
  expect(screen.getByText(new RegExp(`State: ${state}`))).toBeInTheDocument();
  expect(screen.getByText('Recorded closure evidence')).toBeInTheDocument();
  expect(screen.getByText('Not recorded')).toBeInTheDocument();
  expect(screen.queryByText(/Keep original/)).not.toBeInTheDocument();
 });
 it('CT-020 keeps partial entries visible through failed refresh and retry', async () => {
  vi.mocked(getConversation).mockResolvedValueOnce({...page(), diagnostics:[{code:'gap',message:'Retained history has a gap'}]}).mockRejectedValueOnce(new Error('History denied')).mockResolvedValueOnce(page([entry('recovered')]));
  const view = render(<JobConversation tenant="tenant" job="job" refreshKey={0} onOpen={vi.fn()} />);
  await screen.findByText('Retained history has a gap');
  view.rerender(<JobConversation tenant="tenant" job="job" refreshKey={2} onOpen={vi.fn()} />);
  await screen.findByText(/History denied.*page Refresh button/);
  expect(screen.getByText('Occurrence: one')).toBeInTheDocument();
  view.rerender(<JobConversation tenant="tenant" job="job" refreshKey={3} onOpen={vi.fn()} />);
  await screen.findByText('Occurrence: recovered');
 });
 it('CT-022 ignores obsolete request and tenant details even when transport ignores abort', async () => {
  vi.mocked(getConversation).mockResolvedValue(page([]));
  let resolve!: (v: any) => void;
  const detail = (id: string) => ({...entry(id),snapshot_revision:1,snapshot_token:'s',observed_at:'now',completeness:'partial',request_manifest:[],response_manifest:[],actionability:{actionable:false,observation:'read_only',observed_at:'now'}});
  vi.mocked(getConversationEntry).mockImplementationOnce(() => new Promise(r => {resolve = r;})).mockResolvedValueOnce(detail('current'));
  const view = render(<JobConversation tenant="tenant" job="job" occurrence="old" onOpen={vi.fn()} />);
  view.rerender(<JobConversation tenant="other" job="job" occurrence="current" onOpen={vi.fn()} />);
  await screen.findByText('Occurrence: current');
  await act(async () => resolve(detail('obsolete')));
  expect(screen.queryByText('Occurrence: obsolete')).not.toBeInTheDocument();
  expect(vi.mocked(getConversationEntry).mock.calls[0][3]?.signal?.aborted).toBe(true);
 });

});

describe('CC compact presentation and saved interpretation', () => {
 it('CC-002 CC-008 keeps full evidence behind Details, expands feedback and omits pending answers', () => {
  const feedback = 'Complete feedback '.repeat(100);
  const {container,rerender} = render(<HistoryPair entry={{...entry(),accepted_response:{present:true,raw:feedback},diagnostics:[{code:'gap',message:'Original diagnostic'}]}} />);
  expect(container.querySelector('details')).not.toHaveAttribute('open');
  expect(container.querySelector('details')).toHaveTextContent('Original diagnostic');
  expect(container.querySelector('details')).toHaveTextContent('invoke');
  fireEvent.click(screen.getByRole('button',{name:'Expand feedback'}));
  expect(container.querySelector('.conversation-answer.expanded')).toHaveTextContent(feedback.trim());
  rerender(<HistoryPair entry={{...entry(),kind:'ordinary',state:'pending',accepted_response:{present:false}}} />);
  expect(screen.queryByLabelText('Accepted response')).toBeNull();
  expect(container.querySelector('details')).toHaveTextContent('Proceed?');
 });
});

describe('CC-017 snapshot recovery', () => {
 const document={kind:'stored',name:'original.md',stored:{key:{jobId:'producer',taskOrdinal:4,name:'original.md',sizeBytes:10}}};
 const detail=(token:string)=>({...entry('selected'),snapshot_token:token,completeness:'complete',raw_form:{question:'Saved instructions',documents:{original:document},document_metadata:{original:{order:0,title:'Original title'}}},request_manifest:[{artifact_id:'artifact',ref:document,associations:[],availability:'retained'}],response_manifest:[]} as any);
 it('relocates exactly the selected occurrence in a fresh snapshot after byte expiry', async()=>{
  vi.mocked(getConversation).mockResolvedValue({...page([entry('selected')]),snapshot_token:'fresh'});
  vi.mocked(getConversationEntry).mockResolvedValueOnce(detail('expired')).mockResolvedValueOnce(detail('fresh'));
  const transport=vi.fn(async(url:string)=>url.includes('snapshot_token=expired') ? new Response(JSON.stringify({code:'history_snapshot_expired',error:'expired'}),{status:410}) : new Response('# Retained original'));
  vi.stubGlobal('fetch',transport);
  render(<JobConversation tenant="tenant" job="job" occurrence="selected" onOpen={vi.fn()} />);
  await screen.findByRole('heading',{name:'Retained original'});
  expect(getConversationEntry).toHaveBeenLastCalledWith('tenant','job','selected',expect.objectContaining({snapshotToken:'fresh'}));
  expect(transport.mock.calls.every(([url])=>url.includes('entry_id=selected&artifact_id=artifact'))).toBe(true);
  vi.unstubAllGlobals();
 });
 it('keeps review text and shows unavailable selected occurrence without retry loops', async()=>{
  vi.mocked(getConversation).mockResolvedValue(page([]));
  vi.mocked(getConversationEntry).mockResolvedValue(detail('expired'));
  const transport=vi.fn(async()=>new Response(JSON.stringify({code:'history_snapshot_expired',error:'expired'}),{status:410}));vi.stubGlobal('fetch',transport);
  render(<JobConversation tenant="tenant" job="job" occurrence="selected" onOpen={vi.fn()} />);
  await screen.findByText(/Snapshot recovery failed/);
  expect(screen.getByLabelText('Review context')).toHaveTextContent('Saved instructions');
  expect(getConversationEntry).toHaveBeenCalledTimes(1);expect(transport).toHaveBeenCalledTimes(1);
  vi.unstubAllGlobals();
 });
});

describe('CC compact occurrence and time provenance',()=>{
 it('CC-003 CC-005 pairs response with its request in API order and labels reported receipt time',async()=>{
  const entries=['second','first'].map(id=>({...entry(id),request_id:'same',requested_at:{present:true,raw:'2025-01-02T14:30:00Z'},actor:{...entry().actor,receipt:{submitted_at:'2025-01-02T14:35:00Z',actor:{id:'j@j.com',kind:'human'}}}}));
  vi.mocked(getConversation).mockResolvedValue(page(entries));
  const {container}=render(<JobConversation tenant="tenant" job="job" onOpen={vi.fn()}/>);
  await screen.findByText('Occurrence: second');
  expect([...container.querySelectorAll('.conversation-pair')].map(el=>el.getAttribute('data-occurrence'))).toEqual(['second','first']);
  for(const pair of container.querySelectorAll('.conversation-pair'))expect([...pair.children].map(el=>el.getAttribute('aria-label'))).toEqual(['Recorded request step','Accepted response']);
  expect(screen.getAllByText(/Reported time:/)).toHaveLength(2);
  expect(screen.getAllByText(/Reported time:/)[0]).toHaveAttribute('title');
 });
 it('CC-019 refreshes the whole visible range in one snapshot and retains entries on inconsistent refresh',async()=>{
  vi.mocked(getConversation).mockResolvedValueOnce(page([entry('one')],'next')).mockResolvedValueOnce(page([entry('two')])).mockResolvedValueOnce({...page([entry('new')],'fresh-next'),snapshot_token:'fresh'}).mockResolvedValueOnce({...page([entry('invalid')]),snapshot_token:'wrong'});
  const view = render(<JobConversation tenant="tenant" job="job" refreshKey={0} onOpen={vi.fn()}/>);
  fireEvent.click(await screen.findByText('Load more events'));await screen.findByText('Occurrence: two');
  view.rerender(<JobConversation tenant="tenant" job="job" refreshKey={4} onOpen={vi.fn()} />);await screen.findByText(/snapshot changed during refresh/);
  expect(screen.getByText('Occurrence: one')).toBeInTheDocument();expect(screen.getByText('Occurrence: two')).toBeInTheDocument();expect(screen.queryByText('Occurrence: invalid')).toBeNull();
 });
 it('CC-023 explicitly shows an empty retained history',async()=>{
  vi.mocked(getConversation).mockResolvedValue(page([]));render(<JobConversation tenant="tenant" job="job" onOpen={vi.fn()}/>);
  await screen.findByText('No retained events in this history.');
 });
});

describe('current conversation activity', () => {
 const metadata = (status: string, extra = {}) => ({status,store:'ACTIVE',...extra}) as import('@colony2/shared/types').RecipeJob;
 it.each([['ACTIVE','Running'], ['READY','Pending'], ['PENDING_JOBS','Pending'], ['AWAITING_FUTURE','Pending'], ['CRASH_CONCERN','Worker unresponsive'], ['EXPIRED','Expired']])('CV-009 CV-010 renders %s from metadata', (status,label) => {
  render(<CurrentActivity metadata={metadata(status)} freshness="loaded" />);
  expect(screen.getByText(label)).toBeInTheDocument();
  expect(screen.getAllByLabelText('Current job activity')).toHaveLength(1);
  if (status === 'PENDING_JOBS') expect(screen.getByText('Waiting for jobs')).toBeInTheDocument();
  if (status === 'AWAITING_FUTURE') expect(screen.getByText('Scheduled')).toBeInTheDocument();
 });
 it('CV-010 gives cancellation request precedence', () => {
  render(<CurrentActivity metadata={metadata('ACTIVE',{cancel_requested:true})} />);
  expect(screen.getByText('Cancellation requested')).toBeInTheDocument();
 });
 it.each([{status:'COMPLETED'}, {status:'CANCELLED'}, {status:'READY',store:'ARCHIVED'}, {status:'COMPLETED',completion_status:'custom'}, {status:'READY',store:'ARCHIVED',completion_status:'success'}])('CV-010 omits terminal activity %j', extra => {
  render(<CurrentActivity metadata={metadata('ACTIVE',extra)} />);
  expect(screen.queryByLabelText('Current job activity')).not.toBeInTheDocument();
 });
 it('CV-013 replaces one tail and marks failed observations stale', () => {
  const view = render(<CurrentActivity metadata={metadata('ACTIVE')} freshness="loaded" />);
  view.rerender(<CurrentActivity metadata={metadata('READY')} freshness="failed" />);
  expect(screen.getByText('Pending')).toBeInTheDocument();
  expect(screen.queryByText('Running')).not.toBeInTheDocument();
  expect(screen.getByText(/Status stale/)).toBeInTheDocument();
  expect(screen.getAllByLabelText('Current job activity')).toHaveLength(1);
  view.rerender(<CurrentActivity freshness="failed" />);
  expect(screen.getByText('Status unavailable')).toBeInTheDocument();
 });
});

describe('publication event ordering',()=>{
 it('CV-002 CV-003 CV-005 CV-006 keeps interleaved events, identical SHAs, bounded text and nullable messages',async()=>{
  const merge={entry_id:'merge',tenant_id:'tenant',job_id:'job',job_attempt:2,task_attempt:3,invocation_id:'merge/attempt:3',result_ordinal:2,operation:'merge:merge',evidence_availability:'recorded',candidate_sha:'same-sha',candidate_subject:'<script>unsafe</script>'+ 'x'.repeat(200),publication_branch:'refs/heads/main',publication_disposition:'published'};
  vi.mocked(getConversation).mockResolvedValue({...page([entry()]),merges:[merge,{...merge,entry_id:'second',candidate_subject:null,publication_disposition:'already_present'}],events:[{ordinal:1,invocation_id:'invoke',kind:'request_generation',entry_id:'one'},{ordinal:2,invocation_id:'merge',kind:'merge_completion',entry_id:'merge'},{ordinal:3,invocation_id:'invoke',kind:'response_acceptance',entry_id:'one'},{ordinal:4,invocation_id:'second',kind:'merge_completion',entry_id:'second'},{ordinal:4,invocation_id:'second',kind:'merge_completion',entry_id:'second'}]});
  const {container}=render(<JobConversation tenant="tenant" job="job" onOpen={vi.fn()}/>);
  await screen.findAllByLabelText('Verified publication');
  expect([...container.querySelectorAll('.conversation-step')].slice(0,4).map(n=>n.getAttribute('aria-label'))).toEqual(['Recorded request step','Verified publication','Accepted response','Verified publication']);
  expect(screen.getAllByLabelText('Verified publication')).toHaveLength(2);
  expect(container.querySelector('script')).toBeNull();
  expect(screen.getByText(merge.candidate_subject.slice(0,160)+'…')).toBeInTheDocument();
  expect(screen.getAllByText('Commit message not recorded').length).toBeGreaterThan(0);
  expect(screen.getByText('Publication confirmed — already present')).toBeInTheDocument();
 });
});

it('CX-001 completion invalidates an older history read and preserves the final snapshot', async () => {
 const meta = (status: string) => ({status,store:'ACTIVE'}) as import('@colony2/shared/types').RecipeJob;
 let resolveOld!: (value: ReturnType<typeof page>) => void;
 const old = new Promise<ReturnType<typeof page>>(resolve => { resolveOld = resolve; });
 vi.mocked(getConversation).mockResolvedValueOnce(page([entry('initial')])).mockReturnValueOnce(old).mockResolvedValue({...page([{...entry('final'),title:{present:true,raw:'Final snapshot request'}}]),snapshot_token:'final'});
 const view=render(<JobConversation tenant="tenant" job="job" metadata={meta('ACTIVE')} refreshKey={0} onOpen={vi.fn()}/>);
 await waitFor(()=>expect(getConversation).toHaveBeenCalledTimes(1));
 await screen.findByText('Original review');
 view.rerender(<JobConversation tenant="tenant" job="job" metadata={meta('ACTIVE')} refreshKey={1} onOpen={vi.fn()}/>);
 await waitFor(()=>expect(getConversation).toHaveBeenCalledTimes(2));
 const oldSignal=vi.mocked(getConversation).mock.calls[1][2]?.signal;
 view.rerender(<JobConversation tenant="tenant" job="job" metadata={meta('COMPLETED')} refreshKey={1} onOpen={vi.fn()}/>);
 await waitFor(()=>expect(getConversation).toHaveBeenCalledTimes(3));
 expect(oldSignal?.aborted).toBe(true);
 await act(async()=>{resolveOld({...page([entry('obsolete')]),snapshot_token:'obsolete'});await old;});
 expect(screen.queryByLabelText('Current job activity')).not.toBeInTheDocument();
 await screen.findByText('Final snapshot request');
 expect(screen.queryByText('Original review')).not.toBeInTheDocument();
});

it('CR-007 loads initially without a private refresh button or timer', async () => {
 vi.useFakeTimers({toFake:['setInterval','clearInterval']});
 try {
  vi.mocked(getConversation).mockResolvedValue(page());
  render(<JobConversation tenant="tenant" job="job" onOpen={vi.fn()} />);
  await screen.findByText('Occurrence: one');
  expect(screen.queryByRole('button',{name:'Refresh history'})).toBeNull();
  await act(async()=>{await vi.advanceTimersByTimeAsync(30000);});
  expect(getConversation).toHaveBeenCalledTimes(1);
 } finally {vi.useRealTimers();}
});

it('CR-012 Retry request retries only the selected historical request',async()=>{
 vi.mocked(getConversation).mockResolvedValue(page([]));
 vi.mocked(getConversationEntry).mockRejectedValueOnce(new Error('Unavailable')).mockRejectedValueOnce(new Error('Still unavailable'));
 render(<JobConversation tenant="tenant" job="job" occurrence="exact/?" onOpen={vi.fn()}/>);
 fireEvent.click(await screen.findByRole('button',{name:'Retry request'}));
 await screen.findByText(/Still unavailable/);
 expect(getConversation).toHaveBeenCalledTimes(1);
 expect(getConversationEntry).toHaveBeenCalledTimes(2);
 expect(getConversationEntry).toHaveBeenLastCalledWith('tenant','job','exact/?',expect.anything());
});

it('CR-009 rebuilds all visible pages in a fresh snapshot',async()=>{
 vi.mocked(getConversation).mockResolvedValueOnce(page([entry('one')],'old-next')).mockResolvedValueOnce(page([entry('two')]))
 .mockResolvedValueOnce({...page([entry('fresh-one')],'fresh-next'),snapshot_token:'fresh'})
 .mockResolvedValueOnce({...page([entry('fresh-one'),entry('fresh-two')]),snapshot_token:'fresh'});
 const view=render(<JobConversation tenant="tenant" job="job" refreshKey={0} onOpen={vi.fn()}/>);
 fireEvent.click(await screen.findByText('Load more events'));
 await screen.findByText('Occurrence: two');
 view.rerender(<JobConversation tenant="tenant" job="job" refreshKey={1} onOpen={vi.fn()}/>);
 await screen.findByText('Occurrence: fresh-two');
 expect(screen.getAllByText('Occurrence: fresh-one')).toHaveLength(1);
 expect(screen.queryByText('Occurrence: one')).toBeNull();
 expect(vi.mocked(getConversation).mock.calls[3][2]?.pageToken).toBe('fresh-next');
});
