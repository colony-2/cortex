// Disposable local HTTP servers; no production data or JobDB access.
import https from 'node:https';
import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { writeFile, mkdir } from 'node:fs/promises';
const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const evidence = process.env.EVIDENCE_DIR || '/tmp/html-preview-boundary';
await mkdir(evidence, { recursive: true });
execFileSync('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', `${evidence}/key.pem`, '-out', `${evidence}/cert.pem`, '-days', '1', '-subj', '/CN=localhost'], { stdio: 'ignore' });
const tls = { key: readFileSync(`${evidence}/key.pem`), cert: readFileSync(`${evidence}/cert.pem`) };
const received = [];
let mutations = 0;
const servers = [];
async function listen(handler) {
  const server = https.createServer(tls, handler);
  servers.push(server);
  await new Promise(resolve => server.listen(0, '0.0.0.0', resolve));
  return server.address().port;
}
const apiPort = await listen(async (req, res) => {
  let body = '';
  for await (const chunk of req) body += chunk;
  const item = { path: req.url, method: req.method, origin: req.headers.origin || '', cookie: req.headers.cookie || '', authorization: req.headers.authorization || '', body };
  received.push(item);
  // Deliberately no Access-Control-Allow-Origin: a CORS failure must not pass as isolation.
  if (req.method === 'OPTIONS') { res.writeHead(204).end(); return; }
  if (req.url.startsWith('/protected')) {
    const signedIn = item.cookie.includes('colony2:user:email=') || item.cookie.includes('fixture-cross-site=');
    if (!signedIn) { res.writeHead(401).end('sign in'); return; }
    if (req.method === 'POST') mutations++;
    res.end('protected-fixture-value');
    return;
  }
  // Cortex cancellation accepts an empty POST and does not check a browser principal.
  if (req.url.startsWith('/api/projects/fixture/jobs/disposable/cancel') && req.method === 'POST') {
    mutations++;
    res.writeHead(204).end(); return;
  }
  res.end('fixture');
});
const api = `https://localhost:${apiPort}`;
const policySource = readFileSync(new URL('../../../src/components/review/htmlPreparation.ts', import.meta.url), 'utf8');
const policy = /export const INTERACTIVE_POLICY = "([^"]+)";/.exec(policySource)?.[1];
assert.ok(policy, 'Read the implemented frame policy rather than maintaining a second copy');
const attack = `<script>
(async () => {
 const results = {};
 try { parent.document.body; results.parent = 'read'; } catch { results.parent = 'denied'; }
 try { localStorage.getItem('fixture'); results.storage = 'read'; } catch { results.storage = 'denied'; }
 for (const path of ['/protected', '/api/projects/fixture/jobs/disposable/cancel']) {
  try { const r = await fetch('${api}' + path, { method: 'POST', credentials: 'include', mode: 'no-cors', body: '' }); results[path] = r.type; }
  catch (e) { results[path] = e.name; }
 }
 parent.postMessage({probe: true, results}, '*');
})();
</script>`;
const externalPort = await listen((req, res) => {
  res.setHeader('Content-Type', 'text/html');
  // Cooperation makes iframe[csp] enforce its required policy on a remote document.
  res.setHeader('Allow-CSP-From', '*');
  res.end(attack);
});
const external = `https://127.0.0.2:${externalPort}`;
const hostPort = await listen((req, res) => {
  res.setHeader('Content-Type', 'text/html');
  res.end('<!doctype html><title>HTML preview isolation probe</title><h1>Disposable HTML preview boundary fixture</h1><div id="frames"></div>');
});
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH, headless: true, args: ['--no-sandbox'] });
const report = { browser: browser.version(), api, external, policy, cases: [], received };
try {
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  await context.addCookies([
    {name: 'colony2:user:email', value: 'fixture%40example.test', url: api, sameSite: 'Lax'},
    {name: 'fixture-cross-site', value: 'disposable', url: api, sameSite: 'None', secure: true},
  ]);
  const page = await context.newPage();
  await page.goto(`https://localhost:${hostPort}`);
  // Host controls use the same origin as API by port-independent cookie scope,
  // but perform requests from the API origin so protected reads are actually visible.
  await page.goto(api);
  const positive = await page.evaluate(async () => ({ read: await (await fetch('/protected')).text(), write: (await fetch('/protected', { method: 'POST' })).status }));
  assert.equal(positive.read, 'protected-fixture-value');
  assert.equal(positive.write, 200);
  assert.equal(mutations, 1);
  report.positive = positive;
  report.cookieConfiguration = await context.cookies();
  await page.goto(`https://localhost:${hostPort}`);
  for (const scenario of [
    {name:'sandbox', credentialless:false, meta:false, required:false, navigate:false},
    {name:'credentialless', credentialless:true, meta:false, required:false, navigate:false},
    {name:'meta-connect-none', credentialless:true, meta:true, required:false, navigate:false},
    {name:'meta-navigation', credentialless:true, meta:true, required:false, navigate:true},
    {name:'required-csp-direct', credentialless:true, meta:false, required:true, navigate:false},
    {name:'required-csp-blob', credentialless:true, meta:false, required:true, navigate:false, blob:true},
    {name:'required-csp-navigation', credentialless:true, meta:true, required:true, navigate:true},
  ]) {
    const start = received.length, before = mutations;
    const result = await page.evaluate(({ scenario, attack, external, policy }) => new Promise(resolve => {
      const frame = document.createElement('iframe');
      frame.title = scenario.name;
      frame.sandbox = 'allow-scripts';
      frame.referrerPolicy = 'no-referrer';
      frame.credentialless = scenario.credentialless;
      if (scenario.required) frame.setAttribute('csp', policy);
      const content = scenario.navigate ? `<script>location.href=${JSON.stringify(external)}<\/script>` : attack;
      const source = (scenario.meta ? `<meta http-equiv="Content-Security-Policy" content="${policy}">` : '') + content;
      let objectURL;
      if (scenario.blob) { objectURL = URL.createObjectURL(new Blob([source], {type:'text/html'})); frame.src = objectURL; }
      else frame.srcdoc = source;
      const finish = result => { window.removeEventListener('message', handler); clearTimeout(timer); frame.remove(); if(objectURL) URL.revokeObjectURL(objectURL); resolve(result); };
      const handler = event => { if(event.source === frame.contentWindow && event.data?.probe) finish(event.data.results); };
      window.addEventListener('message', handler);
      const timer = setTimeout(() => finish({ timeout: true }), 3000);
      document.querySelector('#frames').append(frame);
    }), {scenario, attack, external, policy});
    report.cases.push({ ...scenario, result, writes: mutations - before, received: received.slice(start) });
  }
  assert.equal(report.cases[0].writes > 0, true, 'unrestricted candidate must expose the ambient write');
  assert.equal(report.cases[1].writes > 0, true, 'credentialless must still expose unauthenticated mutation');
  assert.equal(report.cases[2].writes, 0, 'connect-none blocks direct fetch');
  assert.equal(report.cases[3].writes > 0, true, 'navigation escapes meta policy');
  assert.equal(report.cases.at(-1).writes, 0, 'required CSP must survive remote navigation');
  assert.equal(report.cases.at(-1).result.parent, 'denied');
    assert.equal(report.cases.find(c => c.name === 'required-csp-blob').writes, 0);
  assert.equal(report.cases.find(c => c.name === 'required-csp-blob').result['/protected'], 'TypeError');
  report.support = await page.evaluate(() => ({ csp: 'csp' in HTMLIFrameElement.prototype, credentialless: 'credentialless' in HTMLIFrameElement.prototype }));
  await page.evaluate(cases => { const summary = document.createElement('pre'); summary.textContent = JSON.stringify(cases.map(c => ({case: c.name, writes: c.writes, results: c.result})), null, 2); document.body.append(summary); }, report.cases);
  report.hostState = await page.evaluate(() => ({ url: location.href, body: document.body.innerHTML, headings: [...document.querySelectorAll('h1')].map(el => ({text:el.textContent, display:getComputedStyle(el).display, visibility:getComputedStyle(el).visibility, box:el.getBoundingClientRect().toJSON()})) }));
  console.log(JSON.stringify(report.hostState));
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  await page.screenshot({path: `${evidence}/boundary-probe.png`, fullPage: true});
} finally {
  await writeFile(`${evidence}/boundary-results.json`, JSON.stringify(report, null, 2));
  await browser.close();
  for (const server of servers) await new Promise(resolve => server.close(resolve));
}
console.log(JSON.stringify(report, null, 2));
