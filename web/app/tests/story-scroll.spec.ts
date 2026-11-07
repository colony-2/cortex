import { writeFileSync } from 'node:fs';
import { expect, type Page } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { jobIdentity, mockCells } from './helpers/cells';

const longLabel = 'x'.repeat(200);
function fixture(extra = false) {
  const node = (id: string, children: any[] = []): any => ({ id, kind: 'op', title: id, path: [id], attempt: 1, status: 'completed', children });
  let deep = node(longLabel);
  for (let i = 0; i < 30; i++) deep = node(`depth-${i}`, [deep]);
  return { ...jobIdentity, job_id: 'one', status: 'running', root: { ...node('root', [deep, ...Array.from({ length: extra ? 105 : 100 }, (_, i) => node(`row-${i}`))]), kind: 'recipe' } };
}
let reads = 0;
let append = false;
test.beforeEach(async ({ page, context, baseURL }) => {
  reads = 0; append = false;
  await context.addCookies([{ name: 'colony2:user:email', value: 'scroll@example.com', url: baseURL! }]);
  await page.addInitScript(() => {
    (window as any).subscriptions = 0;
    (window as any).EventSource = class { readyState = 1; constructor() { (window as any).subscriptions++; } close() {} addEventListener() {} removeEventListener() {} };
  });
  await page.route('**/api/projects', route => route.fulfill({ json: [{ ...jobIdentity, tenant_id: 'phone' }] }));
  await mockCells(page, ['platform']);
  await page.route('**/user-inputs/pending', route => route.fulfill({ json: [] }));
  await page.route('**/api/projects/*/jobs/one', route => route.fulfill({ json: { ...jobIdentity, job_id: 'one', status: 'RUNNING', prompt: 'Scrolling fixture' } }));
  await page.route('**/api/projects/*/jobs/one/story', route => { reads++; return route.fulfill({ json: fixture(append) }); });
});
async function open(page: Page) {
  await page.goto('/project/phone/jobs/one/story');
  await page.getByRole('button', { name: 'Expand all', exact: true }).click();
  await expect(page.getByRole('treeitem')).toHaveCount(132);
}
async function geometry(page: Page) {
  const measurements = await page.evaluate(() => {
    const panel = document.querySelector('.story-tree-panel')!;
    const elements = [panel, ...panel.querySelectorAll('.ant-card-body, .story-tree-region, .ant-tree, .ant-tree-list, .ant-tree-list-holder, .ant-tree-treenode[role="treeitem"]')];
    return {
      overflowElements: [...panel.querySelectorAll('*')].filter(e => e.scrollHeight > e.clientHeight + 1).map(e => ({ name: e.className, y: e.scrollHeight-e.clientHeight, html: e.outerHTML.slice(0,150) })).slice(-15),
      horizontal: document.documentElement.scrollWidth - innerWidth,
      height: document.documentElement.scrollHeight,
      viewport: innerHeight,
      boxes: elements.map(e => ({ name: e.className, x: e.scrollWidth - e.clientWidth, y: e.scrollHeight - e.clientHeight, overflow: getComputedStyle(e).overflow, overflowX: getComputedStyle(e).overflowX, maxHeight: getComputedStyle(e).maxHeight })),
      indents: [...panel.querySelectorAll('.ant-tree-indent')].map(e => ({ indent: e.getBoundingClientRect().width, row: e.parentElement!.getBoundingClientRect().width })),
    };
  });
  writeFileSync(test.info().outputPath('geometry.json'), JSON.stringify(measurements, null, 2));
  await test.info().attach('geometry.json', { body: JSON.stringify(measurements), contentType: 'application/json' });
  expect(measurements.horizontal).toBeLessThanOrEqual(1);
  expect(measurements.height).toBeGreaterThan(measurements.viewport);
  for (const box of measurements.boxes) {
    // Revision 2 / JS-SC-001 permits exactly one horizontal owner.
    if (box.name === 'story-tree-region') {
      expect(box.x).toBeGreaterThan(1);
      expect(box.overflowX).toBe('auto');
    } else {
      expect(box.x, box.name + ' horizontal').toBeLessThanOrEqual(1);
      expect(box.overflow, box.name).not.toMatch(/auto|scroll|hidden|clip/);
    }
    expect(box.y, box.name + " vertical").toBeLessThanOrEqual(1);
    expect(box.maxHeight, box.name).toBe('none');
  }
  // JS-SC-006 quarter-width cap superseded by natural indentation.
  expect(Math.max(...measurements.indents.map(box => box.indent))).toBe(31 * 24);
  return measurements;
}
async function offsets(page: Page) {
  return page.evaluate(() => ({ document: scrollY, inner: [...document.querySelectorAll('.story-tree-panel, .story-tree-panel *')].map(e => [e.scrollLeft, e.scrollTop]) }));
}
for (const size of [{ width: 320, height: 568 }, { width: 390, height: 844 }, { width: 768, height: 1024 }, { width: 1100, height: 800 }, { width: 1101, height: 800 }, { width: 1440, height: 900 }, { width: 844, height: 390 }]) {
  test(`JS-SC-001,002,005,006,007 geometry ${size.width}x${size.height}`, async ({ page }, info) => {
    await page.setViewportSize(size); await open(page);
    const before = await geometry(page);
    const tree = await page.locator('.story-tree-panel').boundingBox();
    const details = await page.locator('.story-details-panel').boundingBox();
    if (size.width <= 1100) expect(details!.y).toBeGreaterThanOrEqual(tree!.y + tree!.height);
    else { expect(Math.abs(details!.y - tree!.y)).toBeLessThanOrEqual(1); expect(details!.height).toBeLessThan(tree!.height); }
    await expect(page.getByText(longLabel, { exact: true }).first()).toBeVisible();
    await info.attach('geometry.json', { body: JSON.stringify(before), contentType: 'application/json' });
    if ([390, 1440].includes(size.width)) await page.screenshot({ path: info.outputPath(`story-${size.width}.png`), fullPage: true });
    await page.getByRole('button', { name: 'Collapse all', exact: true }).click();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollHeight)).toBeLessThan(before.height);
  });
}
test('JS-SC-003 wheel belongs to document', async ({ page }, info) => {
  await open(page); const row = page.getByRole('region', { name: 'Execution story tree' }).getByText('row-10', { exact: true }); await row.scrollIntoViewIfNeeded();
  await row.hover(); const before = await offsets(page); await page.mouse.wheel(0, 350);
  await expect.poll(() => page.evaluate(() => scrollY)).toBeGreaterThan(before.document);
  const after = await offsets(page); expect(after.inner).toEqual(before.inner);
  await info.attach('wheel.json', { body: JSON.stringify({ before, after }), contentType: 'application/json' });
});
test.describe('touch', () => {
  test.use({ hasTouch: true, viewport: { width: 390, height: 844 } });
  test('JS-SC-004 trusted touch belongs to document', async ({ page }, info) => {
    await open(page); await page.getByRole('region', { name: 'Execution story tree' }).getByText('row-10', { exact: true }).scrollIntoViewIfNeeded();
    const before = await offsets(page); const cdp = await page.context().newCDPSession(page);
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: 180, y: 650 }] });
    for (let y = 620; y >= 250; y -= 30) {
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: 180, y }] });
      await page.waitForTimeout(20);
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await expect.poll(() => page.evaluate(() => scrollY)).toBeGreaterThan(before.document);
    const after = await offsets(page); expect(after.inner).toEqual(before.inner); await cdp.detach();
    await info.attach('touch.json', { body: JSON.stringify({ before, after }), contentType: 'application/json' });
  });
});
test('JS-SC-008,009,010,011,013,014,015 navigation and resize', async ({ page }, info) => {
  await page.setViewportSize({ width: 390, height: 844 }); await page.emulateMedia({ reducedMotion: 'reduce' }); await open(page);
  const row = page.getByRole('region', { name: 'Execution story tree' }).getByText('row-10', { exact: true }); await row.scrollIntoViewIfNeeded();
  const before = await page.evaluate(() => scrollY); await row.click();
  expect(await page.evaluate(() => scrollY)).toBe(before);
  await expect(page.locator('#selected-story-details')).not.toBeFocused();
  const selectedURL = page.url(); const count = reads; const subscriptions = await page.evaluate(() => (window as any).subscriptions);
  for (const link of await page.getByRole('link', { name: 'View selected details', exact: true }).all()) {
    const box = await link.boundingBox(); expect(box!.width).toBeGreaterThanOrEqual(44); expect(box!.height).toBeGreaterThanOrEqual(44);
    await link.click(); await expect(page.locator('#selected-story-details')).toBeFocused(); await expect(page.locator('#selected-story-details')).toBeInViewport();
    await page.getByRole('link', { name: 'Back to selected node' }).click();
    await expect(page.locator('.ant-tree input')).toBeFocused();
    await expect(page.locator('.ant-tree-treenode-active .ant-tree-node-selected')).toBeVisible();
  }
  await page.setViewportSize({ width: 1440, height: 900 }); await page.setViewportSize({ width: 390, height: 844 });
  expect(page.url()).toBe(selectedURL); expect(reads).toBe(count); expect(await page.evaluate(() => (window as any).subscriptions)).toBe(subscriptions);
  await expect(page.getByRole('treeitem')).toHaveCount(132);
  const switcher = page.locator('.ant-tree-switcher').first(); const box = await switcher.boundingBox(); expect(box!.width).toBeGreaterThanOrEqual(44); expect(box!.height).toBeGreaterThanOrEqual(44);
  await page.getByRole('link', { name: 'View selected details' }).first().click();
  await page.getByRole('link', { name: 'Back to selected node' }).click();
  const input = page.locator('.ant-tree input');
  await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter');
  await expect(page.locator('.ant-tree-node-selected')).toContainText('row-11');
  await expect(input).toBeFocused();
  const active = page.locator('.ant-tree-treenode-active .ant-tree-node-content-wrapper');
  await expect(active).toHaveCSS('outline-style', 'solid');
  await expect(active).toHaveCSS('outline-width', '2px');
  await row.click();
  await page.getByRole('button', { name: 'Collapse all', exact: true }).click();
  await page.getByRole('link', { name: 'Back to selected node' }).click();
  await expect(page.locator('#execution-story-tree')).toBeFocused();
  await info.attach('state.json', { body: JSON.stringify({ reads, subscriptions, selectedURL }), contentType: 'application/json' });
});
test('JS-SC-012 polling retains view', async ({ page }) => {
  await open(page); const row = page.getByRole('region', { name: 'Execution story tree' }).getByText('row-10', { exact: true }); await row.scrollIntoViewIfNeeded(); await row.click();
  const url = page.url(); const before = await page.evaluate(() => scrollY); const count = reads; append = true;
  await expect.poll(() => reads, { timeout: 15000 }).toBeGreaterThan(count);
  await expect(page.getByRole('treeitem')).toHaveCount(137);
  expect(page.url()).toBe(url); expect(await page.evaluate(() => scrollY)).toBe(before);
});
test.describe('zoom', () => {
  test.use({ nativeZoom: true });
  test('JS-SC-016,029 native zoom story', async ({ page }, info) => {
    await open(page); await setNativeZoom(page, 2); await geometry(page);
    const region = page.locator('#execution-story-tree');
    await region.scrollIntoViewIfNeeded();
    await region.hover({ position: { x: 100, y: 100 } });
    await page.mouse.wheel(250, 0);
    await expect.poll(() => region.evaluate(e => e.scrollLeft)).toBeGreaterThan(0);
    await page.getByRole('link', { name: 'View selected details' }).first().click(); await expect(page.locator('#selected-story-details')).toBeFocused();
    await page.getByRole('link', { name: 'Back to selected node' }).click();
    await expect(page.locator('.ant-tree input')).toBeFocused();
    await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter');
    await expect(page.locator('.ant-tree-node-selected')).toContainText('depth-29');
    await captureNativeZoom(page, info.outputPath('story-zoom.png'));
  });
});
for (const state of ['empty', 'failed', 'loading']) {
  test(`JS-SC-019 ${state} story navigation`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.route('**/api/projects/*/jobs/one/story', async route => {
      if (state === 'failed') return route.fulfill({ status: 500, body: 'Unavailable' });
      if (state === 'loading') return; // Pending request is released when the test context closes.
      return route.fulfill({ json: { ...jobIdentity, status: 'completed', root: null } });
    });
    await page.goto('/project/phone/jobs/one/story');
    if (state === 'failed') await expect(page.getByText('Failed to load job story')).toBeVisible();
    else if (state === 'empty') {
      await expect(page.getByText('The story API returned no steps for this finished job.')).toBeVisible();
      await page.getByRole('link', { name: 'View selected details' }).first().click();
      await expect(page.locator('#selected-story-details')).toBeFocused();
      await page.getByRole('link', { name: 'Back to selected node' }).click();
      await expect(page.locator('#execution-story-tree')).toBeFocused();
    } else await expect(page.locator('.ant-spin')).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1);
  });
}

for (const width of [390, 1440]) {
  test(`JS-SC-F001,F002 / JS-SC-006,009,013,026,027 deep wrapped interaction ${width}`, async ({ page }, info) => {
    await page.setViewportSize({ width, height: 900 }); await open(page);
    const tree = page.getByRole('region', { name: 'Execution story tree' });
    const leaf = tree.getByText(longLabel, { exact: true });
    const parent = tree.getByRole('treeitem').filter({ hasText: 'depth-0' });
    const switcher = parent.locator('.ant-tree-switcher');
    await switcher.click(); await expect(leaf).toHaveCount(0);
    await switcher.click(); await expect(leaf).toBeVisible();
    const switcherBox = await switcher.boundingBox();
    expect(switcherBox!.width).toBeGreaterThanOrEqual(width === 390 ? 44 : 24);
    expect(switcherBox!.height).toBeGreaterThanOrEqual(width === 390 ? 44 : 24);
    await leaf.click();
    const selectedURL = page.url();
    expect(new URL(selectedURL).searchParams.get('path')).toBe(longLabel);
    await expect(page.locator('.story-details-panel')).toContainText(longLabel);
    const labelBox = await leaf.boundingBox(); expect(labelBox!.height).toBeGreaterThan(24);
    await tree.evaluate(e => e.scrollLeft = 0);
    if (width === 390) {
      await page.getByRole('link', { name: 'View selected details' }).first().click();
      await page.getByRole('link', { name: 'Back to selected node' }).click();
    } else {
      // Enter the controller through normal tab order, without programmatic focus.
      await page.getByRole('button', { name: 'Expand all', exact: true }).click();
      await page.keyboard.press('Tab'); await page.keyboard.press('Tab'); await page.keyboard.press('Tab');
    }
    await expect(page.locator('.ant-tree input')).toBeFocused();
    const active = page.locator('.ant-tree-treenode-active .ant-tree-node-content-wrapper');
    await expect(active).toContainText(longLabel);
    await expect(active).toHaveCSS('outline-width', '2px');
    await expect(active).toHaveCSS('outline-style', 'solid');
    await expect(active).toBeInViewport();
    await page.screenshot({ path: info.outputPath(`deep-focus-${width}.png`) });
    // Immediate controller operation after horizontal reveal (F001).
    await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter');
    await expect(page.locator('.ant-tree-node-selected')).toContainText('row-0');
    await page.keyboard.press('ArrowUp'); await page.keyboard.press('Enter');
    await expect(page.locator('.ant-tree-node-selected')).toContainText(longLabel);
    // Navigate away and back, then select the wrapped row using the controller.
    await page.keyboard.press('ArrowUp'); await page.keyboard.press('Enter');
    await expect(page.locator('.ant-tree-node-selected')).toContainText('depth-0');
    await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter');
    await expect(page.locator('.ant-tree-node-selected')).toContainText(longLabel);
    expect(page.url()).toBe(selectedURL);
    await expect(page.locator('.story-details-panel')).toContainText(longLabel);
    await expect(page.locator('.ant-tree input')).toBeFocused();
    await info.attach('deep-interaction.json', { body: JSON.stringify({ width, selectedURL, labelBox, switcherBox, focus: await page.locator('.ant-tree input').evaluate(e => ({ active: document.activeElement === e, controller: e.outerHTML })) }), contentType: 'application/json' });
  });
}

async function exposeRegion(page: Page) {
  const region = page.locator('#execution-story-tree');
  await region.evaluate(e => window.scrollTo(0, e.getBoundingClientRect().top + scrollY - 40));
  return region;
}
async function swipe(page: Page, from: {x:number;y:number}, to: {x:number;y:number}) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [from] });
  for (let i = 1; i <= 12; i++) {
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: from.x+(to.x-from.x)*i/12, y: from.y+(to.y-from.y)*i/12 }] });
    await page.waitForTimeout(20);
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await cdp.detach();
}
for (const width of [390, 1440]) {
  test.describe(`horizontal ${width}`, () => {
    test.use({ hasTouch: true, viewport: { width, height: 900 } });
    test('JS-SC-003,004,021,022,024,025,028 horizontal input and page ownership', async ({ page }, info) => {
      await open(page); const region = await exposeRegion(page);
      const box = (await region.boundingBox())!;
      const x = box.x + Math.min(180, box.width-20), y = 160;
      const range = await region.evaluate(e => e.scrollWidth-e.clientWidth);
      expect(range).toBeGreaterThan(1);
      await page.mouse.move(x, y); await page.mouse.wheel(250, 0);
      await expect.poll(() => region.evaluate(e => e.scrollLeft)).toBeGreaterThan(0);
      const wheel = await offsets(page);
      // Trusted swipes to both limits; a capped loop bounds this validation.
      for (let i=0; i<10 && await region.evaluate(e => e.scrollLeft)<range-1; i++) await swipe(page, {x:box.x+box.width-20,y}, {x:box.x+20,y});
      await expect.poll(() => region.evaluate(e => e.scrollLeft)).toBeGreaterThanOrEqual(range-1);
      for (let i=0; i<10 && await region.evaluate(e => e.scrollLeft)>1; i++) await swipe(page, {x:box.x+20,y}, {x:box.x+box.width-20,y});
      await expect.poll(() => region.evaluate(e => e.scrollLeft)).toBeLessThanOrEqual(1);
      // Pan first, then verify ordinary vertical input retains its axis.
      await page.mouse.move(x,y); await page.mouse.wheel(200,0);
      await expect.poll(() => region.evaluate(e => e.scrollLeft)).toBeGreaterThan(0);
      const before = await offsets(page); await page.mouse.wheel(0,250);
      await expect.poll(() => page.evaluate(() => scrollY)).toBeGreaterThan(before.document);
      expect((await offsets(page)).inner).toEqual(before.inner);
      const touchBefore = await offsets(page);
      await swipe(page, {x,y:500}, {x,y:200});
      await expect.poll(() => page.evaluate(() => scrollY)).toBeGreaterThan(touchBefore.document);
      expect((await offsets(page)).inner).toEqual(touchBefore.inner);
      await exposeRegion(page);
      await region.evaluate(e => e.scrollLeft=0);
      // Reach region via tab order, rather than focusing its controller.
      await page.getByRole('button', {name:'Collapse all',exact:true}).focus();
      await page.keyboard.press('Tab'); await expect(region).toBeFocused();
      await expect(region).toHaveCSS('outline-style','solid');
      await page.keyboard.press('ArrowRight');
      await expect.poll(() => region.evaluate(e => e.scrollLeft)).toBeGreaterThan(0);
      const labels = await region.locator('.ant-tree-node-content-wrapper').evaluateAll(elements => elements.map(e => ({width:e.getBoundingClientRect().width, max:parseFloat(getComputedStyle(e).maxWidth), text:getComputedStyle(e).textOverflow})));
      const budget = await region.evaluate(e => Math.min(32*parseFloat(getComputedStyle(document.documentElement).fontSize), e.clientWidth-(innerWidth<=768?96:56)));
      for (const label of labels) { expect(label.width).toBeLessThanOrEqual(budget+1); expect(label.text).not.toBe('ellipsis'); }
      expect(await page.evaluate(() => scrollX)).toBe(0);
      expect(await region.evaluate(e => e.scrollHeight-e.clientHeight)).toBeLessThanOrEqual(1);
      await info.attach('horizontal.json',{body:JSON.stringify({width,range,wheel,before,touchBefore,after:await offsets(page),budget,labels}),contentType:'application/json'});
    });
    test('JS-SC-023 fitting short tree', async ({page}, info) => {
      await page.route('**/api/projects/*/jobs/one/story', route => route.fulfill({json:{...jobIdentity,job_id:'one',status:'completed',root:{id:'root',kind:'recipe',title:'root',path:['root'],attempt:1,status:'completed',children:[]}}}));
      await page.goto('/project/phone/jobs/one/story');
      const region=page.locator('#execution-story-tree'); await expect(region.getByRole('treeitem')).toHaveCount(1);
      expect(await region.evaluate(e=>e.scrollWidth-e.clientWidth)).toBeLessThanOrEqual(1);
      expect(await region.evaluate(e=>e.scrollHeight-e.clientHeight)).toBeLessThanOrEqual(1);
      await page.screenshot({path:info.outputPath(`short-${width}.png`),fullPage:true});
    });
  });
}
test.describe('native pinch', () => {
  test.use({hasTouch:true,isMobile:true,viewport:{width:390,height:844}});
  test('JS-SC-030 pinch is preserved', async ({page},info) => {
    await open(page); await exposeRegion(page);
    const cdp=await page.context().newCDPSession(page);
    const before=await page.evaluate(()=>visualViewport!.scale);
    // Chromium's synthesizePinchGesture is inert on this platform even on a
    // standalone page. Real two-contact CDP input exercises native pinch.
    await cdp.send('Input.dispatchTouchEvent', {type:'touchStart',touchPoints:[{x:150,y:250,id:0},{x:230,y:250,id:1}]});
    for (let i=1;i<=10;i++) {
      await cdp.send('Input.dispatchTouchEvent', {type:'touchMove',touchPoints:[{x:150-i*10,y:250,id:0},{x:230+i*10,y:250,id:1}]});
      await page.waitForTimeout(30);
    }
    await cdp.send('Input.dispatchTouchEvent', {type:'touchEnd',touchPoints:[]});
    await expect.poll(()=>page.evaluate(()=>visualViewport!.scale)).toBeGreaterThan(before);
    const after=await page.evaluate(()=>visualViewport!.scale);
    const actions=await page.locator('#execution-story-tree').evaluate(e=>{const result=[];for(let p:Element|null=e;p;p=p.parentElement)result.push({element:p.tagName,action:getComputedStyle(p).touchAction});return result;});
    for(const item of actions) expect(item.action).toBe('auto');
    const listeners = [];
    for (let depth=0;depth<actions.length;depth++) {
      const {result} = await cdp.send('Runtime.evaluate',{expression:'document.querySelector("#execution-story-tree")'+'.parentElement'.repeat(depth)});
      if (!result.objectId) break;
      const registered = await cdp.send('DOMDebugger.getEventListeners',{objectId:result.objectId});
      for (const listener of registered.listeners.filter(l=>['wheel','touchstart','touchmove'].includes(l.type))) {
        listeners.push({depth,type:listener.type,passive:listener.passive});
        expect(listener.passive).toBe(true);
      }
    }
    await info.attach('pinch.json',{body:JSON.stringify({before,after,actions,listeners}),contentType:'application/json'}); await cdp.detach();
  });
});

for (const size of [{width:320,height:568},{width:768,height:1024},{width:1100,height:800},{width:1101,height:800},{width:844,height:390}]) {
  test(`JS-SC-023 fitting short tree ${size.width}x${size.height}`, async ({page}) => {
    await page.setViewportSize(size);
    await page.route('**/api/projects/*/jobs/one/story',route=>route.fulfill({json:{...jobIdentity,job_id:'one',status:'completed',root:{id:'root',kind:'recipe',title:'root',path:['root'],attempt:1,status:'completed',children:[]}}}));
    await page.goto('/project/phone/jobs/one/story');
    const region=page.locator('#execution-story-tree'); await expect(region.getByRole('treeitem')).toHaveCount(1);
    expect(await region.evaluate(e=>e.scrollWidth-e.clientWidth)).toBeLessThanOrEqual(1);
    expect(await region.evaluate(e=>e.scrollHeight-e.clientHeight)).toBeLessThanOrEqual(1);
    expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  });
}

for (const width of [1280, 390]) {
  test(`JC-016 long job cell wraps between heading and prompt at ${width}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const cell_name = 'cell' + 'x'.repeat(240);
    await page.route('**/api/projects/phone/jobs/one', route => route.fulfill({ json: {
      ...jobIdentity, cell_name, job_id: 'one', title: 'Cell wrapping', prompt: 'Job prompt text', status: 'COMPLETED', store: 'ARCHIVED',
    } }));
    await page.goto('/project/phone/jobs/one/story');
    const cell = page.getByLabel('Job cell', { exact: true });
    await expect(cell).toHaveText(`Cell: ${cell_name}`);
    const heading = await page.getByRole('heading', { name: 'Job Story: Cell wrapping' }).boundingBox();
    const box = await cell.boundingBox();
    const prompt = await page.getByRole('region', { name: 'Job prompt' }).boundingBox();
    expect(box!.y).toBeGreaterThanOrEqual(heading!.y + heading!.height);
    expect(box!.y + box!.height).toBeLessThanOrEqual(prompt!.y);
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    const dimensions = await cell.evaluate(element => {
      const range = document.createRange();
      range.selectNodeContents(element);
      return { client: element.clientWidth, scroll: element.scrollWidth,
        lines: new Set(Array.from(range.getClientRects(), rect => Math.round(rect.top))).size };
    });
    expect(dimensions.scroll).toBeLessThanOrEqual(dimensions.client + 1);
    expect(dimensions.lines).toBeGreaterThan(1);
    await page.screenshot({ path: testInfo.outputPath(`job-cell-${width}.png`), fullPage: true });
  });
}
