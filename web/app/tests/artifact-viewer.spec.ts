import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import { jobIdentity } from './helpers/cells';

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
    await page.getByRole('tab', { name: 'Outbox (4)' }).click();
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
        await expect(page.getByRole('navigation',{name:'Review documents'})).toBeHidden();
        await page.getByRole('button',{name:'Show documents',exact:true}).click();
        if(viewport.width<600) {opener=page.getByRole('button',{name:'Read docs/bar.md',exact:true});await opener.click();}
      } else {
        await page.getByRole('tab',{name:'Outbox (3)'}).click();
        opener=page.getByRole('link',{name:'docs/bar.md',exact:true});await opener.click();
        backgroundChanged=true;
        // Refresh behind the modal without changing its attachment scope.
        await page.getByRole('button',{name:'reload Refresh',exact:true}).evaluate((el:HTMLElement)=>el.click());
        await expect(page.getByRole('tab',{name:'Outbox (4)',includeHidden:true})).toBeAttached();
      }
      const scope=viewer==='story'?page.getByRole('dialog'):page.locator('.review-document-preview');
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
      if(viewer==='review'&&viewport.width>=600)await expect(page.getByRole('navigation',{name:'Review documents'})).toBeAttached();
      await page.screenshot({path:testInfo.outputPath(`${viewer}-${viewport.width}-target.png`)});
      expect(new Set(requested).size).toBe(2);const targetRequests=requested.length;expect(requested.at(-1)).not.toContain('ignored');
      await scope.getByRole('button',{name:'missing anchor',exact:true}).click();await expect(scope.getByRole('status')).toContainText('Anchor unavailable: unknown');
      await scope.getByRole('button',{name:'same',exact:true}).click();await expect(scope.getByRole('heading',{name:'Target',exact:true}).nth(1)).toBeInViewport();
      await scope.getByRole('button',{name:'self',exact:true}).click({modifiers:['Control']});await expect(scope.getByRole('heading',{name:'Target',exact:true}).first()).toBeInViewport();
      expect(requested).toHaveLength(targetRequests);expect(context.pages()).toHaveLength(1);expect(page.url()).toBe(originalURL);
      await scope.getByRole('button',{name:'json',exact:true}).click();await expect(scope.getByLabel('Formatted JSON')).toContainText('42');
      expect(new Set(requested).size).toBe(3);expect(writes).toEqual([]);
      if(viewer==='story')await scope.getByRole('button',{name:'Close',exact:true}).last().click();
      if(viewer==='story'&&opener)await expect(opener).toBeFocused();
      if(viewer==='review') {
        await expect(page.getByRole('dialog')).toHaveCount(0);
        await expect(page.getByLabel('Document preview',{exact:true})).toBeFocused();
      }
      if(viewer==='review')await expect(page.getByRole('textbox',{name:'Notes'})).toHaveValue('Keep this draft');
      expect(requested.every(url=>url.includes('/projects/tenant/'))).toBe(true);
    });
  }
}

for (const width of [375, 767, 768, 1280]) {
  test(`RDM metadata cards and list ${width}`, async ({page,context,baseURL}, testInfo) => {
    await context.addCookies([{name:'colony2:user:email',value:'reviewer@example.com',url:baseURL!}]);
    await page.setViewportSize({width,height:844});
    const reads:string[]=[];
    const longName='long-filename-'.repeat(12)+'.md';
    const documents=Object.fromEntries(['z','a','雪/target',...Array.from({length:24},(_,i)=>`other-${i}`)].map((id,i)=>[id,{kind:'stored',name:id==='z'?longName:`${id}.md`,stored:{key:{jobId:'child',taskOrdinal:i+1,name:`${id}.md`,sizeBytes:100}}}]));
    const metadata={z:{title:'Overview',description:'Description '.repeat(20),role:'primary',order:10,source_document_ids:['雪/target','missing']},a:{title:'Supporting',role:'supporting',order:0},'雪/target':{title:'Diagnostic',role:'diagnostic',order:2,source_document_ids:['z']}};
    await page.route('**/api/**',async route=>{
      const url=new URL(route.request().url());let body:unknown=[];
      if(url.pathname.includes('/documents')) {reads.push(url.toString());return route.fulfill({contentType:'text/plain',body:url.searchParams.get('document_id')==='z'?'# Large overview\n\n'+'Review paragraph.\n\n'.repeat(350)+'Last paragraph.':'# Target contents'});}
      if(url.pathname==='/api/projects')body=[{tenant_id:'tenant',name:'Tenant'}];
      else if(url.pathname.endsWith('/user-inputs/job'))body={jobId:'job',status:'pending',form:{kind:'review',request_id:'r',overview_document_id:'z',document_metadata:metadata,documents,fields:[{id:'notes',type:'paragraph_text',question:'Notes',required:true}]}};
      else if(url.pathname.includes('/events'))return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
      await route.fulfill({json:body});
    });
    await page.goto('/project/tenant/reviews/job');
    const notes=page.getByRole('textbox',{name:'Notes'});await notes.fill('Keep this draft');
    const nav=page.getByRole('navigation',{name:'Review documents'});
    const compact=width<768;
    await expect(nav).toBeHidden();
    await page.getByRole('button',{name:'Show documents',exact:true}).click();
    await expect(nav).toContainText('Overview');
    await expect(nav).toContainText(longName);
    await expect(nav).toContainText('Description');
    await expect(nav).toContainText('Primary');
    await expect(nav).toContainText('Supporting');
    await expect(nav).toContainText('Diagnostic');
    expect(await nav.locator('.review-document-entry strong').allTextContents()).toEqual(['Supporting','Diagnostic','Overview',...Array.from({length:24},(_,i)=>`other-${i}`).sort().map(id=>`${id}.md`)]);
    if(compact) {
      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect(nav.getByText('Starting document')).toBeVisible();
      await expect(page.getByRole('heading',{name:'Large overview'})).toBeVisible();
      expect(reads.length).toBeGreaterThan(0);
      expect(reads.every(url=>new URL(url).searchParams.get('document_id')==='z')).toBe(true);
      const initialReads=reads.length;
      const read=page.getByRole('button',{name:'Read Overview',exact:true});await read.focus();await page.keyboard.press('Enter');
      const selected=page.getByRole('region',{name:'Selected document'});
      await expect(selected.getByRole('heading',{name:'Large overview'})).toBeInViewport();
      await expect(selected.getByLabel('Rendered document')).toContainText('Last paragraph.');
      expect(await selected.getByLabel('Rendered document').evaluate(el=>el.scrollHeight<=el.clientHeight+1)).toBe(true);
      expect(await page.evaluate(()=>getComputedStyle(document.body).overflowY)).not.toBe('hidden');
      await expect(selected.getByRole('link',{name:'Download',exact:true})).toHaveAttribute('download',longName);
      await read.click();
      await expect(selected.getByRole('heading',{name:'Large overview'})).toBeInViewport();
      expect(reads).toHaveLength(initialReads);
    } else {
      await expect(page.getByRole('heading',{name:'Large overview'})).toBeVisible();
      expect(reads.length).toBeGreaterThan(0);
      expect(reads.every(url=>new URL(url).searchParams.get('document_id')==='z')).toBe(true);
      await expect(nav.locator('[aria-current=true]')).toContainText('Overview');
      expect(await nav.locator('.review-document-entry').first().textContent()).toContain('Supporting');
      await expect(nav.locator('button').last()).toBeAttached();await nav.locator('button').last().scrollIntoViewIfNeeded();
    }
    const scope=page.getByRole('region',{name:'Selected document'});
    const source=scope.getByRole('link',{name:'Diagnostic · 雪/target.md (雪/target)',exact:true});
    await expect(source).toHaveAttribute('href',/document_id=%E9%9B%AA%2Ftarget/);
    await expect(scope.getByText('missing (unavailable)')).toBeVisible();
    // Observe the application's event policy, then suppress the browser default in the
    // harness so a background tab/download cannot steal focus from the remaining checks.
    expect(await source.evaluate(el => {
      let prevented: boolean | undefined;
      document.addEventListener('click', event => { prevented = event.defaultPrevented; event.preventDefault(); }, {once:true});
      el.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true,ctrlKey:true}));
      return prevented;
    })).toBe(false);
    await expect(scope.getByRole('heading',{name:'Large overview'})).toBeVisible();
    await source.focus();await page.keyboard.press('Enter');
    await expect(scope.getByRole('heading',{name:'Target contents'})).toBeVisible();
    await expect(scope.getByLabel('Document preview')).toBeFocused();
    expect(reads.every(url=>url.includes('request_id=r')&&!url.includes('child'))).toBe(true);
    await expect(notes).toHaveValue('Keep this draft');
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
    expect(await page.locator('button a, a button').count()).toBe(0);
    await page.screenshot({path:testInfo.outputPath(`metadata-${width}.png`)});
    if(compact) {
      const cardSource=nav.getByRole('link',{name:'Diagnostic · 雪/target.md (雪/target)',exact:true});
      const initialReads=reads.length;
      await cardSource.focus();await page.keyboard.press('Enter');
      await expect(scope.getByRole('heading',{name:'Target contents'})).toBeInViewport();
      await expect(scope.getByLabel('Document preview')).toBeFocused();
      await expect(page.getByRole('dialog')).toHaveCount(0);
      expect(reads).toHaveLength(initialReads);
      await expect(notes).toHaveValue('Keep this draft');
    } else {
      await page.setViewportSize({width:375,height:844});await expect(page.getByRole('dialog')).toHaveCount(0);
      await page.setViewportSize({width,height:844});await expect(nav.locator('[aria-current=true]')).toContainText('Diagnostic');
    }
  });
}

for (const width of [1280, 390]) {
  test(`IA-016,017 queued input artifacts ${width}`, async ({ page, context, baseURL }, testInfo) => {
    await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
    await page.setViewportSize({ width, height: 844 });
    const capture = async (name: string, fullPage = false) => {
      // Capture the settled modal, rather than an Ant Design opening frame.
      await page.evaluate(async () => {
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
        await Promise.all(document.getAnimations()
          .filter(animation => animation.playState === 'running'
            && (animation.effect as KeyframeEffect)?.target instanceof Element
            && ((animation.effect as KeyframeEffect).target as Element).closest('.ant-modal, .ant-modal-mask')
            && animation.effect?.getComputedTiming().iterations !== Infinity)
          .map(animation => animation.finished.catch(() => {})));
      });
      const dialog = page.getByRole('dialog');
      if (await dialog.count()) {
        await expect(dialog).not.toHaveClass(/ant-zoom-(appear|enter)/);
        await expect(dialog).toHaveCSS('opacity', '1');
        await expect(dialog).toHaveCSS('transform', 'none');
      }
      await page.screenshot({ path: testInfo.outputPath(name), fullPage });
    };
    const longName = 'docs/' + 'long-path-segment/'.repeat(8) + 'brief.md';
    const files = [longName, 'docs/next.md', 'wide.png', 'input.bin', 'empty'];
    let inventoryRequests = 0;
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      if (url.pathname.includes('/artifacts/')) {
        const name = decodeURIComponent(url.pathname.split('/artifacts/')[1]);
        if (name === 'wide.png' || name === 'input.bin' || name === 'empty') return route.continue();
        return route.fulfill({ body: name === longName ? '# Submission brief\n\n[next](../../../../../../../../next.md)\n\n![input image](wide.png)' : '# Next input' });
      }
      let body: unknown = [];
      if (url.pathname === '/api/projects') body = [{ tenant_id: 'tenant', name: 'Test tenant' }];
      else if (url.pathname.endsWith('/story')) return route.fulfill({ status: 500, body: 'replay unavailable' });
      else if (url.pathname.endsWith('/input-artifacts')) {
        inventoryRequests++;
        body = { artifacts: files.map(name => ({ jobId: 'job', taskOrdinal: 0, name, sizeBytes: name === 'empty' ? 0 : 120 })) };
      }
      else if (url.pathname.endsWith('/jobs/job')) body = { ...jobIdentity, job_id: 'job', title: 'Queued job', status: 'READY' };
      else if (url.pathname.endsWith('/children')) body = { jobs: [] };
      else if (url.pathname.includes('/events')) return route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' });
      await route.fulfill({ json: body });
    });
    await page.goto('/project/tenant/jobs/job/story');
    await page.getByRole('tab', { name: 'Input artifacts', exact: true }).click();
    await expect(page.getByText('Input artifacts (5)', { exact: true })).toBeVisible();
    // The dev app uses StrictMode, which may repeat the initial effect.
    const loadedInventoryRequests = inventoryRequests;
    await capture(`input-artifacts-${width}.png`, true);
    const runSummary = page.locator('.story-summary-disclosure');
    const inputPane = page.locator('.input-artifacts-disclosure');
    await expect(inputPane.getByRole('heading', { name: 'Input artifacts (5)', exact: true })).toBeVisible();
    await expect(inputPane.locator(':scope > summary')).toHaveCount(0);
    await expect(inputPane).not.toHaveAttribute('open');
    await expect(page.getByRole('button', { name: longName, exact: true })).toBeVisible();
    for (let visit = 0; visit < 2; visit++) {
      await page.getByRole('tab', { name: 'Trace', exact: true }).click();
      await expect(runSummary.getByRole('heading', { name: 'Run Summary', exact: true })).toBeVisible();
      await expect(runSummary.locator(':scope > summary')).toHaveCount(0);
      await expect(runSummary).not.toHaveAttribute('open');
      await expect(page.getByRole('tab', { name: 'Trace', exact: true })).toHaveAttribute('aria-selected', 'true');
      await expect(page.getByRole('alert').filter({ hasText: 'Failed to load job story' })).toBeVisible();
      await expect(page.getByRole('button', { name: longName, exact: true })).not.toBeVisible();
      await page.getByRole('tab', { name: 'Input artifacts', exact: true }).click();
      await expect(page.getByRole('button', { name: longName, exact: true })).toBeVisible();
    }
    await capture(`input-panes-direct-${width}.png`, true);
    // Keyboard selection of the tabs exposes content without a second activation.
    await page.getByRole('tab', { name: 'Input artifacts', exact: true }).focus();
    await page.keyboard.press('End');
    await expect(page.getByRole('tab', { name: 'Trace', exact: true })).toBeFocused();
    await expect(runSummary.getByRole('heading', { name: 'Run Summary', exact: true })).toBeVisible();
    await page.keyboard.press('ArrowLeft');
    await page.keyboard.press('ArrowLeft');
    await page.keyboard.press('ArrowLeft');
    await expect(page.getByRole('tab', { name: 'Input artifacts', exact: true })).toBeFocused();
    await expect(page.getByRole('button', { name: longName, exact: true })).toBeVisible();
    expect(inventoryRequests).toBe(loadedInventoryRequests);

    const opener = page.getByRole('button', { name: longName, exact: true });
    await opener.click();
    const modal = page.getByRole('dialog');
    await expect(modal.getByRole('heading', { name: 'Submission brief' })).toBeVisible();
    await expect(modal.getByRole('img', { name: 'wide.png' })).toBeVisible();
    await capture(`input-markdown-${width}.png`);
    await modal.getByRole('button', { name: 'next', exact: true }).click();
    await expect(modal.getByRole('heading', { name: 'Next input' })).toBeVisible();
    await modal.getByRole('button', { name: 'Close', exact: true }).first().click();
    await expect(modal).not.toBeVisible();
    await expect(opener).toBeFocused();
    await page.getByRole('button', { name: 'wide.png', exact: true }).click();
    await expect(modal.getByRole('img', { name: 'wide.png' })).toBeVisible();
    await capture(`input-image-${width}.png`);
    await modal.getByRole('button', { name: 'Close', exact: true }).first().click();
    await expect(modal).not.toBeVisible();
    for (const [name, bytes] of [['input.bin', Buffer.from([0, 255, 1])], ['empty', Buffer.alloc(0)]] as const) {
      const button = page.getByRole('button', { name: `Download ${name}`, exact: true });
      await expect(button).toBeInViewport();
      const pending = page.waitForEvent('download');
      await button.click();
      const download = await pending;
      expect(await readFile((await download.path())!)).toEqual(bytes);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });
}

for (const width of [1280,390]) {
  test(`DOC-08,10,12,13,15 mixed input documents ${width}`,async({page,context,baseURL},testInfo)=>{
    await context.addCookies([{name:'colony2:user:email',value:'browser@example.com',url:baseURL!}]);
    await page.setViewportSize({width,height:844});
    const externalRequests:string[]=[];
    page.on('request',request=>{if(request.url().includes('invalid.example'))externalRequests.push(request.url());});
    const rows=[
      {id:'attachment:0:direct.md',kind:'stored',name:'direct.md',jobId:'job',taskOrdinal:0,sizeBytes:8,sources:['Attachment','Document'],binding:'direct.md'},
      {id:'document:alias',kind:'stored',name:'docs/alias.md',binding:'source alias ?雪',contentBinding:'source alias ?雪',sizeBytes:18,sources:['Document']},
      {id:'document:inline',kind:'value',name:'docs/inline.md',binding:'inline',contentBinding:'inline',sizeBytes:18,sources:['Document']},
      {id:'document:missing',kind:'stored',name:'missing.md',binding:'missing',contentBinding:'missing',sizeBytes:18,sources:['Document']},
      {id:'document:external',kind:'external',name:'external.md',binding:'external',sizeBytes:-1,sources:['Document'],reason:'External reference — content not loaded'},
      {id:'document:bad',kind:'invalid',name:'bad.md',binding:'bad',sizeBytes:-1,sources:['Document'],reason:'Invalid document: stored ref missing payload'},
    ];
    await page.route('**/api/**',async route=>{
      const url=new URL(route.request().url());let body:unknown=[];
      if(url.pathname.endsWith('/input-document')) {
        const binding=url.searchParams.get('binding');
        return route.fulfill({status:binding==='missing'?404:200,contentType:'text/plain',headers:{'Content-Disposition':`attachment; filename="${binding==='inline'?'inline.md':'alias.md'}"`},body:binding==='missing'?'document missing unavailable':binding==='inline'?'# Inline bytes':'# Stored alias\n\n[next](inline.md)'});
      }
      if(url.pathname.includes('/artifacts/'))return route.fulfill({contentType:'text/plain',body:'# Direct bytes'});
      if(url.pathname==='/api/projects')body=[{tenant_id:'tenant',name:'Test tenant'}];
      else if(url.pathname.endsWith('/input-artifacts'))body={artifacts:rows};
      else if(url.pathname.endsWith('/story'))body={job_id:'job',status:'success',root:null};
      else if(url.pathname.endsWith('/jobs/job'))body={...jobIdentity,job_id:'job',title:'Mixed document fixture',status:'SUCCEEDED'};
      else if(url.pathname.includes('/events'))return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
      await route.fulfill({json:body});
    });
    await page.goto('/project/tenant/jobs/job/story?tab=input-artifacts');
    await expect(page.getByRole('heading',{name:'Input artifacts (6)'})).toBeVisible();
    await expect(page.getByText('External reference — content not loaded')).toBeVisible();
    await expect(page.getByText('Attachment · Document')).toBeVisible();
    expect(await page.getByRole('button',{name:'Download external.md'}).count()).toBe(0);
    await page.screenshot({animations:'disabled',path:testInfo.outputPath(`documents-${width}.png`),fullPage:true});
    await page.getByRole('button',{name:'docs/alias.md',exact:true}).click();
    const modal=page.getByRole('dialog');
    await expect(modal.getByRole('heading',{name:'Stored alias'})).toBeVisible();
    const storedDownload=page.waitForEvent('download');await modal.getByRole('button',{name:'Download'}).click();
    expect(await readFile((await (await storedDownload).path())!,'utf8')).toBe('# Stored alias\n\n[next](inline.md)');
    await modal.getByRole('button',{name:'next',exact:true}).click();
    await expect(modal.getByRole('heading',{name:'Inline bytes'})).toBeVisible();
    await expect(modal.getByRole('button',{name:'Download'})).toBeInViewport();
    await page.screenshot({animations:'disabled',path:testInfo.outputPath(`inline-viewer-${width}.png`)});
    const pending=page.waitForEvent('download');await modal.getByRole('button',{name:'Download'}).click();
    expect(await readFile((await (await pending).path())!,'utf8')).toBe('# Inline bytes');
    await modal.getByRole('button',{name:'Close',exact:true}).last().click();
    await expect(modal).not.toBeVisible();
    await page.getByRole('button',{name:'missing.md',exact:true}).click();
    await expect(modal.getByText(/Could not load this document/)).toBeInViewport();
    await expect(modal.getByRole('button',{name:'Close',exact:true}).last()).toBeInViewport();
    await page.evaluate(async () => { await Promise.all(document.getAnimations().filter(a => a.effect?.getComputedTiming().iterations !== Infinity).map(a => a.finished.catch(() => {}))); });
    await page.screenshot({animations:'disabled',path:testInfo.outputPath(`unavailable-${width}.png`)});
    await modal.getByRole('button',{name:'Close',exact:true}).last().click();
    await expect(modal).not.toBeVisible();
    await page.getByRole('button',{name:'direct.md',exact:true}).click();
    await expect(modal.getByRole('heading',{name:'Direct bytes'})).toBeVisible();
    expect(externalRequests).toEqual([]);
  });
}
