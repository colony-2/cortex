import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { getFinalDocuments } from '@colony2/shared';
import ReturnedDocuments from './ReturnedDocuments';
vi.mock('@colony2/shared', async()=>({...await vi.importActual('@colony2/shared'),getFinalDocuments:vi.fn()}));
beforeEach(()=>vi.clearAllMocks());
it.each([['empty','No documents returned (explicit empty manifest).'],['absent','Final document declaration not recorded.'],['null','Final document declaration is null.'],['malformed','Final document declaration: malformed'],['unsupported','Final document declaration: unsupported']])('CV-016 preserves %s declaration state',async(state,text)=>{
 vi.mocked(getFinalDocuments).mockResolvedValue({job_id:'job',manifest_id:'id',state,documents:[]});
 render(<ReturnedDocuments tenant="tenant" job="job"/>);
 await screen.findByText(text);
});
it('CV-015 CV-016 distinguishes pending, failure with retry and unresolved members',async()=>{
 vi.mocked(getFinalDocuments).mockResolvedValueOnce({status:'pending'}).mockRejectedValueOnce(new Error('read denied')).mockResolvedValueOnce({job_id:'job',manifest_id:'id',state:'present',documents:[{alias:'missing.md',resolution:'unresolved',diagnostic:'mapping missing',ref:{},raw:{}},{alias:'remote.md',resolution:'external',ref:{},raw:{}},{alias:'value.json',resolution:'value',ref:{},raw:{}}]});
 const view=render(<ReturnedDocuments tenant="tenant" job="job" refreshKey={1}/>);
 await screen.findByText('Final documents pending.');
 view.rerender(<ReturnedDocuments tenant="tenant" job="job" refreshKey={2}/>);
 await screen.findByText('Returned documents unavailable');
 fireEvent.click(screen.getByRole('button',{name:'Retry'}));
 await screen.findByText('missing.md — unresolved: mapping missing');
 expect(screen.queryAllByRole('link')).toHaveLength(0);
});
it('CV-022 aborts obsolete manifest reads and ignores their late results',async()=>{
 let resolve!:(r:any)=>void;
 vi.mocked(getFinalDocuments).mockImplementationOnce(()=>new Promise(r=>{resolve=r})).mockResolvedValue({job_id:'other',manifest_id:'new',state:'empty',documents:[]});
 const view=render(<ReturnedDocuments tenant="tenant" job="job"/>);
 const signal=vi.mocked(getFinalDocuments).mock.calls[0][2];
 view.rerender(<ReturnedDocuments tenant="tenant" job="other"/>);
 expect(signal?.aborted).toBe(true);
 await screen.findByText('No documents returned (explicit empty manifest).');
 resolve({job_id:'job',manifest_id:'old',state:'present',documents:[{alias:'obsolete.md'}]});
 expect(screen.queryByText('obsolete.md')).toBeNull();
});
