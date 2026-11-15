import type { ReviewV2Request } from '../reviewV2';
export const reviewV2Fixture = (): ReviewV2Request => ({
 review_id:'review-one',title:'Keep unsent drafts',question:'Approve this direction?',
 documents:{design:{kind:'stored',name:'design.md',stored:{key:{jobId:'child',taskOrdinal:2,name:'review/design.md',sizeBytes:24}}}},
 document_metadata:{design:{title:'Proposed direction',role:'primary',order:0}},overview_document_id:'design',
 directions:[{id:'go',label:'Approve direction',feedback_required:false,continues_review:false},{id:'ask',label:'Ask a question',feedback_required:true,continues_review:true},{id:'fix',label:'Request changes',feedback_required:true,continues_review:false}],
 feedback:{label:'Question or requested change',allow_attachments:true},conversation:[],
 settings:[
  {id:'response_style',control:'choice',label:'Response style',value:'detailed',default:'brief',reset_label:'Use brief answers',choices:[{value:'brief',label:'Brief'},{value:'detailed',label:'Detailed'}],schema:{type:'string',enum:['brief','detailed']},applies_to:['go','ask','fix'],effect:'Selects the report template.'},
  {id:'extra_context',control:'text',label:'Extra context',value:'current',default:null,reset_label:'Use default context',schema:{type:['string','null'],minLength:1,maxLength:40},applies_to:['ask'],effect:'Adds context to the answer.'},
  {id:'citations',control:'boolean',label:'Include citations',value:true,default:false,reset_label:'No citations',schema:{type:'boolean'},applies_to:['ask'],effect:'Controls report citations.'},
  {id:'examples',control:'number',label:'Example count',value:2,default:0,reset_label:'No examples',schema:{type:'integer',minimum:0,maximum:5},applies_to:['fix'],effect:'Controls example count.'}
 ]
});

export const amendedReviewV2Fixture = (): ReviewV2Request => {
 const request=reviewV2Fixture();
 request.feedback={label:'Question or requested change'};
 request.directions=[
  {id:'go',label:'Approve direction',feedback_mode:'forbidden',allow_attachments:false,continues_review:false},
  {id:'ask',label:'Ask a question',feedback_mode:'required',allow_attachments:true,continues_review:true},
  {id:'fix',label:'Request changes',feedback_mode:'required',allow_attachments:true,continues_review:false},
  {id:'annotate',label:'Review doc comments',feedback_mode:'required',allow_attachments:true,continues_review:false},
  {id:'other',label:'Another recipe choice',feedback_mode:'optional',allow_attachments:false,continues_review:false},
 ];
 return request;
};
