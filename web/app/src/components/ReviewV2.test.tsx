import {fireEvent,render as rawRender,screen,within} from '@testing-library/react';
import {afterEach,beforeEach,it,expect,vi} from 'vitest';
import {MemoryRouter,Route,Routes} from 'react-router-dom';
import {inputActivityService,InputActivityProvider,type ReviewV2Turn,type UserInputDetails} from '@colony2/shared';
const render = (ui: Parameters<typeof rawRender>[0]) => rawRender(ui, {wrapper: InputActivityProvider});
import {ReviewV2Discussion} from './ReviewV2';
import InputDetailPage from './InputDetailPage';
import {reviewV2Fixture} from '../../../shared/src/test/reviewV2Fixture';
beforeEach(()=>{vi.mocked(window.matchMedia).mockImplementation(query=>({matches:false,media:query,onchange:null,addListener:vi.fn(),removeListener:vi.fn(),addEventListener:vi.fn(),removeEventListener:vi.fn(),dispatchEvent:vi.fn()}));});
afterEach(()=>{vi.restoreAllMocks();vi.unstubAllGlobals();inputActivityService.clearCaches();});
it('shows frozen labels and exact historical values without applying successor declarations',()=>{
 const old=reviewV2Fixture(),next=reviewV2Fixture();next.settings![0].label='New label';
 const turn:ReviewV2Turn={role:'human',direction:'ask',text:' exact text ',attachments:[],receipt:{request_id:'old'},settings:{response_style:null,removed:false}};
 render(<ReviewV2Discussion turns={[turn]} request={next} declarations={{old}}/>);
 expect(screen.getByText('Response style:')).toBeInTheDocument();expect(screen.queryByText('New label:')).not.toBeInTheDocument();expect(screen.getByText('null')).toBeInTheDocument();expect(screen.getByText('removed:')).toBeInTheDocument();expect(screen.getByText('false')).toBeInTheDocument();
});
it('keeps long threads available through explicit incremental rendering',()=>{
 const turns:ReviewV2Turn[]=Array.from({length:1000},(_,i)=>({role:'agent',text:`Answer ${i}`,attachments:[]}));
 render(<ReviewV2Discussion turns={turns} request={reviewV2Fixture()}/>);
 expect(screen.getAllByRole('article')).toHaveLength(21);expect(screen.getByRole('button',{name:'Show earlier turns (980)'})).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'Show earlier turns (980)'}));expect(screen.getAllByRole('article')).toHaveLength(41);expect(screen.getByText('Answer 960')).toBeInTheDocument();
});
for(const kind of ['unsupported','malformed'] as const)it(`fails closed for ${kind} presentation before ordinary adaptation`,async()=>{
 const req=reviewV2Fixture();if(kind==='malformed')delete (req.directions[0] as Partial<typeof req.directions[0]>).feedback_required;
 const details:UserInputDetails={jobId:'job',status:'pending',startTime:'now',form:{request_id:'native',presentation:{type:kind==='unsupported'?'review/v3':'review/v2'},request:req,request_schema:{type:'object'},response_schema:{type:'object'}}};
 vi.spyOn(inputActivityService,'getInputDetails').mockResolvedValue(details);
 vi.stubGlobal('fetch',vi.fn(async url=>new Response(JSON.stringify(String(url).endsWith('/pending')?[{id:'job',cell_id:'cell',cell_name:'platform',repo:'github.com/acme/platform',client_payload_revision:1}]:{job_id:'job',cell_id:'cell',cell_name:'platform',repo:'github.com/acme/platform'}))));
 render(<MemoryRouter initialEntries={['/reviews/job']}><Routes><Route path="/reviews/:jobId" element={<InputDetailPage projectId="tenant"/>}/></Routes></MemoryRouter>);
 expect(await screen.findByText(kind==='unsupported'?'Unsupported input presentation':'Incompatible review request')).toBeInTheDocument();expect(screen.queryByRole('button',{name:'Continue'})).not.toBeInTheDocument();
});

it('Q01/02/07 renders formatted prose as the final ordinary Agent message with safe public links',()=>{
 const request=reviewV2Fixture();
 request.question='# Proceed?\n\nFirst line\nSecond **bold** and *emphasis*.\n\n- One\n- Two\n\n[web](https://example.com) [http](http://example.com) [email](mailto:team@example.com)\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n```ts\nconst x = 1;\nconst y = 2;\n```';
 const {container}=render(<ReviewV2Discussion request={request} turns={[{role:'agent',text:'Earlier',attachments:[]}]}/>);
 const articles=screen.getAllByRole('article');expect(articles).toHaveLength(2);
 const question=articles.at(-1)!;expect(question.className).toBe(articles[0].className);expect(question).toHaveClass('review-v2-turn','agent');
 expect(within(question).getByText('Agent',{exact:true})).toBeInTheDocument();expect(question.querySelector('time')).toBeNull();
 expect(within(question).getByRole('heading',{name:'Proceed?'})).toBeInTheDocument();
 expect(within(question).getByText('bold').tagName).toBe('STRONG');
 expect(question.querySelector('em')).toHaveTextContent('emphasis');expect(within(question).getAllByRole('listitem')).toHaveLength(2);
 expect(within(question).getByRole('table')).toBeInTheDocument();expect(question.querySelector('pre code')).toHaveTextContent('const x = 1;');
 expect(question.querySelector('pre code')?.textContent).toBe('const x = 1;\nconst y = 2;\n');
 expect(question.querySelector('p br')).toBeInTheDocument();
 for(const [name,href] of [['web','https://example.com'],['http','http://example.com'],['email','mailto:team@example.com']]){
  const link=within(question).getByRole('link',{name});expect(link).toHaveAttribute('href',href);expect(link).toHaveAttribute('target','_blank');expect(link).toHaveAttribute('rel','noopener noreferrer');
 }
 expect(container.querySelector('.review-v2-question')).toBeNull();
});
it.each(['',' \n  '])('Q03 omits blank questions and shows the hint only without recorded turns: %j',question=>{
 const request={...reviewV2Fixture(),question};const {rerender}=render(<ReviewV2Discussion request={request} turns={[]}/>);
 expect(screen.queryAllByRole('article')).toHaveLength(0);expect(screen.getByText(/No questions yet/)).toBeInTheDocument();
 rerender(<ReviewV2Discussion request={request} turns={[{role:'human',text:'History',attachments:[]}]}/>);
 expect(screen.queryByText(/No questions yet/)).toBeNull();expect(screen.getAllByRole('article')).toHaveLength(1);
});
it('Q04/05 keeps matching history literal, unchanged and separately counted after earlier-turn expansion',()=>{
 const request={...reviewV2Fixture(),question:'**Same text**'};
 const turns:ReviewV2Turn[]=Array.from({length:1000},(_,i)=>({role:'agent',text:i===999?request.question:`Turn ${i}`,attachments:[]}));
 const before=JSON.stringify(turns);render(<ReviewV2Discussion request={request} turns={turns}/>);
 expect(screen.getAllByRole('article').at(-1)?.querySelector('strong:last-child')).toHaveTextContent('Same text');
 expect(screen.getByText('**Same text**')).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'Show earlier turns (980)'}));
 expect(screen.getAllByRole('article')).toHaveLength(41);expect(screen.getAllByRole('article').at(-1)).toHaveTextContent('AgentSame text');
 expect(JSON.stringify(turns)).toBe(before);
});
it('Q06 skips raw HTML and renders readable fallbacks for unsafe links, relative links and all images',()=>{
 const request={...reviewV2Fixture(),question:'<script>alert(1)</script>\n\n<div>hidden html</div>\n\n[unsafe](javascript:alert) [relative](review/a.md) [data](data:text/plain,hello)\n\n![diagram](https://example.com/diagram.png)'};
 const {container}=render(<ReviewV2Discussion request={request} turns={[]}/>);
 expect(container.querySelector('script')).toBeNull();expect(screen.queryByText('hidden html')).toBeNull();expect(screen.queryAllByRole('link')).toHaveLength(0);
 for(const text of ['unsafe','relative','data'])expect(screen.getByText(`${text} (link unavailable)`)).toBeInTheDocument();
 expect(screen.getByText('Image unavailable: diagram')).toBeInTheDocument();expect(container.querySelector('img')).toBeNull();
});
it('Q08 keeps pending status before the question and replaces only the selected question on refresh',()=>{
 const request={...reviewV2Fixture(),question:'**Current question**'};
 const attachmentURL=vi.fn(()=>'/retained/note');const turns:ReviewV2Turn[]=[{role:'human',text:'Ask',attachments:[{name:'note.md',stored:{key:{jobId:'p',taskOrdinal:1,name:'note.md'}}}],receipt:{request_id:'old'}}];
 const before=JSON.stringify(turns);const {rerender}=render(<ReviewV2Discussion request={request} turns={turns} pending attachmentURL={attachmentURL}/>);
 const question=screen.getAllByRole('article').at(-1)!;
 expect(screen.getByRole('status').compareDocumentPosition(question)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
 expect(attachmentURL).toHaveBeenCalledWith(0,0);expect(screen.getByRole('link',{name:'Download note.md'})).toHaveAttribute('href','/retained/note');
 rerender(<ReviewV2Discussion request={{...request,question:'Next question'}} turns={turns} attachmentURL={attachmentURL}/>);
 expect(screen.queryByText('Current question')).toBeNull();expect(screen.getAllByRole('article').at(-1)).toHaveTextContent('Next question');expect(JSON.stringify(turns)).toBe(before);
});
it('retains a focused question link through layout-driven parent renders',()=>{
 const request={...reviewV2Fixture(),question:'[Reference](https://example.com)'};
 const {rerender}=render(<ReviewV2Discussion request={request} turns={[]}/>);
 const link=screen.getByRole('link',{name:'Reference'});link.focus();
 rerender(<ReviewV2Discussion request={{...request}} turns={[]} pending/>);
 expect(screen.getByRole('link',{name:'Reference'})).toBe(link);expect(link).toHaveFocus();
});
