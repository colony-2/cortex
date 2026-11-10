import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import JobConversation, { HistoryPair } from './JobConversation';
import { APIError, getConversation, getConversationEntry, type HistoryEntry } from '@colony2/shared';
vi.mock('@colony2/shared', async () => ({ ...await vi.importActual('@colony2/shared'), getConversation: vi.fn(), getConversationEntry: vi.fn() }));
const entry = (id = 'one'): HistoryEntry => ({entry_id:id,kind:'review',title:{present:true,raw:'Original review'},summary:{present:false},state:'answered',requested_at:{present:false},locator:{invocation_id:'invoke'},form_fields:{question:'Proceed?'},accepted_response:{present:true,raw:{decision:'approve',feedback:'Keep original'}},response_source:'autofill',actor:{provenance:'reported_receipt',receipt:{actor:{kind:'human',id:'caller'}},legacy_user_id:{present:false}}});
const page = (entries = [entry()], next?: string) => ({tenant_id:'tenant',job_id:'job',entries,snapshot_revision:1,snapshot_token:'snapshot',observed_at:'now',completeness:'partial',next_page_token:next});
beforeEach(() => vi.clearAllMocks());
describe('conversation retained evidence', () => {
 it('CT-009 CT-010 CT-016 CT-034 renders exact accepted feedback and caller provenance', () => {
  render(<HistoryPair entry={entry()} />);
  expect(screen.getByText(/Keep original/)).toBeInTheDocument();
  expect(screen.getByText(/Caller-reported identity/)).toBeInTheDocument();
  expect(screen.getByText('Requested: not recorded')).toBeInTheDocument();
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
  render(<JobConversation tenant="tenant" job="job" running={false} onOpen={vi.fn()} />);
  fireEvent.click(await screen.findByText('Load more requests'));
  await screen.findByText('Occurrence: two');
  expect(screen.getAllByText('Occurrence: one')).toHaveLength(1);
 });
 it('CT-011 CT-012 preserves supplied order and repeated request identities', async () => {
  vi.mocked(getConversation).mockResolvedValue(page(['two','one'].map(id => ({...entry(id), requested_at:{present:true,raw:'same timestamp'}, locator:{invocation_id:id,collection_ordinals:[7]}}))));
  render(<JobConversation tenant="tenant" job="job" running={false} onOpen={vi.fn()} />);
  await screen.findByText('Occurrence: two');
  expect(screen.getAllByText(/Occurrence:/).map(x => x.textContent)).toEqual(['Occurrence: two','Occurrence: one']);
 });
 it('CT-013 CT-033 retains entries on expiry and refreshes from a fresh snapshot', async () => {
  vi.mocked(getConversation).mockResolvedValueOnce(page([entry()], 'opaque')).mockRejectedValueOnce(new APIError('expired',410,'history_snapshot_expired')).mockResolvedValueOnce(page([entry(),entry('two')]));
  render(<JobConversation tenant="tenant" job="job" running={false} onOpen={vi.fn()} />);
  fireEvent.click(await screen.findByText('Load more requests'));
  await screen.findByText(/History snapshot expired/);
  expect(screen.getByText('Occurrence: one')).toBeInTheDocument();
  fireEvent.click(screen.getByText('Refresh history'));
  await screen.findByText('Occurrence: two');
  expect(vi.mocked(getConversation).mock.calls[2][2]?.pageToken).toBeUndefined();
 });
 it('CT-014 CT-019 opens exact unloaded detail, keeping missing requests unavailable', async () => {
  vi.mocked(getConversation).mockResolvedValue(page([]));
  vi.mocked(getConversationEntry).mockRejectedValue(new Error('not retained'));
  render(<JobConversation tenant="tenant" job="job" occurrence="old/?" running={false} onOpen={vi.fn()} />);
  await screen.findByText('Request unavailable');
  expect(getConversationEntry).toHaveBeenCalledWith('tenant','job','old/?',expect.anything());
  expect(screen.queryByText('Original review')).not.toBeInTheDocument();
 });
 it('CT-022 discards obsolete job loads', async () => {
  let resolve!: (v: ReturnType<typeof page>) => void;
  vi.mocked(getConversation).mockImplementationOnce(() => new Promise(r => {resolve = r;})).mockResolvedValue({...page([entry('new')]),job_id:'other'});
  const view = render(<JobConversation tenant="tenant" job="job" running={false} onOpen={vi.fn()} />);
  view.rerender(<JobConversation tenant="tenant" job="other" running={false} onOpen={vi.fn()} />);
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
  render(<JobConversation tenant="tenant" job="job" running={false} onOpen={vi.fn()} />);
  await screen.findByText('Retained history has a gap');
  fireEvent.click(screen.getByText('Refresh history'));
  await screen.findByText(/History denied/);
  expect(screen.getByText('Occurrence: one')).toBeInTheDocument();
  fireEvent.click(screen.getByText('Refresh history'));
  await screen.findByText('Occurrence: recovered');
 });
 it('CT-022 ignores obsolete request and tenant details even when transport ignores abort', async () => {
  vi.mocked(getConversation).mockResolvedValue(page([]));
  let resolve!: (v: any) => void;
  const detail = (id: string) => ({...entry(id),snapshot_revision:1,snapshot_token:'s',observed_at:'now',completeness:'partial',request_manifest:[],response_manifest:[],actionability:{actionable:false,observation:'read_only',observed_at:'now'}});
  vi.mocked(getConversationEntry).mockImplementationOnce(() => new Promise(r => {resolve = r;})).mockResolvedValueOnce(detail('current'));
  const view = render(<JobConversation tenant="tenant" job="job" occurrence="old" running={false} onOpen={vi.fn()} />);
  view.rerender(<JobConversation tenant="other" job="job" occurrence="current" running={false} onOpen={vi.fn()} />);
  await screen.findByText('Occurrence: current');
  await act(async () => resolve(detail('obsolete')));
  expect(screen.queryByText('Occurrence: obsolete')).not.toBeInTheDocument();
  expect(vi.mocked(getConversationEntry).mock.calls[0][3]?.signal?.aborted).toBe(true);
 });

});
