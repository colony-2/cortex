import { describe,it,expect } from 'vitest';
import { changedReviewSettings,hasReviewFeedback,nonblankPattern,parseReviewV2,portablePattern,reviewV2ContributionError,reviewV2DirectionPolicy,validateReviewScalar } from './reviewV2';
import { amendedReviewV2Fixture,reviewV2Fixture } from './test/reviewV2Fixture';

describe('review/v2 declarations',()=>{
 it('uses explicit direction flags with unfamiliar settings',()=>{const r=reviewV2Fixture();expect(parseReviewV2(r)).toBe(r);expect(r.directions[0].feedback_required).toBe(false);expect(changedReviewSettings(r,'ask',{response_style:'brief',citations:false,extra_context:null,examples:0})).toEqual({response_style:'brief',citations:false,extra_context:null});expect(changedReviewSettings(r,'fix',{examples:0})).toEqual({examples:0});});
 it.each([
  ['missing feedback flag',(r:any)=>delete r.directions[0].feedback_required],['missing continuation',(r:any)=>delete r.directions[0].continues_review],
  ['empty direction',(r:any)=>r.directions[0].id=''],['duplicate direction',(r:any)=>r.directions[1].id='go'],['empty settings id',(r:any)=>r.settings[0].id=''],['duplicate setting',(r:any)=>r.settings[1].id='response_style'],
  ['missing value',(r:any)=>delete r.settings[0].value],['unknown applicability',(r:any)=>r.settings[0].applies_to=['missing']],['duplicate applicability',(r:any)=>r.settings[0].applies_to=['go','go']],['invalid current',(r:any)=>r.settings[0].value='unknown'],['invalid default',(r:any)=>r.settings[0].default=0],
  ['missing reset label',(r:any)=>delete r.settings[0].reset_label],['missing default',(r:any)=>delete r.settings[0].default],['control mismatch',(r:any)=>r.settings[1].control='boolean'],['unknown control',(r:any)=>r.settings[0].control='script'],['schema ref',(r:any)=>r.settings[1].schema.$ref='https://example.com'],['schema default',(r:any)=>r.settings[1].schema.default='x'],
  ['contradictory bounds',(r:any)=>r.settings[3].schema.minimum=10],['invalid integer bounds',(r:any)=>r.settings[3].schema={type:'integer',minimum:0.2,maximum:0.8}],['fractional value',(r:any)=>r.settings[3].value=2.5],['nonfinite value',(r:any)=>r.settings[3].value=Infinity],['duplicate numeric choices',(r:any)=>r.settings[0]={...r.settings[0],schema:{type:'number'},value:1,default:1,choices:[{label:'one',value:1},{label:'again',value:1.0}]}],
  ['nonoffered null',(r:any)=>{r.settings[0].schema={type:['string','null']};r.settings[0].value=null;}],
  ['nonoffered default',(r:any)=>{r.settings[0].schema={type:'string'};r.settings[0].default='other';}],
  ['invalid overview',(r:any)=>r.overview_document_id='other'],['unattached metadata',(r:any)=>r.document_metadata.other={}],['long title',(r:any)=>r.document_metadata.design.title='x'.repeat(121)],['control in description',(r:any)=>r.document_metadata.design.description='a\nb'],['invalid source',(r:any)=>r.document_metadata.design.source_document_ids=['other']],['duplicate sources',(r:any)=>r.document_metadata.design.source_document_ids=['design','design']],['negative order',(r:any)=>r.document_metadata.design.order=-1],['external ref',(r:any)=>r.documents.design={kind:'external',url:'https://example.com'}],['invalid turn',(r:any)=>r.conversation=[{role:'agent',text:'x'}]],['missing receipt',(r:any)=>r.conversation=[{role:'human',direction:'ask',text:'x',attachments:[]}]],['nonscalar historical setting',(r:any)=>r.conversation=[{role:'human',direction:'ask',text:'x',attachments:[],receipt:{},settings:{old:{}}}]],
 ] as const)('rejects %s',(_,mutate)=>{const r=reviewV2Fixture();mutate(r);expect(()=>parseReviewV2(r)).toThrow();});
 it('preserves historical settings without applying new declarations',()=>{const r=reviewV2Fixture();r.conversation=[{role:'human',direction:'old',text:' exact ',attachments:[],receipt:{request_id:'old'},settings:{removed:null,citations:'earlier value'}}];expect(parseReviewV2(r).conversation[0].settings).toEqual({removed:null,citations:'earlier value'});});
 it('permits explicit nullable reset choice default',()=>{const r=reviewV2Fixture();const s=r.settings![0];s.schema={type:['string','null']};s.value=null;s.default=null;expect(()=>parseReviewV2(r)).not.toThrow();});
 it('counts Unicode code points and preserves numeric scalar types',()=>{expect(validateReviewScalar('😀',{type:'string',maxLength:1})).toBeUndefined();expect(validateReviewScalar('😀😀',{type:'string',maxLength:1})).toBeTruthy();expect(validateReviewScalar(0,{type:'integer',exclusiveMinimum:0})).toBeTruthy();expect(validateReviewScalar('0',{type:'number'})).toBeTruthy();expect(validateReviewScalar(false,{type:'boolean'})).toBeUndefined();});
 it('does not validate inapplicable drafts or send unchanged values',()=>{const r=reviewV2Fixture();expect(changedReviewSettings(r,'go',{extra_context:'',examples:'',response_style:'detailed'})).toEqual({});expect(()=>changedReviewSettings(r,'ask',{extra_context:''})).toThrow();});
});
describe('per-direction feedback and frozen compatibility',()=>{
 it('keeps both complete wire shapes unchanged and interprets renamed choices',()=>{
  const old=reviewV2Fixture(),next=amendedReviewV2Fixture();
  expect(parseReviewV2(old)).toBe(old);expect(parseReviewV2(next)).toBe(next);
  expect(reviewV2DirectionPolicy(old,old.directions[0])).toEqual({mode:'optional',allowAttachments:true});
  expect(reviewV2ContributionError(old,old.directions[0],'earlier approval notes',1)).toBeUndefined();
  const renamed={...next.directions[0],id:'unfamiliar',label:'Recipe wording'};
  expect(reviewV2ContributionError(next,renamed,'',0)).toBeUndefined();
  expect(reviewV2ContributionError(next,renamed,' ',0)).toMatch(/does not allow feedback/);
  expect(changedReviewSettings(next,'go',{response_style:'brief'})).toEqual({response_style:'brief'});
 });
 it.each([
  ['unknown mode',(r:any)=>r.directions[0].feedback_mode='sometimes'],
  ['missing mode',(r:any)=>delete r.directions[0].feedback_mode],
  ['missing upload rule',(r:any)=>delete r.directions[0].allow_attachments],
  ['null upload rule',(r:any)=>r.directions[0].allow_attachments=null],
  ['forbidden uploads',(r:any)=>r.directions[0].allow_attachments=true],
  ['old flag even null',(r:any)=>r.directions[0].feedback_required=null],
  ['global rule even null',(r:any)=>r.feedback.allow_attachments=null],
  ['global rule even false',(r:any)=>r.feedback.allow_attachments=false],
  ['mixed directions',(r:any)=>r.directions[1]={id:'ask',label:'Question',feedback_required:true,continues_review:true}],
 ] as const)('rejects %s',(_,edit)=>{const r=amendedReviewV2Fixture();edit(r);expect(()=>parseReviewV2(r)).toThrow();});
 it.each(['feedback_mode','allow_attachments'])('rejects new property %s even null in frozen rules',key=>{const r:any=reviewV2Fixture();r.directions[0][key]=null;expect(()=>parseReviewV2(r)).toThrow();});
 it.each([' ','\u0085','\ufeff','\n','text'])('forbids every nonempty approval contribution %j',text=>{const r=amendedReviewV2Fixture();expect(reviewV2ContributionError(r,r.directions[0],text,0)).toBeTruthy();});
 it.each(['ask','fix','annotate'])('requires a new contribution for %s independently of prior conversation or settings',id=>{
  const r=amendedReviewV2Fixture(),d=r.directions.find(d=>d.id===id)!;
  r.conversation=[{role:'agent',text:'Earlier answer',attachments:[]}];
  expect(reviewV2ContributionError(r,d,' \u0085\ufeff',0)).toMatch(/requires feedback/);
  expect(reviewV2ContributionError(r,d,'',1)).toBeUndefined();
  expect(reviewV2ContributionError(r,d,' exact new text ',0)).toBeUndefined();
 });
 it('treats optional text and local attachment permission independently',()=>{
  const r=amendedReviewV2Fixture(),d=r.directions.find(d=>d.id==='other')!;
  expect(reviewV2ContributionError(r,d,' \u0085',0)).toBeUndefined();
  expect(reviewV2ContributionError(r,d,'',1)).toMatch(/does not allow attachments/);
 });
});
describe('shared feedback whitespace',()=>{
 const blanks=[...Array.from({length:5},(_,i)=>i+9),32,133,160,5760,...Array.from({length:11},(_,i)=>8192+i),8232,8233,8239,8287,12288,65279].map(cp=>String.fromCodePoint(cp));
 it.each(blanks)('rejects blank %j',blank=>expect(hasReviewFeedback(blank)).toBe(false));
 it('matches exact agreed blank set without trimming stored text',()=>{expect(hasReviewFeedback(blanks.join(''))).toBe(false);expect(hasReviewFeedback('')).toBe(false);for(const text of [' x ','\u200b','\u180e','😀'])expect(hasReviewFeedback(text)).toBe(true);expect(portablePattern(nonblankPattern).test('\u0085')).toBe(false);});
});
describe('portable Unicode search patterns',()=>{
 it.each(['a$','a.b','\\s','\\d','\\p{L}','(?=a)','(a)\\1','(?i)a','[[:alpha:]]','a{1001}','a{01}','a{1,02}','a+?','a++','(ab)+','[z-a]','a|','[','a{1,}','^','a^','a|()','[a-b-c]','\\u0085','\ud800'])('rejects %j',pattern=>expect(()=>portablePattern(pattern)).toThrow());
 it.each([['[😀-🙏]+','😀🙏','abc'],['a(b|c)','xxab','ad'],['^ab{2,3}','abbb\n','xabb'],['[^\n]','\r','\n'],['a\\.b','xa.b','axxb'],['[a-]','-','x']])('uses search semantics for %j',(pattern,positive,negative)=>{const regex=portablePattern(pattern);expect(regex.test(positive)).toBe(true);expect(regex.test(negative)).toBe(false);});
});
