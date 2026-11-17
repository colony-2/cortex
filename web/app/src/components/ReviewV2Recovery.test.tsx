import {act,fireEvent,render,screen,waitFor,within} from '@testing-library/react';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {MemoryRouter, Route, Routes, useLocation} from 'react-router-dom';
import {inputActivityService,type ReviewV2Output,type UserInputDetails} from '@colony2/shared';
import {amendedReviewV2Fixture,reviewV2Fixture} from '../../../shared/src/test/reviewV2Fixture';
import {useReviewV2} from '../hooks/useReviewV2';
import type {ReviewV2View} from '../utils/reviewV2State';
import ReviewV2 from './ReviewV2';

vi.mock('./ReviewV2Context',()=>({default:()=>null}));
vi.mock('../hooks/useReviewV2',()=>({useReviewV2:vi.fn()}));
let view:ReviewV2View;
beforeEach(()=>{
 global.ResizeObserver=class {observe(){} unobserve(){} disconnect(){}};
 vi.mocked(window.matchMedia).mockImplementation(query=>({matches:false,media:query,onchange:null,addListener:vi.fn(),removeListener:vi.fn(),addEventListener:vi.fn(),removeEventListener:vi.fn(),dispatchEvent:vi.fn()}));
 const request=amendedReviewV2Fixture();request.documents={};
 const current={jobId:'job',status:'pending',startTime:'now',form:{request_id:'native-one',presentation:{type:'review/v2'},request,request_schema:{type:'object'},response_schema:{type:'object'}}} as UserInputDetails;
 view={request,current,phase:'actionable',conversation:[]};
 vi.mocked(useReviewV2).mockImplementation(()=>({view,error:'',busy:false,refresh:vi.fn().mockResolvedValue(undefined),sending:vi.fn(),accepted:vi.fn(),failed:vi.fn()}));
 vi.spyOn(inputActivityService,'submitReviewV2').mockRejectedValue(new Error('Connection lost'));
});
afterEach(()=>{vi.restoreAllMocks();vi.clearAllMocks();});

const component=()=> <MemoryRouter><ReviewV2 tenant="tenant" job="job" reviewId="review-one"/></MemoryRouter>;
async function attemptedQuestion(){
 fireEvent.click(screen.getByRole('button',{name:'Review settings'}));
 const dialog=screen.getByRole('dialog');
 fireEvent.change(within(dialog).getByLabelText('Response style'),{target:{value:'0'}});
 fireEvent.change(within(dialog).getByLabelText('Example count'),{target:{value:'0'}});
 fireEvent.click(within(dialog).getByRole('button',{name:'Done'}));
 fireEvent.click(screen.getByRole('button',{name:'Ask a question',exact:true}));
 fireEvent.change(screen.getByLabelText('Question or requested change (required)'),{target:{value:'Was it accepted?'}});
 const file=new File(['Notes already sent'],'notes.md',{type:'text/markdown'});
 fireEvent.change(screen.getByLabelText('Attach notes'),{target:{files:[file]}});
 await act(async()=>{fireEvent.click(screen.getByRole('button',{name:'Send · Ask a question'}));});
 await screen.findByText(/Connection lost/);
 // Failed and ambiguous requests keep their drafts until acceptance is proven.
 expect(screen.getByLabelText('Question or requested change (required)')).toHaveValue('Was it accepted?');
 expect(screen.getByRole('button',{name:'Remove notes.md (1)'})).toBeInTheDocument();
 const call=vi.mocked(inputActivityService.submitReviewV2).mock.calls[0];
 return {response:{...call[3],attachments:[]},receipt:{request_id:'native-one',submission_id:call[5]}} as ReviewV2Output;
}
function successor(output:ReviewV2Output){
 const request={...view.request,conversation:[{role:'human' as const,direction:output.response.direction,text:output.response.feedback,attachments:[],settings:output.response.settings,receipt:output.receipt},{role:'agent' as const,text:'Your question was accepted.',attachments:[]}]};
 const current={jobId:'job',status:'pending',startTime:'now',form:{request_id:'native-two',presentation:{type:'review/v2'},request,request_schema:{type:'object'},response_schema:{type:'object'}}} as UserInputDetails;
 view={request,current,phase:'actionable',conversation:request.conversation};
}

it.each(['history','successor'])('clears confirmed contributions recovered through %s and allows clean approval',async source=>{
 const {rerender}=render(component());
 const output=await attemptedQuestion();
 if(source==='history'){
  view={...view,current:undefined,phase:'waiting',output};rerender(component());
  await waitFor(()=>expect(screen.queryByText(/Connection lost/)).not.toBeInTheDocument());
 }
 successor(output);rerender(component());
 await waitFor(()=>expect(screen.getByLabelText('Question or requested change (optional)')).toHaveValue(''));
 expect(screen.queryByRole('radio')).not.toBeInTheDocument();
 expect(screen.queryByRole('button',{name:'Remove notes.md (1)'})).not.toBeInTheDocument();
 expect(screen.queryByText(/Connection lost/)).not.toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'Review settings'}));
 const dialog=screen.getByRole('dialog');
 expect(within(dialog).getByLabelText('Response style')).toHaveValue('1'); // Submitted override was consumed.
 expect(within(dialog).getByLabelText('Example count')).toHaveValue(0); // Inapplicable draft survives.
 fireEvent.click(within(dialog).getByRole('button',{name:'Done'}));
 fireEvent.click(screen.getByRole('button',{name:'Approve direction',exact:true}));

 await waitFor(()=>expect(inputActivityService.submitReviewV2).toHaveBeenCalledTimes(2));
 expect(vi.mocked(inputActivityService.submitReviewV2).mock.calls[1].slice(2,5)).toEqual(['native-two',{direction:'go',feedback:'',attachments:[],settings:{}},[]]);
});

it.each(['request_id','submission_id'])('does not consume a contribution for a different %s',async field=>{
 const {rerender}=render(component());const output=await attemptedQuestion();
 successor({...output,receipt:{...output.receipt,[field]:'different'}});rerender(component());
 expect(screen.getByLabelText('Question or requested change (required)')).toHaveValue('Was it accepted?');
 expect(screen.getByRole('button',{name:'Remove notes.md (1)'})).toBeInTheDocument();
 expect(screen.getByText(/Connection lost/)).toBeInTheDocument();
});

it('keeps newer unsent edits when an earlier contribution is confirmed',async()=>{
 const {rerender}=render(component());const output=await attemptedQuestion();
 fireEvent.change(screen.getByLabelText('Question or requested change (required)'),{target:{value:'A new question'}});
 const newerFile=new File(['New notes'],'new.md',{type:'text/markdown'});
 fireEvent.change(screen.getByLabelText('Attach notes'),{target:{files:[newerFile]}});
 fireEvent.click(screen.getByRole('button',{name:'Review settings'}));
 const dialog=screen.getByRole('dialog');
 fireEvent.change(within(dialog).getByLabelText('Response style'),{target:{value:'1'}});
 fireEvent.click(within(dialog).getByRole('button',{name:'Done'}));
 successor(output);rerender(component());
 await waitFor(()=>expect(screen.getByLabelText('Question or requested change (required)')).toHaveValue('A new question'));
 expect(screen.getByRole('button',{name:'Remove new.md (1)'})).toBeInTheDocument();
 expect(screen.queryByText(/Connection lost/)).not.toBeInTheDocument();
});

function QueueLocation(){const location=useLocation();return <p>{location.pathname+location.search}</p>;}
it.each(['actionable','other','unavailable','sent','waiting','loading'] as const)('preserves only Reviews controls when returning from v2 %s',async phase=>{
 if(phase==='loading')vi.mocked(useReviewV2).mockReturnValue({view:undefined,error:'',busy:true,refresh:vi.fn(),sending:vi.fn(),accepted:vi.fn(),failed:vi.fn()});
 else view={...view,phase};
 render(<MemoryRouter initialEntries={['/project/tenant/reviews/job?q=foo+bar&cellId=platform&sort=title&order=descend&request_id=native&document_id=plan&unrelated=drop']}>
  <Routes><Route path="/project/tenant/reviews/job" element={<ReviewV2 tenant="tenant" job="job" reviewId="review-one"/>}/><Route path="/project/tenant/reviews" element={<QueueLocation/>}/></Routes>
 </MemoryRouter>);
 fireEvent.click(screen.getByRole('button',{name:phase==='other'?'Open pending reviews':/Back to Reviews/}));
 expect(await screen.findByText('/project/tenant/reviews?q=foo+bar&cellId=platform&sort=title&order=descend')).toBeVisible();
});

it('submits long feedback intact and preserves it after an uncertain failure',async()=>{
 render(component());
 fireEvent.click(screen.getByRole('button',{name:'Ask a question',exact:true}));
 const editor=screen.getByLabelText('Question or requested change (required)');
 const feedback='Please clarify the remaining evidence and the requested change. 日本語\n'.repeat(100);
 expect(editor).not.toHaveAttribute('maxlength');
 fireEvent.change(editor,{target:{value:feedback}});
 fireEvent.click(screen.getByRole('button',{name:'Send · Ask a question'}));
 await screen.findByText(/Connection lost/);
 expect(editor).toHaveValue(feedback);
 expect(vi.mocked(inputActivityService.submitReviewV2).mock.calls[0][3].feedback).toBe(feedback);
});

it('A01/A02/R01 action labels keep declaration order, focus sends nothing, and rapid clean activation posts the clicked action once',async()=>{
 vi.mocked(inputActivityService.submitReviewV2).mockImplementation(()=>new Promise(()=>{}));
 render(component());
 expect(within(screen.getByLabelText('Review actions')).getAllByRole('button').map(b=>b.textContent)).toEqual(view.request.directions.map(d=>d.label));
 expect(screen.queryByRole('radio')).toBeNull();expect(screen.queryByRole('button',{name:'Continue'})).toBeNull();
 const approve=screen.getByRole('button',{name:'Approve direction',exact:true});approve.focus();expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();
 fireEvent.click(approve);fireEvent.click(approve);
 expect(inputActivityService.submitReviewV2).toHaveBeenCalledTimes(1);
 expect(vi.mocked(inputActivityService.submitReviewV2).mock.calls[0].slice(2,5)).toEqual(['native-one',{direction:'go',feedback:'',attachments:[],settings:{}},[]]);
});
it('A03/A05/A10 required composer focuses feedback, keeps multiline drafts on Back, and sends only explicitly',async()=>{
 render(component());const ask=screen.getByRole('button',{name:'Ask a question',exact:true});fireEvent.click(ask);
 const input=screen.getByLabelText('Question or requested change (required)');expect(input).toHaveFocus();
 fireEvent.change(input,{target:{value:' \n\ufeff'}});fireEvent.click(screen.getByRole('button',{name:'Send · Ask a question'}));
 expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();expect(screen.getByRole('alert')).toHaveTextContent('requires feedback');
 fireEvent.change(input,{target:{value:'Exact\nquestion '}});fireEvent.keyDown(input,{key:'Enter'});expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'Back',exact:true}));expect(ask).toHaveFocus();
 expect(screen.getByText(/Unsent feedback draft retained/)).toBeVisible();fireEvent.click(ask);expect(input).toHaveValue('Exact\nquestion ');
 fireEvent.click(screen.getByRole('button',{name:'Send · Ask a question'}));await screen.findByText(/Connection lost/);
 expect(vi.mocked(inputActivityService.submitReviewV2).mock.calls[0][3].feedback).toBe('Exact\nquestion ');
});
it('A04/A07 optional feedback permits explicit empty send but retains and blocks incompatible uploads',async()=>{
 render(component());fireEvent.click(screen.getByRole('button',{name:'Ask a question',exact:true}));
 const file=new File([''],'empty.md');fireEvent.change(screen.getByLabelText('Attach notes'),{target:{files:[file]}});
 fireEvent.click(screen.getByRole('button',{name:'Back',exact:true}));
 fireEvent.click(screen.getByRole('button',{name:'Another recipe choice',exact:true}));
 expect(screen.getByRole('button',{name:'Remove empty.md (1)'})).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'Send · Another recipe choice'}));expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'Remove empty.md (1)'}));expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'Send · Another recipe choice'}));await screen.findByText(/Connection lost/);
 expect(vi.mocked(inputActivityService.submitReviewV2).mock.calls[0][3]).toEqual({direction:'other',feedback:'',attachments:[],settings:{}});
});
it('A08/A09 clean approval exposes whitespace and empty files without dropping or submitting them',async()=>{
 render(component());fireEvent.click(screen.getByRole('button',{name:'Ask a question',exact:true}));
 fireEvent.change(screen.getByLabelText('Question or requested change (required)'),{target:{value:' '}});
 fireEvent.change(screen.getByLabelText('Attach notes'),{target:{files:[new File([''],'empty.md')]}});
 fireEvent.click(screen.getByRole('button',{name:'Back',exact:true}));
 fireEvent.click(screen.getByRole('button',{name:'Approve direction',exact:true}));
 expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();expect(screen.getByLabelText('Question or requested change (not allowed)')).toHaveValue(' ');
 expect(screen.getByRole('button',{name:'Remove empty.md (1)'})).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'Remove feedback draft'}));fireEvent.click(screen.getByRole('button',{name:'Remove empty.md (1)'}));
 expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();fireEvent.click(screen.getByRole('button',{name:'Approve direction',exact:true}));
 await screen.findByText(/Connection lost/);expect(inputActivityService.submitReviewV2).toHaveBeenCalledTimes(1);
});
it('A06 permitted new upload satisfies required feedback and upload failure preserves the File object',async()=>{
 render(component());fireEvent.click(screen.getByRole('button',{name:'Request changes',exact:true}));
 const file=new File(['annotated'],'notes.md');fireEvent.change(screen.getByLabelText('Attach notes'),{target:{files:[file]}});
 fireEvent.click(screen.getByRole('button',{name:'Send · Request changes'}));await screen.findByText(/Connection lost/);
 expect(vi.mocked(inputActivityService.submitReviewV2).mock.calls[0][4][0]).toBe(file);
 expect(screen.getByRole('button',{name:'Remove notes.md (1)'})).toBeVisible();
});
it('A11/A12 invalid applicable settings block sending while inapplicable settings stay local; settings close restores focus',async()=>{
 render(component());const gear=screen.getByRole('button',{name:'Review settings'});fireEvent.click(gear);
 const dialog=screen.getByRole('dialog');fireEvent.change(within(dialog).getByLabelText('Example count'),{target:{value:'-1'}});fireEvent.click(within(dialog).getByRole('button',{name:'Done'}));
 expect(gear).toHaveFocus();expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'Request changes',exact:true}));fireEvent.change(screen.getByLabelText('Question or requested change (required)'),{target:{value:'Changes'}});
 fireEvent.click(screen.getByRole('button',{name:'Send · Request changes'}));expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'Remove feedback draft'}));
 fireEvent.click(screen.getByRole('button',{name:'Back',exact:true}));
 fireEvent.click(screen.getByRole('button',{name:'Approve direction',exact:true}));
 await screen.findByText(/Connection lost/);expect(vi.mocked(inputActivityService.submitReviewV2).mock.calls[0][3].settings).toEqual({});
 expect(screen.getByText(/Retained locally; applies to Request changes/)).toBeVisible();
});
it('P01/P03/P05 accepted continuing response waits in discussion, then actual answer restores actions without focus movement',()=>{
 const output:ReviewV2Output={response:{direction:'ask',feedback:'Question',attachments:[],settings:{}},receipt:{request_id:'native-one',submission_id:'s'}};
 const human={role:'human' as const,direction:'ask',text:'Question',attachments:[],receipt:output.receipt};
 view={...view,phase:'waiting',output,conversation:[human],job:{status:'ACTIVE',cell_name:'cortex'} as any};
 const {rerender}=render(component());expect(within(screen.getByRole('region',{name:'Discussion'})).getByText('Response pending')).toBeVisible();
 const refresh=screen.getByRole('button',{name:'Refresh'});refresh.focus();
 successor(output);rerender(component());expect(screen.queryByText('Response pending')).not.toBeInTheDocument();expect(screen.getByText('Your question was accepted.')).toBeVisible();
 expect(screen.getByRole('button',{name:'Approve direction',exact:true})).toBeVisible();expect(screen.getByRole('button',{name:'Approve direction',exact:true})).not.toHaveFocus();
});

it('A13 frozen legacy optional approval opens a composer and sends nothing until explicit Send',async()=>{
 const request=reviewV2Fixture();request.documents={};view={...view,request,current:{...view.current!,form:{...view.current!.form,request}}};
 render(component());fireEvent.click(screen.getByRole('button',{name:'Approve direction',exact:true}));
 expect(screen.getByLabelText('Question or requested change (optional)')).toBeVisible();expect(inputActivityService.submitReviewV2).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'Send · Approve direction'}));await screen.findByText(/Connection lost/);
 expect(vi.mocked(inputActivityService.submitReviewV2).mock.calls[0][3].direction).toBe('go');
});
