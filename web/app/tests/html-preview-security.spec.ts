import {test,expect} from '@playwright/test';
import {fixture} from './helpers/htmlPreview';
import {png} from './helpers/raster';
import {writeFile} from 'node:fs/promises';
const api='http://127.0.0.1:15175';

test('HP-012–015 effect assertions cover connections, resources, nested frames and browser capabilities',async({page,context},info)=>{
 await page.request.get(`${api}/__security/reset`);
 await context.addCookies([{name:'colony2:user:email',value:'synthetic%40example.test',url:api}]);
 // Positive controls establish real received credentials, visible data and mutation.
 await page.goto(`${api}/security/read`);
 const positive=await page.evaluate(async()=>({read:await (await fetch('/security/protected')).text(),write:(await fetch('/security/protected',{method:'POST'})).status}));
 expect(positive).toEqual({read:'protected-fixture-value',write:200});
 let received=await (await page.request.get(`${api}/__security`)).json();expect(received.writes).toBe(1);expect(received.received.some((r:any)=>r.cookie.includes('colony2:user:email'))).toBe(true);
 await page.request.get(`${api}/__security/reset`);
 const attack=`<!doctype html><html><body><img src="${api}/security/preparation.png"><iframe src="${api}/security/preparation-frame"></iframe><object data="${api}/security/preparation-object"></object><output id="results">pending</output><img src="pixel.png"><script>
 (async()=>{
 const result={};
 try{parent.document.body;result.dom='read'}catch{result.dom='denied'}
 try{localStorage.getItem('x');result.storage='read'}catch{result.storage='denied'}
 result.popup=window.open('${api}/security/popup')===null;
 try{top.location.href='${api}/security/top';result.top='allowed'}catch{result.top='denied'}
 try{await fetch('${api}/security/write',{method:'POST',mode:'no-cors',credentials:'include',body:''});result.fetch='allowed'}catch{result.fetch='denied'}
 try{await fetch('${api}/security/redirect',{method:'POST',mode:'no-cors',credentials:'include'});result.redirect='allowed'}catch{result.redirect='denied'}
 try{await fetch('http://127.0.0.2:15175/security/write',{method:'POST',mode:'no-cors',credentials:'include'});result.crossSite='allowed'}catch{result.crossSite='denied'}
 try{await fetch('http://127.0.0.1:15173/security/write',{method:'POST',mode:'no-cors',credentials:'include'});result.sameOrigin='allowed'}catch{result.sameOrigin='denied'}
 result.xhr=await new Promise(resolve=>{const x=new XMLHttpRequest();x.open('POST','${api}/security/write');x.withCredentials=true;x.onload=()=>resolve('allowed');x.onerror=()=>resolve('denied');try{x.send('')}catch{resolve('denied')}});
 try{result.beacon=navigator.sendBeacon('${api}/security/write','')}catch{result.beacon=false}
 result.websocket=await new Promise(resolve=>{try{const ws=new WebSocket('ws://127.0.0.1:15175/security/socket');ws.onerror=()=>resolve('denied');ws.onopen=()=>{ws.close();resolve('allowed')}}catch{resolve('denied')}});
 const nested=document.createElement('iframe');nested.srcdoc='<script>fetch("${api}/security/write",{method:"POST",mode:"no-cors"})<'+ '/script>';document.body.append(nested);
 const form=document.createElement('form');form.method='POST';form.action='${api}/security/write';document.body.append(form);form.submit();
 const download=document.createElement('a');download.href='data:text/plain,fixture';download.download='forbidden.txt';download.click();
 const im=new Image();im.src='${api}/security/resource.png';document.body.append(im);
 const script=document.createElement('script');script.src='${api}/security/public.js';document.body.append(script);
 result.camera=document.permissionsPolicy ? document.permissionsPolicy.allowsFeature('camera') : document.featurePolicy.allowsFeature('camera');
 result.microphone=document.featurePolicy.allowsFeature('microphone');result.geo=document.featurePolicy.allowsFeature('geolocation');
 document.querySelector('output').textContent=JSON.stringify(result);
 })();</script></body></html>`;
 const popups:any[]=[];const downloads:any[]=[];context.on('page',p=>popups.push(p));page.on('download',d=>downloads.push(d));
 const {open}=await fixture(page,{'attack.html':attack,'pixel.png':png()},'review');await open('attack.html');
 const f=page.frameLocator('iframe[title="Interactive HTML preview: attack.html"]');
 await expect(f.locator('output')).not.toHaveText('pending');
 const result=JSON.parse((await f.locator('output').textContent())!);
 expect(result).toMatchObject({dom:'denied',storage:'denied',popup:true,top:'denied',fetch:'denied',redirect:'denied',crossSite:'denied',sameOrigin:'denied',xhr:'denied',websocket:'denied',camera:false,microphone:false,geo:false});
 await expect(f.locator('img[src^="data:image/png"]')).toHaveCount(1);
 await expect.poll(async()=>{const r=await (await page.request.get(`${api}/__security`)).json();return r.received.filter((r:any)=>r.path.endsWith('.js')).length;}).toBe(1);
 received=await (await page.request.get(`${api}/__security`)).json();
 expect(await f.locator('body').evaluate(()=>Reflect.get(window,'resourceLoaded'))).not.toBe(true);
 expect(received.writes).toBe(0);expect(received.received.every((r:any)=>r.method==='GET'&&r.path==='/security/public.js'&&!r.cookie&&!r.authorization)).toBe(true);
 expect(popups).toHaveLength(0);expect(downloads).toHaveLength(0);expect(page.url()).toContain('/reviews/job');
 await writeFile(info.outputPath('authenticated-effects.json'),JSON.stringify({positive,result,received,browser:context.browser()?.version()},null,2));
});

test('HP-008,013 parsing and excluded local dependencies never fall back to Cortex routes',async({page},info)=>{
 const requests:string[]=[];page.on('request',r=>{if(r.url().includes('fallback-'))requests.push(r.url())});
 const document=`<!doctype html><html><head><link rel="stylesheet" href="fallback-style.css"><style>body{background-image:url(fallback-css.png)}</style><script src="fallback-script.js"></script></head><body><img src="fallback-image.svg"><picture><source srcset="fallback-picture.svg 1x"><img src="fallback-missing.png"></picture><iframe src="fallback-frame.html"></iframe><script>new Image().src='fallback-dynamic.png';document.body.dataset.ready='yes'</script></body></html>`;
 const {open,scope}=await fixture(page,{'page.html':document},'story');await open('page.html');const f=page.frameLocator('iframe');await expect(f.locator('body')).toHaveAttribute('data-ready','yes');
 await expect(scope.getByText('Some images could not be prepared')).toBeVisible();
 // Give resource requests a deterministic completion fence in the current frame.
 await f.locator('body').evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
 expect(requests).toEqual([]);
 await writeFile(info.outputPath('excluded-requests.json'),JSON.stringify({requests,base:await f.locator('body').evaluate(()=>document.baseURI)}));
});

test('HP-016,017 source and replacement destroy contexts; hostile messages cannot revive them',async({page})=>{
 const source='<!doctype html><body><h1>Old frame</h1><script>setInterval(()=>parent.postMessage({type:"ready",generation:1,content:"revived"},"*"),10)</script></body>';
 const {open,scope}=await fixture(page,{'old.html':source,'new.html':'<!doctype html><h1>Current frame</h1>'},'review');await open('old.html');await expect(page.frameLocator('iframe').getByRole('heading',{name:'Old frame'})).toBeVisible();
 const old=page.frames().find(f=>f.url().startsWith('blob:'))!;
 await scope.getByRole('radio',{name:'Source',exact:true}).focus();await page.keyboard.press('Space');await expect(page.locator('iframe')).toHaveCount(0);expect(old.isDetached()).toBe(true);
 await page.evaluate(()=>{for(const data of [null,{},'ready',{type:'ready',generation:1,content:'revived'}])window.postMessage(data,'*')});await expect(page.locator('iframe')).toHaveCount(0);
 await open('new.html');await expect(page.frameLocator('iframe').getByRole('heading',{name:'Current frame'})).toBeVisible();await expect(page.locator('iframe')).toHaveCount(1);
});
