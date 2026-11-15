import {act,renderHook,waitFor} from '@testing-library/react';
import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest';
import * as shared from '@colony2/shared';
import {reviewV2Fixture} from '../../../shared/src/test/reviewV2Fixture';
import {useReviewV2} from './useReviewV2';
vi.mock('@colony2/shared',async()=>{const actual=await vi.importActual<typeof shared>('@colony2/shared');return {...actual,getConversation:vi.fn(),getConversationEntry:vi.fn()};});
const request=reviewV2Fixture();
const form={request_id:'native',presentation:{type:'review/v2'},request,request_schema:{type:'object'},response_schema:{type:'object'}};
const entry={entry_id:'entry',request_id:'native',form_fields:form,locator:{generation_ordinal:12},accepted_response:{present:true},actor:{receipt:{request_id:'native',submission_id:'saved'}}} as unknown as shared.HistoryEntry;
const page=(extra={})=>({tenant_id:'tenant',job_id:'job',snapshot_token:'snapshot',snapshot_revision:1,entries:[entry],completeness:'complete',activity:{availability:'available',value:{status:'ACTIVE',store:'ACTIVE',job_id:'job',tenant_id:'tenant'}},...extra}) as shared.HistoryPage;
const detail=()=>({...entry,raw_form:form,raw_accepted_output:{response:{direction:'ask',feedback:' exact ',attachments:[],settings:{removed:null}},receipt:{request_id:'native',submission_id:'saved'}},request_manifest:[],response_manifest:[],actionability:{actionable:false}}) as shared.HistoryDetail;
beforeEach(()=>{
 vi.mocked(shared.getConversation).mockResolvedValue(page());vi.mocked(shared.getConversationEntry).mockResolvedValue(detail());
 vi.spyOn(shared.inputActivityService,'getInputDetails').mockRejectedValue(new shared.APIError('No pending input',404));
});
afterEach(()=>{vi.restoreAllMocks();vi.clearAllMocks();vi.useRealTimers();});
describe('selected review recovery',()=>{
 it('keeps verified originals while accepted history is delayed and after that refresh fails',async()=>{
  const frozen=detail();frozen.accepted_response={present:false};frozen.raw_accepted_output=undefined;
  vi.mocked(shared.getConversationEntry).mockResolvedValue(frozen);
  vi.mocked(shared.inputActivityService.getInputDetails).mockResolvedValue({jobId:'job',status:'pending',startTime:'now',form} as shared.UserInputDetails);
  const {result}=renderHook(()=>useReviewV2('tenant','job','review-one'));
  await waitFor(()=>expect(result.current.view?.phase).toBe('actionable'));
  let reject!:(reason:Error)=>void;
  vi.mocked(shared.getConversation).mockImplementationOnce(()=>new Promise((_,fail)=>{reject=fail;}));
  act(()=>result.current.accepted(request,{response:{direction:'ask',feedback:'Keep reading.',attachments:[],settings:{}},receipt:{request_id:'native',submission_id:'new'}}));
  expect(result.current.view?.phase).toBe('waiting');
  expect(result.current.view?.documentHistory).toBe(frozen);
  expect(result.current.view?.history).toBeUndefined();
  await act(async()=>reject(new Error('History unavailable')));
  expect(result.current.view?.phase).toBe('unavailable');
  expect(result.current.view?.documentHistory).toBe(frozen);
  expect(result.current.view?.conversation[0].text).toBe('Keep reading.');
 });
 it('follows every history page and recovers accepted text during a request gap',async()=>{
  vi.mocked(shared.getConversation).mockImplementation(async(_t,_j,options)=>options?.pageToken?page():page({entries:[{...entry,entry_id:'other',form_fields:{question:'other'}}],next_page_token:'next'}));
  const {result}=renderHook(()=>useReviewV2('tenant','job','review-one'));
  await waitFor(()=>expect(result.current.view?.phase).toBe('waiting'));
  expect(shared.getConversation).toHaveBeenCalledWith('tenant','job',expect.objectContaining({pageToken:'next'}));
  expect(result.current.view?.conversation[0].text).toBe(' exact ');expect(result.current.view?.conversation[0].settings).toEqual({removed:null});
 });
 it('restarts discovery after snapshot expiry',async()=>{
  vi.mocked(shared.getConversationEntry).mockRejectedValueOnce(new shared.APIError('Expired',410,'history_snapshot_expired')).mockResolvedValue(detail());
  const {result}=renderHook(()=>useReviewV2('tenant','job','review-one'));
  await waitFor(()=>expect(result.current.view?.phase).toBe('waiting'));expect(shared.getConversation).toHaveBeenCalledTimes(2);
 });
 it('preserves saved content and disables actions on failed current observations',async()=>{
  const {result}=renderHook(()=>useReviewV2('tenant','job','review-one'));
  await waitFor(()=>expect(result.current.view?.phase).toBe('waiting'));
  vi.mocked(shared.inputActivityService.getInputDetails).mockRejectedValueOnce(new shared.APIError('Offline',503));
  await act(()=>result.current.refresh());
  expect(result.current.view?.request.review_id).toBe('review-one');expect(result.current.view?.phase).toBe('unavailable');expect(result.current.error).toContain('Offline');
 });
 it('coalesces concurrent refreshes and ignores obsolete unmounted reads',async()=>{
  let release!:(v:shared.HistoryPage)=>void;
  vi.mocked(shared.getConversation).mockImplementationOnce(()=>new Promise(resolve=>{release=resolve;}));
  const {result,unmount}=renderHook(()=>useReviewV2('tenant','job','review-one'));
  act(()=>{void result.current.refresh();void result.current.refresh();});expect(shared.getConversation).toHaveBeenCalledTimes(1);
  unmount();await act(async()=>release(page()));expect(shared.getConversationEntry).not.toHaveBeenCalled();
 });
 it('refreshes on input and reconnect events without adding broad polling',async()=>{
  const {result}=renderHook(()=>useReviewV2('tenant','job','review-one'));
  await waitFor(()=>expect(result.current.view?.phase).toBe('waiting'));
  const count=vi.mocked(shared.getConversation).mock.calls.length;
  act(()=>shared.inputActivityService.emit('connected'));
  await waitFor(()=>expect(shared.getConversation).toHaveBeenCalledTimes(count+1));
 });
 it('stops waiting timers at actual terminal outcomes',async()=>{
  vi.useFakeTimers();vi.mocked(shared.getConversation).mockResolvedValue(page({activity:{availability:'available',value:{status:'COMPLETED',completion_status:'failed_app',job_id:'job',tenant_id:'tenant'}}}));
  const {result}=renderHook(()=>useReviewV2('tenant','job','review-one'));
  await act(async()=>{await vi.advanceTimersByTimeAsync(1);});expect(result.current.view?.phase).toBe('terminal');const count=vi.mocked(shared.getConversation).mock.calls.length;
  await act(async()=>{await vi.advanceTimersByTimeAsync(10000);shared.inputActivityService.emit('connected');});expect(shared.getConversation).toHaveBeenCalledTimes(count);
 });
});
