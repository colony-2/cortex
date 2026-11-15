import {fireEvent,render,screen} from '@testing-library/react';
import {afterEach,beforeEach,it,expect,vi} from 'vitest';
import {MemoryRouter,Route,Routes} from 'react-router-dom';
import {inputActivityService,type ReviewV2Turn,type UserInputDetails} from '@colony2/shared';
import {ReviewV2Discussion} from './ReviewV2';
import InputDetailPage from './InputDetailPage';
import {reviewV2Fixture} from '../../../shared/src/test/reviewV2Fixture';
beforeEach(()=>{vi.mocked(window.matchMedia).mockImplementation(query=>({matches:false,media:query,onchange:null,addListener:vi.fn(),removeListener:vi.fn(),addEventListener:vi.fn(),removeEventListener:vi.fn(),dispatchEvent:vi.fn()}));});
afterEach(()=>vi.restoreAllMocks());
it('shows frozen labels and exact historical values without applying successor declarations',()=>{
 const old=reviewV2Fixture(),next=reviewV2Fixture();next.settings![0].label='New label';
 const turn:ReviewV2Turn={role:'human',direction:'ask',text:' exact text ',attachments:[],receipt:{request_id:'old'},settings:{response_style:null,removed:false}};
 render(<ReviewV2Discussion turns={[turn]} request={next} declarations={{old}}/>);
 expect(screen.getByText('Response style:')).toBeInTheDocument();expect(screen.queryByText('New label:')).not.toBeInTheDocument();expect(screen.getByText('null')).toBeInTheDocument();expect(screen.getByText('removed:')).toBeInTheDocument();expect(screen.getByText('false')).toBeInTheDocument();
});
it('keeps long threads available through explicit incremental rendering',()=>{
 const turns:ReviewV2Turn[]=Array.from({length:1000},(_,i)=>({role:'agent',text:`Answer ${i}`,attachments:[]}));
 render(<ReviewV2Discussion turns={turns} request={reviewV2Fixture()}/>);
 expect(screen.getAllByRole('article')).toHaveLength(20);expect(screen.getByRole('button',{name:'Show earlier turns (980)'})).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'Show earlier turns (980)'}));expect(screen.getAllByRole('article')).toHaveLength(40);expect(screen.getByText('Answer 960')).toBeInTheDocument();
});
for(const kind of ['unsupported','malformed'] as const)it(`fails closed for ${kind} presentation before ordinary adaptation`,async()=>{
 const req=reviewV2Fixture();if(kind==='malformed')delete (req.directions[0] as Partial<typeof req.directions[0]>).feedback_required;
 const details:UserInputDetails={jobId:'job',status:'pending',startTime:'now',form:{request_id:'native',presentation:{type:kind==='unsupported'?'review/v3':'review/v2'},request:req,request_schema:{type:'object'},response_schema:{type:'object'}}};
 vi.spyOn(inputActivityService,'getInputDetails').mockResolvedValue(details);
 render(<MemoryRouter initialEntries={['/reviews/job']}><Routes><Route path="/reviews/:jobId" element={<InputDetailPage projectId="tenant"/>}/></Routes></MemoryRouter>);
 expect(await screen.findByText(kind==='unsupported'?'Unsupported input presentation':'Incompatible review request')).toBeInTheDocument();expect(screen.queryByRole('button',{name:'Continue'})).not.toBeInTheDocument();
});
