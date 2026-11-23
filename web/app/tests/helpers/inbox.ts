import { expect, type Page } from '@playwright/test';

export async function storyAction(page: Page, name: string | RegExp) {
  const disclosure = page.locator('.inbox-story-more');
  if (!await disclosure.evaluate(node => (node as HTMLDetailsElement).open)) await disclosure.locator(':scope > summary').click();
  return disclosure.getByRole('button', { name: name === 'Refresh' ? /Refresh$/ : name, exact: typeof name === 'string' && name !== 'Refresh' });
}
export async function storyLink(page: Page, name: string) {
  const disclosure = page.locator('.inbox-story-more');
  await expect(disclosure).toBeAttached();
  if (!await disclosure.evaluate(node => (node as HTMLDetailsElement).open)) await disclosure.locator(':scope > summary').click();
  const link = disclosure.getByRole('link', { name, exact: true });
  await expect(link).toBeVisible();
  return link;
}
export async function originalRequest(page: Page) {
  const disclosure = page.locator('.inbox-original');
  await expect(disclosure).toBeAttached();
  if (!await disclosure.evaluate(node => (node as HTMLDetailsElement).open)) await disclosure.locator(':scope > summary').click();
  return page.getByRole('region', { name: 'Job prompt', exact: true });
}
export async function expandHistory(page: Page) {
  for (const selector of ['.inbox-history-middle', '.inbox-history-message']) {
    for (const disclosure of await page.locator(selector).all()) {
      if (!await disclosure.evaluate(node => (node as HTMLDetailsElement).open)) await disclosure.locator(':scope > summary').click();
    }
  }
}
export async function inboxFolder(page: Page, name: string) {
  await expect(page.getByRole('banner')).toBeVisible();
  const drawer = page.getByRole('button', { name: 'Open navigation', exact: true });
  if (await drawer.isVisible()) await drawer.click();
  await page.getByRole('navigation', { name: 'Inboxes' }).getByRole('link', { name, exact: true }).click();
}
export async function openCompose(page: Page, recipient?: string) {
  await expect(page.getByRole('banner')).toBeVisible();
  const drawer = page.getByRole('button', { name: 'Open navigation', exact: true });
  if (await drawer.isVisible()) await drawer.click();
  await page.getByRole('button', { name: 'Compose', exact: true }).click();
  const composer = page.getByRole('region', { name: 'Compose job', exact: true });
  await expect(composer).toBeVisible();
  if (recipient) {
    await composer.getByRole('combobox', { name: 'Recipient cell' }).click();
    await page.getByRole('option', { name: recipient, exact: true }).click();
  }
  return composer;
}
