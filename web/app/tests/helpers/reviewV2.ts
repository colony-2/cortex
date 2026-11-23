import { expect, type Page } from '@playwright/test';
import { amendedReviewV2Fixture } from '../../../shared/src/test/reviewV2Fixture';
import type { ReviewV2Output } from '../../../shared/src/reviewV2';
import { jobIdentity, mockCells } from './cells';

export const v2Path='/project/v2/reviews/one';
export const v2Ids=['review/plan.md','checks/雪 &+%.md','review/source.txt','review/change.patch','review/image.png'];
export async function reviewV2BrowserFixture(page:Page, compactDocument=false) {
 const request=amendedReviewV2Fixture();request.title='Make reviews easier to read';request.question='Can I proceed with these viewer improvements?';request.review_id='logical/雪 &+%';
 request.documents=Object.fromEntries(v2Ids.map((id,i)=>[id,{kind:'stored' as const,name:['plan.md','checks.md','source.txt','change.patch','image.png'][i],stored:{key:{jobId:'producer',taskOrdinal:1,name:['review/plan.md','review/checks.md','review/source.txt','review/change.patch','review/image.png'][i],sizeBytes:100}}}]));
 request.overview_document_id=v2Ids[0];
 request.document_metadata=Object.fromEntries(v2Ids.map((id,i)=>[id,{title:['Viewer changes','Acceptance checks','Source notes','Code changes','Layout image'][i],description:['Read the proposed reading and response flow.','Verify navigation, drafts, and responsive layout.','Supporting source material.','Inspect the sample patch.','Inspect an attached raster image.'][i],role:i===0?'primary' as const:'supporting' as const,order:i,...(i===0?{source_document_ids:[v2Ids[1]]}:{})}]));
 const raster=await page.evaluate(()=>{const c=document.createElement('canvas');c.width=900;c.height=1400;const ctx=c.getContext('2d')!;ctx.fillStyle='#e6f4ff';ctx.fillRect(0,0,900,1400);ctx.fillStyle='#0958d9';ctx.font='48px sans-serif';ctx.fillText('Review layout reference',60,120);return c.toDataURL().split(',')[1];});
 const bodies=['# Viewer changes\n\nKeep documents in browser history and put responses within reach.\n\n[Open checks](checks.md#acceptance-checks)\n\n'+('Compare the material while keeping your response draft.\n\n'.repeat(compactDocument?1:65))+'FINAL DOCUMENT CONTENT', '# Acceptance checks\n\nConfirm Back and Forward preserve your notes.\n\n[Read source notes](source.txt)\n\n[Same heading](checks.md#acceptance-checks)', ('Source line with a deliberately wide value '+ 'wide '.repeat(60)+'\n').repeat(100)+'SOURCE END', '--- a/viewer\n+++ b/viewer\n@@ -1,80 +1,80 @@\n'+('-old\n+new\n'.repeat(80))+' DIFF END'];
 const state={request,liveReviewId:'',native:'native-one',pending:true,output:undefined as ReviewV2Output|undefined,submits:[] as any[],details:0,reads:[] as string[],complete:[] as string[],failures:new Map<string,number>(),holds:new Map<string,Promise<void>>(),rejected:false,historyError:false};
 const form=()=>({request_id:state.native,presentation:{type:'review/v2'},request:state.request,request_schema:{type:'object'},response_schema:{type:'object'}});
 const activity=()=>({...jobIdentity,tenant_id:'v2',job_id:'one',status:'ACTIVE',store:'ACTIVE',title:'Review viewer usability'});
 const entry=()=>({entry_id:'entry',request_id:state.native,kind:'structured',title:{present:true,raw:state.request.title},summary:{present:false},requested_at:{present:false},state:state.pending?'pending':'answered',locator:{generation_ordinal:state.native==='native-one'?1:2},form_fields:form(),accepted_response:{present:!!state.output},actor:{receipt:state.output?.receipt}});
 const detail=()=>({...entry(),snapshot_token:'snapshot',raw_form:form(),raw_accepted_output:state.output,request_manifest:Object.entries(state.request.documents).map(([id,ref])=>({artifact_id:id,ref,availability:'available',associations:[]})),response_manifest:[],actionability:{actionable:state.pending}});
 await page.route('**/api/**',r=>r.fulfill({status:404,json:{error:'No fixture for this endpoint'}}));
 await page.route('**/api/projects',r=>r.fulfill({json:[{...jobIdentity,tenant_id:'v2'}]}));
 await mockCells(page,['platform']);
 await page.route('**/user-inputs/pending',r=>r.fulfill({json:state.pending?[{...jobIdentity,id:'one',client_payload_revision:state.native==='native-one'?1:2,request_id:state.native}]:[]}));
 await page.route(/\/jobs\/one(?:\?.*)?$/,r=>r.fulfill({json:activity()}));
 await page.route('**/jobs/one/children?*',r=>r.fulfill({json:{jobs:[]}}));
 await page.route('**/tree-node',r=>{const id=new URL(r.request().url()).pathname.split('/').slice(-2)[0];return r.fulfill({json:{node:{tenant_id:'v2',job_id:id,job_type:'build',status:'ACTIVE',...(id==='one'?{title:'Review viewer usability',parent_job_id:'parent'}:id==='parent'?{prompt:'Improve the review reading and response flow',parent_job_id:'root'}:{title:'Build a useful job workspace',store:'ARCHIVED'})},observed_at:'now'}});});
 await page.route('**/user-inputs/one',r=>{state.details++;return r.fulfill(state.pending?{json:{jobId:'one',status:'pending',startTime:'2026-11-16T12:00:00Z',form:{...form(),request:{...state.request,review_id:state.liveReviewId||state.request.review_id}}}}:{status:204,body:''});});
 await page.route('**/jobs/one/conversation?*',r=>r.fulfill(state.historyError?{status:503,json:{error:'History unavailable'}}:{json:{tenant_id:'v2',job_id:'one',snapshot_token:'snapshot',snapshot_revision:1,entries:[entry()],completeness:'complete',activity:{availability:'available',value:activity()}}}));
 await page.route('**/jobs/one/conversation/entry?*',r=>r.fulfill({json:detail()}));
 await page.route('**/jobs/producer/stored-document?*',async r=>{
  const name=new URL(r.request().url()).searchParams.get('name');
  const id=Object.keys(state.request.documents).find(id=>state.request.documents[id].stored.key.name===name)!;state.reads.push(id);
  if(state.holds.has(id))await state.holds.get(id);
  try{
   if(state.failures.has(id))return await r.fulfill({status:state.failures.get(id),json:{error:'Could not load this document'}});
   const index=v2Ids.indexOf(id);
   await r.fulfill(index===4?{contentType:'image/png',body:Buffer.from(raster,'base64')}:{contentType:index<2?'text/markdown':'text/plain',body:bodies[index]});
  }finally{state.complete.push(id);}
 });
 await page.route('**/reviews/one/respond',async r=>{
  const body=r.request().postData()!;const match=body.match(/name="submission"\r\n\r\n([^\r]*)/);expect(match).toBeTruthy();
  const submitted=JSON.parse(match![1]);state.submits.push(submitted);
  if(state.rejected)return r.fulfill({status:503,json:{error:'Upload could not be accepted'}});
  state.output={response:submitted.response,receipt:{request_id:submitted.request_id,submission_id:submitted.submission_id}};
  state.pending=false;return r.fulfill({json:state.output});
 });
 return {state,async successor(){
  const output=state.output!;
  state.request={...state.request,conversation:[...state.request.conversation,{role:'human',text:output.response.feedback,direction:output.response.direction,attachments:[],settings:output.response.settings,receipt:output.receipt},{role:'agent',text:'The documents and drafts stay available. I can proceed after your approval.',attachments:[]}]};
  state.native='native-two';state.pending=true;state.output=undefined;
 }};
}
export async function authorizeV2(page:Page,baseURL:string){
 await page.context().addCookies([{name:'colony2:user:email',value:'review@example.com',url:baseURL}]);
 await page.addInitScript(()=>{(window as any).EventSource=class {readyState=1;close(){}addEventListener(){}removeEventListener(){}};});
}
export async function v2Selected(page:Page,index:number){
 await expect.poll(()=>new URL(page.url()).searchParams.get('document_id')).toBe(v2Ids[index]);
 await expect(page.getByLabel('Loading document')).toHaveCount(0);
 await expect(page.getByLabel('Selected document')).toContainText(index===0?'Viewer changes':index===1?'Acceptance checks':index===2?'Source notes':index===3?'Code changes':'Layout image');
}
export async function v2Catalog(page:Page,index:number){
 if(await page.getByRole('button',{name:'Show documents',exact:true}).isVisible())await page.getByRole('button',{name:'Show documents',exact:true}).click();
 const nav=page.getByRole('navigation',{name:'Review documents'});
 const buttons=nav.getByRole('button');
 await buttons.nth(index).click();await v2Selected(page,index);
}
