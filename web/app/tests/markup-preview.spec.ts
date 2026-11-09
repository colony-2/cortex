import { test, expect, type Page } from '@playwright/test';
import { readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
const png='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=';
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
  if(viewer==='story')await page.getByRole('tab',{name:`Artifacts (${names.length})`}).click();
  const open=async(name:string)=>{
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
    await scope.getByRole(viewer==='story'?'button':'link',{name:/Download/}).click();
    const saved=await pending;
    expect(saved.suggestedFilename()).toBe(name);
    const original=Buffer.from(files[name]);
    const downloaded=await readFile((await saved.path())!);
    expect(downloaded).toEqual(original);
    await writeFile(test.info().outputPath(`${name}-download.json`),JSON.stringify({name,bytes:downloaded.length,sha256:createHash('sha256').update(downloaded).digest('hex'),originalSHA256:createHash('sha256').update(original).digest('hex'),identical:true}));
  };
  return {open,scope,download};
}

for(const viewer of ['review','story'] as const)for(const width of [1280,390]) {
  test(`HSV-005,012,016,017,020,021,022 visual flow ${viewer} ${width}`,async({page},info)=>{
    const files={'start.md':'# Preview links\n\n[page](page.html) [diagram](diagram.svg)\n\n![unsupported SVG](diagram.svg)','page.html':plainHTML,'diagram.svg':diagram,'pixel.png':Buffer.from(png,'base64'),'data.json':'{"large":9007199254740993}','source.txt':'original source','binary.bin':Buffer.from([0,255,1]),'archive.zip':Buffer.from([80,75,3,4])};
    const {open,scope,download}=await fixture(page,files,viewer,width);
    if(viewer==='review')await page.getByRole('textbox',{name:'Notes'}).fill('Preserve this response draft');
    await open('start.md');
    await expect(scope.getByRole('heading',{name:'Preview links'})).toBeVisible();
    await expect(scope.getByText('Attachment is not a raster image.')).toBeVisible();
    const link=scope.getByRole('button',{name:'page',exact:true});await link.focus();await page.keyboard.press('Enter');
    const frame=page.frameLocator('iframe[title="Static HTML preview: page.html"]');
    await expect(frame.locator('.card')).toHaveCSS('background-color','rgb(20, 100, 160)');
    await expect(frame.getByText('Static page 雪')).toBeVisible();
    await expect(frame.locator('svg rect')).toHaveAttribute('fill','orange');
    await expect(scope.getByText(/Static preview:/)).toBeVisible();
    await settle(page);
    await page.screenshot({path:info.outputPath(`${viewer}-${width}-html.png`)});
    await download('page.html');
    await scope.getByRole('radio',{name:'Source',exact:true}).focus();await page.keyboard.press('Space');
    await expect(scope.getByLabel('Document source',{exact:true})).toHaveText(plainHTML);
    await scope.getByLabel('Search document').fill('Static page');await expect(scope.getByText('1 / 1')).toBeVisible();
    await scope.getByText('Rendered',{exact:true}).click();
    await expect(frame.locator('.card')).toBeVisible();
    await open('start.md');await scope.getByRole('button',{name:'diagram',exact:true}).click();
    const image=scope.getByRole('img',{name:'diagram.svg',exact:true});await expect(image).toBeVisible();
    await expect.poll(()=>image.evaluate((e:HTMLImageElement)=>e.complete&&e.naturalWidth)).toBe(240);
    const pixels=await image.evaluate((el:HTMLImageElement)=>{const c=document.createElement('canvas');c.width=240;c.height=120;const ctx=c.getContext('2d')!;ctx.drawImage(el,0,0);return {left:[...ctx.getImageData(5,5,1,1).data],right:[...ctx.getImageData(235,5,1,1).data]};});
    expect(pixels.left[0]).toBeGreaterThan(pixels.left[2]);expect(pixels.right[2]).toBeGreaterThan(pixels.right[0]);
    await scope.getByRole('button',{name:'Actual size',exact:true}).click();await expect(image).toHaveAttribute('width','240');
    await scope.getByRole('button',{name:'Zoom in'}).click();await expect(image).toHaveAttribute('width','300');
    await scope.getByRole('button',{name:'Fit',exact:true}).click();await expect(image).not.toHaveAttribute('width');
    await settle(page);
    await page.screenshot({path:info.outputPath(`${viewer}-${width}-svg.png`)});
    await download('diagram.svg');
    for(const name of ['pixel.png','data.json','source.txt','binary.bin','archive.zip']) {
      await open(name);
      if(name==='pixel.png')await expect(scope.getByRole('img',{name})).toBeVisible();
      if(name==='data.json')await expect(scope.getByLabel('Formatted JSON')).toContainText('9007199254740993');
      if(name==='source.txt')await expect(scope.getByLabel('Document source',{exact:true})).toHaveText('original source');
      if(name==='binary.bin')await expect(scope.getByLabel('Binary bytes')).toContainText('00000000');
      if(name==='archive.zip')await expect(scope.getByText(/Archives are not extracted/)).toBeVisible();
      await download(name);
    }
    if(viewer==='review') {
      await expect(page.getByRole('dialog')).toHaveCount(0);
      await page.getByRole('textbox',{name:'Notes'}).focus();
      await expect(page.getByRole('textbox',{name:'Notes'})).toBeFocused();
      await page.getByRole('button',{name:'Approve',exact:true}).focus();
      await expect(page.getByRole('button',{name:'Approve',exact:true})).toBeFocused();
      await expect(page.getByRole('textbox',{name:'Notes'})).toHaveValue('Preserve this response draft');
    }
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  });
}

const attack=`
<div id="css-subset" style="padding:7px;background:image('data:image/svg+xml,&lt;svg/&gt;');color:rgb(10, 20, 30)">Subset</div>
<style>.bad{background:image-set("data:image/svg+xml,&lt;svg/&gt;" 1x)} .vars{--asset:url(data:image/svg+xml,bad);background:var(--asset)} .attr{background:attr(title type(*))}</style>
<img srcset="data:image/svg+xml,bad 1x" src="data:image/svg+xml,bad">
<svg><image xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="data:image/svg+xml,bad" href="data:text/html,bad"/><image href="data:image/png;base64,AAAA"/></svg>
<p id="safe" style="color:rgb(10, 20, 30)">Safe static content</p>
<script>parent.previewSentinel='script';fetch('/probe-script');window.open('/probe-popup');</script>
<script src="https://evil.test/probe-script"></script>
<img src="/probe-relative" onerror="parent.previewSentinel='event'">
<img src="https://evil.test/probe-remote"><img src="/probe-redirect">
<link rel="stylesheet" href="/probe-css"><style>@import '/probe-import';@font-face{font-family:evil;src:url(/probe-font)}p{background:url(/probe-style)}</style>
<p style="background:u\\72l('/probe-escape')">CSS</p>
<iframe src="/probe-frame" srcdoc="<img src='/probe-inner'>"></iframe><object data="/probe-object"></object><embed src="/probe-embed"><video poster="/probe-poster" src="/probe-video"></video>
<meta http-equiv="refresh" content="0;url=/probe-refresh"><base href="https://evil.test/probe-base">
<a id="outbound" href="https://evil.test/probe-nav" ping="/probe-ping" target="_blank" download>Outbound</a>
<form action="/probe-form" target="_top"><input autofocus><button>Submit</button></form>
<svg onload="parent.previewSentinel='svg'"><script>fetch('/probe-svg')</script><image href="/probe-svg-image"/><use href="https://evil.test/probe-use#x"/><foreignObject><iframe src="/probe-foreign"></iframe></foreignObject><a href="/probe-svg-nav"><text>SVG link</text></a><animate attributeName="href" to="/probe-animate"/></svg>
<img src="data:text/html;base64,PGh0bWw+PC9odG1sPg=="><img src="data:image/svg+xml;base64,PHN2Zy8+">
<math><mtext><table><mglyph><style><!--</style><img title="--><img src='/probe-malformed' onerror='parent.previewSentinel=1'>">
`;
for(const viewer of ['review','story'] as const) {
  test(`HSV-006-009,011,014 browser security ${viewer}`,async({page,context},info)=>{
    const requests:string[]=[];const downloads:string[]=[];const navigations:string[]=[];
    page.on('request',r=>{if(/probe-|evil\.test/.test(r.url()))requests.push(r.url());});
    page.on('download',d=>downloads.push(d.suggestedFilename()));
    page.on('framenavigated',f=>{if(/probe-|evil\.test/.test(f.url()))navigations.push(f.url());});
    await page.route('**/*probe*',route=>route.fulfill({status:302,headers:{location:'https://evil.test/probe-redirected'}}));
    const {open,scope}=await fixture(page,{'attack.html':attack,'attack.svg':svg(`<script>fetch('/probe-script')</script><image href="/probe-image"/><use href="https://evil.test/probe-use#x"/><foreignObject><iframe src="/probe-frame"/></foreignObject><rect width="40" height="40" fill="green" onload="parent.previewSentinel=1"/>`)},viewer);
    await open('attack.html');
    const element=scope.locator('iframe');await expect(element).toHaveAttribute('sandbox','');
    await expect(element).toHaveAttribute('referrerpolicy','no-referrer');await expect(element).toHaveAttribute('title','Static HTML preview: attack.html');
    await expect(page.frameLocator('iframe').locator('#safe')).toBeVisible();
    const frame=(await (await element.elementHandle())!.contentFrame())!;
    await expect(frame.locator('#safe')).toHaveCSS('color','rgb(10, 20, 30)');
    expect(await frame.evaluate(()=>({origin:origin,dom:(()=>{try{return parent.document.body.innerHTML}catch{return 'blocked'}})(),cookie:(()=>{try{return document.cookie}catch{return 'blocked'}})(),storage:(()=>{try{return localStorage.getItem('preview-secret')}catch{return 'blocked'}})()}))).toEqual({origin:'null',dom:'blocked',cookie:'blocked',storage:'blocked'});
    expect(await frame.locator('meta[http-equiv="Content-Security-Policy"]').getAttribute('content')).toContain("default-src 'none'");
    await frame.locator('#outbound').click();
    const copy=await frame.locator('body').innerHTML();
    expect(copy).not.toMatch(/data:|srcset|xlink:href|image-set|var\(|attr\(|--asset/);
    await expect(frame.locator('#css-subset')).toHaveCSS('padding','0px');
    expect(await frame.locator('script,form,iframe,object,embed,video,base,foreignObject,animate').count()).toBe(0);
    expect(await frame.locator('#outbound').getAttribute('href')).toBeNull();
    await page.screenshot({path:info.outputPath(`${viewer}-security.png`)});
    await open('attack.svg');await expect(scope.getByRole('img',{name:'attack.svg'})).toBeVisible();
    await expect.poll(()=>scope.getByRole('img',{name:'attack.svg',exact:true}).evaluate((e:HTMLImageElement)=>e.complete&&e.naturalWidth)).toBe(240);
    // Let refresh/deferred resource processing settle; this is an observation window.
    await page.waitForTimeout(350);
    expect(requests).toEqual([]);expect(downloads).toEqual([]);expect(navigations).toEqual([]);expect(context.pages()).toHaveLength(1);
    expect(await page.evaluate(()=>({sentinel:(window as any).previewSentinel,storage:localStorage.getItem('preview-secret'),cookie:document.cookie}))).toMatchObject({sentinel:'untouched',storage:'parent-storage-sentinel',cookie:expect.stringContaining('parent-cookie-sentinel')});
    const delivered=await (await page.request.get('http://127.0.0.1:15175/__probes')).json();
    expect(delivered).toEqual([]);
    const observations=info.outputPath(`${viewer}-network-observations.json`);
    await writeFile(observations,JSON.stringify({requests,delivered,downloads,navigations,pages:context.pages().length},null,2));
    await info.attach('network-observations',{path:observations,contentType:'application/json'});
  });
}

test('HSV-003,004,013,015,017,018 limited invalid and unavailable previews',async({page},info)=>{
  const files:Files={
    'invalid.svg':svg('<g>'),
    'dtd.svg':'<!DOCTYPE svg [<!ENTITY x "abc">]>'+svg('&x;'),
    'dimensions.svg':svg('<rect/>','width="999999" height="1"'),
    'filter.svg':svg('<filter id="f"><feTurbulence baseFrequency=".99" numOctaves="100"/></filter><rect filter="url(#f)"/>'),
    'cyclic.svg':svg('<g id="a"><use href="#a"/></g>'),
    'deep.svg':svg('<g>'.repeat(40)+'<rect/>'+'</g>'.repeat(40)),
    'limit.html':'<p>bounded</p>'+' '.repeat(1048576),
    'lines.html':'<p>bounded</p>'+'\n'.repeat(20000),
    'truncated.html':'<p>partial</p>'+' '.repeat(2097152),
  };
  const {open,scope,download}=await fixture(page,files,'story');
  for(const name of Object.keys(files)) {
    await open(name);await expect(scope.getByLabel('Document source',{exact:true})).toBeVisible();
    await expect(scope.getByText(/showing (bounded )?source/i)).toBeVisible();
    await expect(scope.locator('img,iframe')).toHaveCount(0);
    expect((await scope.getByLabel('Document source',{exact:true}).textContent())!.length).toBeLessThanOrEqual(24000);
    await download(name);
  }
  await page.screenshot({path:info.outputPath('limited-source.png')});
});

test('HSV-018 retrieval retry and HSV-019 obsolete load cancellation',async({page})=>{
  const {open,scope}=await fixture(page,{'a.html':plainHTML,'b.svg':diagram},'review');
  await expect(scope.locator('iframe')).toBeVisible();
  let fail=true;
  await page.route('**/documents?**',async route=>{
    const url=new URL(route.request().url());
    if(url.searchParams.get('document_id')==='b.svg'&&fail)return route.fulfill({status:503,body:'retry'});
    return route.fulfill({body:url.searchParams.get('document_id')==='b.svg'?diagram:plainHTML});
  });
  await open('b.svg');await expect(scope.getByText(/Could not load this document/)).toBeVisible();
  fail=false;await scope.getByRole('button',{name:'Retry',exact:true}).click();await expect(scope.getByRole('img',{name:'b.svg'})).toBeVisible();
  await page.route('**/documents?**',async route=>{
    const name=new URL(route.request().url()).searchParams.get('document_id');
    if(name==='a.html')await new Promise(resolve=>setTimeout(resolve,400));
    await route.fulfill({body:name==='a.html'?plainHTML:diagram}).catch(()=>{});
  });
  await open('a.html');await expect(scope.getByLabel('Loading document')).toBeVisible();
  await open('b.svg');await expect(scope.getByRole('img',{name:'b.svg'})).toBeVisible();
  await page.waitForTimeout(500);await expect(scope.locator('iframe')).toHaveCount(0);
  await expect(scope.getByRole('img',{name:'b.svg'})).toBeVisible();
});


for (const viewer of ['review','story'] as const) {
  test(`HSV-005,024,027 embedded raster pixels ${viewer}`,async({page},info)=>{
    const content=html+`<svg width="40" height="40" style="background:black"><image width="40" height="40" href="DATA:IMAGE/PNG;base64,${png}"/></svg>`;
    const {open,scope,download}=await fixture(page,{'embedded.html':content},viewer);
    await open('embedded.html');
    const frame=page.frameLocator('iframe');
    const image=frame.getByAltText('embedded');
    await expect.poll(()=>image.evaluate((e:HTMLImageElement)=>e.complete&&e.naturalWidth)).toBe(1);
    await expect(image).toHaveAttribute('src',`data:image/png;base64,${png}`);
    await expect(frame.locator('svg image')).toHaveAttribute('href',`data:image/png;base64,${png}`);
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
  await expect(scope.getByText(/Rendering unavailable/)).toBeVisible();
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
    await scope.getByRole('combobox',{name:'Review document'}).focus();await page.keyboard.press('ArrowDown');
    await page.getByTitle(`${name} · ${name}`,{exact:true}).click();
  };
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
