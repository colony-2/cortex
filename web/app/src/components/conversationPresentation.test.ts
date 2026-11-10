import { describe, expect, it } from 'vitest';
import { recordedTime, savedAnswers, reviewer } from './conversationPresentation';
import { historicalDocuments } from './historicalDocuments';
import type { HistoryDetail, HistoryEntry } from '@colony2/shared';
describe('CC-004 frozen answers', () => {
 const fields = {fields:[{id:'verdict',question:'Decision',options:[{value:'revise',label:'Changes Requested'}]},{id:'notes',question:'Comments'}]};
 it('decodes real output fields and direct keyed answers using saved labels', () => {
  expect(savedAnswers(fields,{fields:{verdict:'revise',notes:'Original feedback'}})).toBe('Decision: Changes Requested\nComments: Original feedback');
  expect(savedAnswers(fields,{verdict:'new',notes:{structured:[1,false]},extra:'kept'})).toContain('extra: kept');
  expect(savedAnswers(fields,{verdict:'new',notes:{structured:[1,false]}})).toContain('"structured"');
  expect(savedAnswers({options:[{value:'yes',label:'Proceed'}]},{response:'yes'})).toBe('Proceed');
  expect(savedAnswers({options:[{value:'a',label:'First'}]},['a','unknown'])).toBe('First, unknown');
  expect(savedAnswers({},'answered')).toBe('answered');
  expect(savedAnswers(fields,{fields:{verdict:'revise',notes:'feedback'}},true)).toBe('Changes Requested · feedback');
 });
});
describe('CC-005 CC-006 timestamps', () => {
 it.each([undefined,null,0,'', 'not time','2025-02-30T14:30:00Z','0001-01-01T00:00:00Z'])('rejects missing or invalid %s', raw => expect(recordedTime({present:true,raw}).text).toBe('Time not recorded'));
 it('uses local time with accessible timezone, not observation time', () => {
  const time=recordedTime({present:true,raw:'2025-01-02T14:30:00Z'});
  expect(time.text).toContain('2025'); expect(time.title).toBeTruthy();
  expect(recordedTime({present:false,raw:'2025-01-02T14:30:00Z'}).text).toBe('Time not recorded');
 });
});
describe('CC-007 provenance', () => {
 it.each([['submission_receipt','Submission receipt'],['reported_receipt','Caller-reported'],['automation_receipt','Automated response'],['reported_legacy','Legacy reported'],['unavailable','Reviewer unavailable']])('%s is explicit', (provenance,label) => {
  expect(reviewer({actor:{provenance,receipt:{actor:{id:'j@j.com',kind:'human'}},legacy_user_id:{present:true,raw:'legacy'}}} as HistoryEntry)).toContain(label);
 });
});
describe('CC-011 CC-012 CC-015 original catalog', () => {
 const stored={kind:'stored',name:'original.md',stored:{key:{jobId:'producer',taskOrdinal:8,name:'docs/original.md',sizeBytes:20}}};
 const detail = (documents: unknown, manifest: unknown[]) => ({raw_form:{documents,document_metadata:{'A/~/雪':{order:0,role:'primary',source_document_ids:['same']}},overview_document_id:'A/~/雪'},request_manifest:manifest} as HistoryDetail);
 it('preserves IDs, zero order, overview and shared members independent of manifest order', () => {
  const catalog=historicalDocuments(detail({'A/~/雪':stored,same:stored},[{artifact_id:'original',ref:stored,associations:[]}]));
  expect(catalog.members).toEqual({'A/~/雪':'original',same:'original'});
  expect(historicalDocuments(detail({Alias:{...stored,name:'display alias.md'}},[{artifact_id:'original',ref:stored,associations:[]}])).members.Alias).toBe('original');
  expect(catalog.metadata['A/~/雪'].order).toBe(0);expect(catalog.overview).toBe('A/~/雪');
 });
 it('uses escaped associations and refuses ambiguity or unsupported placeholder synthesis', () => {
  const doc={kind:'attached',name:'source'};
  const manifest=[{artifact_id:'one',ref:stored,original_ref:doc,associations:[{kind:'document',source:'/documents/A~1~0~1雪'}]}];
  expect(historicalDocuments(detail({'A/~/雪':doc},manifest)).members['A/~/雪']).toBe('one');
  expect(historicalDocuments(detail({'A/~/雪':doc},manifest)).documents['A/~/雪'].name).toBe('original.md');
  expect(historicalDocuments(detail({'A/~/雪':doc},[...manifest,{...manifest[0],artifact_id:'two'}])).members).toEqual({});
  const missing=historicalDocuments(detail({external:{kind:'external',url:'https://example.test'},missing:stored},[]));
  expect(missing.members).toEqual({});expect(missing.documents.external.stored).toBeUndefined();
  expect(historicalDocuments({request_manifest:[]} as unknown as HistoryDetail).limited).toBe(true);
 });
});
