import {it,expect} from 'vitest';
import {historicalDocuments,historicalTurnAttachments} from './historicalDocuments';
import {reviewV2Fixture} from '../../../shared/src/test/reviewV2Fixture';
import type {HistoryDetail} from '@colony2/shared';
it('resolves nested escaped document associations and explicit notes only',()=>{
 const request=reviewV2Fixture();const ref=request.documents.design;request.documents={'a/b~c':ref};
 const detail={raw_form:{presentation:{type:'review/v2'},request},request_manifest:[{artifact_id:'original',original_ref:ref,ref,associations:[{source:'/request/documents/a~1b~0c'}]},{artifact_id:'unrelated',ref:{kind:'attached',name:'other'},associations:[]}],response_manifest:[],accepted_response:{present:false}} as unknown as HistoryDetail;
 const docs=historicalDocuments(detail);expect(docs.members).toEqual({'a/b~c':'original'});expect(Object.keys(docs.documents)).toEqual(['a/b~c']);
 const note={...ref,stored:{key:{...ref.stored.key,name:'note.md'}}};request.conversation=[{role:'human',direction:'question',text:'',receipt:{},attachments:[note]}];detail.request_manifest.push({artifact_id:'note',ref:note,associations:[],availability:'available'});
 expect(historicalTurnAttachments(detail)).toEqual({'0/0':{member:'note'}});
 detail.request_manifest.push({artifact_id:'ambiguous',ref,associations:[],availability:'available'});expect(historicalDocuments(detail).members).toEqual({});
});

it('does not let an old manifest association substitute bytes after an original changes',()=>{
 const request=reviewV2Fixture(),old=request.documents.design;
 const current={...old,stored:{key:{...old.stored.key,name:'changed.md'}}};request.documents.design=current;
 const detail={raw_form:{presentation:{type:'review/v2'},request},request_manifest:[{artifact_id:'old',original_ref:old,ref:old,associations:[{source:'/request/documents/design'}]}],response_manifest:[],accepted_response:{present:false}} as unknown as HistoryDetail;
 expect(historicalDocuments(detail).members).toEqual({});expect(historicalDocuments(detail).documents.design.stored).toEqual(current.stored);
});
