import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { registerCell } from './helpers/cells';

test('ATT-018 ATT-020 ATT-035 creates a goal with persisted file bytes', async ({ page, request, context, baseURL }) => {
 const tenant = `attachments-${randomUUID()}`;
 const cell = await registerCell(request, tenant);
 await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
 await page.goto(`/project/${tenant}/goals`);
 await page.getByRole('button', { name: 'New goal', exact: true }).click();
 const dialog = page.getByRole('dialog');
 await dialog.getByLabel('Prompt').fill('Use the attached requirements');
 await dialog.getByRole('combobox', { name: 'Cell', exact: true }).click();
 await page.getByTitle(`${cell.name} · ${cell.repo}`, { exact: true }).click();
 const picker = dialog.getByRole('button', { name: 'Attach files' });
 await picker.focus();
 const chooserEvent = page.waitForEvent('filechooser');
 await page.keyboard.press('Enter');
 const chooser = await chooserEvent;
 await chooser.setFiles({ name: 'requirements.bin', mimeType: 'application/octet-stream', buffer: Buffer.from([0, 255, 1, 2]) });
 const remove = dialog.getByRole('button', { name: 'Remove requirements.bin' });
 await remove.focus(); await page.keyboard.press('Enter');
 await expect(remove).toHaveCount(0);
 const uploads = [
  { name: 'requirements.bin', mimeType: 'application/octet-stream', buffer: Buffer.from([0, 255, 1, 2]) },
  { name: 'a"b.txt', mimeType: 'text/plain', buffer: Buffer.from('quoted') },
  { name: 'a%22b.txt', mimeType: 'text/plain', buffer: Buffer.from('literal percent') },
 ];
 await dialog.locator('input[type=file]').setInputFiles(uploads);
 await expect(remove).toBeVisible();
 const resultEvent = page.waitForResponse(response => response.url().endsWith(`/api/projects/${tenant}/jobs`) && response.request().method() === 'POST');
 await dialog.getByRole('button', { name: 'Create goal', exact: true }).click();
 const result = await resultEvent;
 expect(result.status(), await result.text()).toBe(201);
 const job = await result.json();
 await expect(page).toHaveURL(new RegExp(`/goals/${job.goal_id}`));
 for (const upload of uploads) {
 const artifact = await request.get(`/api/projects/${tenant}/jobs/${job.job_id}/tasks/0/artifacts/${encodeURIComponent(upload.name)}`);
 expect(artifact.status(), await artifact.text()).toBe(200);
 expect(await artifact.body()).toEqual(upload.buffer);
 }
});
