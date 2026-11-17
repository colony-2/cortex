import { expect, type Page } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { authorizeV2, reviewV2BrowserFixture, v2Path, v2Ids, v2Selected, v2Catalog } from './helpers/reviewV2';

test.beforeEach(async({page,baseURL})=>authorizeV2(page,baseURL!));
async function geometry(page:Page){
 await expect.poll(()=>page.evaluate(()=>{
  const pane=document.querySelector('.review-v2-response')!;const box=pane.getBoundingClientRect();const viewport=window.visualViewport;
  const bottom=viewport?viewport.height+viewport.offsetTop:innerHeight;
  const launcher=document.querySelector('.assistant-launcher')!;const l=launcher.getBoundingClientRect();
  const assistantClear=Array.from(document.querySelectorAll('.app-header button,.review-v2-response button')).every(control=>{const c=control.getBoundingClientRect();return !c.width||!c.height||l.left>=c.right||l.right<=c.left||l.top>=c.bottom||l.bottom<=c.top;});
  return {assistantClear,overflow:document.documentElement.scrollWidth>innerWidth+1,fixed:getComputedStyle(pane).position==='fixed',bounded:box.height<=.75*(viewport?.height??innerHeight)+1,bottom:Math.abs(box.bottom-bottom)<2,reserved:parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--review-action-height'))>=box.height-1};
 })).toEqual({assistantClear:true,overflow:false,fixed:true,bounded:true,bottom:true,reserved:true});
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
 if(width===1680){await expect(page.getByRole('navigation',{name:'Review documents'})).toBeVisible();expect(await page.locator('.review-reading-track').evaluate(el=>el.clientWidth)).toBeGreaterThanOrEqual(720);await screenshot(page,'v2-wide-reading.png');await page.setViewportSize({width:1560,height:900});await expect(page.getByRole('navigation',{name:'Review documents'})).toBeHidden();await page.setViewportSize({width:1680,height:900});await expect(page.getByRole('navigation',{name:'Review documents'})).toBeVisible();}
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
 await page.setViewportSize({width:390,height:844});await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByRole('button',{name:'Send · Ask a question'}).click();await expect(page.getByRole('alert')).toContainText('requires feedback');
 await page.evaluate(()=>{Object.defineProperty(window.visualViewport!,'height',{configurable:true,value:420});Object.defineProperty(window.visualViewport!,'offsetTop',{configurable:true,value:0});window.visualViewport!.dispatchEvent(new Event('resize'));window.dispatchEvent(new Event('resize'));});
 // Exercise the browser viewport event target with a simulated keyboard height.
 await geometry(page);await page.getByRole('button',{name:'Send · Ask a question'}).scrollIntoViewIfNeeded();await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeInViewport();
 await page.evaluate(()=>{Object.defineProperty(window.visualViewport!,'offsetTop',{configurable:true,value:30});window.visualViewport!.dispatchEvent(new Event('scroll'));});await geometry(page);
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
 await expect(last).toContainText('Earlier answer 29.');
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
  await reviewV2BrowserFixture(page);await page.goto(v2Path);await v2Selected(page,0);await setNativeZoom(page,2);await geometry(page);
  await expect(page.getByRole('navigation',{name:'Job ancestors'})).toBeVisible();await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)').fill('Zoomed feedback');await geometry(page);
  const send=page.getByRole('button',{name:'Send · Ask a question'});
  await send.click({trial:true});await expect(send).toBeInViewport({ratio:1});
  await expect.poll(()=>send.evaluate(element=>{
   const button=element.getBoundingClientRect();const pane=element.closest('.review-v2-response')!.getBoundingClientRect();
   return button.top>=pane.top&&button.bottom<=Math.min(innerHeight,pane.bottom);
  })).toBe(true);
  await captureNativeZoom(page,test.info().outputPath('v2-browser-zoom-200.png'));
 });
});
