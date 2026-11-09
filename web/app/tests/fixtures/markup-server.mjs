// Download responses are served over HTTP: Chromium download requests can
// bypass page.route even when preview fetches are intercepted.
import { createServer } from 'node:http';
const files = new Map();
const probes = [];
createServer(async (request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1:15175');
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
