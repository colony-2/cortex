import type { Page, TestInfo } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { jobIdentity, mockCells } from './cells';

export const longBase = 'linux-amd64/' + 'a-very-long-base-selector-'.repeat(12) + '*';
export const envJobs = [
  { ...jobIdentity, tenant_id: 'env', job_id: 'ordinary', goal_id: 'ordinary', title: 'Generate the performance report', status: 'READY', store: 'ACTIVE', created_at: '2026-11-19T09:42:00Z', execution: { status: 'specified', source: 'submission', published: false, demand: { schema_version: 4, recipe_resolution: 'resolved', effective: { base: 'macOS-26-arm', resources: { cpu: '4', memory: '8Gi' }, capabilities: ['docker', 'network'], packages: ['nix:nodejs', 'uv:ruff'], python: { 'requires-python': '>=3.12', dependencies: ['requests', 'numpy', 'pillow'] } } } } },
  { ...jobIdentity, tenant_id: 'env', job_id: 'long', goal_id: 'long', title: 'Rebuild the search index', status: 'ACTIVE', store: 'ACTIVE', created_at: '2026-11-19T09:38:00Z', execution: { status: 'in_flight', source: 'unavailable', published: false }, recorded_execution: { status: 'specified', source: 'yield', published: true, demand: { schema_version: 4, recipe_resolution: 'resolved', effective: { base: longBase, resources: { cpu: '500m', memory: '512Mi', 'ephemeral-storage': '20Gi' }, capabilities: ['docker', 'network', 'gpu', 'a-long-capability-name-with-雪'] } } } },
  { ...jobIdentity, tenant_id: 'env', job_id: 'empty', goal_id: 'empty', title: 'Update project documentation', status: 'READY', store: 'ACTIVE', created_at: '2026-11-19T09:31:00Z', execution: { status: 'unspecified', source: 'submission', published: false, demand: { schema_version: 4, recipe_resolution: 'resolved', effective: { resources: {} } } } },
];
export async function routeEnvironment(page: Page) {
  const reads: string[] = [];
  page.on('request', req => { if (new URL(req.url()).pathname.startsWith('/api/')) reads.push(new URL(req.url()).pathname); });
  await mockCells(page, ['platform']);
  await page.route('**/api/projects/env/jobs?*', route => route.fulfill({ json: { jobs: envJobs } }));
  await page.route('**/api/projects/env/jobs/*', route => route.fulfill({ json: envJobs.find(job => route.request().url().split('?')[0].endsWith('/' + job.job_id)) }));
  await page.route('**/api/projects/env/jobs/*/story', route => route.fulfill({ status: 500, json: { error: 'Trace fixture unavailable' } }));
  await page.route('**/api/projects/env/jobs/*/input-artifacts', route => route.fulfill({ json: { artifacts: [] } }));
  await page.route('**/api/projects/env/jobs/*/conversation?*', route => route.fulfill({ status: 501, json: { error: 'No fixture history', code: 'history_unsupported' } }));
  await page.route('**/api/projects/env/jobs/*/lease-events?*', route => route.fulfill({ json: { events: [] } }));
  return reads;
}
export async function captureEnvironment(page: Page, testInfo: TestInfo, name: string) {
  const directory = process.env.CORTEX_ENV_SCREENSHOTS;
  if (!directory) return;
  mkdirSync(directory, { recursive: true });
  await page.screenshot({ path: join(directory, name + '.png'), fullPage: true, animations: 'disabled' });
  await testInfo.attach(name, { path: join(directory, name + '.png'), contentType: 'image/png' });
}
