import {test,expect} from '@playwright/test';
import {fixture} from './helpers/htmlPreview';
import {png} from './helpers/raster';
const gallery=`<!doctype html><html lang="en" data-envelope="root"><head data-envelope="head"><meta charset="utf-8"><title>Authored gallery</title><style>
:root{--accent:rgb(35,110,190)}body{margin:0;padding:18px;font-family:sans-serif;background:#f3f6fc;color:#192640}.card{background:var(--accent);padding:16px;color:white;border-radius:10px}button{padding:12px;background:white;color:#192640;border:1px solid #192640;cursor:pointer}button:hover{background:rgb(255,220,110)}.shots{display:flex;gap:12px;flex-wrap:wrap}img{max-width:100%}@media(max-width:600px){.card{border-radius:0}h1{font-size:20px}}
</style><script>window.order=['first']</script></head><body data-envelope="body"><h1 class="card">Authored screenshot gallery</h1><p>Interactive fixture, local raster attachments</p><div class="shots"><picture><source srcset="blue.png 1x, red.png 2x"><img id="shot" alt="Screenshot" src="blue.png" width="240" height="160"></picture><img alt="Repeated" src="blue.png" width="120" height="80"></div><button onclick="document.querySelector('source').srcset=document.querySelector('[alt=Repeated]').src;document.getElementById('count').textContent=++window.count">Next screenshot</button> <output id="count">0</output><svg width="40" height="30"><rect width="40" height="30" fill="orange"/></svg><script>order.push('second');window.count=0;document.body.dataset.order=order.join(',');setInterval(()=>document.body.dataset.tick=String(Date.now()),20)</script></body></html>`;
for(const viewer of ['review','story'] as const) for(const width of [1280,390]) {
 test(`HP-004,005,007,012,016,019,020,022 gallery ${viewer} ${width}`,async({page},info)=>{
  const files={'gallery/page.html':gallery,'gallery/blue.png':png(),'gallery/red.png':png(240,160,[190,60,50]),'missing.html':'<!doctype html><h1>Warnings</h1><img src="missing.png"><img src="bad.svg">','diagram.svg':'<svg xmlns="http://www.w3.org/2000/svg" width="240" height="120"><rect width="240" height="120" fill="orange"/></svg>','source.txt':'original source','data.json':'{"large":9007199254740993}','start.md':'# Links\n\n[Gallery](gallery/page.html)','binary.bin':Buffer.from([0,255,1]),'archive.zip':Buffer.from([80,75,3,4])};
  const {open,scope,download}=await fixture(page,files,viewer,width);
  await open('gallery/page.html');
  const frame=page.frameLocator('iframe[title="Interactive HTML preview: gallery/page.html"]');
  await expect(frame.locator('h1')).toHaveCSS('background-color','rgb(35, 110, 190)');
  await expect(frame.locator('body')).toHaveAttribute('data-order','first,second');
  await expect(frame.locator('html')).toHaveAttribute('data-envelope','root');await expect(frame.locator('head')).toHaveAttribute('data-envelope','head');await expect(frame.locator('body')).toHaveAttribute('data-envelope','body');
  expect(await frame.locator('html').evaluate(()=>document.compatMode)).toBe('CSS1Compat');
  await expect.poll(()=>frame.getByAltText('Screenshot',{exact:true}).evaluate((el:HTMLImageElement)=>el.complete&&el.naturalWidth)).toBe(120);
  await expect(frame.locator('svg rect')).toHaveAttribute('fill','orange');
  const preparedDocument=await frame.locator('html').evaluate(el=>el.outerHTML);expect(preparedDocument).not.toMatch(/\/api\/|colony2:user:email|taskOrdinal|producer/);
  const interactionStarted=Date.now();
  const next=frame.getByRole('button',{name:'Next screenshot'});await next.focus();await page.keyboard.press('Enter');await expect(frame.locator('output')).toHaveText('1');await expect.poll(()=>frame.getByAltText('Screenshot',{exact:true}).evaluate((el:HTMLImageElement)=>el.currentSrc===document.querySelector<HTMLImageElement>('[alt=Repeated]')!.src)).toBe(true);
  const interactionMs=Date.now()-interactionStarted;
  const {writeFile}=await import('node:fs/promises');await writeFile(info.outputPath(`${viewer}-${width}-responsiveness.json`),JSON.stringify({viewer,width,interactionMs,hostHeartbeat:await page.evaluate(()=>Reflect.get(window,'previewResponsiveness')),browser:page.context().browser()?.version()},null,2));
  await next.hover();await expect(next).toHaveCSS('background-color','rgb(255, 220, 110)');
  if(width===390) await expect(frame.locator('h1')).toHaveCSS('border-radius','0px');
  await scope.screenshot({path:info.outputPath(`${viewer}-${width}-gallery.png`)});
  if(viewer==='story'){await scope.locator('.ant-modal-body').evaluate(el=>el.scrollTop=0);await scope.screenshot({path:info.outputPath(`${viewer}-${width}-controls.png`)});}
  await download('gallery/page.html');
  const old=page.frames().find(f=>f.name()===''&&f.url().startsWith('blob:'))!;
  await scope.getByRole('radio',{name:'Source',exact:true}).focus();await page.keyboard.press('Space');
  await expect(page.locator('iframe')).toHaveCount(0);expect(old.isDetached()).toBe(true);
  await expect(scope.getByLabel('Document source',{exact:true})).toHaveText(gallery);
  await download('gallery/page.html');
  await scope.getByText('Rendered',{exact:true}).click();await expect(frame.locator('output')).toHaveText('0');
  await open('missing.html');await expect(scope.getByText('Some images could not be prepared')).toBeVisible();await expect(scope.getByRole('listitem').filter({hasText:'missing.png'})).toBeVisible();
  await scope.screenshot({path:info.outputPath(`${viewer}-${width}-warnings.png`)});
  await open('diagram.svg');await expect(scope.getByRole('img',{name:'diagram.svg',exact:true})).toBeVisible();await expect(scope.getByText(/Static preview:/)).toBeVisible();await download('diagram.svg');
  await open('data.json');await expect(scope.getByLabel('Formatted JSON')).toContainText('9007199254740993');await download('data.json');
  await open('source.txt');await expect(scope.getByLabel('Document source',{exact:true})).toHaveText('original source');
  await open('binary.bin');await expect(scope.getByLabel('Binary bytes')).toContainText('00000000');
  await open('archive.zip');await expect(scope.getByText(/Archives are not extracted/)).toBeVisible();
  await open('start.md');await scope.getByRole('button',{name:'Gallery',exact:true}).click();await expect(frame.locator('h1')).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  if(viewer==='review'){await page.getByRole('textbox',{name:'Notes'}).fill('Preserve response draft');const submission=page.waitForRequest(r=>r.url().endsWith('/reviews/job/respond')&&r.method()==='POST');await page.getByRole('button',{name:'Approve',exact:true}).focus();await page.keyboard.press('Enter');await page.getByRole('button',{name:'OK',exact:true}).focus();await page.keyboard.press('Enter');expect((await submission).postData()).toContain('Preserve response draft');}
 });
}

test('HP-005,006 authored CSP and public absolute CSS/JS retain browser behavior',async({page})=>{
 const allowed='<!doctype html><html lang="en"><head><link rel="stylesheet" href="http://127.0.0.1:15175/public.css"><script src="http://127.0.0.1:15175/public.js"></script></head><body><h1>Public dependencies</h1><output id="public">pending</output><script>document.querySelector("output").textContent=window.publicScript</script></body></html>';
 const denied=allowed.replace('<head>','<head><meta http-equiv="Content-Security-Policy" content="script-src \'none\'; style-src \'none\'">');
 const {open}=await fixture(page,{'allowed.html':allowed,'denied.html':denied},'review');await open('allowed.html');
 let f=page.frameLocator('iframe');await expect(f.locator('output')).toHaveText('public script loaded');await expect(f.locator('h1')).toHaveCSS('color','rgb(120, 30, 150)');
 await open('denied.html');f=page.frameLocator('iframe');await expect(f.locator('output')).toHaveText('pending');await expect(f.locator('h1')).not.toHaveCSS('color','rgb(120, 30, 150)');
});

for (const [name,text,error] of [
 ['base.html','<!doctype html><base href="/api/"><h1>Original document</h1>',/Authored base URLs are unsupported/],
 ['aggregate.html','<!doctype html>'+Array.from({length:33},(_,i)=>`<img src="missing-${i}.png">`).join(''),/aggregate limit exceeded/],
] as const) test(`HP-011,018,020 preparation error and retry preserve ${name}`,async({page},info)=>{
 const {open,scope,download}=await fixture(page,{[name]:text},'review');await open(name);
 await expect(scope.getByText(error)).toBeVisible();await expect(scope.getByLabel('Document source',{exact:true})).toHaveText(text);await expect(page.locator('iframe')).toHaveCount(0);await download(name);
 await scope.getByRole('button',{name:'Retry rendering'}).click();await expect(scope.getByText(error)).toBeVisible();
 await scope.screenshot({path:info.outputPath(`review-${name}-error.png`)});
});

test('HP-009,010 measured decoding, aggregate and expansion boundaries',async({page,context},info)=>{
 test.setTimeout(120_000);
 const {paddedPNG}=await import('./helpers/raster');const {readFile,writeFile}=await import('node:fs/promises');
 const max=16*1024*1024;
 const files={'a.png':paddedPNG(max),'b.png':paddedPNG(max),'over.png':paddedPNG(max+1),'small.png':png(1,1),'pixels.png':png(8000,4000),'expand.png':paddedPNG(12*1024*1024-128)};
 await page.request.post('http://127.0.0.1:15175/__files',{data:Object.fromEntries(Object.entries(files).map(([name,value])=>[name,value.toString('base64')]))});
 await page.goto('/');
 const cdp=await context.browser()!.newBrowserCDPSession();const {processInfo}=await cdp.send('SystemInfo.getProcessInfo');
 const rss=async()=>{let total=0;for(const p of processInfo){try{const status=await readFile(`/proc/${p.id}/status`,'utf8');total+=Number(/VmRSS:\s+(\d+)/.exec(status)?.[1]||0)*1024;}catch{/* process exited */}}return total;};
 const baseline=await rss();let peak=baseline;let sampling=false;
 const sampler=setInterval(()=>{if(sampling)return;sampling=true;void rss().then(value=>{peak=Math.max(peak,value)}).finally(()=>sampling=false)},10);
 const started=Date.now();
 try {
 const results=await page.evaluate(async()=>{
  // @ts-expect-error Vite serves this browser-only module URL.
  const {prepareHTML,HTML_LIMITS}=await import('/src/components/review/htmlPreparation.ts');
  const prepare=async(refs:string[],padding=0)=>{const text='<!DOCTYPE html><html><head></head><body>'+refs.map(src=>`<img src="${src}">`).join('')+' '.repeat(padding)+'</body></html>';const data={text,bytes:new TextEncoder().encode(text),format:'html' as const,truncated:false};const context={canonicalPath:'page.html',attachments:['a.png','b.png','over.png','small.png','pixels.png','expand.png'].map(path=>({key:path,path,url:`/api/bench/documents?document_id=${path}`}))};return prepareHTML(data,context,new AbortController().signal);};
  const exact=await prepare(['a.png','b.png']);
  const oversized=await prepare(['over.png']);
  let aggregate='',expansion='';try{await prepare(['a.png','b.png','small.png'])}catch(e){aggregate=(e as Error).message}try{await prepare(['a.png','a.png','a.png'])}catch(e){expansion=(e as Error).message}
  const pixels=await prepare(['pixels.png']);
  const encodedLength=Math.ceil((12*1024*1024-128)/3)*4+'data:image/png;base64,'.length;
  const overhead='<!DOCTYPE html><html><head></head><body></body></html>'.length+3*'<img src="">'.length;
  const padding=HTML_LIMITS.outputBytes-3*encodedLength-overhead;
  const exactExpanded=await prepare(['expand.png','expand.png','expand.png'],padding);
  let exceededExpanded='';try{await prepare(['expand.png','expand.png','expand.png'],padding+1)}catch(e){exceededExpanded=(e as Error).message}
  return {exactExpanded:exactExpanded.outputBytes,exceededExpanded,limits:HTML_LIMITS,exact:{bytes:exact.imageBytes,output:exact.outputBytes,warnings:exact.warnings},oversized:oversized.warnings,aggregate,expansion,pixels:{bytes:pixels.imageBytes,warnings:pixels.warnings}};
 });
 expect(results.exactExpanded).toBe(48*1024*1024);expect(results.exceededExpanded).toContain('48 MiB');expect(results.exact.bytes).toBe(32*1024*1024);expect(results.exact.warnings).toEqual([]);expect(results.exact.output).toBeLessThanOrEqual(48*1024*1024);expect(results.oversized[0]).toContain('16 MiB');expect(results.aggregate).toContain('aggregate');expect(results.expansion).toContain('48 MiB');expect(results.pixels.warnings).toEqual([]);
 await writeFile(info.outputPath('measured-budgets.json'),JSON.stringify({...results,browser:context.browser()?.version(),elapsedMs:Date.now()-started,summedProcessRSSBaseline:baseline,summedProcessRSSPeak:peak,summedProcessRSSIncrease:peak-baseline,samplingIntervalMs:10},null,2));
 }finally{clearInterval(sampler);await cdp.detach()}
});
