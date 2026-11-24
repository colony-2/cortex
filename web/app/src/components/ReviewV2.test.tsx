import {render as rawRender,screen,within} from '@testing-library/react';
import {afterEach,beforeEach,it,expect,vi} from 'vitest';
import {MemoryRouter,Route,Routes} from 'react-router-dom';
import {inputActivityService,InputActivityProvider,type UserInputDetails} from '@colony2/shared';
const render = (ui: Parameters<typeof rawRender>[0]) => rawRender(ui, {wrapper: InputActivityProvider});
import ReviewMessageBody from './ReviewMessageBody';
import InputDetailPage from './InputDetailPage';
import {reviewV2Fixture} from '../../../shared/src/test/reviewV2Fixture';
beforeEach(()=>{vi.mocked(window.matchMedia).mockImplementation(query=>({matches:false,media:query,onchange:null,addListener:vi.fn(),removeListener:vi.fn(),addEventListener:vi.fn(),removeEventListener:vi.fn(),dispatchEvent:vi.fn()}));});
afterEach(()=>{vi.restoreAllMocks();vi.unstubAllGlobals();inputActivityService.clearCaches();});
for(const kind of ['unsupported','malformed'] as const)it(`fails closed for ${kind} presentation before ordinary adaptation`,async()=>{
 const req=reviewV2Fixture();if(kind==='malformed')delete (req.directions[0] as Partial<typeof req.directions[0]>).feedback_required;
 const details:UserInputDetails={jobId:'job',status:'pending',startTime:'now',form:{request_id:'native',presentation:{type:kind==='unsupported'?'review/v3':'review/v2'},request:req,request_schema:{type:'object'},response_schema:{type:'object'}}};
 vi.spyOn(inputActivityService,'getInputDetails').mockResolvedValue(details);
 vi.stubGlobal('fetch',vi.fn(async url=>new Response(JSON.stringify(String(url).endsWith('/pending')?[{id:'job',cell_id:'cell',cell_name:'platform',repo:'github.com/acme/platform',client_payload_revision:1}]:{job_id:'job',cell_id:'cell',cell_name:'platform',repo:'github.com/acme/platform'}))));
 render(<MemoryRouter initialEntries={['/reviews/job']}><Routes><Route path="/reviews/:jobId" element={<InputDetailPage projectId="tenant"/>}/></Routes></MemoryRouter>);
 expect(await screen.findByText(kind==='unsupported'?'Unsupported input presentation':'Incompatible review request')).toBeInTheDocument();expect(screen.queryByRole('button',{name:'Continue'})).not.toBeInTheDocument();
});

it('renders formatted prompts inline with safe public links',()=>{
 const request=reviewV2Fixture();
 request.question='# Proceed?\n\nFirst line\nSecond **bold** and *emphasis*.\n\n- One\n- Two\n\n[web](https://example.com) [http](http://example.com) [email](mailto:team@example.com)\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n```ts\nconst x = 1;\nconst y = 2;\n```';
 render(<ReviewMessageBody request={request} tenant="tenant" hasDocument />);
 const question=screen.getByRole('region',{name:'Review message'});
 expect(within(question).getByRole('heading',{name:'Proceed?'})).toBeInTheDocument();
 expect(within(question).getByText('bold').tagName).toBe('STRONG');
 expect(question.querySelector('em')).toHaveTextContent('emphasis');expect(within(question).getAllByRole('listitem')).toHaveLength(2);
 expect(within(question).getByRole('table')).toBeInTheDocument();expect(question.querySelector('pre code')).toHaveTextContent('const x = 1;');
 expect(question.querySelector('pre code')?.textContent).toBe('const x = 1;\nconst y = 2;\n');
 expect(question.querySelector('p br')).toBeInTheDocument();
 for(const [name,href] of [['web','https://example.com'],['http','http://example.com'],['email','mailto:team@example.com']]){
  const link=within(question).getByRole('link',{name});expect(link).toHaveAttribute('href',href);expect(link).toHaveAttribute('target','_blank');expect(link).toHaveAttribute('rel','noopener noreferrer');
 }
});
it('shows only the current answer and its attachments, then the prompt and divider', () => {
 const request = reviewV2Fixture();
 request.question = 'Proceed?';
 request.conversation = [
   {role:'agent', text:'Old answer', attachments:[]},
   {role:'human', direction:'ask', text:'Follow-up', attachments:[], receipt:{}},
   {role:'agent', text:'Current answer', attachments:[{kind:'stored',name:'result.txt',stored:{key:{jobId:'job',taskOrdinal:1,name:'result.txt'}}}]},
 ];
 render(<ReviewMessageBody request={request} tenant="tenant" hasDocument />);
 expect(screen.queryByText('Old answer')).toBeNull();
 expect(screen.queryByText('Follow-up')).toBeNull();
 const message = screen.getByRole('region', {name:'Review message'});
 expect(message).toHaveTextContent('Current answerDownload result.txtProceed?');
 expect(screen.getByRole('link', {name:'Download result.txt'})).toHaveAttribute('download');
 expect(message.nextElementSibling).toBe(screen.getByRole('separator'));
});
it('omits the divider without a document and skips an empty message', () => {
 const request = reviewV2Fixture();
 const {rerender} = render(<ReviewMessageBody request={request} tenant="tenant" hasDocument={false} />);
 expect(screen.getByText(request.question)).toBeVisible();
 expect(screen.queryByRole('separator')).toBeNull();
 request.question = ' ';
 request.conversation = [
   {role:'agent', text:'Previous answer', attachments:[]},
   {role:'human', direction:'ask', text:'Unanswered', attachments:[], receipt:{}},
 ];
 rerender(<ReviewMessageBody request={request} tenant="tenant" hasDocument />);
 expect(screen.queryByRole('region', {name:'Review message'})).toBeNull();
 expect(screen.queryByRole('separator')).toBeNull();
});
it('Q06 skips raw HTML and renders readable fallbacks for unsafe links, relative links and all images',()=>{
 const request={...reviewV2Fixture(),question:'<script>alert(1)</script>\n\n<div>hidden html</div>\n\n[unsafe](javascript:alert) [relative](review/a.md) [data](data:text/plain,hello)\n\n![diagram](https://example.com/diagram.png)'};
 const {container}=render(<ReviewMessageBody request={request} tenant="tenant" hasDocument />);
 expect(container.querySelector('script')).toBeNull();expect(screen.queryByText('hidden html')).toBeNull();expect(screen.queryAllByRole('link')).toHaveLength(0);
 for(const text of ['unsafe','relative','data'])expect(screen.getByText(`${text} (link unavailable)`)).toBeInTheDocument();
 expect(screen.getByText('Image unavailable: diagram')).toBeInTheDocument();expect(container.querySelector('img')).toBeNull();
});
it('retains a focused question link through layout-driven parent renders',()=>{
 const request={...reviewV2Fixture(),question:'[Reference](https://example.com)'};
 const {rerender}=render(<ReviewMessageBody request={request} tenant="tenant" hasDocument />);
 const link=screen.getByRole('link',{name:'Reference'});link.focus();
 rerender(<ReviewMessageBody request={{...request}} tenant="tenant" hasDocument={false} />);
 expect(screen.getByRole('link',{name:'Reference'})).toBe(link);expect(link).toHaveFocus();
});
