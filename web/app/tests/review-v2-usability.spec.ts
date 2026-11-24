import { expect, type Locator, type Page } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { authorizeV2, reviewV2BrowserFixture, v2Path, v2Ids, v2Selected, v2Catalog, v2Attachment, v2Preview, v2Primary, v2ClosePreview } from './helpers/reviewV2';

test.beforeEach(async({page,baseURL})=>authorizeV2(page,baseURL!));
async function geometry(page:Page){
 await expect.poll(()=>page.evaluate(()=>{
  const pane=document.querySelector('.review-v2-response')!;
  const review=document.querySelector('.review-v2-layout')!;
  return {overflow:document.documentElement.scrollWidth>innerWidth+1,inline:getComputedStyle(pane).position==='static',below:pane.getBoundingClientRect().top>=review.getBoundingClientRect().bottom};
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
 await page.locator('#review-v2-files').evaluate(el=>el.setAttribute('data-retained','yes'));
 await page.getByRole('button',{name:'Review settings'}).click();
 await page.getByRole('dialog').getByLabel('Example count').fill('0');await page.getByRole('button',{name:'Done',exact:true}).click();
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
  await expect(editor).toHaveValue('Keep these notes while I compare documents.');await expect(page.locator('#review-v2-files')).toHaveAttribute('data-retained','yes');
  await expect(page.getByRole('button',{name:'Remove notes.md (1)'})).toBeVisible();await expect(page.locator('.review-v2-response')).toContainText('Retained locally; applies to Request changes');
 }
 expect(fixture.state.details).toBe(initialDetails);expect(fixture.state.submits).toHaveLength(0);expect(new URL(page.url()).searchParams.getAll('filter')).toEqual(['one','two']);expect(new URL(page.url()).hash).toBe('#read');
 await page.goBack();await v2Selected(page,1);
 const shared=page.url();
 await expect.poll(()=>v2Preview(page).evaluate(element=>element.contains(document.activeElement))).toBe(true);
 await page.keyboard.press('Escape');await v2Selected(page,0);
 await page.getByRole('button',{name:'Review settings'}).click();
 await expect(page.getByRole('dialog',{name:'Review settings'}).getByLabel('Example count')).toHaveValue('0');
 await page.getByRole('button',{name:'Done',exact:true}).click();
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
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Can you confirm the response stays in view?');
 await page.getByLabel('Attach notes').setInputFiles({name:'my-review.md',mimeType:'text/markdown',buffer:Buffer.from('notes')});await geometry(page);
 await expect(page.locator('.review-selected-details')).toHaveCount(0);
 await attachments.scrollIntoViewIfNeeded();
 const tail=await attachments.boundingBox();const bar=await page.locator('.review-v2-response').boundingBox();expect(tail!.y+tail!.height).toBeLessThanOrEqual(bar!.y);
 for(const index of [2,3,4]){await v2Catalog(page,index);await geometry(page);}
 await v2Catalog(page,1);await geometry(page);
 await v2Catalog(page,0);
 await expect(page.locator('.thread-message-subject').getByText('Make reviews easier to read',{exact:true})).toHaveCount(1);
 await expect(page.getByRole('heading',{name:'Response',exact:true})).toHaveCount(0);
 await page.getByRole('button',{name:'Send · Ask a question'}).scrollIntoViewIfNeeded();
 await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 if(width===1680||width===390||width===320)await screenshot(page,`v2-composer-${width}.png`);
 await expect.poll(()=>page.getByRole('button',{name:'Send · Ask a question'}).evaluate(element=>{const box=element.getBoundingClientRect();return element.contains(document.elementFromPoint(box.x+box.width/2,box.y+box.height/2));})).toBe(true);
 await page.getByRole('button',{name:'Back',exact:true}).click();await geometry(page);
});
test('P01/P05/N09 accepted question shows pending in discussion; successor retains document and uses new native request',async({page})=>{
 await page.setViewportSize({width:1440,height:900});const f=await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);await v2Catalog(page,1);await v2ClosePreview(page);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Does Back preserve a file draft?');
 await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(page.getByRole('region',{name:'Discussion'}).getByText('Response pending',{exact:true})).toBeVisible();await geometry(page);await screenshot(page,'v2-response-pending.png');
 expect(f.state.submits[0].request_id).toBe('native-one');expect(f.state.submits[0].response.direction).toBe('ask');
 await f.successor();await page.getByRole('button',{name:'Refresh',exact:true}).click();
 await expect(page.getByText('Response pending',{exact:true})).toHaveCount(0);await v2Selected(page,0);
 await expect(page.getByText('The documents and drafts stay available. I can proceed after your approval.')).toBeVisible();
 await page.getByRole('button',{name:'Approve direction',exact:true}).click();
 expect(f.state.submits).toHaveLength(1);
 await page.getByRole('button',{name:'Send · Approve direction',exact:true}).click();await expect.poll(()=>f.state.submits.length).toBe(2);expect(f.state.submits[1]).toMatchObject({request_id:'native-two',response:{direction:'go',feedback:'',attachments:[],settings:{}}});
 await expect(page.getByText('Response pending',{exact:true})).toHaveCount(0);
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
test('L07–09 the simulated keyboard leaves the inline reply and its send control reachable',async({page})=>{
 await page.setViewportSize({width:390,height:844});const fixture=await reviewV2BrowserFixture(page);fixture.state.request.question=formattedQuestion;await page.goto(v2Path);await v2Selected(page,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(page.getByRole('alert')).toContainText('requires feedback');
 await page.evaluate(()=>{Object.defineProperty(window.visualViewport!,'height',{configurable:true,value:420});Object.defineProperty(window.visualViewport!,'offsetTop',{configurable:true,value:0});window.visualViewport!.dispatchEvent(new Event('resize'));window.dispatchEvent(new Event('resize'));});
 // Exercise the browser viewport event target with a simulated keyboard height.
 await geometry(page);await page.getByRole('button',{name:'Send · Ask a question'}).scrollIntoViewIfNeeded();await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 await page.evaluate(()=>{Object.defineProperty(window.visualViewport!,'offsetTop',{configurable:true,value:30});window.visualViewport!.dispatchEvent(new Event('scroll'));});await geometry(page);
 await discussionPane(page).getByRole('article').last().scrollIntoViewIfNeeded();await expect(discussionPane(page).getByRole('heading',{name:'Ready to proceed?'})).toBeVisible();
 await page.getByRole('button',{name:'Send · Ask a question'}).scrollIntoViewIfNeeded();
 await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 await page.getByRole('link',{name:'Back to inbox'}).click();
 await expect.poll(()=>page.evaluate(()=>document.documentElement.style.getPropertyValue('--review-action-height'))).toBe('');
 await page.evaluate(()=>window.dispatchEvent(new Event('resize')));expect(await page.evaluate(()=>document.documentElement.style.getPropertyValue('--review-action-height'))).toBe('');
});
test('L04 the document and discussion share the page scroll above the response controls',async({page})=>{
 await page.setViewportSize({width:1680,height:900});const f=await reviewV2BrowserFixture(page);
 f.state.request.conversation=Array.from({length:30},(_,index)=>({role:'agent',text:`Earlier answer ${index}. `+'Review discussion context. '.repeat(8),attachments:[]}));
 await page.goto(v2Path);await v2Selected(page,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByLabel('Question or requested change (required)').fill('Keep the last answer reachable.');
 await page.getByLabel('Attach notes').setInputFiles({name:'notes.md',mimeType:'text/markdown',buffer:Buffer.from('notes')});
 await geometry(page);
 const discussion=page.getByRole('region',{name:'Discussion'});
 const document=page.getByRole('article',{name:'Rendered document'});
 for (const surface of [document, discussion]) {
  await page.evaluate(()=>window.scrollTo(0,0));
  const metrics=await surface.evaluate(element=>{
   const {x,y,width}=element.getBoundingClientRect();
   return {x:x+width/2,y:y+30,overflow:element.scrollHeight-element.clientHeight};
  });
  expect(metrics.overflow).toBeLessThanOrEqual(1);
  await page.mouse.move(metrics.x, metrics.y);
  await page.mouse.wheel(0, 380);
  await expect.poll(()=>page.evaluate(()=>window.scrollY)).toBeGreaterThan(0);
  expect(await surface.evaluate(element=>element.scrollTop)).toBe(0);
 }
 const last=discussion.getByRole('article').last();
 await expect(last).toContainText(f.state.request.question);
 await last.scrollIntoViewIfNeeded();
 const turn=await last.boundingBox();const pane=await page.locator('.review-v2-response').boundingBox();
 expect(turn!.y+turn!.height).toBeLessThanOrEqual(pane!.y);
 const bounds=await discussion.boundingBox();expect(turn!.y+turn!.height).toBeLessThanOrEqual(bounds!.y+bounds!.height);
 await expect(last).toBeInViewport({ratio:1});
 await screenshot(page,'v2-discussion-last-turn.png');
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
  await expect(page.getByRole('separator')).toHaveCount(0);const question=discussionPane(page).getByRole('article').last();await question.scrollIntoViewIfNeeded();await expect(question.getByRole('heading',{name:'Ready to proceed?'})).toBeVisible();await geometry(page);
  await question.locator('pre').scrollIntoViewIfNeeded();const finalCode=(await question.locator('pre').boundingBox())!,response=(await page.locator('.review-v2-response').boundingBox())!;expect(finalCode.y+finalCode.height).toBeLessThanOrEqual(response.y+1);
  await captureNativeZoom(page,test.info().outputPath('v2-browser-zoom-200.png'));
 });
});

const formattedQuestion = '# Ready to proceed?\n\nReview the **formatted** instructions.\nKeep this authored line break.\n\n- Compare the plan\n- Confirm the checks\n\n[Public reference](https://example.com)\n\n| Check | Result |\n| --- | --- |\n| Formatting | Preserved |\n\n```ts\nconst proceed = true;\n```';
const discussionPane = (page:Page) => page.getByRole('region',{name:'Discussion'});
const discussionSeparator = (page:Page) => page.getByRole('separator',{name:'Resize discussion'});
async function widthOf(page:Page,selector:string){return page.locator(selector).evaluate(el=>el.getBoundingClientRect().width);}
async function dragDiscussion(page:Page,delta:number){
 const box=await discussionSeparator(page).boundingBox();
 const y=Math.max(100,Math.min(450,box!.y+30));
 await page.mouse.move(box!.x+box!.width/2,y);await page.mouse.down();await page.mouse.move(box!.x+box!.width/2+delta,y,{steps:8});await page.mouse.up();
}

test('B01 formatted question stays final through waiting, saved review and successor replacement',async({page})=>{
 await page.setViewportSize({width:1680,height:1000});const f=await reviewV2BrowserFixture(page);f.state.request.question=formattedQuestion;
 await page.goto(v2Path);await v2Selected(page,0);
 const last=discussionPane(page).getByRole('article').last();
 await expect(last).toHaveClass('review-v2-turn agent');await expect(last.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(1);
 await expect(page.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(1);await expect(last.locator('time')).toHaveCount(0);
 await expect(last.getByRole('table')).toHaveCount(1);await expect(last.locator('pre code')).toContainText('const proceed = true;');
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Does the formatting survive submission?');
 await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(discussionPane(page).getByRole('status')).toHaveText('Response pending');
 await expect(last.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(1);
 expect(await discussionPane(page).evaluate(el=>el.lastElementChild?.tagName)).toBe('ARTICLE');
 await page.reload();await v2Selected(page,0);await expect(last.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(1);
 await f.successor();f.state.request.question='## Successor question\n\nThe next **decision**.';
 await page.getByRole('button',{name:'Refresh',exact:true}).click();await expect(last.getByRole('heading',{name:'Successor question'})).toHaveCount(1);
 await expect(page.getByRole('heading',{name:'Ready to proceed?'})).toHaveCount(0);await expect(discussionPane(page).getByText('Response pending',{exact:true})).toHaveCount(0);
 await page.getByRole('button',{name:'Approve direction',exact:true}).click();
 await page.getByRole('button',{name:'Send · Approve direction',exact:true}).click();await expect(page.locator('.review-v2-response')).toContainText('Response sent');
 await page.reload();await v2Selected(page,0);await expect(last.getByRole('heading',{name:'Successor question'})).toHaveCount(1);
});

test('B02–05/B09 discussion resizes beside the response and retain previews, response drafts, files and settings',async({page})=>{
 await page.setViewportSize({width:1920,height:1100});const f=await reviewV2BrowserFixture(page);f.state.request.question=formattedQuestion;await page.goto(v2Path);await v2Selected(page,0);
 const originalURL=page.url();const reads=f.state.reads.length;
 await page.getByRole('article',{name:'Rendered document'}).evaluate(el=>el.setAttribute('data-preview-identity','kept'));
 await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow','320');
 await expect.poll(()=>page.evaluate(()=>{
  const reader=document.querySelector('.review-reading-track')!.getBoundingClientRect();
  const divider=document.querySelector('.review-v2-separator')!.getBoundingClientRect();
  const discussion=document.querySelector('.review-v2-discussion')!.getBoundingClientRect();
  return [reader.right-divider.left, divider.right-discussion.left];
 })).toEqual([0,0]);
 const discussionBefore=await widthOf(page,'.review-v2-discussion'),readerBefore=await widthOf(page,'.review-reading-track');
 await dragDiscussion(page,-96);
 const discussionWidth=await widthOf(page,'.review-v2-discussion');
 expect(discussionWidth).toBeCloseTo(discussionBefore+96,0);
 expect(await widthOf(page,'.review-reading-track')).toBeCloseTo(readerBefore-96,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();const editor=page.getByLabel('Question or requested change (required)');await editor.fill('Keep my pane comparison.');
 await page.getByLabel('Attach notes').setInputFiles({name:'comparison.md',mimeType:'text/markdown',buffer:Buffer.from('draft')});
 await page.getByRole('button',{name:'Review settings'}).click();await page.getByRole('dialog').getByLabel('Example count').fill('3');await page.getByRole('button',{name:'Done',exact:true}).click();
 await discussionPane(page).getByRole('button',{name:'Hide discussion',exact:true}).click();await expect(page.getByRole('separator')).toHaveCount(0);await expect(discussionPane(page)).toHaveCount(0);
 expect(await widthOf(page,'.review-reading-track')).toBeCloseTo(await widthOf(page,'.review-v2-layout'),0);expect(await widthOf(page,'.review-reading-track')).toBeGreaterThan(900);
 await screenshot(page,'v2-reader-only.png');
 const button=v2Primary(page).locator('.review-document-options').getByRole('button',{name:'Show discussion',exact:true});await expect(button).toBeVisible();await button.hover();
 await expect(page.getByRole('tooltip',{name:'Show discussion',exact:true})).toBeVisible();
 const id=await button.getAttribute('aria-controls');expect(id).toBeTruthy();expect(await page.evaluate(id=>!!document.getElementById(id!),id)).toBe(true);
 await button.click();
 await expect(discussionPane(page).getByRole('button',{name:'Hide discussion',exact:true})).toHaveAttribute('aria-expanded','true');
 expect(await widthOf(page,'.review-v2-discussion')).toBeCloseTo(discussionWidth,0);
 await expect(page.getByRole('article',{name:'Rendered document'})).toHaveAttribute('data-preview-identity','kept');expect(f.state.reads.length).toBe(reads);expect(page.url()).toBe(originalURL);expect(f.state.submits).toHaveLength(0);
 await expect(editor).toHaveValue('Keep my pane comparison.');await expect(page.getByRole('button',{name:'Remove comparison.md (1)'})).toBeVisible();
 await page.getByRole('button',{name:'Review settings'}).click();await expect(page.getByRole('dialog').getByLabel('Example count')).toHaveValue('3');await page.getByRole('button',{name:'Done',exact:true}).click();
 await page.getByRole('button',{name:'Back',exact:true}).click();
 await page.mouse.move(400,110);await page.evaluate(()=>window.scrollTo(0,0));await screenshot(page,'v2-formatted-resized-panes.png');
});

test('B06–08 keyboard, focus recovery, clamp limits and pointer cancellation',async({page})=>{
 await page.setViewportSize({width:1680,height:1000});const f=await reviewV2BrowserFixture(page);f.state.request.question='[Public reference](https://example.com)';await page.goto(v2Path);await v2Selected(page,0);
 const separator=discussionSeparator(page);
 await separator.focus();const initial=Number(await separator.getAttribute('aria-valuenow'));
 await page.keyboard.press('ArrowRight');await expect(separator).toHaveAttribute('aria-valuenow',String(initial-16));
 await page.keyboard.press('ArrowLeft');await expect(separator).toHaveAttribute('aria-valuenow',String(initial));
 await page.keyboard.press('Home');await expect(separator).toHaveAttribute('aria-valuenow','240');
 await page.keyboard.press('End');expect(Number(await separator.getAttribute('aria-valuenow'))).toBe(Number(await separator.getAttribute('aria-valuemax')));
 expect(await widthOf(page,'.review-reading-track')).toBeGreaterThanOrEqual(359);
 await separator.press('Home');
 await page.evaluate(()=>window.scrollTo(0,0));
 await dragDiscussion(page,500);await expect(separator).toHaveAttribute('aria-valuenow','240');
 await dragDiscussion(page,-2000);expect(await separator.getAttribute('aria-valuenow')).toBe(await separator.getAttribute('aria-valuemax'));
 await separator.press('Home');
 // Programmatic activation leaves focus in the pane, exercising explicit focus recovery.
 await discussionPane(page).getByRole('link',{name:'Public reference'}).focus();await expect(discussionPane(page).getByRole('link',{name:'Public reference'})).toBeFocused();
 await page.getByRole('button',{name:'Hide discussion',exact:true}).evaluate((el:HTMLButtonElement)=>el.click());await expect(page.getByRole('button',{name:'Show discussion',exact:true})).toBeFocused();
 await expect(page.getByRole('link',{name:'Public reference'})).toHaveCount(0);await page.keyboard.press('Tab');expect(await page.evaluate(()=>document.activeElement?.closest('.review-v2-discussion')!==null)).toBe(false);
 await page.getByRole('button',{name:'Show discussion',exact:true}).click();
 await page.evaluate(()=>window.scrollTo(0,0));const box=(await separator.boundingBox())!;
 await page.mouse.move(box.x+4,box.y+30);await page.mouse.down();await page.mouse.move(box.x-70,box.y+30);await expect(separator).toHaveClass(/dragging/);
 await separator.dispatchEvent('pointercancel',{pointerId:1});const stopped=await widthOf(page,'.review-v2-discussion');await page.mouse.move(box.x-130,box.y+30);await page.mouse.up();expect(await widthOf(page,'.review-v2-discussion')).toBe(stopped);await expect(page.locator('.review-v2-layout .dragging')).toHaveCount(0);
 const next=(await separator.boundingBox())!;await page.mouse.move(next.x+4,next.y+30);await page.mouse.down();await page.getByRole('button',{name:'Hide discussion',exact:true}).evaluate((el:HTMLButtonElement)=>el.click());await page.mouse.up();await expect(page.locator('.review-v2-layout .dragging')).toHaveCount(0);
});

test('B08 touch capture resizes discussion and releases on cancellation',async({page})=>{
 await page.setViewportSize({width:1680,height:1000});await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);
 const box=(await discussionSeparator(page).boundingBox())!;const start=Number(await discussionSeparator(page).getAttribute('aria-valuenow'));
 const session=await page.context().newCDPSession(page);
 try{
  await session.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:box.x+4,y:box.y+40}]});
  await session.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:box.x-60,y:box.y+40}]});
  await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow',String(start+64));
  await session.send('Input.dispatchTouchEvent',{type:'touchCancel',touchPoints:[]});await expect(page.locator('.review-v2-layout .dragging')).toHaveCount(0);
 }finally{await session.detach();}
});

test('B10/B14 clamping, stacking, successor continuity and navigation reset preserve preferences',async({page})=>{
 await page.setViewportSize({width:1920,height:1000});const f=await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);
 await dragDiscussion(page,-480);
 const saved = await widthOf(page,'.review-v2-discussion');
 await page.setViewportSize({width:1001,height:1000});
 await expect.poll(() => widthOf(page,'.review-v2-discussion')).toBeLessThan(saved);
 await expect(page.locator('.review-v2-layout')).not.toHaveClass(/stacked/);await geometry(page);
 await page.setViewportSize({width:980,height:1000});await expect(page.locator('.review-v2-layout')).toHaveClass(/stacked/);await expect(page.getByRole('separator')).toHaveCount(0);
 await expect(page.getByRole('navigation',{name:'Review attachments'})).toBeVisible();await geometry(page);
 await page.setViewportSize({width:1920,height:1000});await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow',String(saved));
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByLabel('Question or requested change (required)').fill('Continue');
 await page.getByRole('button',{name:'Send · Ask a question'}).click();
 await expect(discussionPane(page).getByRole('status')).toHaveText('Response pending');
 await f.successor();
 await expect(discussionPane(page).getByText('The documents and drafts stay available. I can proceed after your approval.',{exact:true})).toBeVisible();
 await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow',String(saved));
 await page.getByRole('button',{name:'Hide discussion',exact:true}).click();await page.reload();await v2Selected(page,0);await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow','320');
 await page.getByRole('button',{name:'Hide discussion',exact:true}).click();const target=page.url();await page.getByRole('link',{name:'Back to inbox'}).click();await page.goto(target);await v2Selected(page,0);await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow','320');
});

test('B11 discussion shares the page reading position and reveals older turns in place',async({page})=>{
 await page.setViewportSize({width:1680,height:1000});const f=await reviewV2BrowserFixture(page);f.state.request.conversation=Array.from({length:60},(_,i)=>({role:'agent',text:`Recorded answer ${i}. `+'Earlier context. '.repeat(15),attachments:[]}));f.state.request.question=formattedQuestion;
 await page.goto(v2Path);await v2Selected(page,0);const discussion=discussionPane(page);
 expect(await page.evaluate(()=>window.scrollY)).toBe(0);
 expect(await page.evaluate(()=>document.activeElement?.closest('.review-v2-discussion')!==null)).toBe(false);
 // A failed explicit send triggers refresh without changing the existing thread.
 f.state.rejected=true;await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Retain position');await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(page.getByRole('alert')).toContainText('could not be accepted');
 expect(await page.evaluate(()=>window.scrollY)).toBeGreaterThan(0);
 await page.evaluate(()=>window.scrollTo(0,0));
 await discussion.getByRole('button',{name:'Show earlier turns (40)'}).click();
 await expect(discussion.getByRole('article').first()).toContainText('Recorded answer 20.');
 await expect(discussion.getByRole('article').first()).toBeInViewport();
 await page.getByRole('button',{name:'Hide discussion',exact:true}).click();
 await page.getByRole('button',{name:'Show discussion',exact:true}).click();
 await expect(discussion.getByRole('article').first()).toContainText('Recorded answer 20.');
 expect(await discussion.evaluate(el=>el.scrollTop)).toBe(0);
});

for(const width of [320,390,980])test(`B12/13 long formatted question, code and tables remain reachable at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900});const f=await reviewV2BrowserFixture(page);f.state.request.question=formattedQuestion+'\n\n'+('Long question paragraph with more review instructions.\n\n'.repeat(30))+'```text\n'+('very_long_value_'.repeat(50))+'\n```\n\n| '+('Wide column '.repeat(20))+' | Result |\n| --- | --- |\n| value | passes |\n\nFINAL QUESTION CONTENT';
 await page.goto(v2Path);await v2Selected(page,0);
 await expect(page.getByRole('separator')).toHaveCount(0);await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Reach the final content');await geometry(page);
 const resultCell=discussionPane(page).getByRole('table').last().getByRole('columnheader',{name:'Result',exact:true});expect(await resultCell.evaluate(el=>el.getBoundingClientRect().width)).toBeGreaterThanOrEqual(96);
 const tail=discussionPane(page).getByText('FINAL QUESTION CONTENT',{exact:true});await tail.scrollIntoViewIfNeeded();await expect(tail).toBeInViewport();const bounds=(await tail.boundingBox())!,response=(await page.locator('.review-v2-response').boundingBox())!;expect(bounds.y+bounds.height).toBeLessThanOrEqual(response.y+1);
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
    await expect(page.getByRole('separator')).toHaveCount(width === 390 ? 0 : 1);

    for (const [surface, root] of [
      ['attachment', page.getByLabel('Rendered document')],
      ['question', discussionPane(page).locator('.review-v2-question-markdown')],
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
