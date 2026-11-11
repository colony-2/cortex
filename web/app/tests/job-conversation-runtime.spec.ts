import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';

const job = '/api/projects/history-browser/jobs/reviews';

test('CC-022 CT-014,015,028–031 real SQLite reviews survive restart, archive and hiding in the browser', async ({ page, context, request, baseURL }, info) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'retained@example.test', url: baseURL! }]);
  const list = async () => {
    const response = await request.get(`${job}/conversation`);
    expect(response.ok(), await response.text()).toBeTruthy();
    return response.json();
  };
  const control = async (action: string) => {
    const response = await request.post(`/api/__test/history/${action}`);
    expect(response.status(), await response.text()).toBe(204);
  };
  const accept = async (requestID: string, decision: string) => {
    const response = await request.post('/api/projects/history-browser/reviews/reviews/respond', {
      headers: { Cookie: 'colony2:user:email=retained%40example.test' },
      data: { request_id: requestID, submission_id: `accepted-${requestID}`, fields: { decision } },
    });
    expect(response.ok(), await response.text()).toBeTruthy();
    await control('advance');
  };
  const initial = await list();
  expect(initial.entries).toHaveLength(1);
  const firstID = initial.entries[0].entry_id;
  await page.goto('/project/history-browser/jobs/reviews/story');
  await expect(page.locator(`[data-occurrence="${firstID}"]`).first()).toBeVisible();
  await expect(page.getByText(/State: pending/)).toBeVisible();
  await expect(page.getByRole('region', { name: 'Job completion' })).toHaveCount(0);
  await accept('first-review', 'Approved first original');
  await expect(page.getByLabel('Accepted response', {exact:true})).toContainText('Approved first original');
  const second = await list();
  expect(second.entries).toHaveLength(2);
  expect(second.entries[1].request_id).toBe('second-review');

  // Closing the page also closes its SSE stream before the SQLite connection
  // and history-service instance are replaced over the same retained file.
  await page.close();
  await control('reopen');
  const reopened = await context.newPage();
  await reopened.goto(`/project/history-browser/jobs/reviews/story?occurrenceId=${encodeURIComponent(firstID)}`);
  const original = reopened.getByRole('region', { name: 'Recorded request', exact: true });
  await expect(original.getByLabel('Recorded answers')).toContainText('Approved first original');
  await expect(original.getByLabel('Rendered document')).toContainText('Original first review document, retained in SQLite.');
  await expect(original.getByRole('button', { name: /Submit/ })).toHaveCount(0);
  await expect(reopened.locator(`[data-occurrence="${second.entries[1].entry_id}"]`).first()).toBeVisible();
  const downloadLink = original.getByRole('link', { name: 'Download',exact:true });
  await expect(downloadLink).toHaveAttribute('href', /snapshot_token=/);
  await expect(reopened.getByRole('button',{name:'Cancel job',exact:true})).toHaveCount(0);
  const [download] = await Promise.all([reopened.waitForEvent('download'), downloadLink.click()]);
  expect(await readFile((await download.path())!, 'utf8')).toBe('Original first review document, retained in SQLite.\n');

  await original.screenshot({path:info.outputPath('native-retained-review.png')});

  const stale = await request.post('/api/projects/history-browser/reviews/reviews/respond', {
    headers: { Cookie: 'colony2:user:email=retained%40example.test' },
    data: { request_id: 'first-review', submission_id: 'stale', fields: { decision: 'Stale' } },
  });
  expect(stale.status()).toBe(409);
  await accept('second-review', 'Approved later review');
  // Metadata completion refresh remains independent of execution inspection:
  // the controlled fixture does not supply a compiled execution-tree recipe.
  await expect(reopened.getByRole('region', { name: 'Job completion' })).toContainText('Succeeded');
  await expect(reopened.getByRole('region', { name: 'Job completion' })).not.toContainText('Merge status');
  await expect(original.getByLabel('Recorded answers')).toContainText('Approved first original');
  expect(new URL(reopened.url()).searchParams.get('occurrenceId')).toBe(firstID);
  // CV-025: real SQLite records, Cortex adapters and authorized byte reads.
  await expect(reopened.getByLabel('Verified publication')).toContainText('Commit message not recorded');
  await reopened.screenshot({path:info.outputPath('native-subjectless-publication.png'),animations:'disabled',fullPage:true});
  const finalList = reopened.getByLabel('Returned documents',{exact:true});
  await expect(finalList.getByRole('link',{name:'report.md',exact:true})).toBeVisible();
  await finalList.getByRole('link',{name:'report.md',exact:true}).click();
  const viewer = reopened.getByRole('dialog',{name:'Returned document viewer'});
  await expect(viewer.getByRole('heading',{name:'Final retained report',exact:true})).toBeVisible();
  await reopened.screenshot({path:info.outputPath('native-final-document-viewer.png'),animations:'disabled',fullPage:false});
  await viewer.getByRole('link',{name:'Original notes',exact:true}).click();
  await expect(viewer.getByLabel('Rendered document')).toContainText('Original first review document, retained in SQLite.');
  await viewer.getByRole('button',{name:'Close',exact:true}).click();
  await expect(finalList.getByRole('link',{name:'report.md',exact:true})).toBeFocused();
  await control('hide');
  const visible = await request.get('/api/projects/history-browser/jobs?status=all');
  expect((await visible.json()).jobs.some((entry: { job_id: string }) => entry.job_id === 'reviews')).toBe(false);
  await reopened.reload();
  await expect(original.getByLabel('Recorded answers')).toContainText('Approved first original');
  const detail = await request.get(`${job}/conversation/entry?entry_id=${encodeURIComponent(firstID)}`);
  expect(detail.ok(), await detail.text()).toBeTruthy();
  const manifest = await detail.json();
  const documentURL = `${job}/conversation/artifact?entry_id=${encodeURIComponent(firstID)}&artifact_id=${encodeURIComponent(manifest.request_manifest[0].artifact_id)}`;
  const retained = await request.get(documentURL);
  expect(await retained.text()).toBe('Original first review document, retained in SQLite.\n');
  const denied = await request.get(documentURL.replace('/history-browser/', '/foreign/'));
  expect(denied.status()).toBe(404);
});
