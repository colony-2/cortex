import { openCompose } from './helpers/inbox';
import { expect, type Locator, type TestInfo } from '@playwright/test';
import { writeFileSync } from 'node:fs';

async function attachJSON(info: TestInfo, name: string, value: unknown) {
  const path = info.outputPath(`${name}.json`);
  writeFileSync(path, JSON.stringify(value, null, 2));
  await info.attach(name, { path, contentType: 'application/json' });
}

import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import { mockCells, jobIdentity } from './helpers/cells';

test.beforeEach(async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'composer@example.com', url: baseURL! }]);
  await page.addInitScript(() => {
    (window as any).EventSource = class { close() {} addEventListener() {} removeEventListener() {} };
  });
  await page.route('**/api/projects', route => route.fulfill({ json: [{ ...jobIdentity, tenant_id: 'composer' }] }));
  await mockCells(page, ['platform']);
  await page.route('**/user-inputs/pending', route => route.fulfill({ json: [] }));
  await page.route(/\/api\/projects\/composer\/jobs(?:\?.*)?$/, route => route.fulfill({ json: { jobs: [] } }));
});

test('CX-01 CX-02 CX-03 desktop size and phone takeover preserve one draft across breakpoints', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto('/project/composer/jobs');
  await openCompose(page);
  const dialog = page.getByRole('region', { name: 'New message' });
  await expect(dialog).toBeVisible();
  expect((await dialog.boundingBox())!.width).toBe(560);
  await expect(dialog.getByRole('radio')).toHaveCount(0);
  const prompt = dialog.getByLabel('Message', { exact: true });
  await prompt.fill('Build a clearer release dashboard.\n\nShow the current release, recent failures, and the next action.');
  await dialog.getByRole('combobox', { name: 'Recipient cell', exact: true }).click();
  await page.getByRole('option', { name: 'platform', exact: true }).click();
  await dialog.locator('input[type=file]').setInputFiles({ name: 'requirements.md', mimeType: 'text/markdown', buffer: Buffer.from('Dashboard requirements') });
  await expect(dialog.getByRole('button', { name: 'Remove requirements.md' })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('desktop-composer.png'), animations: 'disabled' });
  for (const width of [320, 390, 767, 768, 1440]) {
    await page.setViewportSize({ width, height: width === 1440 ? 900 : 844 });
    await expect.poll(() => dialog.evaluate(el => { const r=el.getBoundingClientRect(); return r.x>=0 && r.right<=innerWidth && r.y>=0 && r.bottom<=innerHeight+1; })).toBe(true);
    if(width<768)await expect.poll(()=>dialog.boundingBox()).toEqual({x:0,y:0,width,height:844});
    await expect(dialog.getByRole('button', { name: 'Send', exact: true })).toBeInViewport();
    await expect(prompt).toContainText(/Build a clearer release dashboard/);
    await expect(dialog.getByRole('button', { name: 'Remove requirements.md' })).toHaveCount(1);
    await expect(dialog.locator('.ant-select-selection-item')).toContainText('platform');
    const layout = await page.evaluate(() => ({
      width: innerWidth, scrollWidth: document.documentElement.scrollWidth,
      outside: [...document.querySelectorAll('body *')].flatMap(el => {
        const r = el.getBoundingClientRect(), css = getComputedStyle(el);
        return r.width && (r.x < 0 || r.right > innerWidth) && css.visibility !== 'hidden'
          ? [{ tag: el.tagName, className: el.className, x: r.x, right: r.right, width: r.width }] : [];
      }),
    }));
    if (layout.scrollWidth > layout.width) await attachJSON(testInfo, `overflow-${layout.width}`, layout);
    expect(layout.scrollWidth, `Document overflow at ${layout.width}px`).toBeLessThanOrEqual(layout.width);
    if (width < 768) {
      await expect(dialog).toHaveCSS('border-radius', '0px');
      await expect(dialog).toHaveCSS('box-shadow', 'none');
    }
    if (width === 390) await page.screenshot({ path: testInfo.outputPath('phone-composer.png'), animations: 'disabled' });
  }
});

test('CX-04 CX-06 long content scrolls under reachable actions and Escape restores opener focus', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 400 });
  await page.goto('/project/composer/jobs');
  await openCompose(page);
  const opener = page.getByRole('button', { name: 'Open navigation', exact: true });
  const dialog = page.getByRole('region', { name: 'New message' });
  await dialog.getByLabel('Message', { exact: true }).fill('A long prompt\n'.repeat(90));
  const footer = dialog.locator('footer');
  const header = dialog.locator('header');
  await expect(footer.getByRole('button', { name: 'Send', exact: true })).toBeInViewport();
  await expect(header).toBeInViewport();
  const body = dialog.locator('.inbox-compose-body');
  expect(await body.evaluate(el => el.scrollHeight > el.clientHeight)).toBe(true);
  await dialog.getByRole('button', { name: 'Attach files' }).focus();
  await expect(dialog.getByRole('button', { name: 'Attach files' })).toBeInViewport();
  await expect(footer).toBeInViewport();
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
  await expect(opener).toBeFocused();
});

test('simulated keyboard viewport shrink and pan retain reachable actions and the draft', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/project/composer/jobs');
  await openCompose(page);
  const dialog = page.getByRole('region', { name: 'New message' });
  await dialog.getByLabel('Message', { exact: true }).fill('Keep this draft while the keyboard is open.');
  await page.evaluate(() => {
    Object.defineProperty(visualViewport, 'height', { configurable: true, value: 400 });
    Object.defineProperty(visualViewport, 'offsetTop', { configurable: true, value: 40 });
    visualViewport!.dispatchEvent(new Event('resize'));
  });
  await expect.poll(() => dialog.boundingBox()).toEqual({ x: 0, y: 40, width: 390, height: 400 });
  await expect(dialog.getByRole('button', { name: 'Send', exact: true })).toBeInViewport();
  await expect(dialog.getByLabel('Message', { exact: true })).toContainText('Keep this draft while the keyboard is open.');
});

async function openComposer(page: import('@playwright/test').Page) {
  await page.goto('/project/composer/jobs');
  return openCompose(page, 'platform');
}
async function startImageParagraph(prompt: Locator, text: string) {
  await prompt.click();
  await prompt.pressSequentially(text);
  await prompt.press('ControlOrMeta+End');
  await prompt.press('Enter');
  await expect(prompt.locator(':scope > p')).toHaveText([text, '']);
  await expect.poll(() => prompt.evaluate(element => {
    const selection = getSelection();
    const anchor = selection?.anchorNode;
    return { collapsed: selection?.isCollapsed, anchorOffset: selection?.anchorOffset,
      inLastParagraph: !!anchor && element.lastElementChild?.contains(anchor) };
  }), { message: 'Image paste starts at a collapsed caret in the trailing paragraph' })
    .toEqual({ collapsed: true, anchorOffset: 0, inLastParagraph: true });
}

async function pasteImage(page: import('@playwright/test').Page, bad = false) {
  return page.getByRole('textbox', { name: 'Message', exact: true }).evaluate((element, bad) => {
    const transfer = new DataTransfer();
    const canvas = document.createElement('canvas'); canvas.width = 240; canvas.height = 100;
    const ctx = canvas.getContext('2d')!;
    ctx.fillStyle = '#e6f4ff'; ctx.fillRect(0, 0, 240, 100);
    ctx.fillStyle = '#1677ff'; ctx.font = '18px sans-serif'; ctx.fillText('Release → Review', 25, 55);
    const encoded = canvas.toDataURL().split(',')[1];
    const bytes = Uint8Array.from(atob(encoded), c => c.charCodeAt(0));
    transfer.items.add(new File([bad ? 'not a PNG' : bytes], 'diagram.png', { type: 'image/png' }));
    transfer.setData('text/plain', 'duplicate clipboard text');
    transfer.setData('text/html', '<p>duplicate HTML document content</p>');
    const selection = getSelection();
    (window as any).__beforeImagePaste = { html: element.innerHTML,
      anchor: selection?.anchorNode?.parentElement?.outerHTML, offset: selection?.anchorOffset, focus: selection?.focusOffset };
    element.dispatchEvent(new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: transfer }));
    return Array.from(bytes);
  }, bad);
}

test('CX-18 phone secondary formats are keyboard reachable and preserve the selected text', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('Format this selection');
  await prompt.press('ControlOrMeta+a');
  const more = dialog.getByRole('button', { name: 'More formats', exact: true });
  await expect(dialog.getByRole('button', { name: 'Heading', exact: true })).toBeHidden();
  await more.focus();
  await page.keyboard.press('Enter');
  await expect(more).toHaveAttribute('aria-expanded', 'true');
  const heading = dialog.getByRole('button', { name: 'Heading', exact: true });
  await heading.focus();
  await page.keyboard.press('Enter');
  await expect(prompt.locator('h2')).toHaveText('Format this selection');
  await page.screenshot({ path: testInfo.outputPath('phone-more-formats.png'), animations: 'disabled' });
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(more).toHaveCount(0);
  await expect(prompt.locator('h2')).toHaveText('Format this selection');
  await expect(dialog.getByRole('button', { name: 'Quote', exact: true })).toBeVisible();
});

for (const initial of ['', 'An existing draft']) {
  for (const source of ['---', '## ']) test(`CX-17 rejects ${JSON.stringify(source)} from ${initial ? 'populated' : 'empty'} rich draft without editor errors`, async ({ page }, testInfo) => {
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.stack || error.message));
    const dialog = await openComposer(page);
    const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
    if (initial) await prompt.fill(initial);
    await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
    await prompt.fill(source);
    let submissions = 0;
    await page.route('**/api/projects/composer/jobs', async route => {
      if (route.request().method() !== 'POST') return route.fallback();
      submissions++; await route.fulfill({ status: 503, json: { error: 'Must not submit formatting' } });
    });
    await dialog.getByRole('button', { name: 'Send', exact: true }).click();
    await expect(dialog.getByText('Enter a message with text or an image', { exact: true })).toBeVisible();
    // Read the source DOM rather than innerText's layout-dependent final newline.
    expect(await prompt.textContent()).toBe(source);
    expect(submissions).toBe(0);
    if (errors.length) await page.screenshot({ path: testInfo.outputPath('empty-draft-source-error.png'), animations: 'disabled' });
    expect(errors).toEqual([]);
    if (!initial && source === '## ') {
      await page.screenshot({ path: testInfo.outputPath('fresh-empty-source-validation.png'), animations: 'disabled' });
    }
  });
}

test('CX-17 validated image-only prompt submits original Markdown and bytes', async ({ page }) => {
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.click();
  const bytes = Buffer.from(await pasteImage(page));
  await expect(prompt.locator('img')).toBeVisible();
  await expect(prompt.locator('img')).toHaveCount(1);
  await expect(prompt).not.toContainText('duplicate clipboard text');
  await expect(prompt).not.toContainText('duplicate HTML document content');
  await expect(dialog.getByRole('status').filter({ hasText: 'Pasted images only; accompanying text and HTML were omitted.' })).toBeVisible();
  let body: Buffer | undefined;
  await page.route('**/api/projects/composer/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    body = route.request().postDataBuffer()!;
    await route.fulfill({ status: 503, json: { error: 'Image-only submission reached server' } });
  });
  await dialog.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(dialog.getByText('Image-only submission reached server')).toBeVisible();
  expect(body!.includes(bytes)).toBe(true);
  expect(body!.toString('utf8')).toMatch(/!\[diagram\.png\]\(pasted-[a-f\d-]+\.png\)/);
});

for (const width of [1440, 390]) test(`CX-29 shows live and undo-retained capacity at ${width}px`, async ({ page }, testInfo) => {
  await page.setViewportSize({ width, height: 900 });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('An image to review'); await prompt.press('End'); await prompt.press('Enter');
  const bytes = await pasteImage(page);
  await expect(prompt.locator('img')).toBeVisible();
  await dialog.locator('input[type=file]').setInputFiles({ name: 'brief.md', mimeType: 'text/markdown', buffer: Buffer.from('brief') });
  const capacity = dialog.getByRole('status', { name: 'Attachment capacity' });
  const size = (value: number) => value < 1024 ? `${value} B` : `${(value / 1024).toFixed(1)} KiB`;
  const total = `2 / 10 files · ${size(bytes.length + 5)} / 25 MiB used`;
  await expect(capacity).toContainText(total);
  await expect(capacity).toContainText(`Pasted images in prompt: 1 · ${size(bytes.length)}. Retained for undo: 0 · 0 B.`);
  await prompt.locator('[data-critrev-image]').click(); await prompt.press('Backspace');
  await expect(prompt.locator('img')).toHaveCount(0);
  await expect(capacity).toContainText(total);
  await expect(capacity).toContainText(`Pasted images in prompt: 0 · 0 B. Retained for undo: 1 · ${size(bytes.length)}.`);
  await capacity.scrollIntoViewIfNeeded();
  await page.screenshot({ path: testInfo.outputPath(`retained-capacity-${width}.png`), animations: 'disabled' });
  await dialog.getByRole('button', { name: 'Undo', exact: true }).click();
  await expect(prompt.locator('img')).toBeVisible();
  await expect(capacity).toContainText(total);
  await expect(capacity).toContainText(`Pasted images in prompt: 1 · ${size(bytes.length)}. Retained for undo: 0 · 0 B.`);
});

test('rich formatting saves Markdown, source mismatches retain exact text and fetch no remote images', async ({ page }, testInfo) => {
  const requests: string[] = [];
  page.on('request', request => { if (request.url().includes('never-fetch.invalid')) requests.push(request.url()); });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('Release dashboard');
  await prompt.press('ControlOrMeta+a');
  await dialog.getByRole('button', { name: 'Heading', exact: true }).click();
  await expect(prompt.locator('h2')).toHaveText('Release dashboard');
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  await expect(prompt).toHaveText('## Release dashboard');
  const source = '*alternative emphasis*\n\n![remote](https://never-fetch.invalid/private.png)\n\n<table><tr><td>keep me</td></tr></table>\n';
  await prompt.fill(source);
  await dialog.getByRole('button', { name: 'Rich text', exact: true }).click();
  await expect(dialog.getByText('This Markdown needs source editing to preserve it. Your text is unchanged.')).toBeVisible();
  await expect(prompt).toHaveText(source);
  await page.screenshot({ path: testInfo.outputPath('source-preserved.png'), animations: 'disabled' });
  expect(requests).toEqual([]);
});

for (const width of [1440, 390]) test(`pasted image survives modes, resize and retry with exact Markdown and bytes at ${width}px`, async ({ page }, testInfo) => {
  await page.setViewportSize({ width, height: 900 });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await startImageParagraph(prompt, 'Build the release dashboard.');
  const bytes = Buffer.from(await pasteImage(page));
  await expect(prompt.locator('img')).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath(width === 390 ? 'phone-rich-image.png' : 'desktop-rich-image.png'), animations: 'disabled' });
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  const source = await prompt.innerText();
  expect(source).toMatch(/^Build the release dashboard\.\n\n!\[diagram\.png\]/);
  expect(source).toMatch(/!\[diagram\.png\]\(pasted-[a-f\d-]+\.png\)/);
  expect(source).not.toContain('blob:'); expect(source).not.toContain('duplicate clipboard text');
  await dialog.getByRole('button', { name: 'Rich text', exact: true }).click();
  try { await expect(prompt.locator('img')).toBeVisible(); }
  catch (error) {
    const projected = await page.evaluate(async source => {
      const path = '/node_modules/critrev/dist/lexical/adapter.js';
      try { return (await import(path)).projectMarkdown(source); }
      catch (error) { return String(error); }
    }, source);
    await attachJSON(testInfo, 'image-mode-source', { source, projected, beforePaste: await page.evaluate(() => (window as any).__beforeImagePaste), editor: await prompt.textContent(), notice: await dialog.locator('[role=status]').allTextContents() });
    throw error;
  }
  await page.setViewportSize({ width: width === 390 ? 1440 : 390, height: 900 });
  await expect(prompt.locator('img')).toBeVisible();
  await dialog.locator('input[type=file]').setInputFiles({ name: 'brief.md', mimeType: 'text/markdown', buffer: Buffer.from('requirements') });
  const submissions: Buffer[] = [];
  await page.route('**/api/projects/composer/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    submissions.push(route.request().postDataBuffer()!);
    await route.fulfill({ status: 503, json: { error: 'Retry this exact draft' } });
  });
  for (const count of [1, 2]) {
    await dialog.getByRole('button', { name: 'Send', exact: true }).click();
    await expect(dialog.getByText(/Retry this exact draft/)).toBeVisible();
    await expect.poll(() => submissions.length).toBe(count);
    const body = submissions[count - 1];
    expect(body.includes(bytes)).toBe(true);
    const text = body.toString('utf8');
    const manifest = text.match(/name="filenames"\r\n\r\n([^\r]+)/);
    expect(manifest).not.toBeNull();
    expect(JSON.parse(manifest![1])).toEqual(['brief.md', expect.stringMatching(/^pasted-.*\.png$/)]);
    expect(text).toContain(source.replace(/\n/g, '\\n'));
    expect(text).not.toContain('blob:');
  }
});

test('invalid live image blocks submission, deleting reference allows it and retained pool consumes quota', async ({ page }) => {
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('A prompt'); await prompt.press('End'); await prompt.press('Enter');
  await pasteImage(page, true);
  await expect(dialog.getByText(/Pasted image unavailable:/).first()).toBeVisible();
  let calls = 0;
  await page.route('**/api/projects/composer/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    calls++; await route.fulfill({ status: 503, json: { error: 'Submission reached server' } });
  });
  await dialog.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(dialog.getByText(/Pasted image unavailable:/).last()).toBeVisible(); expect(calls).toBe(0);
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  await prompt.fill('Only text now');
  await dialog.locator('input[type=file]').setInputFiles(Array.from({ length: 10 }, (_, i) => ({ name: `file-${i}`, mimeType: 'text/plain', buffer: Buffer.from('x') })));
  await expect(dialog.getByText('Attach up to 10 files.')).toBeVisible();
  await dialog.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(dialog.getByText(/Submission reached server/)).toBeVisible(); expect(calls).toBe(1);
});

test('submission freezes rich editor, source mode, paste, toolbar and attachment mutation', async ({ page }) => {
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('Frozen draft');
  let release!: () => void;
  const waiting = new Promise<void>(done => { release = done; });
  await page.route('**/api/projects/composer/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    await waiting; await route.fulfill({ status: 503, json: { error: 'Unfreeze' } });
  });
  await dialog.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(prompt).toHaveAttribute('contenteditable', 'false');
  for (const name of ['Markdown source', 'Bold', 'Attach files', 'Discard draft']) await expect(dialog.getByRole('button', { name, exact: true })).toBeDisabled();
  await pasteImage(page); await expect(prompt.locator('img')).toHaveCount(0);
  release(); await expect(dialog.getByText(/Unfreeze/)).toBeVisible();
  await expect(prompt).toHaveAttribute('contenteditable', 'true');
  await expect(prompt).toHaveText('Frozen draft');
});

// Rich history must retain editor-created empty paragraphs without lossy Markdown import.
for (const width of [1440, 390]) test(`image deletion undo and redo retain a trailing empty paragraph at ${width}px`, async ({ page }, testInfo) => {
  await page.setViewportSize({ width, height: 900 });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  const text = 'Build the release dashboard.';
  const paragraphs = prompt.locator(':scope > p');
  await startImageParagraph(prompt, text);
  await pasteImage(page);
  await expect(paragraphs.first()).toHaveText(text);
  await expect(paragraphs.last().locator('img')).toBeVisible();
  await prompt.locator('[data-critrev-image]').click(); await prompt.press('Backspace');
  await expect(prompt.locator('img')).toHaveCount(0);
  await expect(paragraphs).toHaveText([text, '']);
  await dialog.getByRole('button', { name: 'Undo', exact: true }).click();
  await expect(paragraphs.last().locator('img')).toBeVisible();
  await expect(paragraphs.first()).toHaveText(text);
  await dialog.getByRole('button', { name: 'Redo', exact: true }).click();
  await expect(prompt.locator('img')).toHaveCount(0);
  await expect(paragraphs).toHaveText([text, '']);
  await page.screenshot({ path: testInfo.outputPath(`redo-image-deleted-${width}.png`), animations: 'disabled' });
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  // innerText includes the source surface's final visual break; assert exported
  // Markdown at the submission boundary instead of interpreting browser layout.
  let submitted: string | undefined;
  await page.route('**/api/projects/composer/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    submitted = route.request().postDataJSON().inputs.prompt;
    await route.fulfill({ status: 503, json: { error: 'Retain history draft' } });
  });
  await dialog.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(dialog.getByText('Retain history draft')).toBeVisible();
  expect(submitted).toBe('Build the release dashboard.\n\n');
});

test('pending image references block submission and a discarded draft ignores late validation', async ({ page }) => {
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('Pending image'); await prompt.press('End'); await prompt.press('Enter');
  await page.evaluate(() => {
    const original = Blob.prototype.arrayBuffer;
    let release!: () => void;
    const held = new Promise<void>(done => { release = done; });
    (window as any).releaseImageValidation = release;
    Blob.prototype.arrayBuffer = async function() { if (this.type === 'image/png') await held; return original.call(this); };
  });
  await pasteImage(page);
  await expect(dialog.getByText('Checking pasted image…')).toBeVisible();
  let submissions = 0;
  await page.route('**/api/projects/composer/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    submissions++; await route.fulfill({ status: 503, json: { error: 'Should not submit' } });
  });
  await dialog.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(dialog.getByText(/Wait for pasted images/)).toBeVisible(); expect(submissions).toBe(0);
  await dialog.getByRole('button', { name: 'Discard draft', exact: true }).click();
  await expect(dialog).toBeHidden();
  await openCompose(page);
  await page.evaluate(() => (window as any).releaseImageValidation());
  await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toBeEmpty();
  await expect(page.locator('[data-critrev-image]')).toHaveCount(0);
  expect(submissions).toBe(0);
});

// These checks complete the approved Chromium-only acceptance gaps.
test('RC-06 RC-08 prompt errors, focus, touch sizes and simulated safe areas remain usable', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const session = await page.context().newCDPSession(page);
  await session.send('Emulation.setSafeAreaInsetsOverride', { insets: { top: 24, left: 20, right: 20, bottom: 30 } });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await dialog.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(prompt).toHaveAttribute('aria-invalid', 'true');
  const error = dialog.getByText('Enter a message with text or an image', { exact: true });
  await expect(error).toBeVisible();
  const describedBy = (await prompt.getAttribute('aria-describedby'))!.split(/\s+/);
  expect(await error.evaluate((element, ids) => ids.some(id => document.getElementById(id)?.contains(element)), describedBy)).toBe(true);
  expect(await prompt.evaluate(el => parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(16);
  for (const control of [dialog.getByRole('button', { name: 'Close composer', exact: true }), dialog.getByRole('button', { name: 'Send', exact: true }), dialog.getByRole('button', { name: 'Attach files', exact: true }), dialog.getByRole('button', { name: 'More formats', exact: true })]) {
    await control.scrollIntoViewIfNeeded();
    expect((await control.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    expect((await control.boundingBox())!.width).toBeGreaterThanOrEqual(44);
    await expect(control).toBeInViewport({ ratio: 1 });
  }
  await expect(dialog.locator('header')).toHaveCSS('padding-top', '24px');
  await expect(dialog.locator('.inbox-compose-body')).toHaveCSS('padding-left', '20px');
  await expect(dialog.locator('footer')).toHaveCSS('padding-bottom', '30px');
  await prompt.fill('Retain draft across landscape');
  await dialog.getByRole('button', { name: 'Discard draft', exact: true }).focus();
  // Compose is a nonmodal inbox surface; Tab can reach the rest of the app.
  await page.keyboard.press('Tab');
  await dialog.getByRole('button', { name: 'Bold', exact: true }).focus();
  await page.keyboard.press('Tab');
  const focused = dialog.locator(':focus-visible');
  await expect(focused).toHaveCount(1);
  expect(await focused.evaluate(el => parseFloat(getComputedStyle(el).outlineWidth))).toBeGreaterThanOrEqual(2);
  await expect(focused).toBeInViewport();
  await page.setViewportSize({ width: 844, height: 390 });
  await expect(prompt).toContainText('Retain draft across landscape');
  await expect(dialog.getByRole('button', { name: 'Send', exact: true })).toBeInViewport({ ratio: 1 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await session.detach();
});

for (const [label, markdown] of [
  ['Heading', '## Marker'], ['Bold', '**Marker**'], ['Italic', '*Marker*'],
  ['Bullet list', '- Marker'], ['Numbered list', '1. Marker'], ['Quote', '> Marker'],
  ['Inline code', '`Marker`'], ['Code block', '```\nMarker\n```'], ['Link', '[Marker](https://example.com)'],
]) test(`RC-19 ${label} exports Markdown from rich editing`, async ({ page }, testInfo) => {
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('Marker'); await prompt.press('ControlOrMeta+a');
  await dialog.getByRole('button', { name: label, exact: true }).click();
  if (label === 'Link') {
    await expect(prompt.locator('a')).toHaveAttribute('href', 'https://example.com');
    await page.screenshot({ path: testInfo.outputPath('rich-link.png'), animations: 'disabled' });
  }
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  // Source line breaks are BR nodes; innerText includes their visible newlines.
  await expect(prompt).toHaveText(markdown, { useInnerText: true });
});

for (const width of [1440, 390]) test(`RC-19 Link placeholder, undo and replacement at ${width}px`, async ({ page }, testInfo) => {
  await page.setViewportSize({ width, height: 900 });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.evaluate(element => {
    const samples: unknown[] = [];
    (window as any).__linkSelection = samples;
    for (const event of ['focusin', 'selectionchange', 'pointerdown', 'click']) document.addEventListener(event, e => {
      const selection = getSelection();
      samples.push({ event, target: (e.target as Element)?.textContent?.slice(0, 80), html: element.innerHTML,
        active: document.activeElement?.getAttribute('aria-label'), anchor: selection?.anchorNode?.nodeName,
        anchorInside: !!selection?.anchorNode && element.contains(selection.anchorNode), offset: selection?.anchorOffset,
        focus: selection?.focusNode?.nodeName, focusInside: !!selection?.focusNode && element.contains(selection.focusNode),
        focusOffset: selection?.focusOffset, selected: selection?.toString() });
    }, true);
  });
  // Focus alone does not establish a browser editing range. Put the caret in
  // the empty paragraph as a user would before invoking selection formatting.
  await prompt.click();
  await expect.poll(() => prompt.evaluate(element => {
    const selection = getSelection();
    return !!selection?.anchorNode && !!selection.focusNode && selection.isCollapsed
      && element.contains(selection.anchorNode) && element.contains(selection.focusNode);
  }), { message: 'Link insertion starts with a collapsed caret inside Message' }).toBe(true);
  if (width === 390) await dialog.getByRole('button', { name: 'More formats', exact: true }).click();
  await dialog.getByRole('button', { name: 'Link', exact: true }).click();
  try { await expect(prompt.locator('a')).toHaveText('link text'); }
  catch (error) {
    await attachJSON(testInfo, 'link-selection', await page.evaluate(() => (window as any).__linkSelection));
    throw error;
  }
  await expect(prompt.locator('a')).toHaveAttribute('href', 'https://example.com');
  expect(await page.evaluate(() => getSelection()?.toString())).toBe('link text');
  await dialog.getByRole('button', { name: 'Undo', exact: true }).click();
  await expect(prompt).toHaveText('');
  await expect(prompt.locator('a')).toHaveCount(0);
  await dialog.getByRole('button', { name: 'Redo', exact: true }).click();
  await expect(prompt.locator('a')).toHaveText('link text');
  expect(await page.evaluate(() => getSelection()?.toString())).toBe('link text');
  await page.keyboard.type('Guide');
  await expect(prompt.locator('a')).toHaveText('Guide');
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  await expect(prompt).toHaveText('[Guide](https://example.com)');
});

test('RC-19 source Link keeps a literal bracketed label as Markdown text', async ({ page }) => {
  const dialog = await openComposer(page);
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('Read [guide]'); await prompt.press('ControlOrMeta+a');
  await dialog.getByRole('button', { name: 'Link', exact: true }).click();
  const ast = unified().use(remarkParse).parse(await prompt.innerText());
  expect(ast).toMatchObject({ children: [{ type: 'paragraph', children: [{
    type: 'link', url: 'https://example.com', children: [{ type: 'text', value: 'Read [guide]' }],
  }] }] });
});

test('RC-19 literal review delimiters stay ordinary authored text', async ({ page }) => {
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  const literal = '{++new++} {--old--} {>>comment<<} {==highlight==}';
  await prompt.fill(literal);
  await expect(prompt).toHaveText(literal);
  await expect(prompt.locator('ins, del, mark, [data-critrev-suggestion], [data-critrev-comment]')).toHaveCount(0);
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  // Escaped Markdown punctuation must still render as the same ordinary text.
  expect(unified().use(remarkParse).parse(await prompt.innerText())).toMatchObject({
    children: [{ type: 'paragraph', children: [{ type: 'text', value: literal }] }],
  });
});

for (const mime of ['image/png', 'image/jpeg', 'image/gif', 'image/webp']) test(`RC-23 ${mime} paste previews and submits its original bytes`, async ({ page }) => {
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.click();
  const original = Buffer.from(await prompt.evaluate((element, mime) => {
    const canvas = document.createElement('canvas'); canvas.width = 24; canvas.height = 10;
    canvas.getContext('2d')!.fillRect(0, 0, 24, 10);
    const encoded = mime === 'image/gif' ? 'R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7' : canvas.toDataURL(mime).split(',')[1];
    const bytes = Uint8Array.from(atob(encoded), c => c.charCodeAt(0));
    const transfer = new DataTransfer();
    transfer.items.add(new File([bytes], `sample.${mime.split('/')[1]}`, { type: mime }));
    element.dispatchEvent(new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: transfer }));
    return Array.from(bytes);
  }, mime));
  await expect(prompt.locator('img')).toBeVisible();
  expect(await prompt.locator('img').evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0)).toBe(true);
  let body: Buffer | undefined;
  await page.route('**/api/projects/composer/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    body = route.request().postDataBuffer()!;
    await route.fulfill({ status: 503, json: { error: 'Raster bytes inspected' } });
  });
  await dialog.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(dialog.getByText('Raster bytes inspected')).toBeVisible();
  expect(body!.includes(original)).toBe(true);
  expect(body!.toString('utf8')).toContain(`Content-Type: ${mime}`);
});

test('RC-25 HTML-only remote image paste makes no image request and text paste remains editable', async ({ page }) => {
  const requests: string[] = [];
  page.on('request', request => { if (request.url().includes('never-fetch.invalid')) requests.push(request.url()); });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.click();
  await prompt.evaluate(element => {
    const transfer = new DataTransfer(); transfer.setData('text/html', '<img src="https://never-fetch.invalid/clipboard.png">');
    element.dispatchEvent(new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: transfer }));
  });
  await expect(prompt.locator('img')).toHaveCount(0);
  await prompt.evaluate(element => {
    const transfer = new DataTransfer(); transfer.setData('text/plain', 'Editable clipboard text');
    element.dispatchEvent(new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: transfer }));
  });
  await expect(prompt).toContainText('Editable clipboard text');
  await prompt.press('End'); await prompt.pressSequentially(' and typing');
  await expect(prompt).toContainText('Editable clipboard text and typing');
  expect(requests).toEqual([]);
});

test('RC-22 image batch replaces selection in one undo step and delayed previews preserve deletion and caret', async ({ page }) => {
  await page.addInitScript(() => {
    const NativeImage = window.Image;
    (window as any).previewReleases = [];
    window.Image = class extends NativeImage {
      get src() { return super.src; }
      set src(value: string) {
        if (value.startsWith('blob:')) (window as any).previewReleases.push(() => { super.src = value; });
        else super.src = value;
      }
    };
  });
  const dialog = await openComposer(page);
  const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
  await prompt.fill('Replace this selection'); await prompt.press('ControlOrMeta+a');
  await prompt.evaluate(element => {
    const canvas = document.createElement('canvas'); canvas.width = canvas.height = 10;
    const bytes = Uint8Array.from(atob(canvas.toDataURL().split(',')[1]), c => c.charCodeAt(0));
    const transfer = new DataTransfer();
    for (const name of ['first.png', 'second.png']) transfer.items.add(new File([bytes], name, { type: 'image/png' }));
    element.dispatchEvent(new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: transfer }));
  });
  await expect(prompt.locator('[data-critrev-image]')).toHaveCount(2);
  await expect(prompt).not.toContainText('Replace this selection');
  await dialog.getByRole('button', { name: 'Undo', exact: true }).click();
  await expect(prompt.locator('[data-critrev-image]')).toHaveCount(0);
  await expect(prompt).toHaveText('Replace this selection');
  await dialog.getByRole('button', { name: 'Redo', exact: true }).click();
  await expect(prompt.locator('[data-critrev-image]')).toHaveCount(2);
  await prompt.locator('[data-critrev-image]').first().click(); await prompt.press('Backspace');
  await expect(prompt.locator('[data-critrev-image]')).toHaveCount(1);
  await prompt.press('ControlOrMeta+End'); await prompt.press('Enter'); await prompt.pressSequentially('Keep caret here');
  await page.waitForFunction(() => (window as any).previewReleases.length === 2);
  await page.evaluate(() => {
    const selection = getSelection()!;
    (window as any).savedCaret = [selection.anchorNode, selection.anchorOffset];
    (window as any).previewReleases.forEach((release: () => void) => release());
  });
  await expect(prompt.locator('img')).toBeVisible();
  await expect(prompt.locator('[data-critrev-image]')).toHaveCount(1);
  expect(await page.evaluate(() => {
    const selection = getSelection()!, [node, offset] = (window as any).savedCaret;
    return selection.anchorNode === node && selection.anchorOffset === offset;
  })).toBe(true);
  await expect(prompt).toContainText('Keep caret here');
  await dialog.getByRole('button', { name: 'Markdown source', exact: true }).click();
  await expect(prompt).not.toContainText('first.png');
  await expect(prompt).toContainText('second.png');
});

test.describe('RC-09 native desktop zoom', () => {
  test.use({ nativeZoom: true });
  test('200% zoom preserves the draft, editor focus and reachable controls', async ({ page }, testInfo) => {
    await page.goto('/project/composer/jobs');
    await setNativeZoom(page, 2);
    await openCompose(page);
    const dialog = page.getByRole('region', { name: 'New message' });
    const prompt = dialog.getByRole('textbox', { name: 'Message', exact: true });
    const text = 'A substantial prompt\n'.repeat(35);
    await prompt.fill(text);
    for (const control of [dialog.getByRole('button', { name: 'Close composer', exact: true }), dialog.getByRole('combobox', { name: 'Recipient cell', exact: true }), dialog.getByRole('button', { name: 'More formats', exact: true }), dialog.getByRole('button', { name: 'Attach files', exact: true }), dialog.getByRole('button', { name: 'Send', exact: true })]) {
      await control.focus(); await control.scrollIntoViewIfNeeded();
      await expect(control).toBeInViewport({ ratio: 1 });
    }
    await page.keyboard.press('Tab'); // Nonmodal Compose does not trap focus.
    await prompt.focus(); await prompt.press('ControlOrMeta+End');
    const caret = await prompt.evaluate(() => {
      const selection = getSelection()!, range = selection.getRangeAt(0).cloneRange();
      const node = selection.anchorNode!;
      if (node.nodeType === Node.TEXT_NODE && selection.anchorOffset > 0) range.setStart(node, selection.anchorOffset - 1);
      const rect = range.getBoundingClientRect();
      const body = document.querySelector('.inbox-compose-body')!.getBoundingClientRect();
      return { top: rect.top, bottom: rect.bottom, bodyTop: body.top, bodyBottom: body.bottom };
    });
    expect(caret.top).toBeGreaterThanOrEqual(caret.bodyTop);
    expect(caret.bottom).toBeLessThanOrEqual(caret.bodyBottom);
    await expect(prompt).toContainText('A substantial prompt');
    const layout = await page.evaluate(() => ({
      width: innerWidth, scrollWidth: document.documentElement.scrollWidth,
      outside: [...document.querySelectorAll('body *')].flatMap(el => {
        const r = el.getBoundingClientRect(), css = getComputedStyle(el);
        return r.width && (r.x < 0 || r.right > innerWidth) && css.visibility !== 'hidden'
          ? [{ tag: el.tagName, className: el.className, x: r.x, right: r.right, width: r.width }] : [];
      }),
    }));
    if (layout.scrollWidth > layout.width) await attachJSON(testInfo, `overflow-${layout.width}`, layout);
    expect(layout.scrollWidth, `Document overflow at ${layout.width}px`).toBeLessThanOrEqual(layout.width);
    await captureNativeZoom(page, testInfo.outputPath('composer-native-zoom-200.png'));
    await setNativeZoom(page, 1);
    await expect(prompt).toContainText('A substantial prompt');
  });
});
