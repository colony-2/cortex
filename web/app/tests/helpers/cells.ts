import { expect, type APIRequestContext, type Page } from '@playwright/test';
export const jobIdentity = { cell_id: 'cell-platform', cell_name: 'platform', repo: 'github.com/acme/platform' };
export const cellFixture = (name: string, tenantId: string) => ({
  id: `cell-${name}`, name, tenantId, repo: `github.com/acme/${name}`, state: 'ACTIVE',
  description: '', metadata: {}, aliases: [], revision: 'r1', createdAt: '2026-11-04T00:00:00Z', updatedAt: '2026-11-04T00:00:00Z',
});
export async function mockCells(page: Page, names: string[]) {
  await page.route(/\/api\/projects\/[^/]+\/cells(?:\/[^/?]+)?(?:\?.*)?$/, route => {
    const url = new URL(route.request().url());
    const parts = url.pathname.split('/');
    const cells = names.map(name => cellFixture(name, decodeURIComponent(parts[3])));
    const selected = parts[5];
    return route.fulfill({ json: selected ? cells.find(cell => cell.id === selected) : { cells: cells.filter(cell => !url.searchParams.get('name') || cell.name === url.searchParams.get('name')) } });
  });
}

export async function registerCell(request: APIRequestContext, tenant: string, name = 'platform', repo = 'github.com/cortex-test/platform') {
  const response = await request.post(`/api/projects/${tenant}/cells`, { data: { name, repo } });
  expect(response.status(), await response.text()).toBe(201);
  return response.json();
}
