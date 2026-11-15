import {act,fireEvent,render,screen,waitFor,within} from '@testing-library/react';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {MemoryRouter} from 'react-router-dom';
import {inputActivityService,type ReviewV2Output,type UserInputDetails} from '@colony2/shared';
import {amendedReviewV2Fixture} from '../../../shared/src/test/reviewV2Fixture';
import {useReviewV2} from '../hooks/useReviewV2';
import type {ReviewV2View} from '../utils/reviewV2State';
import ReviewV2 from './ReviewV2';

vi.mock('../hooks/useReviewV2',()=>({useReviewV2:vi.fn()}));
let view:ReviewV2View;
beforeEach(()=>{
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
 fireEvent.click(screen.getByRole('radio',{name:/Ask a question/}));
 fireEvent.change(screen.getByLabelText('Question or requested change (required)'),{target:{value:'Was it accepted?'}});
 const file=new File(['Notes already sent'],'notes.md',{type:'text/markdown'});
 fireEvent.change(screen.getByLabelText('Attach notes'),{target:{files:[file]}});
 fireEvent.click(screen.getByRole('button',{name:'Continue',exact:true}));
 await act(async()=>{fireEvent.click(screen.getByRole('button',{name:'Confirm Ask a question'}));});
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
 expect(screen.getAllByRole('radio').every(radio=>!(radio as HTMLInputElement).checked)).toBe(true);
 expect(screen.queryByRole('button',{name:'Remove notes.md (1)'})).not.toBeInTheDocument();
 expect(screen.queryByText(/Connection lost/)).not.toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'Review settings'}));
 const dialog=screen.getByRole('dialog');
 expect(within(dialog).getByLabelText('Response style')).toHaveValue('1'); // Submitted override was consumed.
 expect(within(dialog).getByLabelText('Example count')).toHaveValue(0); // Inapplicable draft survives.
 fireEvent.click(within(dialog).getByRole('button',{name:'Done'}));
 fireEvent.click(screen.getByRole('radio',{name:/Approve direction/}));
 fireEvent.click(screen.getByRole('button',{name:'Continue',exact:true}));
 fireEvent.click(screen.getByRole('button',{name:'Confirm Approve direction'}));
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
 await waitFor(()=>expect(screen.getByLabelText('Question or requested change (optional)')).toHaveValue('A new question'));
 expect(screen.getByRole('button',{name:'Remove new.md (1)'})).toBeInTheDocument();
 expect(screen.queryByText(/Connection lost/)).not.toBeInTheDocument();
});
