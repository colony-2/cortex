import { test, expect, type Page } from '@playwright/test';
import { readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { png as rasterPNG } from './helpers/raster';
const png=rasterPNG(1,1,[255,255,255]).toString('base64');
const svg=(body:string,attrs='width="240" height="120"')=>`<svg xmlns="http://www.w3.org/2000/svg" ${attrs}>${body}</svg>`;
const html=`<!doctype html><style>body{margin:0} .card{background:rgb(20, 100, 160);color:white;padding:20px;font-size:24px}</style><div class="card">Static page 雪</div><img alt="embedded" style="background:black" width="40" height="40" src="data:image/png;base64,${png}"><svg width="60" height="40"><rect width="60" height="40" fill="orange"/></svg>`;
const plainHTML=html.replace(/<img[^>]+>/, '');
const diagram=svg('<defs><linearGradient id="g"><stop stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs><rect width="240" height="120" fill="url(#g)"/><text x="20" y="60" fill="white">Static diagram</text>');
async function settle(page:Page) { await page.evaluate(async()=>{await Promise.all(document.getAnimations().filter(a=>a.effect?.getComputedTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{})));}); }
type Files=Record<string,string|Buffer>;
async function fixture(page:Page,files:Files,viewer:'review'|'story',width=1280) {
  await page.setViewportSize({width,height:900});
  await page.context().addCookies([{name:'colony2:user:email',value:'reviewer@example.com',url:'http://127.0.0.1:15173'},{name:'preview-secret',value:'parent-cookie-sentinel',url:'http://127.0.0.1:15173'}]);
  await page.addInitScript(()=>{localStorage.setItem('preview-secret','parent-storage-sentinel');(window as any).previewSentinel='untouched';});
  const names=Object.keys(files);
  await page.request.post('http://127.0.0.1:15175/__files',{data:Object.fromEntries(Object.entries(files).map(([name,value])=>[name,Buffer.from(value).toString('base64')]))});
  await page.route('**/api/**',async route=>{
    const url=new URL(route.request().url());
    if(url.pathname.includes('/documents')||url.pathname.includes('/artifacts/')) {
      const name=url.searchParams.get('document_id')||decodeURIComponent(url.pathname.split('/artifacts/')[1]);
      return route.fulfill({body:files[name]??'',contentType:'application/octet-stream'});
    }
    let body:unknown=[];
    if(url.pathname==='/api/projects')body=[{tenant_id:'tenant',name:'Tenant'}];
    else if(url.pathname.endsWith('/user-inputs/job'))body={jobId:'job',status:'pending',form:{kind:'review',request_id:'r',overview_document_id:names[0],title:'Static preview review',fields:[{id:'notes',question:'Notes',type:'paragraph_text',required:true},{id:'decision',question:'Decision',type:'multiple_choice',required:true,options:[{value:'approve',label:'Approve'},{value:'reject',label:'Reject'}]}],documents:Object.fromEntries(names.map((name,i)=>[name,{kind:'stored',name,stored:{key:{jobId:'producer',taskOrdinal:i+1,name}}}]))}};
    else if(url.pathname.endsWith('/story'))body={job_id:'job',status:'success',root:{id:'root',kind:'opStep',title:'Contents',status:'success',path:[],artifact_keys:names.map((name,i)=>({name,taskOrdinal:i+1}))}};
    else if(url.pathname.endsWith('/jobs/job'))body={job_id:'job',title:'Fixture job',status:'SUCCEEDED'};
    else if(url.pathname.includes('/events'))return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
    return route.fulfill({json:body});
  });
  await page.goto(viewer==='review'?'/project/tenant/reviews/job':'/project/tenant/jobs/job/story?nodeId=root');
  if(viewer==='story')await page.getByRole('tab',{name:`Outbox (${names.length})`}).click();
  const open=async(name:string)=>{
    if(viewer==='review') await page.getByRole('button',{name:/^(Show|Hide) documents$/}).waitFor();
    if(viewer==='review' && await page.getByRole('button',{name:'Show documents',exact:true}).isVisible()) await page.getByRole('button',{name:'Show documents',exact:true}).click();
    if(viewer==='story') {
      const dialog=page.getByRole('dialog');if(await dialog.isVisible())await dialog.getByRole('button',{name:'Close',exact:true}).first().click();
      await page.getByRole('link',{name,exact:true}).click();
    } else if(width<768) {
      await page.getByRole('button',{name:`Read ${name}`,exact:true}).click();
    } else await page.getByRole('navigation',{name:'Review documents'}).getByRole('button').filter({hasText:name}).click();
    await settle(page);
  };
  const scope=viewer==='story'?page.getByRole('dialog'):page.getByRole('region',{name:'Selected document'});
  const download=async(name:string)=>{
    const pending=page.waitForEvent('download');
    await scope.getByRole(viewer==='story'?'button':'link',{name:/Download/}).focus();
    await page.keyboard.press('Enter');
    const saved=await pending;
    expect(saved.suggestedFilename()).toBe(name);
    const original=Buffer.from(files[name]);
    const downloaded=await readFile((await saved.path())!);
    expect(downloaded).toEqual(original);
    await writeFile(test.info().outputPath(`${name}-download.json`),JSON.stringify({name,bytes:downloaded.length,sha256:createHash('sha256').update(downloaded).digest('hex'),originalSHA256:createHash('sha256').update(original).digest('hex'),identical:true}));
  };
  return {open,scope,download};
}

for (const viewer of ['review','story'] as const) {
  test(`HSV-005,024,027 embedded raster pixels ${viewer}`,async({page},info)=>{
    const content=html+`<svg width="40" height="40" style="background:black"><image width="40" height="40" href="DATA:IMAGE/PNG;base64,${png}"/></svg>`;
    const {open,scope,download}=await fixture(page,{'embedded.html':content},viewer);
    await open('embedded.html');
    const frame=page.frameLocator('iframe');
    const image=frame.getByAltText('embedded');
    await expect.poll(()=>image.evaluate((e:HTMLImageElement)=>e.complete&&e.naturalWidth)).toBe(1);
    await expect(image).toHaveAttribute('src',`data:image/png;base64,${png}`);
    await expect(frame.locator('svg image')).toHaveAttribute('href',new RegExp('^data:image/png;base64,','i'));
    const observations=[];
    for(const target of [image,frame.locator('svg').last()]) {
      const screenshot=await target.screenshot();
      const rgb=execFileSync('ffmpeg',['-v','error','-threads','1','-i','pipe:0','-vf','scale=1:1','-f','rawvideo','-pix_fmt','rgb24','-threads','1','pipe:1'],{input:screenshot});
      expect([...rgb]).toEqual([255,255,255]);
      observations.push([...rgb]);
    }
    await page.screenshot({path:info.outputPath(`${viewer}-raster-pixels.png`)});
    await writeFile(info.outputPath(`${viewer}-raster-observations.json`),JSON.stringify({browser:page.context().browser()?.version(),pixels:observations}));
    await download('embedded.html');
    await scope.getByRole('radio',{name:'Source',exact:true}).focus();await page.keyboard.press('Space');
    await expect(scope.getByLabel('Document source',{exact:true})).toHaveText(content);
  });
}


test('HSV-010,011 SVG retains a validated embedded raster image',async({page})=>{
  const {open,scope,download}=await fixture(page,{'raster.svg':svg(`<image width="40" height="40" href="data:image/png;base64,${png}"/>`)},'story');
  await open('raster.svg');const image=scope.getByRole('img',{name:'raster.svg'});
  await expect.poll(()=>image.evaluate((e:HTMLImageElement)=>e.complete&&e.naturalWidth)).toBe(240);
  expect(await image.evaluate((el:HTMLImageElement)=>{const c=document.createElement('canvas');c.width=240;c.height=120;const ctx=c.getContext('2d')!;ctx.drawImage(el,0,0);return ctx.getImageData(10,10,1,1).data[3];})).toBe(255);
  await download('raster.svg');
});

test('HSV-017,018 unavailable rendering keeps source and original download',async({page})=>{
  await page.addInitScript(()=>Object.defineProperty(URL,'createObjectURL',{value:undefined,configurable:true}));
  const {open,scope,download}=await fixture(page,{'unavailable.html':plainHTML},'story');
  await open('unavailable.html');
  await expect(scope.getByText(/Could not display|createObjectURL|Interactive HTML requires/)).toBeVisible();
  await expect(scope.getByLabel('Document source',{exact:true})).toHaveText(plainHTML);
  await expect(scope.locator('iframe')).toHaveCount(0);
  await download('unavailable.html');
});

// Exercise the inline phone controls separately from artifact-link navigation.
test('HSV-020,028-030 phone selection scrolling failures and draft continuity',async({page})=>{
  const longHTML=plainHTML+'<p>Long static page</p>'.repeat(200)+'<!--'+'source line\n'.repeat(200)+'-->';
  const files={'page.html':longHTML,'diagram.svg':svg('<rect width="1200" height="1200" fill="green"/>','width="1200" height="1200"'),'invalid.svg':svg('<g>')};
  const {open,scope,download}=await fixture(page,files,'review',390);
  const notes=page.getByRole('textbox',{name:'Notes'});
  const draft='Phone draft survives selection, failure and retry 雪';
  await notes.fill(draft);
  const reachable=async()=>{
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await notes.scrollIntoViewIfNeeded();await notes.focus();await expect(notes).toBeFocused();await expect(notes).toHaveValue(draft);
    for(const name of ['Approve','Reject']) {
      const decision=page.getByRole('button',{name,exact:true});await decision.scrollIntoViewIfNeeded();await decision.focus();await expect(decision).toBeFocused();await expect(decision).toBeEnabled();
      expect(await decision.evaluate(e=>{const r=e.getBoundingClientRect();return r.top>=0&&r.bottom<=innerHeight;})).toBe(true);
    }
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  };
  const select=async(name:string)=>{
    await page.getByRole('combobox',{name:'Review document'}).focus();await page.keyboard.press('ArrowDown');
    await page.getByTitle(`${name} · ${name}`,{exact:true}).click();
  };
  await page.getByRole('button',{name:'Show documents',exact:true}).click();
  const read=page.getByRole('button',{name:'Read page.html',exact:true});await read.focus();await page.keyboard.press('Enter');
  const frame=page.frameLocator('iframe');await expect(frame.locator('.card')).toBeVisible();
  expect(await frame.locator('html').evaluate(e=>{e.scrollTop=300;return e.scrollTop>0;})).toBe(true);
  await scope.getByRole('radio',{name:'Source',exact:true}).focus();await page.keyboard.press('Space');
  await scope.getByLabel('Search document').fill('source line');await expect(scope.getByText('1 / 200')).toBeVisible();
  expect(await scope.locator('.review-source-body').evaluate(e=>{e.scrollTop=200;return e.scrollTop>0&&e.clientHeight<=innerHeight;})).toBe(true);
  await download('page.html');await reachable();
  await select('diagram.svg');const image=scope.getByRole('img',{name:'diagram.svg',exact:true});await expect(image).toBeVisible();
  await scope.getByRole('button',{name:'Actual size',exact:true}).click();await scope.getByRole('button',{name:'Zoom in',exact:true}).click();
  const viewport=scope.getByRole('region',{name:'SVG preview'});await viewport.focus();await page.keyboard.press('ArrowRight');
  await expect.poll(()=>viewport.evaluate(e=>e.scrollLeft)).toBeGreaterThan(0);
  await scope.getByRole('button',{name:'Fit',exact:true}).click();await expect(image).not.toHaveAttribute('width');await download('diagram.svg');await reachable();
  let fail=true;
  await page.route('**/documents?**',route=>{
    const name=new URL(route.request().url()).searchParams.get('document_id')!;
    return fail&&name==='page.html'?route.fulfill({status:503,body:'retry'}):route.fulfill({body:files[name as keyof typeof files]});
  });
  await select('page.html');await expect(scope.getByText(/Could not load this document/)).toBeVisible();await reachable();
  fail=false;await scope.getByRole('button',{name:'Retry',exact:true}).click();await expect(frame.locator('.card')).toBeVisible();await reachable();
  await select('invalid.svg');await expect(scope.getByText(/showing source/i)).toBeVisible();await expect(scope.getByLabel('Document source',{exact:true})).toHaveText(files['invalid.svg']);await download('invalid.svg');await reachable();
});
