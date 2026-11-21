import {expect} from '@playwright/test';
import {test} from './helpers/nativeZoom';
import {authorizeV2,reviewV2BrowserFixture,v2Path,v2Selected} from './helpers/reviewV2';

for(const width of [1440,390]) test(`shared input discovery retains reviews and quiet gaps at ${width}px`,async({page,baseURL})=>{
 await authorizeV2(page,baseURL!);
 // Expose only the browser's real EventSource callbacks for deterministic outages.
 await page.addInitScript(()=>{
  (window as any).sources=[];
  (window as any).EventSource=class {
   readyState=1;onopen:any;onerror:any;events=new Map<string,Function>();
   constructor(){(window as any).sources.push(this);}
   close(){}addEventListener(name:string,fn:Function){this.events.set(name,fn);}removeEventListener(){}
  };
 });
 await page.setViewportSize({width,height:900});
 const fixture=await reviewV2BrowserFixture(page);
 const captures:string[]=[];
 const capture=async(name:string)=>{const file=test.info().outputPath(name);await page.screenshot({path:file,fullPage:false});captures.push(file);};
 const statuses:number[]=[];page.on('response',r=>{if(/\/user-inputs\/one$/.test(r.url()))statuses.push(r.status());});
 await page.goto(v2Path);await v2Selected(page,0);
 await expect.poll(()=>fixture.state.details).toBe(1);
 await page.getByRole('button',{name:'Ask a question',exact:true}).click();
 await page.getByLabel('Question or requested change (required)').fill('Keep my draft during reconnect.');
 await page.evaluate(()=>{for(const s of (window as any).sources)s.onerror?.();});
 await expect(page.getByRole('status').filter({hasText:'Review state unavailable'})).toBeVisible();
 await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeDisabled();
 await expect(page.getByLabel('Question or requested change (required)')).toHaveValue('Keep my draft during reconnect.');
 await capture(`discovery-unavailable-${width}.png`);
 await page.evaluate((requestId)=>{for(const s of (window as any).sources)s.events.get('inputs_changed')?.({data:JSON.stringify({inputs:[{id:'one',cell_id:'cell-platform',cell_name:'platform',repo:'github.com/acme/platform',request_id:requestId,client_payload_revision:1}]})});},fixture.state.native);
 await expect(page.getByRole('button',{name:'Send · Ask a question'})).toBeEnabled();
 expect(fixture.state.details).toBe(1);
 await page.getByRole('button',{name:'Send · Ask a question'}).click();
 await expect(page.getByText('Response accepted',{exact:true})).toBeVisible();
 // Independent history ticks and unchanged discovery must keep details quiet.
 await page.waitForTimeout(5600);expect(fixture.state.details).toBe(1);
 await expect(page.locator('.review-v2-response .ant-btn-loading')).toHaveCount(0);
 await capture(`discovery-gap-${width}.png`);
 await fixture.successor();
 await page.getByRole('button',{name:'Refresh',exact:true}).click();
 await expect(page.getByRole('button',{name:'Approve direction',exact:true})).toBeEnabled();
 await expect.poll(()=>fixture.state.details).toBe(2);
 await capture(`discovery-successor-${width}.png`);
 expect(statuses).toEqual([200,200]);
 if(width===1440){
  await page.goto('/project/v2/reviews');
  await expect(page.getByText('Job ID: one',{exact:true})).toBeVisible();
  await page.evaluate(()=>{for(const s of (window as any).sources)s.onerror?.();});
  await expect(page.getByText('Last known: Showing 1 of 1 pending requests',{exact:true})).toBeVisible();
  await capture('discovery-list-unavailable.png');
  expect(statuses).toEqual([200,200]);
 }
 for(const file of captures)await test.info().attach(file.split('/').pop()!,{path:file,contentType:'image/png'});
});

test('direct links show neutral absence and a race returns 204 once',async({page,baseURL})=>{
 await authorizeV2(page,baseURL!);const fixture=await reviewV2BrowserFixture(page);
 fixture.state.pending=false;
 await page.goto(v2Path);await expect(page.getByText('No outstanding input request.',{exact:true})).toBeVisible();
 expect(fixture.state.details).toBe(0);
 await page.screenshot({path:test.info().outputPath('discovery-empty.png')});
 fixture.state.pending=true;
 // Advertise a match then complete it immediately before its content is read.
 await page.route('**/user-inputs/one',r=>{fixture.state.details++;return r.fulfill({status:204,body:''});});
 await page.getByRole('button',{name:'Refresh',exact:true}).click();
 await expect.poll(()=>fixture.state.details).toBe(1);
 await page.waitForTimeout(5500);expect(fixture.state.details).toBe(1);
});
