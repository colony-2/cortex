import { expect, type Locator, type Page } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { authorizeV2, reviewV2BrowserFixture, v2Path, v2Ids, v2Selected, v2Catalog, v2Attachment, v2Preview, v2Primary, v2ClosePreview } from './helpers/reviewV2';

test.beforeEach(async({page,baseURL})=>authorizeV2(page,baseURL!));
const replyWindow = (page:Page) => page.getByRole('region', { name: 'Popped-out review reply' });
const closeReply = (page:Page) => replyWindow(page).getByRole('button', { name: 'Close composer' }).click();
const askQuestion = (page:Page) => page.getByRole('button', { name: 'Ask a question', exact: true }).click();
async function geometry(page:Page){
 await expect.poll(()=>page.evaluate(()=>{
  const pane=document.querySelector('.embedded-review .review-v2-response, .review-v2 .review-v2-response');
  const review=document.querySelector('.review-v2-layout')!;
  return {overflow:document.documentElement.scrollWidth>innerWidth+1,inline:!pane||getComputedStyle(pane).position==='static',below:!pane||pane.getBoundingClientRect().top>=review.getBoundingClientRect().bottom};
 })).toEqual({overflow:false,inline:true,below:true});
}
async function screenshot(page:Page,name:string){await page.screenshot({path:test.info().outputPath(name),fullPage:false});}

for(const width of [1440,390])test(`N06–10 v2 history retains text, files, composer and settings at ${width}px`,async({page,browser,baseURL})=>{
 await page.setViewportSize({width,height:900});const fixture=await reviewV2BrowserFixture(page);
 await page.goto('/project/v2/reviews');await page.goto(v2Path+'?filter=one&filter=two#read');await v2Selected(page,0);
 const logical=new URL(page.url()).searchParams.get('review_id');expect(logical).toBe(fixture.state.request.review_id);
 const initialDetails=fixture.state.details;
 await expect(page.locator('summary').filter({hasText:'Make reviews easier to read'}).getByText('platform',{exact:true})).toBeVisible();
 if(width===1440){
  const opener=page.getByRole('button',{name:'Collapse navigation',exact:true});
  const cellLink=page.getByRole('navigation',{name:'Inboxes',exact:true}).getByRole('link',{name:'platform',exact:true});
  await expect(cellLink).toBeVisible();
  await opener.click();await expect(page.locator('.desktop-navigation')).toHaveCSS('width','76px');
  await page.getByRole('button',{name:'Expand navigation',exact:true}).click();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width','256px');
 }
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 const editor=page.getByLabel('Question or requested change (required)',{exact:true});await expect(editor).toBeFocused();
 await editor.fill('Keep these notes while I compare documents.');
 await page.getByLabel('Attach notes',{exact:true}).setInputFiles({name:'notes.md',mimeType:'text/markdown',buffer:Buffer.from('annotations')});
 if(width>=768) await page.locator('#review-v2-files').evaluate(el=>el.setAttribute('data-retained','yes'));
 await page.getByRole('button',{name:'Review settings'}).click();
 await page.getByRole('dialog').getByLabel('Example count').fill('0');await page.getByRole('button',{name:'Done',exact:true}).click();
 if(width<768) await closeReply(page);
 const primary = v2Primary(page).getByRole('heading',{name:'Viewer changes'});
 await primary.evaluate(element=>element.setAttribute('data-preview-identity','kept'));
 await v2Catalog(page,1);
 await v2Preview(page).getByRole('link',{name:'Read source notes',exact:true}).click();await v2Selected(page,2);
 await expect(primary).toHaveAttribute('data-preview-identity','kept');
 await expect.poll(async()=>(await v2Preview(page).boundingBox())?.width).toBeCloseTo(width*0.95,0);
 await expect.poll(async()=>(await v2Preview(page).boundingBox())?.height).toBeCloseTo(900*0.95,0);
 await expect.poll(()=>v2Preview(page).locator('.ant-modal-body').evaluate(el=>el.scrollHeight>el.clientHeight)).toBe(true);
 const pageTop = await page.evaluate(()=>window.scrollY);
 await v2Preview(page).locator('.ant-modal-body').hover();
 await page.mouse.wheel(0,340);
 await expect.poll(()=>v2Preview(page).locator('.ant-modal-body').evaluate(el=>el.scrollTop)).toBeGreaterThan(0);
 expect(await page.evaluate(()=>window.scrollY)).toBe(pageTop);
 for(const [direction,index] of [['back',1],['back',0],['forward',1],['forward',2]] as const){
  if(direction==='back')await page.goBack();else await page.goForward();await v2Selected(page,index);
  if(width>=768){
   await expect(editor).toHaveValue('Keep these notes while I compare documents.');await expect(page.locator('#review-v2-files')).toHaveAttribute('data-retained','yes');
   await expect(page.getByRole('button',{name:'Remove notes.md (1)'})).toBeVisible();await expect(page.locator('.review-v2-response')).toContainText('Retained locally; applies to Request changes');
  }
 }
 expect(fixture.state.details).toBe(initialDetails);expect(fixture.state.submits).toHaveLength(0);expect(new URL(page.url()).searchParams.getAll('filter')).toEqual(['one','two']);expect(new URL(page.url()).hash).toBe('#read');
 await page.goBack();await v2Selected(page,1);
 const shared=page.url();
 await expect.poll(()=>v2Preview(page).evaluate(element=>element.contains(document.activeElement))).toBe(true);
 await page.keyboard.press('Escape');await v2Selected(page,0);
 if(width<768){
   await askQuestion(page);
   await expect(editor).toHaveValue('Keep these notes while I compare documents.');
   await expect(replyWindow(page).getByRole('button',{name:'Remove notes.md (1)'})).toBeVisible();
   await expect(replyWindow(page)).toContainText('Retained locally; applies to Request changes');
 }
 await page.getByRole('button',{name:'Review settings'}).click();
 await expect(page.getByRole('dialog',{name:'Review settings'}).getByLabel('Example count')).toHaveValue('0');
 await page.getByRole('button',{name:'Done',exact:true}).click();
 if(width<768) await closeReply(page);
 await page.goto(shared);await v2Selected(page,1);
 await page.reload();await v2Selected(page,1);
 const other=await browser.newContext({baseURL});try{const recipient=await other.newPage();await authorizeV2(recipient,baseURL!);await reviewV2BrowserFixture(recipient);await recipient.goto(shared);await v2Selected(recipient,1);}finally{await other.close();}
});
test('N06/N08 catalog, source and Markdown links share history without same-document entries',async({page})=>{
 await page.setViewportSize({width:1440,height:900});await reviewV2BrowserFixture(page);await page.goto('/project/v2/reviews');await page.goto(v2Path);await v2Selected(page,0);
 await v2Primary(page).getByRole('button',{name:'Document options',exact:true}).click();
 const options=page.getByRole('dialog',{name:'Document options',exact:true});
 await expect(options.getByRole('link',{name:'Download',exact:true})).toHaveAttribute('href',/plan.md/);
 await options.getByRole('link',{name:/Acceptance checks/}).click();await v2Selected(page,1);
 await page.getByRole('link',{name:'Same heading',exact:true}).click();await page.goBack();await v2Selected(page,0);
 await page.getByRole('link',{name:'Open checks',exact:true}).click();await v2Selected(page,1);await page.getByRole('link',{name:'Read source notes',exact:true}).click();await v2Selected(page,2);
 await page.goBack();await v2Selected(page,1);await page.goBack();await v2Selected(page,0);await page.goBack();await expect(page).toHaveURL('/project/v2/jobs?inbox=reviews');
});
test('N10/N11 missing originals replace selection without losing drafts; Retry and late reads do not change history',async({page})=>{
 const f=await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Retained after removal');
 f.state.failures.set(v2Ids[1],503);
 await v2Attachment(page,1).click();
 await expect(v2Preview(page).getByLabel('Selected document').getByRole('alert')).toContainText('Could not load this document');
 f.state.failures.delete(v2Ids[1]);await v2Preview(page).getByLabel('Selected document').getByRole('button',{name:'Retry',exact:true}).click();await v2Selected(page,1);
 let release!:()=>void;f.state.holds.set(v2Ids[2],new Promise(r=>{release=r;}));
 // Start the request without waiting for its preview.
 await v2Preview(page).getByRole('link',{name:'Read source notes',exact:true}).click();await expect.poll(()=>f.state.reads.includes(v2Ids[2])).toBe(true);
 await page.goBack();await v2Selected(page,1);release();await expect.poll(()=>f.state.complete.includes(v2Ids[2])).toBe(true);await v2Selected(page,1);
 delete f.state.request.documents[v2Ids[1]];delete f.state.request.document_metadata![v2Ids[1]];f.state.request.document_metadata![v2Ids[0]].source_document_ids=[];
 await v2ClosePreview(page);
 f.state.rejected=true;
 await page.getByRole('button',{name:'Send · Ask a question'}).click();
 await expect(page.getByText(/could not be accepted/i)).toBeVisible();
 await expect(page.getByLabel('Question or requested change (required)')).toHaveValue('Retained after removal');
 await page.goto(v2Path+'?document_id='+encodeURIComponent(v2Ids[1]));
 await expect(page.getByText(/linked document is unavailable/)).toBeVisible();await v2Selected(page,0);

});
test('N04/R05 duplicate links fail closed and another retained review cannot answer the live request',async({page})=>{
 const f=await reviewV2BrowserFixture(page);await page.goto(v2Path+'?review_id='+encodeURIComponent(f.state.request.review_id)+'&review_id=other&document_id='+encodeURIComponent(v2Ids[0]));
 await expect(page.getByText('This review link is malformed')).toBeVisible();await expect(page.getByRole('button',{name:'Approve direction',exact:true})).toHaveCount(0);expect(f.state.reads).toHaveLength(0);
 await page.getByRole('button',{name:'Open selected review'}).click();await v2Selected(page,0);
 f.state.liveReviewId='another-review';await page.reload();await v2Selected(page,0);await expect(page.locator('.review-v2-response')).toContainText('Another request is now pending');await expect(page.getByRole('button',{name:'Approve direction',exact:true})).toHaveCount(0);expect(f.state.submits).toHaveLength(0);
});
for(const width of [1680,1320,320,390])test(`L01–05 response and attachments, inline previews and composer at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900});await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);await geometry(page);
 const attachments = page.getByRole('navigation',{name:'Review attachments'});
 await expect(attachments.getByRole('button')).toHaveText(['Acceptance checks','Source notes','Code changes','Layout image','Changes']);
 await expect(page.locator('.thread-message-subject').getByText('Make reviews easier to read',{exact:true})).toHaveCount(1);
 await expect(page.getByRole('heading',{name:'Response',exact:true})).toHaveCount(0);
 expect(await attachments.evaluate(nav => !!(nav.compareDocumentPosition(document.querySelector('.review-document-content')!) & Node.DOCUMENT_POSITION_PRECEDING))).toBe(true);
 if(width>=1320){expect(await widthOf(page,'.review-reading-track')).toBeGreaterThanOrEqual(width-700);await screenshot(page,`v2-reading-${width}.png`);}
 const startReply = async () => {
  await askQuestion(page);
  await page.getByLabel('Question or requested change (required)').fill('Can you confirm the response stays in view?');
  await page.getByLabel('Attach notes').setInputFiles({name:'my-review.md',mimeType:'text/markdown',buffer:Buffer.from('notes')});
 };
 if(width>=768) await startReply();
 await expect(page.locator('.review-selected-details')).toHaveCount(0);
 await attachments.scrollIntoViewIfNeeded();
 const tail=await attachments.boundingBox();const bar=await page.locator('.review-v2-response').boundingBox();expect(tail!.y+tail!.height).toBeLessThanOrEqual(bar!.y);
 for(const index of [2,3,4]){await v2Catalog(page,index);await geometry(page);}
 await v2Catalog(page,1);await geometry(page);
 await v2Catalog(page,0);
 await expect(page.locator('.thread-message-subject').getByText('Make reviews easier to read',{exact:true})).toHaveCount(1);
 await expect(page.getByRole('heading',{name:'Response',exact:true})).toHaveCount(0);
 if(width<768) await startReply();
 await page.getByRole('button',{name:'Send · Ask a question'}).scrollIntoViewIfNeeded();
 await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 if(width===1680||width===390||width===320)await screenshot(page,`v2-composer-${width}.png`);
 await expect.poll(()=>page.getByRole('button',{name:'Send · Ask a question'}).evaluate(element=>{const box=element.getBoundingClientRect();return element.contains(document.elementFromPoint(box.x+box.width/2,box.y+box.height/2));})).toBe(true);
 await page.getByRole('button',{name:'Back',exact:true}).click();await geometry(page);
});
test('P01/P05/N09 accepted question shows pending; successor retains document and uses new native request',async({page})=>{
 await page.setViewportSize({width:1440,height:900});const f=await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);await v2Catalog(page,1);await v2ClosePreview(page);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Does Back preserve a file draft?');
 const reviewURL=page.url();
 await page.getByRole('button',{name:'Send · Ask a question'}).click();
 await expect(page).toHaveURL(/\/jobs\?inbox=reviews$/);
 await page.goto(reviewURL);await expect(page.getByText('Response pending',{exact:true})).toBeVisible();await geometry(page);await screenshot(page,'v2-response-pending.png');
 expect(f.state.submits[0].request_id).toBe('native-one');expect(f.state.submits[0].response.direction).toBe('ask');
 await f.successor();
 await expect(page.getByText('Response pending',{exact:true})).toHaveCount(0);await v2Selected(page,0);
 await expect(page.getByText('The documents and drafts stay available. I can proceed after your approval.')).toBeVisible();
 await page.getByRole('button',{name:'Approve direction',exact:true}).click();
 expect(f.state.submits).toHaveLength(1);
 await page.getByRole('button',{name:'Send · Approve direction',exact:true}).click();await expect.poll(()=>f.state.submits.length).toBe(2);expect(f.state.submits[1]).toMatchObject({request_id:'native-two',response:{direction:'go',feedback:'',attachments:[],settings:{}}});
 await expect(page.getByText('Response pending',{exact:true})).toHaveCount(0);
});
test('choosing Request changes at the bottom brings Send into view and focuses feedback', async ({page}) => {
 await page.setViewportSize({width:1280, height:640});
 await reviewV2BrowserFixture(page);
 await page.goto(v2Path);
 await v2Selected(page,0);
 await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
 const changes = page.getByRole('button', {name:'Request changes', exact:true});
 await expect(changes).toBeInViewport({ratio:1});
 await changes.click();
 await expect(page.getByRole('textbox', {name:'Question or requested change (required)'})).toBeFocused();
 await expect(page.getByRole('button', {name:'Send · Request changes'})).toBeInViewport({ratio:1, timeout:3000});
});

test('the inline reply can switch between a recipe choice, feedback and approval without posting a draft', async ({page})=>{
 const fixture=await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);
 const composer=page.getByRole('region',{name:'Response composer'});
 await expect(composer).toHaveCount(0);
 await page.getByRole('button',{name:'Another recipe choice',exact:true}).click();
 await expect(composer.getByText('review@example.com')).toBeVisible();
 const recipeType=composer.getByRole('button',{name:'Change response type: Another recipe choice'});
 await expect(recipeType).toHaveText('Another recipe choice');
 await composer.getByLabel('Question or requested change (optional)').fill('Keep this draft');
 await recipeType.click();
 await page.getByRole('menuitem',{name:'Approve direction'}).click();
 await expect(composer.getByRole('button',{name:'Change response type: Approve direction'}).locator('svg[data-icon="check-circle"]')).toBeVisible();
 await expect(composer.getByLabel('Question or requested change (not allowed)')).toHaveValue('Keep this draft');
 expect(fixture.state.submits).toHaveLength(0);
 await composer.getByRole('button',{name:'Remove feedback draft'}).click();
 await expect(composer.getByRole('textbox')).toHaveCount(0);
 await composer.getByRole('button',{name:'Send · Approve direction'}).click();
 await expect.poll(()=>fixture.state.submits.length).toBe(1);
 expect(fixture.state.submits[0].response).toMatchObject({direction:'go',feedback:''});
});
test('L07–09 the simulated keyboard leaves the full-screen reply and its send control reachable',async({page})=>{
 await page.setViewportSize({width:390,height:844});const fixture=await reviewV2BrowserFixture(page);fixture.state.request.question=formattedQuestion;await page.goto(v2Path);await v2Selected(page,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(page.getByRole('alert')).toContainText('requires feedback');
 await page.evaluate(()=>{Object.defineProperty(window.visualViewport!,'height',{configurable:true,value:420});Object.defineProperty(window.visualViewport!,'offsetTop',{configurable:true,value:0});window.visualViewport!.dispatchEvent(new Event('resize'));window.dispatchEvent(new Event('resize'));});
 // Exercise the browser viewport event target with a simulated keyboard height.
 const reply = replyWindow(page);
 await expect.poll(async () => (await reply.boundingBox())?.height).toBe(420);
 await expect(reply.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 await page.evaluate(()=>{Object.defineProperty(window.visualViewport!,'offsetTop',{configurable:true,value:30});window.visualViewport!.dispatchEvent(new Event('scroll'));});
 await expect.poll(async () => (await reply.boundingBox())?.y).toBe(30);
 await closeReply(page);
 await reviewMessage(page).scrollIntoViewIfNeeded();await expect(reviewMessage(page).getByRole('heading',{name:'Ready to proceed?'})).toBeVisible();
 await page.getByRole('link',{name:'Back to inbox'}).click();
 await expect.poll(()=>page.evaluate(()=>document.documentElement.style.getPropertyValue('--review-action-height'))).toBe('');
 await page.evaluate(()=>window.dispatchEvent(new Event('resize')));expect(await page.evaluate(()=>document.documentElement.style.getPropertyValue('--review-action-height'))).toBe('');
});
for (const width of [390, 1680]) test(`the latest answer and prompt precede the primary document at ${width}px`, async ({page}) => {
 const fixture = await reviewV2BrowserFixture(page);
 fixture.state.request.conversation = [
  {role:'agent', text:'Earlier answer', attachments:[]},
  {role:'human', direction:'ask', text:'My follow-up', attachments:[], receipt:{}},
  {role:'agent', text:'Latest answer', attachments:[]},
 ];
 await page.setViewportSize({width, height:900});
 await page.goto(v2Path);
 await v2Selected(page,0);
 const message = reviewMessage(page);
 await expect(message).toHaveText('Latest answer' + fixture.state.request.question);
 await expect(page.getByRole('button', {name:/show earlier turns/i})).toHaveCount(0);
 const answer = (await message.getByText('Latest answer').boundingBox())!;
 const prompt = (await message.getByText(fixture.state.request.question).boundingBox())!;
 const line = (await page.locator('.review-message-divider').boundingBox())!;
 const primary = (await v2Primary(page).boundingBox())!;
 expect(answer.y + answer.height).toBeLessThanOrEqual(prompt.y);
 expect(prompt.y + prompt.height).toBeLessThanOrEqual(line.y);
 expect(line.y + line.height).toBeLessThanOrEqual(primary.y);
 await geometry(page);
});
test.describe('actual browser zoom',()=>{
 test.use({nativeZoom:true});
 test('L06 200% zoom preserves reachable composer and context',async({page})=>{
  const f=await reviewV2BrowserFixture(page);f.state.request.question=formattedQuestion;await page.goto(v2Path);await v2Selected(page,0);await setNativeZoom(page,2);await geometry(page);
  await expect(page.getByRole('tab',{name:'Thread',exact:true})).toBeVisible();await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Zoomed feedback');await geometry(page);
  const send=page.getByRole('button',{name:'Send · Ask a question'});
  await send.click({trial:true});await expect(send).toBeInViewport({ratio:1});
  await expect.poll(()=>send.evaluate(element=>{
   const button=element.getBoundingClientRect();const pane=element.closest('.review-v2-response')!.getBoundingClientRect();
   return button.top>=pane.top&&button.bottom<=Math.min(innerHeight,pane.bottom);
  })).toBe(true);
  if(await replyWindow(page).isVisible()){
   await captureNativeZoom(page,test.info().outputPath('v2-browser-zoom-200.png'));
   await closeReply(page);
  }
  await expect(page.locator('.review-message-divider')).toBeVisible();const question=reviewMessage(page);await question.scrollIntoViewIfNeeded();await expect(question.getByRole('heading',{name:'Ready to proceed?'})).toBeVisible();await geometry(page);
  await question.locator('pre').scrollIntoViewIfNeeded();const finalCode=(await question.locator('pre').boundingBox())!,response=(await page.locator('.review-v2-response').boundingBox())!;expect(finalCode.y+finalCode.height).toBeLessThanOrEqual(response.y+1);
  await captureNativeZoom(page,test.info().outputPath('v2-browser-zoom-message-200.png'));
 });
});

const formattedQuestion = '# Ready to proceed?\n\nReview the **formatted** instructions.\nKeep this authored line break.\n\n- Compare the plan\n- Confirm the checks\n\n[Public reference](https://example.com)\n\n| Check | Result |\n| --- | --- |\n| Formatting | Preserved |\n\n```ts\nconst proceed = true;\n```';
const reviewMessage = (page:Page) => page.locator('.review-v2-layout').getByRole('region',{name:'Review message'});
async function widthOf(page:Page,selector:string){return page.locator(selector).evaluate(el=>el.getBoundingClientRect().width);}
test('B01 formatted question stays final through waiting, saved review and successor replacement',async({page})=>{
 await page.setViewportSize({width:1680,height:1000});const f=await reviewV2BrowserFixture(page);f.state.request.question=formattedQuestion;
 await page.goto(v2Path);await v2Selected(page,0);
 const last=reviewMessage(page);
 await expect(last.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(1);
 await expect(page.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(1);await expect(last.locator('time')).toHaveCount(0);
 await expect(last.getByRole('table')).toHaveCount(1);await expect(last.locator('pre code')).toContainText('const proceed = true;');
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Does the formatting survive submission?');
 const reviewURL=page.url();
 await page.getByRole('button',{name:'Send · Ask a question'}).click();
 await expect(page).toHaveURL(/\/jobs\?inbox=reviews$/);
 await page.goto(reviewURL);await expect(page.getByText('Response pending',{exact:true})).toBeVisible();
 await expect(last.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(1);
 await expect(reviewMessage(page)).not.toContainText('Does the formatting survive submission?');
 await page.reload();await v2Selected(page,0);await expect(last.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(1);
 await f.successor();f.state.request.question='## Successor question\n\nThe next **decision**.';
 await expect(last.getByRole('heading',{name:'Successor question'})).toHaveCount(1,{timeout:20000});
 await expect(page.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(0);await expect(page.getByText('Response pending',{exact:true})).toHaveCount(0);
 await page.getByRole('button',{name:'Approve direction',exact:true}).click();
 await page.getByRole('button',{name:'Send · Approve direction',exact:true}).click();await expect(page).toHaveURL(/\/jobs\?inbox=reviews$/);
 await page.goto(reviewURL);await v2Selected(page,0);await expect(last.getByRole('heading',{name:'Successor question'})).toHaveCount(1);
});

for(const width of [320,390,980])test(`B12/13 long formatted question, code and tables remain reachable at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900});const f=await reviewV2BrowserFixture(page);f.state.request.question=formattedQuestion+'\n\n'+('Long question paragraph with more review instructions.\n\n'.repeat(30))+'```text\n'+('very_long_value_'.repeat(50))+'\n```\n\n| '+('Wide column '.repeat(20))+' | Result |\n| --- | --- |\n| value | passes |\n\nFINAL QUESTION CONTENT';
 await page.goto(v2Path);await v2Selected(page,0);
 await expect(page.locator('.review-message-divider')).toBeVisible();await geometry(page);
 const resultCell=reviewMessage(page).getByRole('table').last().getByRole('columnheader',{name:'Result',exact:true});expect(await resultCell.evaluate(el=>el.getBoundingClientRect().width)).toBeGreaterThanOrEqual(96);
 const tail=reviewMessage(page).getByText('FINAL QUESTION CONTENT',{exact:true});await tail.scrollIntoViewIfNeeded();await expect(tail).toBeInViewport();const bounds=(await tail.boundingBox())!,response=(await page.locator('.review-v2-response').boundingBox())!;expect(bounds.y+bounds.height).toBeLessThanOrEqual(response.y+1);
 await askQuestion(page);await page.getByLabel('Question or requested change (required)').fill('Reach the final content');
 await page.getByRole('button',{name:'Send · Ask a question'}).scrollIntoViewIfNeeded();
 await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();await geometry(page);await screenshot(page,`v2-long-question-${width}.png`);
});

function selectedReview(reviewId: string) {
  return `${v2Path}?${new URLSearchParams({ review_id: reviewId })}`;
}

for (const width of [1920, 2560]) test(`review workspace and composer fill the thread at ${width}px`, async ({page}) => {
  await page.setViewportSize({width, height: 1050});
  const fixture = await reviewV2BrowserFixture(page);
  const target = selectedReview(fixture.state.request.review_id);
  await page.goto(target);
  await v2Selected(page, 0);
  const collapse = page.getByRole('button', {name: 'Collapse navigation'});
  await expect(collapse).toBeVisible();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width','256px');
  await page.getByRole('button', {name: 'Ask a question', exact: true}).click();
  const panel = (await page.getByRole('tabpanel', {name:'Thread', exact:true}).boundingBox())!;
  for (const selector of ['.review-v2-layout', '.review-v2-response-inner', '.review-v2-composer']) {
    const bounds = (await page.locator(selector).boundingBox())!;
    expect(bounds.x).toBeCloseTo(panel.x + 14, 0);
    expect(bounds.x + bounds.width).toBeCloseTo(panel.x + panel.width - 14, 0);
  }
  await geometry(page);
  if (width === 2560) await screenshot(page, 'review-full-width-2560.png');

  await collapse.click();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width','76px');
  await page.getByRole('button', {name: 'Expand navigation'}).click();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width','256px');

  await page.goto(target);
  await v2Selected(page, 0);
  await page.getByRole('link', {name: 'Back to inbox'}).click();
  await expect(page.getByRole('heading', {name: 'Reviews', exact: true})).toBeVisible();
  await expect(page.getByRole('button', {name: 'Collapse navigation'})).toBeVisible();
  await page.goto(`${target}&requestId=native-one`);
  await expect(page).toHaveURL(/\/project\/v2\/jobs\/one\/story\?/);
  await expect(page.getByRole('button', {name: 'Collapse navigation'})).toBeVisible();
});

const longHeader = 'A longer heading that wraps at spaces when the available pane becomes narrow or the document is shown on a small screen';
const tableFixture = [
  `| abcd | ${longHeader} |\n| ---: | :--- |\n| Entry | Ready |`,
  '| Status | Group | Area | Owner | Release | Notes | Decision | Last column |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n| Ready | Platform-engineering | Documentation | Review-coordinator | Current | Compatibility | Approved | Visible at the end |',
].join('\n\n');

async function headerLines(header: Locator) {
  return header.evaluate(element => {
    const text = element.firstChild!;
    const range = document.createRange();
    const lines = new Map<number, string>();
    [...text.textContent!].forEach((character, index) => {
      range.setStart(text, index);
      range.setEnd(text, index + 1);
      const {top} = range.getBoundingClientRect();
      lines.set(top, (lines.get(top) ?? '') + character);
    });
    return [...lines.values()].map(line => line.trim());
  });
}

async function swipeTable(page: Page, table: Locator) {
  const bounds = (await table.boundingBox())!;
  const x = bounds.x + bounds.width - 12;
  const y = bounds.y + 14;
  const session = await page.context().newCDPSession(page);
  try {
    await session.send('Input.dispatchTouchEvent', {type: 'touchStart', touchPoints: [{x, y}]});
    for (let step = 1; step <= 8; step++) {
      await session.send('Input.dispatchTouchEvent', {type: 'touchMove', touchPoints: [{x: x - (bounds.width - 24) * step / 8, y}]});
    }
    await session.send('Input.dispatchTouchEvent', {type: 'touchEnd', touchPoints: []});
  } finally {
    await session.detach();
  }
}

for (const width of [1040, 390]) test.describe(`review Markdown tables at ${width}px`, () => {
  test.use({hasTouch: width === 390});
  test('keep words intact and scroll wide tables without moving the page', async ({page}) => {
    await page.setViewportSize({width, height: 900});
    const fixture = await reviewV2BrowserFixture(page);
    fixture.state.request.question = `## Current question\n\n${tableFixture}`;
    await page.route('**/jobs/producer/stored-document?*', route => route.fulfill({
      contentType: 'text/markdown', body: `# Review table example\n\n${tableFixture}`,
    }));
    await page.goto(selectedReview(fixture.state.request.review_id));
    await v2Selected(page, 0);
    await expect(page.locator('.review-message-divider')).toBeVisible();

    for (const [surface, root] of [
      ['attachment', page.getByLabel('Rendered document')],
      ['question', reviewMessage(page).locator('.review-v2-question-markdown')],
    ] as const) {
      const regions = root.getByRole('region', {name: 'Markdown table'});
      await expect(regions).toHaveCount(2);
      const small = regions.first();
      await small.scrollIntoViewIfNeeded();
      const short = small.getByRole('columnheader', {name: 'abcd', exact: true});
      const long = small.getByRole('columnheader', {name: longHeader, exact: true});
      expect(await headerLines(short)).toEqual(['abcd']);
      const lines = await headerLines(long);
      expect(lines.length).toBeGreaterThan(1);
      expect(lines.join(' ')).toBe(longHeader);
      expect((await short.boundingBox())!.width).toBeLessThan((await long.boundingBox())!.width);
      await expect(short).toHaveCSS('text-align', 'right');
      expect(await small.evaluate(el => el.scrollWidth)).toBe(await small.evaluate(el => el.clientWidth));
      if (width === 390) await screenshot(page, `review-${surface}-table-390.png`);

      const wide = regions.last();
      const lastColumn = wide.getByRole('columnheader', {name: 'Last column', exact: true});
      await wide.scrollIntoViewIfNeeded();
      const offset = () => wide.evaluate(el => el.scrollLeft);
      const lastColumnVisible = () => lastColumn.evaluate(el => el.getBoundingClientRect().right <= el.closest('.review-table-scroll')!.getBoundingClientRect().right + 1);
      expect(await offset()).toBe(0);
      expect(await lastColumnVisible()).toBe(false);
      if (width !== 390) {
        await small.focus();
        await page.keyboard.press('Tab');
        await expect(wide).toBeFocused();
        await expect(wide.locator(':scope:focus-visible')).toHaveCount(1);
        await expect(wide).toHaveCSS('outline-width', '2px');
        await wide.scrollIntoViewIfNeeded();
      }
      const position = () => page.evaluate(() => [window.scrollX, window.scrollY]);
      const before = await position();
      if (width === 390) {
        for (let attempt = 0; attempt < 4 && !await lastColumnVisible(); attempt++) await swipeTable(page, wide);
      } else {
        for (let step = 0; step < 30; step++) await page.keyboard.press('ArrowRight');
      }
      await expect.poll(offset).toBeGreaterThan(0);
      await expect.poll(lastColumnVisible).toBe(true);
      expect(await position()).toEqual(before);
      await geometry(page);
    }
  });
});
