import { expect, type Page } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { authorizeV2, reviewV2BrowserFixture, v2Path, v2Ids, v2Selected, v2Catalog } from './helpers/reviewV2';

test.beforeEach(async({page,baseURL})=>authorizeV2(page,baseURL!));
async function geometry(page:Page){
 await expect.poll(()=>page.evaluate(()=>{
  const pane=document.querySelector('.review-v2-response')!;const box=pane.getBoundingClientRect();const viewport=window.visualViewport;
  const bottom=viewport?viewport.height+viewport.offsetTop:innerHeight;
  return {overflow:document.documentElement.scrollWidth>innerWidth+1,fixed:getComputedStyle(pane).position==='fixed',bounded:box.height<=.75*(viewport?.height??innerHeight)+1,bottom:Math.abs(box.bottom-bottom)<2,reserved:parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--review-action-height'))>=box.height-1};
 })).toEqual({overflow:false,fixed:true,bounded:true,bottom:true,reserved:true});
}
async function screenshot(page:Page,name:string){await page.screenshot({path:test.info().outputPath(name),fullPage:false});}

for(const width of [1440,390])test(`N06–10 v2 history retains text, files, composer and settings at ${width}px`,async({page,browser,baseURL})=>{
 await page.setViewportSize({width,height:900});const fixture=await reviewV2BrowserFixture(page);
 await page.goto('/project/v2/reviews');await page.goto(v2Path+'?filter=one&filter=two#read');await v2Selected(page,0);
 const logical=new URL(page.url()).searchParams.get('review_id');expect(logical).toBe(fixture.state.request.review_id);
 const initialDetails=fixture.state.details;
 await expect(page.getByText('platform',{exact:true})).toBeVisible();
 await expect(page.getByRole('navigation',{name:'Job ancestors'}).getByRole('link')).toHaveText(['Build a useful job workspace','Improve the review reading and response flow','Review viewer usability']);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 const editor=page.getByLabel('Question or requested change (required)',{exact:true});await expect(editor).toBeFocused();
 await editor.fill('Keep these notes while I compare documents.');
 await page.getByLabel('Attach notes',{exact:true}).setInputFiles({name:'notes.md',mimeType:'text/markdown',buffer:Buffer.from('annotations')});
 await page.locator('#review-v2-files').evaluate(el=>el.setAttribute('data-retained','yes'));
 await page.getByRole('button',{name:'Review settings'}).click();
 await page.getByRole('dialog').getByLabel('Example count').fill('0');await page.getByRole('button',{name:'Done',exact:true}).click();
 await v2Catalog(page,1);await v2Catalog(page,2);
 await page.locator('.review-source-body').evaluate(el=>{el.scrollLeft=140;el.setAttribute('data-scroll-retained','yes');});
 const sourceScroll=await page.locator('.review-source-body').evaluate(el=>el.scrollLeft);
 await page.getByRole('button',{name:'Hide documents',exact:true}).click();await ensureDocuments(page);
 await page.getByRole('button',{name:'Hide discussion',exact:true}).click();await page.getByRole('button',{name:'Show discussion',exact:true}).click();
 await expect(page.locator('.review-source-body')).toHaveAttribute('data-scroll-retained','yes');expect(await page.locator('.review-source-body').evaluate(el=>el.scrollLeft)).toBe(sourceScroll);
 for(const [direction,index] of [['back',1],['back',0],['forward',1],['forward',2]] as const){
  if(direction==='back')await page.goBack();else await page.goForward();await v2Selected(page,index);
  await expect(editor).toHaveValue('Keep these notes while I compare documents.');await expect(page.locator('#review-v2-files')).toHaveAttribute('data-retained','yes');
  await expect(page.getByRole('button',{name:'Remove notes.md (1)'})).toBeVisible();await expect(page.locator('.review-v2-response')).toContainText('Retained locally; applies to Request changes');
 }
 expect(fixture.state.details).toBe(initialDetails);expect(fixture.state.submits).toHaveLength(0);expect(new URL(page.url()).searchParams.getAll('filter')).toEqual(['one','two']);expect(new URL(page.url()).hash).toBe('#read');
 await page.getByRole('button',{name:'Review settings'}).click();await expect(page.getByRole('dialog').getByLabel('Example count')).toHaveValue('0');
 await page.goBack();await expect(page.getByRole('dialog')).toBeVisible();await page.getByRole('button',{name:'Done',exact:true}).click();await v2Selected(page,1);
 const shared=page.url();await page.reload();await v2Selected(page,1);
 const other=await browser.newContext({baseURL});try{const recipient=await other.newPage();await authorizeV2(recipient,baseURL!);await reviewV2BrowserFixture(recipient);await recipient.goto(shared);await v2Selected(recipient,1);}finally{await other.close();}
});
test('N06/N08 catalog, source and Markdown links share history without same-document entries',async({page})=>{
 await page.setViewportSize({width:1440,height:900});await reviewV2BrowserFixture(page);await page.goto('/project/v2/reviews');await page.goto(v2Path);await v2Selected(page,0);
 await page.locator('.review-selected-details').getByRole('link',{name:/Acceptance checks/}).click();await v2Selected(page,1);
 await page.getByRole('link',{name:'Same heading',exact:true}).click();await page.goBack();await v2Selected(page,0);
 await page.getByRole('link',{name:'Open checks',exact:true}).click();await v2Selected(page,1);await page.getByRole('link',{name:'Read source notes',exact:true}).click();await v2Selected(page,2);
 await v2Catalog(page,2);await page.goBack();await v2Selected(page,1);await page.goBack();await v2Selected(page,0);await page.goBack();await expect(page).toHaveURL(/\/project\/v2\/reviews$/);
});
test('N10/N11 missing originals replace selection without losing drafts; Retry and late reads do not change history',async({page})=>{
 const f=await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Retained after removal');
 f.state.failures.set(v2Ids[1],503);
 if(await page.getByRole('button',{name:'Show documents'}).isVisible())await page.getByRole('button',{name:'Show documents'}).click();
 await page.getByRole('navigation',{name:'Review documents'}).getByRole('button').nth(1).click();
 await expect(page.getByLabel('Selected document').getByRole('alert')).toContainText('Could not load this document');
 f.state.failures.delete(v2Ids[1]);await page.getByLabel('Selected document').getByRole('button',{name:'Retry',exact:true}).click();await v2Selected(page,1);
 let release!:()=>void;f.state.holds.set(v2Ids[2],new Promise(r=>{release=r;}));
 // Start the request without waiting for its preview.
 if(await page.getByRole('button',{name:'Show documents'}).isVisible())await page.getByRole('button',{name:'Show documents'}).click();
 await page.getByRole('navigation',{name:'Review documents'}).getByRole('button').nth(2).click();await expect.poll(()=>f.state.reads.includes(v2Ids[2])).toBe(true);
 await page.goBack();await v2Selected(page,1);release();await expect.poll(()=>f.state.complete.includes(v2Ids[2])).toBe(true);await v2Selected(page,1);
 delete f.state.request.documents[v2Ids[1]];delete f.state.request.document_metadata![v2Ids[1]];f.state.request.document_metadata![v2Ids[0]].source_document_ids=[];
 await page.getByRole('button',{name:'Review settings'}).click();await page.getByRole('button',{name:'Done',exact:true}).click();
 // A failed explicit submission triggers a fresh observation of the changed catalog.
 f.state.rejected=true;await page.getByRole('button',{name:'Send · Ask a question'}).click();
 await expect(page.getByText(/linked document is unavailable/)).toBeVisible();await v2Selected(page,0);
 await expect(page.getByLabel('Question or requested change (required)')).toHaveValue('Retained after removal');
 await page.goBack();await v2Selected(page,0);
});
test('N04/R05 duplicate links fail closed and another retained review cannot answer the live request',async({page})=>{
 const f=await reviewV2BrowserFixture(page);await page.goto(v2Path+'?review_id='+encodeURIComponent(f.state.request.review_id)+'&review_id=other&document_id='+encodeURIComponent(v2Ids[0]));
 await expect(page.getByText('This review link is malformed')).toBeVisible();await expect(page.getByRole('button',{name:'Approve direction',exact:true})).toHaveCount(0);expect(f.state.reads).toHaveLength(0);
 await page.getByRole('button',{name:'Open selected review'}).click();await v2Selected(page,0);
 f.state.liveReviewId='another-review';await page.reload();await v2Selected(page,0);await expect(page.locator('.review-v2-response')).toContainText('Another request is now pending');await expect(page.getByRole('button',{name:'Approve direction',exact:true})).toHaveCount(0);expect(f.state.submits).toHaveLength(0);
});
for(const width of [1680,1560,320,390])test(`L01–05 wide navigation, inline previews and fixed composer at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900});await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);await geometry(page);
 if(width===1680){await expect(page.getByRole('navigation',{name:'Review documents'})).toBeVisible();expect(await page.locator('.review-reading-track').evaluate(el=>el.clientWidth)).toBeGreaterThanOrEqual(720);await screenshot(page,'v2-wide-reading.png');await page.getByRole('navigation',{name:'Review documents'}).getByRole('button').first().focus();await page.setViewportSize({width:1560,height:900});await expect(page.getByRole('navigation',{name:'Review documents'})).toBeHidden();await expect(page.getByRole('button',{name:'Show documents',exact:true})).toBeFocused();await page.setViewportSize({width:1680,height:900});await expect(page.getByRole('navigation',{name:'Review documents'})).toBeVisible();}
 else{await expect(page.getByRole('navigation',{name:'Review documents'})).toBeHidden();if(width===1560){expect(await page.locator('.review-reading-track').evaluate(el=>el.clientWidth)).toBeGreaterThanOrEqual(720);await screenshot(page,'v2-near-fit-reading.png');}}
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Can you confirm the response stays in view?');
 await page.getByLabel('Attach notes').setInputFiles({name:'my-review.md',mimeType:'text/markdown',buffer:Buffer.from('notes')});await geometry(page);
 await page.locator('.review-selected-details').scrollIntoViewIfNeeded();
 const tail=await page.locator('.review-selected-details').boundingBox();const bar=await page.locator('.review-v2-response').boundingBox();expect(tail!.y+tail!.height).toBeLessThanOrEqual(bar!.y);
 for(const index of [2,3,4]){await v2Catalog(page,index);await geometry(page);}
 await v2Catalog(page,1);await geometry(page);
 await page.locator('.review-document-content').scrollIntoViewIfNeeded();
 if(width===1680||width===390||width===320)await screenshot(page,`v2-composer-${width}.png`);
 // Internal pane scrolling keeps the send button and removals reachable at small sizes.
 await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 await expect.poll(()=>page.getByRole('button',{name:'Send · Ask a question'}).evaluate(element=>{const box=element.getBoundingClientRect();return element.contains(document.elementFromPoint(box.x+box.width/2,box.y+box.height/2));})).toBe(true);
 await page.getByRole('button',{name:'Back',exact:true}).click();await geometry(page);
 await page.getByRole('button',{name:'Hide documents',exact:true}).click();await page.setViewportSize({width:1600,height:900});await expect(page.getByRole('navigation',{name:'Review documents'})).toBeHidden();
 await page.getByRole('button',{name:'Show documents',exact:true}).click();await page.setViewportSize({width:390,height:900});await expect(page.getByRole('navigation',{name:'Review documents'})).toBeVisible();
});
test('P01/P05/N09 accepted question shows pending in discussion; successor retains document and uses new native request',async({page})=>{
 await page.setViewportSize({width:1440,height:900});const f=await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);await v2Catalog(page,1);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Does Back preserve a file draft?');
 await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(page.getByRole('region',{name:'Discussion'}).getByText('Response pending',{exact:true})).toBeVisible();await geometry(page);await screenshot(page,'v2-response-pending.png');
 expect(f.state.submits[0].request_id).toBe('native-one');expect(f.state.submits[0].response.direction).toBe('ask');
 await f.successor();await page.getByRole('button',{name:'Refresh',exact:true}).click();
 await expect(page.getByText('Response pending',{exact:true})).toHaveCount(0);await v2Selected(page,1);
 await expect(page.getByText('The documents and drafts stay available. I can proceed after your approval.')).toBeVisible();
 await page.getByRole('button',{name:'Approve direction',exact:true}).click();await expect.poll(()=>f.state.submits.length).toBe(2);expect(f.state.submits[1]).toMatchObject({request_id:'native-two',response:{direction:'go',feedback:'',attachments:[],settings:{}}});
 await expect(page.getByText('Response pending',{exact:true})).toHaveCount(0);
});
test('L07–09 simulated visual keyboard viewport bounds pane and unmount clears measurements',async({page})=>{
 await page.setViewportSize({width:390,height:844});const fixture=await reviewV2BrowserFixture(page);fixture.state.request.question=formattedQuestion;await page.goto(v2Path);await v2Selected(page,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(page.getByRole('alert')).toContainText('requires feedback');
 await page.evaluate(()=>{Object.defineProperty(window.visualViewport!,'height',{configurable:true,value:420});Object.defineProperty(window.visualViewport!,'offsetTop',{configurable:true,value:0});window.visualViewport!.dispatchEvent(new Event('resize'));window.dispatchEvent(new Event('resize'));});
 // Exercise the browser viewport event target with a simulated keyboard height.
 await geometry(page);await page.getByRole('button',{name:'Send · Ask a question'}).scrollIntoViewIfNeeded();await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 await page.evaluate(()=>{Object.defineProperty(window.visualViewport!,'offsetTop',{configurable:true,value:30});window.visualViewport!.dispatchEvent(new Event('scroll'));});await geometry(page);
 await discussionPane(page).getByRole('article').last().scrollIntoViewIfNeeded();await expect(discussionPane(page).getByRole('heading',{name:'Ready to proceed?'})).toBeVisible();await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 await page.getByRole('button',{name:'Back to Reviews'}).click();
 await expect.poll(()=>page.evaluate(()=>document.documentElement.style.getPropertyValue('--review-action-height'))).toBe('');
 await page.evaluate(()=>window.dispatchEvent(new Event('resize')));expect(await page.evaluate(()=>document.documentElement.style.getPropertyValue('--review-action-height'))).toBe('');
});
test('L04 final discussion turn remains reachable above an expanded response pane',async({page})=>{
 await page.setViewportSize({width:1680,height:900});const f=await reviewV2BrowserFixture(page);
 f.state.request.conversation=Array.from({length:30},(_,index)=>({role:'agent',text:`Earlier answer ${index}. `+'Review discussion context. '.repeat(8),attachments:[]}));
 await page.goto(v2Path);await v2Selected(page,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByLabel('Question or requested change (required)').fill('Keep the last answer reachable.');
 await page.getByLabel('Attach notes').setInputFiles({name:'notes.md',mimeType:'text/markdown',buffer:Buffer.from('notes')});
 await geometry(page);
 const discussion=page.getByRole('region',{name:'Discussion'});
 await discussion.evaluate(element=>{element.scrollTop=element.scrollHeight;});
 const last=discussion.getByRole('article').last();
 await expect(last).toContainText(f.state.request.question);
 // Reach the last turn through both the discussion and the surrounding page.
 // The page head initially places the discussion below its sticky top inset.
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
  await expect(page.getByRole('navigation',{name:'Job ancestors'})).toBeVisible();await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Zoomed feedback');await geometry(page);
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
const docsSeparator = (page:Page) => page.getByRole('separator',{name:'Resize documents'});
const discussionSeparator = (page:Page) => page.getByRole('separator',{name:'Resize discussion'});
async function widthOf(page:Page,selector:string){return page.locator(selector).evaluate(el=>el.getBoundingClientRect().width);}
async function ensureDocuments(page:Page){if(await page.getByRole('button',{name:'Show documents',exact:true}).isVisible())await page.getByRole('button',{name:'Show documents',exact:true}).click();}
async function dragPane(page:Page,side:'documents'|'discussion',delta:number){
 const separator=side==='documents'?docsSeparator(page):discussionSeparator(page);
 const box=await separator.boundingBox();
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
 await page.getByRole('button',{name:'Approve direction',exact:true}).click();await expect(page.locator('.review-v2-response')).toContainText('Response sent');
 await page.reload();await v2Selected(page,0);await expect(last.getByRole('heading',{name:'Successor question'})).toHaveCount(1);
});

test('B02–05/B09 adjacent tracks resize independently and retain previews, response drafts, files and settings',async({page})=>{
 await page.setViewportSize({width:1920,height:1100});const f=await reviewV2BrowserFixture(page);f.state.request.question=formattedQuestion;await page.goto(v2Path);await v2Selected(page,0);await ensureDocuments(page);
 const originalURL=page.url();const reads=f.state.reads.length;
 await page.getByRole('article',{name:'Rendered document'}).evaluate(el=>el.setAttribute('data-preview-identity','kept'));
 await expect.poll(()=>page.evaluate(()=>{
  const boxes=['.review-navigation-track','.review-v2-separator','.review-reading-track','.review-v2-separator + .review-v2-discussion'].map(selector=>document.querySelector(selector)?.getBoundingClientRect());
  const tracks=Array.from(document.querySelectorAll('.review-v2-separator')).map(el=>el.getBoundingClientRect());
  return [Math.abs(boxes[0]!.right-tracks[0].left),Math.abs(tracks[0].right-boxes[2]!.left),Math.abs(boxes[2]!.right-tracks[1].left),Math.abs(tracks[1].right-boxes[3]!.left)];
 })).toEqual([0,0,0,0]);
 await expect(docsSeparator(page)).toHaveAttribute('aria-orientation','vertical');await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow','320');
 const docBefore=await widthOf(page,'.review-navigation-track'),discussionBefore=await widthOf(page,'.review-v2-discussion'),readerBefore=await widthOf(page,'.review-reading-track');
 await dragPane(page,'documents',80);expect(await widthOf(page,'.review-navigation-track')).toBeCloseTo(docBefore+80,0);expect(await widthOf(page,'.review-v2-discussion')).toBeCloseTo(discussionBefore,0);expect(await widthOf(page,'.review-reading-track')).toBeCloseTo(readerBefore-80,0);
 await dragPane(page,'discussion',-96);expect(await widthOf(page,'.review-v2-discussion')).toBeCloseTo(discussionBefore+96,0);expect(await widthOf(page,'.review-navigation-track')).toBeCloseTo(docBefore+80,0);
 const docWidth=await widthOf(page,'.review-navigation-track'),discussionWidth=await widthOf(page,'.review-v2-discussion');
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();const editor=page.getByLabel('Question or requested change (required)');await editor.fill('Keep my pane comparison.');
 await page.getByLabel('Attach notes').setInputFiles({name:'comparison.md',mimeType:'text/markdown',buffer:Buffer.from('draft')});
 await page.getByRole('button',{name:'Review settings'}).click();await page.getByRole('dialog').getByLabel('Example count').fill('3');await page.getByRole('button',{name:'Done',exact:true}).click();
 await page.getByRole('button',{name:'Hide documents',exact:true}).click();await expect(docsSeparator(page)).toHaveCount(0);await expect(page.getByRole('navigation',{name:'Review documents'})).toHaveCount(0);
 await expect(page.getByRole('button',{name:'Show documents',exact:true})).toHaveAttribute('aria-expanded','false');
 await page.getByRole('button',{name:'Hide discussion',exact:true}).click();await expect(page.getByRole('separator')).toHaveCount(0);await expect(discussionPane(page)).toHaveCount(0);
 expect(await widthOf(page,'.review-reading-track')).toBeCloseTo(await widthOf(page,'.review-v2-layout'),0);expect(await widthOf(page,'.review-reading-track')).toBeGreaterThan(900);
 await screenshot(page,'v2-reader-only.png');
 for(const side of ['documents','discussion']){
  const button=page.getByRole('button',{name:`Show ${side}`,exact:true});await expect(button).toBeVisible();await button.hover();await expect(page.getByRole('tooltip',{name:`Show ${side}`,exact:true})).toBeVisible();
  const id=await button.getAttribute('aria-controls');expect(id).toBeTruthy();expect(await page.evaluate(id=>!!document.getElementById(id!),id)).toBe(true);
  await button.click();await expect(page.getByRole('button',{name:`Hide ${side}`,exact:true})).toHaveAttribute('aria-expanded','true');
 }
 expect(await widthOf(page,'.review-navigation-track')).toBeCloseTo(docWidth,0);expect(await widthOf(page,'.review-v2-discussion')).toBeCloseTo(discussionWidth,0);
 await expect(page.getByRole('article',{name:'Rendered document'})).toHaveAttribute('data-preview-identity','kept');expect(f.state.reads.length).toBe(reads);expect(page.url()).toBe(originalURL);expect(f.state.submits).toHaveLength(0);
 await expect(editor).toHaveValue('Keep my pane comparison.');await expect(page.getByRole('button',{name:'Remove comparison.md (1)'})).toBeVisible();
 await page.getByRole('button',{name:'Review settings'}).click();await expect(page.getByRole('dialog').getByLabel('Example count')).toHaveValue('3');await page.getByRole('button',{name:'Done',exact:true}).click();
 await page.getByRole('button',{name:'Back',exact:true}).click();
 await page.mouse.move(400,110);await page.evaluate(()=>window.scrollTo(0,0));await screenshot(page,'v2-formatted-resized-panes.png');
});

test('B06–08 keyboard, focus recovery, clamp limits and pointer cancellation',async({page})=>{
 await page.setViewportSize({width:1680,height:1000});const f=await reviewV2BrowserFixture(page);f.state.request.question='[Public reference](https://example.com)';await page.goto(v2Path);await v2Selected(page,0);await ensureDocuments(page);
 for(const [side,separator,minimum] of [['documents',docsSeparator(page),180],['discussion',discussionSeparator(page),240]] as const){
  await separator.focus();const initial=Number(await separator.getAttribute('aria-valuenow'));await page.keyboard.press('ArrowRight');await expect(separator).toHaveAttribute('aria-valuenow',String(initial+(side==='documents'?16:-16)));await page.keyboard.press('ArrowLeft');await expect(separator).toHaveAttribute('aria-valuenow',String(initial));
  await page.keyboard.press('Home');await expect(separator).toHaveAttribute('aria-valuenow',String(minimum));await page.keyboard.press('ArrowLeft');if(side==='documents')await expect(separator).toHaveAttribute('aria-valuenow',String(minimum));
  await page.keyboard.press('End');expect(Number(await separator.getAttribute('aria-valuenow'))).toBe(Number(await separator.getAttribute('aria-valuemax')));expect(await widthOf(page,'.review-reading-track')).toBeGreaterThanOrEqual(359);
  await separator.press('Home');
 }
 await page.evaluate(()=>window.scrollTo(0,0));
 await dragPane(page,'documents',-500);await expect(docsSeparator(page)).toHaveAttribute('aria-valuenow','180');
 await dragPane(page,'discussion',500);await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow','240');
 await dragPane(page,'documents',2000);expect(Number(await docsSeparator(page).getAttribute('aria-valuenow'))).toBe(Number(await docsSeparator(page).getAttribute('aria-valuemax')));await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow','240');expect(await widthOf(page,'.review-reading-track')).toBeGreaterThanOrEqual(359);await docsSeparator(page).press('Home');
 await page.evaluate(()=>window.scrollTo(0,0));await dragPane(page,'discussion',-2000);expect(Number(await discussionSeparator(page).getAttribute('aria-valuenow'))).toBe(Number(await discussionSeparator(page).getAttribute('aria-valuemax')));await expect(docsSeparator(page)).toHaveAttribute('aria-valuenow','180');expect(await widthOf(page,'.review-reading-track')).toBeGreaterThanOrEqual(359);await discussionSeparator(page).press('Home');
 // Programmatic activation leaves focus in the pane, exercising explicit focus recovery.
 await discussionPane(page).getByRole('link',{name:'Public reference'}).focus();await expect(discussionPane(page).getByRole('link',{name:'Public reference'})).toBeFocused();
 await page.getByRole('button',{name:'Hide discussion',exact:true}).evaluate((el:HTMLButtonElement)=>el.click());await expect(page.getByRole('button',{name:'Show discussion',exact:true})).toBeFocused();
 await expect(page.getByRole('link',{name:'Public reference'})).toHaveCount(0);await page.keyboard.press('Tab');expect(await page.evaluate(()=>document.activeElement?.closest('.review-v2-discussion')!==null)).toBe(false);
 await page.getByRole('button',{name:'Show discussion',exact:true}).click();
 await page.getByRole('navigation',{name:'Review documents'}).getByRole('button').first().focus();await page.getByRole('button',{name:'Hide documents',exact:true}).evaluate((el:HTMLButtonElement)=>el.click());await expect(page.getByRole('button',{name:'Show documents',exact:true})).toBeFocused();await ensureDocuments(page);
 await page.evaluate(()=>window.scrollTo(0,0));const box=(await docsSeparator(page).boundingBox())!;
 await page.mouse.move(box.x+4,box.y+30);await page.mouse.down();await page.mouse.move(box.x+70,box.y+30);await expect(docsSeparator(page)).toHaveClass(/dragging/);
 await docsSeparator(page).dispatchEvent('pointercancel',{pointerId:1});const stopped=await widthOf(page,'.review-navigation-track');await page.mouse.move(box.x+130,box.y+30);await page.mouse.up();expect(await widthOf(page,'.review-navigation-track')).toBe(stopped);await expect(page.locator('.review-v2-layout .dragging')).toHaveCount(0);
 // Removing an active separator also releases the gesture and resize cursor.
 const next=(await docsSeparator(page).boundingBox())!;await page.mouse.move(next.x+4,next.y+30);await page.mouse.down();await page.getByRole('button',{name:'Hide documents',exact:true}).evaluate((el:HTMLButtonElement)=>el.click());await page.mouse.up();await expect(page.locator('.review-v2-layout .dragging')).toHaveCount(0);
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
 await page.setViewportSize({width:1920,height:1000});const f=await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);await ensureDocuments(page);
 await dragPane(page,'documents',120);await dragPane(page,'discussion',-150);
 const saved={documents:await widthOf(page,'.review-navigation-track'),discussion:await widthOf(page,'.review-v2-discussion')};
 // An explicit choice survives every width and same-review successor.
 await page.getByRole('button',{name:'Hide documents',exact:true}).click();await ensureDocuments(page);
 await page.setViewportSize({width:1150,height:1000});
 await expect.poll(() => widthOf(page,'.review-v2-discussion')).toBeLessThan(saved.discussion);
 await expect(page.locator('.review-v2-layout')).not.toHaveClass(/stacked/);await geometry(page);
 const clampedDiscussion=await widthOf(page,'.review-v2-discussion'),clampedDocuments=await widthOf(page,'.review-navigation-track');
 await dragPane(page,'documents',-32);expect(await widthOf(page,'.review-v2-discussion')).toBe(clampedDiscussion);saved.documents=clampedDocuments-32;expect(await widthOf(page,'.review-navigation-track')).toBe(saved.documents);
 await page.setViewportSize({width:1050,height:1000});await expect(page.locator('.review-v2-layout')).toHaveClass(/stacked/);await expect(page.getByRole('separator')).toHaveCount(0);await expect(page.getByRole('navigation',{name:'Review documents'})).toBeVisible();await geometry(page);
 await page.setViewportSize({width:1920,height:1000});await expect(docsSeparator(page)).toHaveAttribute('aria-valuenow',String(saved.documents));await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow',String(saved.discussion));
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByLabel('Question or requested change (required)').fill('Continue');
 await page.getByRole('button',{name:'Send · Ask a question'}).click();
 await expect(discussionPane(page).getByRole('status')).toHaveText('Response pending');
 await f.successor();
 await expect(discussionPane(page).getByText('The documents and drafts stay available. I can proceed after your approval.',{exact:true})).toBeVisible();
 await expect(docsSeparator(page)).toHaveAttribute('aria-valuenow',String(saved.documents));
 await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow',String(saved.discussion));
 await page.getByRole('button',{name:'Hide discussion',exact:true}).click();await page.reload();await v2Selected(page,0);await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow','320');await expect(docsSeparator(page)).toHaveAttribute('aria-valuenow','240');
 await page.getByRole('button',{name:'Hide discussion',exact:true}).click();const target=page.url();await page.getByRole('button',{name:'Back to Reviews'}).click();await page.goto(target);await v2Selected(page,0);await expect(discussionSeparator(page)).toHaveAttribute('aria-valuenow','320');
});

test('B11 initial final question reveal does not steal focus and older reading positions survive refresh, expansion and reopening',async({page})=>{
 await page.setViewportSize({width:1680,height:1000});const f=await reviewV2BrowserFixture(page);f.state.request.conversation=Array.from({length:60},(_,i)=>({role:'agent',text:`Recorded answer ${i}. `+'Earlier context. '.repeat(15),attachments:[]}));f.state.request.question=formattedQuestion;
 await page.goto(v2Path);await v2Selected(page,0);const discussion=discussionPane(page);
 await expect.poll(()=>discussion.evaluate(el=>Math.abs(el.scrollHeight-el.clientHeight-el.scrollTop))).toBeLessThan(2);
 expect(await page.evaluate(()=>document.activeElement?.closest('.review-v2-discussion')!==null)).toBe(false);
 await discussion.evaluate(el=>{el.scrollTop=200;});const position=await discussion.evaluate(el=>el.scrollTop);
 // A failed explicit send triggers refresh without changing the existing thread.
 f.state.rejected=true;await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Retain position');await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(page.getByRole('alert')).toContainText('could not be accepted');expect(await discussion.evaluate(el=>el.scrollTop)).toBe(position);
 const firstOld=discussion.getByRole('article').filter({hasText:'Recorded answer 40.'});const anchorOffset=await firstOld.evaluate(el=>el.getBoundingClientRect().top-el.parentElement!.getBoundingClientRect().top);
 await discussion.getByRole('button',{name:'Show earlier turns (40)'}).evaluate((el:HTMLButtonElement)=>el.click());
 await expect(firstOld).toHaveCount(1);expect(await firstOld.evaluate(el=>el.getBoundingClientRect().top-el.parentElement!.getBoundingClientRect().top)).toBeCloseTo(anchorOffset,0);
 await discussion.evaluate(el=>{el.scrollTop=250;});await page.getByRole('button',{name:'Hide discussion',exact:true}).click();await page.getByRole('button',{name:'Show discussion',exact:true}).click();expect(await discussion.evaluate(el=>el.scrollTop)).toBe(250);
});

for(const width of [320,390,1050])test(`B12/13 long formatted question, code and tables remain reachable at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:900});const f=await reviewV2BrowserFixture(page);f.state.request.question=formattedQuestion+'\n\n'+('Long question paragraph with more review instructions.\n\n'.repeat(30))+'```text\n'+('very_long_value_'.repeat(50))+'\n```\n\n| '+('Wide column '.repeat(20))+' | Result |\n| --- | --- |\n| value | passes |\n\nFINAL QUESTION CONTENT';
 await page.goto(v2Path);await v2Selected(page,0);await ensureDocuments(page);
 await expect(page.getByRole('separator')).toHaveCount(0);await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Reach the final content');await geometry(page);
 const resultCell=discussionPane(page).getByRole('table').last().getByRole('columnheader',{name:'Result',exact:true});expect(await resultCell.evaluate(el=>el.getBoundingClientRect().width)).toBeGreaterThanOrEqual(96);
 const tail=discussionPane(page).getByText('FINAL QUESTION CONTENT',{exact:true});await tail.scrollIntoViewIfNeeded();await expect(tail).toBeInViewport();const bounds=(await tail.boundingBox())!,response=(await page.locator('.review-v2-response').boundingBox())!;expect(bounds.y+bounds.height).toBeLessThanOrEqual(response.y+1);
 await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();await geometry(page);await screenshot(page,`v2-long-question-${width}.png`);
});
