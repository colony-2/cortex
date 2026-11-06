import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';

for (const viewport of [{ width: 1280, height: 800 }, { width: 390, height: 844 }]) {
  test(`SAV-026,027 story viewer ${viewport.width}`, async ({ page, context, baseURL }, testInfo) => {
    await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
    await page.setViewportSize(viewport);
    const errors: string[] = [];
    page.on('pageerror', e => errors.push(e.message));
    const original = '# Browser artifact\n\n' + 'long line '.repeat(220000);
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      if (url.pathname.includes('/artifacts/')) return route.continue();
      let body: unknown = [];
      if (url.pathname === '/api/projects') body = [{ tenant_id: 'tenant', name: 'Test tenant' }];
      else if (url.pathname.endsWith('/story')) body = {job_id:'job', status:'success',root:{id:'root',kind:'opStep',title:'Contents',status:'success',path:[],artifact_keys:[{name:'large.md',taskOrdinal:1},{name:'huge.png',taskOrdinal:2},{name:'wide.png',taskOrdinal:3},{name:'wide.patch',taskOrdinal:4}]}};
      else if (url.pathname.endsWith('/jobs/job')) body = {job_id:'job',title:'Fixture job',status:'SUCCEEDED'};
      else if (url.pathname.includes('/events')) return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
      await route.fulfill({json:body});
    });
    await page.goto('/project/tenant/jobs/job/story?nodeId=root');
    await page.getByRole('tab', { name: 'Artifacts (4)' }).click();
    const opener = page.getByRole('link', { name: 'large.md', exact: true });
    await opener.click();
    const modal = page.getByRole('dialog');
    await expect(modal.getByText(/Preview truncated/)).toBeVisible();
    await expect.poll(async () => (await modal.boundingBox())!.width).toBeGreaterThan(Math.min(1000, viewport.width - 32) * .95);
    const download = modal.getByRole('button', { name: 'Download' });
    await expect(download).toBeInViewport();
    const body = modal.locator('.ant-modal-body');
    expect(await body.evaluate(e => e.scrollHeight >= e.clientHeight)).toBe(true);
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})));
    });
    await page.screenshot({path:testInfo.outputPath(`story-${viewport.width}.png`)});
    const pending = page.waitForEvent('download');
    await download.click();
    const saved = await pending;
    expect(saved.suggestedFilename()).toBe('large.md');
    expect(await readFile((await saved.path())!, 'utf8')).toBe(original);
    await page.keyboard.press('Escape');
    await expect(modal).not.toBeVisible();
    await expect(opener).toBeFocused();
    // An unavailable preview must still allow the complete original download.
    await page.route('**/artifacts/large.md', route => route.fulfill({ status: 503, body: 'unavailable' }));
    await opener.click();
    await expect(modal.getByText(/Could not load this document/)).toBeVisible();
    const unavailableDownload = page.waitForEvent('download');
    await modal.getByRole('button', { name: 'Download' }).click();
    expect(await readFile((await (await unavailableDownload).path())!, 'utf8')).toBe(original);
    await page.keyboard.press('Escape');
    await expect(modal).not.toBeVisible();
    await page.getByRole('link', { name: 'huge.png', exact: true }).click();
    await expect(modal.getByText(/32 megapixel preview limit/)).toBeVisible();
    const rejectedDownload = page.waitForEvent('download');
    await modal.getByRole('button', { name: 'Download' }).click();
    const rejected = await rejectedDownload;
    expect(rejected.suggestedFilename()).toBe('huge.png');
    const bytes = await readFile((await rejected.path())!);
    expect(bytes.length).toBe(24);
    expect(bytes.readUInt32BE(16)).toBe(10000);
    await page.keyboard.press('Escape');
    await page.getByRole('link', { name: 'wide.png', exact: true }).click();
    await expect(modal.getByRole('img', {name:'wide.png'})).toBeVisible();
    await modal.getByRole('button', {name:'Actual size'}).click();
    const imageScroll = modal.getByRole('region', {name:'Image preview'});
    expect(await imageScroll.evaluate(e => e.scrollWidth > e.clientWidth)).toBe(true);
    await expect(modal.getByRole('button', {name:'Download'})).toBeInViewport();
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})));
    });
    await page.screenshot({path:testInfo.outputPath(`image-${viewport.width}.png`)});
    await page.keyboard.press('Escape');
    await page.getByRole('link', { name: 'wide.patch', exact: true }).click();
    await modal.getByText('Side by side', {exact:true}).click();
    await expect(modal.getByLabel('Side by side diff')).toBeVisible();
    expect(await modal.locator('.review-diff-scroll').evaluate(e => e.scrollWidth > e.clientWidth)).toBe(true);
    await expect(modal.getByRole('button', {name:'Download'})).toBeInViewport();
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})));
    });
    await page.screenshot({path:testInfo.outputPath(`diff-${viewport.width}.png`)});
    await page.keyboard.press('Escape');
    expect(errors).toEqual([]);
  });
}

for (const viewport of [{width:1280,height:800},{width:390,height:844}]) {
  for (const viewer of ['review','story'] as const) {
    test(`MAL-013-033 local links ${viewer} ${viewport.width}`,async({page,context,baseURL},testInfo)=>{
      await context.addCookies([{name:'colony2:user:email',value:'browser@example.com',url:baseURL!}]);
      await page.setViewportSize(viewport);
      const requested:string[]=[];
      const writes:string[]=[];
      page.on('request',r=>{if(r.url().includes('/documents?')||r.url().includes('/artifacts/'))requested.push(r.url());if(r.method()!=='GET')writes.push(r.url());});
      const source='# Bar\n\n[next](foo.md?ignored=1#target-1) [missing](outside.md) [escape](../../escape.md)';
      const target='# Foo\n\n[self](foo.md#target) [missing anchor](#unknown) [same](#target-1) [json](data.json)\n\n'+Array.from({length:35},(_,i)=>`Paragraph ${i}.\n\n`).join('')+'## Target\n\nFirst target.\n\n## Target\n\nSecond target.';
      const keys=[{name:'docs/bar.md',taskOrdinal:1},{name:'docs/foo.md',taskOrdinal:2},{name:'docs/data.json',taskOrdinal:3}];
      let backgroundChanged=false;
      await page.route('**/api/**',async route=>{
        const url=new URL(route.request().url());
        let body:unknown=[];
        if(url.pathname.includes('/documents')||url.pathname.includes('/artifacts/')) {
          const id=url.searchParams.get('document_id');
          const content=id==='a'||url.pathname.endsWith('docs%2Fbar.md')?source:id==='b/雪'||url.pathname.endsWith('docs%2Ffoo.md')?target:'{"n":42}';
          return route.fulfill({contentType:'text/plain',body:content});
        }
        if(url.pathname==='/api/projects')body=[{tenant_id:'tenant',name:'Tenant'}];
        else if(url.pathname.endsWith('/user-inputs/job'))body={jobId:'job',status:'pending',startTime:'2026-11-06T00:00:00Z',form:{kind:'review',request_id:'request-1',title:'Link review',fields:[{id:'notes',question:'Notes',type:'paragraph_text',required:true}],documents:Object.fromEntries(keys.map((key,i)=>[i===0?'a':i===1?'b/雪':'z',{kind:'stored',name:key.name,stored:{key:{...key,jobId:`producer-${i}`}}}]))}};
        else if(url.pathname.endsWith('/story'))body={job_id:'job',status:'success',root:{id:'root',kind:'opStep',title:'Contents',status:'success',path:[],artifact_keys:backgroundChanged?[...keys,{name:'docs/outside.md',taskOrdinal:9}]:keys}};
        else if(url.pathname.endsWith('/jobs/job'))body={job_id:'job',title:'Fixture job',status:'SUCCEEDED'};
        else if(url.pathname.includes('/events'))return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
        await route.fulfill({json:body});
      });
      await page.goto(viewer==='review'?'/project/tenant/reviews/job':'/project/tenant/jobs/job/story?nodeId=root');
      let opener;
      if(viewer==='review') {
        await page.getByRole('textbox',{name:'Notes'}).fill('Keep this draft');
        if(viewport.width<600) {opener=page.getByRole('button',{name:'Read docs/bar.md',exact:true});await opener.click();}
      } else {
        await page.getByRole('tab',{name:'Artifacts (3)'}).click();
        opener=page.getByRole('link',{name:'docs/bar.md',exact:true});await opener.click();
        backgroundChanged=true;
        // Refresh behind the modal without changing its attachment scope.
        await page.locator('button').filter({hasText:'Refresh'}).evaluate((el:HTMLElement)=>el.click());
        await expect(page.getByRole('tab',{name:'Artifacts (4)',includeHidden:true})).toBeAttached();
      }
      const scope=viewer==='story'||viewport.width<600?page.getByRole('dialog'):page.locator('.review-document-preview');
      const originalURL=page.url();
      const initialRequests=requested.length;
      await scope.getByRole('button',{name:'missing',exact:true}).click();
      await expect(scope.getByRole('status')).toContainText('missing');
      await scope.getByRole('button',{name:'escape',exact:true}).click();
      await expect(scope.getByRole('status')).toContainText('outside-context');
      expect(requested).toHaveLength(initialRequests);
      const next=scope.getByRole('button',{name:'next',exact:true});await next.focus();await page.keyboard.press('Enter');
      await expect(scope.getByRole('heading',{name:'Target',exact:true}).nth(1)).toBeInViewport();
      await expect(page.getByLabel('Document preview',{exact:true})).toBeFocused();
      if(viewer==='review'&&viewport.width>=600)await expect(page.getByRole('combobox',{name:'Review document'}).locator('xpath=ancestor::div[contains(@class,"ant-select")][1]')).toBeAttached();
      await page.screenshot({path:testInfo.outputPath(`${viewer}-${viewport.width}-target.png`)});
      expect(new Set(requested).size).toBe(2);const targetRequests=requested.length;expect(requested.at(-1)).not.toContain('ignored');
      await scope.getByRole('button',{name:'missing anchor',exact:true}).click();await expect(scope.getByRole('status')).toContainText('Anchor unavailable: unknown');
      await scope.getByRole('button',{name:'same',exact:true}).click();await expect(scope.getByRole('heading',{name:'Target',exact:true}).nth(1)).toBeInViewport();
      await scope.getByRole('button',{name:'self',exact:true}).click({modifiers:['Control']});await expect(scope.getByRole('heading',{name:'Target',exact:true}).first()).toBeInViewport();
      expect(requested).toHaveLength(targetRequests);expect(context.pages()).toHaveLength(1);expect(page.url()).toBe(originalURL);
      await scope.getByRole('button',{name:'json',exact:true}).click();await expect(scope.getByLabel('Formatted JSON')).toContainText('42');
      expect(new Set(requested).size).toBe(3);expect(writes).toEqual([]);
      if(viewer==='story')await scope.getByRole('button',{name:'Close',exact:true}).last().click();
      else if(viewport.width<600)await page.getByRole('button',{name:'Back to review',exact:true}).click();
      if(opener)await expect(opener).toBeFocused();
      if(viewer==='review')await expect(page.getByRole('textbox',{name:'Notes'})).toHaveValue('Keep this draft');
      expect(requested.every(url=>url.includes('/projects/tenant/'))).toBe(true);
    });
  }
}
