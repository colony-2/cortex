// Download responses are served over HTTP: Chromium download requests can
// bypass page.route even when preview fetches are intercepted.
import { createServer } from 'node:http';
const files = new Map();
const probes = [];
const security = {received:[], writes:0};
createServer(async (request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1:15175');
  if (url.pathname === '/__security/reset') {security.received.length=0;security.writes=0;response.end('ok');return;}
  if (url.pathname === '/__security') {response.setHeader('Content-Type','application/json');response.end(JSON.stringify(security));return;}
  if (url.pathname.startsWith('/security/')) {
    security.received.push({path:url.pathname,method:request.method,cookie:request.headers.cookie||'',authorization:request.headers.authorization||'',origin:request.headers.origin||''});
    if (url.pathname === '/security/redirect') {response.writeHead(307,{Location:'/security/write'});response.end();return;}
    const signedIn=(request.headers.cookie||'').includes('colony2:user:email=');
    if ((url.pathname === '/security/protected' || url.pathname.endsWith('.js')) && !signedIn) {response.writeHead(401);response.end('Sign in');return;}
    if(url.pathname === '/security/protected' && request.method === 'POST') security.writes++;
    if(url.pathname === '/security/write' && request.method === 'POST') security.writes++;
    response.setHeader('Content-Type',url.pathname.endsWith('.js')?'text/javascript':'text/plain');
    response.end(url.pathname.endsWith('.js')?'window.resourceLoaded=true':'protected-fixture-value');return;
  }
  if (url.pathname === '/public.css') { response.writeHead(200, {'Content-Type':'text/css'}); response.end('h1{color:rgb(120,30,150)}'); return; }
  if (url.pathname === '/public.js') { response.writeHead(200, {'Content-Type':'text/javascript'}); response.end('window.publicScript="public script loaded"'); return; }
  if (url.pathname === '/__files' && request.method === 'POST') {
    let body = ''; for await (const chunk of request) body += chunk;
    for (const [name, value] of Object.entries(JSON.parse(body))) files.set(name, Buffer.from(value, 'base64'));
    response.end('ok'); return;
  }
  if (url.pathname === '/__probes') { response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify(probes)); return; }
  if (url.pathname.includes('probe-')) {
    probes.push(url.pathname);
    if(url.pathname.includes('redirect')) { response.writeHead(302,{location:'/api/probe-redirected'}); response.end(); return; }
    response.end('resource sentinel'); return;
  }
  const name = url.searchParams.get('document_id') || decodeURIComponent(url.pathname.split('/artifacts/')[1] || '');
  const value = files.get(name);
  if (!value) { response.writeHead(404); response.end('unavailable'); return; }
  response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Disposition': `attachment; filename*=UTF-8''${encodeURIComponent(name)}` });
  response.end(value);
}).listen(15175, '127.0.0.1');
