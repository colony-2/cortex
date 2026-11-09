import { expect, type Page } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { jobIdentity, mockCells } from './helpers/cells';

test.beforeEach(async ({page, context, baseURL}) => {
  await context.addCookies([{name:'colony2:user:email', value:'review@example.com', url:baseURL!}]);
  await page.addInitScript(() => { (window as any).EventSource = class { readyState=1; close(){} addEventListener(){} removeEventListener(){} }; });
  await page.route('**/api/projects', r => r.fulfill({json:[{...jobIdentity, tenant_id:'phone'}]}));
  await mockCells(page,['platform']);
  await page.route('**/user-inputs/pending', r => r.fulfill({json:[]}));
  await page.route('**/jobs/one', r => r.fulfill({json:{...jobIdentity, job_id:'one', status:'ACTIVE', prompt:'Review fixture'}}));
  await page.route('**/jobs/one/children?*', r => r.fulfill({json:{jobs:[]}}));
});
async function fixture(page: Page, count=2, required=false) {
  const raster=await page.evaluate(()=>{ const canvas=document.createElement('canvas');canvas.width=1200;canvas.height=1800;canvas.getContext('2d')!.fillRect(0,0,1200,1800);return canvas.toDataURL('image/png').split(',')[1]; });
  const names=['design.md','source.txt','changes.patch','image.png'];
  const documents=Object.fromEntries(Array.from({length:count},(_,i)=>[`d${i}`,{kind:'stored',name:names[i] || `file${i}.md`,stored:{key:{jobId:'one',taskOrdinal:1,name:names[i] || `file${i}.md`}}}]));
  await page.route('**/user-inputs/one',r=>r.fulfill({json:{jobId:'one',status:'pending',startTime:'2026-10-30T10:00:00Z',form:{kind:'review',title:'Review',request_id:'request',documents,fields:[
    {id:'decision',type:'multiple_choice',question:'Proceed?',required:true,options:[{value:'approve',label:'Approve'},{value:'revise',label:'Revise'}]},
    {id:'feedback',type:'paragraph_text',question:'Reason',required},
    {id:'other',type:'short_answer',question:'Required context',required:true},
  ]}}}));
  await page.route('**/reviews/one/documents?*',r=>{
    const id=new URL(r.request().url()).searchParams.get('document_id');
    if(id==='d1') return r.fulfill({contentType:'text/plain',body:('wide'.repeat(150)+'\n').repeat(170)+'SOURCE END'});
    if(id==='d2') return r.fulfill({contentType:'text/plain',body:'--- a/file\n+++ b/file\n@@ -1,151 +1,151 @@\n'+ Array.from({length:150},()=>'-old\n+new').join('\n')+'\n DIFF END'});
    // A passive SVG is deliberately not used for the raster viewer.
    if(id==='d3') return r.fulfill({contentType:'image/png',body:Buffer.from(raster,'base64')});
    return r.fulfill({contentType:'text/markdown',body: count>5 ? '# Short document\n\nEND' : '# Long document\n\n'+('Review paragraph.\n\n'.repeat(180))+'FINAL DOCUMENT CONTENT'});
  });
  await page.goto('/project/phone/reviews/one');
  await expect(page.getByLabel('Document preview')).toBeVisible();
}
async function geometry(page: Page) {
  await expect.poll(() => page.evaluate(() => {
    const bar=document.querySelector('.review-actions')!;
    const bounds=bar.getBoundingClientRect();
    // Viewport resize events and the ResizeObserver arrive asynchronously.
    // Old reserved space can exceed the new bar height before either runs.
    return bounds.top >= 0 && bounds.bottom <= innerHeight
      && bounds.height <= (window.visualViewport?.height ?? innerHeight) * 0.75 + 1
      && parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--review-action-height')) >= bounds.height - 1;
  })).toBe(true);
  const result=await page.evaluate(()=>{
    const bar=document.querySelector('.review-actions')!;
    const b=bar.getBoundingClientRect();
    return {horizontal:document.documentElement.scrollWidth-innerWidth, barBottom:b.bottom,barTop:b.top,height:b.height,reserved:parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--review-action-height')),
      panes:[...document.querySelectorAll('.review-document-list,.review-document-content,.review-markdown,.review-markdown-source,.review-source-body,.review-diff-scroll,.review-image-scroll')].map(e=>({name:e.className,delta:e.scrollHeight-e.clientHeight})),
      locked:getComputedStyle(document.body).overflowY==='hidden'};
  });
  expect(result.horizontal).toBeLessThanOrEqual(1);
  expect(result.locked).toBe(false);
  expect(result.barBottom).toBeLessThanOrEqual(await page.evaluate(()=>innerHeight));
  expect(result.barTop).toBeGreaterThanOrEqual(0);
  expect(result.reserved).toBeGreaterThanOrEqual(result.height-1);
  for(const p of result.panes) expect(p.delta,p.name).toBeLessThanOrEqual(1);
  await test.info().attach('geometry',{body:JSON.stringify(result,null,2),contentType:'application/json'});
}
async function aboveBar(page: Page, label: string) {
  const field=page.getByLabel(label,{exact:true});
  await field.scrollIntoViewIfNeeded();
  expect(await field.evaluate(e=>e.getBoundingClientRect().bottom <= document.querySelector('.review-actions')!.getBoundingClientRect().top)).toBe(true);
}
for(const count of [2,45]) test(`REV-001 taller ${count===2?'document':'catalog'} controls page height`,async({page})=>{
  await page.setViewportSize({width:1440,height:900}); await fixture(page,count);
  const heights=await page.evaluate(()=>['.review-document-list','.review-document-content'].map(s=>document.querySelector(s)!.getBoundingClientRect().height));
  // Both grid columns stretch to the natural height of the taller content.
  expect(Math.abs(heights[0]-heights[1])).toBeLessThanOrEqual(1);
  await geometry(page);
  if(count>5){
    await page.getByRole('navigation',{name:'Review documents'}).getByRole('button').last().click();
    await expect(page.getByRole('heading',{name:'Short document',exact:true})).toBeInViewport({ratio:1});
  }
  await page.getByRole('button',{name:'Approve',exact:true}).click();
  await page.getByRole('button',{name:'OK',exact:true}).click();
  await expect(page.getByText('Required context is required')).toBeVisible();
  await aboveBar(page,'Required context');
});
for(const width of [320,390,1440]) test(`REV-002/004/005/008 inline previews and measured bar at ${width}`,async({page})=>{
  await page.setViewportSize({width,height:844});await fixture(page,4,true);
  await page.getByRole('button',{name:'Approve',exact:true}).click();
  await page.getByRole('button',{name:'OK',exact:true}).click();
  await expect(page.getByText('Reason is required')).toBeVisible();
  await expect(page.getByLabel('Reason',{exact:true})).toBeInViewport({ratio:1});
  await page.getByRole('button',{name:'Back',exact:true}).click();
  await page.getByRole('button',{name:'Reject',exact:true}).click();
  await expect(page.getByLabel('Reason',{exact:true})).toBeFocused();
  await geometry(page);
  for(const [index,region] of [[0,'Rendered document'],[1,'Document source'],[2,'Unified diff'],[3,'image.png']] as const){
    if(width<768) await page.getByRole('button',{name:`Read ${['design.md','source.txt','changes.patch','image.png'][index]}`,exact:true}).click();
    else await page.getByRole('navigation',{name:'Review documents'}).getByRole('button').nth(index).click();
    if(index===3) {await expect(page.getByRole('img',{name:region,exact:true})).toBeVisible();await page.getByRole('button',{name:'Actual size',exact:true}).click();}
    else await expect(page.getByLabel(region,{exact:true})).toBeVisible();
    await geometry(page);
    if(index===1){
      expect(await page.getByLabel('Document source',{exact:true}).evaluate(e=>e.textContent!.length)).toBeLessThanOrEqual(24000);
      expect(await page.locator('.review-source-body').evaluate(e=>e.scrollWidth>e.clientWidth)).toBe(true);
      await page.getByLabel('Search document').fill('SOURCE END');
      await page.locator('mark').scrollIntoViewIfNeeded();
      await expect(page.locator('mark')).toBeInViewport({ratio:1});
    }
    if(index===0){await page.getByText('FINAL DOCUMENT CONTENT',{exact:true}).scrollIntoViewIfNeeded();await expect(page.getByText('FINAL DOCUMENT CONTENT',{exact:true})).toBeInViewport({ratio:1});await page.getByText('Markdown',{exact:true}).click();await expect(page.getByLabel('Markdown source')).toContainText('FINAL DOCUMENT CONTENT');await geometry(page);}
  }
  await page.setViewportSize({width,height:400});
  await geometry(page);
  await page.getByLabel('Reason',{exact:true}).fill('Reason remains reachable');
  await expect(page.getByLabel('Reason',{exact:true})).toBeInViewport({ratio:1});
  await page.getByRole('button',{name:'OK'}).scrollIntoViewIfNeeded();
  await expect(page.getByRole('button',{name:'OK'})).toBeInViewport({ratio:1});
});
test.describe('zoom',()=>{
  test.use({nativeZoom:true});
  test('REV-005 native 200 percent zoom keeps final content and editor reachable',async({page})=>{
    await fixture(page);await setNativeZoom(page,2);
    await page.getByRole('button',{name:'Reject',exact:true}).click();
    await page.getByLabel('Reason',{exact:true}).fill('Zoomed feedback');
    await geometry(page);
    await page.getByRole('button',{name:'OK'}).scrollIntoViewIfNeeded();
  await expect(page.getByRole('button',{name:'OK'})).toBeInViewport({ratio:1});
    await page.evaluate(()=>window.scrollTo(0,document.documentElement.scrollHeight));
    await aboveBar(page,'Required context');
    await captureNativeZoom(page,test.info().outputPath('review-200.png'));
  });
});

test('REV-004/007/011 optional reason, wrapped errors and network retry retain measured clearance',async({page})=>{
  await page.setViewportSize({width:320,height:568});await fixture(page);
  await page.getByRole('button',{name:'Reject',exact:true}).click();
  await page.getByLabel('Reason',{exact:true}).fill('   ');
  await page.getByRole('button',{name:'OK'}).click();
  await expect(page.getByText('Reason is required')).toBeVisible();
  await geometry(page);
  await page.getByLabel('Reason',{exact:true}).fill('Please revise');
  await page.getByLabel('Required context',{exact:true}).fill('Context');
  await page.route('**/reviews/one/respond',r=>r.fulfill({status:503,json:{error:'Temporarily unavailable; retry this review after checking your connection.'}}));
  await page.getByRole('button',{name:'OK'}).click();
  await expect(page.locator('.review-actions')).toContainText('Temporarily unavailable');
  await expect(page.getByLabel('Reason',{exact:true})).toHaveValue('Please revise');
  await geometry(page);
  await page.getByRole('button',{name:'OK'}).scrollIntoViewIfNeeded();
  await expect(page.getByRole('button',{name:'OK'})).toBeInViewport({ratio:1});
  await page.screenshot({path:test.info().outputPath('review-error-320.png')});
});

test('RC-001/017/018 confirmation captures and visual viewport clearance',async({page},testInfo)=>{
  await page.setViewportSize({width:1440,height:900}); await fixture(page,2);
  let requests=0;
  await page.route('**/reviews/one/respond',route=>{requests++;return route.fulfill({json:{ok:true}});});
  await page.screenshot({path:testInfo.outputPath('review-collapsed-desktop.png')});
  const collapsed=await page.locator('.review-actions').evaluate(e=>e.getBoundingClientRect().height);
  await page.getByRole('button',{name:'Approve',exact:true}).click();
  expect(requests).toBe(0);
  await expect(page.getByLabel('Reason',{exact:true})).toBeFocused();
  await page.getByLabel('Reason',{exact:true}).fill('The proposal is ready to proceed.');
  await geometry(page);
  await page.screenshot({path:testInfo.outputPath('review-confirm-desktop.png')});
  await page.getByRole('button',{name:'Back',exact:true}).click();
  await expect(page.getByRole('button',{name:'Approve',exact:true})).toBeFocused();
  await expect.poll(()=>page.locator('.review-actions').evaluate(e=>e.getBoundingClientRect().height)).toBe(collapsed);
  await page.setViewportSize({width:320,height:568});
  await page.getByRole('button',{name:'Reject',exact:true}).click();
  await expect(page.getByLabel('Reason',{exact:true})).toHaveValue('The proposal is ready to proceed.');
  await geometry(page);
  await page.screenshot({path:testInfo.outputPath('review-confirm-320.png')});
  // Simulate a keyboard's visual viewport geometry, retaining the layout viewport.
  await page.evaluate(()=>{
    Object.defineProperty(window.visualViewport,'height',{configurable:true,value:280});
    window.visualViewport!.dispatchEvent(new Event('resize'));
  });
  await expect.poll(()=>page.locator('.review-actions').evaluate(e=>e.getBoundingClientRect().bottom)).toBe(280);
  await page.getByRole('button',{name:'Back',exact:true}).scrollIntoViewIfNeeded();
  await expect(page.getByRole('button',{name:'Back',exact:true})).toBeInViewport({ratio:1});
  await page.getByRole('button',{name:'OK',exact:true}).scrollIntoViewIfNeeded();
  await expect(page.getByRole('button',{name:'OK',exact:true})).toBeInViewport({ratio:1});
  const positions=await page.getByRole('button',{name:'OK',exact:true}).boundingBox();
  expect(positions!.y+positions!.height).toBeLessThanOrEqual(280);
  await geometry(page);
  await page.screenshot({path:testInfo.outputPath('review-keyboard-geometry-320.png')});
  expect(requests).toBe(0);
  await page.evaluate(()=>{
    delete (window.visualViewport as any).height;
    window.visualViewport!.dispatchEvent(new Event('resize'));
  });
  await page.getByRole('button',{name:/Back to Reviews/}).click();
  await expect.poll(()=>page.evaluate(()=>document.documentElement.style.getPropertyValue('--review-action-height'))).toBe('');
  await page.evaluate(()=>window.visualViewport!.dispatchEvent(new Event('resize')));
  await expect.poll(()=>page.evaluate(()=>document.documentElement.style.getPropertyValue('--review-action-height'))).toBe('');
  await page.goto('/project/phone/reviews/one');
  await expect(page.getByRole('button',{name:'Approve',exact:true})).toBeVisible();
  await geometry(page);
});
