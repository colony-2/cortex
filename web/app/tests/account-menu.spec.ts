import { test, expect } from '@playwright/test';
import { mkdir } from 'node:fs/promises';
import { accountUIBuildEnvironment } from './helpers/uiBuild';
const screenshotDir = process.env.CORTEX_SCREENSHOT_DIR;
const email = 'a.very.long.account.address.for.cortex.build.reports.and.support@example.com';
const metadata = { libraries: { c2j: { module: 'github.com/colony-2/c2j', version: 'v7.8.9', commit: '2222222222222222222222222222222222222222' }, jobdb: { module: 'github.com/colony-2/jobdb', version: 'v4.5.6', commit: '3333333333333333333333333333333333333333' } } };
test.beforeEach(async ({ context, baseURL }) => {
 await context.addCookies([{ name: 'colony2:user:email', value: email, url: baseURL! }]);
});
test('account menu, metadata recovery, identity and compact layout', async ({ page, context }, info) => {
 const shot = async (name: string) => { if (screenshotDir && info.project.name === 'embedded') { await mkdir(screenshotDir, { recursive: true }); if (await page.getByRole('dialog').count()) await expect(page.getByRole('dialog').locator(':focus')).toHaveCount(1); await page.screenshot({ path: `${screenshotDir}/${name}.png`, animations: 'disabled' }); } };
 await page.goto('/project/1/jobs');
 const account = page.getByRole('button', { name: 'User account' });
 await account.click();
 await expect(page.getByText(email, { exact: true })).toBeVisible();
 expect(await page.getByText(email, { exact: true }).evaluate(el => getComputedStyle(el).userSelect)).toBe('text');
 await expect(page.getByRole('menuitem', { name: 'Logout' })).toBeVisible();
 await shot('menu');
 await page.getByRole('menuitem', { name: 'About', exact: true }).click();
 const dialog = page.getByRole('dialog');
 // First load uses the real API's linked metadata.
 await expect(dialog.getByText('Loading…')).toHaveCount(0);
 await expect(dialog.getByText(accountUIBuildEnvironment.CORTEX_UI_COMMIT)).toBeVisible();
 await expect(dialog.getByText(accountUIBuildEnvironment.CORTEX_UI_VERSION)).toBeVisible();
 await shot('about');
 await expect(page.getByRole('dialog').locator(':focus')).toHaveCount(1);await page.keyboard.press('Escape');await expect(dialog).not.toBeVisible();await expect(account).toBeFocused();
 await page.route('**/api/build-info', route => route.fulfill({ json: metadata }));
 await account.press('Enter');
 await page.getByRole('menuitem', { name: 'About', exact: true }).click();
 await expect(dialog.getByText('v7.8.9')).toBeVisible();
 await expect(dialog.getByText('2222222222222222222222222222222222222222')).toBeVisible();
 await expect(dialog.getByText(accountUIBuildEnvironment.CORTEX_UI_COMMIT)).toBeVisible();
 // Modal traps focus in both directions.
 for (const key of [...Array(8).fill('Tab'), ...Array(8).fill('Shift+Tab')]) { await page.keyboard.press(key); expect(await page.evaluate(() => !!document.activeElement?.closest('.ant-modal'))).toBeTruthy(); }
 await expect(page.getByRole('dialog').locator(':focus')).toHaveCount(1);await page.keyboard.press('Escape');
 for (const failure of ['404', 'malformed', 'network']) {
  await page.route('**/api/build-info', route => failure === 'network' ? route.abort() : route.fulfill({ status: failure === '404' ? 404 : 200, json: {} }));
  await account.click();await page.getByRole('menuitem', { name: 'About', exact: true }).click();
  await expect(dialog.getByText('Could not load library build information.')).toBeVisible();
  await expect(dialog.getByText(accountUIBuildEnvironment.CORTEX_UI_COMMIT)).toBeVisible();
  if (failure === '404') await shot('metadata-error');
  await page.route('**/api/build-info', route => route.fulfill({ json: metadata }));
  await dialog.getByRole('button', { name: 'Retry' }).click();await expect(dialog.getByText('v7.8.9')).toBeVisible();
  await expect(page.getByRole('dialog').locator(':focus')).toHaveCount(1);await page.keyboard.press('Escape');
 }
 await page.setViewportSize({ width: 390, height: 844 });
 await expect(page.getByRole('searchbox', { name: 'Tenant ID' })).toBeVisible();
 await account.click();await expect(page.getByText(email, { exact: true })).toBeVisible();
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
 await page.getByRole('menuitem', { name: 'About', exact: true }).click();await expect(dialog.getByText('v7.8.9')).toBeVisible();
 await shot('compact-about');
 expect(await dialog.evaluate(el => el.getBoundingClientRect().right <= innerWidth)).toBeTruthy();
 await expect(page.getByRole('dialog').locator(':focus')).toHaveCount(1);await page.keyboard.press('Escape');
 await page.getByRole('searchbox', { name: 'Tenant ID' }).fill('2');await page.getByRole('button', { name: 'Use', exact: true }).click();await expect(page).toHaveURL(/project\/2\/jobs/);
 await page.setViewportSize({ width: 1440, height: 900 });
 await account.click();await page.getByRole('menuitem', { name: 'Logout' }).click();
 await expect(page.getByPlaceholder('Email address')).toBeVisible();await expect(dialog).not.toBeVisible();
 expect((await context.cookies()).some(c => c.name === 'colony2:user:email')).toBeFalsy();
 await shot('logged-out');
 await page.getByPlaceholder('Email address').fill('second@example.com');await page.getByRole('button', { name: 'Continue' }).click();
 await account.click();await expect(page.getByText('second@example.com', { exact: true })).toBeVisible();await expect(page.getByText(email, { exact: true })).not.toBeVisible();
});
test('pending requests do not block logout and late responses stay closed', async ({ page }) => {
 let finish!: () => void;
 await page.route('**/api/build-info', async route => {await new Promise<void>(resolve => {finish=resolve;});await route.fulfill({ json: metadata }).catch(() => {});});
 await page.goto('/');const account=page.getByRole('button', {name:'User account'});
 await account.click();await page.getByRole('menuitem',{name:'About',exact:true}).click();await expect(page.getByText('Loading…')).toHaveCount(4);
 await expect(page.getByRole('dialog').locator(':focus')).toHaveCount(1);await page.keyboard.press('Escape');await account.click();await page.getByRole('menuitem',{name:'Logout'}).click();
 await expect(page.getByPlaceholder('Email address')).toBeVisible();finish();await expect(page.getByRole('dialog')).not.toBeVisible();
});

test('keyboard menu navigation and Escape return focus', async ({ page }) => {
 await page.goto('/');
 const account=page.getByRole('button',{name:'User account'});
 await account.focus();await account.press('ArrowDown');
 await expect(page.getByRole('menu')).toHaveCount(2);
 await expect(page.getByRole('menuitem',{name:'About',exact:true})).toBeVisible();
 // Wait for the dropdown's asynchronous autofocus before navigating its items.
 await expect(page.locator('.account-dropdown :focus')).toHaveCount(1);
 await page.keyboard.press('ArrowDown');
 await expect(page.locator('.account-dropdown [role="menuitem"]:focus')).toHaveCount(1);
 await page.keyboard.press('Escape');await expect(account).toHaveAttribute('aria-expanded','false');await expect(account).toBeFocused().catch(async error => {
  console.log('Account focus after Escape', await page.evaluate(() => ({ active: document.activeElement?.outerHTML, documentFocused: document.hasFocus() })));
  throw error;
 });
});
