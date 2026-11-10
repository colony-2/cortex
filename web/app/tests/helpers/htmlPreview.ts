import { test, expect, type Page } from '@playwright/test';
import {readFile,writeFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
async function settle(page:Page) { await page.evaluate(async()=>{await Promise.all(document.getAnimations().filter(a=>a.effect?.getComputedTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{})));}); }
type Files=Record<string,string|Buffer>;
export async function fixture(page:Page,files:Files,viewer:'review'|'story',width=1280) {
  await page.setViewportSize({width,height:900});
  await page.context().addCookies([{name:'colony2:user:email',value:'reviewer@example.com',url:'http://127.0.0.1:15173'},{name:'preview-secret',value:'parent-cookie-sentinel',url:'http://127.0.0.1:15173'}]);
  await page.addInitScript(()=>{const metric={last:performance.now(),longestGapMs:0,ticks:0};Reflect.set(window,'previewResponsiveness',metric);setInterval(()=>{const now=performance.now();metric.longestGapMs=Math.max(metric.longestGapMs,now-metric.last);metric.last=now;metric.ticks++},16)});
  await page.addInitScript(()=>{localStorage.setItem('preview-secret','parent-storage-sentinel');(window as any).previewSentinel='untouched';});
  const names=Object.keys(files);
  await page.request.post('http://127.0.0.1:15175/__files',{data:Object.fromEntries(Object.entries(files).map(([name,value])=>[name,Buffer.from(value).toString('base64')]))});
  await page.route('**/api/**',async route=>{
    const url=new URL(route.request().url());
    if(url.port !== '15173') return route.continue();
    if(url.pathname.includes('/documents')||url.pathname.includes('/artifacts/')) {
      const name=url.searchParams.get('document_id')||decodeURIComponent(url.pathname.split('/artifacts/')[1]);
      return route.fulfill({body:files[name]??'',contentType:'application/octet-stream'});
    }
    let body:unknown=[];
    if(url.pathname==='/api/projects')body=[{tenant_id:'tenant',name:'Tenant'}];
    else if(url.pathname.endsWith('/user-inputs/job'))body={jobId:'job',status:'pending',form:{kind:'review',request_id:'r',overview_document_id:names[0],title:'Interactive preview review',fields:[{id:'notes',question:'Notes',type:'paragraph_text',required:true},{id:'decision',question:'Decision',type:'multiple_choice',required:true,options:[{value:'approve',label:'Approve'},{value:'reject',label:'Reject'}]}],documents:Object.fromEntries(names.map((name,i)=>[name,{kind:'stored',name,stored:{key:{jobId:'producer',taskOrdinal:i+1,name}}}]))}};
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
    expect(saved.suggestedFilename()).toBe(name.replaceAll('/','_'));
    const original=Buffer.from(files[name]);
    const downloaded=await readFile((await saved.path())!);
    expect(downloaded).toEqual(original);
    await writeFile(test.info().outputPath(`${name.replaceAll('/','_')}-download.json`),JSON.stringify({name,bytes:downloaded.length,sha256:createHash('sha256').update(downloaded).digest('hex'),originalSHA256:createHash('sha256').update(original).digest('hex'),identical:true}));
  };
  return {open,scope,download};
}
